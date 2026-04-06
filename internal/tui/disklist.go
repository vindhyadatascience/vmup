package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vds-gcp-launch-instance/internal/config"
	"vds-gcp-launch-instance/internal/gcloud"
)

type diskEntry struct {
	cfg    config.DiskConfig
	status gcloud.DiskStatus
}

type diskListModel struct {
	disks       []diskEntry
	cursor      int
	loading     bool
	loadingText string // custom loading text, empty = default
	spinner     spinner.Model
	lastRefreshed time.Time
	refreshStart  time.Time

	flashMsg     string
	flashIsError bool

	// Layout
	layoutWidth int
	renderWidth int
	height      int
	scrollTop   int

	// Resize debounce
	resizeSeq int

	// Help dialog
	showHelp    bool
	hideHelpBar bool

	// Filter
	filterActive    bool
	filterInput     string
	filterProp      string
	filterValue     string
	filteredIndices  []int
	savedFilterProp  string
	savedFilterValue string
}

// Messages
type diskListLoadedMsg struct {
	disks []diskEntry
}

type diskListActionMsg struct {
	action diskAction
	disk   diskEntry
}

type diskAttachReadyMsg struct {
	disk         diskEntry
	vmNames      []string
	vmUsernames  map[string]string
	vmProjectIDs map[string]string
	formatted    bool
}

type diskAction int

const (
	actionDiskCreate diskAction = iota
	actionDiskImport
	actionDiskDelete
	actionDiskResize
	actionDiskAttach
	actionDiskDetach
)

type diskLayoutDoneMsg struct {
	seq int
}

func newDiskListModel() diskListModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return diskListModel{
		loading:      true,
		refreshStart: time.Now(),
		spinner:      s,
		layoutWidth:  114,
		renderWidth:  114,
		height:       40,
	}
}

func (m diskListModel) Init() tea.Cmd {
	return tea.Batch(loadDiskList, m.spinner.Tick)
}

func loadDiskList() tea.Msg {
	// Load all local disk configs (filesystem, fast)
	names := config.ListDisks()
	var cfgs []config.DiskConfig
	for _, name := range names {
		tfvarsPath := filepath.Join(config.DiskDir(name), "terraform.tfvars")
		cfg, err := config.LoadDiskTFVars(tfvarsPath)
		if err != nil {
			continue
		}
		cfgs = append(cfgs, cfg)
	}

	// Collect unique project IDs
	projectSet := make(map[string]bool)
	for _, cfg := range cfgs {
		projectSet[cfg.ProjectID] = true
	}

	// Fetch disks and instances per project in parallel
	type projectData struct {
		disks     map[string]gcloud.DiskInfo
		instances map[string]gcloud.InstanceInfo
	}
	projectResults := make(map[string]*projectData)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for project := range projectSet {
		projectResults[project] = &projectData{}
		wg.Add(2)

		go func(p string) {
			defer wg.Done()
			disks, err := gcloud.FetchDisksByProject(p)
			if err != nil {
				disks = make(map[string]gcloud.DiskInfo)
			}
			mu.Lock()
			projectResults[p].disks = disks
			mu.Unlock()
		}(project)

		go func(p string) {
			defer wg.Done()
			instances, err := gcloud.FetchInstancesByProject(p)
			if err != nil {
				instances = make(map[string]gcloud.InstanceInfo)
			}
			mu.Lock()
			projectResults[p].instances = instances
			mu.Unlock()
		}(project)
	}
	wg.Wait()

	// Build disk entries from batch results
	disks := make([]diskEntry, len(cfgs))
	for i, cfg := range cfgs {
		status := gcloud.DiskStatus{Status: "UNKNOWN"}
		if pd, ok := projectResults[cfg.ProjectID]; ok {
			if info, ok := pd.disks[cfg.Name]; ok {
				status = gcloud.DiskStatus{
					Status:   info.Status,
					Users:    info.Users,
					SizeGB:   info.SizeGB,
					DiskType: info.DiskType,
				}
				// Get attachment mode from instance data
				if len(info.Users) > 0 {
					if instInfo, ok := pd.instances[info.Users[0]]; ok {
						for _, ad := range instInfo.Disks {
							if ad.DiskName == cfg.Name {
								status.Mode = ad.Mode
							}
						}
					}
				}
			}
		}
		disks[i] = diskEntry{cfg: cfg, status: status}
	}

	return diskListLoadedMsg{disks: disks}
}

// visibleDiskTableRows returns how many disks fit on screen in table mode.
func (m diskListModel) visibleTableRows() int {
	// Overhead: title(2) + tab bar(2) + refresh(2) + header+sep(2) + detail(~4) + flash(2) + help(1) = ~15
	v := m.height - 15
	if v < 1 {
		v = 1
	}
	return v
}

func (m diskListModel) visibleCards() int {
	// Overhead: title(2) + tab bar(2) + refresh(2) + flash(2) + help(1) = ~9
	// Each card: ~4 lines. Use 4 as estimate.
	v := (m.height - 9) / 4
	if v < 1 {
		v = 1
	}
	return v
}

func (m *diskListModel) adjustScroll() {
	var visible int
	if m.layoutWidth >= 80 {
		visible = m.visibleTableRows()
	} else {
		visible = m.visibleCards()
	}
	if m.cursor < m.scrollTop {
		m.scrollTop = m.cursor
	}
	if m.cursor >= m.scrollTop+visible {
		m.scrollTop = m.cursor - visible + 1
	}
	if m.scrollTop < 0 {
		m.scrollTop = 0
	}
	maxScroll := m.displayCount() - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollTop > maxScroll {
		m.scrollTop = maxScroll
	}
}

// --- Filter helpers ---

func (m diskListModel) displayCount() int {
	if m.filteredIndices != nil {
		return len(m.filteredIndices)
	}
	return len(m.disks)
}

func (m diskListModel) displayDisk(i int) diskEntry {
	if m.filteredIndices != nil {
		return m.disks[m.filteredIndices[i]]
	}
	return m.disks[i]
}

func (m diskListModel) hasFilter() bool {
	return m.filterProp != "" || m.filterValue != ""
}

func diskFieldValue(disk diskEntry, prop string) string {
	switch prop {
	case "name", "diskname":
		return disk.cfg.Name
	case "project", "projectid":
		return disk.cfg.ProjectID
	case "zone":
		return disk.cfg.Zone
	case "type", "disktype":
		return disk.cfg.DiskType
	case "size", "sizegb":
		if disk.status.SizeGB != "" {
			return disk.status.SizeGB + " GB"
		}
		return disk.cfg.SizeGB + " GB"
	case "status":
		return disk.status.Status
	case "attached", "attachedto", "user":
		return strings.Join(disk.status.Users, ", ")
	default:
		return ""
	}
}

// diskFilterProps is the canonical list of property names used for global search.
// Add new entries here when adding filterable fields to diskFieldValue.
var diskFilterProps = []string{
	"name", "project", "zone", "type", "size", "status", "attached",
}

func diskMatchesAny(disk diskEntry, query string) bool {
	for _, prop := range diskFilterProps {
		val := strings.ToLower(diskFieldValue(disk, prop))
		if val != "" && strings.Contains(val, query) {
			return true
		}
	}
	return false
}

func (m *diskListModel) recomputeFilter() {
	if m.filterProp == "" && m.filterValue == "" {
		m.filteredIndices = nil
		return
	}
	// Must use a non-nil empty slice so displayCount() knows a filter is active
	if m.filteredIndices == nil {
		m.filteredIndices = []int{}
	}
	m.filteredIndices = m.filteredIndices[:0]
	query := strings.ToLower(m.filterValue)
	for i, disk := range m.disks {
		if m.filterProp != "" {
			val := strings.ToLower(diskFieldValue(disk, m.filterProp))
			if strings.Contains(val, query) {
				m.filteredIndices = append(m.filteredIndices, i)
			}
		} else {
			if diskMatchesAny(disk, query) {
				m.filteredIndices = append(m.filteredIndices, i)
			}
		}
	}
}

func (m *diskListModel) applyFilterInput() {
	raw := strings.TrimSpace(m.filterInput)
	if raw == "" {
		m.filterProp = ""
		m.filterValue = ""
		m.filteredIndices = nil
	} else if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		m.filterProp = ""
		m.filterValue = strings.Trim(raw, "\"")
		m.recomputeFilter()
	} else {
		parts := strings.SplitN(raw, " ", 2)
		if len(parts) == 2 {
			m.filterProp = strings.ToLower(parts[0])
			m.filterValue = parts[1]
		} else {
			m.filterProp = ""
			m.filterValue = parts[0]
		}
		m.recomputeFilter()
	}
	m.cursor = 0
	m.scrollTop = 0
	m.adjustScroll()
}

func (m diskListModel) viewFilterInput() string {
	var b strings.Builder
	w := m.renderWidth
	if w < 30 {
		w = 30
	}
	sep := dimStyle.Render(strings.Repeat("─", w))
	prompt := palettePromptStyle.Render("/")

	b.WriteString(sep + "\n")
	if m.filterInput == "" {
		b.WriteString("  " + prompt + dimStyle.Render("[property] [term]") + "\n")
	} else {
		b.WriteString("  " + prompt + statusValStyle.Render(m.filterInput) + dimStyle.Render("▎") + "\n")
	}
	b.WriteString(sep + "\n")
	b.WriteString(dimStyle.Render("tab next • enter apply • esc cancel"))
	return b.String()
}

func (m diskListModel) viewFilterIndicator() string {
	filterText := m.filterValue
	if m.filterProp != "" {
		filterText = m.filterProp + " " + m.filterValue
	}
	count := m.displayCount()
	total := len(m.disks)
	return infoStyle.Render(fmt.Sprintf("filter: %s (%d/%d)", filterText, count, total)) +
		dimStyle.Render(" • / edit • esc clear")
}

func (m diskListModel) Update(msg tea.Msg) (diskListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.renderWidth = msg.Width
		m.resizeSeq++
		seq := m.resizeSeq
		m.adjustScroll()
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
			return diskLayoutDoneMsg{seq: seq}
		})

	case diskLayoutDoneMsg:
		if msg.seq == m.resizeSeq {
			m.layoutWidth = m.renderWidth
			m.adjustScroll()
		}
		return m, nil

	case diskListLoadedMsg:
		m.disks = msg.disks
		m.loading = false
		m.lastRefreshed = time.Now()
		m.recomputeFilter()
		if m.cursor >= m.displayCount() && m.displayCount() > 0 {
			m.cursor = m.displayCount() - 1
		}
		m.adjustScroll()
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		// Filter input mode — capture all keys
		if m.filterActive {
			switch msg.Type {
			case tea.KeyBackspace:
				if len(m.filterInput) > 0 {
					m.filterInput = m.filterInput[:len(m.filterInput)-1]
				} else {
					// Backspace on empty — fully clear filter and dismiss
					m.filterActive = false
					m.filterProp = ""
					m.filterValue = ""
					m.filteredIndices = nil
					m.cursor = 0
					m.scrollTop = 0
					m.adjustScroll()
					return m, nil
				}
			case tea.KeyEnter:
				// Commit: keep current dynamic filter, just exit filter mode
				m.filterActive = false
				return m, nil
			case tea.KeyEscape, tea.KeyCtrlC:
				// Cancel: restore previously committed filter
				m.filterActive = false
				m.filterProp = m.savedFilterProp
				m.filterValue = m.savedFilterValue
				m.recomputeFilter()
				m.cursor = 0
				m.scrollTop = 0
				m.adjustScroll()
				return m, nil
			case tea.KeyTab:
				m.filterInput += " "
			case tea.KeySpace:
				m.filterInput += " "
			case tea.KeyRunes:
				m.filterInput += string(msg.Runes)
			default:
				return m, nil
			}
			// Apply filter dynamically on every change
			m.applyFilterInput()
			return m, nil
		}

		// Dismiss help dialog on any key
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		// Clear committed filter on Backspace or Esc
		if (msg.Type == tea.KeyBackspace || msg.Type == tea.KeyEscape) && m.hasFilter() {
			m.filterProp = ""
			m.filterValue = ""
			m.filteredIndices = nil
			m.cursor = 0
			m.scrollTop = 0
			m.adjustScroll()
			return m, nil
		}

		m.flashMsg = ""
		m.flashIsError = false

		if m.loading && len(m.disks) == 0 {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		if m.displayCount() == 0 {
			switch msg.String() {
			case "n":
				return m, func() tea.Msg {
					return diskListActionMsg{action: actionDiskCreate}
				}
			case "I":
				return m, func() tea.Msg {
					return diskListActionMsg{action: actionDiskImport}
				}
			case "?":
				m.showHelp = true
				return m, nil
			case "r":
				m.loading = true
				m.refreshStart = time.Now()
				return m, tea.Batch(loadDiskList, m.spinner.Tick)
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}
		case "down", "j":
			if m.cursor < m.displayCount()-1 {
				m.cursor++
				m.adjustScroll()
			}
		case "n":
			return m, func() tea.Msg {
				return diskListActionMsg{action: actionDiskCreate}
			}
		case "I":
			return m, func() tea.Msg {
				return diskListActionMsg{action: actionDiskImport}
			}
		case "D":
			disk := m.displayDisk(m.cursor)
			if len(disk.status.Users) > 0 {
				m.flashMsg = "Disk must be detached before deletion"
				m.flashIsError = true
				return m, nil
			}
			return m, func() tea.Msg {
				return diskListActionMsg{action: actionDiskDelete, disk: disk}
			}
		case "e":
			disk := m.displayDisk(m.cursor)
			return m, func() tea.Msg {
				return diskListActionMsg{action: actionDiskResize, disk: disk}
			}
		case "a":
			disk := m.displayDisk(m.cursor)
			if len(disk.status.Users) > 0 && disk.status.Mode == "READ_WRITE" {
				m.flashMsg = fmt.Sprintf("Disk is already attached in read/write mode to %s", disk.status.Users[0])
				m.flashIsError = true
				return m, nil
			}
			return m, func() tea.Msg {
				return diskListActionMsg{action: actionDiskAttach, disk: disk}
			}
		case "d":
			disk := m.displayDisk(m.cursor)
			if len(disk.status.Users) == 0 {
				m.flashMsg = "Disk is not attached to any instance"
				m.flashIsError = true
				return m, nil
			}
			return m, func() tea.Msg {
				return diskListActionMsg{action: actionDiskDetach, disk: disk}
			}
		case "r":
			m.loading = true
			m.refreshStart = time.Now()
			return m, tea.Batch(loadDiskList, m.spinner.Tick)
		case "?":
			m.showHelp = true
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// --- Layout helpers ---

type diskColumnWidths struct {
	name, project, size, zone, diskType, status, attached int
}

func calcDiskColumnWidths(totalWidth int) diskColumnWidths {
	avail := totalWidth - 2 // cursor prefix
	if avail < 60 {
		avail = 60
	}
	cw := diskColumnWidths{
		name:     max(avail*20/100, 14),
		project:  max(avail*16/100, 10),
		size:     max(avail*7/100, 7),
		zone:     max(avail*15/100, 10),
		diskType: max(avail*11/100, 10),
		status:   max(avail*10/100, 8),
	}
	cw.attached = avail - cw.name - cw.project - cw.size - cw.zone - cw.diskType - cw.status
	if cw.attached < 10 {
		cw.attached = 10
	}
	return cw
}

func diskStatusColorStyle(status string, base lipgloss.Style) lipgloss.Style {
	switch status {
	case "READY":
		return base.Foreground(lipgloss.Color("42"))
	case "CREATING", "RESTORING":
		return base.Foreground(lipgloss.Color("214"))
	case "DELETING", "FAILED":
		return base.Foreground(lipgloss.Color("196"))
	default:
		return base.Foreground(lipgloss.Color("241"))
	}
}

// --- View ---

func (m diskListModel) ViewContent() string {
	var b strings.Builder

	if m.loading && len(m.disks) == 0 {
		text := "Loading managed data disks..."
		if m.loadingText != "" {
			text = m.loadingText
		}
		b.WriteString(m.spinner.View() + " " + dimStyle.Render(fmt.Sprintf("%s (%s)", text, formatElapsed(time.Since(m.refreshStart)))))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("q quit • ctrl+c quit"))
		return b.String()
	}

	if m.loading {
		text := "Refreshing..."
		if m.loadingText != "" {
			text = m.loadingText
		}
		b.WriteString(m.spinner.View() + " " + dimStyle.Render(fmt.Sprintf("%s (%s)", text, formatElapsed(time.Since(m.refreshStart)))))
		b.WriteString("\n\n")
	}

	if len(m.disks) == 0 && !m.hasFilter() {
		b.WriteString(dimStyle.Render("No managed data disks found."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press n to create a new data disk, or I to import an existing one."))

		if m.flashMsg != "" {
			b.WriteString("\n")
			if m.flashIsError {
				b.WriteString(errorStyle.Render(m.flashMsg))
			} else {
				b.WriteString(successStyle.Render(m.flashMsg))
			}
		}

		b.WriteString("\n")
		b.WriteString(m.helpBar())
		return b.String()
	}

	if m.displayCount() == 0 && m.hasFilter() {
		b.WriteString(dimStyle.Render("No matching items. / to edit filter • esc or backspace to clear"))
		if m.filterActive {
			b.WriteString("\n")
			b.WriteString(m.viewFilterInput())
		} else {
			b.WriteString("\n")
			b.WriteString(m.viewFilterIndicator())
		}
		return b.String()
	}

	if m.layoutWidth >= 80 {
		b.WriteString(m.viewTable())
	} else {
		b.WriteString(m.viewCards())
	}

	if m.flashMsg != "" {
		b.WriteString("\n")
		if m.flashIsError {
			b.WriteString(errorStyle.Render(m.flashMsg))
		} else {
			b.WriteString(successStyle.Render(m.flashMsg))
		}
	}

	if m.showHelp {
		b.WriteString("\n")
		b.WriteString(m.viewHelpDialog())
		return b.String()
	}

	if m.filterActive {
		b.WriteString("\n")
		b.WriteString(m.viewFilterInput())
	} else if m.hasFilter() {
		b.WriteString("\n")
		b.WriteString(m.viewFilterIndicator())
		if !m.hideHelpBar {
			b.WriteString("\n")
			b.WriteString(m.helpBar())
		}
	} else if !m.hideHelpBar {
		b.WriteString("\n")
		b.WriteString(m.helpBar())
	}

	return b.String()
}

func (m diskListModel) viewTable() string {
	var b strings.Builder
	w := m.renderWidth
	cw := calcDiskColumnWidths(w)

	sName := lipgloss.NewStyle().Width(cw.name).Foreground(lipgloss.Color("255"))
	sProject := lipgloss.NewStyle().Width(cw.project).Foreground(lipgloss.Color("255"))
	sSize := lipgloss.NewStyle().Width(cw.size).Foreground(lipgloss.Color("255"))
	sZone := lipgloss.NewStyle().Width(cw.zone).Foreground(lipgloss.Color("255"))
	sType := lipgloss.NewStyle().Width(cw.diskType).Foreground(lipgloss.Color("255"))
	sStatus := lipgloss.NewStyle().Width(cw.status).Foreground(lipgloss.Color("255"))
	sAttached := lipgloss.NewStyle().Width(cw.attached).Foreground(lipgloss.Color("255"))

	sep := dimStyle.Render(strings.Repeat("─", w))

	// Header
	header := fmt.Sprintf("  %s%s%s%s%s%s%s",
		headerStyle.Render(sName.Render("Disk Name")),
		headerStyle.Render(sProject.Render("Project")),
		headerStyle.Render(sSize.Render("Size")),
		headerStyle.Render(sZone.Render("Zone")),
		headerStyle.Render(sType.Render("Type")),
		headerStyle.Render(sStatus.Render("Status")),
		headerStyle.Render(sAttached.Render("Attached To")),
	)
	b.WriteString(header + "\n")
	b.WriteString(sep + "\n")

	// Scroll bounds
	visible := m.visibleTableRows()
	count := m.displayCount()
	end := m.scrollTop + visible
	if end > count {
		end = count
	}

	if m.scrollTop > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.scrollTop)) + "\n")
	}

	// Rows
	for i := m.scrollTop; i < end; i++ {
		disk := m.displayDisk(i)
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		name := sName.Render(truncate(disk.cfg.Name, cw.name-2))
		project := sProject.Render(truncate(disk.cfg.ProjectID, cw.project-2))
		size := sSize.Render(disk.cfg.SizeGB + " GB")
		zone := sZone.Render(truncate(disk.cfg.Zone, cw.zone-2))
		diskType := sType.Render(truncate(disk.cfg.DiskType, cw.diskType-2))
		status := diskStatusColorStyle(disk.status.Status, sStatus).Render(disk.status.Status)

		attachedTo := "—"
		if len(disk.status.Users) > 0 {
			attachedTo = strings.Join(disk.status.Users, ", ")
		}
		attached := sAttached.Render(truncate(attachedTo, cw.attached-2))

		row := fmt.Sprintf("%s%s%s%s%s%s%s%s", cursor, name, project, size, zone, diskType, status, attached)
		if i == m.cursor {
			row = rowSelected.Render(row)
		}
		b.WriteString(row + "\n")
	}

	if end < count {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more below", count-end)) + "\n")
	}

	// Detail section for selected disk
	if m.cursor < count {
		disk := m.displayDisk(m.cursor)
		b.WriteString("\n" + sep + "\n")
		detail := func(k, v string) {
			b.WriteString(statusKeyStyle.Render(k) + statusValStyle.Render(v) + "\n")
		}
		detail("Disk Name:", disk.cfg.Name)
		detail("Zone:", disk.cfg.Zone)
		detail("Type:", disk.cfg.DiskType)
		if disk.status.SizeGB != "" {
			detail("Size:", disk.status.SizeGB+" GB")
		} else {
			detail("Size:", disk.cfg.SizeGB+" GB")
		}
		detail("Status:", disk.status.Status)
		if len(disk.status.Users) > 0 {
			detail("Attached To:", strings.Join(disk.status.Users, ", "))
		} else {
			detail("Attached To:", "not attached")
		}
	}

	return b.String()
}

func (m diskListModel) viewCards() string {
	var b strings.Builder
	w := m.renderWidth
	if w < 30 {
		w = 30
	}
	sep := dimStyle.Render(strings.Repeat("─", w))
	noWidthStatus := lipgloss.NewStyle()

	// Scroll bounds
	visible := m.visibleCards()
	count := m.displayCount()
	end := m.scrollTop + visible
	if end > count {
		end = count
	}

	if m.scrollTop > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.scrollTop)) + "\n")
	}

	for i := m.scrollTop; i < end; i++ {
		disk := m.displayDisk(i)
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		nameStr := truncate(disk.cfg.Name, w-25)
		statusText := disk.status.Status
		statusStyled := diskStatusColorStyle(disk.status.Status, noWidthStatus).Render(statusText)

		nameWidth := len(cursor) + len(nameStr)
		gap := w - nameWidth - len(statusText)
		if gap < 1 {
			gap = 1
		}
		headerLine := cursor + nameStr + strings.Repeat(" ", gap) + statusStyled
		if i == m.cursor {
			headerLine = rowSelected.Render(headerLine)
		}
		b.WriteString(headerLine + "\n")

		indent := "    "
		sizeGB := disk.cfg.SizeGB
		if disk.status.SizeGB != "" {
			sizeGB = disk.status.SizeGB
		}
		b.WriteString(indent + cardLabel.Render("Project:") + disk.cfg.ProjectID + "\n")
		b.WriteString(indent + cardLabel.Render("Size:") + sizeGB + " GB\n")
		b.WriteString(indent + cardLabel.Render("Zone:") + disk.cfg.Zone + "\n")

		if i == m.cursor {
			b.WriteString(indent + cardLabel.Render("Type:") + disk.cfg.DiskType + "\n")
			if len(disk.status.Users) > 0 {
				b.WriteString(indent + cardLabel.Render("Attached:") + strings.Join(disk.status.Users, ", ") + "\n")
			} else {
				b.WriteString(indent + cardLabel.Render("Attached:") + "not attached\n")
			}
		}

		b.WriteString(sep + "\n")
	}

	if end < count {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more below", count-end)) + "\n")
	}

	return b.String()
}

func (m diskListModel) helpBar() string {
	return dimStyle.Render("↑/↓/←/→ navigate • : command • / filter • r refresh • ? help")
}

func (m diskListModel) viewHelpDialog() string {
	commands := []struct{ key, desc string }{
		{"↑/↓/j/k", "Navigate list"},
		{"←/→/h/l", "Switch tabs"},
		{"tab", "Next tab"},
		{"shift+tab", "Previous tab"},
		{"1/2", "Jump to tab"},
		{"n", "Create a new disk"},
		{"I", "Import existing disk"},
		{"e", "Resize disk"},
		{"a", "Attach disk to VM"},
		{"d", "Detach disk from VM"},
		{"D", "Delete disk"},
		{":", "Command palette"},
		{"/", "Filter list"},
		{"r", "Refresh list"},
		{"p", "View progress"},
		{",", "Settings"},
		{"q", "Quit"},
		{"esc", "Back"},
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).Width(12)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	var lines []string
	for _, c := range commands {
		lines = append(lines, "  "+keyStyle.Render(c.key)+descStyle.Render(c.desc))
	}
	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("99")).
		Padding(1, 2).
		Render(headerStyle.Render("Data Disk Commands") + "\n\n" + content + "\n\n" + dimStyle.Render("Press any key to close"))

	return box
}

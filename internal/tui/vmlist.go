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
	"vds-gcp-launch-instance/internal/tunnel"
)

type vmEntry struct {
	cfg             config.Config
	status          string
	attachedDisks   []string            // display strings like "disk-name (50 GB)"
	attachedDiskCfg []config.DiskConfig // raw configs for attached disks
}

type vmListModel struct {
	vms         []vmEntry
	cursor      int
	loading     bool
	loadingText string // custom loading text, empty = default
	spinner     spinner.Model
	flashMsg     string
	flashIsError bool
	tunnelMgr    *tunnel.Manager
	lastRefreshed time.Time
	refreshStart  time.Time

	// Layout
	layoutWidth int // debounced width — controls table vs card mode
	renderWidth int // immediate width — controls content sizing
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
	filteredIndices []int
}

// Messages
type vmListLoadedMsg struct {
	vms []vmEntry
}

type vmListActionMsg struct {
	action menuAction
	cfg    config.Config
}

type vmAttachDisksReadyMsg struct {
	vmCfg     config.Config
	diskNames []string // managed disk names in same project/zone
	diskCfgs  map[string]config.DiskConfig
}

type vmDetachDiskMsg struct {
	vmCfg    config.Config
	diskCfgs []config.DiskConfig
}

type resizeDoneMsg struct {
	seq int
}

type logoTickMsg struct{}

func logoTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return logoTickMsg{}
	})
}

func newVMListModel(tm *tunnel.Manager) vmListModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return vmListModel{
		loading:      true,
		refreshStart: time.Now(),
		spinner:      s,
		tunnelMgr:    tm,
		layoutWidth:  114,
		renderWidth:  114,
		height:       40,
	}
}

func (m vmListModel) Init() tea.Cmd {
	return tea.Batch(loadVMList, m.spinner.Tick)
}

func loadVMList() tea.Msg {
	// Load all local VM configs (filesystem, fast)
	names := config.ListProjects()
	var vmCfgs []config.Config
	for _, name := range names {
		tfvarsPath := filepath.Join(config.ProjectDir(name), "terraform.tfvars")
		cfg, err := config.LoadTFVars(tfvarsPath)
		if err != nil {
			continue
		}
		vmCfgs = append(vmCfgs, cfg)
	}

	// Load all local disk configs (filesystem, fast)
	diskNames := config.ListDisks()
	var diskCfgs []config.DiskConfig
	for _, name := range diskNames {
		tfvarsPath := filepath.Join(config.DiskDir(name), "terraform.tfvars")
		cfg, err := config.LoadDiskTFVars(tfvarsPath)
		if err != nil {
			continue
		}
		diskCfgs = append(diskCfgs, cfg)
	}

	// Collect unique project IDs from both VMs and disks
	projectSet := make(map[string]bool)
	for _, cfg := range vmCfgs {
		projectSet[cfg.ProjectID] = true
	}
	for _, cfg := range diskCfgs {
		projectSet[cfg.ProjectID] = true
	}

	// Fetch instances and disks per project in parallel
	type projectData struct {
		instances map[string]gcloud.InstanceInfo
		disks     map[string]gcloud.DiskInfo
	}
	projectResults := make(map[string]*projectData)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for project := range projectSet {
		projectResults[project] = &projectData{}
		wg.Add(2)

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
	}
	wg.Wait()

	// Build VM entries with statuses from batch results
	vms := make([]vmEntry, len(vmCfgs))
	for i, cfg := range vmCfgs {
		status := "UNKNOWN"
		if pd, ok := projectResults[cfg.ProjectID]; ok {
			if info, ok := pd.instances[cfg.VMName]; ok {
				status = info.Status
			}
		}
		vms[i] = vmEntry{cfg: cfg, status: status}
	}

	// Cross-reference attached disks to VMs using batch data
	for _, cfg := range diskCfgs {
		pd, ok := projectResults[cfg.ProjectID]
		if !ok {
			continue
		}
		diskInfo, ok := pd.disks[cfg.Name]
		if !ok {
			continue
		}
		for _, user := range diskInfo.Users {
			for i := range vms {
				if user == vms[i].cfg.VMName {
					sizeGB := cfg.SizeGB
					if diskInfo.SizeGB != "" {
						sizeGB = diskInfo.SizeGB
					}
					// Get attachment mode from instance data
					mode := "rw"
					if instPD, ok := projectResults[cfg.ProjectID]; ok {
						if instInfo, ok := instPD.instances[user]; ok {
							for _, ad := range instInfo.Disks {
								if ad.DiskName == cfg.Name && ad.Mode == "READ_ONLY" {
									mode = "ro"
								}
							}
						}
					}
					vms[i].attachedDisks = append(vms[i].attachedDisks, fmt.Sprintf("%s (%s GB, %s)", cfg.Name, sizeGB, mode))
					vms[i].attachedDiskCfg = append(vms[i].attachedDiskCfg, cfg)
				}
			}
		}
	}

	return vmListLoadedMsg{vms: vms}
}

// visibleVMs returns how many VMs fit on screen in table mode.
func (m vmListModel) visibleTableRows() int {
	// Overhead: title(2) + tab bar(2) + refresh(2) + header+sep(2) + detail(~8) + flash(2) + help(1) = ~19
	v := m.height - 19
	if v < 1 {
		v = 1
	}
	return v
}

// visibleCards returns how many cards fit on screen.
func (m vmListModel) visibleCards() int {
	// Overhead: title(2) + tab bar(2) + refresh(2) + flash(2) + help(1) = ~9
	// Each card: ~5 lines non-selected, ~8 selected. Use 5 as estimate.
	v := (m.height - 9) / 5
	if v < 1 {
		v = 1
	}
	return v
}

func (m *vmListModel) adjustScroll() {
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

func (m vmListModel) displayCount() int {
	if m.filteredIndices != nil {
		return len(m.filteredIndices)
	}
	return len(m.vms)
}

func (m vmListModel) displayVM(i int) vmEntry {
	if m.filteredIndices != nil {
		return m.vms[m.filteredIndices[i]]
	}
	return m.vms[i]
}

func (m vmListModel) hasFilter() bool {
	return m.filterProp != "" || m.filterValue != ""
}

func vmFieldValue(vm vmEntry, prop string) string {
	switch prop {
	case "name", "vm", "vmname":
		return vm.cfg.VMName
	case "project", "projectid":
		return vm.cfg.ProjectID
	case "zone":
		return vm.cfg.Zone
	case "region":
		return vm.cfg.Region
	case "machine", "machinetype", "type":
		return vm.cfg.MachineType
	case "image":
		return vm.cfg.Image
	case "status":
		return vm.status
	case "user", "username":
		return vm.cfg.Username
	case "bootdisk", "boot":
		return vm.cfg.BootDiskSize + " GB"
	case "disk", "disks", "datadisk", "datadisks":
		return strings.Join(vm.attachedDisks, ", ")
	case "port", "ports", "portmapping":
		return vm.cfg.PortMapping
	default:
		return ""
	}
}

// vmFilterProps is the canonical list of property names used for global search.
// Add new entries here when adding filterable fields to vmFieldValue.
var vmFilterProps = []string{
	"name", "project", "zone", "region", "machine", "image",
	"status", "user", "bootdisk", "disks", "port",
}

func vmMatchesAny(vm vmEntry, query string) bool {
	for _, prop := range vmFilterProps {
		val := strings.ToLower(vmFieldValue(vm, prop))
		if val != "" && strings.Contains(val, query) {
			return true
		}
	}
	return false
}

func (m *vmListModel) recomputeFilter() {
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
	for i, vm := range m.vms {
		if m.filterProp != "" {
			val := strings.ToLower(vmFieldValue(vm, m.filterProp))
			if strings.Contains(val, query) {
				m.filteredIndices = append(m.filteredIndices, i)
			}
		} else {
			if vmMatchesAny(vm, query) {
				m.filteredIndices = append(m.filteredIndices, i)
			}
		}
	}
}

func (m vmListModel) viewFilterInput() string {
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

func (m vmListModel) viewFilterIndicator() string {
	filterText := m.filterValue
	if m.filterProp != "" {
		filterText = m.filterProp + " " + m.filterValue
	}
	count := m.displayCount()
	total := len(m.vms)
	return infoStyle.Render(fmt.Sprintf("filter: %s (%d/%d)", filterText, count, total)) +
		dimStyle.Render(" • / edit • esc clear")
}

func (m vmListModel) Update(msg tea.Msg) (vmListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.renderWidth = msg.Width
		m.resizeSeq++
		seq := m.resizeSeq
		m.adjustScroll()
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
			return resizeDoneMsg{seq: seq}
		})

	case resizeDoneMsg:
		if msg.seq == m.resizeSeq {
			m.layoutWidth = m.renderWidth
			m.adjustScroll()
		}
		return m, nil

	case vmListLoadedMsg:
		m.vms = msg.vms
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
			case tea.KeyEnter:
				m.filterActive = false
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
				return m, nil
			case tea.KeyEscape, tea.KeyCtrlC:
				m.filterActive = false
				// Re-apply committed filter if any
				m.recomputeFilter()
				m.cursor = 0
				m.scrollTop = 0
				m.adjustScroll()
				return m, nil
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
				}
				return m, nil
			case tea.KeyTab:
				m.filterInput += " "
				return m, nil
			case tea.KeySpace:
				m.filterInput += " "
				return m, nil
			case tea.KeyRunes:
				m.filterInput += string(msg.Runes)
				return m, nil
			}
			return m, nil
		}

		// Dismiss help dialog on any key
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		// Clear committed filter on Esc or Backspace
		if (msg.Type == tea.KeyEscape || msg.Type == tea.KeyBackspace) && m.hasFilter() {
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

		if m.loading && len(m.vms) == 0 {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		if m.displayCount() == 0 {
			switch msg.String() {
			case "n":
				return m, func() tea.Msg {
					return vmListActionMsg{action: actionLaunch}
				}
			case "?":
				m.showHelp = true
				return m, nil
			case "r":
				m.loading = true
				m.refreshStart = time.Now()
				return m, tea.Batch(loadVMList, m.spinner.Tick)
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
				return vmListActionMsg{action: actionLaunch}
			}
		case "s":
			vm := m.displayVM(m.cursor)
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionStartTunnels, cfg: vm.cfg}
			}
		case "x":
			vm := m.displayVM(m.cursor)
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionStopTunnels, cfg: vm.cfg}
			}
		case "X":
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionStopAll}
			}
		case "c":
			vm := m.displayVM(m.cursor)
			if vm.status != "RUNNING" {
				m.flashMsg = fmt.Sprintf("SSH requires a running VM (current status: %s). Use 's' to start it first.", vm.status)
				m.flashIsError = true
				return m, nil
			}
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionSSH, cfg: vm.cfg}
			}
		case "a":
			vm := m.displayVM(m.cursor)
			if vm.status != "RUNNING" {
				m.flashMsg = fmt.Sprintf("VM must be running to attach a disk (current status: %s)", vm.status)
				m.flashIsError = true
				return m, nil
			}
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionAttachDiskToVM, cfg: vm.cfg}
			}
		case "d":
			vm := m.displayVM(m.cursor)
			if len(vm.attachedDiskCfg) == 0 {
				m.flashMsg = "No data disks attached to this instance"
				m.flashIsError = true
				return m, nil
			}
			return m, func() tea.Msg {
				return vmDetachDiskMsg{vmCfg: vm.cfg, diskCfgs: vm.attachedDiskCfg}
			}
		case "D":
			vm := m.displayVM(m.cursor)
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionDestroy, cfg: vm.cfg}
			}
		case "e":
			vm := m.displayVM(m.cursor)
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionEdit, cfg: vm.cfg}
			}
		case "i":
			vm := m.displayVM(m.cursor)
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionInfo, cfg: vm.cfg}
			}
		case "r":
			m.loading = true
			m.refreshStart = time.Now()
			return m, tea.Batch(loadVMList, m.spinner.Tick)
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

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	rowSelected = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	cardLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Width(12)
)

type columnWidths struct {
	name, project, zone, machine, status int
}

func calcColumnWidths(totalWidth int) columnWidths {
	avail := totalWidth - 2 // cursor prefix
	if avail < 60 {
		avail = 60
	}
	cw := columnWidths{
		name:    max(avail*27/100, 15),
		project: max(avail*21/100, 12),
		zone:    max(avail*18/100, 10),
		machine: max(avail*14/100, 8),
	}
	cw.status = avail - cw.name - cw.project - cw.zone - cw.machine
	if cw.status < 15 {
		cw.status = 15
	}
	return cw
}

func statusColorStyle(status string, base lipgloss.Style) lipgloss.Style {
	switch status {
	case "RUNNING":
		return base.Foreground(lipgloss.Color("42"))
	case "STOPPED", "TERMINATED":
		return base.Foreground(lipgloss.Color("196"))
	case "STAGING", "PROVISIONING", "STOPPING", "SUSPENDING":
		return base.Foreground(lipgloss.Color("214"))
	default:
		return base.Foreground(lipgloss.Color("241"))
	}
}

func (m vmListModel) statusText(vm vmEntry) string {
	s := vm.status
	if m.tunnelMgr != nil && vm.status == "RUNNING" {
		if count := m.tunnelMgr.TunnelCount(vm.cfg.VMName); count == 1 {
			s = fmt.Sprintf("%s (1 tunnel)", vm.status)
		} else if count > 1 {
			s = fmt.Sprintf("%s (%d tunnels)", vm.status, count)
		}
	}
	return s
}

// --- Logo ---

// gradientStops are the RGB values for the animated title gradient.
var gradientStops = [3][3]float64{
	{74, 92, 199},  // #4a5cc7 blue
	{123, 76, 181}, // #7b4cb5 purple
	{181, 54, 148}, // #b53694 magenta
}

func gradientColor(t float64) lipgloss.Color {
	t = t - float64(int(t)) // wrap to 0–1
	seg := t * 3
	idx := int(seg) % 3
	frac := seg - float64(int(seg))
	next := (idx + 1) % 3

	r := gradientStops[idx][0] + (gradientStops[next][0]-gradientStops[idx][0])*frac
	g := gradientStops[idx][1] + (gradientStops[next][1]-gradientStops[idx][1])*frac
	b := gradientStops[idx][2] + (gradientStops[next][2]-gradientStops[idx][2])*frac

	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", int(r), int(g), int(b)))
}

const titleText = "vmup - 1.5.1 - GCP Instance Manager"
const gradientCycleLen = 40

func renderTitle(offset int) string {
	var b strings.Builder
	for i, ch := range titleText {
		if ch == ' ' {
			b.WriteRune(' ')
			continue
		}
		t := float64((i+offset)%gradientCycleLen) / float64(gradientCycleLen)
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(gradientColor(t)).Render(string(ch)))
	}
	return b.String()
}

// --- View ---

// ViewContent renders everything below the title and tab bar.
func (m vmListModel) ViewContent() string {
	var b strings.Builder

	if m.loading && len(m.vms) == 0 {
		text := "Fetching instance status from GCP..."
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

	if len(m.vms) == 0 && !m.hasFilter() {
		b.WriteString(dimStyle.Render("No VMs found."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press n to launch a new VM."))
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

func (m vmListModel) viewTable() string {
	var b strings.Builder
	w := m.renderWidth
	cw := calcColumnWidths(w)

	sName := lipgloss.NewStyle().Width(cw.name).Foreground(lipgloss.Color("255"))
	sProject := lipgloss.NewStyle().Width(cw.project).Foreground(lipgloss.Color("255"))
	sZone := lipgloss.NewStyle().Width(cw.zone).Foreground(lipgloss.Color("255"))
	sMachine := lipgloss.NewStyle().Width(cw.machine).Foreground(lipgloss.Color("255"))
	sStatus := lipgloss.NewStyle().Width(cw.status).Foreground(lipgloss.Color("255"))

	sep := dimStyle.Render(strings.Repeat("─", w))

	// Header
	header := fmt.Sprintf("  %s%s%s%s%s",
		headerStyle.Render(sName.Render("VM Name")),
		headerStyle.Render(sProject.Render("Project")),
		headerStyle.Render(sZone.Render("Zone")),
		headerStyle.Render(sMachine.Render("Machine")),
		headerStyle.Render(sStatus.Render("Status")),
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
		vm := m.displayVM(i)
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		name := sName.Render(truncate(vm.cfg.VMName, cw.name-2))
		project := sProject.Render(truncate(vm.cfg.ProjectID, cw.project-2))
		zone := sZone.Render(truncate(vm.cfg.Zone, cw.zone-2))
		machine := sMachine.Render(truncate(vm.cfg.MachineType, cw.machine-2))
		status := statusColorStyle(vm.status, sStatus).Render(m.statusText(vm))

		row := fmt.Sprintf("%s%s%s%s%s%s", cursor, name, project, zone, machine, status)
		if i == m.cursor {
			row = rowSelected.Render(row)
		}
		b.WriteString(row + "\n")
	}

	if end < count {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more below", count-end)) + "\n")
	}

	// Detail section for selected VM
	if m.cursor < count {
		vm := m.displayVM(m.cursor)
		b.WriteString("\n" + sep + "\n")
		detail := func(k, v string) {
			b.WriteString(statusKeyStyle.Render(k) + statusValStyle.Render(v) + "\n")
		}
		detail("Image:", vm.cfg.Image)
		detail("Port Mapping:", vm.cfg.PortMapping)
		detail("Username:", vm.cfg.Username)
		detail("Boot Disk:", vm.cfg.BootDiskSize+" GB")
		if len(vm.attachedDisks) > 0 {
			detail("Data Disks:", strings.Join(vm.attachedDisks, ", "))
		}

		if m.tunnelMgr != nil {
			pids := m.tunnelMgr.ActivePIDsForVM(vm.cfg.VMName)
			for _, pp := range vm.cfg.PortMappings() {
				key := fmt.Sprintf("%s:%s", pp.Local, pp.Remote)
				if pid, ok := pids[key]; ok {
					b.WriteString(infoStyle.Render(fmt.Sprintf("  Tunnel active: http://localhost:%s (PID %d)", pp.Local, pid)) + "\n")
				}
			}
		}
	}

	return b.String()
}

func (m vmListModel) viewCards() string {
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
		vm := m.displayVM(i)
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		nameStr := truncate(vm.cfg.VMName, w-25)
		statusText := m.statusText(vm)
		statusStyled := statusColorStyle(vm.status, noWidthStatus).Render(statusText)

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

		// Card detail lines
		indent := "    "
		b.WriteString(indent + cardLabel.Render("Project:") + vm.cfg.ProjectID + "\n")
		b.WriteString(indent + cardLabel.Render("Zone:") + vm.cfg.Zone + "\n")
		b.WriteString(indent + cardLabel.Render("Machine:") + vm.cfg.MachineType + "\n")

		// Extra detail for selected VM
		if i == m.cursor {
			b.WriteString(indent + cardLabel.Render("Image:") + vm.cfg.Image + "\n")
			b.WriteString(indent + cardLabel.Render("Username:") + vm.cfg.Username + "\n")
			b.WriteString(indent + cardLabel.Render("Boot Disk:") + vm.cfg.BootDiskSize + " GB\n")
			if len(vm.attachedDisks) > 0 {
				b.WriteString(indent + cardLabel.Render("Data Disks:") + strings.Join(vm.attachedDisks, ", ") + "\n")
			}
		}

		// Active tunnels
		if m.tunnelMgr != nil {
			pids := m.tunnelMgr.ActivePIDsForVM(vm.cfg.VMName)
			for _, pp := range vm.cfg.PortMappings() {
				key := fmt.Sprintf("%s:%s", pp.Local, pp.Remote)
				if _, ok := pids[key]; ok {
					b.WriteString(indent + cardLabel.Render("Tunnel:") +
						infoStyle.Render(fmt.Sprintf("http://localhost:%s", pp.Local)) + "\n")
				}
			}
		}

		b.WriteString(sep + "\n")
	}

	if end < count {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more below", count-end)) + "\n")
	}

	return b.String()
}

func (m vmListModel) helpBar() string {
	return dimStyle.Render("↑/↓/←/→ navigate • : command • / filter • r refresh • ? help")
}

func (m vmListModel) viewHelpDialog() string {
	commands := []struct{ key, desc string }{
		{"↑/↓/j/k", "Navigate list"},
		{"←/→/h/l", "Switch tabs"},
		{"tab", "Next tab"},
		{"shift+tab", "Previous tab"},
		{"1/2", "Jump to tab"},
		{"n", "Create a new VM instance"},
		{"e", "Edit VM configuration"},
		{"i", "View VM info"},
		{"s", "Start VM & connect tunnels"},
		{"c", "Connect through SSH"},
		{"a", "Attach disk to VM"},
		{"d", "Detach disk from VM"},
		{"x", "Stop VM"},
		{"X", "Stop all VMs & tunnels"},
		{"D", "Destroy VM"},
		{":", "Command palette"},
		{"/", "Filter list"},
		{"r", "Refresh list"},
		{"p", "View progress"},
		{",", "Settings"},
		{"q", "Quit"},
		{"esc", "Clear filter / back"},
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
		Render(headerStyle.Render("Commands") + "\n\n" + content + "\n\n" + dimStyle.Render("Press any key to close"))

	return box
}

func truncate(s string, max int) string {
	if max < 4 {
		max = 4
	}
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

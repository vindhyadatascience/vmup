package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vds-gcp-launch-instance/internal/config"
	"vds-gcp-launch-instance/internal/gcloud"
	"vds-gcp-launch-instance/internal/tunnel"
)

type vmEntry struct {
	cfg    config.Config
	status string
}

type vmListModel struct {
	vms          []vmEntry
	cursor       int
	loading      bool
	spinner      spinner.Model
	flashMsg     string
	flashIsError bool
	tunnelMgr    *tunnel.Manager

	// Layout
	layoutWidth int // debounced width — controls table vs card mode
	renderWidth int // immediate width — controls content sizing
	height      int
	scrollTop   int

	// Resize debounce
	resizeSeq int

	// Help dialog
	showHelp bool

	// Animated gradient
	gradientOffset int
}

// Messages
type vmListLoadedMsg struct {
	vms []vmEntry
}

type vmListActionMsg struct {
	action menuAction
	cfg    config.Config
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
		loading:     true,
		spinner:     s,
		tunnelMgr:   tm,
		layoutWidth: 114,
		renderWidth: 114,
		height:      40,
	}
}

func (m vmListModel) Init() tea.Cmd {
	return tea.Batch(loadVMList, m.spinner.Tick)
}

func loadVMList() tea.Msg {
	names := config.ListProjects()
	vms := make([]vmEntry, 0, len(names))
	for _, name := range names {
		tfvarsPath := filepath.Join(config.ProjectDir(name), "terraform.tfvars")
		cfg, err := config.LoadTFVars(tfvarsPath)
		if err != nil {
			continue
		}
		status := gcloud.InstanceStatus(cfg.VMName, cfg.ProjectID, cfg.Zone)
		vms = append(vms, vmEntry{cfg: cfg, status: status})
	}
	return vmListLoadedMsg{vms: vms}
}

// visibleVMs returns how many VMs fit on screen in table mode.
func (m vmListModel) visibleTableRows() int {
	// Overhead: title(2) + refresh(2) + header+sep(2) + detail(~8) + flash(2) + help(1) = ~17
	v := m.height - 17
	if v < 1 {
		v = 1
	}
	return v
}

// visibleCards returns how many cards fit on screen.
func (m vmListModel) visibleCards() int {
	// Overhead: title(2) + refresh(2) + flash(2) + help(1) = ~7
	// Each card: ~5 lines non-selected, ~8 selected. Use 5 as estimate.
	v := (m.height - 7) / 5
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
	maxScroll := len(m.vms) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollTop > maxScroll {
		m.scrollTop = maxScroll
	}
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
		if m.cursor >= len(m.vms) && len(m.vms) > 0 {
			m.cursor = len(m.vms) - 1
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
		// Dismiss help dialog on any key
		if m.showHelp {
			m.showHelp = false
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

		if len(m.vms) == 0 {
			switch msg.String() {
			case "n":
				return m, func() tea.Msg {
					return vmListActionMsg{action: actionLaunch}
				}
			case "?":
				m.showHelp = true
				return m, nil
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
			if m.cursor < len(m.vms)-1 {
				m.cursor++
				m.adjustScroll()
			}
		case "n":
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionLaunch}
			}
		case "s":
			vm := m.vms[m.cursor]
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionStartTunnels, cfg: vm.cfg}
			}
		case "x":
			vm := m.vms[m.cursor]
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionStopTunnels, cfg: vm.cfg}
			}
		case "X":
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionStopAll}
			}
		case "c":
			vm := m.vms[m.cursor]
			if vm.status != "RUNNING" {
				m.flashMsg = fmt.Sprintf("SSH requires a running VM (current status: %s). Use 's' to start it first.", vm.status)
				m.flashIsError = true
				return m, nil
			}
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionSSH, cfg: vm.cfg}
			}
		case "d":
			vm := m.vms[m.cursor]
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionDestroy, cfg: vm.cfg}
			}
		case "e":
			vm := m.vms[m.cursor]
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionEdit, cfg: vm.cfg}
			}
		case "i":
			vm := m.vms[m.cursor]
			return m, func() tea.Msg {
				return vmListActionMsg{action: actionInfo, cfg: vm.cfg}
			}
		case "r":
			m.loading = true
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

const titleText = "vmup - 1.0.5 - GCP Instance Manager"
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

func (m vmListModel) View() string {
	var b strings.Builder

	b.WriteString(renderTitle(m.gradientOffset))
	b.WriteString("\n\n")

	if m.loading && len(m.vms) == 0 {
		b.WriteString(m.spinner.View() + " " + dimStyle.Render("Fetching instance status from GCP..."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("q quit • ctrl+c quit"))
		return b.String()
	}

	if m.loading {
		b.WriteString(m.spinner.View() + " " + dimStyle.Render("Refreshing..."))
		b.WriteString("\n\n")
	}

	if len(m.vms) == 0 {
		b.WriteString(dimStyle.Render("No VMs found."))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Press n to launch a new VM."))
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

	b.WriteString("\n")
	b.WriteString(m.helpBar())

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
	sStatus := lipgloss.NewStyle().Width(cw.status)

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
	end := m.scrollTop + visible
	if end > len(m.vms) {
		end = len(m.vms)
	}

	if m.scrollTop > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.scrollTop)) + "\n")
	}

	// Rows
	for i := m.scrollTop; i < end; i++ {
		vm := m.vms[i]
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

	if end < len(m.vms) {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more below", len(m.vms)-end)) + "\n")
	}

	// Detail section for selected VM
	if m.cursor < len(m.vms) {
		vm := m.vms[m.cursor]
		b.WriteString("\n" + sep + "\n")
		detail := func(k, v string) {
			b.WriteString(statusKeyStyle.Render(k) + statusValStyle.Render(v) + "\n")
		}
		detail("Image:", vm.cfg.Image)
		detail("Port Mapping:", vm.cfg.PortMapping)
		detail("Username:", vm.cfg.Username)
		detail("Disk Size:", vm.cfg.BootDiskSize+" GB")

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
	end := m.scrollTop + visible
	if end > len(m.vms) {
		end = len(m.vms)
	}

	if m.scrollTop > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↑ %d more above", m.scrollTop)) + "\n")
	}

	for i := m.scrollTop; i < end; i++ {
		vm := m.vms[i]
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
			b.WriteString(indent + cardLabel.Render("Disk Size:") + vm.cfg.BootDiskSize + " GB\n")
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

	if end < len(m.vms) {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ↓ %d more below", len(m.vms)-end)) + "\n")
	}

	return b.String()
}

func (m vmListModel) helpBar() string {
	if m.renderWidth >= 100 {
		return dimStyle.Render("↑/↓ navigate • n new vm • e edit • i info • s start • x stop • X stop all • c ssh • d destroy • r refresh • q quit")
	}
	return dimStyle.Render("q quit • ? help")
}

func (m vmListModel) viewHelpDialog() string {
	commands := []struct{ key, desc string }{
		{"↑/↓", "Navigate"},
		{"n", "New VM"},
		{"e", "Edit VM"},
		{"i", "VM info"},
		{"s", "Start & tunnel"},
		{"x", "Stop tunnels"},
		{"X", "Stop all"},
		{"c", "SSH connect"},
		{"d", "Destroy VM"},
		{"r", "Refresh"},
		{"q", "Quit"},
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).Width(6)
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

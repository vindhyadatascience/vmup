package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteCommand struct {
	key    string        // shortcut key, e.g. "n"
	name   string        // word command, e.g. "new-instance"
	desc   string        // description, e.g. "Start a new VM instance"
	filter string        // lowercase search text
	action func() tea.Msg // closure producing the action message
}

type cmdPaletteModel struct {
	active   bool
	input    string
	commands []paletteCommand
	filtered []int
	cursor   int
	width    int
}

type cmdPaletteExecMsg struct {
	action func() tea.Msg
	input  string // raw palette input at time of execution
}

// Palette-specific message types for actions handled directly by the App.
type cmdPaletteSwitchTabMsg struct{}
type cmdPaletteRefreshMsg struct{ tab tab }
type cmdPaletteProgressMsg struct{}
type cmdPaletteFilterMsg struct {
	args string // optional pre-filled filter args from ":filter prop term"
}

func newCmdPalette() cmdPaletteModel {
	return cmdPaletteModel{}
}

func (m *cmdPaletteModel) Open(commands []paletteCommand, width int) {
	m.active = true
	m.input = ""
	m.commands = commands
	m.cursor = 0
	m.width = width
	m.filterCommands()
}

func (m *cmdPaletteModel) Close() {
	m.active = false
	m.input = ""
	m.commands = nil
	m.filtered = nil
	m.cursor = 0
}

func (m cmdPaletteModel) Update(msg tea.Msg) (cmdPaletteModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEscape, tea.KeyCtrlC:
		m.Close()
		return m, nil

	case tea.KeyEnter:
		if len(m.filtered) > 0 {
			action := m.commands[m.filtered[m.cursor]].action
			input := m.input
			return m, func() tea.Msg { return cmdPaletteExecMsg{action: action, input: input} }
		}
		return m, nil

	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
			m.filterCommands()
		} else {
			m.Close()
		}
		return m, nil

	case tea.KeyUp, tea.KeyShiftTab:
		if len(m.filtered) > 0 {
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.filtered) - 1
			}
		}
		return m, nil

	case tea.KeyDown, tea.KeyTab:
		if len(m.filtered) > 0 {
			m.cursor++
			if m.cursor >= len(m.filtered) {
				m.cursor = 0
			}
		}
		return m, nil

	case tea.KeyRunes:
		m.input += string(keyMsg.Runes)
		m.filterCommands()
		return m, nil
	}

	return m, nil
}

func (m *cmdPaletteModel) filterCommands() {
	m.filtered = m.filtered[:0] // reuse backing array
	query := strings.ToLower(m.input)
	for i, cmd := range m.commands {
		if query == "" || strings.Contains(cmd.filter, query) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		} else {
			m.cursor = 0
		}
	}
}

const maxPaletteVisible = 10

func (m cmdPaletteModel) View() string {
	w := m.width
	if w < 30 {
		w = 30
	}

	if w >= 65 {
		return m.viewWide(w)
	}
	return m.viewCompact(w)
}

func (m cmdPaletteModel) viewWide(w int) string {
	var b strings.Builder

	sep := dimStyle.Render(strings.Repeat("─", w))

	// Input line
	prompt := palettePromptStyle.Render(":")
	inputText := statusValStyle.Render(m.input) + dimStyle.Render("▎")
	b.WriteString(sep + "\n")
	b.WriteString("  " + prompt + inputText + "\n")
	b.WriteString(sep + "\n")

	if len(m.filtered) == 0 {
		b.WriteString("  " + dimStyle.Render("No matching commands") + "\n")
	} else {
		start, end := m.visibleWindow()

		if start > 0 {
			b.WriteString(dimStyle.Render("  ↑ more") + "\n")
		}

		// Find max name width for alignment
		maxNameWidth := 0
		for i := start; i < end; i++ {
			cmd := m.commands[m.filtered[i]]
			nameWithKey := cmd.name + " (" + cmd.key + ")"
			if len(nameWithKey) > maxNameWidth {
				maxNameWidth = len(nameWithKey)
			}
		}

		nameStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true).
			Width(maxNameWidth + 2)

		keyParenStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

		selectedBg := lipgloss.NewStyle().
			Background(lipgloss.Color("236"))

		for i := start; i < end; i++ {
			cmd := m.commands[m.filtered[i]]
			nameText := cmd.name + " " + keyParenStyle.Render("("+cmd.key+")")
			row := nameStyle.Render(nameText) + "  " + descStyle.Render(cmd.desc)

			if i == m.cursor {
				b.WriteString(selectedBg.Render("> " + row) + "\n")
			} else {
				b.WriteString("  " + row + "\n")
			}
		}

		if end < len(m.filtered) {
			b.WriteString(dimStyle.Render("  ↓ more") + "\n")
		}
	}

	b.WriteString(dimStyle.Render("↑/↓ navigate • enter run • esc close"))

	return b.String()
}

func (m cmdPaletteModel) viewCompact(w int) string {
	var b strings.Builder

	sep := dimStyle.Render(strings.Repeat("─", w))

	// Input line
	prompt := palettePromptStyle.Render(":")
	inputText := statusValStyle.Render(m.input) + dimStyle.Render("▎")
	b.WriteString(sep + "\n")
	b.WriteString("  " + prompt + inputText + "\n")
	b.WriteString(sep + "\n")

	if len(m.filtered) == 0 {
		b.WriteString("  " + dimStyle.Render("No matching commands") + "\n")
	} else {
		start, end := m.visibleWindow()

		if start > 0 {
			b.WriteString(dimStyle.Render("  ↑ more") + "\n")
		}

		nameStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true)

		keyParenStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

		selectedBg := lipgloss.NewStyle().
			Background(lipgloss.Color("236"))

		for i := start; i < end; i++ {
			cmd := m.commands[m.filtered[i]]
			nameText := nameStyle.Render(cmd.name) + " " + keyParenStyle.Render("("+cmd.key+")")
			descText := "    " + descStyle.Render(cmd.desc)

			if i == m.cursor {
				b.WriteString(selectedBg.Render("> "+nameText) + "\n")
				b.WriteString(selectedBg.Render(descText) + "\n")
			} else {
				b.WriteString("  " + nameText + "\n")
				b.WriteString(descText + "\n")
			}

			if i < end-1 {
				b.WriteString("\n")
			}
		}

		if end < len(m.filtered) {
			b.WriteString(dimStyle.Render("  ↓ more") + "\n")
		}
	}

	b.WriteString(dimStyle.Render("↑/↓ • enter run • esc close"))

	return b.String()
}

func (m cmdPaletteModel) visibleWindow() (int, int) {
	total := len(m.filtered)
	if total <= maxPaletteVisible {
		return 0, total
	}

	half := maxPaletteVisible / 2
	start := m.cursor - half
	if start < 0 {
		start = 0
	}
	end := start + maxPaletteVisible
	if end > total {
		end = total
		start = end - maxPaletteVisible
	}
	return start, end
}

// --- Command list builders ---

func makeCommand(key, name, desc string, action func() tea.Msg) paletteCommand {
	return paletteCommand{
		key:    key,
		name:   name,
		desc:   desc,
		filter: strings.ToLower(key + " " + name + " " + desc),
		action: action,
	}
}

func vmPaletteCommands(vms []vmEntry, cursor int, bgRunning bool, progressDone bool) []paletteCommand {
	var cmds []paletteCommand

	cmds = append(cmds, makeCommand("n", "new-instance", "Start a new VM instance", func() tea.Msg {
		return vmListActionMsg{action: actionLaunch}
	}))

	if len(vms) > 0 {
		vm := vms[cursor]

		cmds = append(cmds, makeCommand("e", "edit-instance", "Edit selected VM configuration", func() tea.Msg {
			return vmListActionMsg{action: actionEdit, cfg: vm.cfg}
		}))

		cmds = append(cmds, makeCommand("i", "info", "View selected VM info", func() tea.Msg {
			return vmListActionMsg{action: actionInfo, cfg: vm.cfg}
		}))

		cmds = append(cmds, makeCommand("s", "start-instance", "Start selected VM & connect tunnels", func() tea.Msg {
			return vmListActionMsg{action: actionStartTunnels, cfg: vm.cfg}
		}))

		cmds = append(cmds, makeCommand("x", "stop-instance", "Stop selected VM", func() tea.Msg {
			return vmListActionMsg{action: actionStopTunnels, cfg: vm.cfg}
		}))

		if vm.status == "RUNNING" {
			cmds = append(cmds, makeCommand("c", "connect", "Connect to selected VM through SSH", func() tea.Msg {
				return vmListActionMsg{action: actionSSH, cfg: vm.cfg}
			}))

			cmds = append(cmds, makeCommand("a", "attach-disk", "Attach disk to selected VM", func() tea.Msg {
				return vmListActionMsg{action: actionAttachDiskToVM, cfg: vm.cfg}
			}))
		}

		if len(vm.attachedDiskCfg) > 0 {
			cmds = append(cmds, makeCommand("d", "detach-disk", "Detach disk from selected VM", func() tea.Msg {
				return vmDetachDiskMsg{vmCfg: vm.cfg, diskCfgs: vm.attachedDiskCfg}
			}))
		}

		cmds = append(cmds, makeCommand("D", "destroy-instance", "Destroy VM (careful - destructive)", func() tea.Msg {
			return vmListActionMsg{action: actionDestroy, cfg: vm.cfg}
		}))
	}

	cmds = append(cmds, makeCommand("X", "stop-all", "Stop all VMs & tunnels", func() tea.Msg {
		return vmListActionMsg{action: actionStopAll}
	}))

	cmds = append(cmds, makeCommand("/", "filter", "Filter list by property", func() tea.Msg {
		return cmdPaletteFilterMsg{}
	}))

	cmds = append(cmds, makeCommand("r", "refresh", "Refresh list", func() tea.Msg {
		return cmdPaletteRefreshMsg{tab: tabInstances}
	}))

	if bgRunning || progressDone {
		cmds = append(cmds, makeCommand("p", "progress", "View background progress", func() tea.Msg {
			return cmdPaletteProgressMsg{}
		}))
	}

	cmds = append(cmds, makeCommand("tab", "switch-tab", "Switch to Data Disks", func() tea.Msg {
		return cmdPaletteSwitchTabMsg{}
	}))

	cmds = append(cmds, makeCommand("q", "quit", "Quit/exit application", func() tea.Msg {
		return tea.Quit()
	}))

	return cmds
}

func diskPaletteCommands(disks []diskEntry, cursor int, bgRunning bool, progressDone bool) []paletteCommand {
	var cmds []paletteCommand

	cmds = append(cmds, makeCommand("n", "new-disk", "Create a new disk", func() tea.Msg {
		return diskListActionMsg{action: actionDiskCreate}
	}))

	cmds = append(cmds, makeCommand("I", "import-disk", "Import existing disk", func() tea.Msg {
		return diskListActionMsg{action: actionDiskImport}
	}))

	if len(disks) > 0 {
		disk := disks[cursor]

		cmds = append(cmds, makeCommand("e", "edit-disk", "Edit/resize selected disk", func() tea.Msg {
			return diskListActionMsg{action: actionDiskResize, disk: disk}
		}))

		if !(len(disk.status.Users) > 0 && disk.status.Mode == "READ_WRITE") {
			cmds = append(cmds, makeCommand("a", "attach-disk", "Attach selected disk to VM", func() tea.Msg {
				return diskListActionMsg{action: actionDiskAttach, disk: disk}
			}))
		}

		if len(disk.status.Users) > 0 {
			cmds = append(cmds, makeCommand("d", "detach-disk", "Detach selected disk from VM", func() tea.Msg {
				return diskListActionMsg{action: actionDiskDetach, disk: disk}
			}))
		}

		if len(disk.status.Users) == 0 {
			cmds = append(cmds, makeCommand("D", "delete-disk", "Delete selected disk (careful - destructive)", func() tea.Msg {
				return diskListActionMsg{action: actionDiskDelete, disk: disk}
			}))
		}
	}

	cmds = append(cmds, makeCommand("/", "filter", "Filter list by property", func() tea.Msg {
		return cmdPaletteFilterMsg{}
	}))

	cmds = append(cmds, makeCommand("r", "refresh", "Refresh list", func() tea.Msg {
		return cmdPaletteRefreshMsg{tab: tabDataDisks}
	}))

	if bgRunning || progressDone {
		cmds = append(cmds, makeCommand("p", "progress", "View background progress", func() tea.Msg {
			return cmdPaletteProgressMsg{}
		}))
	}

	cmds = append(cmds, makeCommand("tab", "switch-tab", "Switch to Instances", func() tea.Msg {
		return cmdPaletteSwitchTabMsg{}
	}))

	cmds = append(cmds, makeCommand("q", "quit", "Quit/exit application", func() tea.Msg {
		return tea.Quit()
	}))

	return cmds
}

// vmPaletteCommandsEmpty returns commands available when VM list is empty.
func vmPaletteCommandsEmpty(bgRunning bool, progressDone bool) []paletteCommand {
	return vmPaletteCommands(nil, 0, bgRunning, progressDone)
}

// diskPaletteCommandsEmpty returns commands available when disk list is empty.
func diskPaletteCommandsEmpty(bgRunning bool, progressDone bool) []paletteCommand {
	return diskPaletteCommands(nil, 0, bgRunning, progressDone)
}

package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteCommand struct {
	key      string         // shortcut key, e.g. "n"
	name     string         // word command, e.g. "new-instance"
	desc     string         // description, e.g. "Start a new VM instance"
	filter   string         // lowercase search text
	color    lipgloss.Color // semantic color for the command name
	category string         // group name, e.g. "Create & Connect"
	action   func() tea.Msg // closure producing the action message
}

type cmdPaletteModel struct {
	active     bool
	input      string
	commands   []paletteCommand
	filtered   []int
	cursor     int
	width      int
	categories []string // "All" + unique categories from commands
	catIdx     int      // selected category index (0 = All)
}

type cmdPaletteExecMsg struct {
	action func() tea.Msg
	input  string // raw palette input at time of execution
}

// Palette-specific message types for actions handled directly by the App.
type cmdPaletteSwitchTabMsg struct{}
type cmdPaletteRefreshMsg struct{ tab tab }
type cmdPaletteProgressMsg struct{}
type cmdPaletteSettingsMsg struct{}
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
	m.catIdx = 0

	// Build unique category list preserving order
	m.categories = []string{"All"}
	seen := map[string]bool{}
	for _, cmd := range commands {
		if cmd.category != "" && !seen[cmd.category] {
			m.categories = append(m.categories, cmd.category)
			seen[cmd.category] = true
		}
	}

	m.filterCommands()
}

// Rebuild replaces the command list while preserving input, category, and cursor.
func (m *cmdPaletteModel) Rebuild(commands []paletteCommand) {
	if !m.active {
		return
	}
	m.commands = commands

	// Rebuild categories, try to preserve selection
	prevCat := ""
	if m.catIdx > 0 && m.catIdx < len(m.categories) {
		prevCat = m.categories[m.catIdx]
	}
	m.categories = []string{"All"}
	seen := map[string]bool{}
	for _, cmd := range commands {
		if cmd.category != "" && !seen[cmd.category] {
			m.categories = append(m.categories, cmd.category)
			seen[cmd.category] = true
		}
	}
	m.catIdx = 0
	for i, cat := range m.categories {
		if cat == prevCat {
			m.catIdx = i
			break
		}
	}

	m.filterCommands()
}

func (m *cmdPaletteModel) Close() {
	m.active = false
	m.input = ""
	m.commands = nil
	m.filtered = nil
	m.categories = nil
	m.catIdx = 0
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

	case tea.KeyLeft:
		if len(m.categories) > 1 {
			m.catIdx--
			if m.catIdx < 0 {
				m.catIdx = len(m.categories) - 1
			}
			m.cursor = 0
			m.filterCommands()
		}
		return m, nil

	case tea.KeyRight:
		if len(m.categories) > 1 {
			m.catIdx++
			if m.catIdx >= len(m.categories) {
				m.catIdx = 0
			}
			m.cursor = 0
			m.filterCommands()
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
	activeCat := ""
	if m.catIdx > 0 && m.catIdx < len(m.categories) {
		activeCat = m.categories[m.catIdx]
	}
	for i, cmd := range m.commands {
		if activeCat != "" && cmd.category != activeCat {
			continue
		}
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

func (m cmdPaletteModel) viewCategoryTabs() string {
	indent := "  "
	maxWidth := m.width
	if maxWidth < 30 {
		maxWidth = 30
	}

	var lines []string
	line := indent
	lineLen := len(indent)

	for i, cat := range m.categories {
		var rendered string
		if i == m.catIdx {
			rendered = activeTabStyle.Render(cat)
		} else {
			rendered = inactiveTabStyle.Render(cat)
		}
		// +2 for padding added by tab styles, +1 for space separator
		tabWidth := len(cat) + 2
		if i > 0 {
			tabWidth++ // space before tab
		}

		if lineLen+tabWidth > maxWidth && lineLen > len(indent) {
			lines = append(lines, line)
			line = indent + rendered
			lineLen = len(indent) + len(cat) + 2
		} else {
			if lineLen > len(indent) {
				line += " "
				lineLen++
			}
			line += rendered
			lineLen += len(cat) + 2
		}
	}
	if line != indent {
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

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

	b.WriteString(sep + "\n")
	b.WriteString("  " + palettePromptStyle.Render(":") + statusValStyle.Render(m.input) + dimStyle.Render("▎") + "\n")
	b.WriteString(sep + "\n")

	if len(m.categories) > 1 {
		b.WriteString(m.viewCategoryTabs() + "\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString("  " + dimStyle.Render("No matching commands") + "\n")
	} else {
		start, end := m.visibleWindow()

		if start > 0 {
			b.WriteString(dimStyle.Render("  ↑ more") + "\n")
		}

		maxNameWidth := 0
		for i := start; i < end; i++ {
			cmd := m.commands[m.filtered[i]]
			nameWithKey := cmd.name + " (" + cmd.key + ")"
			if len(nameWithKey) > maxNameWidth {
				maxNameWidth = len(nameWithKey)
			}
		}

		keyParenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
		selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("236"))

		for i := start; i < end; i++ {
			cmd := m.commands[m.filtered[i]]
			nameStyle := lipgloss.NewStyle().
				Foreground(cmd.color).
				Width(maxNameWidth + 2)
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

	b.WriteString(dimStyle.Render("↑/↓ navigate • ←/→ category • enter run • esc close"))

	return b.String()
}

func (m cmdPaletteModel) viewCompact(w int) string {
	var b strings.Builder

	sep := dimStyle.Render(strings.Repeat("─", w))

	b.WriteString(sep + "\n")
	b.WriteString("  " + palettePromptStyle.Render(":") + statusValStyle.Render(m.input) + dimStyle.Render("▎") + "\n")
	b.WriteString(sep + "\n")

	if len(m.categories) > 1 {
		b.WriteString(m.viewCategoryTabs() + "\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString("  " + dimStyle.Render("No matching commands") + "\n")
	} else {
		start, end := m.visibleWindow()

		if start > 0 {
			b.WriteString(dimStyle.Render("  ↑ more") + "\n")
		}

		keyParenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		selectedBg := lipgloss.NewStyle().Background(lipgloss.Color("236"))

		for i := start; i < end; i++ {
			cmd := m.commands[m.filtered[i]]
			nameStyle := lipgloss.NewStyle().
				Foreground(cmd.color)
			nameText := nameStyle.Render(cmd.name) + " " + keyParenStyle.Render("("+cmd.key+")")
			descText := "    " + descStyle.Render(cmd.desc)

			if i == m.cursor {
				b.WriteString(selectedBg.Render("> "+nameText) + "\n")
				b.WriteString(selectedBg.Render(descText) + "\n")
			} else {
				b.WriteString("  " + nameText + "\n")
				b.WriteString(descText + "\n")
			}

		}

		if end < len(m.filtered) {
			b.WriteString(dimStyle.Render("  ↓ more") + "\n")
		}
	}

	b.WriteString(dimStyle.Render("↑/↓ • ←/→ category • enter run • esc close"))

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

// Semantic colors for command palette entries.
var (
	cmdColorCreate      = lipgloss.Color("42")  // green — create, start, connect, import
	cmdColorModify      = lipgloss.Color("39")  // cyan — edit, info, attach, detach
	cmdColorDestructive = lipgloss.Color("196") // red — destroy, delete, stop
	cmdColorNav         = lipgloss.Color("252") // light gray — filter, refresh, quit, utility
)

func makeCommand(key, name, desc, category string, color lipgloss.Color, action func() tea.Msg) paletteCommand {
	return paletteCommand{
		key:      key,
		name:     name,
		desc:     desc,
		filter:   strings.ToLower(key + " " + name + " " + desc),
		color:    color,
		category: category,
		action:   action,
	}
}

const (
	catCreate  = "Create & Connect"
	catModify  = "Modify & Inspect"
	catDestroy = "Stop & Destroy"
	catUtility = "Utility"
)

func vmPaletteCommands(vms []vmEntry, cursor int, bgRunning bool, progressDone bool) []paletteCommand {
	var cmds []paletteCommand

	// Create / start / connect (green)
	cmds = append(cmds, makeCommand("n", "new-instance", "Create a new VM instance", catCreate, cmdColorCreate, func() tea.Msg {
		return vmListActionMsg{action: actionLaunch}
	}))
	if len(vms) > 0 {
		vm := vms[cursor]
		cmds = append(cmds, makeCommand("s", "start-instance", "Start selected VM & connect tunnels", catCreate, cmdColorCreate, func() tea.Msg {
			return vmListActionMsg{action: actionStartTunnels, cfg: vm.cfg}
		}))
		if vm.status == "RUNNING" {
			cmds = append(cmds, makeCommand("c", "connect", "Connect to selected VM through SSH", catCreate, cmdColorCreate, func() tea.Msg {
				return vmListActionMsg{action: actionSSH, cfg: vm.cfg}
			}))
		}
	}

	// Modify / inspect (cyan)
	if len(vms) > 0 {
		vm := vms[cursor]
		cmds = append(cmds, makeCommand("e", "edit-instance", "Edit selected VM configuration", catModify, cmdColorModify, func() tea.Msg {
			return vmListActionMsg{action: actionEdit, cfg: vm.cfg}
		}))
		cmds = append(cmds, makeCommand("i", "info", "View selected VM info", catModify, cmdColorModify, func() tea.Msg {
			return vmListActionMsg{action: actionInfo, cfg: vm.cfg}
		}))
		if vm.status == "RUNNING" {
			cmds = append(cmds, makeCommand("a", "attach-disk", "Attach disk to selected VM", catModify, cmdColorModify, func() tea.Msg {
				return vmListActionMsg{action: actionAttachDiskToVM, cfg: vm.cfg}
			}))
		}
		if len(vm.attachedDiskCfg) > 0 {
			cmds = append(cmds, makeCommand("d", "detach-disk", "Detach disk from selected VM", catModify, cmdColorModify, func() tea.Msg {
				return vmDetachDiskMsg{vmCfg: vm.cfg, diskCfgs: vm.attachedDiskCfg}
			}))
		}
	}

	// Destructive (red)
	if len(vms) > 0 {
		vm := vms[cursor]
		cmds = append(cmds, makeCommand("x", "stop-instance", "Stop selected VM", catDestroy, cmdColorDestructive, func() tea.Msg {
			return vmListActionMsg{action: actionStopTunnels, cfg: vm.cfg}
		}))
		cmds = append(cmds, makeCommand("X", "stop-all", "Stop all VMs & tunnels", catDestroy, cmdColorDestructive, func() tea.Msg {
			return vmListActionMsg{action: actionStopAll}
		}))
		cmds = append(cmds, makeCommand("D", "destroy-instance", "Destroy VM (careful - destructive)", catDestroy, cmdColorDestructive, func() tea.Msg {
			return vmListActionMsg{action: actionDestroy, cfg: vm.cfg}
		}))
	} else {
		cmds = append(cmds, makeCommand("X", "stop-all", "Stop all VMs & tunnels", catDestroy, cmdColorDestructive, func() tea.Msg {
			return vmListActionMsg{action: actionStopAll}
		}))
	}

	// Utility (light gray)
	cmds = append(cmds, makeCommand("/", "filter", "Filter list by property", catUtility, cmdColorNav, func() tea.Msg {
		return cmdPaletteFilterMsg{}
	}))
	cmds = append(cmds, makeCommand("r", "refresh", "Refresh list", catUtility, cmdColorNav, func() tea.Msg {
		return cmdPaletteRefreshMsg{tab: tabInstances}
	}))
	if bgRunning || progressDone {
		cmds = append(cmds, makeCommand("p", "progress", "View background progress", catUtility, cmdColorNav, func() tea.Msg {
			return cmdPaletteProgressMsg{}
		}))
	}
	cmds = append(cmds, makeCommand("tab", "switch-tab", "Switch to Data Disks", catUtility, cmdColorNav, func() tea.Msg {
		return cmdPaletteSwitchTabMsg{}
	}))
	cmds = append(cmds, makeCommand(",", "settings", "Configure vmup settings", catUtility, cmdColorNav, func() tea.Msg {
		return cmdPaletteSettingsMsg{}
	}))
	cmds = append(cmds, makeCommand("q", "quit", "Quit/exit application", catUtility, cmdColorNav, func() tea.Msg {
		return tea.Quit()
	}))

	return cmds
}

func diskPaletteCommands(disks []diskEntry, cursor int, bgRunning bool, progressDone bool) []paletteCommand {
	var cmds []paletteCommand

	// Create / import (green)
	cmds = append(cmds, makeCommand("n", "new-disk", "Create a new disk", catCreate, cmdColorCreate, func() tea.Msg {
		return diskListActionMsg{action: actionDiskCreate}
	}))
	cmds = append(cmds, makeCommand("I", "import-disk", "Import existing disk", catCreate, cmdColorCreate, func() tea.Msg {
		return diskListActionMsg{action: actionDiskImport}
	}))

	// Modify (cyan)
	if len(disks) > 0 {
		disk := disks[cursor]
		cmds = append(cmds, makeCommand("e", "edit-disk", "Edit/resize selected disk", catModify, cmdColorModify, func() tea.Msg {
			return diskListActionMsg{action: actionDiskResize, disk: disk}
		}))
		if !(len(disk.status.Users) > 0 && disk.status.Mode == "READ_WRITE") {
			cmds = append(cmds, makeCommand("a", "attach-disk", "Attach selected disk to VM", catModify, cmdColorModify, func() tea.Msg {
				return diskListActionMsg{action: actionDiskAttach, disk: disk}
			}))
		}
		if len(disk.status.Users) > 0 {
			cmds = append(cmds, makeCommand("d", "detach-disk", "Detach selected disk from VM", catModify, cmdColorModify, func() tea.Msg {
				return diskListActionMsg{action: actionDiskDetach, disk: disk}
			}))
		}
	}

	// Destructive (red)
	if len(disks) > 0 {
		disk := disks[cursor]
		if len(disk.status.Users) == 0 {
			cmds = append(cmds, makeCommand("D", "delete-disk", "Delete selected disk (careful - destructive)", catDestroy, cmdColorDestructive, func() tea.Msg {
				return diskListActionMsg{action: actionDiskDelete, disk: disk}
			}))
		}
	}

	// Utility (light gray)
	cmds = append(cmds, makeCommand("/", "filter", "Filter list by property", catUtility, cmdColorNav, func() tea.Msg {
		return cmdPaletteFilterMsg{}
	}))
	cmds = append(cmds, makeCommand("r", "refresh", "Refresh list", catUtility, cmdColorNav, func() tea.Msg {
		return cmdPaletteRefreshMsg{tab: tabDataDisks}
	}))
	if bgRunning || progressDone {
		cmds = append(cmds, makeCommand("p", "progress", "View background progress", catUtility, cmdColorNav, func() tea.Msg {
			return cmdPaletteProgressMsg{}
		}))
	}
	cmds = append(cmds, makeCommand("tab", "switch-tab", "Switch to Instances", catUtility, cmdColorNav, func() tea.Msg {
		return cmdPaletteSwitchTabMsg{}
	}))
	cmds = append(cmds, makeCommand(",", "settings", "Configure vmup settings", catUtility, cmdColorNav, func() tea.Msg {
		return cmdPaletteSettingsMsg{}
	}))
	cmds = append(cmds, makeCommand("q", "quit", "Quit/exit application", catUtility, cmdColorNav, func() tea.Msg {
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

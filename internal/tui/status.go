package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"vds-gcp-launch-instance/internal/config"
)

type statusModel struct {
	cfg           config.Config
	tunnelPIDs    map[string]int
	attachedDisks []string
	message       string
}

func newStatusModel(cfg config.Config, tunnelPIDs map[string]int, attachedDisks []string, message string) statusModel {
	return statusModel{
		cfg:           cfg,
		tunnelPIDs:    tunnelPIDs,
		attachedDisks: attachedDisks,
		message:       message,
	}
}

func (m statusModel) Init() tea.Cmd {
	return nil
}

func (m statusModel) Update(msg tea.Msg) (statusModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "b", "esc", "ctrl+c":
			return m, func() tea.Msg { return backToMenuMsg{} }
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m statusModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("VM Info"))
	b.WriteString("\n\n")

	if m.message != "" {
		b.WriteString(successStyle.Render(m.message))
		b.WriteString("\n\n")
	}

	row := func(key, val string) {
		b.WriteString(statusKeyStyle.Render(key) + statusValStyle.Render(val) + "\n")
	}

	row("VM Name:", m.cfg.VMName)
	row("Project:", m.cfg.ProjectID)
	row("Zone:", m.cfg.Zone)
	row("Machine:", m.cfg.MachineType)
	row("Image:", m.cfg.Image)
	row("Boot Disk:", m.cfg.BootDiskSize+" GB")
	row("Port Mapping:", m.cfg.PortMapping)
	row("Username:", m.cfg.Username)
	row("Password:", m.cfg.Password)

	if len(m.attachedDisks) > 0 {
		row("Data Disks:", strings.Join(m.attachedDisks, ", "))
	}

	if len(m.tunnelPIDs) > 0 {
		b.WriteString("\n")
		b.WriteString(infoStyle.Render("Active Tunnels:"))
		b.WriteString("\n")
		for _, pp := range m.cfg.PortMappings() {
			key := fmt.Sprintf("%s:%s", pp.Local, pp.Remote)
			if pid, ok := m.tunnelPIDs[key]; ok {
				b.WriteString(fmt.Sprintf("  http://localhost:%s (PID %d)\n", pp.Local, pid))
			}
		}
	} else if len(m.cfg.PortMappings()) > 0 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("No active tunnels"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter/b/esc/ctrl+c back • q quit"))

	return b.String()
}

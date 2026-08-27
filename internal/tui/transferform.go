package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/vindhyadatascience/vmup/internal/config"
	"github.com/vindhyadatascience/vmup/internal/gcloud"
	"github.com/vindhyadatascience/vmup/internal/tunnel"
)

// --- File Transfer Form ---

type transferFormModel struct {
	form *huh.Form
	spec *gcloud.TransferSpec
	cfg  config.Config
}

type transferFormDoneMsg struct {
	cfg  config.Config
	spec gcloud.TransferSpec
}

type transferFormCancelMsg struct{}

func newTransferModel(cfg config.Config) transferFormModel {
	m := transferFormModel{
		cfg:  cfg,
		spec: &gcloud.TransferSpec{Remote: "~/"},
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[gcloud.Direction]().
				Title("Direction").
				Options(
					huh.NewOption("Upload — this machine → "+cfg.VMName, gcloud.Upload),
					huh.NewOption("Download — "+cfg.VMName+" → this machine", gcloud.Download),
				).
				Value(&m.spec.Direction),
			huh.NewInput().
				Title("Local path").
				Description("File or directory on this machine").
				Value(&m.spec.Local).
				Validate(validateLocalPath(m.spec)),
			huh.NewInput().
				Title("Remote path").
				Description("Path on the VM (~ is the VM's home directory)").
				Value(&m.spec.Remote).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("required")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Copy directories recursively").
				Description("Set automatically when the local path is a directory").
				Value(&m.spec.Recurse),
			huh.NewConfirm().
				Title("Compress in transit").
				Description("Helps on slow links, costs CPU on both ends").
				Value(&m.spec.Compress),
		),
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

// validateLocalPath checks that an upload source exists, and doubles as the
// place where --recurse is inferred: plain scp fails on a directory with an
// opaque error rather than telling you the flag is missing.
func validateLocalPath(spec *gcloud.TransferSpec) func(string) error {
	return func(s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			return fmt.Errorf("required")
		}

		// A download destination need not exist yet.
		if spec.Direction == gcloud.Download {
			return nil
		}

		st, err := os.Stat(config.CanonicalPath(s))
		if err != nil {
			return fmt.Errorf("no such file or directory")
		}
		if st.IsDir() {
			spec.Recurse = true
		}
		return nil
	}
}

func (m transferFormModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m transferFormModel) Update(msg tea.Msg) (transferFormModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return transferFormCancelMsg{} }
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		spec := *m.spec
		cfg := m.cfg
		return m, func() tea.Msg { return transferFormDoneMsg{cfg: cfg, spec: spec} }
	}

	return m, cmd
}

func (m transferFormModel) View() string {
	return titleStyle.Render("Copy Files — "+m.cfg.VMName) + "\n\n" +
		m.form.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// transferTunnelEntry returns the local port and PID of a live transfer tunnel
// in pids, or ("", 0) if there is none.
//
// Tunnel listings are otherwise driven by the VM's configured port mappings.
// A transfer tunnel is opened on demand and never written to tfvars, so it has
// to be read back out of the live process map instead.
func transferTunnelEntry(pids map[string]int) (string, int) {
	for key, pid := range pids {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) == 2 && parts[1] == tunnel.TransferRemotePort {
			return parts[0], pid
		}
	}
	return "", 0
}

// --- Transfer Tunnel Panel ---

// transferTunnelModel shows the generated native-tool commands for a live
// transfer tunnel. The commands are rendered with the VM's real values already
// substituted so they can be copied and run as-is.
type transferTunnelModel struct {
	cfg  config.Config
	port string
}

type transferTunnelCloseMsg struct{ cfg config.Config }

func newTransferTunnelModel(cfg config.Config, port string) transferTunnelModel {
	return transferTunnelModel{cfg: cfg, port: port}
}

func (m transferTunnelModel) Update(msg tea.Msg) (transferTunnelModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "x":
			cfg := m.cfg
			return m, func() tea.Msg { return transferTunnelCloseMsg{cfg: cfg} }
		case "esc", "q", "enter":
			return m, func() tea.Msg { return backToMenuMsg{} }
		}
	}
	return m, nil
}

func (m transferTunnelModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Transfer Tunnel — " + m.cfg.VMName))
	b.WriteString("\n\n")
	b.WriteString(successStyle.Render(fmt.Sprintf("Listening on localhost:%s → %s:22", m.port, m.cfg.VMName)))
	b.WriteString("\n\n")
	b.WriteString(infoStyle.Render("Run any of these from another terminal:"))
	b.WriteString("\n\n")

	for _, c := range gcloud.TransferCommands(m.cfg, m.port) {
		b.WriteString("  " + c + "\n\n")
	}

	b.WriteString(dimStyle.Render(gcloud.TransferHint(m.cfg.VMName)))
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("x close tunnel • esc back"))

	return b.String()
}

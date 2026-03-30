package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"vds-gcp-launch-instance/internal/config"
	"vds-gcp-launch-instance/internal/platform"
)

type configModel struct {
	form   *huh.Form
	cfg    *config.Config
	isEdit bool
}

type configDoneMsg struct {
	cfg config.Config
}

type configCancelMsg struct{}

func newConfigModel() configModel {
	username := platform.DetectUsername()
	project := platform.DetectGCPProject()
	ts := platform.GenerateTimestamp()
	pw := platform.GeneratePassword()

	m := configModel{
		cfg: &config.Config{
			Username:     username,
			Password:     pw,
			Timestamp:    ts,
			ProjectID:    project,
			VMName:       fmt.Sprintf("instance-%s", ts),
			Image:        "vds-debian-13-base",
			Region:       "us-central1",
			Zone:         "us-central1-a",
			MachineType:  "e2-highmem-2",
			BootDiskSize: "20",
			PortMapping:  "8787:8787",
		},
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Username").
				Description("GCP username (part before @ in email)").
				Value(&m.cfg.Username),
			huh.NewInput().
				Title("Project ID").
				Description("GCP project to create the instance in").
				Value(&m.cfg.ProjectID),
			huh.NewInput().
				Title("VM Name").
				Description("Must be lowercase, no underscores").
				Value(&m.cfg.VMName),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Image").
				Options(
					huh.NewOption("vds-debian-13-base", "vds-debian-13-base"),
					huh.NewOption("vds-debian-13-rstudio-4-5-3", "vds-debian-13-rstudio-4-5-3"),
					huh.NewOption("vds-ubuntu-2404-lts-amd64-base", "vds-ubuntu-2404-lts-amd64-base"),
					huh.NewOption("vds-ubuntu-2404-lts-amd64-rstudio-4-5-3", "vds-ubuntu-2404-lts-amd64-rstudio-4-5-3"),
				).
				Value(&m.cfg.Image),
			huh.NewInput().
				Title("Region").
				Value(&m.cfg.Region),
			huh.NewInput().
				Title("Zone").
				Value(&m.cfg.Zone),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Machine Type").
				Options(
					huh.NewOption("e2-highmem-2 (2 vCPU, 16 GB)", "e2-highmem-2"),
					huh.NewOption("e2-highmem-4 (4 vCPU, 32 GB)", "e2-highmem-4"),
					huh.NewOption("e2-highmem-8 (8 vCPU, 64 GB)", "e2-highmem-8"),
					huh.NewOption("e2-standard-2 (2 vCPU, 8 GB)", "e2-standard-2"),
					huh.NewOption("e2-standard-4 (4 vCPU, 16 GB)", "e2-standard-4"),
				).
				Value(&m.cfg.MachineType),
			huh.NewInput().
				Title("Boot Disk Size (GB)").
				Description("OS and system files — destroyed with the VM").
				Value(&m.cfg.BootDiskSize),
			huh.NewInput().
				Title("Port Mapping").
				Description("Comma-separated local:remote (e.g. 8787:8787,2222:22)").
				Value(&m.cfg.PortMapping),
		),
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

func (m configModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m configModel) Update(msg tea.Msg) (configModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return configCancelMsg{} }
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		cfg := *m.cfg
		return m, func() tea.Msg { return configDoneMsg{cfg: cfg} }
	}

	return m, cmd
}

func newEditConfigModel(cfg config.Config) configModel {
	m := configModel{
		cfg:    &cfg,
		isEdit: true,
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Locked Settings").
				Description(fmt.Sprintf(
					"Project ID:  %s\nImage:       %s\nRegion:      %s\nZone:        %s\n\nThese settings cannot be changed because\nmodifying them would destroy and recreate the VM.",
					cfg.ProjectID, cfg.Image, cfg.Region, cfg.Zone,
				)).
				Next(true).
				NextLabel("Continue"),
		),
		huh.NewGroup(
			huh.NewNote().
				Title("VM Name").
				Description(cfg.VMName),
			huh.NewInput().
				Title("Username").
				Description("GCP username (part before @ in email)").
				Value(&m.cfg.Username),
			huh.NewSelect[string]().
				Title("Machine Type").
				Options(
					huh.NewOption("e2-highmem-2 (2 vCPU, 16 GB)", "e2-highmem-2"),
					huh.NewOption("e2-highmem-4 (4 vCPU, 32 GB)", "e2-highmem-4"),
					huh.NewOption("e2-highmem-8 (8 vCPU, 64 GB)", "e2-highmem-8"),
					huh.NewOption("e2-standard-2 (2 vCPU, 8 GB)", "e2-standard-2"),
					huh.NewOption("e2-standard-4 (4 vCPU, 16 GB)", "e2-standard-4"),
				).
				Value(&m.cfg.MachineType),
			huh.NewInput().
				Title("Boot Disk Size (GB)").
				Description("OS and system files — destroyed with the VM").
				Value(&m.cfg.BootDiskSize),
			huh.NewInput().
				Title("Port Mapping").
				Description("Comma-separated local:remote (e.g. 8787:8787,2222:22)").
				Value(&m.cfg.PortMapping),
		),
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

func (m configModel) View() string {
	title := "Configure New VM"
	if m.isEdit {
		title = fmt.Sprintf("Edit VM: %s", m.cfg.VMName)
	}
	return titleStyle.Render(title) + "\n\n" + m.form.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

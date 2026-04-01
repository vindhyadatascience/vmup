package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"vds-gcp-launch-instance/internal/config"
	"vds-gcp-launch-instance/internal/gcloud"
	"vds-gcp-launch-instance/internal/platform"
)

// --- Common machine types (hardcoded specs) ---

type machineSpec struct {
	Name     string
	VCPUs    int
	MemoryGB float64
}

var commonMachineTypes = []machineSpec{
	{"e2-highmem-2", 2, 16},
	{"e2-highmem-4", 4, 32},
	{"e2-highmem-8", 8, 64},
	{"e2-standard-2", 2, 8},
	{"e2-standard-4", 4, 16},
}

const seeMoreSentinel = "__see_more__"

// --- Messages ---

type configDoneMsg struct {
	cfg config.Config
}

type configCancelMsg struct{}

type billingRatesMsg struct {
	rates map[string]gcloud.MachineTypeRate
}

type machineTypesMsg struct {
	types []gcloud.MachineTypeInfo
	err   error
}

// --- Config phases ---

type configPhase int

const (
	configPhaseLoading configPhase = iota // fetching billing rates
	configPhaseForm                       // main form
	configPhaseExpanding                  // fetching full machine type list
)

// --- Model ---

type configModel struct {
	phase   configPhase
	form    *huh.Form
	cfg     *config.Config
	isEdit  bool
	spinner spinner.Model
	loadStart time.Time

	// Pricing state
	billingRates    map[string]gcloud.MachineTypeRate
	allMachineTypes []gcloud.MachineTypeInfo
	expandedList    bool
}

func newConfigModel() configModel {
	username := platform.DetectUsername()
	project := platform.DetectGCPProject()
	ts := platform.GenerateTimestamp()
	pw := platform.GeneratePassword()

	s := spinner.New()
	s.Spinner = spinner.Dot

	m := configModel{
		phase:     configPhaseLoading,
		spinner:   s,
		loadStart: time.Now(),
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

	return m
}

func newEditConfigModel(cfg config.Config) configModel {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return configModel{
		phase:     configPhaseLoading,
		spinner:   s,
		loadStart: time.Now(),
		cfg:       &cfg,
		isEdit:    true,
	}
}

func (m configModel) Init() tea.Cmd {
	region := m.cfg.Region
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			rates := gcloud.FetchComputeRates(region)
			return billingRatesMsg{rates: rates}
		},
	)
}

func (m configModel) Update(msg tea.Msg) (configModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Only ctrl+c cancels — esc is handled by huh forms internally
		if msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return configCancelMsg{} }
		}
		// Allow esc to cancel only during loading phases (no huh form active)
		if msg.String() == "esc" && m.phase != configPhaseForm {
			return m, func() tea.Msg { return configCancelMsg{} }
		}

	case billingRatesMsg:
		m.billingRates = msg.rates
		if m.phase == configPhaseLoading {
			// Rates loaded — build and show the form
			m.phase = configPhaseForm
			m.form = m.buildForm()
			return m, m.form.Init()
		}
		// Rates arrived during expand — rebuild form with prices
		if m.phase == configPhaseForm {
			m.form = m.buildForm()
			return m, m.form.Init()
		}
		return m, nil

	case machineTypesMsg:
		if msg.err == nil && len(msg.types) > 0 {
			m.allMachineTypes = msg.types
			m.expandedList = true
		}
		m.phase = configPhaseForm
		m.form = m.buildForm()
		return m, m.form.Init()

	case spinner.TickMsg:
		if m.phase == configPhaseLoading || m.phase == configPhaseExpanding {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.phase != configPhaseForm {
		return m, nil
	}

	// Forward to huh form
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		// Check if "see more" was selected
		if m.cfg.MachineType == seeMoreSentinel {
			m.cfg.MachineType = commonMachineTypes[0].Name
			m.phase = configPhaseExpanding
			m.loadStart = time.Now()
			projectID := m.cfg.ProjectID
			zone := m.cfg.Zone
			region := m.cfg.Region
			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg {
					types, err := gcloud.FetchMachineTypes(projectID, zone)
					return machineTypesMsg{types: types, err: err}
				},
				func() tea.Msg {
					rates := gcloud.FetchComputeRates(region)
					return billingRatesMsg{rates: rates}
				},
			)
		}

		cfg := *m.cfg
		return m, func() tea.Msg { return configDoneMsg{cfg: cfg} }
	}

	return m, cmd
}

func (m configModel) View() string {
	title := "Configure New VM"
	if m.isEdit {
		title = fmt.Sprintf("Edit VM: %s", m.cfg.VMName)
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	switch m.phase {
	case configPhaseLoading:
		ts := formatElapsed(time.Since(m.loadStart))
		b.WriteString(m.spinner.View() + " " + dimStyle.Render("Fetching pricing information... ("+ts+")"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("esc/ctrl+c cancel"))
	case configPhaseExpanding:
		ts := formatElapsed(time.Since(m.loadStart))
		b.WriteString(m.spinner.View() + " " + dimStyle.Render("Loading all machine types... ("+ts+")"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("esc/ctrl+c cancel"))
	case configPhaseForm:
		b.WriteString(m.form.View())
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("ctrl+c cancel"))
	}

	return b.String()
}

// --- Form builder ---

func (m *configModel) buildForm() *huh.Form {
	if m.isEdit {
		return m.buildEditForm()
	}
	return m.buildNewForm()
}

func (m *configModel) buildNewForm() *huh.Form {
	return huh.NewForm(
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
				Options(m.machineTypeOptions()...).
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
}

func (m *configModel) buildEditForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Locked Settings").
				Description(fmt.Sprintf(
					"Project ID:  %s\nImage:       %s\nRegion:      %s\nZone:        %s\n\nThese settings cannot be changed because\nmodifying them would destroy and recreate the VM.",
					m.cfg.ProjectID, m.cfg.Image, m.cfg.Region, m.cfg.Zone,
				)).
				Next(true).
				NextLabel("Continue"),
		),
		huh.NewGroup(
			huh.NewNote().
				Title("VM Name").
				Description(m.cfg.VMName),
			huh.NewInput().
				Title("Username").
				Description("GCP username (part before @ in email)").
				Value(&m.cfg.Username),
			huh.NewSelect[string]().
				Title("Machine Type").
				Options(m.machineTypeOptions()...).
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
}

// --- Machine type option building ---

func (m *configModel) machineTypeOptions() []huh.Option[string] {
	var opts []huh.Option[string]
	if m.expandedList && len(m.allMachineTypes) > 0 {
		opts = m.buildExpandedOptions()
	} else {
		opts = m.buildCommonOptions()
	}
	if m.isEdit {
		opts = ensureCurrentTypeInOptions(opts, m.cfg.MachineType)
	}
	return opts
}

func (m *configModel) buildCommonOptions() []huh.Option[string] {
	var opts []huh.Option[string]
	for _, mt := range commonMachineTypes {
		label := formatMachineLabel(mt.Name, mt.VCPUs, mt.MemoryGB, m.billingRates)
		opts = append(opts, huh.NewOption(label, mt.Name))
	}
	opts = append(opts, huh.NewOption("See more...", seeMoreSentinel))
	return opts
}

func (m *configModel) buildExpandedOptions() []huh.Option[string] {
	types := make([]gcloud.MachineTypeInfo, len(m.allMachineTypes))
	copy(types, m.allMachineTypes)
	sort.Slice(types, func(i, j int) bool {
		fi := gcloud.MachineFamily(types[i].Name)
		fj := gcloud.MachineFamily(types[j].Name)
		if fi != fj {
			return fi < fj
		}
		if types[i].GuestCpus != types[j].GuestCpus {
			return types[i].GuestCpus < types[j].GuestCpus
		}
		return types[i].MemoryMB < types[j].MemoryMB
	})

	var opts []huh.Option[string]

	commonSet := make(map[string]bool)
	for _, mt := range commonMachineTypes {
		commonSet[mt.Name] = true
		label := "★ " + formatMachineLabel(mt.Name, mt.VCPUs, mt.MemoryGB, m.billingRates)
		opts = append(opts, huh.NewOption(label, mt.Name))
	}

	for _, mt := range types {
		if commonSet[mt.Name] {
			continue
		}
		memGB := float64(mt.MemoryMB) / 1024.0
		label := formatMachineLabel(mt.Name, mt.GuestCpus, memGB, m.billingRates)
		opts = append(opts, huh.NewOption(label, mt.Name))
	}

	return opts
}

func formatMachineLabel(name string, vcpus int, memGB float64, rates map[string]gcloud.MachineTypeRate) string {
	var memStr string
	if memGB == float64(int(memGB)) {
		memStr = fmt.Sprintf("%d", int(memGB))
	} else {
		memStr = fmt.Sprintf("%.1f", memGB)
	}

	label := fmt.Sprintf("%s (%d vCPU, %s GB)", name, vcpus, memStr)

	if rates != nil {
		family := gcloud.MachineFamily(name)
		hourly := gcloud.CalculateHourlyRate(family, vcpus, memGB, rates)
		if hourly > 0 {
			label += fmt.Sprintf(" ~$%.2f/hr", hourly)
		}
	}

	return label
}

func ensureCurrentTypeInOptions(opts []huh.Option[string], currentType string) []huh.Option[string] {
	if currentType == "" {
		return opts
	}
	for _, o := range opts {
		if strings.EqualFold(o.Value, currentType) {
			return opts
		}
	}
	label := currentType + " (current)"
	return append([]huh.Option[string]{huh.NewOption(label, currentType)}, opts...)
}

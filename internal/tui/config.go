package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"vmup/internal/config"
	"vmup/internal/gcloud"
	"vmup/internal/platform"
)

// --- Common machine types (hardcoded specs, used as fallback) ---

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

// --- Messages ---

type configDoneMsg struct {
	cfg config.Config
}

type configCancelMsg struct{}

type configDataReadyMsg struct {
	rates        map[string]gcloud.MachineTypeRate
	machineTypes []gcloud.MachineTypeInfo
	gcpProject   string // detected from gcloud config

	customImages      []gcloud.ImageInfo
	standardImages    []gcloud.ImageInfo
	imageDenied       bool   // configured image project returned 403/404
	imageProjectTried string // the image project that was attempted
}

// --- Phases ---

type configPhase int

const (
	configPhaseLoading configPhase = iota
	configPhaseForm
)

// --- Model ---

type configModel struct {
	phase   configPhase
	form    *huh.Form
	cfg     *config.Config
	isEdit  bool
	spinner spinner.Model
	loadStart time.Time

	billingRates    map[string]gcloud.MachineTypeRate
	allMachineTypes []gcloud.MachineTypeInfo

	// Image picker state
	imageProjectSetting string // effective image-project setting to list custom images from
	customImages        []gcloud.ImageInfo
	standardImages      []gcloud.ImageInfo
	imageChoice         string // composite "project/name" bound to the Select
	imageNotice         string // transient notice (e.g. image-project access cleared)
}

func newConfigModel() configModel {
	// Derive the username and IAP email domain from the active gcloud account
	// (e.g. user@example.com → "user", "example.com"); fall back to the OS user.
	username, domain := platform.DetectUserAccount()
	if username == "" {
		username = platform.DetectUsername()
	}
	ts := platform.GenerateTimestamp()
	pw := platform.GeneratePassword()

	imageProject := config.LoadSettings().EffectiveImageProject()

	s := spinner.New()
	s.Spinner = spinner.Dot

	return configModel{
		phase:               configPhaseLoading,
		spinner:             s,
		loadStart:           time.Now(),
		imageProjectSetting: imageProject,
		cfg: &config.Config{
			Username:     username,
			UserDomain:   domain,
			Password:     pw,
			Timestamp:    ts,
			ProjectID:    "", // filled by background fetch
			VMName:       fmt.Sprintf("instance-%s", ts),
			Image:        "", // selected from the fetched image list
			ImageProject: imageProject,
			Region:       "us-central1",
			Zone:         "us-central1-a",
			MachineType:  "e2-highmem-2",
			BootDiskSize: "20",
			PortMapping:  "8787:8787",
		},
	}
}

func newEditConfigModel(cfg config.Config) configModel {
	// Backfill the IAP domain for VMs created before user-domain was tracked,
	// so re-applying an older project produces a valid IAM member.
	if cfg.UserDomain == "" {
		if _, domain := platform.DetectUserAccount(); domain != "" {
			cfg.UserDomain = domain
		}
	}

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
	projectID := m.cfg.ProjectID
	zone := m.cfg.Zone
	imageProject := m.imageProjectSetting
	isNew := !m.isEdit
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			type ratesResult struct {
				rates map[string]gcloud.MachineTypeRate
			}
			type typesResult struct {
				types []gcloud.MachineTypeInfo
			}
			type projectResult struct {
				project string
			}
			type imagesResult struct {
				custom   []gcloud.ImageInfo
				standard []gcloud.ImageInfo
				denied   bool
			}

			ratesCh := make(chan ratesResult, 1)
			typesCh := make(chan typesResult, 1)
			projectCh := make(chan projectResult, 1)
			imagesCh := make(chan imagesResult, 1)

			go func() {
				r := gcloud.FetchComputeRates(region)
				ratesCh <- ratesResult{rates: r}
			}()
			go func() {
				// For new VMs, projectID is empty — need to detect it first
				pid := projectID
				if pid == "" {
					pid = platform.DetectGCPProject()
				}
				t, _ := gcloud.FetchMachineTypes(pid, zone)
				typesCh <- typesResult{types: t}
				projectCh <- projectResult{project: pid}
			}()
			if isNew {
				go func() {
					var res imagesResult
					if imageProject != "" {
						imgs, err := gcloud.FetchImages(imageProject)
						if err != nil {
							res.denied = gcloud.IsAccessDenied(err)
						} else {
							res.custom = imgs
						}
					}
					res.standard, _ = gcloud.FetchStandardImages()
					imagesCh <- res
				}()
			}

			rr := <-ratesCh
			tr := <-typesCh
			var gcpProject string
			var ir imagesResult
			if isNew {
				pr := <-projectCh
				gcpProject = pr.project
				ir = <-imagesCh
			}
			return configDataReadyMsg{
				rates:             rr.rates,
				machineTypes:      tr.types,
				gcpProject:        gcpProject,
				customImages:      ir.custom,
				standardImages:    ir.standard,
				imageDenied:       ir.denied,
				imageProjectTried: imageProject,
			}
		},
	)
}

func (m configModel) Update(msg tea.Msg) (configModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return configCancelMsg{} }
		}
		if msg.String() == "esc" && m.phase == configPhaseLoading {
			return m, func() tea.Msg { return configCancelMsg{} }
		}

	case configDataReadyMsg:
		m.billingRates = msg.rates
		m.allMachineTypes = msg.machineTypes
		if msg.gcpProject != "" && m.cfg.ProjectID == "" {
			m.cfg.ProjectID = msg.gcpProject
		}
		m.customImages = msg.customImages
		m.standardImages = msg.standardImages
		if msg.imageDenied {
			// Self-heal: the configured image project is unusable for this
			// user. Clear the setting (once) so future runs skip it, and tell
			// the user why only the standard images are shown.
			s := config.LoadSettings()
			empty := ""
			s.ImageProject = &empty
			_ = config.SaveSettings(s)
			m.imageNotice = fmt.Sprintf(
				"No access to image project %q — showing standard images (setting cleared).",
				msg.imageProjectTried,
			)
		}
		m.imageChoice = m.defaultImageChoice()
		m.phase = configPhaseForm
		m.form = m.buildForm()
		return m, m.form.Init()

	case spinner.TickMsg:
		if m.phase == configPhaseLoading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.phase != configPhaseForm {
		return m, nil
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		// Resolve the picked image into the per-VM image + project. The
		// fallback (no images fetched) path sets cfg.Image/ImageProject
		// directly via inputs and leaves imageChoice empty.
		if !m.isEdit && m.imageChoice != "" && m.imageChoice != imageDividerValue {
			proj, name := parseImageKey(m.imageChoice)
			m.cfg.ImageProject = proj
			m.cfg.Image = name
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
		b.WriteString(m.spinner.View() + " " + dimStyle.Render("Fetching pricing and machine types... ("+ts+")"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("esc/ctrl+c cancel"))
	case configPhaseForm:
		if m.imageNotice != "" {
			b.WriteString(dimStyle.Render("⚠ " + m.imageNotice))
			b.WriteString("\n\n")
		}
		b.WriteString(m.form.View())
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("ctrl+c cancel"))
	}

	return b.String()
}

// --- Form builder ---

func (m *configModel) buildForm() *huh.Form {
	if m.isEdit {
		return huh.NewForm(
			huh.NewGroup(
				huh.NewNote().
					Title("Locked Settings").
					Description(fmt.Sprintf(
						"Project ID:    %s\nImage:         %s\nImage Project: %s\nRegion:        %s\nZone:          %s\n\nThese settings cannot be changed because\nmodifying them would destroy and recreate the VM.",
						m.cfg.ProjectID, m.cfg.Image, m.cfg.ImageProject, m.cfg.Region, m.cfg.Zone,
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
				huh.NewInput().
					Title("User Domain").
					Description("Email domain for IAP access (e.g. example.com)").
					Value(&m.cfg.UserDomain),
				huh.NewSelect[string]().
					Title("Machine Type").
					Height(10).
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

	// Image group: a picker of fetched images, or free-form inputs when none
	// could be fetched (e.g. gcloud is not authenticated).
	imageGroup := append(m.imageFields(),
		huh.NewInput().
			Title("Region").
			Value(&m.cfg.Region),
		huh.NewInput().
			Title("Zone").
			Value(&m.cfg.Zone),
	)

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Username").
				Description("GCP username (part before @ in email)").
				Value(&m.cfg.Username),
			huh.NewInput().
				Title("User Domain").
				Description("Email domain for IAP access (e.g. example.com)").
				Value(&m.cfg.UserDomain),
			huh.NewInput().
				Title("Project ID").
				Description("GCP project to create the instance in").
				Value(&m.cfg.ProjectID),
			huh.NewInput().
				Title("VM Name").
				Description("Must be lowercase, no underscores").
				Value(&m.cfg.VMName),
		),
		huh.NewGroup(imageGroup...),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Machine Type").
				Height(10).
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

// --- Image selection ---

// imageDividerValue is the sentinel option value separating custom-project
// images from the standard public images; selecting it is rejected.
const imageDividerValue = "__divider__"

// imageFields returns the form field(s) for choosing an image: a grouped Select
// when images were fetched, or free-form project/name inputs as a fallback.
func (m *configModel) imageFields() []huh.Field {
	opts := m.imageOptions()
	if len(opts) == 0 {
		return []huh.Field{
			huh.NewInput().
				Title("Image Project").
				Description("GCP project that hosts the image").
				Value(&m.cfg.ImageProject),
			huh.NewInput().
				Title("Image").
				Description("Image name within that project").
				Value(&m.cfg.Image),
		}
	}
	return []huh.Field{
		huh.NewSelect[string]().
			Title("Image").
			Height(10).
			Options(opts...).
			Validate(func(v string) error {
				if v == imageDividerValue {
					return fmt.Errorf("select an image")
				}
				return nil
			}).
			Value(&m.imageChoice),
	}
}

// imageOptions builds the picker options: custom-project images first (starred),
// then a divider, then the standard public images.
func (m *configModel) imageOptions() []huh.Option[string] {
	var opts []huh.Option[string]
	for _, img := range m.customImages {
		opts = append(opts, huh.NewOption("★ "+imageLabel(img), imageKey(img)))
	}
	if len(m.customImages) > 0 && len(m.standardImages) > 0 {
		opts = append(opts, huh.NewOption("──────── standard images ────────", imageDividerValue))
	}
	for _, img := range m.standardImages {
		opts = append(opts, huh.NewOption(imageLabel(img), imageKey(img)))
	}
	return opts
}

func (m *configModel) defaultImageChoice() string {
	if len(m.customImages) > 0 {
		return imageKey(m.customImages[0])
	}
	if len(m.standardImages) > 0 {
		return imageKey(m.standardImages[0])
	}
	return ""
}

func imageLabel(img gcloud.ImageInfo) string {
	if img.Family != "" {
		return img.Name + "  (" + img.Family + ")"
	}
	return img.Name
}

// imageKey encodes an image as "project/name" for the Select value.
func imageKey(img gcloud.ImageInfo) string {
	return img.Project + "/" + img.Name
}

// parseImageKey splits a "project/name" Select value back into its parts.
func parseImageKey(key string) (project, name string) {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

// --- Machine type options ---

func (m *configModel) machineTypeOptions() []huh.Option[string] {
	commonSet := make(map[string]bool)
	var opts []huh.Option[string]

	// Common types first, starred
	for _, mt := range commonMachineTypes {
		commonSet[mt.Name] = true
		label := "★ " + formatMachineLabel(mt.Name, mt.VCPUs, mt.MemoryGB, m.billingRates)
		opts = append(opts, huh.NewOption(label, mt.Name))
	}

	// All other types from API, sorted by family then vCPU
	if len(m.allMachineTypes) > 0 {
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

		for _, mt := range types {
			if commonSet[mt.Name] {
				continue
			}
			memGB := float64(mt.MemoryMB) / 1024.0
			label := formatMachineLabel(mt.Name, mt.GuestCpus, memGB, m.billingRates)
			opts = append(opts, huh.NewOption(label, mt.Name))
		}
	}

	// Ensure current type is in list (edit mode with non-standard type)
	if m.isEdit {
		opts = ensureCurrentTypeInOptions(opts, m.cfg.MachineType)
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
		hourly := gcloud.CalculateHourlyRate(name, family, vcpus, memGB, rates)
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

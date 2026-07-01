package tui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/vindhyadatascience/vmup/internal/config"
	"github.com/vindhyadatascience/vmup/internal/gcloud"
	"github.com/vindhyadatascience/vmup/internal/platform"
)

// mtStore caches machine types per zone. It is held by pointer on configModel so
// the cache is shared across the value-copied Bubble Tea model and safe for the
// goroutines huh runs OptionsFunc in.
type mtStore struct {
	mu sync.Mutex
	m  map[string][]gcloud.MachineTypeInfo
}

func newMTStore() *mtStore { return &mtStore{m: map[string][]gcloud.MachineTypeInfo{}} }

func (s *mtStore) put(zone string, types []gcloud.MachineTypeInfo) {
	s.mu.Lock()
	s.m[zone] = types
	s.mu.Unlock()
}

// getOrFetch returns the cached machine types for a zone, fetching and caching
// them on the first request for that zone.
func (s *mtStore) getOrFetch(project, zone string) []gcloud.MachineTypeInfo {
	s.mu.Lock()
	v, ok := s.m[zone]
	s.mu.Unlock()
	if ok {
		return v
	}
	t, _ := gcloud.FetchMachineTypes(project, zone)
	s.mu.Lock()
	s.m[zone] = t
	s.mu.Unlock()
	return t
}

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
	gcpProject    string              // detected from gcloud config
	regionZones   map[string][]string // region → zones
	editImageArch string              // architecture of the edited VM's locked image

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

	// Region/zone selection (new VMs)
	regions     []string            // sorted list of all regions
	regionZones map[string][]string // region → sorted zones
	mt          *mtStore            // machine types cached per zone

	// Image picker state
	imageProjectSetting string // effective image-project setting to list custom images from
	customImages        []gcloud.ImageInfo
	standardImages      []gcloud.ImageInfo
	imageArch           map[string]string // imageKey → architecture (X86_64/ARM64)
	editImageArch       string            // architecture of the locked image (edit mode)
	imageChoice         *string           // composite "project/name" bound to the Select; heap-allocated so the huh binding survives value-receiver Update copies (like cfg)
	imageNotice         string            // transient notice (e.g. image-project access cleared)
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
		mt:                  newMTStore(),
		imageArch:           map[string]string{},
		imageChoice:         new(string),
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
		phase:       configPhaseLoading,
		spinner:     s,
		loadStart:   time.Now(),
		cfg:         &cfg,
		isEdit:      true,
		imageChoice: new(string),
	}
}

func (m configModel) Init() tea.Cmd {
	region := m.cfg.Region
	projectID := m.cfg.ProjectID
	zone := m.cfg.Zone
	imageProject := m.imageProjectSetting
	editImage := m.cfg.Image
	editImageProject := m.cfg.ImageProject
	isNew := !m.isEdit
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			// Resolve the project once (new VMs start with an empty project id).
			pid := projectID
			if pid == "" {
				pid = platform.DetectGCPProject()
			}

			type ratesResult struct{ rates map[string]gcloud.MachineTypeRate }
			type typesResult struct{ types []gcloud.MachineTypeInfo }
			type imagesResult struct {
				custom   []gcloud.ImageInfo
				standard []gcloud.ImageInfo
				denied   bool
			}

			ratesCh := make(chan ratesResult, 1)
			typesCh := make(chan typesResult, 1)
			imagesCh := make(chan imagesResult, 1)
			zonesCh := make(chan map[string][]string, 1)
			editArchCh := make(chan string, 1)

			go func() { ratesCh <- ratesResult{rates: gcloud.FetchComputeRates(region)} }()
			go func() {
				t, _ := gcloud.FetchMachineTypes(pid, zone)
				typesCh <- typesResult{types: t}
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
				go func() {
					z, _ := gcloud.FetchZonesByRegion(pid)
					zonesCh <- z
				}()
			} else {
				go func() {
					a, _ := gcloud.FetchImageArch(editImageProject, editImage)
					editArchCh <- a
				}()
			}

			rr := <-ratesCh
			tr := <-typesCh
			msg := configDataReadyMsg{rates: rr.rates, machineTypes: tr.types}
			if isNew {
				ir := <-imagesCh
				msg.gcpProject = pid
				msg.customImages = ir.custom
				msg.standardImages = ir.standard
				msg.imageDenied = ir.denied
				msg.imageProjectTried = imageProject
				msg.regionZones = <-zonesCh
			} else {
				msg.editImageArch = <-editArchCh
			}
			return msg
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
		m.editImageArch = msg.editImageArch
		if msg.gcpProject != "" && m.cfg.ProjectID == "" {
			m.cfg.ProjectID = msg.gcpProject
		}
		m.customImages = msg.customImages
		m.standardImages = msg.standardImages
		m.regionZones = msg.regionZones
		m.regions = sortedRegions(msg.regionZones)
		m.imageArch = buildImageArchMap(msg.customImages, msg.standardImages)
		if m.mt != nil {
			m.mt.put(m.cfg.Zone, msg.machineTypes)
		}
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
		*m.imageChoice = m.defaultImageChoice()
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
		if !m.isEdit && *m.imageChoice != "" && *m.imageChoice != imageDividerValue {
			proj, name := parseImageKey(*m.imageChoice)
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

// reopen rebuilds the form (resetting its completed state) and returns the
// command to re-initialise it. Used when the user backs out of the create/update
// confirmation so they land on the still-populated, editable form.
func (m *configModel) reopen() tea.Cmd {
	m.form = m.buildForm()
	return m.form.Init()
}

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
					Options(m.buildMachineOptions(m.allMachineTypes, m.editImageArch)...).
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

	// Image group: a picker of fetched images (or free-form inputs as a
	// fallback), plus Region/Zone selects. Zone options react to the chosen
	// Region, and the Machine Type options (below) react to Zone + image arch.
	imageGroup := append(m.imageFields(), m.regionField(), m.zoneField())

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
			m.machineTypeField(),
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
			Value(m.imageChoice),
	}
}

// imageOptions builds the picker options: custom-project images first (starred),
// then a divider, then the standard public images. Standard images that duplicate
// a custom image (same project/name) are omitted so each image appears once —
// the custom image project also surfaces via `gcloud compute images list`.
func (m *configModel) imageOptions() []huh.Option[string] {
	var opts []huh.Option[string]
	custom := make(map[string]bool, len(m.customImages))
	for _, img := range m.customImages {
		custom[imageKey(img)] = true
		opts = append(opts, huh.NewOption("★ "+imageLabel(img), imageKey(img)))
	}

	var standard []huh.Option[string]
	for _, img := range m.standardImages {
		if custom[imageKey(img)] {
			continue
		}
		standard = append(standard, huh.NewOption(imageLabel(img), imageKey(img)))
	}
	if len(opts) > 0 && len(standard) > 0 {
		opts = append(opts, huh.NewOption("──────── standard images ────────", imageDividerValue))
	}
	return append(opts, standard...)
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

// --- Region / zone / machine type fields ---

// regionField is a Select of all regions (or a free-text input if the region
// list could not be fetched).
func (m *configModel) regionField() huh.Field {
	if len(m.regions) == 0 {
		return huh.NewInput().Title("Region").Value(&m.cfg.Region)
	}
	// Value must be set before Options: huh only scrolls the viewport to the
	// selected option inside Options(), and only if the value accessor is
	// already wired up. Otherwise the list stays scrolled to the top and the
	// us-central1 selection is off-screen until the field is focused.
	return huh.NewSelect[string]().
		Title("Region").
		Height(8).
		Value(&m.cfg.Region).
		Options(toOptions(m.regions)...)
}

// zoneField is a Select whose options are the zones in the chosen Region (it
// re-evaluates when Region changes), or a free-text input as a fallback.
func (m *configModel) zoneField() huh.Field {
	if len(m.regionZones) == 0 {
		return huh.NewInput().Title("Zone").Value(&m.cfg.Zone)
	}
	return huh.NewSelect[string]().
		Title("Zone").
		Height(8).
		OptionsFunc(func() []huh.Option[string] {
			return toOptions(m.regionZones[m.cfg.Region])
		}, &m.cfg.Region).
		Value(&m.cfg.Zone)
}

// machineTypeField is a Select whose options react to the chosen Zone (machine
// types are fetched per-zone and cached) and the selected image's architecture
// (incompatible machine types are filtered out so the VM can actually boot).
func (m *configModel) machineTypeField() huh.Field {
	return huh.NewSelect[string]().
		Title("Machine Type").
		Height(10).
		OptionsFunc(func() []huh.Option[string] {
			mts := m.mt.getOrFetch(m.cfg.ProjectID, m.cfg.Zone)
			return m.buildMachineOptions(mts, m.imageArch[*m.imageChoice])
		}, []any{&m.cfg.Zone, m.imageChoice}).
		Value(&m.cfg.MachineType)
}

// toOptions builds Select options whose label and value are the same string.
func toOptions(values []string) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(values))
	for _, v := range values {
		opts = append(opts, huh.NewOption(v, v))
	}
	return opts
}

func sortedRegions(rz map[string][]string) []string {
	regions := make([]string, 0, len(rz))
	for r := range rz {
		regions = append(regions, r)
	}
	sort.Strings(regions)
	return regions
}

func buildImageArchMap(custom, standard []gcloud.ImageInfo) map[string]string {
	arch := make(map[string]string, len(custom)+len(standard))
	for _, img := range custom {
		arch[imageKey(img)] = img.Architecture
	}
	for _, img := range standard {
		arch[imageKey(img)] = img.Architecture
	}
	return arch
}

// buildMachineOptions builds the machine-type Select options: compatible
// recommended (★) types first, then the rest — each group sorted by estimated
// hourly cost (cheapest first). When wantArch is non-empty, only machine types
// of that architecture are included so the choice can't mismatch the selected
// image. Falls back to the curated common list when the API returned none.
func (m *configModel) buildMachineOptions(mts []gcloud.MachineTypeInfo, wantArch string) []huh.Option[string] {
	recommended := make(map[string]bool, len(commonMachineTypes))
	for _, c := range commonMachineTypes {
		recommended[c.Name] = true
	}

	// Fall back to the curated common list when the API returned nothing.
	if len(mts) == 0 {
		for _, c := range commonMachineTypes {
			mts = append(mts, gcloud.MachineTypeInfo{
				Name:      c.Name,
				GuestCpus: c.VCPUs,
				MemoryMB:  int(c.MemoryGB * 1024),
			})
		}
	}

	// Filter by architecture.
	var types []gcloud.MachineTypeInfo
	for _, mt := range mts {
		if wantArch == "" || mt.Arch() == wantArch {
			types = append(types, mt)
		}
	}

	// Sort by hourly cost (cheapest first). Unknown costs sort last; ties (and
	// the all-unknown case) fall back to vCPU then memory.
	sort.Slice(types, func(i, j int) bool {
		ri, rj := m.hourlyRate(types[i]), m.hourlyRate(types[j])
		if (ri == 0) != (rj == 0) {
			return rj == 0
		}
		if ri != rj {
			return ri < rj
		}
		if types[i].GuestCpus != types[j].GuestCpus {
			return types[i].GuestCpus < types[j].GuestCpus
		}
		return types[i].MemoryMB < types[j].MemoryMB
	})

	// Recommended (★) types first, then the rest. Each group stays in cost order
	// because `types` is already cost-sorted.
	var top, rest []huh.Option[string]
	for _, mt := range types {
		memGB := float64(mt.MemoryMB) / 1024.0
		label := formatMachineLabel(mt.Name, mt.GuestCpus, memGB, m.billingRates)
		if recommended[mt.Name] {
			top = append(top, huh.NewOption("★ "+label, mt.Name))
		} else {
			rest = append(rest, huh.NewOption(label, mt.Name))
		}
	}
	opts := append(top, rest...)

	// Ensure the current type is in the list (edit mode with a non-standard
	// type) — but never re-add a type that mismatches the image's architecture,
	// so editing a VM with a bad machine type forces a compatible choice.
	if m.isEdit && (wantArch == "" || m.machineTypeArch(m.cfg.MachineType) == wantArch) {
		opts = ensureCurrentTypeInOptions(opts, m.cfg.MachineType)
	}

	return opts
}

// machineTypeArch returns the architecture of a machine type by name from the
// fetched machine-type list, or "" if unknown.
func (m *configModel) machineTypeArch(name string) string {
	for _, mt := range m.allMachineTypes {
		if mt.Name == name {
			return mt.Arch()
		}
	}
	return ""
}

// hourlyRate returns the estimated hourly cost of a machine type, or 0 if it is
// unknown (no billing rates loaded, or no rate for the family).
func (m *configModel) hourlyRate(mt gcloud.MachineTypeInfo) float64 {
	if m.billingRates == nil {
		return 0
	}
	memGB := float64(mt.MemoryMB) / 1024.0
	return gcloud.CalculateHourlyRate(mt.Name, gcloud.MachineFamily(mt.Name), mt.GuestCpus, memGB, m.billingRates)
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

package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/vindhyadatascience/vmup/internal/config"
	"github.com/vindhyadatascience/vmup/internal/platform"
)

// --- Disk Create Form ---

type diskFormModel struct {
	form *huh.Form
	cfg  *config.DiskConfig
}

type diskFormDoneMsg struct {
	cfg config.DiskConfig
}

type diskFormCancelMsg struct{}

func newDiskCreateModel() diskFormModel {
	project := platform.DetectGCPProject()

	m := diskFormModel{
		cfg: &config.DiskConfig{
			Name:      "",
			ProjectID: project,
			Zone:      "us-central1-a",
			DiskType:  "pd-balanced",
			SizeGB:    "50",
		},
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Disk Name").
				Description("Must be lowercase, no underscores").
				Value(&m.cfg.Name),
			huh.NewInput().
				Title("Project ID").
				Description("GCP project to create the disk in").
				Value(&m.cfg.ProjectID),
			huh.NewInput().
				Title("Zone").
				Value(&m.cfg.Zone),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Disk Type").
				Options(
					huh.NewOption("pd-standard (HDD)", "pd-standard"),
					huh.NewOption("pd-balanced (Balanced SSD)", "pd-balanced"),
					huh.NewOption("pd-ssd (SSD)", "pd-ssd"),
				).
				Value(&m.cfg.DiskType),
			huh.NewInput().
				Title("Disk Size (GB)").
				Description("Minimum 10 GB").
				Value(&m.cfg.SizeGB).
				Validate(func(s string) error {
					n, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					if n < 10 {
						return fmt.Errorf("minimum 10 GB")
					}
					return nil
				}),
		),
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

func (m diskFormModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m diskFormModel) Update(msg tea.Msg) (diskFormModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return diskFormCancelMsg{} }
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		cfg := *m.cfg
		return m, func() tea.Msg { return diskFormDoneMsg{cfg: cfg} }
	}

	return m, cmd
}

func (m diskFormModel) View() string {
	return titleStyle.Render("Create New Data Disk") + "\n\n" + m.form.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Disk Import Form ---

type diskImportModel struct {
	form *huh.Form
	cfg  *config.DiskConfig
}

type diskImportDoneMsg struct {
	cfg config.DiskConfig
}

type diskImportCancelMsg struct{}

func newDiskImportModel() diskImportModel {
	project := platform.DetectGCPProject()

	m := diskImportModel{
		cfg: &config.DiskConfig{
			Name:      "",
			ProjectID: project,
			Zone:      "us-central1-a",
		},
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Disk Name").
				Description("Name of the existing GCP disk to import").
				Value(&m.cfg.Name),
			huh.NewInput().
				Title("Project ID").
				Description("GCP project containing the disk").
				Value(&m.cfg.ProjectID),
			huh.NewInput().
				Title("Zone").
				Description("Zone where the disk exists").
				Value(&m.cfg.Zone),
		),
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

func (m diskImportModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m diskImportModel) Update(msg tea.Msg) (diskImportModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return diskImportCancelMsg{} }
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		cfg := *m.cfg
		return m, func() tea.Msg { return diskImportDoneMsg{cfg: cfg} }
	}

	return m, cmd
}

func (m diskImportModel) View() string {
	return titleStyle.Render("Import Existing Disk") + "\n\n" + m.form.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Disk Resize Form ---

type diskResizeModel struct {
	form       *huh.Form
	cfg        *config.DiskConfig
	currentSize string
}

type diskResizeDoneMsg struct {
	cfg config.DiskConfig
}

type diskResizeCancelMsg struct{}

func newDiskResizeModel(cfg config.DiskConfig, currentSizeGB string) diskResizeModel {
	m := diskResizeModel{
		cfg:        &cfg,
		currentSize: currentSizeGB,
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Resize Disk: "+cfg.Name).
				Description(fmt.Sprintf("Current size: %s GB\nDisks can only be grown, never shrunk.", currentSizeGB)),
			huh.NewInput().
				Title("New Size (GB)").
				Value(&m.cfg.SizeGB).
				Validate(func(s string) error {
					n, err := strconv.Atoi(s)
					if err != nil {
						return fmt.Errorf("must be a number")
					}
					current, _ := strconv.Atoi(currentSizeGB)
					if n < current {
						return fmt.Errorf("cannot shrink disk (current: %s GB)", currentSizeGB)
					}
					if n == current {
						return fmt.Errorf("size is the same as current")
					}
					return nil
				}),
		),
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

func (m diskResizeModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m diskResizeModel) Update(msg tea.Msg) (diskResizeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return diskResizeCancelMsg{} }
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		cfg := *m.cfg
		return m, func() tea.Msg { return diskResizeDoneMsg{cfg: cfg} }
	}

	return m, cmd
}

func (m diskResizeModel) View() string {
	return titleStyle.Render("Resize Data Disk") + "\n\n" + m.form.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Disk Attach Form ---

// diskAttachFields holds mutable form-bound fields via pointer so that
// huh form writes survive BubbleTea's value-copy model updates.
type diskAttachFields struct {
	instanceName string
	mode         string
	mount        bool
	mountPoint   string
	formatDisk   bool
	fsType       string
	owner        string
	sourceTab    tab // which tab initiated the flow
}

type diskAttachModel struct {
	form         *huh.Form
	diskCfg      config.DiskConfig
	fields       *diskAttachFields
	vmProjectIDs map[string]string // vmName → projectID

	// Used when attaching from VM side (user picks a disk)
	diskCfgs       map[string]config.DiskConfig
	diskNames      []string
	selectedDisk   *string
	lastAttachDone diskAttachDoneMsg // stored for rebuilding form on back-nav
}

type diskAttachDoneMsg struct {
	diskCfg           config.DiskConfig
	instanceName      string
	instanceProjectID string
	mode              string
	mount             bool
	mountPoint        string
	formatDisk        bool
	fsType            string
	owner             string
	sourceTab         tab
}

type diskAttachCancelMsg struct{}

func newDiskAttachModel(diskCfg config.DiskConfig, vmNames []string, vmUsernames map[string]string, vmProjectIDs map[string]string, formatted bool) diskAttachModel {
	defaultOwner := ""
	if len(vmNames) > 0 {
		defaultOwner = vmUsernames[vmNames[0]]
	}

	f := &diskAttachFields{
		mode:       "rw",
		mount:      true,
		mountPoint: fmt.Sprintf("/mnt/disks/%s", diskCfg.Name),
		fsType:     "ext4",
		sourceTab:  tabDataDisks,
		owner:      defaultOwner,
	}

	m := diskAttachModel{
		diskCfg:      diskCfg,
		fields:       f,
		vmProjectIDs: vmProjectIDs,
	}

	if len(vmNames) == 0 {
		m.form = huh.NewForm(
			huh.NewGroup(
				huh.NewNote().
					Title("No Running VMs").
					Description(fmt.Sprintf("No running VMs found in project %s, zone %s.\nThe disk and VM must be in the same project and zone.\nThe VM must be running to attach and mount a disk.", diskCfg.ProjectID, diskCfg.Zone)).
					Next(true).
					NextLabel("Back"),
			),
		).WithShowHelp(true)
		return m
	}

	var opts []huh.Option[string]
	for _, name := range vmNames {
		opts = append(opts, huh.NewOption(name, name))
	}

	// Build mount options group based on whether disk is already formatted
	var mountOptionsGroup *huh.Group
	if formatted {
		mountOptionsGroup = huh.NewGroup(
			huh.NewInput().
				Title("Mount Point").
				Description("Directory path where the disk will be mounted").
				Value(&f.mountPoint).
				Validate(func(s string) error {
					if !strings.HasPrefix(s, "/") {
						return fmt.Errorf("must be an absolute path starting with /")
					}
					return nil
				}),
			huh.NewInput().
				Title("Owner").
				Description("User who will own the mount point").
				Value(&f.owner),
		)
	} else {
		mountOptionsGroup = huh.NewGroup(
			huh.NewInput().
				Title("Mount Point").
				Description("Directory path where the disk will be mounted").
				Value(&f.mountPoint).
				Validate(func(s string) error {
					if !strings.HasPrefix(s, "/") {
						return fmt.Errorf("must be an absolute path starting with /")
					}
					return nil
				}),
			huh.NewConfirm().
				Title("Format disk?").
				Description("WARNING: erases all data. Only needed for new/blank disks.").
				Value(&f.formatDisk),
			huh.NewSelect[string]().
				Title("Filesystem").
				Options(
					huh.NewOption("ext4 (recommended)", "ext4"),
					huh.NewOption("xfs", "xfs"),
				).
				Value(&f.fsType),
			huh.NewInput().
				Title("Owner").
				Description("User who will own the mount point").
				Value(&f.owner),
		)
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Attach Disk: "+diskCfg.Name).
				Description(fmt.Sprintf("Zone: %s • Size: %s GB", diskCfg.Zone, diskCfg.SizeGB)),
			huh.NewSelect[string]().
				Title("Instance").
				Description("Select a running VM to attach this disk to").
				Options(opts...).
				Value(&f.instanceName),
			huh.NewSelect[string]().
				Title("Mode").
				Description("Read/Write is exclusive to one VM. Read-Only allows multiple VMs to share the disk.").
				Options(
					huh.NewOption("Read/Write (single VM)", "rw"),
					huh.NewOption("Read-Only (shareable across VMs)", "ro"),
				).
				Value(&f.mode),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Mount after attaching?").
				Description("Mount the disk inside the VM so it's ready to use").
				Value(&f.mount),
		),
		mountOptionsGroup,
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

func (m diskAttachModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m diskAttachModel) Update(msg tea.Msg) (diskAttachModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return diskAttachCancelMsg{} }
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		if m.fields.instanceName == "" {
			return m, func() tea.Msg { return diskAttachCancelMsg{} }
		}
		f := m.fields
		projectID := m.vmProjectIDs[f.instanceName]

		// Resolve disk config and mount point from selection (VM-side attach flow)
		diskCfg := m.diskCfg
		if m.selectedDisk != nil && *m.selectedDisk != "" {
			if cfg, ok := m.diskCfgs[*m.selectedDisk]; ok {
				diskCfg = cfg
			}
			// Correct mount point if user left the default pattern
			// (huh.Input caches its buffer, so the displayed default may not match the selected disk)
			if strings.HasPrefix(f.mountPoint, "/mnt/disks/") {
				f.mountPoint = fmt.Sprintf("/mnt/disks/%s", *m.selectedDisk)
			}
		}

		return m, func() tea.Msg {
			return diskAttachDoneMsg{
				diskCfg:           diskCfg,
				instanceName:      f.instanceName,
				instanceProjectID: projectID,
				mode:              f.mode,
				mount:             f.mount,
				mountPoint:        f.mountPoint,
				formatDisk:        f.formatDisk,
				fsType:            f.fsType,
				owner:             f.owner,
				sourceTab:         f.sourceTab,
			}
		}
	}

	return m, cmd
}

// newDiskAttachFromVMModel creates an attach form starting from a VM.
// Group 0: disk + mode (hideable via skipDiskMode for back-nav)
// Group 1: mount confirm
func newDiskAttachFromVMModel(vmCfg config.Config, diskNames []string, diskCfgs map[string]config.DiskConfig) diskAttachModel {
	f := &diskAttachFields{
		instanceName: vmCfg.VMName,
		mode:         "rw",
		mount:        true,
		fsType:       "ext4",
		owner:        vmCfg.Username,
		sourceTab:    tabInstances,
	}

	selectedDisk := new(string)

	var diskOpts []huh.Option[string]
	for _, name := range diskNames {
		cfg := diskCfgs[name]
		label := fmt.Sprintf("%s (%s GB, %s)", name, cfg.SizeGB, cfg.DiskType)
		diskOpts = append(diskOpts, huh.NewOption(label, name))
	}

	vmProjectIDs := map[string]string{vmCfg.VMName: vmCfg.ProjectID}

	m := diskAttachModel{
		fields:       f,
		vmProjectIDs: vmProjectIDs,
		diskCfgs:     diskCfgs,
		diskNames:    diskNames,
		selectedDisk: selectedDisk,
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("Attach Disk to: %s", vmCfg.VMName)).
				Description(fmt.Sprintf("Project: %s • Zone: %s", vmCfg.ProjectID, vmCfg.Zone)),
			huh.NewSelect[string]().
				Title("Disk").
				Description("Select a managed data disk to attach").
				Options(diskOpts...).
				Value(selectedDisk),
			huh.NewSelect[string]().
				Title("Mode").
				Description("Read/Write is exclusive to one VM. Read-Only allows multiple VMs to share the disk.").
				Options(
					huh.NewOption("Read/Write (single VM)", "rw"),
					huh.NewOption("Read-Only (shareable across VMs)", "ro"),
				).
				Value(&f.mode),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Mount after attaching?").
				Description("Selecting Yes will configure mount options. Selecting No will attach the disk without mounting.").
				Affirmative("Yes, configure mount").
				Negative("No, attach only").
				Value(&f.mount),
		),
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

// rebuildForm recreates the huh form preserving current field values.
func (m *diskAttachModel) rebuildForm() {
	f := m.fields
	m.selectedDisk = new(string)
	if m.lastAttachDone.diskCfg.Name != "" {
		*m.selectedDisk = m.lastAttachDone.diskCfg.Name
	}

	var diskOpts []huh.Option[string]
	for _, name := range m.diskNames {
		cfg := m.diskCfgs[name]
		label := fmt.Sprintf("%s (%s GB, %s)", name, cfg.SizeGB, cfg.DiskType)
		diskOpts = append(diskOpts, huh.NewOption(label, name))
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("Attach Disk to: %s", f.instanceName)).
				Description(fmt.Sprintf("Project: %s", m.vmProjectIDs[f.instanceName])),
			huh.NewSelect[string]().
				Title("Disk").
				Description("Select a managed data disk to attach").
				Options(diskOpts...).
				Value(m.selectedDisk),
			huh.NewSelect[string]().
				Title("Mode").
				Description("Read/Write is exclusive to one VM. Read-Only allows multiple VMs to share the disk.").
				Options(
					huh.NewOption("Read/Write (single VM)", "rw"),
					huh.NewOption("Read-Only (shareable across VMs)", "ro"),
				).
				Value(&f.mode),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Mount after attaching?").
				Description("Selecting Yes will configure mount options. Selecting No will attach the disk without mounting.").
				Affirmative("Yes, configure mount").
				Negative("No, attach only").
				Value(&f.mount),
		),
	).WithShowHelp(true).WithShowErrors(true)
}

func (m diskAttachModel) View() string {
	return titleStyle.Render("Attach Disk to VM") + "\n\n" + m.form.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Mount Options Form (second step after attach form) ---

type diskMountOptionsModel struct {
	form   *huh.Form
	fields *diskAttachFields
}

type diskMountOptionsDoneMsg struct {
	fields *diskAttachFields
}

type diskMountOptionsCancelMsg struct{}
type diskMountOptionsBackMsg struct{}

func newDiskMountOptionsModel(fields *diskAttachFields, formatted bool) diskMountOptionsModel {
	m := diskMountOptionsModel{fields: fields}

	var groups []*huh.Group
	if formatted || fields.mode == "ro" {
		groups = append(groups, huh.NewGroup(
			huh.NewInput().
				Title("Mount Point").
				Description("Directory path where the disk will be mounted").
				Value(&fields.mountPoint).
				Validate(func(s string) error {
					if !strings.HasPrefix(s, "/") {
						return fmt.Errorf("must be an absolute path starting with /")
					}
					return nil
				}),
			huh.NewInput().
				Title("Owner").
				Description("User who will own the mount point").
				Value(&fields.owner),
		))
	} else {
		groups = append(groups, huh.NewGroup(
			huh.NewInput().
				Title("Mount Point").
				Description("Directory path where the disk will be mounted").
				Value(&fields.mountPoint).
				Validate(func(s string) error {
					if !strings.HasPrefix(s, "/") {
						return fmt.Errorf("must be an absolute path starting with /")
					}
					return nil
				}),
			huh.NewConfirm().
				Title("Format disk?").
				Description("WARNING: erases all data. Only needed for new/blank disks.").
				Value(&fields.formatDisk),
			huh.NewSelect[string]().
				Title("Filesystem").
				Options(
					huh.NewOption("ext4 (recommended)", "ext4"),
					huh.NewOption("xfs", "xfs"),
				).
				Value(&fields.fsType),
			huh.NewInput().
				Title("Owner").
				Description("User who will own the mount point").
				Value(&fields.owner),
		))
	}

	m.form = huh.NewForm(groups...).WithShowHelp(true).WithShowErrors(true)
	return m
}

func (m diskMountOptionsModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m diskMountOptionsModel) Update(msg tea.Msg) (diskMountOptionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "shift+tab" {
			return m, func() tea.Msg { return diskMountOptionsBackMsg{} }
		}
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return diskMountOptionsCancelMsg{} }
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		return m, func() tea.Msg {
			return diskMountOptionsDoneMsg{fields: m.fields}
		}
	}

	return m, cmd
}

func (m diskMountOptionsModel) View() string {
	return titleStyle.Render("Attach Disk to VM") + "\n\n" + m.form.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Detach Disk from VM Form ---

type diskDetachFromVMModel struct {
	form     *huh.Form
	vmCfg    config.Config
	diskCfgs []config.DiskConfig
	selected *string // selected disk name or "__all__"
}

type diskDetachFromVMDoneMsg struct {
	vmCfg    config.Config
	diskCfgs []config.DiskConfig // disks to detach
}

type diskDetachFromVMCancelMsg struct{}

func newDiskDetachFromVMModel(vmCfg config.Config, diskCfgs []config.DiskConfig) diskDetachFromVMModel {
	selected := new(string)

	var opts []huh.Option[string]
	if len(diskCfgs) > 1 {
		opts = append(opts, huh.NewOption(fmt.Sprintf("All (%d disks)", len(diskCfgs)), "__all__"))
	}
	for _, cfg := range diskCfgs {
		label := fmt.Sprintf("%s (%s GB, %s)", cfg.Name, cfg.SizeGB, cfg.DiskType)
		opts = append(opts, huh.NewOption(label, cfg.Name))
	}

	m := diskDetachFromVMModel{
		vmCfg:    vmCfg,
		diskCfgs: diskCfgs,
		selected: selected,
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("Detach Disk from: %s", vmCfg.VMName)).
				Description(fmt.Sprintf("Project: %s • Zone: %s", vmCfg.ProjectID, vmCfg.Zone)),
			huh.NewSelect[string]().
				Title("Disk to detach").
				Options(opts...).
				Value(selected),
		),
	).WithShowHelp(true).WithShowErrors(true)

	return m
}

func (m diskDetachFromVMModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m diskDetachFromVMModel) Update(msg tea.Msg) (diskDetachFromVMModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			return m, func() tea.Msg { return diskDetachFromVMCancelMsg{} }
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		var toDetach []config.DiskConfig
		if *m.selected == "__all__" {
			toDetach = m.diskCfgs
		} else {
			for _, cfg := range m.diskCfgs {
				if cfg.Name == *m.selected {
					toDetach = append(toDetach, cfg)
					break
				}
			}
		}
		return m, func() tea.Msg {
			return diskDetachFromVMDoneMsg{vmCfg: m.vmCfg, diskCfgs: toDetach}
		}
	}

	return m, cmd
}

func (m diskDetachFromVMModel) View() string {
	return titleStyle.Render("Detach Disk from VM") + "\n\n" + m.form.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

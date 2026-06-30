package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"vmup/internal/config"
)

type settingsPhase int

const (
	settingsPhaseForm    settingsPhase = iota // edit path
	settingsPhaseSummary                      // show counts + action keys
	settingsPhaseReview                       // detailed listing
	settingsPhaseConfirm                      // y/n before executing
	settingsPhaseSuccess                      // done message
)

type settingsAction int

const (
	settingsActionSwitch  settingsAction = iota // switch only
	settingsActionMigrate                       // migrate & switch
)

type settingsModel struct {
	phase        settingsPhase
	form         *huh.Form
	dataDir      *string
	imageProject *string // optional GCP project to list custom images from
	original     string  // canonicalized value when screen opened
	err          string
	width        int

	// Summary/review/confirm phase
	pendingDir              string
	pendingAction           settingsAction
	summaryForm             *huh.Form
	summaryChoice           *string
	srcProjects, srcDisks   int
	dstProjects, dstDisks   int
	srcProjectList          []config.ProjectSummary
	srcDiskList             []config.DiskSummary
	dstProjectList          []config.ProjectSummary
	dstDiskList             []config.DiskSummary
	reviewLoaded            bool
	reviewScrollOffset      int

	// Confirm phase
	confirmForm  *huh.Form
	confirmValue *bool

	// Success phase
	successMsg string
}

type settingsDoneMsg struct{}
type settingsCancelMsg struct{}

func newSettingsModel(width int) settingsModel {
	current := config.DataDir()
	dir := current
	imageProject := config.LoadSettings().EffectiveImageProject()

	m := settingsModel{
		phase:        settingsPhaseForm,
		dataDir:      &dir,
		imageProject: &imageProject,
		original:     current,
		width:        width,
	}

	m.form = rebuildSettingsForm(m.dataDir, m.imageProject, width)
	return m
}

func (m settingsModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m settingsModel) Update(msg tea.Msg) (settingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			switch m.phase {
			case settingsPhaseSummary:
				m.phase = settingsPhaseForm
				m.err = ""
				m.form = rebuildSettingsForm(m.dataDir, m.imageProject, m.width)
				return m, m.form.Init()
			case settingsPhaseReview:
				m.summaryForm, m.summaryChoice = buildSummaryForm(m.width)
				m.phase = settingsPhaseSummary
				return m, m.summaryForm.Init()
			case settingsPhaseConfirm:
				m.summaryForm, m.summaryChoice = buildSummaryForm(m.width)
				m.phase = settingsPhaseSummary
				return m, m.summaryForm.Init()
			case settingsPhaseSuccess:
				return m, func() tea.Msg { return settingsDoneMsg{} }
			default:
				return m, func() tea.Msg { return settingsCancelMsg{} }
			}
		}
	}

	switch m.phase {
	case settingsPhaseForm:
		return m.updateForm(msg)
	case settingsPhaseSummary:
		return m.updateSummary(msg)
	case settingsPhaseReview:
		return m.updateReview(msg)
	case settingsPhaseConfirm:
		return m.updateConfirm(msg)
	case settingsPhaseSuccess:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == "enter" {
				return m, func() tea.Msg { return settingsDoneMsg{} }
			}
		}
		return m, nil
	}

	return m, nil
}

func (m settingsModel) updateForm(msg tea.Msg) (settingsModel, tea.Cmd) {
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		m.err = ""

		// Persist the image-project setting independently of the data-directory
		// migration flow below. Storing a pointer (even to "") marks it as
		// explicitly configured so it no longer falls back to the shipped default.
		ip := strings.TrimSpace(*m.imageProject)
		s := config.LoadSettings()
		s.ImageProject = &ip
		_ = config.SaveSettings(s)

		newDir := strings.TrimSpace(*m.dataDir)
		if newDir == "" {
			newDir = config.BaseDir()
		}
		newDir = config.CanonicalPath(newDir)

		// No change — just go back
		if newDir == m.original {
			return m, func() tea.Msg { return settingsDoneMsg{} }
		}

		// Basic path validation (nested paths, accessibility)
		oldWithSep := m.original + string('/')
		newWithSep := newDir + string('/')
		if strings.HasPrefix(newWithSep, oldWithSep) {
			m.err = "New path cannot be inside the current data directory"
			m.form = rebuildSettingsForm(m.dataDir, m.imageProject, m.width)
			return m, m.form.Init()
		}
		if strings.HasPrefix(oldWithSep, newWithSep) {
			m.err = "New path cannot be a parent of the current data directory"
			m.form = rebuildSettingsForm(m.dataDir, m.imageProject, m.width)
			return m, m.form.Init()
		}

		// Gather counts and transition to summary
		m.pendingDir = newDir
		m.srcProjects, m.srcDisks = config.CountDataItems(m.original)
		m.dstProjects, m.dstDisks = config.CountDataItems(newDir)
		m.reviewLoaded = false
		m.reviewScrollOffset = 0
		m.summaryForm, m.summaryChoice = buildSummaryForm(m.width)
		m.phase = settingsPhaseSummary
		return m, m.summaryForm.Init()
	}

	return m, cmd
}

func (m settingsModel) updateSummary(msg tea.Msg) (settingsModel, tea.Cmd) {
	// Intercept 'r' before the form gets it
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "r" {
		if !m.reviewLoaded {
			m.srcProjectList = config.LoadProjectSummaries(m.original)
			m.srcDiskList = config.LoadDiskSummaries(m.original)
			m.dstProjectList = config.LoadProjectSummaries(m.pendingDir)
			m.dstDiskList = config.LoadDiskSummaries(m.pendingDir)
			m.reviewLoaded = true
		}
		m.reviewScrollOffset = 0
		m.phase = settingsPhaseReview
		return m, nil
	}

	form, cmd := m.summaryForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.summaryForm = f
	}

	if m.summaryForm.State == huh.StateCompleted {
		choice := *m.summaryChoice
		switch choice {
		case "migrate":
			m.pendingAction = settingsActionMigrate
			m.confirmValue = new(bool)
			prompt := fmt.Sprintf(
				"This will move %d project(s) and %d disk(s) from %s to %s. Continue?",
				m.srcProjects, m.srcDisks, m.original, m.pendingDir,
			)
			cf := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().Title(prompt).Value(m.confirmValue),
			))
			if m.width > 0 {
				cf = cf.WithWidth(m.width)
			}
			m.confirmForm = cf
			m.phase = settingsPhaseConfirm
			return m, m.confirmForm.Init()

		case "switch":
			m.pendingAction = settingsActionSwitch
			m.confirmValue = new(bool)
			prompt := fmt.Sprintf(
				"This will switch your data directory from %s to %s without moving any projects or disks. Continue?",
				m.original, m.pendingDir,
			)
			cf := huh.NewForm(huh.NewGroup(
				huh.NewConfirm().Title(prompt).Value(m.confirmValue),
			))
			if m.width > 0 {
				cf = cf.WithWidth(m.width)
			}
			m.confirmForm = cf
			m.phase = settingsPhaseConfirm
			return m, m.confirmForm.Init()
		}
	}

	return m, cmd
}

func (m settingsModel) updateReview(msg tea.Msg) (settingsModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "r":
		m.summaryForm, m.summaryChoice = buildSummaryForm(m.width)
		m.phase = settingsPhaseSummary
		return m, m.summaryForm.Init()
	case "j", "down":
		m.reviewScrollOffset++
	case "k", "up":
		if m.reviewScrollOffset > 0 {
			m.reviewScrollOffset--
		}
	}

	return m, nil
}

func (m settingsModel) updateConfirm(msg tea.Msg) (settingsModel, tea.Cmd) {
	form, cmd := m.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.confirmForm = f
	}

	if m.confirmForm.State == huh.StateCompleted {
		if !*m.confirmValue {
			m.summaryForm, m.summaryChoice = buildSummaryForm(m.width)
			m.phase = settingsPhaseSummary
			return m, m.summaryForm.Init()
		}

		switch m.pendingAction {
		case settingsActionSwitch:
			return m.saveSettings(m.pendingDir, "Switched data directory to "+m.pendingDir)
		case settingsActionMigrate:
			if err := config.ValidateDataDir(m.original, m.pendingDir); err != nil {
				m.err = err.Error()
				m.summaryForm, m.summaryChoice = buildSummaryForm(m.width)
				m.phase = settingsPhaseSummary
				return m, m.summaryForm.Init()
			}
			if err := config.MigrateData(m.original, m.pendingDir); err != nil {
				m.err = "Migration failed: " + err.Error()
				m.summaryForm, m.summaryChoice = buildSummaryForm(m.width)
				m.phase = settingsPhaseSummary
				return m, m.summaryForm.Init()
			}
			return m.saveSettings(m.pendingDir, "Migrated and switched data directory to "+m.pendingDir)
		}
	}

	return m, cmd
}

func (m settingsModel) saveSettings(dir, successMsg string) (settingsModel, tea.Cmd) {
	s := config.LoadSettings()
	if dir == config.BaseDir() {
		s.DataDir = ""
	} else {
		s.DataDir = dir
	}
	if err := config.SaveSettings(s); err != nil {
		m.err = "Save failed: " + err.Error()
		return m, nil
	}

	m.successMsg = successMsg
	m.phase = settingsPhaseSuccess
	return m, nil
}

// --- Views ---

func (m settingsModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Settings"))
	b.WriteString("\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render("Error: "+m.err) + "\n\n")
	}

	switch m.phase {
	case settingsPhaseForm:
		b.WriteString(m.form.View())
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("esc cancel"))
	case settingsPhaseSummary:
		b.WriteString(m.viewSummary())
	case settingsPhaseReview:
		b.WriteString(m.viewReview())
	case settingsPhaseConfirm:
		b.WriteString(m.viewConfirm())
	case settingsPhaseSuccess:
		b.WriteString(successStyle.Render(m.successMsg))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("press enter to continue"))
	}

	return b.String()
}

func (m settingsModel) viewSummary() string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	var b strings.Builder

	b.WriteString(labelStyle.Render("Source") + "  " + pathStyle.Render(m.original) + "\n")
	b.WriteString(countStyle.Render(fmt.Sprintf("  %d project(s), %d disk(s)", m.srcProjects, m.srcDisks)) + "\n")
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Destination") + "  " + pathStyle.Render(m.pendingDir) + "\n")
	b.WriteString(countStyle.Render(fmt.Sprintf("  %d project(s), %d disk(s)", m.dstProjects, m.dstDisks)) + "\n")
	b.WriteString("\n")
	b.WriteString(m.summaryForm.View())
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("r review • esc cancel"))

	return b.String()
}

func (m settingsModel) viewConfirm() string {
	return m.confirmForm.View() + "\n" + dimStyle.Render("esc cancel")
}

func (m settingsModel) viewReview() string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Width(20)
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Width(18)

	var lines []string

	// Source section
	lines = append(lines, labelStyle.Render("Source")+"  "+pathStyle.Render(m.original))
	lines = append(lines, "")

	if len(m.srcProjectList) > 0 {
		lines = append(lines, headerStyle.Render("  Projects"))
		for _, p := range m.srcProjectList {
			lines = append(lines, "  "+nameStyle.Render(p.Name)+valStyle.Render(p.Zone)+valStyle.Render(p.MachineType)+valStyle.Render(p.Image))
		}
	} else {
		lines = append(lines, headerStyle.Render("  Projects")+"  "+dimStyle.Render("(none)"))
	}
	lines = append(lines, "")

	if len(m.srcDiskList) > 0 {
		lines = append(lines, headerStyle.Render("  Disks"))
		for _, d := range m.srcDiskList {
			lines = append(lines, "  "+nameStyle.Render(d.Name)+valStyle.Render(d.Zone)+valStyle.Render(d.DiskType)+valStyle.Render(d.SizeGB+" GB"))
		}
	} else {
		lines = append(lines, headerStyle.Render("  Disks")+"  "+dimStyle.Render("(none)"))
	}
	lines = append(lines, "")

	// Destination section
	lines = append(lines, labelStyle.Render("Destination")+"  "+pathStyle.Render(m.pendingDir))
	lines = append(lines, "")

	if len(m.dstProjectList) > 0 {
		lines = append(lines, headerStyle.Render("  Projects"))
		for _, p := range m.dstProjectList {
			lines = append(lines, "  "+nameStyle.Render(p.Name)+valStyle.Render(p.Zone)+valStyle.Render(p.MachineType)+valStyle.Render(p.Image))
		}
	} else {
		lines = append(lines, headerStyle.Render("  Projects")+"  "+dimStyle.Render("(none)"))
	}
	lines = append(lines, "")

	if len(m.dstDiskList) > 0 {
		lines = append(lines, headerStyle.Render("  Disks"))
		for _, d := range m.dstDiskList {
			lines = append(lines, "  "+nameStyle.Render(d.Name)+valStyle.Render(d.Zone)+valStyle.Render(d.DiskType)+valStyle.Render(d.SizeGB+" GB"))
		}
	} else {
		lines = append(lines, headerStyle.Render("  Disks")+"  "+dimStyle.Render("(none)"))
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("↑/↓ scroll • r/esc back"))

	// Apply scroll offset
	if m.reviewScrollOffset > 0 && m.reviewScrollOffset < len(lines) {
		lines = lines[m.reviewScrollOffset:]
	}

	return strings.Join(lines, "\n")
}

func buildSummaryForm(width int) (*huh.Form, *string) {
	choice := "migrate"
	f := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Action").
			Options(
				huh.NewOption("Migrate & switch", "migrate"),
				huh.NewOption("Switch only", "switch"),
			).
			Value(&choice),
	))
	if width > 0 {
		f = f.WithWidth(width)
	}
	return f, &choice
}

func rebuildSettingsForm(dataDir, imageProject *string, width int) *huh.Form {
	f := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Data Directory").
				Description("Where VM projects and disks are stored").
				Value(dataDir),
			huh.NewInput().
				Title("Image Project (optional)").
				Description("GCP project whose images are listed first when creating a VM; leave blank to show only standard public images").
				Value(imageProject),
		),
	).WithShowHelp(true).WithShowErrors(true)
	if width > 0 {
		f = f.WithWidth(width)
	}
	return f
}

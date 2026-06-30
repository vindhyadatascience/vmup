package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/vindhyadatascience/vmup/internal/config"
	"github.com/vindhyadatascience/vmup/internal/gcloud"
	tf "github.com/vindhyadatascience/vmup/internal/terraform"
	"github.com/vindhyadatascience/vmup/internal/tunnel"
)

type App struct {
	screen    screen
	activeTab tab
	config    configModel
	progress  progressModel
	status    statusModel
	vmlist    vmListModel
	disklist  diskListModel

	// State
	activeConfig          config.Config
	tunnelMgr             *tunnel.Manager
	embeddedMainTF        string
	embeddedDiskTF        string
	embeddedDiskDeletable string
	program               *tea.Program

	// Background operation tracking
	bgRunning   bool
	bgTitle     string
	bgSourceTab tab

	// Edit mode
	editMode bool

	// For confirmation prompts
	confirmForm      *huh.Form
	confirmValue     *bool
	confirmNameValue *string

	// Disk operation state
	diskForm    diskFormModel
	diskImport  diskImportModel
	diskResize      diskResizeModel
	diskAttach      diskAttachModel
	diskMountOpts   diskMountOptionsModel
	diskDetachVM    diskDetachFromVMModel
	activeDisk      diskEntry
	lastAttachOpts  diskAttachDoneMsg
	detachInstance  *string

	// Settings
	settings settingsModel

	// Command palette
	cmdPalette cmdPaletteModel

	// Animated gradient (shared across tabs)
	gradientOffset int

	width, height int
}

func NewApp(embeddedMainTF, embeddedDiskTF, embeddedDiskDeletable string) *App {
	tm := tunnel.NewManager()
	return &App{
		screen:                screenMain,
		activeTab:             tabInstances,
		vmlist:                newVMListModel(tm),
		disklist:              newDiskListModel(),
		tunnelMgr:             tm,
		embeddedMainTF:        embeddedMainTF,
		embeddedDiskTF:        embeddedDiskTF,
		embeddedDiskDeletable: embeddedDiskDeletable,
	}
}

func (a *App) SetProgram(p *tea.Program) {
	a.program = p
}

func (a App) Init() tea.Cmd {
	return tea.Batch(a.vmlist.Init(), a.disklist.Init(), logoTick())
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Ctrl+D force quit — always works, regardless of screen or filter state
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "ctrl+d" {
		return a, tea.Quit
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		if a.cmdPalette.active {
			a.cmdPalette.width = msg.Width
		}
		// Propagate to both tab models so they have correct dimensions
		var vmCmd, diskCmd tea.Cmd
		a.vmlist, vmCmd = a.vmlist.Update(msg)
		a.disklist, diskCmd = a.disklist.Update(msg)
		return a, tea.Batch(vmCmd, diskCmd)
	case logoTickMsg:
		a.gradientOffset++
		return a, logoTick()
	case vmListLoadedMsg:
		// Always deliver to vmlist regardless of active tab/screen
		a.vmlist, _ = a.vmlist.Update(msg)
		if a.cmdPalette.active && a.activeTab == tabInstances {
			a.cmdPalette.Rebuild(vmPaletteCommands(a.vmlist.vms, a.vmlist.cursor, a.bgRunning, a.progress.done))
		}
		return a, nil
	case diskListLoadedMsg:
		// Always deliver to disklist regardless of active tab/screen
		a.disklist, _ = a.disklist.Update(msg)
		if a.cmdPalette.active && a.activeTab == tabDataDisks {
			a.cmdPalette.Rebuild(diskPaletteCommands(a.disklist.disks, a.disklist.cursor, a.bgRunning, a.progress.done))
		}
		return a, nil
	case resizeDoneMsg:
		// Always deliver resize debounce to vmlist
		a.vmlist, _ = a.vmlist.Update(msg)
		return a, nil
	case diskLayoutDoneMsg:
		// Always deliver resize debounce to disklist
		a.disklist, _ = a.disklist.Update(msg)
		return a, nil
	case cmdPaletteExecMsg:
		paletteInput := msg.input
		a.cmdPalette.Close()
		// Call action synchronously to check if it's a filter with args
		result := msg.action()
		if _, ok := result.(cmdPaletteFilterMsg); ok {
			// Extract args after the "filter" command name
			args := strings.TrimSpace(paletteInput)
			if strings.HasPrefix(args, "filter") {
				args = strings.TrimSpace(args[len("filter"):])
			} else {
				args = "" // partial match like "fil" — no args
			}
			return a, func() tea.Msg { return cmdPaletteFilterMsg{args: args} }
		}
		return a, func() tea.Msg { return result }
	case cmdPaletteFilterMsg:
		a.applyFilterArgs(msg.args)
		return a, nil
	case cmdPaletteSwitchTabMsg:
		if a.activeTab == tabInstances {
			a.activeTab = tabDataDisks
		} else {
			a.activeTab = tabInstances
		}
		return a, nil
	case cmdPaletteRefreshMsg:
		if msg.tab == tabInstances {
			a.vmlist.loading = true
		a.vmlist.refreshStart = time.Now()
			return a, tea.Batch(loadVMList, a.vmlist.spinner.Tick)
		}
		a.disklist.loading = true
		a.disklist.refreshStart = time.Now()
		return a, tea.Batch(loadDiskList, a.disklist.spinner.Tick)
	case cmdPaletteProgressMsg:
		if a.bgRunning {
			a.screen = screenProgress
			return a, a.progress.spinner.Tick
		}
		if a.progress.done {
			a.screen = screenProgress
			return a, nil
		}
		return a, nil
	case cmdPaletteSettingsMsg:
		a.settings = newSettingsModel(a.width)
		a.screen = screenSettings
		return a, a.settings.Init()
	}

	// Command palette — capture all keys when active
	if a.screen == screenMain && a.cmdPalette.active {
		if _, ok := msg.(tea.KeyMsg); ok {
			var cmd tea.Cmd
			a.cmdPalette, cmd = a.cmdPalette.Update(msg)
			return a, cmd
		}
	}

	// Tab switching — only on main screen
	if a.screen == screenMain {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			// When filter input is active, skip all app-level key interception
			isFilterActive := (a.activeTab == tabInstances && a.vmlist.filterActive) ||
				(a.activeTab == tabDataDisks && a.disklist.filterActive)
			if !isFilterActive {
				switch keyMsg.String() {
				case ":":
					a.vmlist.showHelp = false
					a.disklist.showHelp = false
					var commands []paletteCommand
					switch a.activeTab {
					case tabInstances:
						commands = vmPaletteCommands(a.vmlist.vms, a.vmlist.cursor, a.bgRunning, a.progress.done)
					case tabDataDisks:
						commands = diskPaletteCommands(a.disklist.disks, a.disklist.cursor, a.bgRunning, a.progress.done)
					}
					a.cmdPalette.Open(commands, a.width)
					return a, nil
				case "/":
					a.vmlist.showHelp = false
					a.disklist.showHelp = false
					switch a.activeTab {
					case tabInstances:
						a.vmlist.filterActive = true
						a.vmlist.savedFilterProp = a.vmlist.filterProp
						a.vmlist.savedFilterValue = a.vmlist.filterValue
						if a.vmlist.hasFilter() {
							a.vmlist.filterInput = strings.TrimSpace(a.vmlist.filterProp + " " + a.vmlist.filterValue)
						} else {
							a.vmlist.filterInput = ""
						}
						// Suspend filter to show full list while editing
						a.vmlist.filteredIndices = nil
						a.vmlist.cursor = 0
						a.vmlist.scrollTop = 0
						a.vmlist.adjustScroll()
					case tabDataDisks:
						a.disklist.filterActive = true
						a.disklist.savedFilterProp = a.disklist.filterProp
						a.disklist.savedFilterValue = a.disklist.filterValue
						if a.disklist.hasFilter() {
							a.disklist.filterInput = strings.TrimSpace(a.disklist.filterProp + " " + a.disklist.filterValue)
						} else {
							a.disklist.filterInput = ""
						}
						// Suspend filter to show full list while editing
						a.disklist.filteredIndices = nil
						a.disklist.cursor = 0
						a.disklist.scrollTop = 0
						a.disklist.adjustScroll()
					}
					return a, nil
				case "tab", "right", "l":
					if a.activeTab < tabDataDisks {
						a.activeTab++
					}
					return a, nil
				case "shift+tab", "left", "h":
					if a.activeTab > tabInstances {
						a.activeTab--
					}
					return a, nil
				case "1":
					a.activeTab = tabInstances
					return a, nil
				case "2":
					a.activeTab = tabDataDisks
					return a, nil
				}

				// Intercept ',' key for settings screen (blocked during background ops)
				if keyMsg.String() == "," && !a.bgRunning {
					a.settings = newSettingsModel(a.width)
					a.screen = screenSettings
					return a, a.settings.Init()
				}

				// Intercept 'p' key for progress viewing regardless of active tab
				if keyMsg.String() == "p" {
					if a.bgRunning {
						a.screen = screenProgress
						return a, a.progress.spinner.Tick
					}
					if a.progress.done {
						a.screen = screenProgress
						return a, nil
					}
				}
			}
		}
	}

	// Always clear bgRunning when an operation finishes.
	if _, ok := msg.(progressDoneMsg); ok {
		a.bgRunning = false
	}

	// When a background op is running and we are NOT on the progress screen,
	// keep the progress model updated so it stays current if the user returns.
	var bgCmd tea.Cmd
	if a.screen != screenProgress {
		switch msg.(type) {
		case logLineMsg:
			a.progress, _ = a.progress.Update(msg)
			return a, nil

		case progressDoneMsg:
			doneMsg := msg.(progressDoneMsg)
			a.progress, _ = a.progress.Update(msg)
			if a.screen == screenMain {
				if a.bgSourceTab == tabInstances {
					if doneMsg.err != nil {
						a.vmlist.flashMsg = fmt.Sprintf("%s failed: %v", a.bgTitle, doneMsg.err)
						a.vmlist.flashIsError = true
					} else {
						a.vmlist.flashMsg = fmt.Sprintf("%s complete!", a.bgTitle)
						a.vmlist.flashIsError = false
					}
					if a.progress.title == "Destroying VM..." && doneMsg.err == nil {
						a.activeConfig = config.Config{}
					}
					// Refresh VM list
					a.vmlist.loading = true
		a.vmlist.refreshStart = time.Now()
					return a, tea.Batch(loadVMList, a.vmlist.spinner.Tick)
				} else if a.bgSourceTab == tabDataDisks {
					if doneMsg.err != nil {
						a.disklist.flashMsg = fmt.Sprintf("%s failed: %v", a.bgTitle, doneMsg.err)
						a.disklist.flashIsError = true
					} else {
						a.disklist.flashMsg = fmt.Sprintf("%s complete!", a.bgTitle)
						a.disklist.flashIsError = false
					}
					// Refresh both lists (disk ops affect VM attachment display)
					a.disklist.loading = true
		a.disklist.refreshStart = time.Now()
					a.vmlist.loading = true
		a.vmlist.refreshStart = time.Now()
					return a, tea.Batch(loadDiskList, a.disklist.spinner.Tick, loadVMList, a.vmlist.spinner.Tick)
				}
			}
			return a, nil

		case spinner.TickMsg:
			if a.bgRunning {
				a.progress, bgCmd = a.progress.Update(msg)
			}
		}
	}

	// Handle auth needed: suspend TUI and run gcloud auth interactively
	if msg, ok := msg.(authNeededMsg); ok {
		a.progress.lines = append(a.progress.lines, infoStyle.Render("Authentication required. Launching gcloud auth..."))
		a.progress.viewport.SetContent(strings.Join(a.progress.lines, "\n"))
		a.progress.viewport.GotoBottom()
		var cmd *exec.Cmd
		if msg.kind == "login" {
			cmd = gcloud.AuthLoginCommand()
		} else {
			cmd = gcloud.ADCLoginCommand()
		}
		return a, tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return progressDoneMsg{err: fmt.Errorf("authentication failed: %w", err)}
			}
			return authRetryMsg{}
		})
	}

	// After successful auth, re-dispatch the pending operation
	if _, ok := msg.(authRetryMsg); ok {
		switch {
		case strings.Contains(a.bgTitle, "Launching") || strings.Contains(a.bgTitle, "Updating"):
			return a, a.cmdLaunchVM(a.activeConfig)
		case strings.Contains(a.bgTitle, "Destroying VM"):
			return a, a.cmdDestroyVM()
		case strings.Contains(a.bgTitle, "Creating data disk"):
			return a, a.cmdCreateDisk(a.activeDisk.cfg)
		case strings.Contains(a.bgTitle, "Importing disk"):
			return a, a.cmdImportDisk(a.activeDisk.cfg)
		case strings.Contains(a.bgTitle, "Deleting data disk"):
			return a, a.cmdDeleteDisk(a.activeDisk.cfg)
		case strings.Contains(a.bgTitle, "Resizing data disk"):
			return a, a.cmdResizeDisk(a.activeDisk.cfg)
		case strings.Contains(a.bgTitle, "Attaching"):
			return a, a.cmdAttachDisk(a.lastAttachOpts)
		case strings.Contains(a.bgTitle, "Detaching disk"):
			return a, a.cmdDetachDisk(a.activeDisk.cfg, a.activeDisk.status.Users[0])
		}
	}

	// Normal screen routing
	var model tea.Model
	var cmd tea.Cmd

	switch a.screen {
	case screenConfig:
		model, cmd = a.updateConfig(msg)
	case screenProgress:
		model, cmd = a.updateProgress(msg)
	case screenStatus:
		model, cmd = a.updateStatus(msg)
	case screenConfirmDestroy:
		model, cmd = a.updateConfirmDestroy(msg)
	case screenConfirmDestroyName:
		model, cmd = a.updateConfirmDestroyName(msg)
	case screenConfirmStopVM:
		model, cmd = a.updateConfirmStopVM(msg)
	case screenConfirmStopAll:
		model, cmd = a.updateConfirmStopAll(msg)
	case screenDiskCreate:
		model, cmd = a.updateDiskCreate(msg)
	case screenDiskImport:
		model, cmd = a.updateDiskImportScreen(msg)
	case screenDiskConfirmDelete:
		model, cmd = a.updateDiskConfirmDelete(msg)
	case screenDiskConfirmDeleteName:
		model, cmd = a.updateDiskConfirmDeleteName(msg)
	case screenDiskResize:
		model, cmd = a.updateDiskResizeScreen(msg)
	case screenDiskAttach:
		model, cmd = a.updateDiskAttachScreen(msg)
	case screenDiskAttachConfirm:
		model, cmd = a.updateDiskAttachConfirm(msg)
	case screenDiskMountOptions:
		model, cmd = a.updateDiskMountOptionsScreen(msg)
	case screenDiskDetach:
		model, cmd = a.updateDiskDetachScreen(msg)
	case screenDiskDetachFromVM:
		model, cmd = a.updateDiskDetachFromVMScreen(msg)
	case screenSettings:
		model, cmd = a.updateSettings(msg)
	case screenMain:
		// Forward spinner ticks to non-active tab if it's loading,
		// so its spinner keeps running and doesn't freeze.
		var crossTabCmd tea.Cmd
		if _, ok := msg.(spinner.TickMsg); ok {
			if a.activeTab != tabInstances && a.vmlist.loading {
				a.vmlist, crossTabCmd = a.vmlist.Update(msg)
			}
			if a.activeTab != tabDataDisks && a.disklist.loading {
				var dCmd tea.Cmd
				a.disklist, dCmd = a.disklist.Update(msg)
				crossTabCmd = tea.Batch(crossTabCmd, dCmd)
			}
		}

		switch a.activeTab {
		case tabInstances:
			model, cmd = a.updateVMList(msg)
		case tabDataDisks:
			model, cmd = a.updateDiskList(msg)
		default:
			model, cmd = a.updateVMList(msg)
		}

		if crossTabCmd != nil {
			cmd = tea.Batch(cmd, crossTabCmd)
		}
	default:
		model = a
	}

	if bgCmd != nil {
		return model, tea.Batch(cmd, bgCmd)
	}
	return model, cmd
}

func (a App) View() string {
	switch a.screen {
	case screenConfig:
		return a.config.View()
	case screenProgress:
		return a.progress.View()
	case screenStatus:
		return a.status.View()
	case screenConfirmDestroy:
		return a.viewConfirmDestroy()
	case screenConfirmDestroyName:
		return a.viewConfirmDestroyName()
	case screenConfirmStopVM:
		return a.viewConfirmStopVM()
	case screenConfirmStopAll:
		return a.viewConfirmStopAll()
	case screenDiskCreate:
		return a.diskForm.View()
	case screenDiskImport:
		return a.diskImport.View()
	case screenDiskConfirmDelete:
		return a.viewDiskConfirmDelete()
	case screenDiskConfirmDeleteName:
		return a.viewDiskConfirmDeleteName()
	case screenDiskResize:
		return a.diskResize.View()
	case screenDiskAttach:
		return a.diskAttach.View()
	case screenDiskAttachConfirm:
		return titleStyle.Render("Attach Disk to VM") + "\n\n" + a.confirmForm.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
	case screenDiskMountOptions:
		return a.diskMountOpts.View()
	case screenDiskDetach:
		return a.viewDiskDetach()
	case screenDiskDetachFromVM:
		return a.diskDetachVM.View()
	case screenSettings:
		return a.settings.View()
	case screenMain:
		a.vmlist.hideHelpBar = a.cmdPalette.active
		a.disklist.hideHelpBar = a.cmdPalette.active
		var b strings.Builder
		b.WriteString(renderTitle(a.gradientOffset))
		b.WriteString("\n\n")
		var lastRefreshed time.Time
		switch a.activeTab {
		case tabInstances:
			lastRefreshed = a.vmlist.lastRefreshed
		case tabDataDisks:
			lastRefreshed = a.disklist.lastRefreshed
		}
		b.WriteString(renderTabBar(a.activeTab, a.width, lastRefreshed))
		b.WriteString("\n\n")
		switch a.activeTab {
		case tabInstances:
			b.WriteString(a.vmlist.ViewContent())
		case tabDataDisks:
			b.WriteString(a.disklist.ViewContent())
		}
		if a.bgRunning {
			b.WriteString("\n" + infoStyle.Render(fmt.Sprintf("⟳ %s (p to view)", a.bgTitle)))
		}
		if a.cmdPalette.active {
			b.WriteString("\n")
			b.WriteString(a.cmdPalette.View())
		}
		return b.String()
	}
	return ""
}

func (a *App) applyFilterArgs(args string) {
	args = strings.TrimSpace(args)
	if args != "" {
		// Has args — apply filter directly
		var prop, value string
		if strings.HasPrefix(args, "\"") && strings.HasSuffix(args, "\"") {
			value = strings.Trim(args, "\"")
		} else {
			parts := strings.SplitN(args, " ", 2)
			if len(parts) == 2 {
				prop = strings.ToLower(parts[0])
				value = parts[1]
			} else {
				value = parts[0]
			}
		}
		switch a.activeTab {
		case tabInstances:
			a.vmlist.filterProp = prop
			a.vmlist.filterValue = value
			a.vmlist.recomputeFilter()
			a.vmlist.cursor = 0
			a.vmlist.scrollTop = 0
			a.vmlist.adjustScroll()
		case tabDataDisks:
			a.disklist.filterProp = prop
			a.disklist.filterValue = value
			a.disklist.recomputeFilter()
			a.disklist.cursor = 0
			a.disklist.scrollTop = 0
			a.disklist.adjustScroll()
		}
	} else {
		// No args — open filter input, suspend filter to show full list
		switch a.activeTab {
		case tabInstances:
			a.vmlist.filterActive = true
			a.vmlist.filterInput = ""
			a.vmlist.filteredIndices = nil
			a.vmlist.cursor = 0
			a.vmlist.scrollTop = 0
			a.vmlist.adjustScroll()
		case tabDataDisks:
			a.disklist.filterActive = true
			a.disklist.filterInput = ""
			a.disklist.filteredIndices = nil
			a.disklist.cursor = 0
			a.disklist.scrollTop = 0
			a.disklist.adjustScroll()
		}
	}
}

// --- Dispatch Action ---

func (a App) dispatchAction(action menuAction) (tea.Model, tea.Cmd) {
	switch action {
	case actionStartTunnels:
		a.bgRunning = true
		a.bgSourceTab = tabInstances
		a.bgTitle = "Starting instance and tunnels..."
		a.progress = newProgressModel("Starting instance and tunnels...")
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdStartTunnels())

	case actionStopTunnels:
		a.confirmValue = new(bool)
		a.confirmForm = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Also stop the VM to save costs?").
					Value(a.confirmValue),
			),
		)
		a.screen = screenConfirmStopVM
		return a, a.confirmForm.Init()

	case actionSSH:
		c := gcloud.SSHCommand(a.activeConfig)
		return a, tea.ExecProcess(c, func(err error) tea.Msg {
			return backToMenuMsg{}
		})

	case actionStopAll:
		a.confirmValue = new(bool)
		a.confirmForm = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Stop all tunnels and instances?").
					Description("This will stop tunnels and VMs across all projects.").
					Value(a.confirmValue),
			),
		)
		a.screen = screenConfirmStopAll
		return a, a.confirmForm.Init()

	case actionDestroy:
		a.confirmValue = new(bool)
		a.confirmForm = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Destroy VM '%s' and all resources?", a.activeConfig.VMName)).
					Description("This cannot be undone.").
					Value(a.confirmValue),
			),
		)
		a.screen = screenConfirmDestroy
		return a, a.confirmForm.Init()
	}
	return a, nil
}

// --- Config ---

func (a App) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.config, cmd = a.config.Update(msg)

	switch msg.(type) {
	case configDoneMsg:
		cfg := msg.(configDoneMsg).cfg
		a.activeConfig = cfg
		a.bgRunning = true
		a.bgSourceTab = tabInstances

		title := "Launching VM..."
		if a.editMode {
			title = "Updating VM..."
			a.tunnelMgr.StopAll(cfg.VMName)
			a.editMode = false
		}

		a.bgTitle = title
		a.progress = newProgressModel(title)
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdLaunchVM(cfg))

	case configCancelMsg:
		a.editMode = false
		a, cmd = a.refreshVMList()
		return a, cmd
	}

	return a, cmd
}

// --- Settings ---

func (a App) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.settings, cmd = a.settings.Update(msg)

	switch msg.(type) {
	case settingsDoneMsg:
		// Refresh both lists since data directory may have changed
		a.screen = screenMain
		a.vmlist.loading = true
		a.vmlist.refreshStart = time.Now()
		a.disklist.loading = true
		a.disklist.refreshStart = time.Now()
		return a, tea.Batch(loadVMList, a.vmlist.spinner.Tick, loadDiskList, a.disklist.spinner.Tick)
	case settingsCancelMsg:
		// Refresh lists in case files were moved externally
		a.screen = screenMain
		a.vmlist.loading = true
		a.vmlist.refreshStart = time.Now()
		a.disklist.loading = true
		a.disklist.refreshStart = time.Now()
		return a, tea.Batch(loadVMList, a.vmlist.spinner.Tick, loadDiskList, a.disklist.spinner.Tick)
	}

	return a, cmd
}

// --- Progress ---

func (a App) updateProgress(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.progress, cmd = a.progress.Update(msg)

	switch msg.(type) {
	case tea.KeyMsg:
		key := msg.(tea.KeyMsg).String()

		if key == "ctrl+c" {
			if a.progress.done {
				if a.progress.title == "Destroying VM..." && a.progress.err == nil {
					a.activeConfig = config.Config{}
				}
				return a.refreshSourceTab()
			}
			a.screen = screenMain
			return a, a.progress.spinner.Tick
		}

		// Allow esc to return to main screen while operation runs in background
		if key == "esc" && !a.progress.done {
			a.screen = screenMain
			return a, a.progress.spinner.Tick
		}

		if key == "enter" && a.progress.done {
			if a.progress.err != nil {
				return a.refreshSourceTab()
			}
			// Disk operations return to their source tab's list
			if a.bgSourceTab == tabDataDisks {
				return a.refreshDiskList()
			}
			if strings.Contains(a.bgTitle, "Attaching") ||
				strings.Contains(a.bgTitle, "Detaching") {
				return a.refreshSourceTab()
			}
			if a.progress.title == "Destroying VM..." {
				a.activeConfig = config.Config{}
				a, cmd = a.refreshVMList()
				return a, cmd
			}
			if a.progress.title == "Stopping all tunnels and instances..." ||
				a.progress.title == "Stopping tunnels and VM..." {
				a, cmd = a.refreshVMList()
				return a, cmd
			}
			// After successful launch or tunnel start, show status
			a.status = newStatusModel(a.activeConfig, a.tunnelMgr.ActivePIDsForVM(a.activeConfig.VMName), a.findVMAttachedDisks(a.activeConfig.VMName), "")
			a.screen = screenStatus
			return a, nil
		}
	}

	return a, cmd
}

// refreshSourceTab returns to the tab that started the background operation.
func (a App) refreshSourceTab() (App, tea.Cmd) {
	if a.bgSourceTab == tabDataDisks {
		return a.refreshDiskList()
	}
	return a.refreshVMList()
}

// --- Status ---

func (a App) updateStatus(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.status, cmd = a.status.Update(msg)

	switch msg.(type) {
	case backToMenuMsg:
		a, cmd = a.refreshVMList()
		return a, cmd
	}

	return a, cmd
}

// --- Confirm Destroy ---

func (a App) updateConfirmDestroy(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenMain
			return a, nil
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		if *a.confirmValue {
			// Transition to name confirmation screen
			a.confirmNameValue = new(string)
			vmName := a.activeConfig.VMName
			a.confirmForm = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title(fmt.Sprintf("Type '%s' to confirm destruction", vmName)).
						Description("This cannot be undone.").
						Value(a.confirmNameValue).
						Validate(func(s string) error {
							if s != vmName {
								return fmt.Errorf("name does not match")
							}
							return nil
						}),
				),
			)
			a.screen = screenConfirmDestroyName
			return a, a.confirmForm.Init()
		}
		a.screen = screenMain
		return a, nil
	}

	return a, cmd
}

func (a App) viewConfirmDestroy() string {
	return titleStyle.Render("Destroy VM") + "\n\n" + a.confirmForm.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Confirm Destroy Name ---

func (a App) updateConfirmDestroyName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenMain
			return a, nil
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		a.tunnelMgr.StopAll(a.activeConfig.VMName)
		a.bgRunning = true
		a.bgSourceTab = tabInstances
		a.bgTitle = "Destroying VM..."
		a.progress = newProgressModel("Destroying VM...")
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdDestroyVM())
	}

	return a, cmd
}

func (a App) viewConfirmDestroyName() string {
	return titleStyle.Render("Confirm Destroy") + "\n\n" + a.confirmForm.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Confirm Stop VM ---

func (a App) updateConfirmStopVM(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenMain
			return a, nil
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		a.tunnelMgr.StopAll(a.activeConfig.VMName)
		msg := "Tunnels stopped."

		if *a.confirmValue {
			a.bgRunning = true
			a.bgSourceTab = tabInstances
			a.bgTitle = "Stopping tunnels and VM..."
			a.progress = newProgressModel("Stopping tunnels and VM...")
			a.screen = screenProgress
			return a, tea.Batch(a.progress.Init(), a.cmdStopVM())
		}

		a.status = newStatusModel(a.activeConfig, nil, a.findVMAttachedDisks(a.activeConfig.VMName), msg)
		a.screen = screenStatus
		return a, nil
	}

	return a, cmd
}

func (a App) viewConfirmStopVM() string {
	return titleStyle.Render("Stop Tunnels") + "\n\n" + a.confirmForm.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Confirm Stop All ---

func (a App) updateConfirmStopAll(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenMain
			return a, nil
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		if *a.confirmValue {
			a.bgRunning = true
			a.bgSourceTab = tabInstances
			a.bgTitle = "Stopping all tunnels and instances..."
			a.progress = newProgressModel("Stopping all tunnels and instances...")
			a.screen = screenProgress
			return a, tea.Batch(a.progress.Init(), a.cmdStopAll())
		}
		a.screen = screenMain
		return a, nil
	}

	return a, cmd
}

func (a App) viewConfirmStopAll() string {
	return titleStyle.Render("Stop All") + "\n\n" + a.confirmForm.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// refreshVMList switches to the VM list screen and triggers a background
// reload while keeping the existing VM data and layout dimensions visible.
func (a App) refreshVMList() (App, tea.Cmd) {
	a.screen = screenMain
	a.activeTab = tabInstances
	a.vmlist.loading = true
		a.vmlist.refreshStart = time.Now()
	return a, tea.Batch(loadVMList, a.vmlist.spinner.Tick)
}

// --- VM List ---

func (a App) updateVMList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.vmlist, cmd = a.vmlist.Update(msg)

	switch msg := msg.(type) {
	case backToMenuMsg:
		a, cmd = a.refreshVMList()
		return a, cmd

	case vmListActionMsg:
		if a.bgRunning {
			a.vmlist.flashMsg = fmt.Sprintf("Please wait — %s (p to view progress)", a.bgTitle)
			a.vmlist.flashIsError = true
			return a, nil
		}
		if msg.action == actionLaunch {
			a.config = newConfigModel()
			a.screen = screenConfig
			return a, a.config.Init()
		}
		if msg.action == actionInfo {
			a.activeConfig = msg.cfg
			a.status = newStatusModel(msg.cfg, a.tunnelMgr.ActivePIDsForVM(msg.cfg.VMName), a.findVMAttachedDisks(msg.cfg.VMName), "")
			a.screen = screenStatus
			return a, nil
		}
		if msg.action == actionStopAll {
			return a.dispatchAction(actionStopAll)
		}
		if msg.action == actionAttachDiskToVM {
			a.activeConfig = msg.cfg
			a.vmlist.loading = true
		a.vmlist.refreshStart = time.Now()
			a.vmlist.loadingText = "Loading available disks..."
			return a, tea.Batch(a.vmlist.spinner.Tick, loadDisksForVM(msg.cfg))
		}
		if msg.action == actionEdit {
			a.editMode = true
			a.activeConfig = msg.cfg
			a.config = newEditConfigModel(msg.cfg)
			a.screen = screenConfig
			return a, a.config.Init()
		}
		a.activeConfig = msg.cfg
		return a.dispatchAction(msg.action)
	case vmAttachDisksReadyMsg:
		a.vmlist.loading = false
		a.vmlist.loadingText = ""
		if len(msg.diskNames) == 0 {
			a.vmlist.flashMsg = fmt.Sprintf("No managed disks found in project %s, zone %s", msg.vmCfg.ProjectID, msg.vmCfg.Zone)
			a.vmlist.flashIsError = true
			return a, nil
		}
		a.diskAttach = newDiskAttachFromVMModel(msg.vmCfg, msg.diskNames, msg.diskCfgs)
		a.screen = screenDiskAttach
		return a, a.diskAttach.Init()

	case vmDetachDiskMsg:
		a.diskDetachVM = newDiskDetachFromVMModel(msg.vmCfg, msg.diskCfgs)
		a.screen = screenDiskDetachFromVM
		return a, a.diskDetachVM.Init()

	case tea.KeyMsg:
		_ = msg // handled by vmlist model
	}

	return a, cmd
}

// --- Disk List ---

func (a App) updateDiskList(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.disklist, cmd = a.disklist.Update(msg)

	switch msg := msg.(type) {
	case diskListActionMsg:
		if a.bgRunning {
			a.disklist.flashMsg = fmt.Sprintf("Please wait — %s (p to view progress)", a.bgTitle)
			a.disklist.flashIsError = true
			return a, nil
		}
		a.activeDisk = msg.disk
		return a.dispatchDiskAction(msg.action, msg.disk)

	case diskAttachReadyMsg:
		a.disklist.loading = false
		a.disklist.loadingText = ""
		a.diskAttach = newDiskAttachModel(msg.disk.cfg, msg.vmNames, msg.vmUsernames, msg.vmProjectIDs, msg.formatted)
		a.screen = screenDiskAttach
		return a, a.diskAttach.Init()
	}

	return a, cmd
}

func (a App) dispatchDiskAction(action diskAction, disk diskEntry) (tea.Model, tea.Cmd) {
	switch action {
	case actionDiskCreate:
		a.diskForm = newDiskCreateModel()
		a.screen = screenDiskCreate
		return a, a.diskForm.Init()

	case actionDiskImport:
		a.diskImport = newDiskImportModel()
		a.screen = screenDiskImport
		return a, a.diskImport.Init()

	case actionDiskDelete:
		a.confirmValue = new(bool)
		a.confirmForm = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Delete disk '%s'?", disk.cfg.Name)).
					Description("This permanently destroys the data and cannot be undone.").
					Value(a.confirmValue),
			),
		)
		a.screen = screenDiskConfirmDelete
		return a, a.confirmForm.Init()

	case actionDiskResize:
		currentSize := disk.status.SizeGB
		if currentSize == "" {
			currentSize = disk.cfg.SizeGB
		}
		a.diskResize = newDiskResizeModel(disk.cfg, currentSize)
		a.screen = screenDiskResize
		return a, a.diskResize.Init()

	case actionDiskAttach:
		// Show spinner while checking running VMs
		a.disklist.loading = true
		a.disklist.refreshStart = time.Now()
		a.disklist.loadingText = "Checking running instances..."
		a.activeDisk = disk
		return a, tea.Batch(a.disklist.spinner.Tick, loadRunningVMs(disk))

	case actionDiskDetach:
		if len(disk.status.Users) == 0 {
			return a, nil
		}
		a.detachInstance = new(string)
		a.confirmValue = new(bool)
		if len(disk.status.Users) == 1 {
			// Single user — simple confirm
			*a.detachInstance = disk.status.Users[0]
			a.confirmForm = huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Detach disk '%s' from '%s'?", disk.cfg.Name, disk.status.Users[0])).
						Value(a.confirmValue),
				),
			)
		} else {
			// Multiple users — select which instance or all
			var opts []huh.Option[string]
			opts = append(opts, huh.NewOption(fmt.Sprintf("All (%d instances)", len(disk.status.Users)), "__all__"))
			for _, u := range disk.status.Users {
				opts = append(opts, huh.NewOption(u, u))
			}
			a.confirmForm = huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title(fmt.Sprintf("Detach disk '%s' from which instance?", disk.cfg.Name)).
						Options(opts...).
						Value(a.detachInstance),
				),
				huh.NewGroup(
					huh.NewConfirm().
						Title("Confirm detach?").
						Value(a.confirmValue),
				),
			)
		}
		a.screen = screenDiskDetach
		return a, a.confirmForm.Init()
	}
	return a, nil
}

// vmsInZone returns names of managed VMs in the given zone.
// findVMAttachedDisks returns the attached disk names for a VM from the already-loaded vmlist.
func (a App) findVMAttachedDisks(vmName string) []string {
	for _, vm := range a.vmlist.vms {
		if vm.cfg.VMName == vmName {
			return vm.attachedDisks
		}
	}
	return nil
}

func vmsInZone(zone string) []string {
	names, _ := vmsInZoneWithUsers(zone)
	return names
}

// loadRunningVMs returns a tea.Cmd that checks for running VMs in the background.
// loadDisksForVM returns a tea.Cmd that finds managed disks in the same project/zone as the VM.
func loadDisksForVM(vmCfg config.Config) tea.Cmd {
	return func() tea.Msg {
		diskNames := config.ListDisks()
		var names []string
		cfgs := make(map[string]config.DiskConfig)
		for _, name := range diskNames {
			tfvarsPath := filepath.Join(config.DiskDir(name), "terraform.tfvars")
			cfg, err := config.LoadDiskTFVars(tfvarsPath)
			if err != nil {
				continue
			}
			if cfg.ProjectID == vmCfg.ProjectID && cfg.Zone == vmCfg.Zone {
				names = append(names, cfg.Name)
				cfgs[cfg.Name] = cfg
			}
		}
		return vmAttachDisksReadyMsg{vmCfg: vmCfg, diskNames: names, diskCfgs: cfgs}
	}
}

func loadRunningVMs(disk diskEntry) tea.Cmd {
	return func() tea.Msg {
		vmNames, vmUsernames, vmProjectIDs := runningVMsInZoneAndProject(disk.cfg.Zone, disk.cfg.ProjectID)
		return diskAttachReadyMsg{
			disk:         disk,
			vmNames:      vmNames,
			vmUsernames:  vmUsernames,
			vmProjectIDs: vmProjectIDs,
			formatted:    disk.cfg.Formatted == "true",
		}
	}
}

// runningVMsInZoneAndProject returns names, usernames, and project IDs of running VMs
// in the given zone and project. GCP requires disk and instance to be in the same project.
func runningVMsInZoneAndProject(zone, projectID string) ([]string, map[string]string, map[string]string) {
	names := config.ListProjects()

	// Load configs and filter by zone + project (local, fast)
	var candidates []config.Config
	for _, name := range names {
		tfvarsPath := filepath.Join(config.ProjectDir(name), "terraform.tfvars")
		cfg, err := config.LoadTFVars(tfvarsPath)
		if err != nil {
			continue
		}
		if cfg.Zone == zone && cfg.ProjectID == projectID {
			candidates = append(candidates, cfg)
		}
	}

	// Query all statuses concurrently
	statuses := make([]string, len(candidates))
	var wg sync.WaitGroup
	for i, cfg := range candidates {
		wg.Add(1)
		go func(idx int, cfg config.Config) {
			defer wg.Done()
			statuses[idx] = gcloud.InstanceStatus(cfg.VMName, cfg.ProjectID, cfg.Zone)
		}(i, cfg)
	}
	wg.Wait()

	// Collect running VMs
	var result []string
	usernames := make(map[string]string)
	projectIDs := make(map[string]string)
	for i, cfg := range candidates {
		if statuses[i] == "RUNNING" {
			result = append(result, cfg.VMName)
			usernames[cfg.VMName] = cfg.Username
			projectIDs[cfg.VMName] = cfg.ProjectID
		}
	}
	return result, usernames, projectIDs
}

// vmsInZoneWithUsers returns VM names and a map of vmName→username for VMs in the given zone.
func vmsInZoneWithUsers(zone string) ([]string, map[string]string) {
	names := config.ListProjects()
	var result []string
	usernames := make(map[string]string)
	for _, name := range names {
		tfvarsPath := filepath.Join(config.ProjectDir(name), "terraform.tfvars")
		cfg, err := config.LoadTFVars(tfvarsPath)
		if err != nil {
			continue
		}
		if cfg.Zone == zone {
			result = append(result, cfg.VMName)
			usernames[cfg.VMName] = cfg.Username
		}
	}
	return result, usernames
}

// refreshDiskList switches to the disk list tab and triggers a reload.
func (a App) refreshDiskList() (App, tea.Cmd) {
	a.screen = screenMain
	a.activeTab = tabDataDisks
	a.disklist.loading = true
		a.disklist.refreshStart = time.Now()
	a.vmlist.loading = true
		a.vmlist.refreshStart = time.Now()
	return a, tea.Batch(loadDiskList, a.disklist.spinner.Tick, loadVMList, a.vmlist.spinner.Tick)
}

// --- Disk Create Screen ---

func (a App) updateDiskCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.diskForm, cmd = a.diskForm.Update(msg)

	switch msg.(type) {
	case diskFormDoneMsg:
		cfg := msg.(diskFormDoneMsg).cfg
		a.bgRunning = true
		a.bgSourceTab = tabDataDisks
		a.bgTitle = "Creating data disk..."
		a.progress = newProgressModel("Creating data disk...")
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdCreateDisk(cfg))

	case diskFormCancelMsg:
		a, cmd = a.refreshDiskList()
		return a, cmd
	}

	return a, cmd
}

// --- Disk Import Screen ---

func (a App) updateDiskImportScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.diskImport, cmd = a.diskImport.Update(msg)

	switch msg.(type) {
	case diskImportDoneMsg:
		cfg := msg.(diskImportDoneMsg).cfg
		a.bgRunning = true
		a.bgSourceTab = tabDataDisks
		a.bgTitle = "Importing disk..."
		a.progress = newProgressModel("Importing disk...")
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdImportDisk(cfg))

	case diskImportCancelMsg:
		a, cmd = a.refreshDiskList()
		return a, cmd
	}

	return a, cmd
}

// --- Disk Confirm Delete ---

func (a App) updateDiskConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenMain
			return a, nil
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		if *a.confirmValue {
			a.confirmNameValue = new(string)
			diskName := a.activeDisk.cfg.Name
			a.confirmForm = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title(fmt.Sprintf("Type '%s' to confirm deletion", diskName)).
						Description("This cannot be undone.").
						Value(a.confirmNameValue).
						Validate(func(s string) error {
							if s != diskName {
								return fmt.Errorf("name does not match")
							}
							return nil
						}),
				),
			)
			a.screen = screenDiskConfirmDeleteName
			return a, a.confirmForm.Init()
		}
		a.screen = screenMain
		return a, nil
	}

	return a, cmd
}

func (a App) viewDiskConfirmDelete() string {
	return titleStyle.Render("Delete Data Disk") + "\n\n" + a.confirmForm.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Disk Confirm Delete Name ---

func (a App) updateDiskConfirmDeleteName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenMain
			return a, nil
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		a.bgRunning = true
		a.bgSourceTab = tabDataDisks
		a.bgTitle = "Deleting data disk..."
		a.progress = newProgressModel("Deleting data disk...")
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdDeleteDisk(a.activeDisk.cfg))
	}

	return a, cmd
}

func (a App) viewDiskConfirmDeleteName() string {
	return titleStyle.Render("Confirm Delete Disk") + "\n\n" + a.confirmForm.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Disk Resize Screen ---

func (a App) updateDiskResizeScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.diskResize, cmd = a.diskResize.Update(msg)

	switch msg.(type) {
	case diskResizeDoneMsg:
		cfg := msg.(diskResizeDoneMsg).cfg
		a.bgRunning = true
		a.bgSourceTab = tabDataDisks
		a.bgTitle = "Resizing data disk..."
		a.progress = newProgressModel("Resizing data disk...")
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdResizeDisk(cfg))

	case diskResizeCancelMsg:
		a, cmd = a.refreshDiskList()
		return a, cmd
	}

	return a, cmd
}

// --- Disk Attach Screen ---

func (a App) updateDiskAttachScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.diskAttach, cmd = a.diskAttach.Update(msg)

	switch msg.(type) {
	case diskAttachDoneMsg:
		done := msg.(diskAttachDoneMsg)
		a.lastAttachOpts = done
		a.diskAttach.lastAttachDone = done

		if !done.mount {
			// Confirm attach without mount
			a.confirmValue = new(bool)
			a.confirmForm = huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Attach '%s' to '%s' without mounting?", done.diskCfg.Name, done.instanceName)).
						Description("The disk will be attached but not mounted. You can mount it later.").
						Value(a.confirmValue),
				),
			)
			a.screen = screenDiskAttachConfirm
			return a, a.confirmForm.Init()
		}

		// Mount requested
		if done.sourceTab == tabDataDisks {
			// Disk-tab form already collected mount options — go straight to attach
			a.bgRunning = true
			a.bgSourceTab = done.sourceTab
			a.bgTitle = "Attaching and mounting disk..."
			a.progress = newProgressModel("Attaching and mounting disk...")
			a.screen = screenProgress
			return a, tea.Batch(a.progress.Init(), a.cmdAttachDisk(done))
		}
		// VM-tab flow — show separate mount options form with correct disk name
		a.diskAttach.fields.mountPoint = fmt.Sprintf("/mnt/disks/%s", done.diskCfg.Name)
		formatted := done.diskCfg.Formatted == "true"
		a.diskMountOpts = newDiskMountOptionsModel(a.diskAttach.fields, formatted)
		a.screen = screenDiskMountOptions
		return a, a.diskMountOpts.Init()

	case diskAttachCancelMsg:
		a, cmd = a.refreshDiskList()
		return a, cmd
	}

	return a, cmd
}

// --- Disk Attach Confirm (no-mount) ---

func (a App) updateDiskAttachConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			// Back to Form 1
			a.diskAttach.rebuildForm()
			a.screen = screenDiskAttach
			return a, a.diskAttach.Init()
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		if *a.confirmValue {
			done := a.lastAttachOpts
			a.bgRunning = true
			a.bgSourceTab = done.sourceTab
			a.bgTitle = "Attaching disk..."
			a.progress = newProgressModel("Attaching disk...")
			a.screen = screenProgress
			return a, tea.Batch(a.progress.Init(), a.cmdAttachDisk(done))
		}
		// User declined — back to Form 1
		a.diskAttach.rebuildForm()
		a.screen = screenDiskAttach
		return a, a.diskAttach.Init()
	}

	return a, cmd
}

// --- Disk Mount Options Screen ---

func (a App) updateDiskMountOptionsScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.diskMountOpts, cmd = a.diskMountOpts.Update(msg)

	switch msg.(type) {
	case diskMountOptionsDoneMsg:
		// Build the full done message from the accumulated fields
		f := a.lastAttachOpts
		fields := a.diskAttach.fields
		f.mount = true
		f.mountPoint = fields.mountPoint
		f.formatDisk = fields.formatDisk
		f.fsType = fields.fsType
		f.owner = fields.owner
		a.lastAttachOpts = f

		a.bgRunning = true
		a.bgSourceTab = f.sourceTab
		a.bgTitle = "Attaching and mounting disk..."
		a.progress = newProgressModel("Attaching and mounting disk...")
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdAttachDisk(f))

	case diskMountOptionsBackMsg:
		// Go back to Form 1 (attach form) with previous selections preserved
		a.diskAttach.rebuildForm()
		a.screen = screenDiskAttach
		return a, a.diskAttach.Init()

	case diskMountOptionsCancelMsg:
		// Full cancel — back to disk list
		a, cmd = a.refreshDiskList()
		return a, cmd
	}

	return a, cmd
}

// --- Disk Detach from VM Screen ---

func (a App) updateDiskDetachFromVMScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.diskDetachVM, cmd = a.diskDetachVM.Update(msg)

	switch msg.(type) {
	case diskDetachFromVMDoneMsg:
		done := msg.(diskDetachFromVMDoneMsg)
		a.bgRunning = true
		a.bgSourceTab = tabInstances
		diskCount := len(done.diskCfgs)
		if diskCount == 1 {
			a.bgTitle = fmt.Sprintf("Detaching disk '%s'...", done.diskCfgs[0].Name)
		} else {
			a.bgTitle = fmt.Sprintf("Detaching %d disks...", diskCount)
		}
		a.progress = newProgressModel(a.bgTitle)
		a.screen = screenProgress
		return a, tea.Batch(a.progress.Init(), a.cmdDetachDisksFromVM(done.vmCfg, done.diskCfgs))

	case diskDetachFromVMCancelMsg:
		a.screen = screenMain
		return a, nil
	}

	return a, cmd
}

// --- Disk Detach Screen ---

func (a App) updateDiskDetachScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenMain
			return a, nil
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		if *a.confirmValue {
			a.bgRunning = true
			a.bgSourceTab = tabDataDisks
			if *a.detachInstance == "__all__" {
				a.bgTitle = fmt.Sprintf("Detaching disk from %d instances...", len(a.activeDisk.status.Users))
				a.progress = newProgressModel(a.bgTitle)
				a.screen = screenProgress
				return a, tea.Batch(a.progress.Init(), a.cmdDetachDiskFromAll(a.activeDisk.cfg, a.activeDisk.status.Users))
			}
			a.bgTitle = "Detaching disk..."
			a.progress = newProgressModel("Detaching disk...")
			a.screen = screenProgress
			return a, tea.Batch(a.progress.Init(), a.cmdDetachDisk(a.activeDisk.cfg, *a.detachInstance))
		}
		a.screen = screenMain
		return a, nil
	}

	return a, cmd
}

func (a App) viewDiskDetach() string {
	return titleStyle.Render("Detach Disk") + "\n\n" + a.confirmForm.View() + "\n" + dimStyle.Render("esc/ctrl+c cancel")
}

// --- Commands ---

func (a *App) cmdLaunchVM(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}
		if !gcloud.HasADC() {
			return authNeededMsg{kind: "adc"}
		}
		a.program.Send(logLineMsg("GCP authentication verified."))

		projDir := config.ProjectDir(cfg.VMName)

		// Write main.tf
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			return progressDoneMsg{err: fmt.Errorf("create project dir: %w", err)}
		}
		mainTFPath := filepath.Join(projDir, "main.tf")
		if err := os.WriteFile(mainTFPath, []byte(a.embeddedMainTF), 0o644); err != nil {
			return progressDoneMsg{err: fmt.Errorf("write main.tf: %w", err)}
		}

		// Write terraform.tfvars
		tfvarsPath := filepath.Join(projDir, "terraform.tfvars")
		if err := config.WriteTFVars(tfvarsPath, cfg); err != nil {
			return progressDoneMsg{err: fmt.Errorf("write tfvars: %w", err)}
		}

		// Ensure terraform is installed
		a.program.Send(logLineMsg("Checking Terraform installation..."))
		ctx := context.Background()
		execPath, err := tf.EnsureInstalled(ctx)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("install terraform: %w", err)}
		}
		a.program.Send(logLineMsg(fmt.Sprintf("Terraform: %s", execPath)))

		// Create runner
		runner, err := tf.NewRunner(projDir, execPath)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("create runner: %w", err)}
		}

		lw := newLogWriter(a.program)
		runner.SetOutput(lw, lw)

		// Init
		a.program.Send(logLineMsg("Running terraform init..."))
		if err := runner.Init(ctx); err != nil {
			return progressDoneMsg{err: fmt.Errorf("terraform init: %w", err)}
		}

		// Apply
		a.program.Send(logLineMsg("Running terraform apply..."))
		if err := runner.Apply(ctx); err != nil {
			lw.Flush()
			return progressDoneMsg{err: fmt.Errorf("terraform apply: %w", err)}
		}
		lw.Flush()

		// Only start tunnels if the instance is running
		status := gcloud.InstanceStatus(cfg.VMName, cfg.ProjectID, cfg.Zone)
		if status == "RUNNING" {
			a.program.Send(logLineMsg("Waiting for SSH to become available..."))
			if err := gcloud.WaitForSSH(cfg, 120*time.Second, func(attempt int, elapsed time.Duration) {
				a.program.Send(logLineMsg(fmt.Sprintf("  SSH not ready yet (attempt %d, %.0fs elapsed), retrying...", attempt, elapsed.Seconds())))
			}); err != nil {
				a.program.Send(logLineMsg(fmt.Sprintf("Warning: SSH readiness check failed: %v", err)))
				a.program.Send(logLineMsg("Attempting tunnels anyway..."))
			}
			a.program.Send(logLineMsg("Starting SSH tunnels..."))
			for _, pp := range cfg.PortMappings() {
				if err := a.tunnelMgr.StartTunnel(cfg, pp); err != nil {
					a.program.Send(logLineMsg(fmt.Sprintf("Warning: tunnel %s:%s failed: %v", pp.Local, pp.Remote, err)))
				} else {
					a.program.Send(logLineMsg(fmt.Sprintf("Tunnel %s:%s started", pp.Local, pp.Remote)))
				}
			}
		} else {
			a.program.Send(logLineMsg(fmt.Sprintf("Instance is %s — skipping tunnel setup.", status)))
		}

		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdStartTunnels() tea.Cmd {
	return func() tea.Msg {
		cfg := a.activeConfig

		status := gcloud.InstanceStatus(cfg.VMName, cfg.ProjectID, cfg.Zone)

		if status != "RUNNING" {
			a.program.Send(logLineMsg("Starting instance..."))
			if err := gcloud.StartInstance(cfg); err != nil {
				return progressDoneMsg{err: err}
			}
			a.program.Send(logLineMsg("Instance started."))
		}

		sshTimeout := 120 * time.Second
		if status == "RUNNING" {
			sshTimeout = 30 * time.Second
		}
		a.program.Send(logLineMsg("Waiting for SSH to become available..."))
		if err := gcloud.WaitForSSH(cfg, sshTimeout, func(attempt int, elapsed time.Duration) {
			a.program.Send(logLineMsg(fmt.Sprintf("  SSH not ready yet (attempt %d, %.0fs elapsed), retrying...", attempt, elapsed.Seconds())))
		}); err != nil {
			return progressDoneMsg{err: fmt.Errorf("SSH not available: %w", err)}
		}
		a.program.Send(logLineMsg("SSH is ready."))

		a.program.Send(logLineMsg("Starting SSH tunnels..."))
		for _, pp := range cfg.PortMappings() {
			if err := a.tunnelMgr.StartTunnel(cfg, pp); err != nil {
				a.program.Send(logLineMsg(fmt.Sprintf("Warning: tunnel %s:%s failed: %v", pp.Local, pp.Remote, err)))
			} else {
				a.program.Send(logLineMsg(fmt.Sprintf("Tunnel %s:%s started", pp.Local, pp.Remote)))
			}
		}

		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdStopVM() tea.Cmd {
	return func() tea.Msg {
		cfg := a.activeConfig

		a.program.Send(logLineMsg("Stopping instance..."))
		if err := gcloud.StopInstance(cfg); err != nil {
			return progressDoneMsg{err: err}
		}
		a.program.Send(logLineMsg("Instance stopped."))

		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdStopAll() tea.Cmd {
	// Snapshot VMs to avoid race with background refresh
	vms := make([]vmEntry, len(a.vmlist.vms))
	copy(vms, a.vmlist.vms)
	return func() tea.Msg {
		// Stop all tunnels for every VM
		for _, vm := range vms {
			a.tunnelMgr.StopAll(vm.cfg.VMName)
			a.program.Send(logLineMsg(fmt.Sprintf("Stopped tunnels for %s", vm.cfg.VMName)))
		}

		// Stop all running instances
		for _, vm := range vms {
			if vm.status != "RUNNING" {
				continue
			}
			a.program.Send(logLineMsg(fmt.Sprintf("Stopping instance %s...", vm.cfg.VMName)))
			if err := gcloud.StopInstance(vm.cfg); err != nil {
				a.program.Send(logLineMsg(fmt.Sprintf("Warning: failed to stop %s: %v", vm.cfg.VMName, err)))
			} else {
				a.program.Send(logLineMsg(fmt.Sprintf("Instance %s stopped.", vm.cfg.VMName)))
			}
		}

		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdDestroyVM() tea.Cmd {
	return func() tea.Msg {
		cfg := a.activeConfig
		projDir := config.ProjectDir(cfg.VMName)

		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}
		if !gcloud.HasADC() {
			return authNeededMsg{kind: "adc"}
		}
		a.program.Send(logLineMsg("GCP authentication verified."))

		ctx := context.Background()
		execPath, err := tf.EnsureInstalled(ctx)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("install terraform: %w", err)}
		}

		runner, err := tf.NewRunner(projDir, execPath)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("create runner: %w", err)}
		}

		lw := newLogWriter(a.program)
		runner.SetOutput(lw, lw)

		a.program.Send(logLineMsg("Running terraform destroy..."))
		if err := runner.Init(ctx); err != nil {
			return progressDoneMsg{err: fmt.Errorf("terraform init: %w", err)}
		}
		if err := runner.Destroy(ctx); err != nil {
			lw.Flush()
			return progressDoneMsg{err: fmt.Errorf("terraform destroy: %w", err)}
		}
		lw.Flush()

		// Clean up project directory
		a.program.Send(logLineMsg("Cleaning up project directory..."))
		_ = os.RemoveAll(projDir)

		return progressDoneMsg{err: nil}
	}
}

// --- Disk Commands ---

func (a *App) cmdCreateDisk(cfg config.DiskConfig) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}
		if !gcloud.HasADC() {
			return authNeededMsg{kind: "adc"}
		}
		a.program.Send(logLineMsg("GCP authentication verified."))

		diskDir := config.DiskDir(cfg.Name)

		// Write disk.tf
		if err := os.MkdirAll(diskDir, 0o755); err != nil {
			return progressDoneMsg{err: fmt.Errorf("create disk dir: %w", err)}
		}
		diskTFPath := filepath.Join(diskDir, "main.tf")
		if err := os.WriteFile(diskTFPath, []byte(a.embeddedDiskTF), 0o644); err != nil {
			return progressDoneMsg{err: fmt.Errorf("write disk main.tf: %w", err)}
		}

		// Write terraform.tfvars
		tfvarsPath := filepath.Join(diskDir, "terraform.tfvars")
		if err := config.WriteDiskTFVars(tfvarsPath, cfg); err != nil {
			return progressDoneMsg{err: fmt.Errorf("write disk tfvars: %w", err)}
		}

		// Ensure terraform is installed
		a.program.Send(logLineMsg("Checking Terraform installation..."))
		ctx := context.Background()
		execPath, err := tf.EnsureInstalled(ctx)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("install terraform: %w", err)}
		}

		// Create runner
		runner, err := tf.NewRunner(diskDir, execPath)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("create runner: %w", err)}
		}

		lw := newLogWriter(a.program)
		runner.SetOutput(lw, lw)

		// Init
		a.program.Send(logLineMsg("Running terraform init..."))
		if err := runner.Init(ctx); err != nil {
			return progressDoneMsg{err: fmt.Errorf("terraform init: %w", err)}
		}

		// Apply
		a.program.Send(logLineMsg("Running terraform apply..."))
		if err := runner.Apply(ctx); err != nil {
			lw.Flush()
			return progressDoneMsg{err: fmt.Errorf("terraform apply: %w", err)}
		}
		lw.Flush()

		a.program.Send(logLineMsg(fmt.Sprintf("Data disk '%s' created successfully.", cfg.Name)))
		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdImportDisk(cfg config.DiskConfig) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}
		if !gcloud.HasADC() {
			return authNeededMsg{kind: "adc"}
		}
		a.program.Send(logLineMsg("GCP authentication verified."))

		// Verify the disk exists in GCP
		a.program.Send(logLineMsg(fmt.Sprintf("Verifying disk '%s' exists in %s/%s...", cfg.Name, cfg.ProjectID, cfg.Zone)))
		if !gcloud.DiskExists(cfg.Name, cfg.ProjectID, cfg.Zone) {
			return progressDoneMsg{err: fmt.Errorf("disk '%s' not found in project '%s' zone '%s'", cfg.Name, cfg.ProjectID, cfg.Zone)}
		}

		// Get current disk info to populate config
		status := gcloud.GetDiskStatus(cfg.Name, cfg.ProjectID, cfg.Zone)
		if status.SizeGB != "" {
			cfg.SizeGB = status.SizeGB
		}
		if status.DiskType != "" {
			cfg.DiskType = status.DiskType
		}

		diskDir := config.DiskDir(cfg.Name)

		// Write disk.tf
		if err := os.MkdirAll(diskDir, 0o755); err != nil {
			return progressDoneMsg{err: fmt.Errorf("create disk dir: %w", err)}
		}
		diskTFPath := filepath.Join(diskDir, "main.tf")
		if err := os.WriteFile(diskTFPath, []byte(a.embeddedDiskTF), 0o644); err != nil {
			return progressDoneMsg{err: fmt.Errorf("write disk main.tf: %w", err)}
		}

		// Write terraform.tfvars
		tfvarsPath := filepath.Join(diskDir, "terraform.tfvars")
		if err := config.WriteDiskTFVars(tfvarsPath, cfg); err != nil {
			return progressDoneMsg{err: fmt.Errorf("write disk tfvars: %w", err)}
		}

		// Ensure terraform is installed
		a.program.Send(logLineMsg("Checking Terraform installation..."))
		ctx := context.Background()
		execPath, err := tf.EnsureInstalled(ctx)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("install terraform: %w", err)}
		}

		// Create runner
		runner, err := tf.NewRunner(diskDir, execPath)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("create runner: %w", err)}
		}

		lw := newLogWriter(a.program)
		runner.SetOutput(lw, lw)

		// Init
		a.program.Send(logLineMsg("Running terraform init..."))
		if err := runner.Init(ctx); err != nil {
			return progressDoneMsg{err: fmt.Errorf("terraform init: %w", err)}
		}

		// Import
		importID := fmt.Sprintf("projects/%s/zones/%s/disks/%s", cfg.ProjectID, cfg.Zone, cfg.Name)
		a.program.Send(logLineMsg(fmt.Sprintf("Running terraform import for %s...", importID)))
		if err := runner.Import(ctx, "google_compute_disk.data", importID); err != nil {
			lw.Flush()
			return progressDoneMsg{err: fmt.Errorf("terraform import: %w", err)}
		}
		lw.Flush()

		a.program.Send(logLineMsg(fmt.Sprintf("Disk '%s' imported and now managed by vmup.", cfg.Name)))
		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdDeleteDisk(cfg config.DiskConfig) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}
		if !gcloud.HasADC() {
			return authNeededMsg{kind: "adc"}
		}
		a.program.Send(logLineMsg("GCP authentication verified."))

		diskDir := config.DiskDir(cfg.Name)

		// Rewrite main.tf without prevent_destroy
		diskTFPath := filepath.Join(diskDir, "main.tf")
		a.program.Send(logLineMsg("Removing prevent_destroy lifecycle rule..."))
		if err := os.WriteFile(diskTFPath, []byte(a.embeddedDiskDeletable), 0o644); err != nil {
			return progressDoneMsg{err: fmt.Errorf("write deletable disk tf: %w", err)}
		}

		// Ensure terraform is installed
		ctx := context.Background()
		execPath, err := tf.EnsureInstalled(ctx)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("install terraform: %w", err)}
		}

		// Create runner
		runner, err := tf.NewRunner(diskDir, execPath)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("create runner: %w", err)}
		}

		lw := newLogWriter(a.program)
		runner.SetOutput(lw, lw)

		// Apply to update lifecycle (remove prevent_destroy)
		a.program.Send(logLineMsg("Running terraform apply to update lifecycle..."))
		if err := runner.Init(ctx); err != nil {
			return progressDoneMsg{err: fmt.Errorf("terraform init: %w", err)}
		}
		if err := runner.Apply(ctx); err != nil {
			lw.Flush()
			return progressDoneMsg{err: fmt.Errorf("terraform apply: %w", err)}
		}

		// Now destroy
		a.program.Send(logLineMsg("Running terraform destroy..."))
		if err := runner.Destroy(ctx); err != nil {
			lw.Flush()
			return progressDoneMsg{err: fmt.Errorf("terraform destroy: %w", err)}
		}
		lw.Flush()

		// Clean up disk directory
		a.program.Send(logLineMsg("Cleaning up disk directory..."))
		_ = os.RemoveAll(diskDir)

		a.program.Send(logLineMsg(fmt.Sprintf("Data disk '%s' deleted.", cfg.Name)))
		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdResizeDisk(cfg config.DiskConfig) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}
		if !gcloud.HasADC() {
			return authNeededMsg{kind: "adc"}
		}
		a.program.Send(logLineMsg("GCP authentication verified."))

		diskDir := config.DiskDir(cfg.Name)

		// Update terraform.tfvars with new size
		tfvarsPath := filepath.Join(diskDir, "terraform.tfvars")
		if err := config.WriteDiskTFVars(tfvarsPath, cfg); err != nil {
			return progressDoneMsg{err: fmt.Errorf("write disk tfvars: %w", err)}
		}

		// Ensure terraform is installed
		ctx := context.Background()
		execPath, err := tf.EnsureInstalled(ctx)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("install terraform: %w", err)}
		}

		// Create runner
		runner, err := tf.NewRunner(diskDir, execPath)
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("create runner: %w", err)}
		}

		lw := newLogWriter(a.program)
		runner.SetOutput(lw, lw)

		// Apply
		a.program.Send(logLineMsg("Running terraform apply to resize disk..."))
		if err := runner.Init(ctx); err != nil {
			return progressDoneMsg{err: fmt.Errorf("terraform init: %w", err)}
		}
		if err := runner.Apply(ctx); err != nil {
			lw.Flush()
			return progressDoneMsg{err: fmt.Errorf("terraform apply: %w", err)}
		}
		lw.Flush()

		a.program.Send(logLineMsg(fmt.Sprintf("Disk '%s' resized to %s GB.", cfg.Name, cfg.SizeGB)))
		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdAttachDisk(opts diskAttachDoneMsg) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}

		// Step 1: GCP-level attach
		a.program.Send(logLineMsg(fmt.Sprintf("Attaching disk '%s' to instance '%s' (mode: %s)...", opts.diskCfg.Name, opts.instanceName, opts.mode)))
		if err := gcloud.AttachDisk(opts.instanceProjectID, opts.instanceName, opts.diskCfg.Name, opts.diskCfg.Zone, opts.mode); err != nil {
			return progressDoneMsg{err: err}
		}
		a.program.Send(logLineMsg(fmt.Sprintf("Disk '%s' attached to '%s'.", opts.diskCfg.Name, opts.instanceName)))

		if !opts.mount {
			return progressDoneMsg{err: nil}
		}

		// Build a config.Config for SSH commands to the target VM
		vmCfg := config.Config{
			VMName:    opts.instanceName,
			ProjectID: opts.instanceProjectID,
			Zone:      opts.diskCfg.Zone,
		}

		// Step 2: Wait for SSH
		a.program.Send(logLineMsg("Waiting for SSH access..."))
		if err := gcloud.WaitForSSH(vmCfg, 60*time.Second, func(attempt int, elapsed time.Duration) {
			a.program.Send(logLineMsg(fmt.Sprintf("  SSH not ready yet (attempt %d), retrying...", attempt)))
		}); err != nil {
			a.program.Send(logLineMsg(fmt.Sprintf("Warning: SSH not available: %v. Disk is attached but not mounted.", err)))
			return progressDoneMsg{err: fmt.Errorf("disk attached but mount failed: SSH unavailable: %w", err)}
		}

		// Step 3: Find the device path (may take a few seconds to appear)
		a.program.Send(logLineMsg("Finding disk device..."))
		device, err := gcloud.FindDiskDevice(vmCfg, opts.diskCfg.Name, func(attempt int) {
			a.program.Send(logLineMsg(fmt.Sprintf("  Device not ready yet (attempt %d), retrying...", attempt)))
		})
		if err != nil {
			return progressDoneMsg{err: fmt.Errorf("disk attached but mount failed: %w", err)}
		}
		a.program.Send(logLineMsg(fmt.Sprintf("Disk device: %s", device)))

		// Step 4: Format if needed (skip for read-only — can't write to ro block device)
		if opts.mode == "ro" {
			// Verify the disk has a filesystem — can't mount a blank disk even in ro mode
			hasFS, _ := gcloud.CheckFilesystem(vmCfg, device)
			if !hasFS {
				a.program.Send(logLineMsg("Disk has no filesystem and cannot be formatted in read-only mode."))
				a.program.Send(logLineMsg("Attach in read/write mode first to format the disk."))
				return progressDoneMsg{err: fmt.Errorf("disk attached but cannot mount: no filesystem (format requires read/write mode)")}
			}
			a.program.Send(logLineMsg("Read-only mode — skipping format."))
		} else {
			a.program.Send(logLineMsg(fmt.Sprintf("Checking filesystem on %s...", device)))
			hasFS, _ := gcloud.CheckFilesystem(vmCfg, device)

			needsFormat := opts.formatDisk || !hasFS
			if needsFormat {
				if hasFS && opts.formatDisk {
					a.program.Send(logLineMsg("Warning: disk already has a filesystem — formatting as requested."))
				} else if !hasFS {
					a.program.Send(logLineMsg("Disk has no filesystem — formatting is required."))
				}
				a.program.Send(logLineMsg(fmt.Sprintf("Formatting %s as %s...", device, opts.fsType)))
				if err := gcloud.FormatDisk(vmCfg, device, opts.fsType); err != nil {
					return progressDoneMsg{err: fmt.Errorf("disk attached but format failed: %w", err)}
				}
				a.program.Send(logLineMsg("Format complete."))

				// Save formatted state
				opts.diskCfg.Formatted = "true"
				tfvarsPath := filepath.Join(config.DiskDir(opts.diskCfg.Name), "terraform.tfvars")
				_ = config.WriteDiskTFVars(tfvarsPath, opts.diskCfg)
			} else {
				a.program.Send(logLineMsg("Filesystem detected — skipping format."))
			}
		}

		// Step 5: Clean up any stale mount from previous attach, then mount fresh
		a.program.Send(logLineMsg("Cleaning up any stale mount..."))
		_ = gcloud.UnmountDisk(vmCfg, device)

		a.program.Send(logLineMsg(fmt.Sprintf("Mounting %s at %s (mode: %s)...", device, opts.mountPoint, opts.mode)))
		if err := gcloud.MountDisk(vmCfg, device, opts.mountPoint, opts.mode); err != nil {
			return progressDoneMsg{err: fmt.Errorf("disk attached but mount failed: %w", err)}
		}
		a.program.Send(logLineMsg("Disk mounted."))

		// Step 6: Configure fstab
		a.program.Send(logLineMsg("Configuring /etc/fstab for persistent mount..."))
		if err := gcloud.ConfigureFstab(vmCfg, device, opts.mountPoint, opts.fsType, opts.mode); err != nil {
			a.program.Send(logLineMsg(fmt.Sprintf("Warning: fstab configuration failed: %v", err)))
			// Non-fatal — disk is still mounted for this session
		} else {
			a.program.Send(logLineMsg("Fstab configured (with nofail)."))
		}

		// Step 7: Set ownership (skip for read-only mounts)
		if opts.owner != "" && opts.mode != "ro" {
			a.program.Send(logLineMsg(fmt.Sprintf("Setting ownership to %s...", opts.owner)))
			if err := gcloud.SetMountOwnership(vmCfg, opts.mountPoint, opts.owner); err != nil {
				a.program.Send(logLineMsg(fmt.Sprintf("Warning: ownership change failed: %v", err)))
			} else {
				a.program.Send(logLineMsg("Ownership set."))
			}
		}

		a.program.Send(logLineMsg(fmt.Sprintf("Disk '%s' attached and mounted at %s on '%s'.", opts.diskCfg.Name, opts.mountPoint, opts.instanceName)))
		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdDetachDisk(diskCfg config.DiskConfig, instanceName string) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}

		// Unmount the disk inside the VM before GCP-level detach
		vmCfg := config.Config{
			VMName:    instanceName,
			ProjectID: diskCfg.ProjectID,
			Zone:      diskCfg.Zone,
		}
		status := gcloud.InstanceStatus(instanceName, diskCfg.ProjectID, diskCfg.Zone)
		if status == "RUNNING" {
			a.program.Send(logLineMsg("Unmounting disk and cleaning up fstab..."))
			device, err := gcloud.FindDiskDevice(vmCfg, diskCfg.Name, nil)
			if err == nil {
				_ = gcloud.UnmountDisk(vmCfg, device)
				a.program.Send(logLineMsg("Disk unmounted."))
			}
		}

		a.program.Send(logLineMsg(fmt.Sprintf("Detaching disk '%s' from instance '%s'...", diskCfg.Name, instanceName)))
		if err := gcloud.DetachDisk(diskCfg.ProjectID, instanceName, diskCfg.Name, diskCfg.Zone); err != nil {
			return progressDoneMsg{err: err}
		}

		a.program.Send(logLineMsg(fmt.Sprintf("Disk '%s' detached from '%s'.", diskCfg.Name, instanceName)))
		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdDetachDisksFromVM(vmCfg config.Config, diskCfgs []config.DiskConfig) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}

		instanceName := vmCfg.VMName
		status := gcloud.InstanceStatus(vmCfg.VMName, vmCfg.ProjectID, vmCfg.Zone)

		for _, diskCfg := range diskCfgs {
			// Unmount if VM is running
			if status == "RUNNING" {
				a.program.Send(logLineMsg(fmt.Sprintf("Unmounting '%s'...", diskCfg.Name)))
				device, err := gcloud.FindDiskDevice(config.Config{
					VMName:    instanceName,
					ProjectID: vmCfg.ProjectID,
					Zone:      vmCfg.Zone,
				}, diskCfg.Name, nil)
				if err == nil {
					_ = gcloud.UnmountDisk(config.Config{
						VMName:    instanceName,
						ProjectID: vmCfg.ProjectID,
						Zone:      vmCfg.Zone,
					}, device)
				}
			}

			// GCP-level detach
			a.program.Send(logLineMsg(fmt.Sprintf("Detaching '%s' from '%s'...", diskCfg.Name, instanceName)))
			if err := gcloud.DetachDisk(vmCfg.ProjectID, instanceName, diskCfg.Name, vmCfg.Zone); err != nil {
				a.program.Send(logLineMsg(fmt.Sprintf("Warning: failed to detach '%s': %v", diskCfg.Name, err)))
				continue
			}
			a.program.Send(logLineMsg(fmt.Sprintf("Disk '%s' detached.", diskCfg.Name)))
		}

		return progressDoneMsg{err: nil}
	}
}

func (a *App) cmdDetachDiskFromAll(diskCfg config.DiskConfig, instanceNames []string) tea.Cmd {
	return func() tea.Msg {
		a.program.Send(logLineMsg("Checking GCP authentication..."))
		if !gcloud.HasAuth() {
			return authNeededMsg{kind: "login"}
		}

		for _, instanceName := range instanceNames {
			vmCfg := config.Config{
				VMName:    instanceName,
				ProjectID: diskCfg.ProjectID,
				Zone:      diskCfg.Zone,
			}

			// Unmount if VM is running
			status := gcloud.InstanceStatus(instanceName, diskCfg.ProjectID, diskCfg.Zone)
			if status == "RUNNING" {
				a.program.Send(logLineMsg(fmt.Sprintf("Unmounting '%s' from '%s'...", diskCfg.Name, instanceName)))
				device, err := gcloud.FindDiskDevice(vmCfg, diskCfg.Name, nil)
				if err == nil {
					_ = gcloud.UnmountDisk(vmCfg, device)
				}
			}

			// GCP-level detach
			a.program.Send(logLineMsg(fmt.Sprintf("Detaching '%s' from '%s'...", diskCfg.Name, instanceName)))
			if err := gcloud.DetachDisk(diskCfg.ProjectID, instanceName, diskCfg.Name, diskCfg.Zone); err != nil {
				a.program.Send(logLineMsg(fmt.Sprintf("Warning: failed to detach from '%s': %v", instanceName, err)))
				continue
			}
			a.program.Send(logLineMsg(fmt.Sprintf("Detached from '%s'.", instanceName)))
		}

		return progressDoneMsg{err: nil}
	}
}

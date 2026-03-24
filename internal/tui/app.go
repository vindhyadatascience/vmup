package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"vds-gcp-launch-instance/internal/config"
	"vds-gcp-launch-instance/internal/gcloud"
	tf "vds-gcp-launch-instance/internal/terraform"
	"vds-gcp-launch-instance/internal/tunnel"
)

type App struct {
	screen   screen
	config   configModel
	progress progressModel
	status   statusModel
	vmlist   vmListModel

	// State
	activeConfig   config.Config
	tunnelMgr      *tunnel.Manager
	embeddedMainTF string
	program        *tea.Program

	// Background operation tracking
	bgRunning bool
	bgTitle   string

	// Edit mode
	editMode bool

	// For confirmation prompts
	confirmForm  *huh.Form
	confirmValue *bool

	width, height int
}

func NewApp(embeddedMainTF string) *App {
	tm := tunnel.NewManager()
	return &App{
		screen:         screenVMList,
		vmlist:         newVMListModel(tm),
		tunnelMgr:      tm,
		embeddedMainTF: embeddedMainTF,
	}
}

func (a *App) SetProgram(p *tea.Program) {
	a.program = p
}

func (a App) Init() tea.Cmd {
	return tea.Batch(a.vmlist.Init(), logoTick())
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
	case logoTickMsg:
		a.vmlist.gradientOffset++
		return a, logoTick()
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
			if a.screen == screenVMList {
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
				return a, tea.Batch(loadVMList, a.vmlist.spinner.Tick)
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
		if strings.Contains(a.bgTitle, "Launching") || strings.Contains(a.bgTitle, "Updating") {
			return a, a.cmdLaunchVM(a.activeConfig)
		}
		if strings.Contains(a.bgTitle, "Destroying") {
			return a, a.cmdDestroyVM()
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
	case screenConfirmStopVM:
		model, cmd = a.updateConfirmStopVM(msg)
	case screenConfirmStopAll:
		model, cmd = a.updateConfirmStopAll(msg)
	case screenVMList:
		model, cmd = a.updateVMList(msg)
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
	case screenConfirmStopVM:
		return a.viewConfirmStopVM()
	case screenConfirmStopAll:
		return a.viewConfirmStopAll()
	case screenVMList:
		v := a.vmlist.View()
		if a.bgRunning {
			v += "\n" + infoStyle.Render(fmt.Sprintf("⟳ %s (p to view)", a.bgTitle))
		}
		return v
	}
	return ""
}

// --- Dispatch Action ---

func (a App) dispatchAction(action menuAction) (tea.Model, tea.Cmd) {
	switch action {
	case actionStartTunnels:
		a.bgRunning = true
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
		*a.confirmValue = true
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
				a, cmd = a.refreshVMList()
				return a, cmd
			}
			a.screen = screenVMList
			return a, a.progress.spinner.Tick
		}

		// Allow esc to return to VM list while operation runs in background
		if key == "esc" && !a.progress.done {
			a.screen = screenVMList
			return a, a.progress.spinner.Tick
		}

		if key == "enter" && a.progress.done {
			if a.progress.err != nil {
				a, cmd = a.refreshVMList()
				return a, cmd
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
			a.status = newStatusModel(a.activeConfig, a.tunnelMgr.ActivePIDsForVM(a.activeConfig.VMName), "")
			a.screen = screenStatus
			return a, nil
		}
	}

	return a, cmd
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
			a.screen = screenVMList
			return a, nil
		}
	}

	form, cmd := a.confirmForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		a.confirmForm = f
	}

	if a.confirmForm.State == huh.StateCompleted {
		if *a.confirmValue {
			a.tunnelMgr.StopAll(a.activeConfig.VMName)
			a.bgRunning = true
			a.bgTitle = "Destroying VM..."
			a.progress = newProgressModel("Destroying VM...")
			a.screen = screenProgress
			return a, tea.Batch(a.progress.Init(), a.cmdDestroyVM())
		}
		a.screen = screenVMList
		return a, nil
	}

	return a, cmd
}

func (a App) viewConfirmDestroy() string {
	return titleStyle.Render("Destroy VM") + "\n\n" + a.confirmForm.View()
}

// --- Confirm Stop VM ---

func (a App) updateConfirmStopVM(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenVMList
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
			a.bgTitle = "Stopping tunnels and VM..."
			a.progress = newProgressModel("Stopping tunnels and VM...")
			a.screen = screenProgress
			return a, tea.Batch(a.progress.Init(), a.cmdStopVM())
		}

		a.status = newStatusModel(a.activeConfig, nil, msg)
		a.screen = screenStatus
		return a, nil
	}

	return a, cmd
}

func (a App) viewConfirmStopVM() string {
	return titleStyle.Render("Stop Tunnels") + "\n\n" + a.confirmForm.View()
}

// --- Confirm Stop All ---

func (a App) updateConfirmStopAll(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			a.screen = screenVMList
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
			a.bgTitle = "Stopping all tunnels and instances..."
			a.progress = newProgressModel("Stopping all tunnels and instances...")
			a.screen = screenProgress
			return a, tea.Batch(a.progress.Init(), a.cmdStopAll())
		}
		a.screen = screenVMList
		return a, nil
	}

	return a, cmd
}

func (a App) viewConfirmStopAll() string {
	return titleStyle.Render("Stop All") + "\n\n" + a.confirmForm.View()
}

// refreshVMList switches to the VM list screen and triggers a background
// reload while keeping the existing VM data and layout dimensions visible.
func (a App) refreshVMList() (App, tea.Cmd) {
	a.screen = screenVMList
	a.vmlist.loading = true
	return a, tea.Batch(loadVMList, a.vmlist.spinner.Tick)
}

// --- VM List ---

func (a App) updateVMList(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Intercept 'p' key before forwarding to vmlist model
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "p" {
		if a.bgRunning {
			a.screen = screenProgress
			return a, a.progress.spinner.Tick
		}
		if a.progress.done {
			a.screen = screenProgress
			return a, nil
		}
	}

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
			a.status = newStatusModel(msg.cfg, a.tunnelMgr.ActivePIDsForVM(msg.cfg.VMName), "")
			a.screen = screenStatus
			return a, nil
		}
		if msg.action == actionStopAll {
			return a.dispatchAction(actionStopAll)
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
	case tea.KeyMsg:
		_ = msg // handled by vmlist model
	}

	return a, cmd
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
	return func() tea.Msg {
		// Stop all tunnels for every VM
		for _, vm := range a.vmlist.vms {
			a.tunnelMgr.StopAll(vm.cfg.VMName)
			a.program.Send(logLineMsg(fmt.Sprintf("Stopped tunnels for %s", vm.cfg.VMName)))
		}

		// Stop all running instances
		for _, vm := range a.vmlist.vms {
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

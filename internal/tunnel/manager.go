package tunnel

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"vds-gcp-launch-instance/internal/config"
)

type Manager struct {
	mu        sync.Mutex
	processes map[string]map[string]*os.Process // vmName → (portKey → process)
}

func NewManager() *Manager {
	m := &Manager{
		processes: make(map[string]map[string]*os.Process),
	}
	m.recoverTunnels()
	return m
}

func (m *Manager) isAlive(proc *os.Process) bool {
	return isProcessAlive(proc)
}

func pidFilePath(vmName, local, remote string) string {
	return filepath.Join(config.ProjectDir(vmName), fmt.Sprintf("tunnel-%s-%s.pid", local, remote))
}

func writePIDFile(path string, pid int) {
	_ = os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

func removePIDFile(path string) {
	_ = os.Remove(path)
}

func isPortBound(localPort string) bool {
	ln, err := net.Listen("tcp", "127.0.0.1:"+localPort)
	if err != nil {
		return true // port is in use
	}
	ln.Close()
	return false
}

// recoverTunnels scans project directories for PID files and recovers live tunnels.
func (m *Manager) recoverTunnels() {
	for _, vmName := range config.ListProjects() {
		projDir := config.ProjectDir(vmName)
		entries, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "tunnel-") || !strings.HasSuffix(e.Name(), ".pid") {
				continue
			}

			pidPath := filepath.Join(projDir, e.Name())

			// Parse port pair from filename: tunnel-{local}-{remote}.pid
			name := strings.TrimPrefix(e.Name(), "tunnel-")
			name = strings.TrimSuffix(name, ".pid")
			parts := strings.SplitN(name, "-", 2)
			if len(parts) != 2 {
				removePIDFile(pidPath)
				continue
			}
			localPort, remotePort := parts[0], parts[1]

			data, err := os.ReadFile(pidPath)
			if err != nil {
				removePIDFile(pidPath)
				continue
			}

			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				removePIDFile(pidPath)
				continue
			}

			proc, err := os.FindProcess(pid)
			if err != nil || !isProcessAlive(proc) {
				removePIDFile(pidPath)
				continue
			}

			if !isPortBound(localPort) {
				removePIDFile(pidPath)
				continue
			}

			key := fmt.Sprintf("%s:%s", localPort, remotePort)
			if m.processes[vmName] == nil {
				m.processes[vmName] = make(map[string]*os.Process)
			}
			m.processes[vmName][key] = proc
		}
	}
}

// localPortOwner returns the VM name that owns a tunnel on the given local port, or empty string.
func (m *Manager) localPortOwner(localPort string) string {
	for vmName, tunnels := range m.processes {
		for key, proc := range tunnels {
			local := strings.SplitN(key, ":", 2)[0]
			if local == localPort && m.isAlive(proc) {
				return vmName
			}
		}
	}
	return ""
}

func (m *Manager) removeStalePID(vmName, key string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) == 2 {
		removePIDFile(pidFilePath(vmName, parts[0], parts[1]))
	}
}

func (m *Manager) StartTunnel(cfg config.Config, pp config.PortPair) error {
	m.mu.Lock()
	if owner := m.localPortOwner(pp.Local); owner != "" {
		m.mu.Unlock()
		if owner == cfg.VMName {
			return fmt.Errorf("port %s already tunneled for this VM", pp.Local)
		}
		return fmt.Errorf("port %s already in use by tunnel for %s", pp.Local, owner)
	}
	m.mu.Unlock()

	// Check if port is bound by an external process
	ln, err := net.Listen("tcp", "127.0.0.1:"+pp.Local)
	if err != nil {
		return fmt.Errorf("port %s already in use by another process", pp.Local)
	}
	ln.Close()

	args := []string{
		"compute", "ssh", cfg.VMName,
		"--project", cfg.ProjectID,
		"--zone", cfg.Zone,
		"--tunnel-through-iap",
		"--", "-L", fmt.Sprintf("%s:localhost:%s", pp.Local, pp.Remote),
		"-N",
	}

	cmd := exec.Command("gcloud", args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start tunnel %s:%s: %w", pp.Local, pp.Remote, err)
	}

	// Verify the tunnel process stays alive and binds the local port.
	// This catches immediate failures (auth errors, connection refused).
	const verifyChecks = 10
	const verifyInterval = 500 * time.Millisecond
	for i := 0; i < verifyChecks; i++ {
		time.Sleep(verifyInterval)
		if !isProcessAlive(cmd.Process) {
			go cmd.Wait() // reap
			return fmt.Errorf("tunnel %s:%s process exited prematurely", pp.Local, pp.Remote)
		}
		if isPortBound(pp.Local) {
			break
		}
	}

	// Reap the child process when it exits to prevent zombies.
	go cmd.Wait()

	key := fmt.Sprintf("%s:%s", pp.Local, pp.Remote)
	m.mu.Lock()
	if m.processes[cfg.VMName] == nil {
		m.processes[cfg.VMName] = make(map[string]*os.Process)
	}
	m.processes[cfg.VMName][key] = cmd.Process
	m.mu.Unlock()

	writePIDFile(pidFilePath(cfg.VMName, pp.Local, pp.Remote), cmd.Process.Pid)

	return nil
}

func (m *Manager) StartAll(cfg config.Config) []error {
	var errs []error
	for _, pp := range cfg.PortMappings() {
		if err := m.StartTunnel(cfg, pp); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (m *Manager) StopAll(vmName string) {
	m.mu.Lock()
	if tunnels, ok := m.processes[vmName]; ok {
		for key, proc := range tunnels {
			_ = killProcessTree(proc)
			m.removeStalePID(vmName, key)
			delete(tunnels, key)
		}
		delete(m.processes, vmName)
	}
	m.mu.Unlock()

	// Fallback: kill any lingering gcloud ssh processes for this VM
	killByName(vmName)
}

func (m *Manager) ActivePIDsForVM(vmName string) map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()

	tunnels := m.processes[vmName]
	pids := make(map[string]int, len(tunnels))
	for key, proc := range tunnels {
		if m.isAlive(proc) {
			pids[key] = proc.Pid
		} else {
			m.removeStalePID(vmName, key)
			delete(tunnels, key)
		}
	}
	if len(tunnels) == 0 {
		delete(m.processes, vmName)
	}
	return pids
}

func (m *Manager) TunnelCount(vmName string) int {
	return len(m.ActivePIDsForVM(vmName))
}

func (m *Manager) ActivePIDs() map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()

	pids := make(map[string]int)
	for vmName, tunnels := range m.processes {
		for key, proc := range tunnels {
			if m.isAlive(proc) {
				pids[key] = proc.Pid
			} else {
				m.removeStalePID(vmName, key)
				delete(tunnels, key)
			}
		}
		if len(tunnels) == 0 {
			delete(m.processes, vmName)
		}
	}
	return pids
}

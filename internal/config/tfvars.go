package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PortPair struct {
	Local  string
	Remote string
}

type Config struct {
	Username     string
	Password     string
	Timestamp    string
	ProjectID    string
	VMName       string
	Image        string
	Region       string
	Zone         string
	MachineType  string
	BootDiskSize string
	PortMapping  string
}

func (c Config) PortMappings() []PortPair {
	var pairs []PortPair
	for _, m := range strings.Split(c.PortMapping, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		parts := strings.SplitN(m, ":", 2)
		if len(parts) == 2 {
			pairs = append(pairs, PortPair{Local: parts[0], Remote: parts[1]})
		}
	}
	return pairs
}

func BaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".vmup")
}

func ProjectDir(vmName string) string {
	return filepath.Join(BaseDir(), "projects", vmName)
}

func LoadTFVars(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	kv := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"`)
		kv[key] = val
	}

	return Config{
		Username:     kv["username"],
		Password:     kv["password"],
		Timestamp:    kv["timestamp"],
		ProjectID:    kv["project-id"],
		VMName:       kv["vm-name"],
		Image:        kv["image"],
		Region:       kv["region"],
		Zone:         kv["zone"],
		MachineType:  kv["machine-type"],
		BootDiskSize: kv["boot-disk-size"],
		PortMapping:  kv["port-mapping"],
	}, scanner.Err()
}

func WriteTFVars(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	lines := []struct{ k, v string }{
		{"username", cfg.Username},
		{"password", cfg.Password},
		{"timestamp", cfg.Timestamp},
		{"project-id", cfg.ProjectID},
		{"vm-name", cfg.VMName},
		{"image", cfg.Image},
		{"region", cfg.Region},
		{"zone", cfg.Zone},
		{"machine-type", cfg.MachineType},
		{"boot-disk-size", cfg.BootDiskSize},
		{"port-mapping", cfg.PortMapping},
	}
	for _, l := range lines {
		if _, err := fmt.Fprintf(f, "%s=\"%s\"\n", l.k, l.v); err != nil {
			return err
		}
	}
	return nil
}

func ListProjects() []string {
	dir := filepath.Join(BaseDir(), "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			tfvars := filepath.Join(dir, e.Name(), "terraform.tfvars")
			if _, err := os.Stat(tfvars); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	return names
}

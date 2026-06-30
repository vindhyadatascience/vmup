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
	UserDomain   string // email domain for the IAP access grant (e.g. example.com)
	Password     string
	Timestamp    string
	ProjectID    string
	VMName       string
	Image        string
	ImageProject string // GCP project the selected image belongs to
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
	return filepath.Join(DataDir(), "projects", vmName)
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

	// Legacy fallback: projects created before the image-project field existed
	// have no "image-project" key; their images came from DefaultImageProject.
	imageProject := kv["image-project"]
	if imageProject == "" {
		imageProject = DefaultImageProject
	}

	return Config{
		Username:     kv["username"],
		UserDomain:   kv["user-domain"],
		Password:     kv["password"],
		Timestamp:    kv["timestamp"],
		ProjectID:    kv["project-id"],
		VMName:       kv["vm-name"],
		Image:        kv["image"],
		ImageProject: imageProject,
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
		{"user-domain", cfg.UserDomain},
		{"password", cfg.Password},
		{"timestamp", cfg.Timestamp},
		{"project-id", cfg.ProjectID},
		{"vm-name", cfg.VMName},
		{"image", cfg.Image},
		{"image-project", cfg.ImageProject},
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
	dir := filepath.Join(DataDir(), "projects")
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

// --- Data Disk Configuration ---

type DiskConfig struct {
	Name      string
	ProjectID string
	Zone      string
	DiskType  string // pd-standard, pd-balanced, pd-ssd
	SizeGB    string
	Formatted string // "true" if disk has been formatted
}

func DiskDir(name string) string {
	return filepath.Join(DataDir(), "disks", name)
}

func ListDisks() []string {
	dir := filepath.Join(DataDir(), "disks")
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

func LoadDiskTFVars(path string) (DiskConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return DiskConfig{}, err
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

	return DiskConfig{
		Name:      kv["disk-name"],
		ProjectID: kv["project-id"],
		Zone:      kv["zone"],
		DiskType:  kv["disk-type"],
		SizeGB:    kv["disk-size"],
		Formatted: kv["formatted"],
	}, scanner.Err()
}

func WriteDiskTFVars(path string, cfg DiskConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	lines := []struct{ k, v string }{
		{"disk-name", cfg.Name},
		{"project-id", cfg.ProjectID},
		{"zone", cfg.Zone},
		{"disk-type", cfg.DiskType},
		{"disk-size", cfg.SizeGB},
		{"formatted", cfg.Formatted},
	}
	for _, l := range lines {
		if _, err := fmt.Fprintf(f, "%s=\"%s\"\n", l.k, l.v); err != nil {
			return err
		}
	}
	return nil
}

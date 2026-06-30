package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Settings struct {
	DataDir string `json:"data_dir,omitempty"`

	// ImageProject is an optional GCP project whose images are listed first
	// (above the standard public images) in the VM-creation picker.
	//
	// Pointer semantics distinguish three states:
	//   nil          → never configured; fall back to DefaultImageProject
	//   pointer to "" → explicitly cleared; show only standard public images
	//   pointer to p  → list images from project p
	ImageProject *string `json:"image_project,omitempty"`
}

// EffectiveImageProject resolves the configured image project, applying the
// shipped default when the setting has never been configured. An empty result
// means "list only the standard public GCP images".
func (s Settings) EffectiveImageProject() string {
	if s.ImageProject == nil {
		return DefaultImageProject
	}
	return *s.ImageProject
}

var (
	cachedSettings   *Settings
	cachedSettingsMu sync.Mutex
)

func SettingsPath() string {
	return filepath.Join(BaseDir(), "settings.json")
}

func LoadSettings() Settings {
	cachedSettingsMu.Lock()
	defer cachedSettingsMu.Unlock()

	if cachedSettings != nil {
		return *cachedSettings
	}

	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		s := Settings{}
		cachedSettings = &s
		return s
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		s = Settings{}
		cachedSettings = &s
		return s
	}
	cachedSettings = &s
	return s
}

func SaveSettings(s Settings) error {
	if err := os.MkdirAll(BaseDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(SettingsPath(), data, 0o644); err != nil {
		return err
	}

	// Invalidate cache
	cachedSettingsMu.Lock()
	cachedSettings = nil
	cachedSettingsMu.Unlock()

	return nil
}

// DataDir returns the directory containing projects/ and disks/.
// If a custom data directory is configured, it returns that; otherwise BaseDir().
func DataDir() string {
	s := LoadSettings()
	if s.DataDir != "" {
		return s.DataDir
	}
	return BaseDir()
}

// CanonicalPath expands ~ and resolves a path to an absolute, clean form.
func CanonicalPath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(abs)
}

// ValidateDataDir checks whether newPath is a valid target for data migration.
// It returns an error listing any conflicting project or disk directory names.
// Both paths must already be canonicalized.
func ValidateDataDir(oldPath, newPath string) error {
	if oldPath == newPath {
		return fmt.Errorf("new path is the same as the current path")
	}

	// Reject nested paths (one is a subdirectory of the other)
	oldWithSep := oldPath + string(filepath.Separator)
	newWithSep := newPath + string(filepath.Separator)
	if strings.HasPrefix(newWithSep, oldWithSep) {
		return fmt.Errorf("new path cannot be inside the current data directory")
	}
	if strings.HasPrefix(oldWithSep, newWithSep) {
		return fmt.Errorf("new path cannot be a parent of the current data directory")
	}

	// Check target is accessible (don't create it yet — that's MigrateData's job)
	if info, err := os.Stat(newPath); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot access path: %w", err)
	}

	var conflicts []string

	// Check project name conflicts
	oldProjects := listSubdirs(filepath.Join(oldPath, "projects"))
	newProjects := listSubdirs(filepath.Join(newPath, "projects"))
	for _, name := range oldProjects {
		for _, existing := range newProjects {
			if name == existing {
				conflicts = append(conflicts, "project: "+name)
			}
		}
	}

	// Check disk name conflicts
	oldDisks := listSubdirs(filepath.Join(oldPath, "disks"))
	newDisks := listSubdirs(filepath.Join(newPath, "disks"))
	for _, name := range oldDisks {
		for _, existing := range newDisks {
			if name == existing {
				conflicts = append(conflicts, "disk: "+name)
			}
		}
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("conflicting names in target:\n  %s", strings.Join(conflicts, "\n  "))
	}

	return nil
}

// MigrateData moves projects/ and disks/ subdirectories from oldPath to newPath.
func MigrateData(oldPath, newPath string) error {
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		return fmt.Errorf("cannot create target directory: %w", err)
	}

	// Move individual project directories
	if err := moveSubdirs(filepath.Join(oldPath, "projects"), filepath.Join(newPath, "projects")); err != nil {
		return fmt.Errorf("moving projects: %w", err)
	}

	// Move individual disk directories
	if err := moveSubdirs(filepath.Join(oldPath, "disks"), filepath.Join(newPath, "disks")); err != nil {
		return fmt.Errorf("moving disks: %w", err)
	}

	// Clean up empty source dirs
	removeIfEmptyDir(filepath.Join(oldPath, "projects"))
	removeIfEmptyDir(filepath.Join(oldPath, "disks"))

	return nil
}

// CountDataItems returns the number of project and disk subdirectories at the given data path.
// Only counts directories that contain a terraform.tfvars file.
func CountDataItems(dataPath string) (projects, disks int) {
	projects = countTFVarsDirs(filepath.Join(dataPath, "projects"))
	disks = countTFVarsDirs(filepath.Join(dataPath, "disks"))
	return
}

func countTFVarsDirs(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			tfvars := filepath.Join(dir, e.Name(), "terraform.tfvars")
			if _, err := os.Stat(tfvars); err == nil {
				n++
			}
		}
	}
	return n
}

type ProjectSummary struct {
	Name        string
	ProjectID   string
	Zone        string
	MachineType string
	Image       string
}

type DiskSummary struct {
	Name     string
	ProjectID string
	Zone     string
	DiskType string
	SizeGB   string
}

// LoadProjectSummaries reads all project tfvars in the given data path.
func LoadProjectSummaries(dataPath string) []ProjectSummary {
	dir := filepath.Join(dataPath, "projects")
	names := listSubdirs(dir)
	var summaries []ProjectSummary
	for _, name := range names {
		tfvars := filepath.Join(dir, name, "terraform.tfvars")
		cfg, err := LoadTFVars(tfvars)
		if err != nil {
			continue
		}
		summaries = append(summaries, ProjectSummary{
			Name:        cfg.VMName,
			ProjectID:   cfg.ProjectID,
			Zone:        cfg.Zone,
			MachineType: cfg.MachineType,
			Image:       cfg.Image,
		})
	}
	return summaries
}

// LoadDiskSummaries reads all disk tfvars in the given data path.
func LoadDiskSummaries(dataPath string) []DiskSummary {
	dir := filepath.Join(dataPath, "disks")
	names := listSubdirs(dir)
	var summaries []DiskSummary
	for _, name := range names {
		tfvars := filepath.Join(dir, name, "terraform.tfvars")
		cfg, err := LoadDiskTFVars(tfvars)
		if err != nil {
			continue
		}
		summaries = append(summaries, DiskSummary{
			Name:      cfg.Name,
			ProjectID: cfg.ProjectID,
			Zone:      cfg.Zone,
			DiskType:  cfg.DiskType,
			SizeGB:    cfg.SizeGB,
		})
	}
	return summaries
}

func listSubdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func moveSubdirs(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		src := filepath.Join(srcDir, e.Name())
		dst := filepath.Join(dstDir, e.Name())
		if err := os.Rename(src, dst); err != nil {
			// Fallback for cross-device moves
			if cpErr := copyDirRecursive(src, dst); cpErr != nil {
				return fmt.Errorf("moving %s: %w", e.Name(), cpErr)
			}
			os.RemoveAll(src)
		}
	}
	return nil
}

func copyDirRecursive(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func removeIfEmptyDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		os.Remove(dir)
	}
}

package terraform

import (
	"context"
	"os"
	"path/filepath"

	goversion "github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"

	"github.com/vindhyadatascience/vmup/internal/config"
)

const terraformVersion = "1.12.1"

func BinDir() string {
	return filepath.Join(config.BaseDir(), "bin")
}

func EnsureInstalled(ctx context.Context) (string, error) {
	binDir := BinDir()
	execPath := filepath.Join(binDir, "terraform")

	if _, err := os.Stat(execPath); err == nil {
		return execPath, nil
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}

	v, err := goversion.NewVersion(terraformVersion)
	if err != nil {
		return "", err
	}

	installer := &releases.ExactVersion{
		Product:    product.Terraform,
		Version:    v,
		InstallDir: binDir,
	}

	return installer.Install(ctx)
}

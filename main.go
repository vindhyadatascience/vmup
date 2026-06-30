package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vindhyadatascience/vmup/internal/gcloud"
	"github.com/vindhyadatascience/vmup/internal/tui"
)

// version is the build version, injected at release time via -ldflags
// "-X main.version=...". It defaults to "dev" for local builds.
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Printf("vmup %s\n", version)
			return
		case "--help", "-h", "help":
			fmt.Println("vmup — a TUI for launching and managing GCP compute instances")
			fmt.Println("\nUsage:")
			fmt.Println("  vmup            Launch the interactive TUI")
			fmt.Println("  vmup --version  Print the version and exit")
			return
		}
	}

	if !gcloud.IsInstalled() {
		fmt.Fprintln(os.Stderr, "Error: gcloud CLI is required but not found in PATH.")
		fmt.Fprintln(os.Stderr, "Install it from: https://cloud.google.com/sdk/docs/install")
		os.Exit(1)
	}

	// Share the build version with the TUI so the title bar matches --version.
	tui.Version = version

	app := tui.NewApp(embeddedMainTF, embeddedDiskTF, embeddedDiskDeletableTF)

	p := tea.NewProgram(app, tea.WithAltScreen())
	app.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

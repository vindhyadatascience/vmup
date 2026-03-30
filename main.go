package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"vds-gcp-launch-instance/internal/gcloud"
	"vds-gcp-launch-instance/internal/tui"
)

func main() {
	if !gcloud.IsInstalled() {
		fmt.Fprintln(os.Stderr, "Error: gcloud CLI is required but not found in PATH.")
		fmt.Fprintln(os.Stderr, "Install it from: https://cloud.google.com/sdk/docs/install")
		os.Exit(1)
	}

	app := tui.NewApp(embeddedMainTF, embeddedDiskTF, embeddedDiskDeletableTF)

	p := tea.NewProgram(app, tea.WithAltScreen())
	app.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

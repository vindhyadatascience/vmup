package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vindhyadatascience/vmup/internal/gcloud"
	"github.com/vindhyadatascience/vmup/internal/selfupdate"
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
		case "update":
			// Deliberately handled before the gcloud check below: updating
			// vmup has nothing to do with gcloud, and someone whose gcloud
			// install is broken should still be able to upgrade.
			os.Exit(runUpdate(os.Args[2:]))
		case "--help", "-h", "help":
			fmt.Println("vmup — a TUI for launching and managing GCP compute instances")
			fmt.Println("\nUsage:")
			fmt.Println("  vmup                  Launch the interactive TUI")
			fmt.Println("  vmup update           Download and install the latest release")
			fmt.Println("  vmup update --check   Report whether an update is available")
			fmt.Println("  vmup --version        Print the version and exit")
			fmt.Println("\nEnvironment:")
			fmt.Println("  " + selfupdate.EnvDisable + "   Set to any value to disable update checks")
			return
		}
	}

	// Remove a binary displaced by a previous update. No-op except on Windows,
	// where the old file could not be deleted while the updating process was
	// still running.
	selfupdate.SweepLeftovers()

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

	_, err := p.Run()

	// Covers every exit path, including `q`, which bubbletea handles before
	// Update sees the QuitMsg. Called explicitly rather than deferred so the
	// os.Exit below cannot skip it.
	app.Cleanup()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runUpdate implements `vmup update` and returns the process exit code.
//
// Parsed by hand to match the existing os.Args switch rather than introducing
// a flag package for one subcommand with one flag.
func runUpdate(args []string) int {
	checkOnly := false
	for _, a := range args {
		switch a {
		case "--check":
			checkOnly = true
		case "-h", "--help":
			fmt.Println("Usage:")
			fmt.Println("  vmup update           Download and install the latest release")
			fmt.Println("  vmup update --check   Report whether an update is available")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "Unknown option %q for 'vmup update'.\n", a)
			return 2
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if checkOnly {
		res, err := selfupdate.Check(ctx, version, true)
		switch {
		case errors.Is(err, selfupdate.ErrDisabled):
			fmt.Printf("Update checks are disabled by %s.\n", selfupdate.EnvDisable)
			return 0
		case errors.Is(err, selfupdate.ErrNotARelease):
			fmt.Printf("vmup %s is a local build, so there is nothing to update to.\n", version)
			return 0
		case err != nil:
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		if !res.Available {
			fmt.Printf("vmup %s is up to date.\n", selfupdate.NormalizeVersion(version))
			return 0
		}
		fmt.Printf("vmup %s is available (you have %s).\n\nRun 'vmup update' to install it.\n",
			selfupdate.NormalizeVersion(res.Latest), selfupdate.NormalizeVersion(version))
		return 0
	}

	fmt.Println("Checking for updates...")
	res, err := selfupdate.Update(ctx, version)
	switch {
	case errors.Is(err, selfupdate.ErrUpToDate):
		fmt.Printf("vmup %s is already up to date.\n", selfupdate.NormalizeVersion(version))
		return 0
	case err != nil:
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Updated to vmup %s.\n", res.Version)
	return 0
}

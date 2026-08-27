package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vindhyadatascience/vmup/internal/selfupdate"
)

// updateAvailable holds the newer version discovered by the background check,
// or "" when there is none. It is package state for the same reason Version is:
// renderTitle needs it and is not a method on App.
var updateAvailable string

// updateCheckedMsg carries the result of the background update check.
type updateCheckedMsg struct {
	latest string
}

// checkForUpdate looks for a newer release without blocking startup.
//
// Errors are deliberately swallowed. A failed update check is not the user's
// problem and must never interrupt what they are doing — offline, behind a
// proxy, or rate limited all simply mean no badge. selfupdate caches the
// attempt either way, so a machine with no network retries daily rather than
// on every launch.
func checkForUpdate(current string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		res, err := selfupdate.Check(ctx, current, false)
		if err != nil || !res.Available {
			return updateCheckedMsg{}
		}
		return updateCheckedMsg{latest: selfupdate.NormalizeVersion(res.Latest)}
	}
}

// renderUpdateInfo is the screen shown when the user asks about the update.
//
// It tells them what to run rather than running it here. Replacing the binary
// would not change the process already executing, so an in-TUI install would
// still end in "now restart vmup" — and it would put a network download and a
// binary swap inside the screen state machine to save typing one command.
func renderUpdateInfo(width int) string {
	var b strings.Builder

	cmd := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))

	b.WriteString(titleStyle.Render("Update available"))
	b.WriteString("\n\n")

	if updateAvailable == "" {
		b.WriteString(fmt.Sprintf("vmup %s is up to date.\n", selfupdate.NormalizeVersion(Version)))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("esc  back"))
		return b.String()
	}

	b.WriteString(fmt.Sprintf("vmup %s is available. You have %s.\n\n",
		updateAvailable, selfupdate.NormalizeVersion(Version)))

	in, err := selfupdate.DetectInstall()
	switch {
	case err != nil:
		b.WriteString("Install it by re-running the installer:\n\n")
		b.WriteString("    " + cmd.Render(selfupdate.InstallerHint()) + "\n")
	case in.Kind == selfupdate.KindManaged:
		b.WriteString(fmt.Sprintf("vmup was installed with %s. Update it with:\n\n", in.Manager))
		b.WriteString("    " + cmd.Render(in.UpgradeCmd) + "\n")
	case in.Kind == selfupdate.KindNotWritable:
		b.WriteString(fmt.Sprintf("%s is not writable, so vmup cannot replace itself.\n", in.Dir))
		b.WriteString("Reinstall with:\n\n")
		b.WriteString("    " + cmd.Render(selfupdate.InstallerHint()) + "\n")
	default:
		b.WriteString("Quit vmup and run:\n\n")
		b.WriteString("    " + cmd.Render("vmup update") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("esc  back"))
	return b.String()
}

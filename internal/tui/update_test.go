package tui

import (
	"strings"
	"testing"
)

// stripANSI removes the colour codes lipgloss emits so assertions can look at
// the text itself.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestTitleShowsUpdateBadgeOnlyWhenOneIsAvailable(t *testing.T) {
	original := updateAvailable
	t.Cleanup(func() { updateAvailable = original })

	updateAvailable = ""
	if got := stripANSI(renderTitle(0)); strings.Contains(got, "available") {
		t.Errorf("title advertised an update when none was found: %q", got)
	}

	updateAvailable = "1.9.1"
	got := stripANSI(renderTitle(0))
	if !strings.Contains(got, "1.9.1 available") {
		t.Errorf("title did not show the badge: %q", got)
	}
	// The key that opens the detail screen has to be discoverable from the
	// badge, since that is the only thing pointing at it.
	if !strings.Contains(got, "(u)") {
		t.Errorf("badge did not name the key: %q", got)
	}
}

func TestUpdateInfoReportsUpToDate(t *testing.T) {
	original := updateAvailable
	t.Cleanup(func() { updateAvailable = original })

	updateAvailable = ""
	got := stripANSI(renderUpdateInfo(80))
	if !strings.Contains(got, "up to date") {
		t.Errorf("expected an up-to-date message, got %q", got)
	}
}

// The palette entry appears only when there is something to update to,
// matching the badge.
func TestPaletteUpdateEntryIsConditional(t *testing.T) {
	original := updateAvailable
	t.Cleanup(func() { updateAvailable = original })

	hasUpdate := func(cmds []paletteCommand) bool {
		for _, c := range cmds {
			if c.name == "update" {
				return true
			}
		}
		return false
	}

	updateAvailable = ""
	if hasUpdate(vmPaletteCommands(nil, 0, false, false)) {
		t.Error("instances palette offered update with none available")
	}
	if hasUpdate(diskPaletteCommands(nil, 0, false, false)) {
		t.Error("disks palette offered update with none available")
	}

	updateAvailable = "1.9.1"
	if !hasUpdate(vmPaletteCommands(nil, 0, false, false)) {
		t.Error("instances palette missing the update command")
	}
	// The Utility block is duplicated across both builders, so it is easy to
	// add a command to one and forget the other.
	if !hasUpdate(diskPaletteCommands(nil, 0, false, false)) {
		t.Error("disks palette missing the update command")
	}
}

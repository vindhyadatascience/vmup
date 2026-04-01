package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func renderTabBar(active tab, width int, lastRefreshed time.Time) string {
	tabs := []struct {
		label string
		key   string
		t     tab
	}{
		{"Instances", "1", tabInstances},
		{"Data Disks", "2", tabDataDisks},
	}

	var parts []string
	for _, t := range tabs {
		label := fmt.Sprintf(" %s %s ", t.key, t.label)
		if t.t == active {
			parts = append(parts, activeTabStyle.Render(label))
		} else {
			parts = append(parts, inactiveTabStyle.Render(label))
		}
	}

	tabStr := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	// Right-align the last refreshed timestamp
	if !lastRefreshed.IsZero() && width > 0 {
		ts := dimStyle.Render("refreshed " + lastRefreshed.Format("3:04:05 PM"))
		tabWidth := lipgloss.Width(tabStr)
		tsWidth := lipgloss.Width(ts)
		gap := width - tabWidth - tsWidth
		if gap > 0 {
			return tabStr + strings.Repeat(" ", gap) + ts
		}
	}

	return tabStr
}

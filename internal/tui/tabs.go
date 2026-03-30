package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func renderTabBar(active tab) string {
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

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

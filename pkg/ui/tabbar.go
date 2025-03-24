package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TabBar is a component that renders a tab bar
type TabBar struct {
	Tabs      []string
	ActiveTab int
	width     int
}

// NewTabBar creates a new tab bar
func NewTabBar(tabs []string) *TabBar {
	return &TabBar{
		Tabs:      tabs,
		ActiveTab: 0,
	}
}

// SetWidth sets the width of the tab bar
func (t *TabBar) SetWidth(width int) {
	t.width = width
}

// Render renders the tab bar
func (t *TabBar) Render() string {
	if len(t.Tabs) == 0 {
		return ""
	}

	// Render tabs
	renderedTabs := make([]string, len(t.Tabs))

	for i, tab := range t.Tabs {
		if i == t.ActiveTab {
			renderedTabs[i] = Theme.TabActive.Render(tab)
		} else {
			renderedTabs[i] = Theme.TabInactive.Render(tab)
		}
	}

	// Join tabs and wrap to width
	tabsBar := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	// Add a line below tabs
	divider := strings.Repeat("─", t.width)

	return lipgloss.JoinVertical(lipgloss.Left, tabsBar, divider)
}

// HandleKeyPress handles key presses for the tab bar
func (t *TabBar) HandleKeyPress(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Number keys for tab navigation (1-9)
		num := int(msg.Runes[0] - '1')
		if num >= 0 && num < len(t.Tabs) {
			t.ActiveTab = num
			return true, nil
		}
	case "tab", "right":
		// Tab/right arrow to cycle forward through tabs
		t.ActiveTab = (t.ActiveTab + 1) % len(t.Tabs)
		return true, nil
	case "shift+tab", "left":
		// Shift+tab/left arrow to cycle backward through tabs
		t.ActiveTab = (t.ActiveTab - 1 + len(t.Tabs)) % len(t.Tabs)
		return true, nil
	}

	return false, nil
}

// GetActiveTab returns the name of the currently active tab
func (t *TabBar) GetActiveTab() string {
	if t.ActiveTab >= 0 && t.ActiveTab < len(t.Tabs) {
		return t.Tabs[t.ActiveTab]
	}
	return ""
}

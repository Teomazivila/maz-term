package tests

import (
	"testing"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/ui"
	"github.com/Teomazivila/maz-term/tests/helpers"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewTabBar(t *testing.T) {
	// Create a tab bar with two tabs
	tabs := []string{"Tab 1", "Tab 2"}
	tabBar := ui.NewTabBar(tabs)

	// Assert that the tab bar has the correct tabs
	assert.Equal(t, tabs, tabBar.Tabs)

	// Assert that the active tab is the first one
	assert.Equal(t, 0, tabBar.ActiveTab)

	// Test tab switching
	tabBar.SetWidth(100)

	// We can't properly test key handling here, as it depends on Bubble Tea's
	// internal implementation. We'll just verify the Tab bar was created with
	// correct initial state.
	assert.Equal(t, "Tab 1", tabBar.GetActiveTab())
}

func TestStatusBar(t *testing.T) {
	// Create a status bar
	statusBar := ui.NewStatusBar(false)
	statusBar.SetWidth(100)

	// Add an item
	statusBar.AddItem("Test", "Value", ui.Theme.Good)

	// Update the item
	statusBar.UpdateItem("Test", "Updated")

	// Update with style
	statusBar.UpdateItemWithStyle("Test", "Warning", ui.Theme.Warning)

	// Add a new item via update
	statusBar.UpdateItem("New", "Item")

	// Remove an item
	statusBar.RemoveItem("New")

	// Render test (just ensure it doesn't crash)
	result := statusBar.Render()
	assert.NotEmpty(t, result)
}

func TestTheme(t *testing.T) {
	// Test default theme
	ui.SetTheme("dark")

	// Test light theme
	ui.SetTheme("light")

	// Test high contrast theme
	ui.SetTheme("high-contrast")

	// Test unknown theme (should default to dark)
	ui.SetTheme("unknown")
}

func TestPlaceholderPanel(t *testing.T) {
	// Create a placeholder panel
	panel := ui.NewPlaceholderPanel("Test Panel")

	// Test title
	assert.Equal(t, "Test Panel", panel.Title())

	// Test rendering
	result := panel.Render(20, 10)
	assert.NotEmpty(t, result)

	// Test update (should not crash)
	panel.Update()
}

func TestLayoutManager(t *testing.T) {
	// Create a layout
	layout := ui.NewLayout(1)

	// Set size
	layout.SetSize(100, 50)

	// Add rows with panels
	panel1 := ui.NewPlaceholderPanel("Panel 1")
	panel2 := ui.NewPlaceholderPanel("Panel 2")
	panel3 := ui.NewPlaceholderPanel("Panel 3")

	layout.AddRow(1, panel1)
	layout.AddRow(2, panel2, panel3)

	// Test rendering
	result := layout.Render()
	assert.NotEmpty(t, result)

	// Test update
	layout.Update()
}

func TestKeyMap(t *testing.T) {
	// Create a keymap
	keyMap := ui.NewKeyMap()

	// Add global binding
	keyMap.AddGlobalBinding("g", "Global Action", func() tea.Cmd {
		return nil
	})

	// Add tab binding
	keyMap.AddTabBinding("Tab1", "t", "Tab Action", func() tea.Cmd {
		return nil
	})

	// Check that bindings were added correctly
	globalBindings := keyMap.GetGlobalBindings()
	assert.Equal(t, 1, len(globalBindings))
	assert.Equal(t, "g", globalBindings[0].Key)
	assert.Equal(t, "Global Action", globalBindings[0].Description)

	tabBindings := keyMap.GetTabBindings("Tab1")
	assert.Equal(t, 1, len(tabBindings))
	assert.Equal(t, "t", tabBindings[0].Key)
	assert.Equal(t, "Tab Action", tabBindings[0].Description)

	// Test help view rendering
	helpView := keyMap.RenderHelpView(100, 50, "Tab1")
	assert.NotEmpty(t, helpView)
}

func TestDashboardTab(t *testing.T) {
	// Create test config for the dashboard tab
	cfg := config.LayoutTab{
		Name: "Test Dashboard",
		Rows: []config.LayoutRow{
			{
				Size:   1,
				Panels: []string{"panel1", "panel2"},
			},
			{
				Size:   2,
				Panels: []string{"panel3"},
			},
		},
	}

	// Create a dashboard tab
	dashTab := ui.NewDashboardTab(cfg)

	// Test title
	assert.Equal(t, "Test Dashboard", dashTab.Title())

	// Test size setting
	dashTab.SetSize(100, 50)

	// Test update (just ensure it doesn't crash)
	_, _ = dashTab.Update(helpers.CreateKeyMsg("x"))

	// Test view (just ensure it doesn't crash)
	view := dashTab.View()
	assert.NotEmpty(t, view)
}

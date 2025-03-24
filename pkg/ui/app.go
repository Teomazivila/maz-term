package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// App is the main application model
type App struct {
	config    interface{} // Will be replaced with actual config type
	tabs      []Tab
	activeTab int
	width     int
	height    int
}

// Tab represents a tab in the UI
type Tab interface {
	Title() string
	Update(msg tea.Msg) (Tab, tea.Cmd)
	View() string
	SetSize(width, height int)
}

// NewApp creates a new application model
func NewApp() *App {
	return &App{
		tabs:      []Tab{},
		activeTab: 0,
	}
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	// This will be implemented in Phase 2
	return nil
}

// Update handles application updates
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return a, tea.Quit
		}
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Resize all tabs
		for i := range a.tabs {
			if tab, ok := a.tabs[i].(Tab); ok {
				tab.SetSize(msg.Width, msg.Height)
			}
		}
	}

	// If we have active tabs, pass the message to the active tab
	if len(a.tabs) > 0 {
		var cmd tea.Cmd
		a.tabs[a.activeTab], cmd = a.tabs[a.activeTab].Update(msg)
		return a, cmd
	}

	return a, nil
}

// View renders the application UI
func (a *App) View() string {
	// If no tabs, render a welcome screen
	if len(a.tabs) == 0 {
		return "Welcome to DevOps Terminal Dashboard!\nNo tabs are currently loaded."
	}

	// Render the active tab
	return a.tabs[a.activeTab].View()
}

// AddTab adds a new tab to the application
func (a *App) AddTab(tab Tab) {
	tab.SetSize(a.width, a.height)
	a.tabs = append(a.tabs, tab)
}

// SetActiveTab sets the active tab
func (a *App) SetActiveTab(index int) {
	if index >= 0 && index < len(a.tabs) {
		a.activeTab = index
	}
}

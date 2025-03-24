package ui

import (
	"fmt"

	"github.com/Teomazivila/maz-term/pkg/config"
	tea "github.com/charmbracelet/bubbletea"
)

// App is the main application model
type App struct {
	config    *config.Config
	tabs      []Tab
	activeTab int
	width     int
	height    int
	tabBar    *TabBar
	statusBar *StatusBar
	keyMap    KeyMap
	showHelp  bool
}

// Tab represents a tab in the UI
type Tab interface {
	Title() string
	Update(msg tea.Msg) (Tab, tea.Cmd)
	View() string
	SetSize(width, height int)
}

// CloseableTab represents a tab that can be closed
type CloseableTab interface {
	Tab
	Close()
}

// NewApp creates a new application model
func NewApp() *App {
	return &App{
		tabs:      []Tab{},
		activeTab: 0,
		showHelp:  false,
		keyMap:    NewKeyMap(),
	}
}

// SetConfig sets the application configuration
func (a *App) SetConfig(cfg *config.Config) {
	a.config = cfg

	// Set theme
	SetTheme(cfg.General.Theme)

	// Create tabs from configuration
	tabNames := []string{}
	for _, tab := range cfg.Layout {
		// Create a new dashboard tab
		dashTab := NewDashboardTab(tab)
		dashTab.SetSize(a.width, a.height-4) // Reserve space for tab bar and status bar

		// Add the tab
		a.tabs = append(a.tabs, dashTab)
		tabNames = append(tabNames, tab.Name)
	}

	// Create the tab bar
	a.tabBar = NewTabBar(tabNames)
	a.tabBar.SetWidth(a.width)

	// Create the status bar
	a.statusBar = NewStatusBar(true)
	a.statusBar.SetWidth(a.width)
	a.statusBar.AddItem("Status", "Ready", Theme.Good)

	// Set up keybindings
	a.setupKeyBindings()
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	// Load configuration
	cfg, err := config.LoadConfig("")
	if err != nil {
		// Log the error - in a real app, we'd handle this better
		fmt.Println("Error loading config:", err)

		// Create a default configuration manually since LoadConfig() had an error
		cfg = DefaultConfig()
	}

	// Set the configuration
	a.SetConfig(cfg)

	return nil
}

// Update handles application updates
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if a.showHelp {
			// When help is shown, any key dismisses it
			a.showHelp = false
			return a, nil
		}

		// First, check if tab bar handles the key
		if handled, cmd := a.tabBar.HandleKeyPress(msg); handled {
			a.activeTab = a.tabBar.ActiveTab
			return a, cmd
		}

		// Second, check if keybindings handle the key
		if handled, cmd := a.keyMap.HandleKeyPress(msg, a.tabBar.GetActiveTab()); handled {
			return a, cmd
		}

		// Global key handling
		switch msg.String() {
		case "ctrl+c", "q":
			return a, tea.Quit
		case "?":
			a.showHelp = true
			return a, nil
		}
	case tea.WindowSizeMsg:
		// Debounce window resize events by queueing a single resize operation
		a.width = msg.Width
		a.height = msg.Height

		var cmds []tea.Cmd

		// Update component sizes
		if a.tabBar != nil {
			a.tabBar.SetWidth(msg.Width)
		}
		if a.statusBar != nil {
			a.statusBar.SetWidth(msg.Width)
		}

		// Calculate the available height for tabs
		availableHeight := msg.Height
		if a.tabBar != nil {
			availableHeight -= 2 // Tab bar height
		}
		if a.statusBar != nil {
			availableHeight -= 2 // Status bar height
		}

		// Ensure minimum height
		availableHeight = MaxInt(availableHeight, 20)

		// Resize all tabs with the new available height
		for i := range a.tabs {
			if tab, ok := a.tabs[i].(Tab); ok {
				tab.SetSize(msg.Width, availableHeight)
			}
		}

		// Forward window resize to active tab
		if len(a.tabs) > 0 {
			if tab, ok := a.tabs[a.activeTab].(tea.Model); ok {
				var cmd tea.Cmd
				_, cmd = tab.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

		return a, tea.Batch(cmds...)
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
	// If showing help, render the help view
	if a.showHelp {
		return a.keyMap.RenderHelpView(a.width, a.height, a.tabBar.GetActiveTab())
	}

	// If no tabs, render a welcome screen
	if len(a.tabs) == 0 {
		return "Welcome to DevOps Terminal Dashboard!\nNo tabs are currently loaded."
	}

	// Render the tab bar
	tabBarView := a.tabBar.Render()

	// Render the active tab
	tabView := a.tabs[a.activeTab].View()

	// Render the status bar
	statusBarView := a.statusBar.Render()

	// Combine all views
	return tabBarView + "\n" + tabView + "\n" + statusBarView
}

// Close cleans up resources and closes all tabs
func (a *App) Close() {
	// Close all tabs that implement CloseableTab
	for _, tab := range a.tabs {
		if closeableTab, ok := tab.(CloseableTab); ok {
			closeableTab.Close()
		}
	}
}

// setupKeyBindings sets up the keyboard shortcuts
func (a *App) setupKeyBindings() {
	// Global keybindings
	a.keyMap.AddGlobalBinding("q", "Quit", func() tea.Cmd {
		return tea.Quit
	})

	a.keyMap.AddGlobalBinding("?", "Show Help", func() tea.Cmd {
		return nil
	})

	a.keyMap.AddGlobalBinding("r", "Refresh Data", func() tea.Cmd {
		a.statusBar.UpdateItemWithStyle("Status", "Refreshing...", Theme.Warning)
		return nil
	})

	// Add more keybindings as needed
}

func DefaultConfig() *config.Config {
	return &config.Config{
		Layout: []config.LayoutTab{
			{
				Name:   "System",
				Panels: []string{"system"},
			},
			{
				Name:   "HTTP",
				Panels: []string{"http"},
			},
			{
				Name:   "Git",
				Panels: []string{"git"},
			},
		},
	}
}

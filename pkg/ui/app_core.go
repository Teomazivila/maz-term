package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/Teomazivila/maz-term/pkg/plugins"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// NewApp creates a new terminal UI application
func NewApp(cfg *config.Config) *App {
	app := &App{
		Config:          cfg,
		ActiveTabIndex:  0,
		Tabs:            []*Tab{},
		HistoryRange:    24 * time.Hour,             // Default to 24 hours
		HistoryRangeIdx: 3,                          // Default index for 24 hours
		ShowAnnotations: true,                       // Show annotations by default
		PluginManager:   plugins.NewPluginManager(), // Initialize plugin manager
	}

	// Create tabs from configuration
	for _, tab := range cfg.Layout {
		// Create a new dashboard tab
		app.Tabs = append(app.Tabs, NewTab(tab))
	}

	// Add History tab if not already included
	hasHistoryTab := false
	for _, tab := range app.Tabs {
		if tab.Name == "History" {
			hasHistoryTab = true
			break
		}
	}

	if !hasHistoryTab {
		// Add history tab
		app.Tabs = append(app.Tabs, NewTab(config.LayoutTab{
			Name:   "History",
			Panels: []string{"history"},
		}))
	}

	// Add Notifications tab if not already included
	hasNotificationsTab := false
	for _, tab := range app.Tabs {
		if tab.Name == "Notifications" {
			hasNotificationsTab = true
			break
		}
	}

	if !hasNotificationsTab {
		// Add notifications tab
		app.Tabs = append(app.Tabs, NewTab(config.LayoutTab{
			Name:   "Notifications",
			Panels: []string{"notifications"},
		}))
	}

	// Add Plugins tab if not already included
	hasPluginsTab := false
	for _, tab := range app.Tabs {
		if tab.Name == "Plugins" {
			hasPluginsTab = true
			break
		}
	}

	if !hasPluginsTab {
		// Add plugins tab
		app.Tabs = append(app.Tabs, NewTab(config.LayoutTab{
			Name:   "Plugins",
			Panels: []string{"plugins"},
		}))
	}

	return app
}

// NewTab creates a new terminal UI tab
func NewTab(cfg config.LayoutTab) *Tab {
	return &Tab{
		Name:       cfg.Name,
		Widgets:    []ui.Drawable{},
		Panels:     []*widgets.Paragraph{},
		Gauges:     []*widgets.Gauge{},
		Tables:     []*widgets.Table{},
		Sparklines: []*widgets.SparklineGroup{},
		BarCharts:  []*widgets.BarChart{},
		Plots:      []*widgets.Plot{},
		Lists:      []*widgets.List{},
		HasUnread:  false,
	}
}

// Run starts the terminal UI application
func (a *App) Run() error {
	// Initialize termui
	if err := ui.Init(); err != nil {
		return fmt.Errorf("failed to initialize termui: %w", err)
	}
	defer ui.Close()

	// Initialize collectors
	a.initCollectors()

	// Create UI elements before trying to access them
	a.createUI()

	// Load plugins
	a.loadPlugins()

	// Set up event handling
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Render the initial UI
	a.updateData()
	a.updateLayout()

	a.Running = true
	for a.Running {
		select {
		case e := <-uiEvents:
			a.handleEvent(e)
		case <-ticker.C:
			a.updateData()
			a.updateLayout()
		}
	}

	// Stop the plugin watcher
	a.PluginManager.StopWatcher()

	// Clean up plugins
	a.PluginManager.ShutdownAll()

	return nil
}

// initCollectors initializes data collectors
func (a *App) initCollectors() {
	appCtx := context.Background()

	// System metrics collector
	a.SystemCollector = collector.NewSystemMetricsCollector()
	go a.SystemCollector.Start(appCtx, 2*time.Second)

	// HTTP health checker
	// Use endpoints from configuration if available
	var endpoints []models.EndpointConfig
	if a.Config != nil && len(a.Config.Endpoints) > 0 {
		endpoints = a.Config.Endpoints
	} else {
		// Fallback to defaults
		endpoints = []models.EndpointConfig{
			{Name: "Google", URL: "https://www.google.com", Method: "GET"},
			{Name: "GitHub", URL: "https://github.com", Method: "GET"},
			{Name: "Example", URL: "https://example.com", Method: "GET"},
		}
	}
	a.HTTPCollector = collector.NewHTTPHealthChecker(endpoints)
	go a.HTTPCollector.Start(appCtx, 5*time.Second)

	// Git status collector
	// Use git repository path from configuration if available
	gitPath := ""
	if a.Config != nil && a.Config.Git.Repositories != nil && len(a.Config.Git.Repositories) > 0 {
		gitPath = a.Config.Git.Repositories[0].Path
	}
	a.GitCollector = collector.NewGitStatusCollector(gitPath)
	go a.GitCollector.Start(appCtx, 5*time.Second)
}

// SetStorageProvider sets the storage provider for all collectors
func (a *App) SetStorageProvider(provider collector.StorageProvider) {
	if a.SystemCollector != nil {
		a.SystemCollector.SetStorageProvider(provider)
	}

	if a.HTTPCollector != nil {
		a.HTTPCollector.SetStorageProvider(provider)
	}

	if a.GitCollector != nil {
		a.GitCollector.SetStorageProvider(provider)
	}

	// Store the StorageInterface for the UI if it implements it
	if storageInterface, ok := provider.(StorageInterface); ok {
		a.Storage = storageInterface
	}
}

// StartApp starts the terminal UI application
func StartApp(cfg *config.Config, storageProvider collector.StorageProvider) error {
	app := NewApp(cfg)

	// Set the storage provider if available
	if storageProvider != nil {
		app.SetStorageProvider(storageProvider)
	}

	return app.Run()
}

// getTabNames extracts tab names from tab array

// getTabByName returns a tab by its name
func (a *App) getTabByName(name string) *Tab {
	for _, tab := range a.Tabs {
		if tab.Name == name {
			return tab
		}
	}
	return nil
}

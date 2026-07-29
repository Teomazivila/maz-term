package ui

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/Teomazivila/maz-term/pkg/plugins"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

const (
	// refreshInterval is how often the dashboard redraws.
	refreshInterval = time.Second

	// statusBarHeight and tabBarHeight are the rows reserved at the bottom of
	// the screen; the remainder is the tab content area. Both bars are drawn
	// without a border, so one row each is enough to show their text: a bordered
	// widget needs three rows before any content is visible.
	statusBarHeight = 1
	tabBarHeight    = 1

	// minContentHeight is the smallest content area worth laying out. A bordered
	// widget needs three rows to show anything between its borders.
	minContentHeight = 3
	minContentWidth  = 20
)

// requiredTabs are always present, in addition to whatever the configuration
// defines.
var requiredTabs = []config.LayoutTab{
	{Name: "System", Panels: []string{"system"}},
	{Name: "HTTP", Panels: []string{"http"}},
	{Name: "Git", Panels: []string{"git"}},
	{Name: "History", Panels: []string{"history"}},
	{Name: "Notifications", Panels: []string{"notifications"}},
	{Name: "Plugins", Panels: []string{"plugins"}},
}

// NewApp creates a new terminal UI application.
func NewApp(cfg *config.Config) *App {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	app := &App{
		Config:          cfg,
		HistoryRange:    historyRanges[defaultHistoryRangeIndex].Value,
		HistoryRangeIdx: defaultHistoryRangeIndex,
		ShowAnnotations: true,
		PluginManager:   plugins.NewPluginManager(),
		filter: filterDraft{
			Sources:    make(map[string]bool),
			Severities: make(map[string]bool),
		},
	}

	for _, tab := range cfg.Layout {
		app.Tabs = append(app.Tabs, NewTab(tab))
	}

	// The dashboard's own tabs are appended when the configuration does not
	// already provide them, so a custom layout cannot hide core views.
	for _, required := range requiredTabs {
		if app.getTabByName(required.Name) == nil {
			app.Tabs = append(app.Tabs, NewTab(required))
		}
	}

	// Provider tabs exist only when that provider is configured. An
	// unconfigured provider has no tab rather than an empty one claiming a
	// connection it does not have.
	for _, provider := range []struct {
		name    string
		enabled bool
	}{
		{"Cloud", awsEnabled(cfg)},
		{"Kubernetes", cfg.Kubernetes.Enabled},
		{"CI/CD", githubEnabled(cfg)},
	} {
		if provider.enabled && app.getTabByName(provider.name) == nil {
			app.Tabs = append(app.Tabs, NewTab(config.LayoutTab{Name: provider.name}))
		}
	}

	return app
}

// awsEnabled reports whether the configuration asks for AWS collection.
func awsEnabled(cfg *config.Config) bool {
	for _, provider := range cfg.Cloud.Enabled {
		if strings.EqualFold(strings.TrimSpace(provider), "aws") {
			return true
		}
	}
	return false
}

// githubEnabled reports whether the configuration asks for GitHub Actions
// collection.
func githubEnabled(cfg *config.Config) bool {
	for _, provider := range cfg.CICD.Enabled {
		if strings.EqualFold(strings.TrimSpace(provider), "github") {
			return true
		}
	}
	return false
}

// NewTab creates a tab from its layout configuration.
func NewTab(cfg config.LayoutTab) *Tab {
	return &Tab{Name: cfg.Name}
}

// Run initialises the terminal, starts the collectors and drives the event loop
// until the operator quits or ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	if err := ui.Init(); err != nil {
		return fmt.Errorf("failed to initialize termui: %w", err)
	}
	defer ui.Close()

	// Collectors are created here, with the run context, so cancelling it stops
	// them. They previously used context.Background and outlived shutdown,
	// writing to a database that had already been closed.
	a.initCollectors(ctx)
	defer a.stopCollectors()

	a.createUI()
	a.loadPlugins()
	defer a.shutdownPlugins()

	a.loadNotifications()

	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	// Draw once before waiting for the first tick so the dashboard is populated
	// immediately rather than after a blank second.
	a.updateData()
	a.render()

	for {
		select {
		case <-ctx.Done():
			return nil

		case event := <-uiEvents:
			a.handleEvent(event)
			if a.quit {
				return nil
			}
			// Redraw immediately so input feels responsive instead of waiting
			// for the next tick.
			a.render()

		case <-ticker.C:
			a.updateData()
			a.render()
		}
	}
}

// initCollectors creates and starts the MVP collectors.
func (a *App) initCollectors(ctx context.Context) {
	logger := slog.Default()
	interval := a.Config.General.RefreshInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}

	a.SystemCollector = collector.NewSystemMetricsCollector()
	a.HTTPCollector = collector.NewHTTPHealthChecker(a.Config.Endpoints)

	gitPath := ""
	if len(a.Config.Git.Repositories) > 0 {
		gitPath = a.Config.Git.Repositories[0].Path
	}
	a.GitCollector = collector.NewGitStatusCollector(gitPath)

	started := []collector.Collector{a.SystemCollector, a.HTTPCollector, a.GitCollector}
	intervals := []time.Duration{interval, interval, interval}

	// Infrastructure providers poll remote APIs, so they default to their own
	// slower intervals to stay within rate limits and cost budgets.
	if awsEnabled(a.Config) {
		a.CloudCollector = collector.NewAWSCollector(collector.AWSConfig{
			Region:            a.Config.Cloud.AWS.Region,
			Profile:           a.Config.Cloud.AWS.Profile,
			AdditionalRegions: a.Config.Cloud.AWS.AdditionalRegions,
			Resources:         a.Config.Cloud.AWS.Resources,
		})
		started = append(started, a.CloudCollector)
		intervals = append(intervals, orDefault(a.Config.Cloud.Refresh, time.Minute))
	}

	if a.Config.Kubernetes.Enabled {
		a.KubernetesCollector = collector.NewKubernetesCollector(collector.KubernetesConfig{
			ConfigPath: a.Config.Kubernetes.ConfigPath,
			Context:    a.Config.Kubernetes.Context,
			Namespaces: a.Config.Kubernetes.Namespaces,
			Resources:  a.Config.Kubernetes.Resources,
		})
		started = append(started, a.KubernetesCollector)
		intervals = append(intervals, orDefault(a.Config.Kubernetes.Refresh, 30*time.Second))
	}

	if githubEnabled(a.Config) {
		a.CICDCollector = collector.NewGitHubActionsCollector(collector.GitHubConfig{
			Owner:        a.Config.CICD.GitHub.Owner,
			Repositories: a.Config.CICD.GitHub.Repositories,
			Workflows:    a.Config.CICD.GitHub.Workflows,
			// Read from the environment, never from the configuration file.
			Token: config.GitHubToken(),
		})
		started = append(started, a.CICDCollector)
		intervals = append(intervals, orDefault(a.Config.CICD.Refresh, 2*time.Minute))
	}

	// Apply the store recorded by SetStorageProvider before wiring up, so the
	// very first sample is persisted.
	a.applyStorageProvider()

	for i, c := range started {
		if err := c.Start(ctx, intervals[i]); err != nil {
			logger.Error("failed to start collector", "collector", c.Name(), "error", err)
			a.setStatus("collector %s failed to start: %v", c.Name(), err)
		}
	}
}

// orDefault returns value when positive, otherwise fallback.
func orDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

// collectors returns every constructed collector.
func (a *App) collectors() []collector.Collector {
	all := []collector.Collector{a.SystemCollector, a.HTTPCollector, a.GitCollector}

	// Typed nil pointers would satisfy the interface and pass a != nil check, so
	// each is tested before being added.
	if a.CloudCollector != nil {
		all = append(all, a.CloudCollector)
	}
	if a.KubernetesCollector != nil {
		all = append(all, a.KubernetesCollector)
	}
	if a.CICDCollector != nil {
		all = append(all, a.CICDCollector)
	}
	return all
}

// stopCollectors stops every collector and waits for them to finish.
func (a *App) stopCollectors() {
	for _, c := range a.collectors() {
		if c == nil {
			continue
		}
		if err := c.Stop(); err != nil {
			slog.Default().Error("failed to stop collector", "collector", c.Name(), "error", err)
		}
	}
}

// shutdownPlugins stops the plugin watcher and unloads every plugin.
func (a *App) shutdownPlugins() {
	if a.PluginManager == nil {
		return
	}
	a.PluginManager.StopWatcher()
	for _, err := range a.PluginManager.ShutdownAll() {
		slog.Default().Error("plugin shutdown failed", "error", err)
	}
}

// createUI builds the chrome that lives outside the tab content area.
func (a *App) createUI() {
	a.TermWidth, a.TermHeight = ui.TerminalDimensions()

	a.StatusBar = widgets.NewParagraph()
	a.StatusBar.Border = false
	a.StatusBar.TextStyle = ui.NewStyle(ui.ColorWhite)

	a.TabBar = widgets.NewTabPane(a.getTabNames()...)
	a.TabBar.ActiveTabIndex = a.ActiveTabIndex
	a.TabBar.Border = false
	a.TabBar.ActiveTabStyle = ui.NewStyle(ui.ColorCyan, ui.ColorClear, ui.ModifierBold)

	a.HelpPanel = widgets.NewParagraph()
	a.HelpPanel.Title = "Help - press ? to close"
	a.HelpPanel.Text = a.helpText()
	a.HelpPanel.WrapText = true
	a.HelpPanel.BorderStyle.Fg = ui.ColorYellow

	a.setStatus("maz-term ready - press ? for help")
}

// helpText documents the bindings that handleEvent actually implements.
func (a *App) helpText() string {
	return `Navigation
  Tab / Right / l / n   next tab
  BackTab / Left / h / p  previous tab
  1-9                   jump to tab by position
  Up / Down             move selection within a list

Actions
  ?    toggle this help
  r    refresh now
  e    export metrics to CSV
  q    quit  (Ctrl-C also quits)

History tab
  [ / ]  previous / next time range
  a      toggle event annotations
  A      add an annotation
  z      toggle zoom mode

Notifications tab
  m    mark selected as read
  D    dismiss selected
  C    clear all
  d    toggle detail view
  f    toggle filter mode
  o    open the selected notification's URL

Plugins tab
  Up / Down  select plugin
  R          reload plugins from disk`
}

// contentRect returns the rectangle available to the active tab, after the
// status and tab bars are reserved.
func (a *App) contentRect() image.Rectangle {
	height := a.TermHeight - statusBarHeight - tabBarHeight
	if height < 0 {
		height = 0
	}
	return image.Rect(0, 0, a.TermWidth, height)
}

// annotationsInRange returns the cached annotations that fall inside the
// currently selected history window.
func (a *App) annotationsInRange() []models.EventAnnotation {
	if !a.ShowAnnotations {
		return nil
	}

	cutoff := time.Now().Add(-a.HistoryRange)
	var out []models.EventAnnotation
	for _, annotation := range a.Annotations {
		if annotation.Timestamp.After(cutoff) {
			out = append(out, annotation)
		}
	}
	return out
}

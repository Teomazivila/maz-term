package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Teomazivila/maz-term/pkg/plugins"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// loadPlugins configures the plugin manager from the application configuration
// and loads the enabled plugins.
func (a *App) loadPlugins() {
	if a.PluginManager == nil {
		return
	}

	cfg := a.Config.Plugins
	if len(cfg.Enabled) == 0 {
		return
	}

	a.PluginManager.SetEnabledPlugins(cfg.Enabled)
	a.PluginManager.SetAllowedDigests(cfg.Allow)

	directory := cfg.Directory
	if directory == "" {
		a.setStatus("plugins enabled but plugins.directory is unset")
		return
	}

	loaded, errs := a.PluginManager.LoadEnabled(directory)
	for _, err := range errs {
		// A build without plugin support is a configuration mismatch worth
		// stating plainly, not an error to repeat for every plugin.
		if errors.Is(err, plugins.ErrPluginsNotSupported) {
			a.setStatus("plugins are not supported by this build")
			slog.Default().Warn("plugin loading skipped", "error", err)
			return
		}
		slog.Default().Error("plugin load failed", "error", err)
	}

	for _, name := range loaded {
		settings, ok := cfg.Settings[name]
		if !ok {
			continue
		}
		if err := a.PluginManager.InitializePlugin(name, settings); err != nil {
			slog.Default().Error("plugin initialisation failed", "plugin", name, "error", err)
		}
	}

	switch {
	case len(loaded) > 0:
		a.setStatus("loaded %d plugins: %s", len(loaded), strings.Join(loaded, ", "))
	case len(errs) > 0:
		a.setStatus("no plugins loaded (%d errors, see log)", len(errs))
	}

	if err := a.PluginManager.StartWatcher(directory); err != nil &&
		!errors.Is(err, plugins.ErrPluginsNotSupported) {
		slog.Default().Error("plugin watcher failed to start", "error", err)
	}
}

// handlePluginEvents handles bindings specific to the Plugins tab, reporting
// whether the event was consumed.
func (a *App) handlePluginEvents(e ui.Event) bool {
	loaded := a.PluginManager.Plugins()

	switch e.ID {
	case "<Up>":
		if a.SelectedPlugin > 0 {
			a.SelectedPlugin--
		}
		return true

	case "<Down>":
		if a.SelectedPlugin < len(loaded)-1 {
			a.SelectedPlugin++
		}
		return true

	case "R":
		a.reloadPlugins()
		return true
	}

	return false
}

// reloadPlugins reloads every plugin from disk.
func (a *App) reloadPlugins() {
	directory := a.Config.Plugins.Directory
	if directory == "" {
		a.setStatus("plugins.directory is unset")
		return
	}

	loaded, errs := a.PluginManager.Reload(directory)
	for _, err := range errs {
		slog.Default().Error("plugin reload failed", "error", err)
	}

	for _, name := range loaded {
		if settings, ok := a.Config.Plugins.Settings[name]; ok {
			if err := a.PluginManager.InitializePlugin(name, settings); err != nil {
				slog.Default().Error("plugin initialisation failed", "plugin", name, "error", err)
			}
		}
	}

	a.setStatus("reloaded %d plugins (%d errors)", len(loaded), len(errs))
}

// drainPluginEvents applies queued plugin lifecycle events.
//
// Events are consumed here, on the render goroutine, rather than dispatched from
// the watcher goroutine. A watcher-side callback previously mutated tabs, the
// status bar and widget state concurrently with rendering.
func (a *App) drainPluginEvents() {
	if a.PluginManager == nil {
		return
	}

	events := a.PluginManager.DrainEvents()
	if len(events) == 0 {
		return
	}

	reloadNeeded := false
	for _, event := range events {
		switch event.Kind {
		case plugins.EventReloaded:
			reloadNeeded = true
		case plugins.EventFailed:
			a.setStatus("plugin %s failed: %v", event.Plugin, event.Err)
		case plugins.EventRemoved:
			a.setStatus("plugin %s removed", event.Plugin)
		case plugins.EventLoaded:
			a.setStatus("plugin %s loaded", event.Plugin)
		}
	}

	if reloadNeeded {
		a.reloadPlugins()
	}
}

// updatePluginsTabData refreshes the Plugins tab.
func (a *App) updatePluginsTabData() {
	tab := a.getTabByName("Plugins")
	if tab == nil {
		return
	}

	if len(tab.Widgets) == 0 {
		list := widgets.NewList()
		list.Title = "Plugins"
		list.WrapText = false
		list.BorderStyle.Fg = ui.ColorCyan
		list.TextStyle = normalStyle
		list.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorCyan)

		details := newPanel("Plugin details")

		tab.Widgets = []ui.Drawable{list, details}
		tab.Lists = []*widgets.List{list}
		tab.Panels = []*widgets.Paragraph{details}
	}

	if a.PluginManager == nil {
		return
	}

	a.drainPluginEvents()

	// Collecting here keeps plugin execution on the render goroutine, bounded by
	// the manager's per-plugin timeout and panic containment.
	metrics, notifications := a.PluginManager.CollectAll(context.Background())
	a.PluginMetrics = metrics

	for _, notification := range notifications {
		if a.Storage == nil {
			break
		}
		if err := a.Storage.AddNotification(notification); err != nil {
			slog.Default().Error("failed to store plugin notification", "error", err)
		}
	}

	loaded := a.PluginManager.Plugins()
	if a.SelectedPlugin >= len(loaded) {
		a.SelectedPlugin = max(len(loaded)-1, 0)
	}

	if len(tab.Lists) > 0 {
		list := tab.Lists[0]
		rows := make([]string, 0, len(loaded))

		for _, plugin := range loaded {
			rows = append(rows, fmt.Sprintf("%-20s %-10s %s",
				TruncateString(plugin.Name(), 20),
				TruncateString(plugin.Version(), 10),
				TruncateString(plugin.Description(), 50)))
		}

		if len(rows) == 0 {
			if plugins.SupportsNativePlugins() {
				rows = append(rows, "  no plugins loaded")
			} else {
				rows = append(rows, "  plugin loading is not compiled into this build")
			}
		}

		list.Rows = rows
		list.SelectedRow = a.SelectedPlugin
	}

	if len(tab.Panels) > 0 {
		tab.Panels[0].Text = a.pluginDetails(loaded)
	}
}

// pluginDetails renders the detail panel for the selected plugin.
func (a *App) pluginDetails(loaded []plugins.Plugin) string {
	if len(loaded) == 0 {
		if !plugins.SupportsNativePlugins() {
			return "Native plugin loading is not compiled into this binary.\n\n" +
				"Rebuild with:\n  make build-with-plugins\n\n" +
				"Go's plugin package requires cgo and does not support Windows, so the\n" +
				"default build omits it to stay portable."
		}
		return "No plugins loaded.\n\nList them under plugins.enabled and record each\n" +
			"digest under plugins.allow in your configuration file."
	}

	if a.SelectedPlugin < 0 || a.SelectedPlugin >= len(loaded) {
		return "No plugin selected."
	}

	plugin := loaded[a.SelectedPlugin]
	paths := a.PluginManager.PluginPaths()

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n\n%s\n\n", plugin.Name(), plugin.Version(), plugin.Description())

	if path, ok := paths[plugin.Name()]; ok {
		fmt.Fprintf(&b, "path  %s\n\n", path)
	}

	if schema := plugin.GetConfigSchema(); len(schema) > 0 {
		b.WriteString("configuration\n")
		for key, description := range schema {
			fmt.Fprintf(&b, "  %-16s %s\n", key, description)
		}
		b.WriteString("\n")
	}

	pluginMetrics := plugin.GetMetrics()
	if len(pluginMetrics) == 0 {
		b.WriteString("no metrics reported yet\n")
		return b.String()
	}

	b.WriteString("metrics\n")
	for _, metric := range pluginMetrics {
		fmt.Fprintf(&b, "  %-24s %10.2f %s\n", metric.Name, metric.Value, metric.Unit)
	}

	return b.String()
}

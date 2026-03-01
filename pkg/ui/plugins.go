package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// loadPlugins loads plugins from the configured plugins directory
func (a *App) loadPlugins() {
	// Get the plugins directory from config
	pluginsDir := a.Config.Plugins.Directory
	if pluginsDir == "" {
		pluginsDir = "plugins" // Default directory
	}

	// Check if plugins directory exists
	if _, err := os.Stat(pluginsDir); !os.IsNotExist(err) {
		// Directory exists, load plugins
		a.StatusBar.Text = "Loading plugins..."
		ui.Render(a.StatusBar)

		// Set enabled plugins from configuration
		a.PluginManager.SetEnabledPlugins(a.Config.Plugins.Enabled)

		// Set up plugin change callback
		a.PluginManager.SetChangeCallback(func(name string, event string) {
			// Update UI when a plugin changes
			a.StatusBar.Text = fmt.Sprintf("Plugin %s %s", name, event)

			// If the plugin was reloaded, try to initialize it
			if event == "reloaded" {
				if settings, ok := a.Config.Plugins.Settings[name]; ok {
					if settingsMap, ok := settings.(map[string]interface{}); ok {
						if err := a.PluginManager.InitializePlugin(name, settingsMap); err != nil {
							log.Printf("Failed to initialize reloaded plugin %s: %v", name, err)
							a.StatusBar.Text = fmt.Sprintf("Error initializing plugin %s", name)
						}
					}
				}
			}

			// Update plugins tab data if it's visible
			if a.Tabs[a.ActiveTabIndex].Name == "Plugins" {
				a.updatePluginsTabData()
				a.updateLayout()
			}
		})

		// Load enabled plugins
		loaded, errors := a.PluginManager.LoadEnabledPlugins(pluginsDir)

		if len(loaded) > 0 {
			a.StatusBar.Text = fmt.Sprintf("Loaded %d plugins: %s", len(loaded), strings.Join(loaded, ", "))
		} else {
			a.StatusBar.Text = "No plugins loaded"
		}

		// Log errors if any
		for _, err := range errors {
			log.Printf("Plugin loading error: %v", err)
		}

		// Initialize plugins with their configuration
		for _, pluginName := range loaded {
			if settings, ok := a.Config.Plugins.Settings[pluginName]; ok {
				if settingsMap, ok := settings.(map[string]interface{}); ok {
					if err := a.PluginManager.InitializePlugin(pluginName, settingsMap); err != nil {
						log.Printf("Failed to initialize plugin %s: %v", pluginName, err)
						a.StatusBar.Text = fmt.Sprintf("Error initializing plugin %s", pluginName)
					}
				} else {
					log.Printf("Invalid settings for plugin %s: not a map", pluginName)
				}
			}
		}

		// Start watching for plugin changes
		if err := a.PluginManager.StartWatcher(pluginsDir); err != nil {
			log.Printf("Failed to start plugin watcher: %v", err)
		}

		ui.Render(a.StatusBar)
	}
}

// updatePluginsTabData updates the data for the Plugins tab
func (a *App) updatePluginsTabData() {
	// Get the plugins tab
	tab := a.getTabByName("Plugins")
	if tab == nil {
		return
	}

	// If no widgets for this tab yet, create them
	if len(tab.Widgets) == 0 {
		// Create plugin list
		pluginList := widgets.NewList()
		pluginList.Title = "Plugins"
		pluginList.WrapText = false
		pluginList.BorderStyle.Fg = ui.ColorCyan
		pluginList.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorCyan)

		// Create plugin details panel
		pluginDetails := widgets.NewParagraph()
		pluginDetails.Title = "Plugin Details"
		pluginDetails.WrapText = true
		pluginDetails.BorderStyle.Fg = ui.ColorCyan

		// Add widgets to tab
		tab.Widgets = []ui.Drawable{pluginList, pluginDetails}
		tab.Lists = []*widgets.List{pluginList}
		tab.Panels = []*widgets.Paragraph{pluginDetails}
	}

	// Get all plugins
	plugins := a.PluginManager.GetPlugins()

	// Update the plugins list
	list := tab.Lists[0]
	rows := make([]string, len(plugins))
	for i, plugin := range plugins {
		rows[i] = fmt.Sprintf("%s (v%s) - %s",
			plugin.Name(),
			plugin.Version(),
			plugin.Description())
	}

	if len(rows) == 0 {
		rows = []string{"No plugins loaded. Press [L] to load a plugin."}
	}

	list.Rows = rows

	// Update the plugin details if there are plugins
	details := tab.Panels[0]
	if len(plugins) > 0 && list.SelectedRow < len(plugins) {
		selectedPlugin := plugins[list.SelectedRow]

		// Get metrics and notifications
		metrics := selectedPlugin.GetMetrics()
		notifications := selectedPlugin.GetNotifications()

		// Format the details
		detailsText := fmt.Sprintf(`
Name: %s
Version: %s
Description: %s

Configuration Schema:
`, selectedPlugin.Name(), selectedPlugin.Version(), selectedPlugin.Description())

		// Add configuration schema
		configSchema := selectedPlugin.GetConfigSchema()
		if len(configSchema) > 0 {
			for key, desc := range configSchema {
				detailsText += fmt.Sprintf("  %s: %s\n", key, desc)
			}
		} else {
			detailsText += "  No configuration options available\n"
		}

		// Add metrics info
		detailsText += fmt.Sprintf("\nMetrics: %d\n", len(metrics))
		if len(metrics) > 0 {
			for i, metric := range metrics {
				if i < 3 { // Show only first 3 metrics
					detailsText += fmt.Sprintf("  %s: %.2f %s\n",
						metric.Name, metric.Value, metric.Unit)
				}
			}
			if len(metrics) > 3 {
				detailsText += fmt.Sprintf("  ... and %d more\n", len(metrics)-3)
			}
		}

		// Add notifications info
		detailsText += fmt.Sprintf("\nNotifications: %d\n", len(notifications))
		if len(notifications) > 0 {
			for i, notification := range notifications {
				if i < 3 { // Show only first 3 notifications
					detailsText += fmt.Sprintf("  [%s] %s\n",
						notification.Severity, notification.Title)
				}
			}
			if len(notifications) > 3 {
				detailsText += fmt.Sprintf("  ... and %d more\n", len(notifications)-3)
			}
		}

		// Add controls
		detailsText += "\nControls: [c] Collect data, [u] Unload plugin"

		details.Text = detailsText
	} else {
		details.Text = "No plugin selected or no plugins installed.\n\nUse [L] to load a plugin."
	}
}

// loadPlugin shows a dialog to load a plugin from a file
func (a *App) loadPlugin() {
	// Get the plugins directory
	pluginsDir := a.Config.Plugins.Directory
	if pluginsDir == "" {
		pluginsDir = "plugins" // Default directory
	}

	// Check if plugins directory exists, create it if not
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(pluginsDir, 0755); err != nil {
			a.StatusBar.Text = fmt.Sprintf("Failed to create plugins directory: %v", err)
			return
		}
	}

	// Show a "file picker" in the status bar
	a.StatusBar.Text = "Enter plugin path (relative to plugins directory):"
	ui.Render(a.StatusBar)

	// Handle input in the handlePluginEvents function
	// which will call loadPluginWithPath when the user confirms
}

// loadPluginWithPath loads a plugin from the given path
func (a *App) loadPluginWithPath(inputBuffer string) {
	// Get the plugins directory
	pluginsDir := a.Config.Plugins.Directory
	if pluginsDir == "" {
		pluginsDir = "plugins" // Default directory
	}

	// Construct the plugin path
	pluginPath := filepath.Join(pluginsDir, inputBuffer)
	if !strings.HasSuffix(pluginPath, ".so") {
		pluginPath += ".so"
	}

	// Check if file exists
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		a.StatusBar.Text = fmt.Sprintf("Plugin file not found: %s", pluginPath)
		return
	}

	// Load the plugin
	if err := a.PluginManager.LoadPlugin(pluginPath); err != nil {
		a.StatusBar.Text = fmt.Sprintf("Failed to load plugin: %v", err)
		return
	}

	// Get the plugin name
	var pluginName string
	for name, path := range a.PluginManager.GetPluginPaths() {
		if path == pluginPath {
			pluginName = name
			break
		}
	}

	if pluginName == "" {
		a.StatusBar.Text = "Plugin loaded but name could not be determined"
		return
	}

	// Initialize the plugin
	if settings, ok := a.Config.Plugins.Settings[pluginName]; ok {
		if settingsMap, ok := settings.(map[string]interface{}); ok {
			if err := a.PluginManager.InitializePlugin(pluginName, settingsMap); err != nil {
				a.StatusBar.Text = fmt.Sprintf("Error initializing plugin %s: %v", pluginName, err)
				return
			}
		}
	}

	// Add to enabled plugins list if not already there
	found := false
	for _, name := range a.Config.Plugins.Enabled {
		if name == pluginName {
			found = true
			break
		}
	}

	if !found {
		a.Config.Plugins.Enabled = append(a.Config.Plugins.Enabled, pluginName)
		a.PluginManager.SetEnabledPlugins(a.Config.Plugins.Enabled)
	}

	a.StatusBar.Text = fmt.Sprintf("Plugin '%s' loaded successfully", pluginName)
	a.updatePluginsTabData()
}

// handlePluginEvents handles events specific to the Plugins tab
func (a *App) handlePluginEvents(e ui.Event) {
	// Handle keys
	switch e.ID {
	case "q", "<C-c>":
		a.Running = false
	case "<Left>", "h":
		// Switch to previous tab
		a.ActiveTabIndex--
		if a.ActiveTabIndex < 0 {
			a.ActiveTabIndex = len(a.Tabs) - 1
		}
		a.TabBar.ActiveTabIndex = a.ActiveTabIndex
		a.updateLayout()
		return
	case "<Right>", "l":
		// Switch to next tab
		a.ActiveTabIndex++
		if a.ActiveTabIndex >= len(a.Tabs) {
			a.ActiveTabIndex = 0
		}
		a.TabBar.ActiveTabIndex = a.ActiveTabIndex
		a.updateLayout()
		return
	case "<Up>", "k":
		// Move selection up
		list := a.Tabs[a.ActiveTabIndex].Widgets[0].(*widgets.List)
		if list.SelectedRow > 0 {
			list.SelectedRow--
			a.updatePluginsTabData()
		}
	case "<Down>", "j":
		// Move selection down
		list := a.Tabs[a.ActiveTabIndex].Widgets[0].(*widgets.List)
		plugins := a.PluginManager.GetPlugins()
		if list.SelectedRow < len(plugins)-1 {
			list.SelectedRow++
			a.updatePluginsTabData()
		}
	case "L":
		// Load a plugin (uppercase L to avoid conflict with navigation)
		a.loadPlugin()
	case "u":
		// Unload a plugin
		list := a.Tabs[a.ActiveTabIndex].Widgets[0].(*widgets.List)
		plugins := a.PluginManager.GetPlugins()
		if len(plugins) > 0 && list.SelectedRow < len(plugins) {
			plugin := plugins[list.SelectedRow]
			if err := a.PluginManager.UnloadPlugin(plugin.Name()); err != nil {
				a.StatusBar.Text = fmt.Sprintf("Error unloading plugin: %v", err)
			} else {
				a.StatusBar.Text = fmt.Sprintf("Plugin '%s' unloaded successfully", plugin.Name())
				list.SelectedRow = 0
				a.updatePluginsTabData()
			}
		}
	case "c":
		// Collect data from plugins
		plugins := a.PluginManager.GetPlugins()
		if len(plugins) > 0 {
			list := a.Tabs[a.ActiveTabIndex].Widgets[0].(*widgets.List)
			if list.SelectedRow < len(plugins) {
				plugin := plugins[list.SelectedRow]
				if _, err := plugin.Collect(); err != nil {
					a.StatusBar.Text = fmt.Sprintf("Error collecting data: %v", err)
				} else {
					a.StatusBar.Text = fmt.Sprintf("Data collected successfully from plugin '%s'", plugin.Name())
					a.updatePluginsTabData()

					// Check for notifications from plugin
					notifications := plugin.GetNotifications()
					if len(notifications) > 0 {
						// Add notifications to storage if available
						if a.Storage != nil {
							for _, notification := range notifications {
								if err := a.Storage.AddNotification(notification); err != nil {
									a.StatusBar.Text = fmt.Sprintf("Error adding notification: %v", err)
								}
							}
						}
					}
				}
			}
		}
	}
}

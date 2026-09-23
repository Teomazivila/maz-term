// Package plugins loads and manages maz-term plugins.
//
// Native plugin loading is only compiled in when the "plugins" build tag is set
// and cgo is enabled, because Go's plugin package requires both and does not
// support Windows. The default build is therefore a single portable binary with
// plugin loading disabled; see loader_enabled.go and loader_disabled.go.
package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

const (
	// collectTimeout bounds a single plugin's Collect call. Plugin code is
	// third-party and cannot be trusted to return promptly.
	collectTimeout = 10 * time.Second

	// maxEventQueue caps the pending event queue so a plugin churning on disk
	// cannot grow it without bound.
	maxEventQueue = 64
)

// ErrPluginsNotSupported is returned when plugin loading is requested from a
// build that does not include it.
var ErrPluginsNotSupported = errors.New(
	"plugins: native plugin loading is not compiled into this binary (build with -tags plugins and CGO_ENABLED=1)")

// Plugin is implemented by dashboard plugins.
type Plugin interface {
	// Initialize configures the plugin. It is called once after loading.
	Initialize(config map[string]any) error

	// Name uniquely identifies the plugin.
	Name() string

	// Version reports the plugin's version.
	Version() string

	// Description is a one-line summary shown in the UI.
	Description() string

	// Collect gathers data. It must respect ctx cancellation.
	Collect(ctx context.Context) (any, error)

	// GetMetrics returns the metrics gathered by the most recent Collect.
	GetMetrics() []models.Metric

	// GetNotifications returns notifications raised by the plugin.
	GetNotifications() []models.Notification

	// GetConfigSchema documents the plugin's configuration keys.
	GetConfigSchema() map[string]string

	// Shutdown releases the plugin's resources.
	Shutdown() error
}

// EventKind classifies a plugin lifecycle event.
type EventKind string

const (
	EventLoaded   EventKind = "loaded"
	EventReloaded EventKind = "reloaded"
	EventRemoved  EventKind = "removed"
	EventFailed   EventKind = "failed"
)

// Event records a plugin lifecycle change for the UI to consume.
type Event struct {
	Kind   EventKind
	Plugin string
	Err    error
}

// PluginManager loads, tracks and unloads plugins.
type PluginManager struct {
	logger *slog.Logger

	mu      sync.RWMutex
	plugins map[string]Plugin
	paths   map[string]string
	enabled []string
	allow   map[string]string

	// events is drained by the UI thread. Lifecycle changes are queued rather
	// than dispatched through a callback, because the watcher runs on its own
	// goroutine and a callback there would mutate UI state concurrently with
	// the render loop.
	eventMu sync.Mutex
	events  []Event

	watcherMu     sync.Mutex
	watcherCancel context.CancelFunc
	watcherDone   chan struct{}
}

// NewPluginManager creates an empty plugin manager.
func NewPluginManager() *PluginManager {
	return &PluginManager{
		logger:  slog.Default().With("component", "plugins"),
		plugins: make(map[string]Plugin),
		paths:   make(map[string]string),
		allow:   make(map[string]string),
	}
}

// SetLogger replaces the manager's logger.
func (pm *PluginManager) SetLogger(logger *slog.Logger) {
	if logger == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.logger = logger.With("component", "plugins")
}

// SetEnabledPlugins records which plugins the configuration enables.
//
// The slice is copied: retaining the caller's backing array would let a later
// mutation silently change which plugins this manager considers enabled.
func (pm *PluginManager) SetEnabledPlugins(enabled []string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.enabled = append([]string(nil), enabled...)
}

// SetAllowedDigests records the expected SHA-256 digest for each plugin name.
// Loading refuses any plugin without a matching entry.
func (pm *PluginManager) SetAllowedDigests(allow map[string]string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.allow = make(map[string]string, len(allow))
	for name, digest := range allow {
		pm.allow[name] = digest
	}
}

// EnabledPlugins returns the configured plugin names.
func (pm *PluginManager) EnabledPlugins() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return append([]string(nil), pm.enabled...)
}

// Plugins returns the loaded plugins ordered by name, so the UI list is stable.
func (pm *PluginManager) Plugins() []Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	names := make([]string, 0, len(pm.plugins))
	for name := range pm.plugins {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Plugin, 0, len(names))
	for _, name := range names {
		out = append(out, pm.plugins[name])
	}
	return out
}

// GetPlugin returns the named plugin.
func (pm *PluginManager) GetPlugin(name string) (Plugin, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plugin, ok := pm.plugins[name]
	if !ok {
		return nil, fmt.Errorf("plugin %s: %w", name, models.ErrNotFound)
	}
	return plugin, nil
}

// PluginPaths returns a copy of the name-to-path mapping.
func (pm *PluginManager) PluginPaths() map[string]string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	out := make(map[string]string, len(pm.paths))
	for name, path := range pm.paths {
		out[name] = path
	}
	return out
}

// InitializePlugin configures a loaded plugin, containing any panic it raises.
func (pm *PluginManager) InitializePlugin(name string, config map[string]any) error {
	plugin, err := pm.GetPlugin(name)
	if err != nil {
		return err
	}

	return guard("initialize", name, func() error {
		return plugin.Initialize(config)
	})
}

// UnloadPlugin shuts a plugin down and forgets it.
//
// The plugin's Shutdown runs outside the manager lock so that a slow or hostile
// plugin cannot block every other manager operation.
func (pm *PluginManager) UnloadPlugin(name string) error {
	pm.mu.Lock()
	plugin, ok := pm.plugins[name]
	if ok {
		delete(pm.plugins, name)
		delete(pm.paths, name)
	}
	pm.mu.Unlock()

	if !ok {
		return fmt.Errorf("plugin %s: %w", name, models.ErrNotFound)
	}

	if err := guard("shutdown", name, plugin.Shutdown); err != nil {
		return err
	}

	pm.logger.Info("plugin unloaded", "plugin", name)
	return nil
}

// ShutdownAll unloads every plugin and returns any errors encountered.
func (pm *PluginManager) ShutdownAll() []error {
	pm.mu.Lock()
	loaded := make(map[string]Plugin, len(pm.plugins))
	for name, plugin := range pm.plugins {
		loaded[name] = plugin
	}
	pm.plugins = make(map[string]Plugin)
	pm.paths = make(map[string]string)
	pm.mu.Unlock()

	var errs []error
	for name, plugin := range loaded {
		if err := guard("shutdown", name, plugin.Shutdown); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// CollectAll runs Collect on every plugin and returns the metrics and
// notifications they produced.
//
// Each plugin is called outside the manager lock, with its own timeout, and with
// panics contained: one broken plugin must not stall or crash the dashboard.
func (pm *PluginManager) CollectAll(ctx context.Context) ([]models.Metric, []models.Notification) {
	var (
		metrics       []models.Metric
		notifications []models.Notification
	)

	for _, plugin := range pm.Plugins() {
		name := plugin.Name()

		collectCtx, cancel := context.WithTimeout(ctx, collectTimeout)
		err := guard("collect", name, func() error {
			_, err := plugin.Collect(collectCtx)
			return err
		})
		cancel()

		if err != nil {
			pm.logger.Warn("plugin collection failed", "plugin", name, "error", err)
			pm.queueEvent(Event{Kind: EventFailed, Plugin: name, Err: err})
			continue
		}

		if err := guard("metrics", name, func() error {
			metrics = append(metrics, plugin.GetMetrics()...)
			notifications = append(notifications, plugin.GetNotifications()...)
			return nil
		}); err != nil {
			pm.logger.Warn("plugin reporting failed", "plugin", name, "error", err)
		}
	}

	return metrics, notifications
}

// queueEvent appends a lifecycle event, dropping the oldest when full.
func (pm *PluginManager) queueEvent(event Event) {
	pm.eventMu.Lock()
	defer pm.eventMu.Unlock()

	if len(pm.events) >= maxEventQueue {
		pm.events = pm.events[1:]
	}
	pm.events = append(pm.events, event)
}

// DrainEvents returns and clears the pending lifecycle events. The UI calls this
// from its own goroutine, so no plugin callback ever touches UI state.
func (pm *PluginManager) DrainEvents() []Event {
	pm.eventMu.Lock()
	defer pm.eventMu.Unlock()

	if len(pm.events) == 0 {
		return nil
	}

	out := pm.events
	pm.events = nil
	return out
}

// guard runs fn, converting a panic in plugin code into an error so that a
// misbehaving plugin cannot take the dashboard down with it.
func guard(operation, name string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("plugin %s panicked during %s: %v", name, operation, r)
		}
	}()
	return fn()
}

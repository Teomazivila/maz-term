//go:build !plugins || !cgo

package plugins

// This is the default build. Go's plugin package requires cgo and does not
// support Windows, which is incompatible with shipping one portable static
// binary, so native loading is opt-in via -tags plugins with CGO_ENABLED=1.
//
// The manager itself still works: plugins compiled into the binary can be
// registered directly, and every UI path handles an empty plugin set.

// LoadEnabled reports that plugin loading is unavailable in this build.
func (pm *PluginManager) LoadEnabled(directory string) ([]string, []error) {
	if len(pm.EnabledPlugins()) == 0 {
		// Nothing was requested, so nothing is missing.
		return nil, nil
	}
	return nil, []error{ErrPluginsNotSupported}
}

// StartWatcher reports that plugin hot-reload is unavailable in this build.
func (pm *PluginManager) StartWatcher(directory string) error {
	if len(pm.EnabledPlugins()) == 0 {
		return nil
	}
	return ErrPluginsNotSupported
}

// StopWatcher is a no-op in this build.
func (pm *PluginManager) StopWatcher() {}

// Reload reports that plugin loading is unavailable in this build.
func (pm *PluginManager) Reload(directory string) ([]string, []error) {
	return pm.LoadEnabled(directory)
}

// SupportsNativePlugins reports whether this build can load plugins from disk.
func SupportsNativePlugins() bool { return false }

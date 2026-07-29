//go:build plugins && cgo

package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"sync"
	"time"
)

// debounceInterval collapses the burst of filesystem events editors and
// compilers emit when replacing a file.
const debounceInterval = 500 * time.Millisecond

// SupportsNativePlugins reports whether this build can load plugins from disk.
func SupportsNativePlugins() bool { return true }

// LoadEnabled loads every plugin named in the configuration from directory.
//
// A plugin is a native shared object executed inside this process, so each one
// is verified before it is opened: the path must resolve inside directory, the
// file and its parents must not be writable by other users, and its SHA-256
// digest must match the configured allowlist. Without those checks anything able
// to drop a .so into the plugin directory gains code execution in a process that
// holds the operator's credentials.
func (pm *PluginManager) LoadEnabled(directory string) ([]string, []error) {
	enabled := pm.EnabledPlugins()
	if len(enabled) == 0 {
		return nil, nil
	}

	root, err := resolveDirectory(directory)
	if err != nil {
		return nil, []error{err}
	}

	var (
		loaded []string
		errs   []error
	)

	for _, name := range enabled {
		path, err := pm.locatePlugin(root, name)
		if err != nil {
			errs = append(errs, err)
			pm.queueEvent(Event{Kind: EventFailed, Plugin: name, Err: err})
			continue
		}

		if err := pm.loadPlugin(name, path); err != nil {
			errs = append(errs, err)
			pm.queueEvent(Event{Kind: EventFailed, Plugin: name, Err: err})
			continue
		}

		loaded = append(loaded, name)
		pm.queueEvent(Event{Kind: EventLoaded, Plugin: name})
	}

	return loaded, errs
}

// Reload unloads every plugin and loads the enabled set again.
func (pm *PluginManager) Reload(directory string) ([]string, []error) {
	errs := pm.ShutdownAll()
	loaded, loadErrs := pm.LoadEnabled(directory)
	return loaded, append(errs, loadErrs...)
}

// resolveDirectory returns the absolute, symlink-resolved plugin directory.
func resolveDirectory(directory string) (string, error) {
	if directory == "" {
		return "", errors.New("plugins: directory is required")
	}

	abs, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("plugins: resolving %s: %w", directory, err)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("plugins: resolving %s: %w", abs, err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("plugins: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("plugins: %s is not a directory", resolved)
	}
	if err := checkNotSharedWritable(resolved, info); err != nil {
		return "", err
	}

	return resolved, nil
}

// locatePlugin finds the shared object for name, accepting either
// <dir>/<name>/<name>.so or <dir>/<name>.so, and rejects anything that escapes
// the plugin directory.
func (pm *PluginManager) locatePlugin(root, name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("plugins: invalid plugin name %q", name)
	}

	candidates := []string{
		filepath.Join(root, name, name+".so"),
		filepath.Join(root, name+".so"),
	}

	for _, candidate := range candidates {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}

		// Confine the resolved path to the plugin directory so a symlink cannot
		// point at an object elsewhere on the filesystem.
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("plugins: %s resolves outside %s", candidate, root)
		}

		info, err := os.Stat(resolved)
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("plugins: %s is not a regular file", resolved)
		}
		if err := checkNotSharedWritable(resolved, info); err != nil {
			return "", err
		}

		return resolved, nil
	}

	return "", fmt.Errorf("plugins: no shared object found for %s under %s", name, root)
}

// checkNotSharedWritable rejects paths writable by group or others, which would
// let another account substitute the code this process executes.
func checkNotSharedWritable(path string, info fs.FileInfo) error {
	const sharedWrite = 0o022
	if info.Mode().Perm()&sharedWrite != 0 {
		return fmt.Errorf("plugins: refusing %s: writable by group or others (%#o)",
			path, info.Mode().Perm())
	}
	return nil
}

// loadPlugin verifies and opens one plugin.
func (pm *PluginManager) loadPlugin(name, path string) error {
	digest, err := fileDigest(path)
	if err != nil {
		return err
	}

	pm.mu.RLock()
	expected, listed := pm.allow[name]
	pm.mu.RUnlock()

	if !listed {
		return fmt.Errorf(
			"plugins: %s is not in plugins.allow; add its digest to load it (sha256:%s)",
			name, digest)
	}
	if !strings.EqualFold(strings.TrimPrefix(expected, "sha256:"), digest) {
		return fmt.Errorf("plugins: %s digest mismatch: configured %s, on disk sha256:%s",
			name, expected, digest)
	}

	opened, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("plugins: opening %s: %w", path, err)
	}

	symbol, err := opened.Lookup("NewPlugin")
	if err != nil {
		return fmt.Errorf("plugins: %s does not export NewPlugin: %w", path, err)
	}

	constructor, ok := symbol.(func() Plugin)
	if !ok {
		return fmt.Errorf("plugins: %s exports NewPlugin with the wrong signature", path)
	}

	var instance Plugin
	if err := guard("construct", name, func() error {
		instance = constructor()
		return nil
	}); err != nil {
		return err
	}
	if instance == nil {
		return fmt.Errorf("plugins: %s returned a nil plugin", path)
	}

	// The manifest name must match the file it came from, so an allowlisted
	// digest cannot be used to register a plugin under another identity.
	reported := instance.Name()
	if reported != name {
		return fmt.Errorf("plugins: %s reports name %q, expected %q", path, reported, name)
	}

	pm.mu.Lock()
	pm.plugins[name] = instance
	pm.paths[name] = path
	pm.mu.Unlock()

	pm.logger.Info("plugin loaded", "plugin", name, "path", path, "sha256", digest)
	return nil
}

// fileDigest returns the hex SHA-256 of the file at path.
func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("plugins: reading %s: %w", path, err)
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", fmt.Errorf("plugins: hashing %s: %w", path, err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// StartWatcher watches the plugin directory and queues a reload event whenever a
// shared object changes. Reloading itself is performed by the UI thread when it
// drains the queue, so plugin code never runs on the watcher goroutine.
func (pm *PluginManager) StartWatcher(directory string) error {
	root, err := resolveDirectory(directory)
	if err != nil {
		return err
	}

	pm.watcherMu.Lock()
	defer pm.watcherMu.Unlock()

	if pm.watcherCancel != nil {
		return nil
	}

	watcher, err := newDirWatcher(root)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	pm.watcherCancel = cancel
	pm.watcherDone = done

	go func() {
		defer close(done)
		defer watcher.Close()
		pm.watch(ctx, watcher)
	}()

	return nil
}

// StopWatcher cancels the watcher and waits for its goroutine to exit.
//
// Waiting on a channel replaces the previous approach of sleeping while holding
// the manager lock and then setting the watcher field to nil, which raced with
// the goroutine still reading that field.
func (pm *PluginManager) StopWatcher() {
	pm.watcherMu.Lock()
	cancel, done := pm.watcherCancel, pm.watcherDone
	pm.watcherCancel, pm.watcherDone = nil, nil
	pm.watcherMu.Unlock()

	if cancel == nil {
		return
	}

	cancel()
	<-done
}

// watch consumes filesystem events until ctx is cancelled.
//
// The event and error channels are captured once by newDirWatcher and read only
// from this goroutine, so there is no shared field for StopWatcher to race with.
func (pm *PluginManager) watch(ctx context.Context, w *dirWatcher) {
	lastEvent := make(map[string]time.Time)

	for {
		select {
		case <-ctx.Done():
			return

		case path, ok := <-w.changes:
			if !ok {
				return
			}
			if !strings.HasSuffix(path, ".so") {
				continue
			}

			now := time.Now()
			if seen, exists := lastEvent[path]; exists && now.Sub(seen) < debounceInterval {
				continue
			}
			lastEvent[path] = now

			name := strings.TrimSuffix(filepath.Base(path), ".so")
			pm.logger.Info("plugin changed on disk", "plugin", name, "path", path)
			pm.queueEvent(Event{Kind: EventReloaded, Plugin: name})

		case err, ok := <-w.errors:
			if !ok {
				return
			}
			pm.logger.Error("plugin watcher error", "error", err)
		}
	}
}

// dirWatcher wraps fsnotify with channels owned by a single reader.
type dirWatcher struct {
	changes chan string
	errors  chan error
	closeFn func() error
	once    sync.Once
}

func (w *dirWatcher) Close() error {
	var err error
	w.once.Do(func() { err = w.closeFn() })
	return err
}

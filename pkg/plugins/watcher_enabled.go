//go:build plugins && cgo

package plugins

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// newDirWatcher watches root and its subdirectories, forwarding changed paths on
// a channel owned by a single reader.
//
// fsnotify's own channels are consumed here and never exposed, so no other
// goroutine can observe the watcher being closed mid-read.
func newDirWatcher(root string) (*dirWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("plugins: creating file watcher: %w", err)
	}

	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return watcher.Add(path)
		}
		return nil
	}); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("plugins: watching %s: %w", root, err)
	}

	changes := make(chan string, 32)
	errs := make(chan error, 8)

	go func() {
		defer close(changes)
		defer close(errs)

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
					continue
				}
				// Drop events when the consumer is behind: the reload is
				// idempotent, so a coalesced burst loses nothing.
				select {
				case changes <- event.Name:
				default:
				}

			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				select {
				case errs <- watchErr:
				default:
				}
			}
		}
	}()

	return &dirWatcher{
		changes: changes,
		errors:  errs,
		closeFn: watcher.Close,
	}, nil
}

// Package collector gathers metrics from the local system, HTTP endpoints, Git
// repositories and remote infrastructure providers.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// ErrInvalidInterval is returned when a collector is started with a
// non-positive interval. time.NewTicker panics on such a value, so it is
// rejected at the boundary instead.
var ErrInvalidInterval = errors.New("collector: interval must be positive")

// StorageProvider persists collected metrics.
//
// Every method takes a concrete type. An earlier revision accepted any and
// re-asserted the concrete type inside the storage adapter; when a collector
// passed a pointer instead of a value the assertion failed, returned an error,
// and that error was discarded by the caller, so samples were dropped in
// silence. Concrete types make the same mistake a compile error.
type StorageProvider interface {
	StoreSystemMetrics(metrics models.SystemMetrics) error
	StoreHTTPMetrics(name string, metrics models.EndpointMetrics) error
	StoreGitMetrics(metrics models.GitRepoMetrics) error
}

// Collector is implemented by every metrics source.
type Collector interface {
	// Collect gathers one sample. It must respect ctx cancellation.
	Collect(ctx context.Context) (any, error)

	// Name identifies the collector in logs and the UI.
	Name() string

	// Start begins periodic collection at the given interval.
	Start(ctx context.Context, interval time.Duration) error

	// Stop cancels collection and blocks until the collector has stopped.
	Stop() error
}

// BaseCollector provides the lifecycle, storage and logging plumbing shared by
// all collectors. Embedders supply their own Collect.
//
// The collection context is deliberately not retained as a field: it is scoped
// to the running loop and reaching it from elsewhere would outlive its
// cancellation. Only the cancel function and a completion channel are kept, so
// Stop can cancel the loop and wait for it.
type BaseCollector struct {
	name   string
	logger *slog.Logger

	mu         sync.RWMutex
	storage    StorageProvider
	running    bool
	cancel     context.CancelFunc
	done       chan struct{}
	lastData   any
	lastUpdate time.Time
}

// NewBaseCollector creates a base collector with the given name.
func NewBaseCollector(name string) *BaseCollector {
	return &BaseCollector{
		name:   name,
		logger: slog.Default().With("collector", name),
	}
}

// SetLogger replaces the collector's logger. A nil logger is ignored.
func (c *BaseCollector) SetLogger(logger *slog.Logger) {
	if logger == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger = logger.With("collector", c.name)
}

// Logger returns the collector's logger.
func (c *BaseCollector) Logger() *slog.Logger {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.logger
}

// SetStorageProvider sets the store used to persist samples. Passing nil
// disables persistence.
func (c *BaseCollector) SetStorageProvider(provider StorageProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.storage = provider
}

// Storage returns the configured store, or nil when persistence is disabled.
func (c *BaseCollector) Storage() StorageProvider {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.storage
}

// Name returns the collector's name.
func (c *BaseCollector) Name() string { return c.name }

// GetName returns the collector's name.
//
// Deprecated: use Name. Retained because existing call sites reference it.
func (c *BaseCollector) GetName() string { return c.name }

// start launches the periodic collection loop and returns immediately. collect
// runs once up front and then on every tick, always with the loop context.
//
// Stop blocks until the loop has returned, so a stopped collector never leaves
// a goroutine writing to a closed store.
func (c *BaseCollector) start(ctx context.Context, interval time.Duration, collect func(context.Context)) error {
	if interval <= 0 {
		return fmt.Errorf("%w, got %s", ErrInvalidInterval, interval)
	}

	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	c.cancel, c.done, c.running = cancel, done, true
	logger := c.logger
	c.mu.Unlock()

	logger.Debug("collector started", "interval", interval)

	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		collect(loopCtx)

		for {
			select {
			case <-ticker.C:
				collect(loopCtx)
			case <-loopCtx.Done():
				logger.Debug("collector loop stopped")
				return
			}
		}
	}()

	return nil
}

// Stop cancels the collection loop and waits for it to exit. It is safe to call
// on a collector that was never started, and safe to call more than once.
func (c *BaseCollector) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	cancel, done := c.cancel, c.done
	c.running, c.cancel, c.done = false, nil, nil
	c.mu.Unlock()

	cancel()
	<-done

	return nil
}

// IsRunning reports whether the collection loop is active.
func (c *BaseCollector) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// UpdateData records the most recent sample and the time it was taken.
func (c *BaseCollector) UpdateData(data any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastData = data
	c.lastUpdate = time.Now()
}

// GetLastData returns the most recent sample.
func (c *BaseCollector) GetLastData() any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastData
}

// GetLastUpdateTime returns when the most recent sample was taken.
func (c *BaseCollector) GetLastUpdateTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUpdate
}

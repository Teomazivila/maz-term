package collector

import (
	"context"
	"sync"
	"time"
)

// StorageProvider represents an interface for storing metrics
type StorageProvider interface {
	StoreSystemMetrics(metrics interface{}) error
	StoreHTTPMetrics(name string, metrics interface{}) error
	StoreGitMetrics(metrics interface{}) error
	StoreCloudMetrics(metrics interface{}) error
	StoreKubernetesMetrics(metrics interface{}) error
	StoreCICDMetrics(metrics interface{}) error
}

// Collector is the interface that wraps the basic Collect method
type Collector interface {
	// Collect returns the collected metrics or an error
	Collect(ctx context.Context) (interface{}, error)

	// Name returns the name of the collector
	Name() string

	// Start starts the collector with the specified interval
	Start(ctx context.Context, interval time.Duration) error

	// Stop stops the collector
	Stop() error
}

// BaseCollector provides common functionality for all collectors
type BaseCollector struct {
	name       string
	running    bool
	ctx        context.Context
	cancel     context.CancelFunc
	mutex      sync.RWMutex
	lastData   interface{}
	lastUpdate time.Time
	storage    StorageProvider
}

// NewBaseCollector creates a new base collector
func NewBaseCollector(name string) *BaseCollector {
	return &BaseCollector{
		name:    name,
		running: false,
	}
}

// SetStorageProvider sets the storage provider for the collector
func (c *BaseCollector) SetStorageProvider(provider StorageProvider) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.storage = provider
}

// Start starts the collector with proper context management
func (c *BaseCollector) Start(ctx context.Context, interval time.Duration) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.running {
		return nil // Already running
	}

	// Create cancellable context for this collector
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.running = true

	return nil
}

// Stop stops the collector with proper cleanup
func (c *BaseCollector) Stop() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.running {
		return nil // Already stopped
	}

	c.running = false
	if c.cancel != nil {
		c.cancel()
	}

	return nil
}

// IsRunning returns true if the collector is running
func (c *BaseCollector) IsRunning() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.running
}

// GetName returns the name of the collector
func (c *BaseCollector) GetName() string {
	return c.name
}

// UpdateData updates the last collected data
func (c *BaseCollector) UpdateData(data interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.lastData = data
	c.lastUpdate = time.Now()
}

// GetLastData returns the last collected data
func (c *BaseCollector) GetLastData() interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lastData
}

// GetLastUpdateTime returns the time of the last data update
func (c *BaseCollector) GetLastUpdateTime() time.Time {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lastUpdate
}

// StoreData attempts to store the data if a storage provider is available
func (c *BaseCollector) StoreData(name string, data interface{}, storageType string) {
	c.mutex.RLock()
	storage := c.storage
	c.mutex.RUnlock()

	if storage == nil {
		return // No storage provider available
	}

	// Try to store the data based on the type
	switch storageType {
	case "system":
		_ = storage.StoreSystemMetrics(data)
	case "http":
		_ = storage.StoreHTTPMetrics(name, data)
	case "git":
		_ = storage.StoreGitMetrics(data)
	case "cloud":
		_ = storage.StoreCloudMetrics(data)
	case "kubernetes":
		_ = storage.StoreKubernetesMetrics(data)
	case "cicd":
		_ = storage.StoreCICDMetrics(data)
	}
}

// Context returns the collector's context for cancellation
func (c *BaseCollector) Context() context.Context {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.ctx
}

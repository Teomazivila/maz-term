package collector

import (
	"context"
	"sync"
	"time"
)

// BaseCollector provides common collector functionality
type BaseCollector struct {
	name     string
	running  bool
	mutex    sync.RWMutex
	stopChan chan struct{}
}

// NewBaseCollector creates a new base collector
func NewBaseCollector(name string) *BaseCollector {
	return &BaseCollector{
		name:     name,
		running:  false,
		stopChan: make(chan struct{}),
	}
}

// Name returns the name of the collector
func (c *BaseCollector) Name() string {
	return c.name
}

// IsRunning returns whether the collector is running
func (c *BaseCollector) IsRunning() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.running
}

// Start starts the collector
func (c *BaseCollector) Start(ctx context.Context, interval time.Duration) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.running {
		return nil // Already running
	}

	c.running = true
	c.stopChan = make(chan struct{})
	return nil
}

// Stop stops the collector
func (c *BaseCollector) Stop() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.running {
		return nil // Already stopped
	}

	close(c.stopChan)
	c.running = false
	return nil
}

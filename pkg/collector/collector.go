package collector

import (
	"context"
	"time"
)

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

// BaseCollector provides common functionality for collectors
type BaseCollector struct {
	name     string
	interval time.Duration
	running  bool
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

// Start starts the collector with the specified interval
func (c *BaseCollector) Start(ctx context.Context, interval time.Duration) error {
	if c.running {
		return nil
	}

	c.interval = interval
	c.running = true
	c.stopChan = make(chan struct{})

	return nil
}

// Stop stops the collector
func (c *BaseCollector) Stop() error {
	if !c.running {
		return nil
	}

	c.running = false
	close(c.stopChan)

	return nil
}

// IsRunning returns whether the collector is running
func (c *BaseCollector) IsRunning() bool {
	return c.running
}

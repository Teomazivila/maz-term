package collector

import (
	"context"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// CloudProviderType represents a type of cloud provider
type CloudProviderType string

const (
	// CloudProviderAWS represents Amazon Web Services
	CloudProviderAWS CloudProviderType = "aws"

	// CloudProviderGCP represents Google Cloud Platform
	CloudProviderGCP CloudProviderType = "gcp"

	// CloudProviderAzure represents Microsoft Azure
	CloudProviderAzure CloudProviderType = "azure"
)

// CloudMetricsCollector interface defines methods for cloud provider metrics collectors
type CloudMetricsCollector interface {
	Collector

	// GetProviderType returns the type of cloud provider
	GetProviderType() CloudProviderType

	// GetResourceCount returns the number of resources being monitored
	GetResourceCount() int

	// GetRegions returns the list of monitored regions
	GetRegions() []string

	// GetInstanceMetrics returns metrics for compute instances
	GetInstanceMetrics() ([]models.CloudInstanceMetrics, error)

	// GetStorageMetrics returns metrics for storage services
	GetStorageMetrics() ([]models.CloudStorageMetrics, error)

	// GetDatabaseMetrics returns metrics for database services
	GetDatabaseMetrics() ([]models.CloudDatabaseMetrics, error)

	// GetLatestMetrics returns the most recently collected metrics
	GetLatestMetrics() models.CloudProviderMetrics
}

// CloudCollectorConfig represents configuration for a cloud metrics collector
type CloudCollectorConfig struct {
	// ProviderType is the type of cloud provider
	ProviderType CloudProviderType

	// Regions is the list of regions to monitor
	Regions []string

	// Credentials contains provider-specific credentials
	Credentials map[string]string

	// ResourceTypes specifies which resource types to monitor
	ResourceTypes []string

	// RefreshInterval is how often to refresh metrics
	RefreshInterval time.Duration

	// MaxResources is the maximum number of resources to monitor
	MaxResources int
}

// BaseCloudCollector provides common functionality for cloud provider collectors
type BaseCloudCollector struct {
	*BaseCollector
	config  CloudCollectorConfig
	metrics models.CloudProviderMetrics
}

// NewBaseCloudCollector creates a new base cloud collector
func NewBaseCloudCollector(name string, config CloudCollectorConfig) *BaseCloudCollector {
	return &BaseCloudCollector{
		BaseCollector: NewBaseCollector(name),
		config:        config,
		metrics: models.CloudProviderMetrics{
			ProviderType: string(config.ProviderType),
			Regions:      config.Regions,
			LastUpdated:  time.Now(),
		},
	}
}

// GetProviderType returns the type of cloud provider
func (c *BaseCloudCollector) GetProviderType() CloudProviderType {
	return c.config.ProviderType
}

// GetRegions returns the list of monitored regions
func (c *BaseCloudCollector) GetRegions() []string {
	return c.config.Regions
}

// GetLatestMetrics returns the most recently collected metrics
func (c *BaseCloudCollector) GetLatestMetrics() models.CloudProviderMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.metrics
}

// Start starts the collector
func (c *BaseCloudCollector) Start(ctx context.Context, interval time.Duration) error {
	// Call the parent Start method
	if err := c.BaseCollector.Start(ctx, interval); err != nil {
		return err
	}

	if !c.running {
		return nil
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Collect initial data
		c.collect(ctx)

		for {
			select {
			case <-ticker.C:
				c.collect(ctx)
			case <-c.Context().Done():
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// collect performs the actual metrics collection
func (c *BaseCloudCollector) collect(ctx context.Context) {
	// Since BaseCloudCollector doesn't implement Collect directly,
	// we need to make sure the actual implementation handles this
	if collector, ok := interface{}(c).(Collector); ok {
		data, err := collector.Collect(ctx)
		if err != nil {
			// Log error but continue
			return
		}

		// Update the metrics with the collected data
		if cloudMetrics, ok := data.(models.CloudProviderMetrics); ok {
			c.mutex.Lock()
			c.metrics = cloudMetrics
			c.metrics.LastUpdated = time.Now()
			c.mutex.Unlock()
		}

		// Store the data if a storage provider is available
		c.mutex.RLock()
		defer c.mutex.RUnlock()

		if c.storage == nil {
			return // No storage provider available
		}

		// Store the cloud metrics using the appropriate method
		_ = c.storage.StoreCloudMetrics(data)
	}
}

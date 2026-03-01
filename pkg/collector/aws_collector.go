package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
)

// AWSMetricsCollector collects metrics from AWS
type AWSMetricsCollector struct {
	*BaseCloudCollector
	awsConfig config.AWSConfig
	instances []models.CloudInstanceMetrics
	s3Buckets []models.CloudStorageMetrics
	databases []models.CloudDatabaseMetrics
}

// NewAWSMetricsCollector creates a new AWS metrics collector
func NewAWSMetricsCollector(cfg config.AWSConfig) *AWSMetricsCollector {
	// Combine the primary region with additional regions
	regions := append([]string{cfg.Region}, cfg.AdditionalRegions...)

	// Create cloud collector config
	cloudConfig := CloudCollectorConfig{
		ProviderType:    CloudProviderAWS,
		Regions:         regions,
		ResourceTypes:   cfg.Resources,
		RefreshInterval: 1 * time.Minute,
		MaxResources:    100,
		Credentials: map[string]string{
			"profile":           cfg.Profile,
			"access_key_id":     cfg.AccessKeyID,
			"secret_access_key": cfg.SecretAccessKey,
		},
	}

	return &AWSMetricsCollector{
		BaseCloudCollector: NewBaseCloudCollector("aws", cloudConfig),
		awsConfig:          cfg,
		instances:          []models.CloudInstanceMetrics{},
		s3Buckets:          []models.CloudStorageMetrics{},
		databases:          []models.CloudDatabaseMetrics{},
	}
}

// Name returns the name of the collector
func (c *AWSMetricsCollector) Name() string {
	return "AWS Metrics Collector"
}

// Collect gathers metrics from AWS services
func (c *AWSMetricsCollector) Collect(ctx context.Context) (interface{}, error) {
	// For the MVP, we'll simulate AWS data collection with demo data
	// In a real implementation, we would use the AWS SDK to collect actual metrics

	metrics := models.CloudProviderMetrics{
		ProviderType:    string(CloudProviderAWS),
		Name:            "AWS",
		Regions:         c.config.Regions,
		InstanceMetrics: []models.CloudInstanceMetrics{},
		StorageMetrics:  []models.CloudStorageMetrics{},
		DatabaseMetrics: []models.CloudDatabaseMetrics{},
		LastUpdated:     time.Now(),
	}

	// Use a waitgroup to collect metrics concurrently
	var wg sync.WaitGroup
	var instancesErr, storageErr, dbErr error

	// Collect EC2 instances if configured
	if contains(c.awsConfig.Resources, "ec2") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			instances, err := c.collectEC2Instances(ctx)
			if err != nil {
				instancesErr = err
				return
			}
			c.instances = instances
		}()
	}

	// Collect S3 buckets if configured
	if contains(c.awsConfig.Resources, "s3") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buckets, err := c.collectS3Buckets(ctx)
			if err != nil {
				storageErr = err
				return
			}
			c.s3Buckets = buckets
		}()
	}

	// Collect RDS instances if configured
	if contains(c.awsConfig.Resources, "rds") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dbs, err := c.collectRDSInstances(ctx)
			if err != nil {
				dbErr = err
				return
			}
			c.databases = dbs
		}()
	}

	// Wait for all collectors to finish
	wg.Wait()

	// Check for errors
	if instancesErr != nil {
		metrics.Errors = append(metrics.Errors, fmt.Sprintf("EC2 error: %v", instancesErr))
	}
	if storageErr != nil {
		metrics.Errors = append(metrics.Errors, fmt.Sprintf("S3 error: %v", storageErr))
	}
	if dbErr != nil {
		metrics.Errors = append(metrics.Errors, fmt.Sprintf("RDS error: %v", dbErr))
	}

	// Add collected metrics to the result
	metrics.InstanceMetrics = c.instances
	metrics.StorageMetrics = c.s3Buckets
	metrics.DatabaseMetrics = c.databases

	return metrics, nil
}

// GetResourceCount returns the number of resources being monitored
func (c *AWSMetricsCollector) GetResourceCount() int {
	return len(c.instances) + len(c.s3Buckets) + len(c.databases)
}

// GetInstanceMetrics returns metrics for compute instances
func (c *AWSMetricsCollector) GetInstanceMetrics() ([]models.CloudInstanceMetrics, error) {
	return c.instances, nil
}

// GetStorageMetrics returns metrics for storage services
func (c *AWSMetricsCollector) GetStorageMetrics() ([]models.CloudStorageMetrics, error) {
	return c.s3Buckets, nil
}

// GetDatabaseMetrics returns metrics for database services
func (c *AWSMetricsCollector) GetDatabaseMetrics() ([]models.CloudDatabaseMetrics, error) {
	return c.databases, nil
}

// collectEC2Instances collects metrics from EC2 instances
func (c *AWSMetricsCollector) collectEC2Instances(ctx context.Context) ([]models.CloudInstanceMetrics, error) {
	// In a real implementation, we would use the AWS SDK to collect actual metrics
	// For the MVP, we'll return demo data

	instances := []models.CloudInstanceMetrics{
		{
			ID:                "i-0123456789abcdef0",
			Name:              "web-server-1",
			Type:              "t3.medium",
			Region:            c.awsConfig.Region,
			Status:            models.ResourceStatusRunning,
			CPUUtilization:    35.2,
			MemoryUtilization: 67.8,
			NetworkIn:         1548576,
			NetworkOut:        687924,
			DiskIO:            12345,
			UptimeHours:       168.5,
			Tags: map[string]string{
				"Environment": "production",
				"Role":        "web",
			},
		},
		{
			ID:                "i-0123456789abcdef1",
			Name:              "api-server-1",
			Type:              "t3.large",
			Region:            c.awsConfig.Region,
			Status:            models.ResourceStatusRunning,
			CPUUtilization:    42.7,
			MemoryUtilization: 55.3,
			NetworkIn:         2548576,
			NetworkOut:        1687924,
			DiskIO:            23456,
			UptimeHours:       336.2,
			Tags: map[string]string{
				"Environment": "production",
				"Role":        "api",
			},
		},
		{
			ID:                "i-0123456789abcdef2",
			Name:              "db-server-1",
			Type:              "m5.xlarge",
			Region:            c.awsConfig.Region,
			Status:            models.ResourceStatusRunning,
			CPUUtilization:    65.4,
			MemoryUtilization: 88.2,
			NetworkIn:         548576,
			NetworkOut:        487924,
			DiskIO:            56789,
			UptimeHours:       720.0,
			Tags: map[string]string{
				"Environment": "production",
				"Role":        "database",
			},
		},
	}

	// If additional regions are configured, add instances from those regions
	for _, region := range c.awsConfig.AdditionalRegions {
		instances = append(instances, models.CloudInstanceMetrics{
			ID:                fmt.Sprintf("i-region%s", region[:4]),
			Name:              fmt.Sprintf("server-%s", region),
			Type:              "t3.medium",
			Region:            region,
			Status:            models.ResourceStatusRunning,
			CPUUtilization:    25.5,
			MemoryUtilization: 45.2,
			NetworkIn:         1048576,
			NetworkOut:        587924,
			DiskIO:            9876,
			UptimeHours:       72.5,
			Tags: map[string]string{
				"Environment": "staging",
				"Region":      region,
			},
		})
	}

	return instances, nil
}

// collectS3Buckets collects metrics from S3 buckets
func (c *AWSMetricsCollector) collectS3Buckets(ctx context.Context) ([]models.CloudStorageMetrics, error) {
	// In a real implementation, we would use the AWS SDK to collect actual metrics
	// For the MVP, we'll return demo data

	buckets := []models.CloudStorageMetrics{
		{
			ID:             "my-app-static-assets",
			Name:           "my-app-static-assets",
			Type:           "S3 Bucket",
			Region:         c.awsConfig.Region,
			TotalSizeBytes: 5368709120, // 5GB
			ObjectCount:    12345,
			RequestCount:   67890,
			ReadBytes:      1073741824, // 1GB
			WriteBytes:     536870912,  // 512MB
			Tags: map[string]string{
				"Environment": "production",
				"Purpose":     "static-assets",
			},
		},
		{
			ID:             "my-app-logs",
			Name:           "my-app-logs",
			Type:           "S3 Bucket",
			Region:         c.awsConfig.Region,
			TotalSizeBytes: 10737418240, // 10GB
			ObjectCount:    98765,
			RequestCount:   54321,
			ReadBytes:      536870912,  // 512MB
			WriteBytes:     2147483648, // 2GB
			Tags: map[string]string{
				"Environment": "production",
				"Purpose":     "logs",
			},
		},
		{
			ID:             "my-app-backups",
			Name:           "my-app-backups",
			Type:           "S3 Bucket",
			Region:         c.awsConfig.Region,
			TotalSizeBytes: 107374182400, // 100GB
			ObjectCount:    5432,
			RequestCount:   1234,
			ReadBytes:      21474836480, // 20GB
			WriteBytes:     10737418240, // 10GB
			Tags: map[string]string{
				"Environment": "production",
				"Purpose":     "backups",
			},
		},
	}

	return buckets, nil
}

// collectRDSInstances collects metrics from RDS instances
func (c *AWSMetricsCollector) collectRDSInstances(ctx context.Context) ([]models.CloudDatabaseMetrics, error) {
	// In a real implementation, we would use the AWS SDK to collect actual metrics
	// For the MVP, we'll return demo data

	databases := []models.CloudDatabaseMetrics{
		{
			ID:                 "db-prod-01",
			Name:               "production-db",
			Type:               "RDS",
			Engine:             "PostgreSQL",
			Region:             c.awsConfig.Region,
			Status:             models.ResourceStatusRunning,
			CPUUtilization:     62.3,
			MemoryUtilization:  75.8,
			StorageUtilization: 68.5,
			ConnectionCount:    145,
			ReadIOPS:           512.7,
			WriteIOPS:          123.4,
			Latency:            5.2,
			Tags: map[string]string{
				"Environment": "production",
				"Role":        "primary",
			},
		},
		{
			ID:                 "db-prod-read-01",
			Name:               "production-read-replica",
			Type:               "RDS",
			Engine:             "PostgreSQL",
			Region:             c.awsConfig.Region,
			Status:             models.ResourceStatusRunning,
			CPUUtilization:     42.1,
			MemoryUtilization:  65.3,
			StorageUtilization: 67.2,
			ConnectionCount:    87,
			ReadIOPS:           756.3,
			WriteIOPS:          0.0,
			Latency:            3.7,
			Tags: map[string]string{
				"Environment": "production",
				"Role":        "read-replica",
			},
		},
	}

	return databases, nil
}

// Helper function to check if a slice contains a string
func contains(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

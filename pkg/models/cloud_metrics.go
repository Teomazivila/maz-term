package models

import (
	"time"
)

// ResourceStatus represents the status of a cloud resource
type ResourceStatus string

const (
	// ResourceStatusRunning indicates the resource is running normally
	ResourceStatusRunning ResourceStatus = "running"

	// ResourceStatusStopped indicates the resource is stopped
	ResourceStatusStopped ResourceStatus = "stopped"

	// ResourceStatusPending indicates the resource is in a transitional state
	ResourceStatusPending ResourceStatus = "pending"

	// ResourceStatusError indicates the resource is in an error state
	ResourceStatusError ResourceStatus = "error"
)

// CloudProviderMetrics represents metrics for an entire cloud provider
type CloudProviderMetrics struct {
	// ProviderType is the type of cloud provider (aws, gcp, azure)
	ProviderType string `json:"provider_type"`

	// Name is a friendly name for this cloud provider account
	Name string `json:"name"`

	// Regions is the list of regions being monitored
	Regions []string `json:"regions"`

	// InstanceMetrics contains metrics for compute instances
	InstanceMetrics []CloudInstanceMetrics `json:"instance_metrics"`

	// StorageMetrics contains metrics for storage services
	StorageMetrics []CloudStorageMetrics `json:"storage_metrics"`

	// DatabaseMetrics contains metrics for database services
	DatabaseMetrics []CloudDatabaseMetrics `json:"database_metrics"`

	// LastUpdated is when the metrics were last collected
	LastUpdated time.Time `json:"last_updated"`

	// Errors contains any errors encountered during collection
	Errors []string `json:"errors,omitempty"`
}

// CloudInstanceMetrics represents metrics for a cloud compute instance
type CloudInstanceMetrics struct {
	// ID is the unique identifier for the instance
	ID string `json:"id"`

	// Name is the user-assigned name of the instance
	Name string `json:"name"`

	// Type is the instance type/size
	Type string `json:"type"`

	// Region is the region where the instance is deployed
	Region string `json:"region"`

	// Status is the current status of the instance
	Status ResourceStatus `json:"status"`

	// CPUUtilization is the CPU utilization percentage
	CPUUtilization float64 `json:"cpu_utilization"`

	// MemoryUtilization is the memory utilization percentage
	MemoryUtilization float64 `json:"memory_utilization"`

	// NetworkIn is incoming network traffic in bytes
	NetworkIn uint64 `json:"network_in"`

	// NetworkOut is outgoing network traffic in bytes
	NetworkOut uint64 `json:"network_out"`

	// DiskIO is disk I/O operations
	DiskIO uint64 `json:"disk_io"`

	// UptimeHours is the instance uptime in hours
	UptimeHours float64 `json:"uptime_hours"`

	// Tags are instance tags/labels
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudStorageMetrics represents metrics for a cloud storage service
type CloudStorageMetrics struct {
	// ID is the unique identifier for the storage resource
	ID string `json:"id"`

	// Name is the user-assigned name of the storage resource
	Name string `json:"name"`

	// Type is the storage type (bucket, blob container, etc.)
	Type string `json:"type"`

	// Region is the region where the storage is deployed
	Region string `json:"region"`

	// TotalSizeBytes is the total size in bytes
	TotalSizeBytes uint64 `json:"total_size_bytes"`

	// ObjectCount is the number of objects stored
	ObjectCount int `json:"object_count"`

	// RequestCount is the number of requests in the monitoring period
	RequestCount int `json:"request_count"`

	// ReadBytes is the number of bytes read in the monitoring period
	ReadBytes uint64 `json:"read_bytes"`

	// WriteBytes is the number of bytes written in the monitoring period
	WriteBytes uint64 `json:"write_bytes"`

	// Tags are storage resource tags/labels
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudDatabaseMetrics represents metrics for a cloud database service
type CloudDatabaseMetrics struct {
	// ID is the unique identifier for the database
	ID string `json:"id"`

	// Name is the user-assigned name of the database
	Name string `json:"name"`

	// Type is the database type (RDS, DynamoDB, etc.)
	Type string `json:"type"`

	// Engine is the database engine (MySQL, PostgreSQL, etc.)
	Engine string `json:"engine"`

	// Region is the region where the database is deployed
	Region string `json:"region"`

	// Status is the current status of the database
	Status ResourceStatus `json:"status"`

	// CPUUtilization is the CPU utilization percentage
	CPUUtilization float64 `json:"cpu_utilization"`

	// MemoryUtilization is the memory utilization percentage
	MemoryUtilization float64 `json:"memory_utilization"`

	// StorageUtilization is the storage utilization percentage
	StorageUtilization float64 `json:"storage_utilization"`

	// ConnectionCount is the number of active connections
	ConnectionCount int `json:"connection_count"`

	// ReadIOPS is read operations per second
	ReadIOPS float64 `json:"read_iops"`

	// WriteIOPS is write operations per second
	WriteIOPS float64 `json:"write_iops"`

	// Latency is the average query latency in milliseconds
	Latency float64 `json:"latency"`

	// Tags are database tags/labels
	Tags map[string]string `json:"tags,omitempty"`
}

// CloudResourceIdentifier uniquely identifies a cloud resource
type CloudResourceIdentifier struct {
	// Provider is the cloud provider (aws, gcp, azure)
	Provider string `json:"provider"`

	// Service is the service type (ec2, s3, rds, etc.)
	Service string `json:"service"`

	// Region is the region where the resource is deployed
	Region string `json:"region"`

	// ID is the unique identifier for the resource
	ID string `json:"id"`

	// Name is the user-assigned name of the resource
	Name string `json:"name"`
}

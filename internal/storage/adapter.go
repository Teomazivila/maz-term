package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// Adapter implements the collector.StorageProvider interface
type Adapter struct {
	db *Database
}

// NewAdapter creates a new storage adapter
func NewAdapter(db *Database) *Adapter {
	return &Adapter{
		db: db,
	}
}

// StoreSystemMetrics stores system metrics in the database
func (a *Adapter) StoreSystemMetrics(metrics interface{}) error {
	// Type assertion to get the concrete type
	sysMetrics, ok := metrics.(models.SystemMetrics)
	if !ok {
		return fmt.Errorf("invalid metrics type: expected SystemMetrics")
	}

	// Store the metrics in the database
	return a.db.StoreSystemMetrics(sysMetrics)
}

// StoreHTTPMetrics stores HTTP metrics in the database
func (a *Adapter) StoreHTTPMetrics(name string, metrics interface{}) error {
	// Type assertion to get the concrete type
	httpMetrics, ok := metrics.(models.EndpointMetrics)
	if !ok {
		return fmt.Errorf("invalid metrics type: expected EndpointMetrics")
	}

	// Store the metrics in the database
	return a.db.StoreHTTPMetrics(name, httpMetrics)
}

// StoreGitMetrics stores Git metrics in the database
func (a *Adapter) StoreGitMetrics(metrics interface{}) error {
	// Type assertion to get the concrete type
	gitMetrics, ok := metrics.(models.GitRepoMetrics)
	if !ok {
		return fmt.Errorf("invalid metrics type: expected GitRepoMetrics")
	}

	// Store the metrics in the database
	return a.db.StoreGitMetrics(gitMetrics)
}

// GetCPUUsageHistory fetches historical CPU usage data
func (a *Adapter) GetCPUUsageHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetCPUUsageHistory(period, points)
}

// GetMemoryUsageHistory fetches historical memory usage data
func (a *Adapter) GetMemoryUsageHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetMemoryUsageHistory(period, points)
}

// GetDiskUsageHistory fetches historical disk usage data for a specific mount point
func (a *Adapter) GetDiskUsageHistory(mountPoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetDiskUsageHistory(mountPoint, period, points)
}

// GetHTTPResponseTimeHistory fetches historical response time data for a specific endpoint
func (a *Adapter) GetHTTPResponseTimeHistory(endpoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetHTTPResponseTimeHistory(endpoint, period, points)
}

// GetHTTPAvailabilityHistory fetches historical availability data for a specific endpoint
func (a *Adapter) GetHTTPAvailabilityHistory(endpoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetHTTPAvailabilityHistory(endpoint, period, points)
}

// GetAllEndpoints returns a list of all monitored HTTP endpoints
func (a *Adapter) GetAllEndpoints() ([]string, error) {
	return a.db.GetAllEndpoints()
}

// GetCommitCountHistory fetches historical commit count data
func (a *Adapter) GetCommitCountHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetCommitCountHistory(repoName, period, points)
}

// GetModifiedFilesHistory fetches historical modified files count data
func (a *Adapter) GetModifiedFilesHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetModifiedFilesHistory(repoName, period, points)
}

// GetPendingCommitsHistory fetches historical pending commits count data
func (a *Adapter) GetPendingCommitsHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetPendingCommitsHistory(repoName, period, points)
}

// GetEventAnnotations fetches event annotations for a specific period
func (a *Adapter) GetEventAnnotations(period time.Duration) ([]models.EventAnnotation, error) {
	return a.db.GetEventAnnotations(period)
}

// AddEventAnnotation adds a new event annotation to storage
func (a *Adapter) AddEventAnnotation(event models.EventAnnotation) error {
	// If ID is not set, generate one based on timestamp
	if event.ID == "" {
		event.ID = fmt.Sprintf("event_%d", time.Now().UnixNano())
	}

	return a.db.AddEventAnnotation(event)
}

// DeleteEventAnnotation removes an event annotation from storage by ID
func (a *Adapter) DeleteEventAnnotation(id string) error {
	return a.db.DeleteEventAnnotation(id)
}

// ExportData exports metrics data to CSV files based on the provided options
func (a *Adapter) ExportData(options interface{}) error {
	// Create default export options
	exportOpts := ExportOptions{
		OutputDir:     filepath.Join(os.Getenv("HOME"), "Downloads", "maz-term_export"),
		Period:        7 * 24 * time.Hour, // Last 7 days
		IncludeSystem: true,
		IncludeHTTP:   true,
		IncludeGit:    true,
	}

	// If options is a map, try to extract values
	if opts, ok := options.(map[string]interface{}); ok {
		// Output directory
		if outputDir, ok := opts["OutputDir"].(string); ok && outputDir != "" {
			exportOpts.OutputDir = outputDir
		}

		// Period
		if period, ok := opts["Period"].(time.Duration); ok && period > 0 {
			exportOpts.Period = period
		}

		// Which metrics to include
		if includeSystem, ok := opts["IncludeSystem"].(bool); ok {
			exportOpts.IncludeSystem = includeSystem
		}

		if includeHTTP, ok := opts["IncludeHTTP"].(bool); ok {
			exportOpts.IncludeHTTP = includeHTTP
		}

		if includeGit, ok := opts["IncludeGit"].(bool); ok {
			exportOpts.IncludeGit = includeGit
		}
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(exportOpts.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create a timestamp for filenames
	timestamp := time.Now().Format("20060102_150405")

	// Export system metrics if enabled
	if exportOpts.IncludeSystem {
		// Export CPU usage data
		cpuData, err := a.db.GetCPUUsageHistory(exportOpts.Period, 1000)
		if err == nil && len(cpuData) > 0 {
			filename := filepath.Join(exportOpts.OutputDir, fmt.Sprintf("cpu_usage_%s.csv", timestamp))
			if err := exportTimeSeriesData(filename, cpuData, "Timestamp", "CPU Usage (%)"); err != nil {
				return err
			}
		}

		// Export memory usage data
		memData, err := a.db.GetMemoryUsageHistory(exportOpts.Period, 1000)
		if err == nil && len(memData) > 0 {
			filename := filepath.Join(exportOpts.OutputDir, fmt.Sprintf("memory_usage_%s.csv", timestamp))
			if err := exportTimeSeriesData(filename, memData, "Timestamp", "Memory Usage (%)"); err != nil {
				return err
			}
		}
	}

	// Export HTTP metrics if enabled
	if exportOpts.IncludeHTTP {
		// Get all endpoints
		endpoints, err := a.db.GetAllEndpoints()
		if err == nil {
			for _, endpoint := range endpoints {
				// Export response time data
				respTimeData, err := a.db.GetHTTPResponseTimeHistory(endpoint, exportOpts.Period, 1000)
				if err == nil && len(respTimeData) > 0 {
					safeEndpoint := sanitizeFileName(endpoint)
					filename := filepath.Join(exportOpts.OutputDir, fmt.Sprintf("http_%s_response_time_%s.csv", safeEndpoint, timestamp))
					if err := exportTimeSeriesData(filename, respTimeData, "Timestamp", "Response Time (ms)"); err != nil {
						return err
					}
				}

				// Export availability data
				availData, err := a.db.GetHTTPAvailabilityHistory(endpoint, exportOpts.Period, 1000)
				if err == nil && len(availData) > 0 {
					safeEndpoint := sanitizeFileName(endpoint)
					filename := filepath.Join(exportOpts.OutputDir, fmt.Sprintf("http_%s_availability_%s.csv", safeEndpoint, timestamp))
					if err := exportTimeSeriesData(filename, availData, "Timestamp", "Availability (%)"); err != nil {
						return err
					}
				}
			}
		}
	}

	// Export Git metrics if enabled
	if exportOpts.IncludeGit {
		// Get repository name (could be enhanced to support multiple repos)
		repoName := "current_repo" // Default name

		// Export commit count data
		commitData, err := a.db.GetCommitCountHistory(repoName, exportOpts.Period, 1000)
		if err == nil && len(commitData) > 0 {
			safeRepo := sanitizeFileName(repoName)
			filename := filepath.Join(exportOpts.OutputDir, fmt.Sprintf("git_%s_commits_%s.csv", safeRepo, timestamp))
			if err := exportTimeSeriesData(filename, commitData, "Timestamp", "Commit Count"); err != nil {
				return err
			}
		}

		// Export modified files data
		modifiedData, err := a.db.GetModifiedFilesHistory(repoName, exportOpts.Period, 1000)
		if err == nil && len(modifiedData) > 0 {
			safeRepo := sanitizeFileName(repoName)
			filename := filepath.Join(exportOpts.OutputDir, fmt.Sprintf("git_%s_modified_files_%s.csv", safeRepo, timestamp))
			if err := exportTimeSeriesData(filename, modifiedData, "Timestamp", "Modified Files"); err != nil {
				return err
			}
		}
	}

	return nil
}

// Helper functions for export

// ExportOptions contains options for exporting data
type ExportOptions struct {
	OutputDir     string        // Directory where to save exported files
	Period        time.Duration // How far back to export data
	IncludeSystem bool          // Whether to include system metrics
	IncludeHTTP   bool          // Whether to include HTTP metrics
	IncludeGit    bool          // Whether to include Git metrics
}

// exportTimeSeriesData exports time series data to a CSV file
func exportTimeSeriesData(filename string, data []models.TimeSeriesPoint, timeHeader, valueHeader string) error {
	// Implementation from export.go, copied here to avoid circular imports
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Create CSV writer
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{timeHeader, valueHeader}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write data
	for _, point := range data {
		// Format timestamp as ISO format
		timestamp := point.Timestamp.Format(time.RFC3339)

		// Format value
		value := fmt.Sprintf("%.2f", point.Value)

		if err := writer.Write([]string{timestamp, value}); err != nil {
			return fmt.Errorf("failed to write data: %w", err)
		}
	}

	return nil
}

// sanitizeFileName removes invalid characters from a filename
func sanitizeFileName(name string) string {
	// Implementation from export.go
	invalidChars := `<>:"/\|?*`
	result := name
	for _, char := range invalidChars {
		result = filepath.Clean(result)
		result = filepath.Base(result)
		result = strings.ReplaceAll(result, string(char), "_")
	}
	return result
}

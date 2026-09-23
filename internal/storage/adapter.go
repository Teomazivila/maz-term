package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// StoreSystemMetrics stores system metrics in the database.
func (a *Adapter) StoreSystemMetrics(metrics models.SystemMetrics) error {
	return a.db.StoreSystemMetrics(metrics)
}

// StoreHTTPMetrics stores HTTP metrics for a single endpoint.
func (a *Adapter) StoreHTTPMetrics(name string, metrics models.EndpointMetrics) error {
	return a.db.StoreHTTPMetrics(name, metrics)
}

// StoreGitMetrics stores Git repository metrics.
func (a *Adapter) StoreGitMetrics(metrics models.GitRepoMetrics) error {
	return a.db.StoreGitMetrics(metrics)
}

// StoreCloudSummary stores one cloud provider aggregate.
func (a *Adapter) StoreCloudSummary(summary models.CloudSummary) error {
	return a.db.StoreCloudSummary(summary)
}

// StoreKubernetesSummary stores one cluster aggregate.
func (a *Adapter) StoreKubernetesSummary(summary models.KubernetesSummary) error {
	return a.db.StoreKubernetesSummary(summary)
}

// StoreCICDSummary stores one CI/CD provider aggregate.
func (a *Adapter) StoreCICDSummary(summary models.CICDSummary) error {
	return a.db.StoreCICDSummary(summary)
}

// GetCloudInstanceCountHistory returns the running-instance count over time.
func (a *Adapter) GetCloudInstanceCountHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetCloudInstanceCountHistory(period, points)
}

// GetCloudCPUHistory returns mean instance CPU utilisation over time.
func (a *Adapter) GetCloudCPUHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetCloudCPUHistory(period, points)
}

// GetKubernetesPodCountHistory returns the running-pod count over time.
func (a *Adapter) GetKubernetesPodCountHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetKubernetesPodCountHistory(period, points)
}

// GetKubernetesNodeReadyHistory returns the ready-node count over time.
func (a *Adapter) GetKubernetesNodeReadyHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetKubernetesNodeReadyHistory(period, points)
}

// GetCICDSuccessRateHistory returns the workflow success rate over time.
func (a *Adapter) GetCICDSuccessRateHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return a.db.GetCICDSuccessRateHistory(period, points)
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

// ExportAll writes every metric family covering the given period and returns the
// paths written. An empty outputDir or non-positive period uses the defaults.
//
// It exists so callers can request an export without depending on the
// ExportOptions type, keeping the UI's storage interface free of storage types.
func (a *Adapter) ExportAll(outputDir string, period time.Duration) ([]string, error) {
	opts, err := DefaultExportOptions()
	if err != nil {
		return nil, err
	}
	if outputDir != "" {
		opts.OutputDir = outputDir
	}
	if period > 0 {
		opts.Period = period
	}

	return a.ExportData(opts)
}

// DefaultExportOptions returns the export defaults: the last seven days of every
// metric family, written under the user's home directory.
//
// os.UserHomeDir is used rather than os.Getenv("HOME"), which is empty on
// Windows and would silently export to a path relative to the process's working
// directory.
func DefaultExportOptions() (ExportOptions, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ExportOptions{}, fmt.Errorf("resolving home directory: %w", err)
	}

	return ExportOptions{
		OutputDir:     filepath.Join(home, "maz-term-exports"),
		Period:        7 * 24 * time.Hour,
		IncludeSystem: true,
		IncludeHTTP:   true,
		IncludeGit:    true,
	}, nil
}

// ExportData writes the selected metrics to CSV files and returns the paths
// written. An empty OutputDir or non-positive Period falls back to the defaults.
func (a *Adapter) ExportData(opts ExportOptions) ([]string, error) {
	defaults, err := DefaultExportOptions()
	if err != nil {
		return nil, err
	}

	exportOpts := opts
	if exportOpts.OutputDir == "" {
		exportOpts.OutputDir = defaults.OutputDir
	}
	if exportOpts.Period <= 0 {
		exportOpts.Period = defaults.Period
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(exportOpts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	var written []string

	// Create a timestamp for filenames
	timestamp := time.Now().Format("20060102_150405")

	// export writes one series and records the path. A read failure is fatal:
	// silently producing no file made a broken query look like an empty range.
	export := func(name string, points []models.TimeSeriesPoint, valueHeader string) error {
		if len(points) == 0 {
			return nil
		}
		filename := filepath.Join(exportOpts.OutputDir, fmt.Sprintf("%s_%s.csv", name, timestamp))
		if err := exportTimeSeriesData(filename, points, "Timestamp", valueHeader); err != nil {
			return err
		}
		written = append(written, filename)
		return nil
	}

	if exportOpts.IncludeSystem {
		cpuData, err := a.db.GetCPUUsageHistory(exportOpts.Period, exportRowLimit)
		if err != nil {
			return nil, fmt.Errorf("reading CPU history: %w", err)
		}
		if err := export("cpu_usage", cpuData, "CPU Usage (%)"); err != nil {
			return nil, err
		}

		memData, err := a.db.GetMemoryUsageHistory(exportOpts.Period, exportRowLimit)
		if err != nil {
			return nil, fmt.Errorf("reading memory history: %w", err)
		}
		if err := export("memory_usage", memData, "Memory Usage (%)"); err != nil {
			return nil, err
		}
	}

	if exportOpts.IncludeHTTP {
		endpoints, err := a.db.GetAllEndpoints()
		if err != nil {
			return nil, fmt.Errorf("listing endpoints: %w", err)
		}

		for _, endpoint := range endpoints {
			safe := sanitizeFileName(endpoint)

			respTime, err := a.db.GetHTTPResponseTimeHistory(endpoint, exportOpts.Period, exportRowLimit)
			if err != nil {
				return nil, fmt.Errorf("reading response-time history for %s: %w", endpoint, err)
			}
			if err := export("http_"+safe+"_response_time", respTime, "Response Time (ms)"); err != nil {
				return nil, err
			}

			availability, err := a.db.GetHTTPAvailabilityHistory(endpoint, exportOpts.Period, exportRowLimit)
			if err != nil {
				return nil, fmt.Errorf("reading availability history for %s: %w", endpoint, err)
			}
			if err := export("http_"+safe+"_availability", availability, "Availability (%)"); err != nil {
				return nil, err
			}
		}
	}

	if exportOpts.IncludeGit {
		repos, err := a.db.GetAllGitRepositories()
		if err != nil {
			return nil, fmt.Errorf("listing git repositories: %w", err)
		}

		for _, repo := range repos {
			safe := sanitizeFileName(repo)

			commits, err := a.db.GetCommitCountHistory(repo, exportOpts.Period, exportRowLimit)
			if err != nil {
				return nil, fmt.Errorf("reading commit history for %s: %w", repo, err)
			}
			if err := export("git_"+safe+"_commits", commits, "Commit Count"); err != nil {
				return nil, err
			}

			modified, err := a.db.GetModifiedFilesHistory(repo, exportOpts.Period, exportRowLimit)
			if err != nil {
				return nil, fmt.Errorf("reading modified-file history for %s: %w", repo, err)
			}
			if err := export("git_"+safe+"_modified_files", modified, "Modified Files"); err != nil {
				return nil, err
			}
		}
	}

	return written, nil
}

// exportRowLimit caps how many points each exported series contains.
const exportRowLimit = 10000

// ExportOptions contains options for exporting data
type ExportOptions struct {
	OutputDir     string        // Directory where to save exported files
	Period        time.Duration // How far back to export data
	IncludeSystem bool          // Whether to include system metrics
	IncludeHTTP   bool          // Whether to include HTTP metrics
	IncludeGit    bool          // Whether to include Git metrics
}

// exportTimeSeriesData writes one time series to a CSV file.
//
// Both the writer flush and the file close are checked. Deferring them
// unchecked, as an earlier revision did, discards short-write and
// disk-full errors and produces a silently truncated export.
func exportTimeSeriesData(filename string, data []models.TimeSeriesPoint, timeHeader, valueHeader string) (err error) {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating %s: %w", filename, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing %s: %w", filename, closeErr)
		}
	}()

	writer := csv.NewWriter(file)

	if err := writer.Write([]string{timeHeader, valueHeader}); err != nil {
		return fmt.Errorf("writing header to %s: %w", filename, err)
	}

	for _, point := range data {
		record := []string{
			point.Timestamp.Format(time.RFC3339),
			strconv.FormatFloat(point.Value, 'f', 2, 64),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("writing row to %s: %w", filename, err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flushing %s: %w", filename, err)
	}

	return nil
}

// sanitizeFileName reduces an arbitrary endpoint or repository name to a safe
// single path element.
//
// The previous implementation called filepath.Clean and filepath.Base once per
// character of its blocklist, which meant separators were stripped by Base
// rather than replaced and the loop did the same work nine times.
func sanitizeFileName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r == 0x7f:
			return '_'
		case strings.ContainsRune(`<>:"/\|?*`, r):
			return '_'
		default:
			return r
		}
	}, name)

	// Checked before filepath.Base, which maps an empty path to ".".
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return "unnamed"
	}

	// Collapse to a single element so no input can traverse directories, and
	// reject the relative-path names outright.
	cleaned = filepath.Base(cleaned)
	if cleaned == "." || cleaned == ".." || cleaned == string(filepath.Separator) {
		return "_"
	}

	return cleaned
}

// AddNotification adds a new notification
func (a *Adapter) AddNotification(notification models.Notification) error {
	return a.db.AddNotification(notification)
}

// GetNotifications retrieves notifications from the database
func (a *Adapter) GetNotifications(count int, includeRead bool) ([]models.Notification, error) {
	return a.db.GetNotifications(count, includeRead)
}

// MarkAsRead marks a notification as read
func (a *Adapter) MarkAsRead(id string) error {
	return a.db.MarkAsRead(id)
}

// DismissNotification marks a notification as dismissed
func (a *Adapter) DismissNotification(id string) error {
	return a.db.DismissNotification(id)
}

// ClearAllNotifications marks all notifications as dismissed
func (a *Adapter) ClearAllNotifications() error {
	return a.db.ClearAllNotifications()
}

// GetUnreadNotificationCount returns the count of unread notifications
func (a *Adapter) GetUnreadNotificationCount() (int, error) {
	return a.db.GetUnreadNotificationCount()
}

// GetFilteredNotifications retrieves notifications matching the given sources
// and severities.
//
// Filtering happens entirely in SQL. Re-filtering here over a bounded row window
// silently dropped matches that fell outside it.
func (a *Adapter) GetFilteredNotifications(count int, includeRead bool, sources []string, severities []string) ([]models.Notification, error) {
	return a.db.GetFilteredNotifications(count, includeRead, sources, severities)
}

// GetNotificationSources returns the distinct notification sources on record.
func (a *Adapter) GetNotificationSources() ([]string, error) {
	return a.db.GetNotificationSources()
}

// GetNotificationSeverities returns the distinct notification severities on record.
func (a *Adapter) GetNotificationSeverities() ([]string, error) {
	return a.db.GetNotificationSeverities()
}

// GetAllGitRepositories returns the repository names with recorded metrics.
func (a *Adapter) GetAllGitRepositories() ([]string, error) {
	return a.db.GetAllGitRepositories()
}

// Close closes the storage adapter and underlying database connection
func (a *Adapter) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

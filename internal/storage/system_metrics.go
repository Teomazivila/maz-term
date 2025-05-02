package storage

import (
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// StoreSystemMetrics saves system metrics to the database
func (d *Database) StoreSystemMetrics(metrics models.SystemMetrics) error {
	// Begin a transaction
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store CPU and memory metrics
	timestamp := metrics.CollectedAt.Unix()
	_, err = tx.Exec(
		"INSERT INTO system_metrics (timestamp, cpu_usage, memory_usage, memory_total, memory_used) VALUES (?, ?, ?, ?, ?)",
		timestamp,
		metrics.CPU.UsagePercent,
		metrics.Memory.UsagePercent,
		metrics.Memory.Total,
		metrics.Memory.Used,
	)
	if err != nil {
		return fmt.Errorf("failed to insert system metrics: %w", err)
	}

	// Store disk metrics
	diskStmt, err := tx.Prepare(
		"INSERT INTO disk_metrics (timestamp, mount_point, total, used, usage_percent) VALUES (?, ?, ?, ?, ?)",
	)
	if err != nil {
		return fmt.Errorf("failed to prepare disk metrics statement: %w", err)
	}
	defer diskStmt.Close()

	for _, fs := range metrics.Disk.Filesystems {
		_, err = diskStmt.Exec(
			timestamp,
			fs.MountPoint,
			fs.Total,
			fs.Used,
			fs.UsagePercent,
		)
		if err != nil {
			return fmt.Errorf("failed to insert disk metrics for %s: %w", fs.MountPoint, err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetCPUUsageHistory fetches historical CPU usage data
func (d *Database) GetCPUUsageHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	// Calculate the time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query the database
	rows, err := d.db.Query(
		"SELECT timestamp, cpu_usage FROM system_metrics WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		startTime.Unix(),
		endTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query CPU history: %w", err)
	}
	defer rows.Close()

	// Collect all data points
	var allPoints []models.TimeSeriesPoint
	for rows.Next() {
		var timestamp int64
		var value float64
		if err := rows.Scan(&timestamp, &value); err != nil {
			return nil, fmt.Errorf("failed to scan CPU history row: %w", err)
		}
		allPoints = append(allPoints, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     value,
		})
	}

	// If there aren't enough points, return all we have
	if len(allPoints) <= points {
		return allPoints, nil
	}

	// Down-sample the data to the requested number of points
	return downsampleTimeSeries(allPoints, points), nil
}

// GetMemoryUsageHistory fetches historical memory usage data
func (d *Database) GetMemoryUsageHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	// Calculate the time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query the database
	rows, err := d.db.Query(
		"SELECT timestamp, memory_usage FROM system_metrics WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		startTime.Unix(),
		endTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query memory history: %w", err)
	}
	defer rows.Close()

	// Collect all data points
	var allPoints []models.TimeSeriesPoint
	for rows.Next() {
		var timestamp int64
		var value float64
		if err := rows.Scan(&timestamp, &value); err != nil {
			return nil, fmt.Errorf("failed to scan memory history row: %w", err)
		}
		allPoints = append(allPoints, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     value,
		})
	}

	// If there aren't enough points, return all we have
	if len(allPoints) <= points {
		return allPoints, nil
	}

	// Down-sample the data to the requested number of points
	return downsampleTimeSeries(allPoints, points), nil
}

// GetDiskUsageHistory fetches historical disk usage data for a specific mount point
func (d *Database) GetDiskUsageHistory(mountPoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	// Calculate the time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query the database
	rows, err := d.db.Query(
		"SELECT timestamp, usage_percent FROM disk_metrics WHERE mount_point = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		mountPoint,
		startTime.Unix(),
		endTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query disk history: %w", err)
	}
	defer rows.Close()

	// Collect all data points
	var allPoints []models.TimeSeriesPoint
	for rows.Next() {
		var timestamp int64
		var value float64
		if err := rows.Scan(&timestamp, &value); err != nil {
			return nil, fmt.Errorf("failed to scan disk history row: %w", err)
		}
		allPoints = append(allPoints, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     value,
		})
	}

	// If there aren't enough points, return all we have
	if len(allPoints) <= points {
		return allPoints, nil
	}

	// Down-sample the data to the requested number of points
	return downsampleTimeSeries(allPoints, points), nil
}

// downsampleTimeSeries reduces the number of points in a time series to the specified count
func downsampleTimeSeries(points []models.TimeSeriesPoint, targetCount int) []models.TimeSeriesPoint {
	if len(points) <= targetCount {
		return points
	}

	// Calculate the step size
	step := float64(len(points)) / float64(targetCount)
	result := make([]models.TimeSeriesPoint, targetCount)

	// Sample at regular intervals
	for i := 0; i < targetCount; i++ {
		idx := int(float64(i) * step)
		if idx >= len(points) {
			idx = len(points) - 1
		}
		result[i] = points[idx]
	}

	return result
}

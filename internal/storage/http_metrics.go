package storage

import (
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// StoreHTTPMetrics saves HTTP endpoint metrics to the database
func (d *Database) StoreHTTPMetrics(name string, metrics models.EndpointMetrics) error {
	// Convert boolean to integer for SQLite
	isUp := 0
	if metrics.IsUp {
		isUp = 1
	}

	// Insert the metrics
	_, err := d.db.Exec(
		"INSERT INTO http_metrics (timestamp, endpoint_name, endpoint_url, status_code, response_time, is_up) VALUES (?, ?, ?, ?, ?, ?)",
		metrics.LastChecked.Unix(),
		name,
		metrics.URL,
		metrics.StatusCode,
		metrics.ResponseTime.Microseconds(),
		isUp,
	)
	if err != nil {
		return fmt.Errorf("failed to insert HTTP metrics: %w", err)
	}

	return nil
}

// GetHTTPResponseTimeHistory fetches historical response time data for a specific endpoint
func (d *Database) GetHTTPResponseTimeHistory(endpoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	// Calculate the time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query the database
	rows, err := d.db.Query(
		"SELECT timestamp, response_time FROM http_metrics WHERE endpoint_name = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		endpoint,
		startTime.Unix(),
		endTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query HTTP response time history: %w", err)
	}
	defer rows.Close()

	// Collect all data points
	var allPoints []models.TimeSeriesPoint
	for rows.Next() {
		var timestamp int64
		var responseTimeMicros int64
		if err := rows.Scan(&timestamp, &responseTimeMicros); err != nil {
			return nil, fmt.Errorf("failed to scan HTTP history row: %w", err)
		}

		// Convert microseconds to milliseconds for display
		responseTimeMs := float64(responseTimeMicros) / 1000.0

		allPoints = append(allPoints, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     responseTimeMs,
		})
	}

	// If there aren't enough points, return all we have
	if len(allPoints) <= points {
		return allPoints, nil
	}

	// Down-sample the data to the requested number of points
	return downsampleTimeSeries(allPoints, points), nil
}

// GetHTTPAvailabilityHistory fetches historical availability data for a specific endpoint
func (d *Database) GetHTTPAvailabilityHistory(endpoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	// Calculate the time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query the database
	rows, err := d.db.Query(
		"SELECT timestamp, is_up FROM http_metrics WHERE endpoint_name = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		endpoint,
		startTime.Unix(),
		endTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query HTTP availability history: %w", err)
	}
	defer rows.Close()

	// Collect all data points
	var allPoints []models.TimeSeriesPoint
	for rows.Next() {
		var timestamp int64
		var isUp int
		if err := rows.Scan(&timestamp, &isUp); err != nil {
			return nil, fmt.Errorf("failed to scan HTTP availability row: %w", err)
		}

		// Convert integer to percentage (0% or 100%)
		availability := float64(isUp) * 100.0

		allPoints = append(allPoints, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     availability,
		})
	}

	// If there aren't enough points, return all we have
	if len(allPoints) <= points {
		return allPoints, nil
	}

	// Down-sample the data to the requested number of points
	return downsampleTimeSeries(allPoints, points), nil
}

// GetAllEndpoints returns a list of all monitored HTTP endpoints
func (d *Database) GetAllEndpoints() ([]string, error) {
	// Query the database for distinct endpoint names
	rows, err := d.db.Query("SELECT DISTINCT endpoint_name FROM http_metrics ORDER BY endpoint_name")
	if err != nil {
		return nil, fmt.Errorf("failed to query endpoints: %w", err)
	}
	defer rows.Close()

	// Collect endpoint names
	var endpoints []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint name: %w", err)
		}
		endpoints = append(endpoints, name)
	}

	return endpoints, nil
}

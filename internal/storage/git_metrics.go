package storage

import (
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// StoreGitMetrics saves Git repository metrics to the database
func (d *Database) StoreGitMetrics(metrics models.GitRepoMetrics) error {
	// Insert the metrics
	_, err := d.db.Exec(
		"INSERT INTO git_metrics (timestamp, repo_name, branch, commit_count, modified_files, pending_commits) VALUES (?, ?, ?, ?, ?, ?)",
		time.Now().Unix(),
		metrics.Name,
		metrics.Branch,
		metrics.CommitCount,
		metrics.ModifiedFiles,
		metrics.PendingCommits,
	)
	if err != nil {
		return fmt.Errorf("failed to insert Git metrics: %w", err)
	}

	return nil
}

// GetCommitCountHistory fetches historical commit count data
func (d *Database) GetCommitCountHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	// Calculate the time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query the database
	rows, err := d.db.Query(
		"SELECT timestamp, commit_count FROM git_metrics WHERE repo_name = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		repoName,
		startTime.Unix(),
		endTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query commit count history: %w", err)
	}
	defer rows.Close()

	// Collect all data points
	var allPoints []models.TimeSeriesPoint
	for rows.Next() {
		var timestamp int64
		var commitCount int
		if err := rows.Scan(&timestamp, &commitCount); err != nil {
			return nil, fmt.Errorf("failed to scan commit count history row: %w", err)
		}

		allPoints = append(allPoints, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     float64(commitCount),
		})
	}

	// If there aren't enough points, return all we have
	if len(allPoints) <= points {
		return allPoints, nil
	}

	// Down-sample the data to the requested number of points
	return downsampleTimeSeries(allPoints, points), nil
}

// GetModifiedFilesHistory fetches historical modified files count data
func (d *Database) GetModifiedFilesHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	// Calculate the time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query the database
	rows, err := d.db.Query(
		"SELECT timestamp, modified_files FROM git_metrics WHERE repo_name = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		repoName,
		startTime.Unix(),
		endTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query modified files history: %w", err)
	}
	defer rows.Close()

	// Collect all data points
	var allPoints []models.TimeSeriesPoint
	for rows.Next() {
		var timestamp int64
		var modifiedFiles int
		if err := rows.Scan(&timestamp, &modifiedFiles); err != nil {
			return nil, fmt.Errorf("failed to scan modified files history row: %w", err)
		}

		allPoints = append(allPoints, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     float64(modifiedFiles),
		})
	}

	// If there aren't enough points, return all we have
	if len(allPoints) <= points {
		return allPoints, nil
	}

	// Down-sample the data to the requested number of points
	return downsampleTimeSeries(allPoints, points), nil
}

// GetPendingCommitsHistory fetches historical pending commits count data
func (d *Database) GetPendingCommitsHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	// Calculate the time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query the database
	rows, err := d.db.Query(
		"SELECT timestamp, pending_commits FROM git_metrics WHERE repo_name = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		repoName,
		startTime.Unix(),
		endTime.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending commits history: %w", err)
	}
	defer rows.Close()

	// Collect all data points
	var allPoints []models.TimeSeriesPoint
	for rows.Next() {
		var timestamp int64
		var pendingCommits int
		if err := rows.Scan(&timestamp, &pendingCommits); err != nil {
			return nil, fmt.Errorf("failed to scan pending commits history row: %w", err)
		}

		allPoints = append(allPoints, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     float64(pendingCommits),
		})
	}

	// If there aren't enough points, return all we have
	if len(allPoints) <= points {
		return allPoints, nil
	}

	// Down-sample the data to the requested number of points
	return downsampleTimeSeries(allPoints, points), nil
}

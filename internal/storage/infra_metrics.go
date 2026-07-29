package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// initInfraSchema creates the infrastructure summary tables.
//
// Only aggregates are stored, not per-resource rows: the History tab plots time
// series and nothing plots individual instances or pods. See
// docs/adr/0001-infrastructure-integrations.md.
func (d *Database) initInfraSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS cloud_summary (
			id INTEGER PRIMARY KEY,
			timestamp INTEGER NOT NULL,
			provider_type TEXT NOT NULL,
			instances_total INTEGER NOT NULL,
			instances_running INTEGER NOT NULL,
			instances_stopped INTEGER NOT NULL,
			mean_cpu_utilization REAL NOT NULL,
			buckets_total INTEGER NOT NULL,
			databases_total INTEGER NOT NULL,
			error_count INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS kubernetes_summary (
			id INTEGER PRIMARY KEY,
			timestamp INTEGER NOT NULL,
			cluster_name TEXT NOT NULL,
			nodes_total INTEGER NOT NULL,
			nodes_ready INTEGER NOT NULL,
			pods_total INTEGER NOT NULL,
			pods_running INTEGER NOT NULL,
			pods_pending INTEGER NOT NULL,
			pods_failed INTEGER NOT NULL,
			pods_succeeded INTEGER NOT NULL,
			deployments_total INTEGER NOT NULL,
			deployments_available INTEGER NOT NULL,
			restarts_total INTEGER NOT NULL,
			error_count INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS cicd_summary (
			id INTEGER PRIMARY KEY,
			timestamp INTEGER NOT NULL,
			provider_type TEXT NOT NULL,
			workflows_total INTEGER NOT NULL,
			workflows_failed INTEGER NOT NULL,
			runs_running INTEGER NOT NULL,
			success_rate REAL NOT NULL,
			mean_duration_seconds REAL NOT NULL,
			error_count INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cloud_summary_timestamp ON cloud_summary(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_kubernetes_summary_timestamp ON kubernetes_summary(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_cicd_summary_timestamp ON cicd_summary(timestamp)`,
	}

	for _, statement := range statements {
		if _, err := d.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to create infrastructure schema: %w", err)
		}
	}

	return nil
}

// StoreCloudSummary records one cloud provider aggregate.
func (d *Database) StoreCloudSummary(summary models.CloudSummary) error {
	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	_, err := d.db.ExecContext(ctx, `
	INSERT INTO cloud_summary (
		timestamp, provider_type, instances_total, instances_running,
		instances_stopped, mean_cpu_utilization, buckets_total, databases_total,
		error_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		timestampOf(summary.Timestamp),
		summary.ProviderType,
		summary.InstancesTotal,
		summary.InstancesRunning,
		summary.InstancesStopped,
		summary.MeanCPUUtilization,
		summary.BucketsTotal,
		summary.DatabasesTotal,
		summary.ErrorCount,
	)
	if err != nil {
		return fmt.Errorf("inserting cloud summary: %w", err)
	}

	return nil
}

// StoreKubernetesSummary records one cluster aggregate.
func (d *Database) StoreKubernetesSummary(summary models.KubernetesSummary) error {
	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	_, err := d.db.ExecContext(ctx, `
	INSERT INTO kubernetes_summary (
		timestamp, cluster_name, nodes_total, nodes_ready, pods_total,
		pods_running, pods_pending, pods_failed, pods_succeeded,
		deployments_total, deployments_available, restarts_total, error_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		timestampOf(summary.Timestamp),
		summary.ClusterName,
		summary.NodesTotal,
		summary.NodesReady,
		summary.PodsTotal,
		summary.PodsRunning,
		summary.PodsPending,
		summary.PodsFailed,
		summary.PodsSucceeded,
		summary.DeploymentsTotal,
		summary.DeploymentsAvailable,
		summary.RestartsTotal,
		summary.ErrorCount,
	)
	if err != nil {
		return fmt.Errorf("inserting kubernetes summary: %w", err)
	}

	return nil
}

// StoreCICDSummary records one CI/CD provider aggregate.
func (d *Database) StoreCICDSummary(summary models.CICDSummary) error {
	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	_, err := d.db.ExecContext(ctx, `
	INSERT INTO cicd_summary (
		timestamp, provider_type, workflows_total, workflows_failed,
		runs_running, success_rate, mean_duration_seconds, error_count
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		timestampOf(summary.Timestamp),
		summary.ProviderType,
		summary.WorkflowsTotal,
		summary.WorkflowsFailed,
		summary.RunsRunning,
		summary.SuccessRate,
		summary.MeanDurationSeconds,
		summary.ErrorCount,
	)
	if err != nil {
		return fmt.Errorf("inserting CI/CD summary: %w", err)
	}

	return nil
}

// GetCloudInstanceCountHistory returns the running-instance count over time.
func (d *Database) GetCloudInstanceCountHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return d.infraSeries("cloud_summary", "instances_running", period, points)
}

// GetCloudCPUHistory returns mean instance CPU utilisation over time.
func (d *Database) GetCloudCPUHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return d.infraSeries("cloud_summary", "mean_cpu_utilization", period, points)
}

// GetKubernetesPodCountHistory returns the running-pod count over time.
func (d *Database) GetKubernetesPodCountHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return d.infraSeries("kubernetes_summary", "pods_running", period, points)
}

// GetKubernetesNodeReadyHistory returns the ready-node count over time.
func (d *Database) GetKubernetesNodeReadyHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return d.infraSeries("kubernetes_summary", "nodes_ready", period, points)
}

// GetCICDSuccessRateHistory returns the workflow success rate over time.
func (d *Database) GetCICDSuccessRateHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	return d.infraSeries("cicd_summary", "success_rate", period, points)
}

// infraSeries reads one numeric column from a summary table as a time series.
//
// table and column are supplied only by this package's own accessors above,
// never by user input.
func (d *Database) infraSeries(table, column string, period time.Duration, points int) ([]models.TimeSeriesPoint, error) {
	if period <= 0 {
		return nil, fmt.Errorf("storage: period must be positive, got %s", period)
	}

	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	end := time.Now()
	start := end.Add(-period)

	query := fmt.Sprintf(
		"SELECT timestamp, %s FROM %s WHERE timestamp >= ? AND timestamp <= ? ORDER BY timestamp ASC",
		column, table)

	rows, err := d.db.QueryContext(ctx, query, start.Unix(), end.Unix())
	if err != nil {
		return nil, fmt.Errorf("querying %s.%s: %w", table, column, err)
	}
	defer rows.Close()

	var series []models.TimeSeriesPoint
	for rows.Next() {
		var (
			timestamp int64
			value     float64
		)
		if err := rows.Scan(&timestamp, &value); err != nil {
			return nil, fmt.Errorf("scanning %s.%s: %w", table, column, err)
		}
		series = append(series, models.TimeSeriesPoint{
			Timestamp: time.Unix(timestamp, 0),
			Value:     value,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating %s.%s: %w", table, column, err)
	}

	if len(series) <= points {
		return series, nil
	}
	return downsampleTimeSeries(series, points), nil
}

// timestampOf returns t as a Unix timestamp, substituting now for a zero value so
// no row is written with a year-1 timestamp that time-window queries exclude.
func timestampOf(t time.Time) int64 {
	if t.IsZero() {
		return time.Now().Unix()
	}
	return t.Unix()
}

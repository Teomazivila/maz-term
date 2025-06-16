package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Database represents a SQLite database connection with proper resource management
type Database struct {
	db              *sql.DB
	dataPath        string
	retentionPeriod time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
	cleanupWG       sync.WaitGroup
	logger          *slog.Logger
}

// Config holds the database configuration
type Config struct {
	DataPath        string        // Path to the database file
	RetentionPeriod time.Duration // How long to keep historical data
	MaxOpenConns    int           // Maximum number of open connections
	MaxIdleConns    int           // Maximum number of idle connections
	ConnMaxLifetime time.Duration // Maximum connection lifetime
	Logger          *slog.Logger  // Structured logger
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	return &Config{
		DataPath:        filepath.Join(homeDir, ".config", "maz-term", "data.db"),
		RetentionPeriod: 7 * 24 * time.Hour, // 7 days
		MaxOpenConns:    25,                 // SQLite recommended max
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		Logger:          slog.Default(),
	}
}

// New creates a new database connection with proper resource management
func New(config *Config) (*Database, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Ensure the directory exists
	dbDir := filepath.Dir(config.DataPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Connect to the database with proper connection string
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_foreign_keys=ON", config.DataPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	// Test the connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create cancellable context for background operations
	dbCtx, dbCancel := context.WithCancel(context.Background())

	// Create the database instance
	database := &Database{
		db:              db,
		dataPath:        config.DataPath,
		retentionPeriod: config.RetentionPeriod,
		ctx:             dbCtx,
		cancel:          dbCancel,
		logger:          config.Logger,
	}

	// Initialize the schema
	if err := database.initSchema(dbCtx); err != nil {
		db.Close()
		dbCancel()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Start periodic cleanup for retention policy
	database.startCleanupTask()

	database.logger.Info("Database initialized successfully",
		"path", config.DataPath,
		"retention_period", config.RetentionPeriod,
		"max_open_conns", config.MaxOpenConns)

	return database, nil
}

// Close closes the database connection with proper cleanup
func (d *Database) Close() error {
	d.logger.Info("Closing database connection")

	// Cancel background operations
	d.cancel()

	// Wait for cleanup tasks to finish
	d.cleanupWG.Wait()

	// Close database connection
	if err := d.db.Close(); err != nil {
		d.logger.Error("Error closing database", "error", err)
		return err
	}

	d.logger.Info("Database connection closed successfully")
	return nil
}

// initSchema creates the database tables if they don't exist
func (d *Database) initSchema(ctx context.Context) error {
	// Create the system_metrics table
	_, err := d.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS system_metrics (
		id INTEGER PRIMARY KEY,
		timestamp INTEGER NOT NULL,
		cpu_usage REAL NOT NULL,
		memory_usage REAL NOT NULL,
		memory_total INTEGER NOT NULL,
		memory_used INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("failed to create system_metrics table: %w", err)
	}

	// Create the disk_metrics table
	_, err = d.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS disk_metrics (
		id INTEGER PRIMARY KEY,
		timestamp INTEGER NOT NULL,
		mount_point TEXT NOT NULL,
		total INTEGER NOT NULL,
		used INTEGER NOT NULL,
		usage_percent REAL NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("failed to create disk_metrics table: %w", err)
	}

	// Create the http_metrics table
	_, err = d.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS http_metrics (
		id INTEGER PRIMARY KEY,
		timestamp INTEGER NOT NULL,
		endpoint_name TEXT NOT NULL,
		endpoint_url TEXT NOT NULL,
		status_code INTEGER,
		response_time INTEGER NOT NULL,
		is_up INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("failed to create http_metrics table: %w", err)
	}

	// Create the git_metrics table
	_, err = d.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS git_metrics (
		id INTEGER PRIMARY KEY,
		timestamp INTEGER NOT NULL,
		repo_name TEXT NOT NULL,
		branch TEXT NOT NULL,
		commit_count INTEGER NOT NULL,
		modified_files INTEGER NOT NULL,
		pending_commits INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("failed to create git_metrics table: %w", err)
	}

	// Create indexes for fast queries
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_system_metrics_timestamp ON system_metrics(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_disk_metrics_timestamp ON disk_metrics(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_http_metrics_timestamp ON http_metrics(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_git_metrics_timestamp ON git_metrics(timestamp)",
	}

	for _, indexSQL := range indexes {
		if _, err := d.db.ExecContext(ctx, indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Initialize notifications schema
	if err := d.InitNotificationsSchema(); err != nil {
		return fmt.Errorf("failed to initialize notifications schema: %w", err)
	}

	return nil
}

// startCleanupTask periodically cleans up old data with proper resource management
func (d *Database) startCleanupTask() {
	d.cleanupWG.Add(1)
	go func() {
		defer d.cleanupWG.Done()

		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := d.cleanupOldData(); err != nil {
					d.logger.Error("Failed to cleanup old data", "error", err)
				}
			case <-d.ctx.Done():
				d.logger.Info("Cleanup task stopped")
				return
			}
		}
	}()
}

// cleanupOldData removes data older than the retention period with proper transaction handling
func (d *Database) cleanupOldData() error {
	ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
	defer cancel()

	// Calculate the cutoff time
	cutoff := time.Now().Add(-d.retentionPeriod).Unix()

	// Begin a transaction with context
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Delete old data from all tables
	tables := []string{"system_metrics", "disk_metrics", "http_metrics", "git_metrics"}
	totalDeleted := 0

	for _, table := range tables {
		result, deleteErr := tx.ExecContext(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE timestamp < ?", table), cutoff)
		if deleteErr != nil {
			err = fmt.Errorf("failed to delete old data from %s: %w", table, deleteErr)
			return err
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
			totalDeleted += int(rowsAffected)
		}
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit cleanup transaction: %w", err)
	}

	if totalDeleted > 0 {
		d.logger.Info("Cleaned up old data",
			"rows_deleted", totalDeleted,
			"cutoff_time", time.Unix(cutoff, 0))
	}

	return nil
}

// GetEventAnnotations fetches event annotations for a specific period
func (db *Database) GetEventAnnotations(period time.Duration) ([]models.EventAnnotation, error) {
	// In a real implementation, this would query the database
	// For now, we'll return a placeholder list of annotations

	// Example annotations
	startTime := time.Now().Add(-period)
	endTime := time.Now()

	// Demo annotations
	annotations := []models.EventAnnotation{
		{
			ID:          "event_001",
			Timestamp:   startTime.Add(period / 4),
			Title:       "System Restart",
			Description: "Scheduled restart for system maintenance",
			Type:        models.EventTypeRestart,
			Severity:    models.SeverityInfo,
			Source:      "system",
			Tags:        []string{"cpu", "memory", "system"},
		},
		{
			ID:          "event_002",
			Timestamp:   startTime.Add(period / 2),
			Title:       "Application Deployment",
			Description: "Deployed version 1.2.3 of the application",
			Type:        models.EventTypeDeployment,
			Severity:    models.SeverityInfo,
			Source:      "ci/cd",
			Tags:        []string{"http", "deployment"},
		},
		{
			ID:          "event_003",
			Timestamp:   startTime.Add(period * 3 / 4),
			Title:       "High CPU Alert",
			Description: "CPU usage exceeded 90% for 5 minutes",
			Type:        models.EventTypeAlert,
			Severity:    models.SeverityWarning,
			Source:      "monitor",
			Tags:        []string{"cpu", "system", "alert"},
		},
	}

	// Filter annotations based on time period
	filteredAnnotations := []models.EventAnnotation{}
	for _, ann := range annotations {
		if (ann.Timestamp.After(startTime) || ann.Timestamp.Equal(startTime)) &&
			(ann.Timestamp.Before(endTime) || ann.Timestamp.Equal(endTime)) {
			filteredAnnotations = append(filteredAnnotations, ann)
		}
	}

	return filteredAnnotations, nil
}

// SeedDemoDataIfEmpty checks if the database is empty and seeds it with demo data
func (d *Database) SeedDemoDataIfEmpty() error {
	// Check if system_metrics table is empty
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM system_metrics").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check if system_metrics table is empty: %w", err)
	}

	// If not empty, return
	if count > 0 {
		return nil
	}

	fmt.Println("Database is empty. Seeding with demo data...")

	// Begin a transaction
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Generate demo data for the past 24 hours
	now := time.Now()

	// Seed system metrics
	for i := 0; i < 100; i++ {
		timestamp := now.Add(-time.Duration(i*15) * time.Minute).Unix()
		cpuUsage := 20.0 + float64(i%30)               // Generate fluctuating CPU usage
		memoryUsage := 45.0 + float64(i%20)            // Generate fluctuating memory usage
		memoryTotal := uint64(16 * 1024 * 1024 * 1024) // 16GB
		memoryUsed := uint64(float64(memoryTotal) * memoryUsage / 100.0)

		_, err = tx.Exec(
			"INSERT INTO system_metrics (timestamp, cpu_usage, memory_usage, memory_total, memory_used) VALUES (?, ?, ?, ?, ?)",
			timestamp,
			cpuUsage,
			memoryUsage,
			memoryTotal,
			memoryUsed,
		)
		if err != nil {
			return fmt.Errorf("failed to insert demo system metrics: %w", err)
		}
	}

	// Seed disk metrics
	mountPoints := []string{"/", "/home", "/var"}
	for i := 0; i < 100; i++ {
		timestamp := now.Add(-time.Duration(i*15) * time.Minute).Unix()

		for _, mp := range mountPoints {
			diskUsage := 30.0 + float64(i%25)         // Generate fluctuating disk usage
			total := uint64(500 * 1024 * 1024 * 1024) // 500GB
			used := uint64(float64(total) * diskUsage / 100.0)

			_, err = tx.Exec(
				"INSERT INTO disk_metrics (timestamp, mount_point, total, used, usage_percent) VALUES (?, ?, ?, ?, ?)",
				timestamp,
				mp,
				total,
				used,
				diskUsage,
			)
			if err != nil {
				return fmt.Errorf("failed to insert demo disk metrics: %w", err)
			}
		}
	}

	// Seed HTTP metrics
	endpoints := []struct {
		name string
		url  string
	}{
		{"Google", "https://www.google.com"},
		{"GitHub", "https://github.com"},
		{"Example", "https://example.com"},
	}

	for i := 0; i < 100; i++ {
		timestamp := now.Add(-time.Duration(i*15) * time.Minute).Unix()

		for _, ep := range endpoints {
			responseTime := 80 + i%150 // 80-230ms
			isUp := rand.Intn(20) > 0  // 95% uptime
			statusCode := 200
			if !isUp {
				statusCode = 500
			}

			_, err = tx.Exec(
				"INSERT INTO http_metrics (timestamp, endpoint_name, endpoint_url, status_code, response_time, is_up) VALUES (?, ?, ?, ?, ?, ?)",
				timestamp,
				ep.name,
				ep.url,
				statusCode,
				responseTime,
				isUp,
			)
			if err != nil {
				return fmt.Errorf("failed to insert demo HTTP metrics: %w", err)
			}
		}
	}

	// Seed Git metrics
	for i := 0; i < 100; i++ {
		timestamp := now.Add(-time.Duration(i*15) * time.Minute).Unix()
		commitCount := 100 + i/2 // Increasing commit count
		modifiedFiles := i % 8   // Fluctuating modified files
		pendingCommits := i % 5  // Fluctuating pending commits

		_, err = tx.Exec(
			"INSERT INTO git_metrics (timestamp, repo_name, branch, commit_count, modified_files, pending_commits) VALUES (?, ?, ?, ?, ?, ?)",
			timestamp,
			"current_repo",
			"main",
			commitCount,
			modifiedFiles,
			pendingCommits,
		)
		if err != nil {
			return fmt.Errorf("failed to insert demo Git metrics: %w", err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	fmt.Println("Successfully seeded database with demo data")
	return nil
}

// AddEventAnnotation adds a new event annotation to storage
func (db *Database) AddEventAnnotation(event models.EventAnnotation) error {
	// In a real implementation, this would insert into the database
	// For now, just return success
	return nil
}

// DeleteEventAnnotation removes an event annotation from storage by ID
func (db *Database) DeleteEventAnnotation(id string) error {
	// In a real implementation, this would delete from the database
	// For now, just return success
	return nil
}

// StoreCloudMetrics stores cloud provider metrics in the database
func (db *Database) StoreCloudMetrics(metrics models.CloudProviderMetrics) error {
	// For the MVP, we'll just log that we received metrics
	// In a full implementation, this would store data in the database

	fmt.Printf("Storing cloud metrics for %s with %d instances\n",
		metrics.ProviderType,
		len(metrics.InstanceMetrics))

	// For now, return success
	return nil
}

// StoreKubernetesMetrics stores Kubernetes metrics in the database
func (db *Database) StoreKubernetesMetrics(metrics models.KubernetesMetrics) error {
	// For the MVP, we'll just log that we received metrics
	// In a full implementation, this would store data in the database

	fmt.Printf("Storing Kubernetes metrics for cluster %s with %d pods\n",
		metrics.ClusterName,
		len(metrics.Pods))

	// For now, return success
	return nil
}

// StoreCICDMetrics stores CI/CD metrics in the database
func (db *Database) StoreCICDMetrics(metrics models.CICDMetrics) error {
	// For the MVP, we'll just log that we received metrics
	// In a full implementation, this would store data in the database

	fmt.Printf("Storing CI/CD metrics for %s provider with %d workflows\n",
		metrics.ProviderType,
		len(metrics.Workflows))

	// For now, return success
	return nil
}

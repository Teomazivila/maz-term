package storage

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Database represents a SQLite database connection
type Database struct {
	db              *sql.DB
	dataPath        string
	retentionPeriod time.Duration
}

// Config holds the database configuration
type Config struct {
	DataPath        string        // Path to the database file
	RetentionPeriod time.Duration // How long to keep historical data
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
	}
}

// New creates a new database connection
func New(config *Config) (*Database, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Ensure the directory exists
	dbDir := filepath.Dir(config.DataPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Connect to the database
	db, err := sql.Open("sqlite3", config.DataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create the database instance
	database := &Database{
		db:              db,
		dataPath:        config.DataPath,
		retentionPeriod: config.RetentionPeriod,
	}

	// Initialize the schema
	if err := database.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Start periodic cleanup for retention policy
	go database.startCleanupTask()

	return database, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// initSchema creates the database tables if they don't exist
func (d *Database) initSchema() error {
	// Create the system_metrics table
	_, err := d.db.Exec(`
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
	_, err = d.db.Exec(`
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
	_, err = d.db.Exec(`
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
	_, err = d.db.Exec(`
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
	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_system_metrics_timestamp ON system_metrics(timestamp)`)
	if err != nil {
		return fmt.Errorf("failed to create system_metrics index: %w", err)
	}

	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_disk_metrics_timestamp ON disk_metrics(timestamp)`)
	if err != nil {
		return fmt.Errorf("failed to create disk_metrics index: %w", err)
	}

	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_http_metrics_timestamp ON http_metrics(timestamp)`)
	if err != nil {
		return fmt.Errorf("failed to create http_metrics index: %w", err)
	}

	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_git_metrics_timestamp ON git_metrics(timestamp)`)
	if err != nil {
		return fmt.Errorf("failed to create git_metrics index: %w", err)
	}

	return nil
}

// startCleanupTask periodically cleans up old data
func (d *Database) startCleanupTask() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		d.cleanupOldData()
	}
}

// cleanupOldData removes data older than the retention period
func (d *Database) cleanupOldData() error {
	// Calculate the cutoff time
	cutoff := time.Now().Add(-d.retentionPeriod).Unix()

	// Begin a transaction
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete old data from system_metrics
	_, err = tx.Exec("DELETE FROM system_metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return fmt.Errorf("failed to delete old system metrics: %w", err)
	}

	// Delete old data from disk_metrics
	_, err = tx.Exec("DELETE FROM disk_metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return fmt.Errorf("failed to delete old disk metrics: %w", err)
	}

	// Delete old data from http_metrics
	_, err = tx.Exec("DELETE FROM http_metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return fmt.Errorf("failed to delete old HTTP metrics: %w", err)
	}

	// Delete old data from git_metrics
	_, err = tx.Exec("DELETE FROM git_metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return fmt.Errorf("failed to delete old Git metrics: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Vacuum the database to reclaim space
	_, err = d.db.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum database: %w", err)
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

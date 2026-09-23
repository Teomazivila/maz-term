package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	_ "modernc.org/sqlite" // pure-Go SQLite driver, so the binary needs no cgo
)

// ErrNotFound is an alias for models.ErrNotFound, re-exported so storage
// callers can reference it without importing models directly.
var ErrNotFound = models.ErrNotFound

// queryTimeout bounds every individual statement so a locked database cannot
// stall the dashboard's render loop indefinitely.
const queryTimeout = 10 * time.Second

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

// New creates a new database connection with proper resource management.
//
// DataPath is required. An empty path would make SQLite open an anonymous
// temporary database that is destroyed on close, silently discarding all
// recorded history, so it is rejected rather than defaulted.
func New(config *Config) (*Database, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if config.DataPath == "" {
		return nil, errors.New("storage: DataPath is required")
	}

	defaults := DefaultConfig()
	if config.RetentionPeriod <= 0 {
		config.RetentionPeriod = defaults.RetentionPeriod
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = defaults.MaxOpenConns
	}
	if config.MaxIdleConns <= 0 {
		config.MaxIdleConns = defaults.MaxIdleConns
	}
	if config.ConnMaxLifetime <= 0 {
		config.ConnMaxLifetime = defaults.ConnMaxLifetime
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	// Ensure the directory exists
	dbDir := filepath.Dir(config.DataPath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// busy_timeout keeps concurrent collector writes from failing outright with
	// SQLITE_BUSY; WAL alone still serialises writers.
	//
	// The driver is modernc.org/sqlite rather than mattn/go-sqlite3 because the
	// latter requires cgo, and every cross-compilation target in the Makefile
	// sets CGO_ENABLED=0. Those builds linked and then failed at runtime with
	// `unknown driver "sqlite3"`, so no released binary could store anything.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_pragma=cache_size(-10000)",
		config.DataPath)
	db, err := sql.Open("sqlite", dsn)
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

	// Create the annotations table
	_, err = d.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS annotations (
		id TEXT PRIMARY KEY,
		timestamp INTEGER NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL DEFAULT 'other',
		severity TEXT NOT NULL DEFAULT 'info',
		source TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]'
	)`)
	if err != nil {
		return fmt.Errorf("failed to create annotations table: %w", err)
	}

	// Create indexes for fast queries. The composite indexes match the
	// (identifier, time-window) shape of the history queries in
	// http_metrics.go, git_metrics.go and system_metrics.go, which would
	// otherwise scan the whole table.
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_system_metrics_timestamp ON system_metrics(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_disk_metrics_timestamp ON disk_metrics(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_disk_metrics_mount_timestamp ON disk_metrics(mount_point, timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_http_metrics_timestamp ON http_metrics(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_http_metrics_endpoint_timestamp ON http_metrics(endpoint_name, timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_git_metrics_timestamp ON git_metrics(timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_git_metrics_repo_timestamp ON git_metrics(repo_name, timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_annotations_timestamp ON annotations(timestamp)",
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

	// Initialize infrastructure summary schema
	if err := d.initInfraSchema(ctx); err != nil {
		return err
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

	// Delete old data from all tables. Annotations are included so they do not
	// outlive the metrics they annotate and grow without bound.
	tables := []string{
		"system_metrics", "disk_metrics", "http_metrics", "git_metrics", "annotations",
		"cloud_summary", "kubernetes_summary", "cicd_summary",
	}
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

// GetEventAnnotations returns the annotations recorded within the given period,
// most recent first.
func (d *Database) GetEventAnnotations(period time.Duration) ([]models.EventAnnotation, error) {
	if period <= 0 {
		return nil, fmt.Errorf("storage: period must be positive, got %s", period)
	}

	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	cutoff := time.Now().Add(-period).Unix()

	rows, err := d.db.QueryContext(ctx, `
	SELECT id, timestamp, title, description, type, severity, source, tags
	FROM annotations
	WHERE timestamp >= ?
	ORDER BY timestamp DESC`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("querying annotations: %w", err)
	}
	defer rows.Close()

	var annotations []models.EventAnnotation
	for rows.Next() {
		var (
			annotation models.EventAnnotation
			timestamp  int64
			tagsJSON   string
		)
		if err := rows.Scan(
			&annotation.ID,
			&timestamp,
			&annotation.Title,
			&annotation.Description,
			&annotation.Type,
			&annotation.Severity,
			&annotation.Source,
			&tagsJSON,
		); err != nil {
			return nil, fmt.Errorf("scanning annotation: %w", err)
		}

		annotation.Timestamp = time.Unix(timestamp, 0)
		if tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &annotation.Tags); err != nil {
				return nil, fmt.Errorf("decoding tags for annotation %s: %w", annotation.ID, err)
			}
		}

		annotations = append(annotations, annotation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating annotations: %w", err)
	}

	return annotations, nil
}

// AddEventAnnotation stores a new annotation. Callers must supply a stable ID so
// the annotation can be deleted later.
func (d *Database) AddEventAnnotation(event models.EventAnnotation) error {
	if event.ID == "" {
		return errors.New("storage: annotation ID is required")
	}
	if event.Title == "" {
		return errors.New("storage: annotation title is required")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	tagsJSON, err := json.Marshal(event.Tags)
	if err != nil {
		return fmt.Errorf("encoding tags for annotation %s: %w", event.ID, err)
	}

	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	if _, err := d.db.ExecContext(ctx, `
	INSERT INTO annotations (id, timestamp, title, description, type, severity, source, tags)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.Timestamp.Unix(),
		event.Title,
		event.Description,
		string(event.Type),
		string(event.Severity),
		event.Source,
		string(tagsJSON),
	); err != nil {
		return fmt.Errorf("inserting annotation %s: %w", event.ID, err)
	}

	d.logger.Debug("annotation stored", "id", event.ID, "title", event.Title)
	return nil
}

// DeleteEventAnnotation removes the annotation with the given ID. It returns an
// error wrapping ErrNotFound when no such annotation exists, so the UI reports
// the truth rather than a successful no-op.
func (d *Database) DeleteEventAnnotation(id string) error {
	if id == "" {
		return errors.New("storage: annotation ID is required")
	}

	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	result, err := d.db.ExecContext(ctx, "DELETE FROM annotations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting annotation %s: %w", id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("counting deleted annotations: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("annotation %s: %w", id, ErrNotFound)
	}

	return nil
}

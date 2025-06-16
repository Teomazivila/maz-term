package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Teomazivila/maz-term/internal/storage"
	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/ui"
)

// Version is set during build time
var Version = "dev"

func main() {
	// Initialize structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	noStorageFlag := flag.Bool("no-storage", false, "Disable metrics storage")
	debugFlag := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Set debug logging if requested
	if *debugFlag {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)
	}

	if *versionFlag {
		fmt.Println("DevOps Terminal Dashboard v" + Version)
		os.Exit(0)
	}

	logger.Info("Starting DevOps Terminal Dashboard", "version", Version)

	// Create main context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Warn("Error loading config, using defaults", "error", err)
		cfg = config.DefaultConfig()
	}

	// Initialize application components
	app, storageAdapter, collectors, err := initializeApp(ctx, cfg, *noStorageFlag, logger)
	if err != nil {
		logger.Error("Failed to initialize application", "error", err)
		os.Exit(1)
	}

	// Start application in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := app.Run(); err != nil {
			logger.Error("Application error", "error", err)
			cancel() // Trigger shutdown
		}
	}()

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig)
	case <-ctx.Done():
		logger.Info("Application context cancelled")
	}

	// Graceful shutdown
	logger.Info("Starting graceful shutdown...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop collectors
	stopCollectors(collectors, logger)

	// Close storage
	if storageAdapter != nil {
		if err := storageAdapter.Close(); err != nil {
			logger.Error("Error closing storage", "error", err)
		}
	}

	// Wait for application to finish or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("Application shutdown completed")
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout exceeded, forcing exit")
	}

	logger.Info("DevOps Terminal Dashboard stopped")
}

// initializeApp initializes all application components
func initializeApp(ctx context.Context, cfg *config.Config, noStorage bool, logger *slog.Logger) (*ui.App, *storage.Adapter, []collector.Collector, error) {
	var storageAdapter *storage.Adapter
	var collectors []collector.Collector

	// Initialize storage if enabled
	if !noStorage {
		dbConfig := &storage.Config{
			RetentionPeriod: cfg.General.HistoryRetention,
			Logger:          logger,
		}

		db, err := storage.New(dbConfig)
		if err != nil {
			logger.Error("Failed to initialize database", "error", err)
			return nil, nil, nil, fmt.Errorf("database initialization failed: %w", err)
		}

		logger.Info("Database initialized successfully", "retention_period", cfg.General.HistoryRetention)

		// Seed the database with demo data if it's empty
		if err := db.SeedDemoDataIfEmpty(); err != nil {
			logger.Warn("Failed to seed demo data", "error", err)
		}

		storageAdapter = storage.NewAdapter(db)
	}

	// Create app with extended integrations support
	app := ui.NewApp(cfg)

	// Set the storage provider if available
	if storageAdapter != nil {
		app.SetStorageProvider(storageAdapter)
		app.SetStorage(storageAdapter)
	}

	// Initialize collectors with proper lifecycle management
	collectors = initializeCollectors(ctx, app, storageAdapter, logger)

	return app, storageAdapter, collectors, nil
}

// initializeCollectors creates and starts all collectors
func initializeCollectors(ctx context.Context, app *ui.App, storageAdapter *storage.Adapter, logger *slog.Logger) []collector.Collector {
	var collectors []collector.Collector

	logger.Info("Initializing collectors for extended integrations")

	// Initialize mock cloud collector
	cloudCollector := collector.NewMockCloudCollector()
	if storageAdapter != nil {
		cloudCollector.SetStorageProvider(storageAdapter)
	}
	if err := cloudCollector.Start(ctx, 5*time.Second); err != nil {
		logger.Error("Failed to start cloud collector", "error", err)
	} else {
		app.SetCloudCollector(cloudCollector)
		collectors = append(collectors, cloudCollector)
		logger.Debug("Cloud collector started")
	}

	// Initialize mock Kubernetes collector
	k8sCollector := collector.NewMockKubernetesCollector()
	if storageAdapter != nil {
		k8sCollector.SetStorageProvider(storageAdapter)
	}
	if err := k8sCollector.Start(ctx, 5*time.Second); err != nil {
		logger.Error("Failed to start Kubernetes collector", "error", err)
	} else {
		app.SetKubernetesCollector(k8sCollector)
		collectors = append(collectors, k8sCollector)
		logger.Debug("Kubernetes collector started")
	}

	// Initialize mock CI/CD collector
	cicdCollector := collector.NewMockCICDCollector()
	if storageAdapter != nil {
		cicdCollector.SetStorageProvider(storageAdapter)
	}
	if err := cicdCollector.Start(ctx, 5*time.Second); err != nil {
		logger.Error("Failed to start CI/CD collector", "error", err)
	} else {
		app.SetCICDCollector(cicdCollector)
		collectors = append(collectors, cicdCollector)
		logger.Debug("CI/CD collector started")
	}

	logger.Info("Collectors initialized", "count", len(collectors))
	return collectors
}

// stopCollectors gracefully stops all collectors
func stopCollectors(collectors []collector.Collector, logger *slog.Logger) {
	logger.Info("Stopping collectors", "count", len(collectors))

	var wg sync.WaitGroup
	for _, c := range collectors {
		wg.Add(1)
		go func(collector collector.Collector) {
			defer wg.Done()
			if err := collector.Stop(); err != nil {
				logger.Error("Error stopping collector", "collector", collector.Name(), "error", err)
			} else {
				logger.Debug("Collector stopped", "collector", collector.Name())
			}
		}(c)
	}

	// Wait for all collectors to stop with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("All collectors stopped successfully")
	case <-time.After(10 * time.Second):
		logger.Warn("Timeout waiting for collectors to stop")
	}
}

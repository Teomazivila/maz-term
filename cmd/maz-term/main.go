package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Teomazivila/maz-term/internal/storage"
	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/ui"
)

// Version is set during build time
var Version = "dev"

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	noStorageFlag := flag.Bool("no-storage", false, "Disable metrics storage")
	flag.Parse()

	if *versionFlag {
		fmt.Println("DevOps Terminal Dashboard v" + Version)
		os.Exit(0)
	}

	fmt.Println("DevOps Terminal Dashboard v" + Version)
	fmt.Println("Starting application...")

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v, using default configuration\n", err)
		cfg = config.DefaultConfig()
	}

	// Initialize storage if enabled
	var storageAdapter *storage.Adapter
	if !*noStorageFlag {
		// Initialize SQLite database with configuration
		dbConfig := &storage.Config{
			RetentionPeriod: cfg.General.HistoryRetention,
		}
		db, err := storage.New(dbConfig)
		if err != nil {
			fmt.Printf("Error initializing database: %v, metrics will not be stored\n", err)
		} else {
			fmt.Println("Database initialized successfully")
			fmt.Printf("Using retention period of %s\n", cfg.General.HistoryRetention)

			// Seed the database with demo data if it's empty
			if err := db.SeedDemoDataIfEmpty(); err != nil {
				fmt.Printf("Warning: Failed to seed demo data: %v\n", err)
			}

			storageAdapter = storage.NewAdapter(db)
		}
	}

	// Create app with extended integrations support
	app := ui.NewApp(cfg)

	// Set the storage provider if available
	if storageAdapter != nil {
		app.SetStorageProvider(storageAdapter)

		// For the UI storage features
		app.SetStorage(storageAdapter)
	}

	// Initialize and use mock collectors for extended integrations
	fmt.Println("Initializing mock collectors for extended integrations...")

	// Create context for collectors
	ctx := context.Background()

	// Initialize mock cloud collector
	cloudCollector := collector.NewMockCloudCollector()
	cloudCollector.Start(ctx, 5*time.Second)
	app.SetCloudCollector(cloudCollector)

	// Initialize mock Kubernetes collector
	k8sCollector := collector.NewMockKubernetesCollector()
	k8sCollector.Start(ctx, 5*time.Second)
	app.SetKubernetesCollector(k8sCollector)

	// Initialize mock CI/CD collector
	cicdCollector := collector.NewMockCICDCollector()
	cicdCollector.Start(ctx, 5*time.Second)
	app.SetCICDCollector(cicdCollector)

	// Run the TermUI implementation
	if err := app.Run(); err != nil {
		fmt.Printf("Error running application: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Exiting application...")
	os.Exit(0)
}

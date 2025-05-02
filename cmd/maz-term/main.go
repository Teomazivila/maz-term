package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Teomazivila/maz-term/internal/storage"
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
		// Initialize SQLite database
		db, err := storage.New(nil) // Use default configuration
		if err != nil {
			fmt.Printf("Error initializing database: %v, metrics will not be stored\n", err)
		} else {
			fmt.Println("Database initialized successfully")
			storageAdapter = storage.NewAdapter(db)
		}
	}

	// Run the TermUI implementation
	if err := ui.StartApp(cfg, storageAdapter); err != nil {
		fmt.Printf("Error running application: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Exiting application...")
	os.Exit(0)
}

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/ui"
)

// Version is set during build time
var Version = "dev"

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
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

	// Run the TermUI implementation
	if err := ui.StartApp(cfg); err != nil {
		fmt.Printf("Error running application: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Exiting application...")
	os.Exit(0)
}

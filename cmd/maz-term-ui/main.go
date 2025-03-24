package main

import (
	"fmt"
	"os"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/ui"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		fmt.Printf("Error loading config: %v, using default configuration\n", err)
		cfg = config.DefaultConfig()
	}

	// Start the termui application
	if err := ui.StartTermUIApp(cfg); err != nil {
		fmt.Printf("Error running application: %v\n", err)
		os.Exit(1)
	}
}

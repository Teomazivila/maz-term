package main

import (
	"fmt"
	"os"

	"github.com/Teomazivila/maz-term/pkg/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// Version is set during build time
var Version = "dev"

func main() {
	fmt.Println("DevOps Terminal Dashboard v" + Version)
	fmt.Println("Starting application...")

	// Create a new application
	app := ui.NewApp()

	// Run the Bubble Tea program
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running application: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Exiting application...")
	os.Exit(0)
}

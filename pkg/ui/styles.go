package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Default colors
var (
	// Base colors
	ColorBackground = lipgloss.Color("#1E1E2E") // Dark background
	ColorForeground = lipgloss.Color("#CDD6F4") // Light text
	ColorPrimary    = lipgloss.Color("#89B4FA") // Blue accent
	ColorSecondary  = lipgloss.Color("#F5C2E7") // Pink accent
	ColorSuccess    = lipgloss.Color("#A6E3A1") // Green for success
	ColorWarning    = lipgloss.Color("#F9E2AF") // Yellow for warnings
	ColorError      = lipgloss.Color("#F38BA8") // Red for errors
	ColorNeutral    = lipgloss.Color("#9399B2") // Gray for neutral

	// Special purpose colors
	ColorBorder    = lipgloss.Color("#6C7086") // Border color
	ColorHighlight = lipgloss.Color("#CBA6F7") // Highlight color
)

// DefaultStyles contains the default styles for various UI elements
type DefaultStyles struct {
	// General
	App    lipgloss.Style
	Title  lipgloss.Style
	Text   lipgloss.Style
	Subtle lipgloss.Style

	// Components
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	StatusBar   lipgloss.Style
	Panel       lipgloss.Style
	PanelTitle  lipgloss.Style

	// Indicators
	Good    lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style

	// Special
	Selected lipgloss.Style
	Help     lipgloss.Style
}

// NewDefaultStyles creates the default styles
func NewDefaultStyles() DefaultStyles {
	return DefaultStyles{
		// General styles
		App: lipgloss.NewStyle().
			Background(ColorBackground).
			Foreground(ColorForeground),

		Title: lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			MarginBottom(1),

		Text: lipgloss.NewStyle().
			Foreground(ColorForeground),

		Subtle: lipgloss.NewStyle().
			Foreground(ColorNeutral),

		// Component styles
		TabActive: lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(ColorPrimary).
			Padding(0, 2).
			Bold(true),

		TabInactive: lipgloss.NewStyle().
			Foreground(ColorForeground).
			Background(ColorBackground).
			Padding(0, 2).
			Border(lipgloss.Border{
				Top:         "─",
				Bottom:      " ",
				Left:        "│",
				Right:       "│",
				TopLeft:     "┌",
				TopRight:    "┐",
				BottomLeft:  "┘",
				BottomRight: "└",
			}, false, false, false, true).
			BorderForeground(ColorBorder),

		StatusBar: lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(ColorSecondary).
			Padding(0, 1).
			Width(100),

		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 2).
			Margin(0, 1, 1, 0),

		PanelTitle: lipgloss.NewStyle().
			Foreground(ColorHighlight).
			Bold(true).
			Padding(0, 1),

		// Indicator styles
		Good: lipgloss.NewStyle().
			Foreground(ColorSuccess),

		Warning: lipgloss.NewStyle().
			Foreground(ColorWarning),

		Error: lipgloss.NewStyle().
			Foreground(ColorError),

		// Special styles
		Selected: lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(ColorPrimary).
			Padding(0, 1),

		Help: lipgloss.NewStyle().
			Foreground(ColorForeground).
			Background(ColorNeutral).
			Padding(1, 2),
	}
}

// Theme contains the styles for the application
var Theme = NewDefaultStyles()

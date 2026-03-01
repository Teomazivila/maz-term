package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ColorScheme defines a set of colors for a theme
type ColorScheme struct {
	Background lipgloss.Color
	Foreground lipgloss.Color
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Error      lipgloss.Color
	Neutral    lipgloss.Color
	Border     lipgloss.Color
	Highlight  lipgloss.Color
}

// ThemeConfig contains configurable theme options
type ThemeConfig struct {
	Name        string
	ColorScheme ColorScheme
	BorderStyle lipgloss.Border
}

// Available themes
var (
	// DarkTheme is a dark theme with blue accents
	DarkTheme = ThemeConfig{
		Name: "dark",
		ColorScheme: ColorScheme{
			Background: lipgloss.Color("#1E1E2E"),
			Foreground: lipgloss.Color("#CDD6F4"),
			Primary:    lipgloss.Color("#89B4FA"),
			Secondary:  lipgloss.Color("#F5C2E7"),
			Success:    lipgloss.Color("#A6E3A1"),
			Warning:    lipgloss.Color("#F9E2AF"),
			Error:      lipgloss.Color("#F38BA8"),
			Neutral:    lipgloss.Color("#9399B2"),
			Border:     lipgloss.Color("#6C7086"),
			Highlight:  lipgloss.Color("#CBA6F7"),
		},
		BorderStyle: lipgloss.RoundedBorder(),
	}

	// LightTheme is a light theme with blue accents
	LightTheme = ThemeConfig{
		Name: "light",
		ColorScheme: ColorScheme{
			Background: lipgloss.Color("#FFFFFF"),
			Foreground: lipgloss.Color("#1A1A1A"),
			Primary:    lipgloss.Color("#0077CC"),
			Secondary:  lipgloss.Color("#9933CC"),
			Success:    lipgloss.Color("#00AA55"),
			Warning:    lipgloss.Color("#FFBB33"),
			Error:      lipgloss.Color("#F44336"),
			Neutral:    lipgloss.Color("#767676"),
			Border:     lipgloss.Color("#BBBBBB"),
			Highlight:  lipgloss.Color("#33BBEE"),
		},
		BorderStyle: lipgloss.RoundedBorder(),
	}

	// HighContrastTheme is a high contrast theme
	HighContrastTheme = ThemeConfig{
		Name: "high-contrast",
		ColorScheme: ColorScheme{
			Background: lipgloss.Color("#000000"),
			Foreground: lipgloss.Color("#FFFFFF"),
			Primary:    lipgloss.Color("#00FFFF"),
			Secondary:  lipgloss.Color("#FF00FF"),
			Success:    lipgloss.Color("#00FF00"),
			Warning:    lipgloss.Color("#FFFF00"),
			Error:      lipgloss.Color("#FF0000"),
			Neutral:    lipgloss.Color("#AAAAAA"),
			Border:     lipgloss.Color("#FFFFFF"),
			Highlight:  lipgloss.Color("#00FFFF"),
		},
		BorderStyle: lipgloss.NormalBorder(),
	}
)

// ThemeRegistry contains all available themes
var ThemeRegistry = map[string]ThemeConfig{
	"dark":          DarkTheme,
	"light":         LightTheme,
	"high-contrast": HighContrastTheme,
}

// SetTheme sets the active theme
func SetTheme(themeName string) {
	// Default to dark theme if not found
	theme, ok := ThemeRegistry[strings.ToLower(themeName)]
	if !ok {
		theme = DarkTheme
	}

	// Update colors
	ColorBackground = theme.ColorScheme.Background
	ColorForeground = theme.ColorScheme.Foreground
	ColorPrimary = theme.ColorScheme.Primary
	ColorSecondary = theme.ColorScheme.Secondary
	ColorSuccess = theme.ColorScheme.Success
	ColorWarning = theme.ColorScheme.Warning
	ColorError = theme.ColorScheme.Error
	ColorNeutral = theme.ColorScheme.Neutral
	ColorBorder = theme.ColorScheme.Border
	ColorHighlight = theme.ColorScheme.Highlight

	// Recreate the Theme
	Theme = NewDefaultStyles()
}

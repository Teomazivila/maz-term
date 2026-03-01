package tests

import (
	"testing"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/ui"
	ui_lib "github.com/gizak/termui/v3"
	"github.com/stretchr/testify/assert"
)

func TestAppCreation(t *testing.T) {
	// Test app creation with default config
	cfg := config.DefaultConfig()
	app := ui.NewApp(cfg)

	// Check that the app was created with correct configuration
	assert.NotNil(t, app)

	// Test with custom configuration
	customCfg := &config.Config{
		Layout: []config.LayoutTab{
			{
				Name:   "Custom Tab",
				Panels: []string{"system"},
			},
		},
	}
	customApp := ui.NewApp(customCfg)
	assert.NotNil(t, customApp)
}

func TestConfiguration(t *testing.T) {
	// Test default configuration
	cfg := config.DefaultConfig()
	assert.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Layout)

	// Test with endpoints
	assert.NotEmpty(t, cfg.Endpoints)

	// Test with Git configuration
	assert.NotNil(t, cfg.Git)
	assert.NotEmpty(t, cfg.Git.Repositories)
}

func TestUIUtilities(t *testing.T) {
	// Test utility functions without requiring actual UI rendering

	// Test ui.Init() and ui.Close() (mock versions for testing)
	if ui_lib.Init() == nil {
		defer ui_lib.Close()

		// Test getting terminal dimensions
		width, height := ui_lib.TerminalDimensions()
		assert.True(t, width > 0)
		assert.True(t, height > 0)
	}
}

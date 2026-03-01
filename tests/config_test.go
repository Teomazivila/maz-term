package tests

import (
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestCreateDefaultConfig(t *testing.T) {
	// Create a default configuration
	defaultConfig, err := config.LoadConfig("")

	// Assert that no error occurred
	assert.NoError(t, err)

	// Assert that the default configuration is not nil
	assert.NotNil(t, defaultConfig)

	// Assert that the default configuration has the expected values
	assert.Equal(t, 5*time.Second, defaultConfig.General.RefreshInterval)
	assert.Equal(t, "default", defaultConfig.General.Theme)
	assert.Equal(t, 7*24*time.Hour, defaultConfig.General.HistoryRetention)

	// Assert that the default layout has the expected number of tabs
	assert.Equal(t, 4, len(defaultConfig.Layout))

	// Assert that the local metrics are enabled by default
	assert.True(t, defaultConfig.Metrics.Local.Enabled)
}

func TestConfigWithLayout(t *testing.T) {
	// Create a config with layout
	cfg := config.Config{
		Layout: []config.LayoutTab{
			{
				Name:   "System",
				Panels: []string{"cpu", "memory", "disk"},
			},
			{
				Name:   "Git",
				Panels: []string{"git-status"},
			},
		},
	}

	// Validate that layout has tabs with names and panels
	assert.Equal(t, 2, len(cfg.Layout))
	assert.Equal(t, "System", cfg.Layout[0].Name)
	assert.Equal(t, "Git", cfg.Layout[1].Name)
	assert.Equal(t, 3, len(cfg.Layout[0].Panels))
	assert.Equal(t, 1, len(cfg.Layout[1].Panels))

	// Test that empty layout is also valid
	emptyCfg := config.Config{
		Layout: []config.LayoutTab{},
	}
	assert.Equal(t, 0, len(emptyCfg.Layout))
}

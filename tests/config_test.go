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

	// Assert that the default layout has one tab
	assert.Equal(t, 1, len(defaultConfig.Layout))

	// Assert that the local metrics are enabled by default
	assert.True(t, defaultConfig.Metrics.Local.Enabled)
}

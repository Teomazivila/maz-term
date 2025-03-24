package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	General GeneralConfig `mapstructure:"general"`
	Layout  []LayoutTab   `mapstructure:"layout"`
	Metrics MetricsConfig `mapstructure:"metrics"`
}

// GeneralConfig contains general application settings
type GeneralConfig struct {
	RefreshInterval  time.Duration `mapstructure:"refresh"`
	Theme            string        `mapstructure:"theme"`
	HistoryRetention time.Duration `mapstructure:"history_retention"`
}

// LayoutTab represents a tab in the dashboard layout
type LayoutTab struct {
	Name string      `mapstructure:"name"`
	Rows []LayoutRow `mapstructure:"rows"`
}

// LayoutRow represents a row in the dashboard layout
type LayoutRow struct {
	Size   int      `mapstructure:"size"`
	Panels []string `mapstructure:"panels"`
}

// MetricsConfig contains configuration for metrics collection
type MetricsConfig struct {
	Local     LocalMetricsConfig      `mapstructure:"local"`
	Endpoints []EndpointMetricsConfig `mapstructure:"endpoints"`
	Git       GitMetricsConfig        `mapstructure:"git"`
}

// LocalMetricsConfig contains configuration for local system metrics
type LocalMetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// EndpointMetricsConfig contains configuration for HTTP endpoint metrics
type EndpointMetricsConfig struct {
	Name     string        `mapstructure:"name"`
	URL      string        `mapstructure:"url"`
	Method   string        `mapstructure:"method"`
	Interval time.Duration `mapstructure:"interval"`
	Alert    AlertConfig   `mapstructure:"alert"`
}

// AlertConfig contains alert thresholds
type AlertConfig struct {
	StatusCode   int           `mapstructure:"status_code"`
	ResponseTime time.Duration `mapstructure:"response_time"`
}

// GitMetricsConfig contains configuration for Git repository metrics
type GitMetricsConfig struct {
	Repositories []GitRepositoryConfig `mapstructure:"repositories"`
}

// GitRepositoryConfig contains configuration for a Git repository
type GitRepositoryConfig struct {
	Path   string `mapstructure:"path"`
	Remote string `mapstructure:"remote"`
	Branch string `mapstructure:"branch"`
}

// LoadConfig loads the application configuration from the specified file
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("general.refresh", "5s")
	v.SetDefault("general.theme", "default")
	v.SetDefault("general.history_retention", "7d")

	if configPath != "" {
		// Use config file from the flag
		v.SetConfigFile(configPath)
	} else {
		// Search for config in default locations
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.config/maz-term/")
		v.AddConfigPath("/etc/maz-term/")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// Read environment variables
	v.AutomaticEnv()
	v.SetEnvPrefix("MAZTERM")

	// If a config file is found, read it in
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, create default config
			return createDefaultConfig()
		}
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode config into struct: %w", err)
	}

	return &config, nil
}

// createDefaultConfig creates a default configuration
func createDefaultConfig() (*Config, error) {
	defaultConfig := &Config{
		General: GeneralConfig{
			RefreshInterval:  5 * time.Second,
			Theme:            "default",
			HistoryRetention: 7 * 24 * time.Hour,
		},
		Layout: []LayoutTab{
			{
				Name: "System Overview",
				Rows: []LayoutRow{
					{
						Size:   1,
						Panels: []string{"cpu", "memory", "disk", "network"},
					},
					{
						Size:   2,
						Panels: []string{"processes"},
					},
				},
			},
		},
		Metrics: MetricsConfig{
			Local: LocalMetricsConfig{
				Enabled: true,
			},
		},
	}

	return defaultConfig, nil
}

// SaveConfig saves the configuration to a file
func SaveConfig(config *Config, filePath string) error {
	v := viper.New()
	v.SetConfigFile(filePath)

	// Convert config struct to map
	if err := v.MergeConfigMap(map[string]interface{}{
		"general": config.General,
		"layout":  config.Layout,
		"metrics": config.Metrics,
	}); err != nil {
		return fmt.Errorf("error merging config: %w", err)
	}

	// Create directory if it doesn't exist
	dir := filePath[:len(filePath)-len("/config.yaml")]
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	// Write config to file
	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

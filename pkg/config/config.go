package config

import (
	"fmt"
	"os"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/spf13/viper"
)

// GitConfig represents git repository configuration
type GitConfig struct {
	Repositories []GitRepoConfig `mapstructure:"repositories"`
}

// GitRepoConfig represents a git repository configuration
type GitRepoConfig struct {
	Path   string `mapstructure:"path"`
	Remote string `mapstructure:"remote"`
	Branch string `mapstructure:"branch"`
}

// Config represents the application configuration
type Config struct {
	General   GeneralConfig           `mapstructure:"general"`
	Layout    []LayoutTab             `mapstructure:"layout"`
	Metrics   MetricsConfig           `mapstructure:"metrics"`
	Endpoints []models.EndpointConfig `mapstructure:"endpoints"`
	Git       GitConfig               `mapstructure:"git"`
}

// GeneralConfig contains general application settings
type GeneralConfig struct {
	RefreshInterval  time.Duration `mapstructure:"refresh"`
	Theme            string        `mapstructure:"theme"`
	HistoryRetention time.Duration `mapstructure:"history_retention"`
}

// LayoutTab represents a tab in the dashboard layout
type LayoutTab struct {
	Name   string   `mapstructure:"name"`
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
				Name:   "System Overview",
				Panels: []string{"cpu", "memory", "disk", "network"},
			},
			{
				Name:   "System",
				Panels: []string{"system"},
			},
			{
				Name:   "HTTP",
				Panels: []string{"http"},
			},
			{
				Name:   "Git",
				Panels: []string{"git"},
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

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Layout: []LayoutTab{
			{
				Name:   "System",
				Panels: []string{"system"},
			},
			{
				Name:   "HTTP",
				Panels: []string{"http"},
			},
			{
				Name:   "Git",
				Panels: []string{"git"},
			},
		},
		Endpoints: []models.EndpointConfig{
			{Name: "Google", URL: "https://www.google.com", Method: "GET"},
			{Name: "GitHub", URL: "https://github.com", Method: "GET"},
			{Name: "Example", URL: "https://example.com", Method: "GET"},
		},
		Git: GitConfig{
			Repositories: []GitRepoConfig{
				{
					Path:   ".",
					Remote: "origin",
					Branch: "main",
				},
			},
		},
	}
}

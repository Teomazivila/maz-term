// Package config loads and validates the maz-term configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// envPrefix namespaces every environment override, for example
// MAZTERM_GENERAL_REFRESH.
const envPrefix = "MAZTERM"

// minRefreshInterval is the floor for any collection interval. A zero interval
// would make time.NewTicker panic and a sub-second one would spin.
const minRefreshInterval = time.Second

// Config is the complete application configuration.
//
// There is deliberately one place for each setting. An earlier revision declared
// endpoints and git repositories twice, at the top level and again under a
// metrics key, and read the pair that the shipped configuration file did not
// populate, so the documented file silently configured nothing.
type Config struct {
	General    GeneralConfig           `mapstructure:"general"`
	Layout     []LayoutTab             `mapstructure:"layout"`
	Endpoints  []models.EndpointConfig `mapstructure:"endpoints"`
	Git        GitConfig               `mapstructure:"git"`
	Plugins    PluginsConfig           `mapstructure:"plugins"`
	Cloud      CloudConfig             `mapstructure:"cloud"`
	Kubernetes KubernetesConfig        `mapstructure:"kubernetes"`
	CICD       CICDConfig              `mapstructure:"cicd"`
}

// GeneralConfig holds application-wide settings.
type GeneralConfig struct {
	RefreshInterval  time.Duration `mapstructure:"refresh"`
	Theme            string        `mapstructure:"theme"`
	HistoryRetention time.Duration `mapstructure:"history_retention"`
}

// LayoutTab describes one dashboard tab.
type LayoutTab struct {
	Name string `mapstructure:"name"`

	// Rows is the documented nested form. Panels is the shorthand for a tab
	// with a single row. Both are accepted; AllPanels flattens them.
	Rows   []LayoutRow `mapstructure:"rows"`
	Panels []string    `mapstructure:"panels"`
}

// LayoutRow is one row of panels within a tab.
type LayoutRow struct {
	Size   float64  `mapstructure:"size"`
	Panels []string `mapstructure:"panels"`
}

// AllPanels returns every panel in the tab, in order.
func (t LayoutTab) AllPanels() []string {
	panels := append([]string(nil), t.Panels...)
	for _, row := range t.Rows {
		panels = append(panels, row.Panels...)
	}
	return panels
}

// GitConfig lists the repositories to monitor.
type GitConfig struct {
	Repositories []GitRepoConfig `mapstructure:"repositories"`
}

// GitRepoConfig describes one repository.
type GitRepoConfig struct {
	Path   string `mapstructure:"path"`
	Remote string `mapstructure:"remote"`
	Branch string `mapstructure:"branch"`
}

// PluginsConfig configures plugin loading.
type PluginsConfig struct {
	Directory string   `mapstructure:"directory"`
	Enabled   []string `mapstructure:"enabled"`

	// Allow maps a plugin name to the SHA-256 digest of its shared object.
	// Loading a plugin absent from this map is refused, because a plugin runs as
	// native code inside this process.
	Allow map[string]string `mapstructure:"allow"`

	// Settings holds per-plugin configuration, keyed by plugin name.
	Settings map[string]map[string]any `mapstructure:"settings"`
}

// CloudConfig configures cloud provider monitoring.
type CloudConfig struct {
	Enabled []string      `mapstructure:"enabled"`
	Refresh time.Duration `mapstructure:"refresh"`
	AWS     AWSConfig     `mapstructure:"aws"`
}

// AWSConfig configures the AWS integration.
//
// It holds no credential fields. Credentials are resolved by the AWS SDK's
// default chain, which reads the environment, the shared profile and instance
// metadata. Access keys previously appeared here as plain configuration fields,
// in a file that is committed to the repository.
type AWSConfig struct {
	Region            string   `mapstructure:"region"`
	Profile           string   `mapstructure:"profile"`
	AdditionalRegions []string `mapstructure:"additional_regions"`
	Resources         []string `mapstructure:"resources"`
}

// KubernetesConfig configures the Kubernetes integration.
type KubernetesConfig struct {
	Enabled    bool          `mapstructure:"enabled"`
	Refresh    time.Duration `mapstructure:"refresh"`
	ConfigPath string        `mapstructure:"config_path"`
	Context    string        `mapstructure:"context"`
	Namespaces []string      `mapstructure:"namespaces"`
	Resources  []string      `mapstructure:"resources"`
}

// CICDConfig configures CI/CD monitoring.
type CICDConfig struct {
	Enabled []string      `mapstructure:"enabled"`
	Refresh time.Duration `mapstructure:"refresh"`
	GitHub  GitHubConfig  `mapstructure:"github"`
}

// GitHubConfig configures the GitHub Actions integration.
//
// The API token is read from MAZTERM_GITHUB_TOKEN or GITHUB_TOKEN, never from the
// configuration file.
type GitHubConfig struct {
	Owner        string   `mapstructure:"owner"`
	Repositories []string `mapstructure:"repositories"`
	Workflows    []string `mapstructure:"workflows"`
}

// GitHubToken returns the GitHub API token from the environment.
func GitHubToken() string {
	if token := os.Getenv(envPrefix + "_GITHUB_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GITHUB_TOKEN")
}

// forbiddenKeys are configuration keys that would place a credential in a file.
// They are rejected with an explanation rather than quietly ignored.
var forbiddenKeys = map[string]string{
	"cloud.aws.access_key_id":     "use the AWS credential chain (AWS_ACCESS_KEY_ID or a shared profile)",
	"cloud.aws.secret_access_key": "use the AWS credential chain (AWS_SECRET_ACCESS_KEY or a shared profile)",
	"cloud.aws.session_token":     "use the AWS credential chain",
	"cicd.github.token":           "set " + envPrefix + "_GITHUB_TOKEN or GITHUB_TOKEN instead",
}

// envBoundKeys are bound to environment variables explicitly.
//
// Binding is required for overrides to take effect: viper's AllSettings, which
// the decoder consumes, does not include values discovered only through
// AutomaticEnv, so environment configuration silently did nothing.
var envBoundKeys = []string{
	"general.refresh",
	"general.theme",
	"general.history_retention",
	"plugins.directory",
	"cloud.refresh",
	"cloud.aws.region",
	"cloud.aws.profile",
	"kubernetes.enabled",
	"kubernetes.refresh",
	"kubernetes.config_path",
	"kubernetes.context",
	"cicd.refresh",
	"cicd.github.owner",
}

// UserDir returns the per-user maz-term directory, creating it if it does not
// yet exist. It is the default location for the metrics database and the log
// file.
func UserDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}

	dir := filepath.Join(home, ".config", "maz-term")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	return dir, nil
}

// LoadConfig loads the configuration from configPath, or from the default search
// path when configPath is empty. A missing file yields the defaults; a malformed
// or invalid file is an error.
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	v.SetDefault("general.refresh", "5s")
	v.SetDefault("general.theme", "default")
	v.SetDefault("general.history_retention", "7d")
	v.SetDefault("plugins.directory", "")

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	for _, key := range envBoundKeys {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("binding %s: %w", key, err)
		}
	}

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.AddConfigPath(".")
		v.AddConfigPath(filepath.Join("$HOME", ".config", "maz-term"))
		v.AddConfigPath("/etc/maz-term")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || (configPath == "" && os.IsNotExist(err)) {
			cfg := DefaultConfig()
			if err := cfg.normalise(); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	for key, advice := range forbiddenKeys {
		if v.IsSet(key) {
			return nil, fmt.Errorf(
				"%s must not appear in a configuration file: %s", key, advice)
		}
	}

	cfg := DefaultConfig()

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           cfg,
		WeaklyTypedInput: true,
		// A misspelled key is a silent misconfiguration, so unknown keys fail.
		ErrorUnused: true,
		// Decoding happens into a pre-populated defaults struct. Without
		// ZeroFields, mapstructure merges: a layout or endpoint list in the file
		// would be appended to the defaults instead of replacing them. Fields
		// absent from the file are still left at their default, because
		// ZeroFields only clears fields the input actually provides.
		ZeroFields: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			StringToCustomDurationHookFunc(),
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("creating config decoder: %w", err)
	}

	if err := decoder.Decode(v.AllSettings()); err != nil {
		return nil, fmt.Errorf("decoding configuration: %w", err)
	}

	if err := cfg.normalise(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// normalise expands paths and validates the configuration.
func (c *Config) normalise() error {
	if c.General.RefreshInterval < minRefreshInterval {
		return fmt.Errorf("general.refresh must be at least %s, got %s",
			minRefreshInterval, c.General.RefreshInterval)
	}
	if c.General.HistoryRetention <= 0 {
		return fmt.Errorf("general.history_retention must be positive, got %s",
			c.General.HistoryRetention)
	}
	if c.General.HistoryRetention < c.General.RefreshInterval {
		return fmt.Errorf("general.history_retention (%s) must be at least general.refresh (%s)",
			c.General.HistoryRetention, c.General.RefreshInterval)
	}

	seen := make(map[string]struct{}, len(c.Endpoints))
	for i := range c.Endpoints {
		endpoint := &c.Endpoints[i]

		if endpoint.URL == "" {
			return fmt.Errorf("endpoints[%d]: url is required", i)
		}
		parsed, err := url.Parse(endpoint.URL)
		if err != nil {
			return fmt.Errorf("endpoints[%d]: invalid url %q: %w", i, endpoint.URL, err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("endpoints[%d]: url scheme must be http or https, got %q", i, parsed.Scheme)
		}
		if parsed.Host == "" {
			return fmt.Errorf("endpoints[%d]: url %q has no host", i, endpoint.URL)
		}

		if endpoint.Name == "" {
			endpoint.Name = parsed.Host
		}
		if _, duplicate := seen[endpoint.Name]; duplicate {
			return fmt.Errorf("endpoints[%d]: duplicate name %q", i, endpoint.Name)
		}
		seen[endpoint.Name] = struct{}{}

		if endpoint.Method == "" {
			endpoint.Method = "GET"
		}
		endpoint.Method = strings.ToUpper(endpoint.Method)

		if endpoint.Interval != 0 && endpoint.Interval < minRefreshInterval {
			return fmt.Errorf("endpoints[%d]: interval must be at least %s, got %s",
				i, minRefreshInterval, endpoint.Interval)
		}
		if endpoint.Timeout != 0 && endpoint.Timeout <= 0 {
			return fmt.Errorf("endpoints[%d]: timeout must be positive", i)
		}
	}

	// Repository paths are expanded here so a leading ~, which neither Go nor
	// YAML resolves, does not silently become an unusable working directory.
	for i := range c.Git.Repositories {
		repo := &c.Git.Repositories[i]
		if repo.Path == "" {
			return fmt.Errorf("git.repositories[%d]: path is required", i)
		}
		expanded, err := ExpandPath(repo.Path)
		if err != nil {
			return fmt.Errorf("git.repositories[%d]: %w", i, err)
		}
		repo.Path = expanded
	}

	if len(c.Plugins.Enabled) > 0 {
		if c.Plugins.Directory == "" {
			return errors.New("plugins.directory is required when plugins.enabled is set")
		}
		expanded, err := ExpandPath(c.Plugins.Directory)
		if err != nil {
			return fmt.Errorf("plugins.directory: %w", err)
		}
		c.Plugins.Directory = expanded

		// Fail closed: an enabled plugin without a recorded digest would be
		// loaded as unverified native code.
		for _, name := range c.Plugins.Enabled {
			if _, ok := c.Plugins.Allow[name]; !ok {
				return fmt.Errorf(
					"plugins.enabled lists %q but plugins.allow has no sha256 digest for it", name)
			}
		}
	}

	if c.Kubernetes.ConfigPath != "" {
		expanded, err := ExpandPath(c.Kubernetes.ConfigPath)
		if err != nil {
			return fmt.Errorf("kubernetes.config_path: %w", err)
		}
		c.Kubernetes.ConfigPath = expanded
	}

	for _, interval := range []struct {
		name  string
		value time.Duration
	}{
		{"cloud.refresh", c.Cloud.Refresh},
		{"kubernetes.refresh", c.Kubernetes.Refresh},
		{"cicd.refresh", c.CICD.Refresh},
	} {
		if interval.value != 0 && interval.value < minRefreshInterval {
			return fmt.Errorf("%s must be at least %s, got %s",
				interval.name, minRefreshInterval, interval.value)
		}
	}

	return nil
}

// ExpandPath resolves a leading ~ to the user's home directory and returns an
// absolute path.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding %q: %w", path, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", path, err)
	}
	return abs, nil
}

// StringToCustomDurationHookFunc converts strings to time.Duration, additionally
// accepting the day and week suffixes that time.ParseDuration rejects.
func StringToCustomDurationHookFunc() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data any) (any, error) {
		if from.Kind() != reflect.String || to != reflect.TypeOf(time.Duration(0)) {
			return data, nil
		}

		raw, ok := data.(string)
		if !ok {
			return data, nil
		}
		if raw == "" {
			return time.Duration(0), nil
		}

		if duration, err := time.ParseDuration(raw); err == nil {
			return duration, nil
		}

		for suffix, unit := range map[string]time.Duration{
			"d": 24 * time.Hour,
			"w": 7 * 24 * time.Hour,
		} {
			if !strings.HasSuffix(raw, suffix) {
				continue
			}
			value, err := strconv.Atoi(strings.TrimSuffix(raw, suffix))
			if err != nil {
				return nil, fmt.Errorf("invalid duration %q: %w", raw, err)
			}
			return time.Duration(value) * unit, nil
		}

		return nil, fmt.Errorf("invalid duration %q", raw)
	}
}

// DefaultConfig returns the built-in configuration.
//
// It monitors only the local machine. No endpoint is polled by default, because
// the product is specified to operate locally and send nothing anywhere without
// explicit configuration; the previous defaults polled the author's personal
// domains.
func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			RefreshInterval:  5 * time.Second,
			Theme:            "default",
			HistoryRetention: 7 * 24 * time.Hour,
		},
		Layout: []LayoutTab{
			{Name: "System", Panels: []string{"cpu", "memory", "disk", "processes"}},
			{Name: "HTTP", Panels: []string{"http-endpoints"}},
			{Name: "Git", Panels: []string{"git-status", "git-commits"}},
			{Name: "History", Panels: []string{"history"}},
			{Name: "Notifications", Panels: []string{"notifications"}},
			{Name: "Plugins", Panels: []string{"plugins"}},
		},
		Endpoints: nil,
		Git: GitConfig{
			Repositories: []GitRepoConfig{
				{Path: ".", Remote: "origin", Branch: ""},
			},
		},
		Plugins: PluginsConfig{
			Allow:    map[string]string{},
			Settings: map[string]map[string]any{},
		},
	}
}

// SaveConfig writes the configuration to filePath.
func SaveConfig(config *Config, filePath string) error {
	if config == nil {
		return errors.New("config: nothing to save")
	}
	if filePath == "" {
		return errors.New("config: file path is required")
	}

	// filepath.Dir handles every shape of path. The previous implementation
	// sliced off a fixed "/config.yaml" suffix, which panicked on any path
	// shorter than that suffix and produced a wrong directory otherwise.
	if dir := filepath.Dir(filePath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating config directory %s: %w", dir, err)
		}
	}

	v := viper.New()
	v.SetConfigFile(filePath)

	if err := v.MergeConfigMap(config.toMap()); err != nil {
		return fmt.Errorf("building config map: %w", err)
	}

	if err := v.WriteConfig(); err != nil {
		return fmt.Errorf("writing config file %s: %w", filePath, err)
	}

	return nil
}

// toMap renders the configuration using the same keys LoadConfig reads.
//
// The mapping is written out explicitly rather than handing viper the structs:
// viper serialises Go field names, so a saved file used keys like
// "refreshinterval" that LoadConfig then rejected, and no saved configuration
// could be read back. Durations are written as strings so the file stays legible.
func (c *Config) toMap() map[string]any {
	layout := make([]map[string]any, 0, len(c.Layout))
	for _, tab := range c.Layout {
		entry := map[string]any{"name": tab.Name}
		if len(tab.Panels) > 0 {
			entry["panels"] = tab.Panels
		}
		if len(tab.Rows) > 0 {
			rows := make([]map[string]any, 0, len(tab.Rows))
			for _, row := range tab.Rows {
				rows = append(rows, map[string]any{"size": row.Size, "panels": row.Panels})
			}
			entry["rows"] = rows
		}
		layout = append(layout, entry)
	}

	endpoints := make([]map[string]any, 0, len(c.Endpoints))
	for _, endpoint := range c.Endpoints {
		entry := map[string]any{
			"name":   endpoint.Name,
			"url":    endpoint.URL,
			"method": endpoint.Method,
		}
		if len(endpoint.Headers) > 0 {
			entry["headers"] = endpoint.Headers
		}
		if endpoint.ExpectedStatus != 0 {
			entry["expected_status"] = endpoint.ExpectedStatus
		}
		if endpoint.Timeout > 0 {
			entry["timeout"] = endpoint.Timeout.String()
		}
		if endpoint.Interval > 0 {
			entry["interval"] = endpoint.Interval.String()
		}
		endpoints = append(endpoints, entry)
	}

	repositories := make([]map[string]any, 0, len(c.Git.Repositories))
	for _, repo := range c.Git.Repositories {
		repositories = append(repositories, map[string]any{
			"path":   repo.Path,
			"remote": repo.Remote,
			"branch": repo.Branch,
		})
	}

	out := map[string]any{
		"general": map[string]any{
			"refresh":           c.General.RefreshInterval.String(),
			"theme":             c.General.Theme,
			"history_retention": c.General.HistoryRetention.String(),
		},
		"layout": layout,
		"git":    map[string]any{"repositories": repositories},
	}

	if len(endpoints) > 0 {
		out["endpoints"] = endpoints
	}

	plugins := map[string]any{}
	if c.Plugins.Directory != "" {
		plugins["directory"] = c.Plugins.Directory
	}
	if len(c.Plugins.Enabled) > 0 {
		plugins["enabled"] = c.Plugins.Enabled
	}
	if len(c.Plugins.Allow) > 0 {
		plugins["allow"] = c.Plugins.Allow
	}
	if len(c.Plugins.Settings) > 0 {
		plugins["settings"] = c.Plugins.Settings
	}
	if len(plugins) > 0 {
		out["plugins"] = plugins
	}

	if len(c.Cloud.Enabled) > 0 {
		cloud := map[string]any{"enabled": c.Cloud.Enabled}
		if c.Cloud.Refresh > 0 {
			cloud["refresh"] = c.Cloud.Refresh.String()
		}
		aws := map[string]any{}
		if c.Cloud.AWS.Region != "" {
			aws["region"] = c.Cloud.AWS.Region
		}
		if c.Cloud.AWS.Profile != "" {
			aws["profile"] = c.Cloud.AWS.Profile
		}
		if len(c.Cloud.AWS.AdditionalRegions) > 0 {
			aws["additional_regions"] = c.Cloud.AWS.AdditionalRegions
		}
		if len(c.Cloud.AWS.Resources) > 0 {
			aws["resources"] = c.Cloud.AWS.Resources
		}
		if len(aws) > 0 {
			cloud["aws"] = aws
		}
		out["cloud"] = cloud
	}

	if c.Kubernetes.Enabled {
		kubernetes := map[string]any{"enabled": true}
		if c.Kubernetes.Refresh > 0 {
			kubernetes["refresh"] = c.Kubernetes.Refresh.String()
		}
		if c.Kubernetes.ConfigPath != "" {
			kubernetes["config_path"] = c.Kubernetes.ConfigPath
		}
		if c.Kubernetes.Context != "" {
			kubernetes["context"] = c.Kubernetes.Context
		}
		if len(c.Kubernetes.Namespaces) > 0 {
			kubernetes["namespaces"] = c.Kubernetes.Namespaces
		}
		if len(c.Kubernetes.Resources) > 0 {
			kubernetes["resources"] = c.Kubernetes.Resources
		}
		out["kubernetes"] = kubernetes
	}

	if len(c.CICD.Enabled) > 0 {
		cicd := map[string]any{"enabled": c.CICD.Enabled}
		if c.CICD.Refresh > 0 {
			cicd["refresh"] = c.CICD.Refresh.String()
		}
		github := map[string]any{}
		if c.CICD.GitHub.Owner != "" {
			github["owner"] = c.CICD.GitHub.Owner
		}
		if len(c.CICD.GitHub.Repositories) > 0 {
			github["repositories"] = c.CICD.GitHub.Repositories
		}
		if len(c.CICD.GitHub.Workflows) > 0 {
			github["workflows"] = c.CICD.GitHub.Workflows
		}
		if len(github) > 0 {
			cicd["github"] = github
		}
		out["cicd"] = cicd
	}

	return out
}

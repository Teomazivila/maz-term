package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeConfig writes body to a temporary config file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := DefaultConfig()
	require.NoError(t, cfg.normalise())

	assert.Equal(t, 5*time.Second, cfg.General.RefreshInterval)
	assert.Equal(t, 7*24*time.Hour, cfg.General.HistoryRetention)
	assert.NotEmpty(t, cfg.Layout)
}

// TestDefaultConfigContactsNothing pins the local-only default. The previous
// defaults polled two of the author's personal domains on every run.
func TestDefaultConfigContactsNothing(t *testing.T) {
	cfg := DefaultConfig()

	assert.Empty(t, cfg.Endpoints, "no endpoint may be polled unless configured")
	assert.Empty(t, cfg.Cloud.Enabled, "no cloud provider may be contacted by default")
	assert.False(t, cfg.Kubernetes.Enabled, "no cluster may be contacted by default")
	assert.Empty(t, cfg.CICD.Enabled, "no CI provider may be contacted by default")
}

func TestLoadConfigMissingFileUsesDefaults(t *testing.T) {
	// An empty path with no config.yaml anywhere on the search path.
	dir := t.TempDir()
	t.Chdir(dir)

	cfg, err := LoadConfig("")
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, cfg.General.RefreshInterval)
}

func TestLoadConfigRejectsMalformedYAML(t *testing.T) {
	path := writeConfig(t, "general:\n  refresh: [unclosed\n")

	_, err := LoadConfig(path)
	require.Error(t, err, "a malformed file must not be silently replaced by defaults")
}

// TestLoadConfigRejectsUnknownKeys pins the guard against silent
// misconfiguration from a typo.
func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, "general:\n  refresh: 5s\n  reffresh: 10s\n")

	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reffresh")
}

// TestLoadConfigRejectsSecretsInFile pins H-017: credentials must never be
// readable from a file that is committed to a repository.
func TestLoadConfigRejectsSecretsInFile(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"aws access key", "cloud:\n  aws:\n    access_key_id: AKIAEXAMPLE\n"},
		{"aws secret key", "cloud:\n  aws:\n    secret_access_key: hunter2\n"},
		{"github token", "cicd:\n  github:\n    token: ghp_example\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, tt.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must not appear in a configuration file")
		})
	}
}

// TestLoadConfigEnvironmentOverride pins H-016. Values reachable only through
// AutomaticEnv were absent from viper's AllSettings, which the decoder consumes,
// so environment configuration silently had no effect.
func TestLoadConfigEnvironmentOverride(t *testing.T) {
	path := writeConfig(t, "general:\n  refresh: 5s\n  theme: default\n")

	t.Setenv("MAZTERM_GENERAL_REFRESH", "30s")
	t.Setenv("MAZTERM_GENERAL_THEME", "solarized")

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, 30*time.Second, cfg.General.RefreshInterval, "env must override the file")
	assert.Equal(t, "solarized", cfg.General.Theme)
}

func TestLoadConfigParsesDocumentedLayout(t *testing.T) {
	path := writeConfig(t, `
general:
  refresh: 5s
layout:
  - name: "System Overview"
    rows:
      - size: 1
        panels: ["cpu", "memory"]
      - size: 2
        panels: ["processes"]
`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Layout, 1)

	// The nested rows form is what the README documents; it previously decoded
	// into a tab with no panels at all.
	assert.Equal(t, []string{"cpu", "memory", "processes"}, cfg.Layout[0].AllPanels())
}

func TestLayoutTabAllPanelsCombinesBothForms(t *testing.T) {
	tab := LayoutTab{
		Panels: []string{"a"},
		Rows:   []LayoutRow{{Panels: []string{"b", "c"}}},
	}
	assert.Equal(t, []string{"a", "b", "c"}, tab.AllPanels())
	assert.Empty(t, LayoutTab{}.AllPanels())
}

func TestCustomDurationSuffixes(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "5s", want: 5 * time.Second},
		{in: "10m", want: 10 * time.Minute},
		{in: "2h", want: 2 * time.Hour},
		{in: "7d", want: 7 * 24 * time.Hour},
		{in: "2w", want: 14 * 24 * time.Hour},
		{in: "", want: 0},
		{in: "banana", wantErr: true},
		{in: "5x", wantErr: true},
		{in: "xd", wantErr: true},
	}

	for _, tt := range tests {
		t.Run("retention="+tt.in, func(t *testing.T) {
			path := writeConfig(t, "general:\n  refresh: 5s\n  history_retention: \""+tt.in+"\"\n")
			cfg, err := LoadConfig(path)

			// A zero retention parses successfully but fails validation, so both
			// an unparseable value and an empty one are errors here.
			if tt.wantErr || tt.want == 0 {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.General.HistoryRetention)
		})
	}
}

func TestValidationRejectsBadIntervals(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"zero refresh", "general:\n  refresh: 0s\n"},
		{"sub-second refresh", "general:\n  refresh: 100ms\n"},
		{"zero retention", "general:\n  refresh: 5s\n  history_retention: 0s\n"},
		{"retention below refresh", "general:\n  refresh: 10m\n  history_retention: 1m\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, tt.body))
			require.Error(t, err)
		})
	}
}

func TestValidationOfEndpoints(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing url",
			body:    "endpoints:\n  - name: api\n",
			wantErr: "url is required",
		},
		{
			name:    "non http scheme",
			body:    "endpoints:\n  - name: api\n    url: file:///etc/passwd\n",
			wantErr: "scheme must be http or https",
		},
		{
			name:    "no host",
			body:    "endpoints:\n  - name: api\n    url: https://\n",
			wantErr: "no host",
		},
		{
			name:    "duplicate names",
			body:    "endpoints:\n  - name: api\n    url: https://a.example\n  - name: api\n    url: https://b.example\n",
			wantErr: "duplicate name",
		},
		{
			name:    "sub-second interval",
			body:    "endpoints:\n  - name: api\n    url: https://a.example\n    interval: 10ms\n",
			wantErr: "interval must be at least",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, tt.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestEndpointDefaultsAreFilledIn(t *testing.T) {
	path := writeConfig(t, "endpoints:\n  - url: https://api.example/health\n")

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Endpoints, 1)

	assert.Equal(t, "api.example", cfg.Endpoints[0].Name, "name defaults to the host")
	assert.Equal(t, "GET", cfg.Endpoints[0].Method)
}

// TestGitRepositoryPathIsExpanded pins H-015: neither Go nor YAML expands a
// leading ~, so the documented example path could never work.
func TestGitRepositoryPathIsExpanded(t *testing.T) {
	path := writeConfig(t, "git:\n  repositories:\n    - path: \"~/projects/example\"\n")

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Git.Repositories, 1)

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	got := cfg.Git.Repositories[0].Path
	assert.Equal(t, filepath.Join(home, "projects", "example"), got)
	assert.NotContains(t, got, "~")
	assert.True(t, filepath.IsAbs(got))
}

// TestPluginsRequireAnAllowlistedDigest pins the fail-closed behaviour: an
// enabled plugin without a recorded digest would be loaded as unverified native
// code.
func TestPluginsRequireAnAllowlistedDigest(t *testing.T) {
	path := writeConfig(t, `
plugins:
  directory: "./plugins"
  enabled: ["sample"]
`)

	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugins.allow")
}

func TestPluginsWithDigestAreAccepted(t *testing.T) {
	path := writeConfig(t, `
plugins:
  directory: "./plugins"
  enabled: ["sample"]
  allow:
    sample: "sha256:0000000000000000000000000000000000000000000000000000000000000000"
  settings:
    sample:
      interval: "30s"
`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, []string{"sample"}, cfg.Plugins.Enabled)
	assert.Contains(t, cfg.Plugins.Allow, "sample")
	assert.Equal(t, "30s", cfg.Plugins.Settings["sample"]["interval"])
	assert.True(t, filepath.IsAbs(cfg.Plugins.Directory))
}

func TestPluginsDirectoryRequiredWhenEnabled(t *testing.T) {
	path := writeConfig(t, "plugins:\n  enabled: [\"sample\"]\n")

	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugins.directory")
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	got, err := ExpandPath("~")
	require.NoError(t, err)
	assert.Equal(t, home, got)

	got, err = ExpandPath("~/x/y")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "x", "y"), got)

	got, err = ExpandPath("")
	require.NoError(t, err)
	assert.Empty(t, got)

	// A tilde that is part of a name is not a home reference.
	got, err = ExpandPath("~notauser/x")
	require.NoError(t, err)
	assert.NotEqual(t, filepath.Join(home, "notauser", "x"), got)
}

// TestSaveConfigShortPath pins H-019: the previous implementation sliced a fixed
// "/config.yaml" suffix off the path and panicked on anything shorter.
func TestSaveConfigShortPath(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"c.yaml", "cfg.yaml", "config.yaml", "a.yml"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)

			assert.NotPanics(t, func() {
				require.NoError(t, SaveConfig(DefaultConfig(), path))
			})
			assert.FileExists(t, path)
		})
	}
}

func TestSaveConfigCreatesNestedDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "config.yaml")

	require.NoError(t, SaveConfig(DefaultConfig(), path))
	assert.FileExists(t, path)
}

func TestSaveConfigRejectsBadInput(t *testing.T) {
	require.Error(t, SaveConfig(nil, "x.yaml"))
	require.Error(t, SaveConfig(DefaultConfig(), ""))
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	original := DefaultConfig()
	original.General.Theme = "custom"
	original.General.RefreshInterval = 15 * time.Second
	require.NoError(t, SaveConfig(original, path))

	reloaded, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "custom", reloaded.General.Theme)
	assert.Equal(t, 15*time.Second, reloaded.General.RefreshInterval)
}

func TestGitHubTokenComesFromEnvironment(t *testing.T) {
	t.Setenv("MAZTERM_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "fallback")
	assert.Equal(t, "fallback", GitHubToken())

	t.Setenv("MAZTERM_GITHUB_TOKEN", "preferred")
	assert.Equal(t, "preferred", GitHubToken())
}

func TestUserDirIsCreated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir, err := UserDir()
	require.NoError(t, err)
	assert.DirExists(t, dir)
	assert.Contains(t, dir, filepath.Join(".config", "maz-term"))
}

// TestLayoutFromFileReplacesDefaults pins the decoder's ZeroFields behaviour:
// without it, a layout in the file is merged into the built-in one and the tab
// ends up with both sets of panels.
func TestLayoutFromFileReplacesDefaults(t *testing.T) {
	path := writeConfig(t, `
layout:
  - name: "Only"
    panels: ["cpu"]
`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	require.Len(t, cfg.Layout, 1, "the file's layout must replace the defaults")
	assert.Equal(t, "Only", cfg.Layout[0].Name)
	assert.Equal(t, []string{"cpu"}, cfg.Layout[0].AllPanels())
}

// TestEndpointsFromFileReplaceDefaults covers the same merge hazard for lists
// that a user is likely to override.
func TestEndpointsFromFileReplaceDefaults(t *testing.T) {
	path := writeConfig(t, "endpoints:\n  - url: https://one.example\n")

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	require.Len(t, cfg.Endpoints, 1)
	assert.Equal(t, "https://one.example", cfg.Endpoints[0].URL)
}

// TestEndpointFullyPopulatedFromFile pins every documented endpoint key.
//
// models.EndpointConfig originally carried only json tags. mapstructure matches
// field names case-insensitively but does not convert snake_case, so
// expected_status silently never reached ExpectedStatus, and once unknown keys
// became an error the documented example stopped loading at all.
func TestEndpointFullyPopulatedFromFile(t *testing.T) {
	path := writeConfig(t, `
endpoints:
  - name: "API gateway"
    url: "https://api.example.com/health"
    method: POST
    expected_status: 204
    timeout: 5s
    interval: 30s
    headers:
      X-Probe: "maz-term"
`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Endpoints, 1)

	endpoint := cfg.Endpoints[0]
	assert.Equal(t, "API gateway", endpoint.Name)
	assert.Equal(t, "https://api.example.com/health", endpoint.URL)
	assert.Equal(t, "POST", endpoint.Method)
	assert.Equal(t, 204, endpoint.ExpectedStatus, "expected_status must reach the field")
	assert.Equal(t, 5*time.Second, endpoint.Timeout)
	assert.Equal(t, 30*time.Second, endpoint.Interval)
	// Viper lowercases map keys while preserving values. That is harmless for
	// HTTP, whose header names are case-insensitive and which Go canonicalises
	// when the request is built, but it is asserted here so the behaviour is
	// documented rather than discovered.
	assert.Equal(t, "maz-term", endpoint.Headers["x-probe"])
	assert.NotContains(t, endpoint.Headers, "X-Probe")
}

// TestShippedExampleConfigLoads guards against the documented example drifting
// out of step with the schema, which is how the expected_status defect hid: the
// line was commented out in the example, so nothing exercised it.
func TestShippedExampleConfigLoads(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("example config not present")
	}

	cfg, err := LoadConfig(path)
	require.NoError(t, err, "config.example.yaml must load with the current schema")
	assert.NotNil(t, cfg)
}

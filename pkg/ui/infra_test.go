package ui

import (
	"strings"
	"testing"

	"github.com/Teomazivila/maz-term/pkg/config"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// providerConfig enables the three infrastructure providers.
func providerConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Cloud.Enabled = []string{"aws"}
	cfg.Cloud.AWS.Region = "eu-west-1"
	cfg.Kubernetes.Enabled = true
	cfg.CICD.Enabled = []string{"github"}
	cfg.CICD.GitHub.Owner = "acme"
	cfg.CICD.GitHub.Repositories = []string{"widget"}
	return cfg
}

// TestProviderTabsAbsentWhenNotConfigured pins the honest default: an
// unconfigured provider has no tab rather than an empty one implying a connection.
func TestProviderTabsAbsentWhenNotConfigured(t *testing.T) {
	app := NewApp(config.DefaultConfig())

	for _, name := range []string{"Cloud", "Kubernetes", "CI/CD"} {
		assert.Nil(t, app.getTabByName(name), "%s must not exist until configured", name)
	}
}

func TestProviderTabsPresentWhenConfigured(t *testing.T) {
	app := NewApp(providerConfig())

	for _, name := range []string{"Cloud", "Kubernetes", "CI/CD"} {
		assert.NotNil(t, app.getTabByName(name), "%s must exist once configured", name)
	}
}

func TestProviderEnablementParsing(t *testing.T) {
	cfg := config.DefaultConfig()
	assert.False(t, awsEnabled(cfg))
	assert.False(t, githubEnabled(cfg))

	cfg.Cloud.Enabled = []string{" AWS "}
	cfg.CICD.Enabled = []string{"GitHub"}
	assert.True(t, awsEnabled(cfg), "matching is case- and space-insensitive")
	assert.True(t, githubEnabled(cfg))

	cfg.Cloud.Enabled = []string{"gcp"}
	assert.False(t, awsEnabled(cfg), "an unsupported provider does not enable AWS")
}

// TestProviderTabsWithoutCollectorSaySo checks the tab explains how to configure
// the provider instead of showing a connected or empty-but-healthy state.
func TestProviderTabsWithoutCollectorSaySo(t *testing.T) {
	app := NewApp(providerConfig())
	app.TermWidth, app.TermHeight = 140, 44
	app.StatusBar = widgets.NewParagraph()
	app.TabBar = widgets.NewTabPane(app.getTabNames()...)

	// The collectors are nil because Run was never called.
	require.Nil(t, app.CloudCollector)
	require.Nil(t, app.KubernetesCollector)
	require.Nil(t, app.CICDCollector)

	app.updateData()

	tests := []struct {
		tab      string
		wantText []string
	}{
		{tab: "Cloud", wantText: []string{"not configured", "cloud:", "credential chain"}},
		{tab: "Kubernetes", wantText: []string{"not configured", "kubernetes:", "kubeconfig"}},
		{tab: "CI/CD", wantText: []string{"not configured", "cicd:", "GITHUB_TOKEN"}},
	}

	for _, tt := range tests {
		t.Run(tt.tab, func(t *testing.T) {
			tab := app.getTabByName(tt.tab)
			require.NotNil(t, tab)
			require.NotEmpty(t, tab.Panels)

			text := tab.Panels[0].Text
			for _, want := range tt.wantText {
				assert.Contains(t, text, want)
			}

			// Nothing may claim a working connection.
			assert.NotContains(t, text, "Connected")
			assert.NotContains(t, strings.ToLower(text), "monitoring cloud resources")
		})
	}
}

// TestProviderTabsRenderWithoutData exercises the layout for every provider tab.
func TestProviderTabsRenderWithoutData(t *testing.T) {
	app := NewApp(providerConfig())
	app.TermWidth, app.TermHeight = 140, 44
	app.StatusBar = widgets.NewParagraph()
	app.TabBar = widgets.NewTabPane(app.getTabNames()...)
	app.updateData()

	for i, tab := range app.Tabs {
		t.Run(tab.Name, func(t *testing.T) {
			app.ActiveTabIndex = i
			app.updateData()

			frame := app.buildFrame()
			assert.NotEmpty(t, frame, "%s produced nothing to draw", tab.Name)

			contentBottom := app.TermHeight - statusBarHeight - tabBarHeight
			for _, drawable := range frame {
				if drawable == ui.Drawable(app.StatusBar) || drawable == ui.Drawable(app.TabBar) {
					continue
				}
				rect := drawable.GetRect()
				assert.GreaterOrEqual(t, rect.Dy(), minWidgetExtent)
				assert.LessOrEqual(t, rect.Max.Y, contentBottom)
			}
		})
	}
}

func TestProviderTabsToleratePartialWidgets(t *testing.T) {
	app := newTestApp(t)

	for _, name := range []string{"Cloud", "Kubernetes", "CI/CD"} {
		for count := 0; count <= 5; count++ {
			tab := &Tab{Name: name}
			for range count {
				tab.Widgets = append(tab.Widgets, widgets.NewParagraph())
			}

			assert.NotPanics(t, func() {
				app.layoutTab(tab, app.contentRect())
			}, "%s with %d widgets panicked", name, count)
		}
	}
}

func TestEmptyLabelDistinguishesUnavailableFromEmpty(t *testing.T) {
	assert.Equal(t, "no instances", emptyLabel(nil, "no instances"))
	assert.Equal(t, "unavailable", emptyLabel([]string{"AccessDenied"}, "no instances"),
		"a failure must not read as an empty inventory")
}

func TestLimitStringsNotesOmissions(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, limitStrings([]string{"a", "b"}, 5))

	got := limitStrings([]string{"a", "b", "c", "d"}, 2)
	require.Len(t, got, 3)
	assert.Equal(t, "... and 2 more", got[2], "truncation must be visible, not silent")
}

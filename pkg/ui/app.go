// Package ui renders the maz-term dashboard.
package ui

import (
	"fmt"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/Teomazivila/maz-term/pkg/plugins"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// Storage is the persistence surface the dashboard needs. It combines the write
// side used by collectors with the read side used by the History and
// Notifications tabs, so a single object satisfies both.
type Storage interface {
	collector.StorageProvider

	// System metrics history
	GetCPUUsageHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetMemoryUsageHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetDiskUsageHistory(mountPoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)

	// HTTP metrics history
	GetHTTPResponseTimeHistory(endpoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetHTTPAvailabilityHistory(endpoint string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetAllEndpoints() ([]string, error)

	// Git metrics history
	GetCommitCountHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetModifiedFilesHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetPendingCommitsHistory(repoName string, period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetAllGitRepositories() ([]string, error)

	// Infrastructure summary history
	GetCloudInstanceCountHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetCloudCPUHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetKubernetesPodCountHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetKubernetesNodeReadyHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)
	GetCICDSuccessRateHistory(period time.Duration, points int) ([]models.TimeSeriesPoint, error)

	// Event annotations
	GetEventAnnotations(period time.Duration) ([]models.EventAnnotation, error)
	AddEventAnnotation(event models.EventAnnotation) error
	DeleteEventAnnotation(id string) error

	// Notifications
	AddNotification(notification models.Notification) error
	GetNotifications(count int, includeRead bool) ([]models.Notification, error)
	GetFilteredNotifications(count int, includeRead bool, sources, severities []string) ([]models.Notification, error)
	GetNotificationSources() ([]string, error)
	GetNotificationSeverities() ([]string, error)
	MarkAsRead(id string) error
	DismissNotification(id string) error
	ClearAllNotifications() error
	GetUnreadNotificationCount() (int, error)

	// Export
	ExportAll(outputDir string, period time.Duration) ([]string, error)
}

// StorageInterface is an alias retained for compatibility with existing call
// sites.
//
// Deprecated: use Storage.
type StorageInterface = Storage

// historyRange is one selectable window on the History tab.
type historyRange struct {
	Label string
	Value time.Duration
}

// historyRanges is the single source of truth for the History tab's selectable
// windows. The on-screen legend and the keyboard handler both derive from it, so
// they cannot disagree.
var historyRanges = []historyRange{
	{"1h", time.Hour},
	{"6h", 6 * time.Hour},
	{"12h", 12 * time.Hour},
	{"24h", 24 * time.Hour},
	{"3d", 72 * time.Hour},
	{"7d", 168 * time.Hour},
	{"30d", 720 * time.Hour},
}

// defaultHistoryRangeIndex selects the 24h window.
const defaultHistoryRangeIndex = 3

// annotationDraft holds in-progress input for the annotation form.
//
// This state lives on App rather than inside the key handler. Holding it in a
// function-local struct meant it was re-zeroed on every keystroke, so the title
// could never grow past one character and the form could never be submitted.
type annotationDraft struct {
	Title       string
	Description string
	Field       int // 0 = title, 1 = description
}

// filterDraft holds in-progress notification filter selections. It lives on App
// for the same reason as annotationDraft.
type filterDraft struct {
	Section    int // 0 = sources, 1 = severities
	Index      int
	Sources    map[string]bool
	Severities map[string]bool
}

// selected returns the chosen keys of m in stable order relative to order.
func selected(m map[string]bool, order []string) []string {
	var out []string
	for _, key := range order {
		if m[key] {
			out = append(out, key)
		}
	}
	return out
}

// App represents the main terminal UI application.
type App struct {
	Config *config.Config

	// Layout
	ActiveTabIndex int
	Tabs           []*Tab
	StatusBar      *widgets.Paragraph
	TabBar         *widgets.TabPane
	HelpPanel      *widgets.Paragraph
	AnnotationForm *widgets.Paragraph
	TermWidth      int
	TermHeight     int
	ShowHelp       bool

	// Collectors. The infrastructure ones are nil unless their configuration
	// block enables them, and the corresponding tab is only created when they
	// exist, so an unconfigured provider is absent rather than shown empty.
	SystemCollector     *collector.SystemMetricsCollector
	HTTPCollector       *collector.HTTPHealthChecker
	GitCollector        *collector.GitStatusCollector
	CloudCollector      *collector.AWSCollector
	KubernetesCollector *collector.KubernetesCollector
	CICDCollector       *collector.GitHubActionsCollector

	// Persistence
	Storage         Storage
	storageProvider collector.StorageProvider

	// History tab
	HistoryRange    time.Duration
	HistoryRangeIdx int
	ShowAnnotations bool
	Annotations     []models.EventAnnotation

	// Zoom
	ZoomMode         bool
	ZoomStartPercent float64
	ZoomEndPercent   float64
	ZoomStartTime    time.Time
	ZoomEndTime      time.Time

	// Notifications
	Notifications          []models.Notification
	SelectedNotification   int
	NotificationDetailMode bool
	NotificationFilterMode bool
	NotificationSources    []string
	NotificationSeverities []string
	activeSources          []string
	activeSeverities       []string

	// In-progress form input
	addingAnnotation bool
	annotation       annotationDraft
	filter           filterDraft

	// Plugins
	PluginManager  *plugins.PluginManager
	PluginMetrics  []models.Metric
	SelectedPlugin int

	// Status
	statusMessage string
	quit          bool
}

// Tab represents one dashboard tab and the widgets it owns.
type Tab struct {
	Name       string
	Widgets    []ui.Drawable
	Panels     []*widgets.Paragraph
	Gauges     []*widgets.Gauge
	Tables     []*widgets.Table
	Sparklines []*widgets.SparklineGroup
	BarCharts  []*widgets.BarChart
	Plots      []*widgets.Plot
	Lists      []*widgets.List
	HasUnread  bool
}

// SetStorage sets the storage used for history and notification reads.
func (a *App) SetStorage(store Storage) {
	a.Storage = store
}

// SetStorageProvider records the store used to persist collected samples and
// applies it to any collector that already exists.
//
// The provider is retained so that collectors created later, inside Run, also
// receive it. Previously this was called before the collectors existed, every
// nil check failed, and nothing was ever persisted.
func (a *App) SetStorageProvider(provider collector.StorageProvider) {
	a.storageProvider = provider
	a.applyStorageProvider()
}

// applyStorageProvider pushes the recorded provider into every live collector.
func (a *App) applyStorageProvider() {
	if a.storageProvider == nil {
		return
	}
	if a.SystemCollector != nil {
		a.SystemCollector.SetStorageProvider(a.storageProvider)
	}
	if a.HTTPCollector != nil {
		a.HTTPCollector.SetStorageProvider(a.storageProvider)
	}
	if a.GitCollector != nil {
		a.GitCollector.SetStorageProvider(a.storageProvider)
	}
	if a.CloudCollector != nil {
		a.CloudCollector.SetStorageProvider(a.storageProvider)
	}
	if a.KubernetesCollector != nil {
		a.KubernetesCollector.SetStorageProvider(a.storageProvider)
	}
	if a.CICDCollector != nil {
		a.CICDCollector.SetStorageProvider(a.storageProvider)
	}
}

// setStatus records the message shown in the status bar.
func (a *App) setStatus(format string, args ...any) {
	if len(args) == 0 {
		a.statusMessage = format
		return
	}
	a.statusMessage = fmt.Sprintf(format, args...)
}

// activeTab returns the currently selected tab, or nil when there are none.
func (a *App) activeTab() *Tab {
	if a.ActiveTabIndex < 0 || a.ActiveTabIndex >= len(a.Tabs) {
		return nil
	}
	return a.Tabs[a.ActiveTabIndex]
}

// activeTabName returns the current tab's name, or "" when there are no tabs.
func (a *App) activeTabName() string {
	if tab := a.activeTab(); tab != nil {
		return tab.Name
	}
	return ""
}

// getTabByName returns the tab with the given name, or nil.
func (a *App) getTabByName(name string) *Tab {
	for _, tab := range a.Tabs {
		if tab.Name == name {
			return tab
		}
	}
	return nil
}

// getTabNames returns the display names of all tabs, including unread badges.
func (a *App) getTabNames() []string {
	names := make([]string, len(a.Tabs))
	for i, tab := range a.Tabs {
		names[i] = tab.Name
		if tab.HasUnread {
			names[i] += " *"
		}
	}
	return names
}

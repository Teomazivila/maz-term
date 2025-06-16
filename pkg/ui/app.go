package ui

import (
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/Teomazivila/maz-term/pkg/plugins"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

// StorageInterface defines the interface for data storage operations
type StorageInterface interface {
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

	// Event annotations
	GetEventAnnotations(period time.Duration) ([]models.EventAnnotation, error)
	AddEventAnnotation(event models.EventAnnotation) error
	DeleteEventAnnotation(id string) error

	// Notifications
	AddNotification(notification models.Notification) error
	GetNotifications(count int, includeRead bool) ([]models.Notification, error)
	GetFilteredNotifications(count int, includeRead bool, sources []string, severities []string) ([]models.Notification, error)
	MarkAsRead(id string) error
	DismissNotification(id string) error
	ClearAllNotifications() error
	GetUnreadNotificationCount() (int, error)
}

// App represents the main terminal UI application
type App struct {
	Config                 *config.Config
	ActiveTabIndex         int
	Tabs                   []*Tab
	StatusBar              *widgets.Paragraph
	Grid                   *ui.Grid
	TabBar                 *widgets.TabPane
	Running                bool
	SystemCollector        *collector.SystemMetricsCollector
	HTTPCollector          *collector.HTTPHealthChecker
	GitCollector           *collector.GitStatusCollector
	CloudCollector         interface{ GetLatestMetrics() interface{} }
	KubernetesCollector    interface{ GetLatestMetrics() interface{} }
	CICDCollector          interface{ GetLatestMetrics() interface{} }
	TermWidth              int
	TermHeight             int
	ShowHelp               bool
	HelpPanel              *widgets.Paragraph
	Storage                StorageInterface
	ExportInProgress       bool
	HistoryRange           time.Duration            // Selected time range for history tab
	HistoryRangeIdx        int                      // Index of currently selected range option
	ShowAnnotations        bool                     // Whether to show event annotations
	Annotations            []models.EventAnnotation // Cached event annotations
	AddingAnnotation       bool                     // Whether we're currently adding a new annotation
	AnnotationForm         *widgets.Paragraph       // Form for adding new annotations
	ZoomMode               bool                     // Whether we're in zoom mode
	ZoomActiveChart        int                      // Index of chart being zoomed
	ZoomStartPercent       float64                  // Start position of zoom region (percentage)
	ZoomEndPercent         float64                  // End position of zoom region (percentage)
	ZoomStartTime          time.Time                // Start time for zoomed view
	ZoomEndTime            time.Time                // End time for zoomed view
	ComparisonMode         bool                     // Whether comparison mode is active
	PrimaryMetric          string                   // Primary metric being compared
	ComparisonMetrics      []string                 // List of metrics being compared
	Notifications          []models.Notification    // Cached notifications
	SelectedNotification   int                      // Index of the selected notification
	NotificationDetailMode bool                     // Whether notification detail mode is active
	NotificationSources    []string                 // List of sources to filter notifications by
	NotificationSeverities []string                 // List of severities to filter notifications by
	NotificationFilterMode bool                     // Whether filter mode is active
	PluginManager          *plugins.PluginManager   // Plugin manager
	PluginMetrics          []models.Metric          // Metrics from plugins
}

// Tab represents a terminal UI tab
type Tab struct {
	Name       string
	Grid       *ui.Grid
	Widgets    []ui.Drawable
	Panels     []*widgets.Paragraph
	Gauges     []*widgets.Gauge
	Tables     []*widgets.Table
	Sparklines []*widgets.SparklineGroup
	BarCharts  []*widgets.BarChart
	Plots      []*widgets.Plot
	Lists      []*widgets.List
	HasUnread  bool // Whether this tab has unread notifications
}

// createUI creates the terminal UI elements
func (a *App) createUI() {
	// Create status bar
	a.StatusBar = widgets.NewParagraph()
	a.StatusBar.Title = "Status"
	a.StatusBar.Text = "Initializing..."

	// Create tab bar
	a.TabBar = widgets.NewTabPane(a.getTabNames()...)
	a.TabBar.ActiveTabIndex = a.ActiveTabIndex
}

// SetCloudCollector sets the cloud metrics collector
func (a *App) SetCloudCollector(collector interface{ GetLatestMetrics() interface{} }) {
	a.CloudCollector = collector
}

// SetKubernetesCollector sets the Kubernetes metrics collector
func (a *App) SetKubernetesCollector(collector interface{ GetLatestMetrics() interface{} }) {
	a.KubernetesCollector = collector
}

// SetCICDCollector sets the CI/CD metrics collector
func (a *App) SetCICDCollector(collector interface{ GetLatestMetrics() interface{} }) {
	a.CICDCollector = collector
}

// SetStorage sets the storage interface
func (a *App) SetStorage(storage StorageInterface) {
	a.Storage = storage
}

// getTabNames returns the names of all tabs
func (a *App) getTabNames() []string {
	names := make([]string, len(a.Tabs))
	for i, tab := range a.Tabs {
		names[i] = tab.Name
	}
	return names
}

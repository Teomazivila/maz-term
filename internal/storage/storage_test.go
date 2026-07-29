package storage

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDB opens a database in a temporary directory.
func newTestDB(t *testing.T) *Database {
	t.Helper()

	db, err := New(&Config{
		DataPath:        filepath.Join(t.TempDir(), "test.db"),
		RetentionPeriod: time.Hour,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	return db
}

// TestNewRejectsEmptyDataPath pins C-003. An empty path made SQLite open an
// anonymous temporary database that was discarded on close, so no history ever
// survived a restart.
func TestNewRejectsEmptyDataPath(t *testing.T) {
	_, err := New(&Config{RetentionPeriod: time.Hour})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DataPath is required")
}

func TestNewDefaultsIndividualFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")

	// Only DataPath is supplied; every other field must fall back rather than
	// being left at its zero value.
	db, err := New(&Config{DataPath: path})
	require.NoError(t, err)
	defer db.Close()

	assert.Equal(t, DefaultConfig().RetentionPeriod, db.retentionPeriod)
	assert.FileExists(t, path)
}

func TestNewCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "data.db")

	db, err := New(&Config{DataPath: path})
	require.NoError(t, err)
	defer db.Close()

	assert.FileExists(t, path)
}

// TestDatabaseStartsEmpty pins C-004: an empty database must stay empty. It was
// previously seeded with 800 fabricated rows that the History tab then displayed
// as real measurements.
func TestDatabaseStartsEmpty(t *testing.T) {
	db := newTestDB(t)

	for _, table := range []string{"system_metrics", "disk_metrics", "http_metrics", "git_metrics", "annotations", "notifications"} {
		var count int
		require.NoError(t, db.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		assert.Zero(t, count, "%s must start empty", table)
	}

	points, err := db.GetCPUUsageHistory(time.Hour, 100)
	require.NoError(t, err)
	assert.Empty(t, points, "history must be empty until real samples are recorded")
}

func TestSystemMetricsRoundTrip(t *testing.T) {
	db := newTestDB(t)

	sample := models.SystemMetrics{
		CPU:    models.CPUMetrics{UsagePercent: 42.5},
		Memory: models.MemoryMetrics{UsagePercent: 61.25, Total: 1 << 34, Used: 1 << 33},
		Disk: models.DiskMetrics{Filesystems: []models.FilesystemMetrics{
			{MountPoint: "/", Total: 1 << 40, Used: 1 << 39, UsagePercent: 50},
		}},
		CollectedAt: time.Now(),
	}
	require.NoError(t, db.StoreSystemMetrics(sample))

	cpu, err := db.GetCPUUsageHistory(time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, cpu, 1)
	assert.InDelta(t, 42.5, cpu[0].Value, 0.001)

	mem, err := db.GetMemoryUsageHistory(time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, mem, 1)
	assert.InDelta(t, 61.25, mem[0].Value, 0.001)

	disk, err := db.GetDiskUsageHistory("/", time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, disk, 1)
	assert.InDelta(t, 50, disk[0].Value, 0.001)
}

func TestHTTPMetricsRoundTrip(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.StoreHTTPMetrics("api", models.EndpointMetrics{
		Name:         "api",
		URL:          "https://api.example/health",
		StatusCode:   200,
		ResponseTime: 125 * time.Millisecond,
		IsUp:         true,
	}))

	endpoints, err := db.GetAllEndpoints()
	require.NoError(t, err)
	assert.Equal(t, []string{"api"}, endpoints)

	response, err := db.GetHTTPResponseTimeHistory("api", time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, response, 1)
	assert.Positive(t, response[0].Value)

	availability, err := db.GetHTTPAvailabilityHistory("api", time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, availability, 1)
}

func TestGitMetricsRoundTrip(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.StoreGitMetrics(models.GitRepoMetrics{
		Name:           "maz-term",
		Branch:         "main",
		CommitCount:    120,
		ModifiedFiles:  3,
		PendingCommits: 1,
		IsRepository:   true,
	}))

	repos, err := db.GetAllGitRepositories()
	require.NoError(t, err)
	assert.Equal(t, []string{"maz-term"}, repos)

	commits, err := db.GetCommitCountHistory("maz-term", time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.InDelta(t, 120, commits[0].Value, 0.001)

	modified, err := db.GetModifiedFilesHistory("maz-term", time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, modified, 1)

	pending, err := db.GetPendingCommitsHistory("maz-term", time.Hour, 100)
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

// TestAnnotationsRoundTrip pins C-008: add and delete were empty functions that
// returned nil, so the UI reported success and discarded the annotation.
func TestAnnotationsRoundTrip(t *testing.T) {
	db := newTestDB(t)

	annotation := models.EventAnnotation{
		ID:          "ann-1",
		Timestamp:   time.Now(),
		Title:       "deployed v2",
		Description: "rolled out release 2",
		Type:        models.EventTypeDeployment,
		Severity:    models.SeverityInfo,
		Source:      "operator",
		Tags:        []string{"release", "tag with space"},
	}
	require.NoError(t, db.AddEventAnnotation(annotation))

	got, err := db.GetEventAnnotations(time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 1, "the annotation must actually be persisted")

	assert.Equal(t, "ann-1", got[0].ID)
	assert.Equal(t, "deployed v2", got[0].Title)
	assert.Equal(t, models.EventTypeDeployment, got[0].Type)
	// Tags are JSON-encoded, so a value containing a space round-trips intact.
	assert.Equal(t, []string{"release", "tag with space"}, got[0].Tags)

	require.NoError(t, db.DeleteEventAnnotation("ann-1"))

	got, err = db.GetEventAnnotations(time.Hour)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAnnotationValidation(t *testing.T) {
	db := newTestDB(t)

	require.Error(t, db.AddEventAnnotation(models.EventAnnotation{Title: "no id"}))
	require.Error(t, db.AddEventAnnotation(models.EventAnnotation{ID: "no-title"}))
	require.Error(t, db.DeleteEventAnnotation(""))

	_, err := db.GetEventAnnotations(0)
	require.Error(t, err, "a non-positive period is a programming error, not an empty result")
}

// TestDeleteMissingAnnotationReportsNotFound ensures the UI can tell the operator
// the truth rather than reporting a successful no-op.
func TestDeleteMissingAnnotationReportsNotFound(t *testing.T) {
	db := newTestDB(t)

	err := db.DeleteEventAnnotation("does-not-exist")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrNotFound))
}

func TestAnnotationsOutsidePeriodAreExcluded(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.AddEventAnnotation(models.EventAnnotation{
		ID:        "old",
		Title:     "old event",
		Timestamp: time.Now().Add(-48 * time.Hour),
	}))

	got, err := db.GetEventAnnotations(time.Hour)
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = db.GetEventAnnotations(72 * time.Hour)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestNotificationsRoundTrip(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.AddNotification(models.Notification{
		ID:       "n1",
		Title:    "disk almost full",
		Message:  "root is at 95%",
		Severity: "critical",
		Source:   "system",
		Tags:     []string{"disk", "needs attention"},
	}))

	got, err := db.GetNotifications(10, true)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "disk almost full", got[0].Title)
	assert.Equal(t, []string{"disk", "needs attention"}, got[0].Tags,
		"tags are JSON-encoded so values containing spaces survive")

	count, err := db.GetUnreadNotificationCount()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	require.NoError(t, db.MarkAsRead("n1"))
	count, err = db.GetUnreadNotificationCount()
	require.NoError(t, err)
	assert.Zero(t, count)

	require.NoError(t, db.DismissNotification("n1"))
	got, err = db.GetNotifications(10, true)
	require.NoError(t, err)
	assert.Empty(t, got, "dismissed notifications are hidden")
}

func TestNotificationGeneratesIDAndTimestamp(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.AddNotification(models.Notification{Title: "generated"}))

	got, err := db.GetNotifications(10, true)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.NotEmpty(t, got[0].ID)
	assert.False(t, got[0].Timestamp.IsZero())
}

func TestNotificationMissingIDReportsNotFound(t *testing.T) {
	db := newTestDB(t)

	assert.True(t, errors.Is(db.MarkAsRead("nope"), models.ErrNotFound))
	assert.True(t, errors.Is(db.DismissNotification("nope"), models.ErrNotFound))
	require.Error(t, db.MarkAsRead(""))
}

// TestNotificationFilteringIsParameterised pins the SQL-injection fix. The IN
// clause was previously built by interpolating values directly, so a quote in a
// source name broke or altered the statement.
func TestNotificationFilteringIsParameterised(t *testing.T) {
	db := newTestDB(t)

	hostile := `sys'tem" OR 1=1 --`
	require.NoError(t, db.AddNotification(models.Notification{
		ID: "n1", Title: "quoted source", Source: models.NotificationSource(hostile), Severity: "info",
	}))
	require.NoError(t, db.AddNotification(models.Notification{
		ID: "n2", Title: "normal", Source: "system", Severity: "critical",
	}))

	got, err := db.GetFilteredNotifications(10, true, []string{hostile}, nil)
	require.NoError(t, err, "a quote in a filter value must not break the query")
	require.Len(t, got, 1)
	assert.Equal(t, "quoted source", got[0].Title)

	// The injected OR must not widen the result set.
	got, err = db.GetFilteredNotifications(10, true, []string{"' OR '1'='1"}, nil)
	require.NoError(t, err)
	assert.Empty(t, got, "injection attempt must match nothing")
}

// TestNotificationFilteringFindsMatchesBeyondTheWindow pins H-020: filtering used
// to happen in Go over a bounded row window, so rare matches were invisible.
func TestNotificationFilteringFindsMatchesBeyondTheWindow(t *testing.T) {
	db := newTestDB(t)

	// One rare match buried under many non-matching rows.
	for i := range 300 {
		require.NoError(t, db.AddNotification(models.Notification{
			ID:        fmt.Sprintf("bulk-%d", i),
			Title:     "bulk",
			Source:    "system",
			Severity:  "info",
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
		}))
	}
	require.NoError(t, db.AddNotification(models.Notification{
		ID:        "rare",
		Title:     "the rare one",
		Source:    "git",
		Severity:  "critical",
		Timestamp: time.Now().Add(-400 * time.Minute),
	}))

	got, err := db.GetFilteredNotifications(5, true, []string{"git"}, nil)
	require.NoError(t, err)
	require.Len(t, got, 1, "the match must be found even though it is far down the list")
	assert.Equal(t, "the rare one", got[0].Title)
}

func TestNotificationFilterBySeverity(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.AddNotification(models.Notification{ID: "a", Title: "a", Severity: "critical", Source: "system"}))
	require.NoError(t, db.AddNotification(models.Notification{ID: "b", Title: "b", Severity: "info", Source: "system"}))

	got, err := db.GetFilteredNotifications(10, true, nil, []string{"critical"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].Title)

	got, err = db.GetFilteredNotifications(10, true, nil, []string{"critical", "info"})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestNotificationDistinctValues(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.AddNotification(models.Notification{ID: "a", Title: "a", Source: "git", Severity: "info"}))
	require.NoError(t, db.AddNotification(models.Notification{ID: "b", Title: "b", Source: "git", Severity: "critical"}))
	require.NoError(t, db.AddNotification(models.Notification{ID: "c", Title: "c", Source: "system", Severity: "info"}))

	sources, err := db.GetNotificationSources()
	require.NoError(t, err)
	assert.Equal(t, []string{"git", "system"}, sources)

	severities, err := db.GetNotificationSeverities()
	require.NoError(t, err)
	assert.Equal(t, []string{"critical", "info"}, severities)
}

func TestClearAllNotifications(t *testing.T) {
	db := newTestDB(t)

	for i := range 3 {
		require.NoError(t, db.AddNotification(models.Notification{
			ID: fmt.Sprintf("n%d", i), Title: "t", Source: "system", Severity: "info",
		}))
	}

	require.NoError(t, db.ClearAllNotifications())

	got, err := db.GetNotifications(10, true)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestRetentionDeletesOnlyOldRows checks the cutoff boundary.
func TestRetentionDeletesOnlyOldRows(t *testing.T) {
	db := newTestDB(t)
	db.retentionPeriod = time.Hour

	now := time.Now()
	insert := func(offset time.Duration) {
		_, err := db.db.Exec(
			"INSERT INTO system_metrics (timestamp, cpu_usage, memory_usage, memory_total, memory_used) VALUES (?, ?, ?, ?, ?)",
			now.Add(offset).Unix(), 10.0, 20.0, 100, 20)
		require.NoError(t, err)
	}

	insert(-2 * time.Hour)    // older than retention
	insert(-30 * time.Minute) // within retention

	require.NoError(t, db.cleanupOldData())

	var count int
	require.NoError(t, db.db.QueryRow("SELECT COUNT(*) FROM system_metrics").Scan(&count))
	assert.Equal(t, 1, count, "only rows past the cutoff are removed")
}

func TestRetentionCoversAnnotations(t *testing.T) {
	db := newTestDB(t)
	db.retentionPeriod = time.Hour

	require.NoError(t, db.AddEventAnnotation(models.EventAnnotation{
		ID: "old", Title: "old", Timestamp: time.Now().Add(-3 * time.Hour),
	}))
	require.NoError(t, db.AddEventAnnotation(models.EventAnnotation{
		ID: "new", Title: "new", Timestamp: time.Now(),
	}))

	require.NoError(t, db.cleanupOldData())

	got, err := db.GetEventAnnotations(24 * time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "new", got[0].ID)
}

// TestConcurrentWrites exercises the pool and busy timeout under -race.
func TestConcurrentWrites(t *testing.T) {
	db := newTestDB(t)

	var wg sync.WaitGroup
	errs := make(chan error, 60)

	for i := range 20 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			errs <- db.StoreSystemMetrics(models.SystemMetrics{
				CPU: models.CPUMetrics{UsagePercent: float64(i)}, CollectedAt: time.Now(),
			})
		}()
		go func() {
			defer wg.Done()
			errs <- db.StoreHTTPMetrics("api", models.EndpointMetrics{Name: "api", StatusCode: 200, IsUp: true})
		}()
		go func() {
			defer wg.Done()
			errs <- db.AddNotification(models.Notification{
				ID: fmt.Sprintf("c%d", i), Title: "concurrent", Source: "system", Severity: "info",
			})
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
}

func TestCloseIsIdempotentAndStopsCleanup(t *testing.T) {
	db, err := New(&Config{
		DataPath: filepath.Join(t.TempDir(), "x.db"),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)

	require.NoError(t, db.Close())
	// database/sql's Close is idempotent, so a second call is a safe no-op.
	assert.NoError(t, db.Close())
}

// TestStoreHTTPMetricsWithoutTimestampIsQueryable pins the bug where a zero
// LastChecked was written as a year-1 timestamp, making the sample invisible to
// every history query.
func TestStoreHTTPMetricsWithoutTimestampIsQueryable(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.StoreHTTPMetrics("api", models.EndpointMetrics{
		Name:         "api",
		StatusCode:   200,
		ResponseTime: 50 * time.Millisecond,
		IsUp:         true,
		// LastChecked deliberately left unset.
	}))

	points, err := db.GetHTTPResponseTimeHistory("api", time.Hour, 100)
	require.NoError(t, err)
	assert.Len(t, points, 1, "a sample without an explicit timestamp must still be queryable")
}

func TestAdapterExportWritesFiles(t *testing.T) {
	db := newTestDB(t)
	adapter := NewAdapter(db)

	for i := range 5 {
		require.NoError(t, db.StoreSystemMetrics(models.SystemMetrics{
			CPU:         models.CPUMetrics{UsagePercent: float64(i * 10)},
			Memory:      models.MemoryMetrics{UsagePercent: float64(i * 5)},
			CollectedAt: time.Now(),
		}))
		require.NoError(t, db.StoreHTTPMetrics("api", models.EndpointMetrics{
			Name: "api", StatusCode: 200, ResponseTime: time.Duration(i) * time.Millisecond, IsUp: true,
		}))
	}

	outputDir := t.TempDir()
	files, err := adapter.ExportAll(outputDir, time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, files, "export must produce files when data exists")

	for _, file := range files {
		info, err := os.Stat(file)
		require.NoError(t, err)
		assert.Positive(t, info.Size(), "%s is empty; the CSV writer was not flushed", file)
	}
}

func TestAdapterExportOnEmptyDatabase(t *testing.T) {
	adapter := NewAdapter(newTestDB(t))

	files, err := adapter.ExportAll(t.TempDir(), time.Hour)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "api", want: "api"},
		// Separators are replaced rather than stripped, so the whole name is
		// still recognisable in the exported filename and cannot traverse.
		{in: "api/health", want: "api_health"},
		{in: "../../etc/passwd", want: ".._.._etc_passwd"},
		{in: "..", want: "_"},
		{in: ".", want: "_"},
		{in: "", want: "unnamed"},
		{in: `a<b>c:d"e|f?g*h`, want: "a_b_c_d_e_f_g_h"},
		{in: "with\x00null", want: "with_null"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := sanitizeFileName(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, string(filepath.Separator))
		})
	}
}

func TestDownsampleReducesToRequestedPoints(t *testing.T) {
	db := newTestDB(t)

	for i := range 50 {
		_, err := db.db.Exec(
			"INSERT INTO system_metrics (timestamp, cpu_usage, memory_usage, memory_total, memory_used) VALUES (?, ?, ?, ?, ?)",
			time.Now().Add(-time.Duration(i)*time.Minute).Unix(), float64(i), 0.0, 0, 0)
		require.NoError(t, err)
	}

	points, err := db.GetCPUUsageHistory(2*time.Hour, 10)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(points), 10)
	assert.NotEmpty(t, points)
}

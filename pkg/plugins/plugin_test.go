package plugins

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePlugin is a test double whose behaviour each test configures.
type fakePlugin struct {
	name          string
	panicOn       string
	collectErr    error
	collectDelay  time.Duration
	metrics       []models.Metric
	notifications []models.Notification

	mu            sync.Mutex
	initCalls     int
	collectCalls  int
	shutdownCalls int
}

func (p *fakePlugin) Initialize(map[string]any) error {
	p.mu.Lock()
	p.initCalls++
	p.mu.Unlock()

	if p.panicOn == "initialize" {
		panic("boom in initialize")
	}
	return nil
}

func (p *fakePlugin) Name() string        { return p.name }
func (p *fakePlugin) Version() string     { return "1.0.0" }
func (p *fakePlugin) Description() string { return "fake plugin" }

func (p *fakePlugin) Collect(ctx context.Context) (any, error) {
	p.mu.Lock()
	p.collectCalls++
	p.mu.Unlock()

	if p.panicOn == "collect" {
		panic("boom in collect")
	}
	if p.collectDelay > 0 {
		select {
		case <-time.After(p.collectDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, p.collectErr
}

func (p *fakePlugin) GetMetrics() []models.Metric {
	if p.panicOn == "metrics" {
		panic("boom in metrics")
	}
	return p.metrics
}

func (p *fakePlugin) GetNotifications() []models.Notification { return p.notifications }
func (p *fakePlugin) GetConfigSchema() map[string]string {
	return map[string]string{"interval": "collection interval"}
}

func (p *fakePlugin) Shutdown() error {
	p.mu.Lock()
	p.shutdownCalls++
	p.mu.Unlock()

	if p.panicOn == "shutdown" {
		panic("boom in shutdown")
	}
	return nil
}

func (p *fakePlugin) counts() (int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initCalls, p.collectCalls, p.shutdownCalls
}

var _ Plugin = (*fakePlugin)(nil)

// register injects a plugin without going through the native loader, so manager
// behaviour is testable in the default build.
func register(pm *PluginManager, plugin Plugin) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.plugins[plugin.Name()] = plugin
	pm.paths[plugin.Name()] = "/test/" + plugin.Name() + ".so"
}

func TestPluginsAreReturnedInStableOrder(t *testing.T) {
	pm := NewPluginManager()
	for _, name := range []string{"zeta", "alpha", "middle"} {
		register(pm, &fakePlugin{name: name})
	}

	got := pm.Plugins()
	require.Len(t, got, 3)
	assert.Equal(t, "alpha", got[0].Name())
	assert.Equal(t, "middle", got[1].Name())
	assert.Equal(t, "zeta", got[2].Name())
}

func TestGetPluginReportsNotFound(t *testing.T) {
	pm := NewPluginManager()

	_, err := pm.GetPlugin("absent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrNotFound))
}

// TestPanicInCollectIsContained pins the guard: a third-party plugin must not be
// able to take the dashboard down.
func TestPanicInCollectIsContained(t *testing.T) {
	pm := NewPluginManager()
	broken := &fakePlugin{name: "broken", panicOn: "collect"}
	healthy := &fakePlugin{
		name:    "healthy",
		metrics: []models.Metric{{Name: "ok", Value: 1}},
	}
	register(pm, broken)
	register(pm, healthy)

	var metrics []models.Metric
	assert.NotPanics(t, func() {
		metrics, _ = pm.CollectAll(context.Background())
	})

	// The healthy plugin still reported despite its neighbour panicking.
	require.Len(t, metrics, 1)
	assert.Equal(t, "ok", metrics[0].Name)

	events := pm.DrainEvents()
	require.NotEmpty(t, events)
	assert.Equal(t, EventFailed, events[0].Kind)
	assert.Equal(t, "broken", events[0].Plugin)
}

func TestPanicInInitializeIsContained(t *testing.T) {
	pm := NewPluginManager()
	register(pm, &fakePlugin{name: "broken", panicOn: "initialize"})

	var err error
	assert.NotPanics(t, func() {
		err = pm.InitializePlugin("broken", map[string]any{})
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "panicked")
}

func TestPanicInShutdownIsContained(t *testing.T) {
	pm := NewPluginManager()
	register(pm, &fakePlugin{name: "broken", panicOn: "shutdown"})

	var errs []error
	assert.NotPanics(t, func() { errs = pm.ShutdownAll() })

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "panicked")
	assert.Empty(t, pm.Plugins(), "the plugin is forgotten even when shutdown fails")
}

func TestPanicInMetricsIsContained(t *testing.T) {
	pm := NewPluginManager()
	register(pm, &fakePlugin{name: "broken", panicOn: "metrics"})

	assert.NotPanics(t, func() { pm.CollectAll(context.Background()) })
}

// TestCollectTimeoutIsEnforced pins the per-plugin deadline.
func TestCollectTimeoutIsEnforced(t *testing.T) {
	pm := NewPluginManager()
	register(pm, &fakePlugin{name: "slow", collectDelay: 5 * time.Second})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	pm.CollectAll(ctx)

	assert.Less(t, time.Since(start), 2*time.Second, "a slow plugin must not stall collection")
}

func TestCollectErrorIsReportedAsAnEvent(t *testing.T) {
	pm := NewPluginManager()
	register(pm, &fakePlugin{name: "failing", collectErr: errors.New("upstream down")})

	pm.CollectAll(context.Background())

	events := pm.DrainEvents()
	require.Len(t, events, 1)
	assert.Equal(t, EventFailed, events[0].Kind)
	assert.Contains(t, events[0].Err.Error(), "upstream down")
}

func TestCollectAllAggregatesMetricsAndNotifications(t *testing.T) {
	pm := NewPluginManager()
	register(pm, &fakePlugin{
		name:          "a",
		metrics:       []models.Metric{{Name: "a.one"}},
		notifications: []models.Notification{{ID: "n1", Title: "from a"}},
	})
	register(pm, &fakePlugin{
		name:    "b",
		metrics: []models.Metric{{Name: "b.one"}, {Name: "b.two"}},
	})

	metrics, notifications := pm.CollectAll(context.Background())

	assert.Len(t, metrics, 3)
	require.Len(t, notifications, 1)
	assert.Equal(t, "from a", notifications[0].Title)
}

func TestUnloadPlugin(t *testing.T) {
	pm := NewPluginManager()
	plugin := &fakePlugin{name: "target"}
	register(pm, plugin)

	require.NoError(t, pm.UnloadPlugin("target"))

	_, _, shutdowns := plugin.counts()
	assert.Equal(t, 1, shutdowns)
	assert.Empty(t, pm.Plugins())

	err := pm.UnloadPlugin("target")
	assert.True(t, errors.Is(err, models.ErrNotFound))
}

// TestEventQueueIsBounded stops a plugin churning on disk from growing the queue
// without limit.
func TestEventQueueIsBounded(t *testing.T) {
	pm := NewPluginManager()

	for i := range maxEventQueue * 3 {
		pm.queueEvent(Event{Kind: EventReloaded, Plugin: "p"})
		_ = i
	}

	events := pm.DrainEvents()
	assert.Len(t, events, maxEventQueue)
	assert.Empty(t, pm.DrainEvents(), "draining must clear the queue")
}

func TestDrainEventsOnEmptyQueue(t *testing.T) {
	assert.Nil(t, NewPluginManager().DrainEvents())
}

func TestEnabledAndAllowedAreCopied(t *testing.T) {
	pm := NewPluginManager()

	enabled := []string{"a", "b"}
	pm.SetEnabledPlugins(enabled)
	enabled[0] = "mutated"
	assert.Equal(t, []string{"a", "b"}, pm.EnabledPlugins(),
		"the manager must not alias the caller's slice")

	allow := map[string]string{"a": "sha256:abc"}
	pm.SetAllowedDigests(allow)
	allow["a"] = "tampered"

	pm.mu.RLock()
	got := pm.allow["a"]
	pm.mu.RUnlock()
	assert.Equal(t, "sha256:abc", got, "the manager must not alias the caller's map")
}

func TestPluginPathsAreCopied(t *testing.T) {
	pm := NewPluginManager()
	register(pm, &fakePlugin{name: "a"})

	paths := pm.PluginPaths()
	paths["a"] = "tampered"

	assert.NotEqual(t, "tampered", pm.PluginPaths()["a"])
}

// TestLoadEnabledWithoutPluginsConfigured must not error in a build without
// native plugin support, because nothing was asked for.
func TestLoadEnabledWithoutPluginsConfigured(t *testing.T) {
	pm := NewPluginManager()

	loaded, errs := pm.LoadEnabled("")
	assert.Empty(t, loaded)
	assert.Empty(t, errs)

	require.NoError(t, pm.StartWatcher(""))
	assert.NotPanics(t, pm.StopWatcher)
}

// TestLoadEnabledReportsUnsupportedWhenRequested documents the default build's
// behaviour: plugin loading needs cgo and is opt-in.
func TestLoadEnabledReportsUnsupportedWhenRequested(t *testing.T) {
	if SupportsNativePlugins() {
		t.Skip("this build supports native plugins")
	}

	pm := NewPluginManager()
	pm.SetEnabledPlugins([]string{"sample"})

	_, errs := pm.LoadEnabled(t.TempDir())
	require.Len(t, errs, 1)
	assert.True(t, errors.Is(errs[0], ErrPluginsNotSupported))
}

func TestStopWatcherIsSafeWhenNeverStarted(t *testing.T) {
	pm := NewPluginManager()

	assert.NotPanics(t, pm.StopWatcher)
	assert.NotPanics(t, pm.StopWatcher)
}

// TestConcurrentManagerAccess exercises the lock discipline under -race.
func TestConcurrentManagerAccess(t *testing.T) {
	pm := NewPluginManager()
	for _, name := range []string{"a", "b", "c"} {
		register(pm, &fakePlugin{name: name, metrics: []models.Metric{{Name: name}}})
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(4)
		go func() { defer wg.Done(); pm.Plugins() }()
		go func() { defer wg.Done(); pm.CollectAll(context.Background()) }()
		go func() { defer wg.Done(); pm.PluginPaths() }()
		go func() { defer wg.Done(); pm.DrainEvents() }()
	}
	wg.Wait()
}

// TestGuardReturnsErrorsUnchanged ensures the panic guard is transparent for the
// non-panicking path.
func TestGuardReturnsErrorsUnchanged(t *testing.T) {
	sentinel := errors.New("plain error")

	err := guard("op", "p", func() error { return sentinel })
	assert.Same(t, sentinel, err)

	assert.NoError(t, guard("op", "p", func() error { return nil }))
}

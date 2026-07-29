package collector

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingStore captures what collectors persist and can be made to fail.
type recordingStore struct {
	mu     sync.Mutex
	system []models.SystemMetrics
	http   []models.EndpointMetrics
	git    []models.GitRepoMetrics
	cloud  []models.CloudSummary
	k8s    []models.KubernetesSummary
	cicd   []models.CICDSummary
	fail   bool
}

func (s *recordingStore) StoreSystemMetrics(m models.SystemMetrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("store unavailable")
	}
	s.system = append(s.system, m)
	return nil
}

func (s *recordingStore) StoreHTTPMetrics(name string, m models.EndpointMetrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("store unavailable")
	}
	s.http = append(s.http, m)
	return nil
}

func (s *recordingStore) StoreGitMetrics(m models.GitRepoMetrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("store unavailable")
	}
	s.git = append(s.git, m)
	return nil
}

func (s *recordingStore) StoreCloudSummary(summary models.CloudSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("store unavailable")
	}
	s.cloud = append(s.cloud, summary)
	return nil
}

func (s *recordingStore) StoreKubernetesSummary(summary models.KubernetesSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("store unavailable")
	}
	s.k8s = append(s.k8s, summary)
	return nil
}

func (s *recordingStore) StoreCICDSummary(summary models.CICDSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("store unavailable")
	}
	s.cicd = append(s.cicd, summary)
	return nil
}

func (s *recordingStore) counts() (int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.system), len(s.http), len(s.git)
}

// infraCounts reports how many provider summaries were recorded.
func (s *recordingStore) infraCounts() (int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cloud), len(s.k8s), len(s.cicd)
}

// lastCloud returns the most recent cloud summary.
func (s *recordingStore) lastCloud() models.CloudSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cloud) == 0 {
		return models.CloudSummary{}
	}
	return s.cloud[len(s.cloud)-1]
}

// recordingStore must satisfy the interface collectors depend on. A concrete
// signature makes a mismatch a compile error rather than a runtime assertion
// whose error was discarded.
var _ StorageProvider = (*recordingStore)(nil)

// TestStartRejectsNonPositiveInterval pins the NewTicker(0) panic.
func TestStartRejectsNonPositiveInterval(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second} {
		c := NewSystemMetricsCollector()
		err := c.Start(context.Background(), interval)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidInterval))
		assert.False(t, c.IsRunning())
	}
}

// TestStopWaitsForTheLoop pins the goroutine ownership fix: Stop must not return
// while collection is still running, or a collector keeps writing to a store that
// has already been closed.
func TestStopWaitsForTheLoop(t *testing.T) {
	var running atomic.Bool
	var iterations atomic.Int64

	base := NewBaseCollector("test")
	require.NoError(t, base.start(context.Background(), 5*time.Millisecond, func(ctx context.Context) {
		running.Store(true)
		iterations.Add(1)
		defer running.Store(false)
		time.Sleep(2 * time.Millisecond)
	}))

	// Let a few cycles happen.
	time.Sleep(30 * time.Millisecond)
	require.Positive(t, iterations.Load())

	require.NoError(t, base.Stop())

	assert.False(t, running.Load(), "Stop must not return while a collection is in flight")
	assert.False(t, base.IsRunning())

	after := iterations.Load()
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, after, iterations.Load(), "no collection may happen after Stop returns")
}

func TestStopIsIdempotentAndSafeWhenNeverStarted(t *testing.T) {
	base := NewBaseCollector("test")

	require.NoError(t, base.Stop())
	require.NoError(t, base.Stop())

	require.NoError(t, base.start(context.Background(), time.Hour, func(context.Context) {}))
	require.NoError(t, base.Stop())
	require.NoError(t, base.Stop())
}

func TestStartIsIdempotent(t *testing.T) {
	base := NewBaseCollector("test")
	var calls atomic.Int64

	collect := func(context.Context) { calls.Add(1) }
	require.NoError(t, base.start(context.Background(), time.Hour, collect))
	require.NoError(t, base.start(context.Background(), time.Hour, collect))

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, base.Stop())

	assert.Equal(t, int64(1), calls.Load(), "a second Start must not launch another loop")
}

// TestCancellingContextStopsTheLoop covers shutdown driven from outside.
func TestCancellingContextStopsTheLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	base := NewBaseCollector("test")
	var calls atomic.Int64
	require.NoError(t, base.start(ctx, 5*time.Millisecond, func(context.Context) { calls.Add(1) }))

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	settled := calls.Load()
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, settled, calls.Load(), "cancelling the context must stop collection")

	require.NoError(t, base.Stop())
}

func TestStorageProviderIsOptional(t *testing.T) {
	c := NewSystemMetricsCollector()
	assert.Nil(t, c.Storage())

	store := &recordingStore{}
	c.SetStorageProvider(store)
	assert.NotNil(t, c.Storage())

	c.SetStorageProvider(nil)
	assert.Nil(t, c.Storage())
}

func TestSystemCollectorPersistsAndPublishes(t *testing.T) {
	store := &recordingStore{}
	c := NewSystemMetricsCollector()
	c.SetStorageProvider(store)

	updates, id := c.Subscribe()
	require.NotEmpty(t, id)
	defer c.Unsubscribe(id)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	system, _, _ := store.counts()
	assert.Equal(t, 1, system, "the sample must be persisted")

	select {
	case sample := <-updates:
		assert.False(t, sample.CollectedAt.IsZero())
	case <-time.After(time.Second):
		t.Fatal("subscriber received no sample")
	}

	latest := c.GetLatestMetrics()
	assert.False(t, latest.CollectedAt.IsZero())
	assert.NotEmpty(t, latest.Disk.Filesystems, "at least one filesystem must be reported")
}

// TestSystemCollectorReportsRealProcesses pins the removal of the hardcoded
// "sample-process" row.
func TestSystemCollectorReportsRealProcesses(t *testing.T) {
	c := NewSystemMetricsCollector()

	// The sweep runs on its own cadence, so it is driven directly here rather
	// than waiting for the sampler's interval.
	c.sampleProcesses(context.Background())

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	processes := c.GetLatestMetrics().Processes
	require.NotEmpty(t, processes, "the process table must be populated from the OS")
	assert.LessOrEqual(t, len(processes), topProcessCount)

	for _, proc := range processes {
		assert.Positive(t, proc.PID)
		assert.NotEmpty(t, proc.Name)
		assert.NotEqual(t, "sample-process", proc.Name)
	}

	// Sorted by CPU descending.
	for i := 1; i < len(processes); i++ {
		assert.GreaterOrEqual(t, processes[i-1].CPUPercent, processes[i].CPUPercent)
	}
}

// TestStoreFailureDoesNotBreakCollection covers the logged-not-swallowed path.
// TestSystemCollectRemainsFastWithoutAProcessSample pins the fix for process
// enumeration blocking the whole cycle: CPU, memory, disk and network must be
// recorded even before the first process sweep completes.
func TestSystemCollectRemainsFastWithoutAProcessSample(t *testing.T) {
	store := &recordingStore{}
	c := NewSystemMetricsCollector()
	c.SetStorageProvider(store)

	start := time.Now()
	_, err := c.Collect(context.Background())
	require.NoError(t, err)
	elapsed := time.Since(start)

	// Enumerating processes takes seconds on a busy machine; the fast path must
	// not wait for it.
	assert.Less(t, elapsed, 2*time.Second,
		"a collection cycle must not block on process enumeration")

	system, _, _ := store.counts()
	assert.Equal(t, 1, system, "the sample must be recorded even with no process data yet")

	metrics := c.GetLatestMetrics()
	assert.NotEmpty(t, metrics.Disk.Filesystems)
	assert.Empty(t, metrics.Processes, "the process table is empty until the sampler runs")
}

// TestProcessSamplerStopsWithTheCollector ensures the extra goroutine is owned.
func TestProcessSamplerStopsWithTheCollector(t *testing.T) {
	c := NewSystemMetricsCollector()

	require.NoError(t, c.Start(context.Background(), 50*time.Millisecond))
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, c.Stop())

	select {
	case <-c.procDone:
	default:
		t.Fatal("the process sampler goroutine outlived Stop")
	}
}

func TestStoreFailureDoesNotBreakCollection(t *testing.T) {
	store := &recordingStore{fail: true}
	c := NewSystemMetricsCollector()
	c.SetStorageProvider(store)

	_, err := c.Collect(context.Background())
	assert.NoError(t, err, "a store failure must not fail collection")
}

func TestCollectRespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := NewSystemMetricsCollector()
	_, err := c.Collect(ctx)
	require.Error(t, err, "a cancelled context must surface as an error")
}

// TestSubscribeUnsubscribeUnderLoad is the regression test for the
// send-on-closed-channel panic: publishing and closing now happen under one
// mutex, so the two can no longer race.
func TestSubscribeUnsubscribeUnderLoad(t *testing.T) {
	b := newBroadcaster[int]()

	var (
		churn     sync.WaitGroup // subscribers and the closer
		publisher sync.WaitGroup // the publisher, stopped last
		stop      = make(chan struct{})
	)

	// Continuous publishing. It is tracked separately because it only exits once
	// the churn has finished and stop is closed; waiting on it in the same group
	// would deadlock.
	publisher.Add(1)
	go func() {
		defer publisher.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				b.publish(i)
			}
		}
	}()

	// Churn subscribers against the publisher.
	for range 8 {
		churn.Add(1)
		go func() {
			defer churn.Done()
			for range 200 {
				ch, id := b.subscribe(2)
				// Drain sometimes so the buffer is alternately full and not.
				select {
				case <-ch:
				default:
				}
				b.unsubscribe(id)
			}
		}()
	}

	// Concurrent shutdown, racing the publisher and the churn.
	churn.Add(1)
	go func() {
		defer churn.Done()
		time.Sleep(20 * time.Millisecond)
		b.closeAll()
	}()

	churn.Wait()
	close(stop)
	publisher.Wait()

	assert.Zero(t, b.subscriberCount())
}

func TestBroadcasterUnsubscribeIsIdempotent(t *testing.T) {
	b := newBroadcaster[int]()
	ch, id := b.subscribe(1)

	b.unsubscribe(id)
	assert.NotPanics(t, func() { b.unsubscribe(id) })
	assert.NotPanics(t, func() { b.unsubscribe("never-existed") })

	_, open := <-ch
	assert.False(t, open, "the channel must be closed exactly once")
}

func TestBroadcasterDropsForSlowSubscribers(t *testing.T) {
	b := newBroadcaster[int]()
	ch, _ := b.subscribe(1)

	// More publishes than the buffer can hold must not block.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 100 {
			b.publish(i)
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish blocked on a slow subscriber")
	}

	assert.Len(t, ch, 1, "only the buffered sample is retained")
}

func TestBroadcasterAfterCloseReturnsClosedChannel(t *testing.T) {
	b := newBroadcaster[int]()
	b.closeAll()

	ch, id := b.subscribe(1)
	assert.Empty(t, id)

	_, open := <-ch
	assert.False(t, open, "subscribing after close must not block the caller forever")

	assert.NotPanics(t, func() { b.publish(1) })
	assert.NotPanics(t, func() { b.closeAll() })
}

func TestBroadcasterDeliversToEverySubscriber(t *testing.T) {
	b := newBroadcaster[string]()

	first, firstID := b.subscribe(4)
	second, secondID := b.subscribe(4)
	defer b.unsubscribe(firstID)
	defer b.unsubscribe(secondID)

	b.publish("sample")

	assert.Equal(t, "sample", <-first)
	assert.Equal(t, "sample", <-second)
}

func TestExpandPathHandlesTilde(t *testing.T) {
	got, err := ExpandPath("~")
	require.NoError(t, err)
	assert.NotContains(t, got, "~")

	got, err = ExpandPath("")
	require.NoError(t, err)
	assert.Empty(t, got)
}

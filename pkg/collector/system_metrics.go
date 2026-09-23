package collector

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	// topProcessCount is how many of the highest-CPU processes are reported.
	topProcessCount = 15

	// processInterval is how often the process table is refreshed.
	//
	// Enumerating every process costs one or more syscalls per process, which on
	// a busy machine takes far longer than a dashboard refresh. Sampling it on
	// the main path blocked the whole cycle, so CPU, memory and disk were never
	// recorded at all. It now runs on its own slower cadence and the last result
	// is served from cache.
	processInterval = 10 * time.Second

	// processTimeout bounds one process sweep.
	processTimeout = 15 * time.Second
)

// SystemMetricsCollector collects local system metrics.
type SystemMetricsCollector struct {
	*BaseCollector

	mu      sync.RWMutex
	metrics models.SystemMetrics

	// processes is refreshed by its own sampler, independently of the main
	// collection cycle.
	procMu    sync.RWMutex
	processes []models.ProcessMetrics

	procOnce   sync.Once
	procCancel context.CancelFunc
	procDone   chan struct{}

	subscribers *broadcaster[models.SystemMetrics]
}

// NewSystemMetricsCollector creates a new system metrics collector.
func NewSystemMetricsCollector() *SystemMetricsCollector {
	return &SystemMetricsCollector{
		BaseCollector: NewBaseCollector("system_metrics"),
		subscribers:   newBroadcaster[models.SystemMetrics](),
		procDone:      make(chan struct{}),
	}
}

// cachedProcesses returns the most recent process sample.
func (c *SystemMetricsCollector) cachedProcesses() []models.ProcessMetrics {
	c.procMu.RLock()
	defer c.procMu.RUnlock()
	return c.processes
}

// startProcessSampler runs the process sweep on its own schedule. It samples once
// immediately so the table populates without waiting a full interval.
func (c *SystemMetricsCollector) startProcessSampler(ctx context.Context) {
	c.procOnce.Do(func() {
		sampleCtx, cancel := context.WithCancel(ctx)
		c.procCancel = cancel

		go func() {
			defer close(c.procDone)

			ticker := time.NewTicker(processInterval)
			defer ticker.Stop()

			for {
				c.sampleProcesses(sampleCtx)

				select {
				case <-ticker.C:
				case <-sampleCtx.Done():
					return
				}
			}
		}()
	})
}

// sampleProcesses refreshes the cached process list.
func (c *SystemMetricsCollector) sampleProcesses(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, processTimeout)
	defer cancel()

	processes, err := collectTopProcesses(ctx, topProcessCount)
	if err != nil {
		if ctx.Err() == nil {
			c.Logger().Warn("process sampling failed", "error", err)
		}
		return
	}

	c.procMu.Lock()
	c.processes = processes
	c.procMu.Unlock()
}

// Collect gathers a full system metrics sample.
func (c *SystemMetricsCollector) Collect(ctx context.Context) (any, error) {
	metrics := models.SystemMetrics{CollectedAt: time.Now()}

	var (
		wg                              sync.WaitGroup
		cpuErr, memErr, diskErr, netErr error
	)

	// Each goroutine writes a distinct field, so the struct needs no lock.
	wg.Add(4)
	go func() {
		defer wg.Done()
		metrics.CPU, cpuErr = collectCPUMetrics(ctx)
	}()
	go func() {
		defer wg.Done()
		metrics.Memory, memErr = collectMemoryMetrics(ctx)
	}()
	go func() {
		defer wg.Done()
		metrics.Disk, diskErr = collectDiskMetrics(ctx)
	}()
	go func() {
		defer wg.Done()
		metrics.Network, netErr = collectNetworkMetrics(ctx)
	}()
	wg.Wait()

	// Served from the sampler's cache: enumerating processes is far slower than
	// a refresh cycle and must not gate the rest of the sample.
	metrics.Processes = c.cachedProcesses()

	for _, e := range []struct {
		what string
		err  error
	}{
		{"cpu", cpuErr},
		{"memory", memErr},
		{"disk", diskErr},
		{"network", netErr},
	} {
		if e.err != nil {
			return nil, fmt.Errorf("collecting %s metrics: %w", e.what, e.err)
		}
	}

	c.mu.Lock()
	c.metrics = metrics
	c.mu.Unlock()

	c.UpdateData(metrics)
	c.subscribers.publish(metrics)
	c.store(metrics)

	return metrics, nil
}

// store persists the sample when a store is configured. Failures are logged
// rather than discarded: a database that has become unwritable would otherwise
// look identical to one that is recording correctly.
func (c *SystemMetricsCollector) store(metrics models.SystemMetrics) {
	store := c.Storage()
	if store == nil {
		return
	}
	if err := store.StoreSystemMetrics(metrics); err != nil {
		c.Logger().Error("failed to store system metrics", "error", err)
	}
}

// GetLatestMetrics returns the most recent sample.
func (c *SystemMetricsCollector) GetLatestMetrics() models.SystemMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// Subscribe returns a channel receiving every new sample, plus an id to pass to
// Unsubscribe. The channel is owned and closed by the collector.
func (c *SystemMetricsCollector) Subscribe() (<-chan models.SystemMetrics, string) {
	return c.subscribers.subscribe(subscriberBuffer)
}

// Unsubscribe releases a subscription.
func (c *SystemMetricsCollector) Unsubscribe(id string) {
	c.subscribers.unsubscribe(id)
}

// Start begins periodic collection, plus the independent process sampler.
func (c *SystemMetricsCollector) Start(ctx context.Context, interval time.Duration) error {
	if err := c.start(ctx, interval, func(ctx context.Context) {
		if _, err := c.Collect(ctx); err != nil && ctx.Err() == nil {
			c.Logger().Warn("system metrics collection failed", "error", err)
		}
	}); err != nil {
		return err
	}

	c.startProcessSampler(ctx)
	return nil
}

// Stop halts collection, waits for the process sampler, and releases all
// subscribers.
func (c *SystemMetricsCollector) Stop() error {
	err := c.BaseCollector.Stop()

	if c.procCancel != nil {
		c.procCancel()
		<-c.procDone
	}

	c.subscribers.closeAll()
	return err
}

func collectCPUMetrics(ctx context.Context) (models.CPUMetrics, error) {
	metrics := models.CPUMetrics{}

	// Overall utilisation is the only reading treated as required.
	overall, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return metrics, err
	}
	if len(overall) > 0 {
		metrics.UsagePercent = overall[0]
	}

	// Per-core utilisation is best-effort. gopsutil v3 reported "not implemented
	// yet" for it on some platforms, and treating that as fatal failed the whole
	// system sample, so CPU, memory and disk were never recorded there at all.
	if perCore, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
		metrics.CoreUsage = perCore
	}

	// Load average is unavailable on Windows.
	if runtime.GOOS != "windows" {
		if avg, err := load.AvgWithContext(ctx); err == nil {
			metrics.LoadAverage = models.LoadAverageMetrics{
				Load1:  avg.Load1,
				Load5:  avg.Load5,
				Load15: avg.Load15,
			}
		}
	}

	return metrics, nil
}

func collectMemoryMetrics(ctx context.Context) (models.MemoryMetrics, error) {
	metrics := models.MemoryMetrics{}

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return metrics, err
	}
	metrics.Total = vm.Total
	metrics.Used = vm.Used
	metrics.Free = vm.Free
	metrics.UsagePercent = vm.UsedPercent

	// Swap is reported as zero on systems without it rather than as an error.
	if swap, err := mem.SwapMemoryWithContext(ctx); err == nil {
		metrics.SwapTotal = swap.Total
		metrics.SwapUsed = swap.Used
		metrics.SwapFree = swap.Free
	}

	return metrics, nil
}

func collectDiskMetrics(ctx context.Context) (models.DiskMetrics, error) {
	metrics := models.DiskMetrics{Filesystems: []models.FilesystemMetrics{}}

	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return metrics, err
	}

	for _, partition := range partitions {
		// Unreadable mount points (permission denied, disconnected network
		// shares) are skipped: one bad mount must not fail the whole sample.
		usage, err := disk.UsageWithContext(ctx, partition.Mountpoint)
		if err != nil {
			continue
		}

		metrics.Filesystems = append(metrics.Filesystems, models.FilesystemMetrics{
			MountPoint:   partition.Mountpoint,
			Total:        usage.Total,
			Used:         usage.Used,
			Free:         usage.Free,
			UsagePercent: usage.UsedPercent,
		})
	}

	return metrics, nil
}

func collectNetworkMetrics(ctx context.Context) (models.NetworkMetrics, error) {
	metrics := models.NetworkMetrics{Interfaces: []models.InterfaceMetrics{}}

	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return metrics, err
	}

	interfaces, err := net.InterfacesWithContext(ctx)
	if err != nil {
		return metrics, err
	}

	known := make(map[string]struct{}, len(interfaces))
	for _, iface := range interfaces {
		known[iface.Name] = struct{}{}
	}

	for _, counter := range counters {
		if _, ok := known[counter.Name]; !ok {
			continue
		}
		metrics.Interfaces = append(metrics.Interfaces, models.InterfaceMetrics{
			Name:        counter.Name,
			BytesSent:   counter.BytesSent,
			BytesRecv:   counter.BytesRecv,
			PacketsSent: counter.PacketsSent,
			PacketsRecv: counter.PacketsRecv,
		})
	}

	return metrics, nil
}

// collectTopProcesses returns the limit highest-CPU processes.
//
// The sweep is two-phase: every process is ranked using only its CPU share, then
// the expensive details (name, command line, memory) are read for the handful
// that will actually be displayed. Reading all of them for every process meant
// several syscalls per process across hundreds of processes.
//
// Processes that exit while being inspected are skipped, which is normal on a
// busy system.
func collectTopProcesses(ctx context.Context, limit int) ([]models.ProcessMetrics, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	type ranked struct {
		proc *process.Process
		cpu  float64
	}

	candidates := make([]ranked, 0, len(procs))
	for _, p := range procs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		cpuPercent, err := p.CPUPercentWithContext(ctx)
		if err != nil {
			continue
		}
		candidates = append(candidates, ranked{proc: p, cpu: cpuPercent})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].cpu > candidates[j].cpu
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	collected := make([]models.ProcessMetrics, 0, len(candidates))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		entry := models.ProcessMetrics{
			PID:        candidate.proc.Pid,
			CPUPercent: candidate.cpu,
		}

		// Details are best-effort: reading them for another user's process is
		// often denied, and that must not drop the row.
		if name, err := candidate.proc.NameWithContext(ctx); err == nil {
			entry.Name = name
			entry.Command = name
		}
		if cmdline, err := candidate.proc.CmdlineWithContext(ctx); err == nil && cmdline != "" {
			entry.Command = cmdline
		}
		if memPercent, err := candidate.proc.MemoryPercentWithContext(ctx); err == nil {
			entry.MemoryPercent = float64(memPercent)
		}
		if memInfo, err := candidate.proc.MemoryInfoWithContext(ctx); err == nil && memInfo != nil {
			entry.MemoryBytes = memInfo.RSS
		}

		if entry.Name == "" {
			entry.Name = fmt.Sprintf("pid-%d", entry.PID)
			entry.Command = entry.Name
		}

		collected = append(collected, entry)
	}

	return collected, nil
}

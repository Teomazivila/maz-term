package collector

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// topProcessCount is how many of the highest-CPU processes are reported. The
// process table only has room for a handful, and enumerating every process is
// the most expensive part of a collection cycle.
const topProcessCount = 15

// SystemMetricsCollector collects local system metrics.
type SystemMetricsCollector struct {
	*BaseCollector

	mu      sync.RWMutex
	metrics models.SystemMetrics

	subscribers *broadcaster[models.SystemMetrics]
}

// NewSystemMetricsCollector creates a new system metrics collector.
func NewSystemMetricsCollector() *SystemMetricsCollector {
	return &SystemMetricsCollector{
		BaseCollector: NewBaseCollector("system_metrics"),
		subscribers:   newBroadcaster[models.SystemMetrics](),
	}
}

// Collect gathers a full system metrics sample.
func (c *SystemMetricsCollector) Collect(ctx context.Context) (any, error) {
	metrics := models.SystemMetrics{CollectedAt: time.Now()}

	var (
		wg                                       sync.WaitGroup
		cpuErr, memErr, diskErr, netErr, procErr error
	)

	// Each goroutine writes a distinct field, so the struct needs no lock.
	wg.Add(5)
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
	go func() {
		defer wg.Done()
		metrics.Processes, procErr = collectTopProcesses(ctx, topProcessCount)
	}()
	wg.Wait()

	for _, e := range []struct {
		what string
		err  error
	}{
		{"cpu", cpuErr},
		{"memory", memErr},
		{"disk", diskErr},
		{"network", netErr},
		{"processes", procErr},
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

// Start begins periodic collection.
func (c *SystemMetricsCollector) Start(ctx context.Context, interval time.Duration) error {
	return c.start(ctx, interval, func(ctx context.Context) {
		if _, err := c.Collect(ctx); err != nil && ctx.Err() == nil {
			c.Logger().Warn("system metrics collection failed", "error", err)
		}
	})
}

// Stop halts collection and releases all subscribers.
func (c *SystemMetricsCollector) Stop() error {
	err := c.BaseCollector.Stop()
	c.subscribers.closeAll()
	return err
}

func collectCPUMetrics(ctx context.Context) (models.CPUMetrics, error) {
	metrics := models.CPUMetrics{}

	overall, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return metrics, err
	}
	if len(overall) > 0 {
		metrics.UsagePercent = overall[0]
	}

	perCore, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		return metrics, err
	}
	metrics.CoreUsage = perCore

	// Load average is not available on Windows.
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

// collectTopProcesses returns the limit highest-CPU processes. Processes that
// exit while being inspected are skipped, which is expected on a busy system.
func collectTopProcesses(ctx context.Context, limit int) ([]models.ProcessMetrics, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	collected := make([]models.ProcessMetrics, 0, len(procs))
	for _, p := range procs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		cpuPercent, err := p.CPUPercentWithContext(ctx)
		if err != nil {
			continue
		}

		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}

		entry := models.ProcessMetrics{
			PID:        p.Pid,
			Name:       name,
			Command:    name,
			CPUPercent: cpuPercent,
		}

		// Command line and memory are best-effort: reading them for another
		// user's process is often denied, and that must not drop the row.
		if cmdline, err := p.CmdlineWithContext(ctx); err == nil && cmdline != "" {
			entry.Command = cmdline
		}
		if memPercent, err := p.MemoryPercentWithContext(ctx); err == nil {
			entry.MemoryPercent = float64(memPercent)
		}
		if memInfo, err := p.MemoryInfoWithContext(ctx); err == nil && memInfo != nil {
			entry.MemoryBytes = memInfo.RSS
		}

		collected = append(collected, entry)
	}

	sort.Slice(collected, func(i, j int) bool {
		if collected[i].CPUPercent != collected[j].CPUPercent {
			return collected[i].CPUPercent > collected[j].CPUPercent
		}
		return collected[i].MemoryBytes > collected[j].MemoryBytes
	})

	if len(collected) > limit {
		collected = collected[:limit]
	}

	return collected, nil
}

package collector

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// SystemMetricsCollector collects system metrics
type SystemMetricsCollector struct {
	*BaseCollector
	metrics      models.SystemMetrics
	mutex        sync.RWMutex
	subscription []chan models.SystemMetrics
}

// NewSystemMetricsCollector creates a new system metrics collector
func NewSystemMetricsCollector() *SystemMetricsCollector {
	return &SystemMetricsCollector{
		BaseCollector: NewBaseCollector("system_metrics"),
		metrics:       models.SystemMetrics{},
		subscription:  []chan models.SystemMetrics{},
	}
}

// Collect gathers system metrics
func (c *SystemMetricsCollector) Collect(ctx context.Context) (interface{}, error) {
	// Gather all metrics in parallel
	var wg sync.WaitGroup
	var cpuErr, memErr, diskErr, networkErr error

	// Initialize the metrics
	metrics := models.SystemMetrics{
		CollectedAt: time.Now(),
	}

	// Collect CPU metrics
	wg.Add(1)
	go func() {
		defer wg.Done()
		metrics.CPU, cpuErr = c.collectCPUMetrics(ctx)
	}()

	// Collect memory metrics
	wg.Add(1)
	go func() {
		defer wg.Done()
		metrics.Memory, memErr = c.collectMemoryMetrics(ctx)
	}()

	// Collect disk metrics
	wg.Add(1)
	go func() {
		defer wg.Done()
		metrics.Disk, diskErr = c.collectDiskMetrics(ctx)
	}()

	// Collect network metrics
	wg.Add(1)
	go func() {
		defer wg.Done()
		metrics.Network, networkErr = c.collectNetworkMetrics(ctx)
	}()

	// Wait for all collectors to finish
	wg.Wait()

	// Check for errors
	if cpuErr != nil {
		return nil, fmt.Errorf("failed to collect CPU metrics: %w", cpuErr)
	}
	if memErr != nil {
		return nil, fmt.Errorf("failed to collect memory metrics: %w", memErr)
	}
	if diskErr != nil {
		return nil, fmt.Errorf("failed to collect disk metrics: %w", diskErr)
	}
	if networkErr != nil {
		return nil, fmt.Errorf("failed to collect network metrics: %w", networkErr)
	}

	// Update the metrics
	c.mutex.Lock()
	c.metrics = metrics
	c.mutex.Unlock()

	// Notify subscribers
	for _, ch := range c.subscription {
		select {
		case ch <- metrics:
			// Successfully sent
		default:
			// Channel is full or closed, skip
		}
	}

	// Store the metrics in the database
	c.StoreData("", metrics, "system")

	return metrics, nil
}

// GetLatestMetrics returns the latest metrics
func (c *SystemMetricsCollector) GetLatestMetrics() models.SystemMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.metrics
}

// Subscribe returns a channel that will receive metrics updates
func (c *SystemMetricsCollector) Subscribe() chan models.SystemMetrics {
	ch := make(chan models.SystemMetrics, 10)
	c.mutex.Lock()
	c.subscription = append(c.subscription, ch)
	c.mutex.Unlock()
	return ch
}

// Unsubscribe removes a subscription channel
func (c *SystemMetricsCollector) Unsubscribe(ch chan models.SystemMetrics) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for i, subCh := range c.subscription {
		if subCh == ch {
			c.subscription = append(c.subscription[:i], c.subscription[i+1:]...)
			close(ch)
			break
		}
	}
}

// Start starts the collector
func (c *SystemMetricsCollector) Start(ctx context.Context, interval time.Duration) error {
	if err := c.BaseCollector.Start(ctx, interval); err != nil {
		return err
	}

	// Initial collection
	if _, err := c.Collect(ctx); err != nil {
		return err
	}

	// Start periodic collection
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_, _ = c.Collect(ctx) // Ignore errors during background collection
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// collectCPUMetrics collects CPU metrics
func (c *SystemMetricsCollector) collectCPUMetrics(ctx context.Context) (models.CPUMetrics, error) {
	metrics := models.CPUMetrics{}

	// Get CPU usage percentage
	percentages, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return metrics, err
	}
	if len(percentages) > 0 {
		metrics.UsagePercent = percentages[0]
	}

	// Get per-core usage
	perCorePercentages, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		return metrics, err
	}
	metrics.CoreUsage = perCorePercentages

	// Get load average if not on Windows
	if runtime.GOOS != "windows" {
		loadAvg, err := load.AvgWithContext(ctx)
		if err == nil {
			metrics.LoadAverage = models.LoadAverageMetrics{
				Load1:  loadAvg.Load1,
				Load5:  loadAvg.Load5,
				Load15: loadAvg.Load15,
			}
		}
	}

	return metrics, nil
}

// collectMemoryMetrics collects memory metrics
func (c *SystemMetricsCollector) collectMemoryMetrics(ctx context.Context) (models.MemoryMetrics, error) {
	metrics := models.MemoryMetrics{}

	// Get virtual memory stats
	vmstat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return metrics, err
	}
	metrics.Total = vmstat.Total
	metrics.Used = vmstat.Used
	metrics.Free = vmstat.Free
	metrics.UsagePercent = vmstat.UsedPercent

	// Get swap memory stats
	swap, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return metrics, err
	}
	metrics.SwapTotal = swap.Total
	metrics.SwapUsed = swap.Used
	metrics.SwapFree = swap.Free

	return metrics, nil
}

// collectDiskMetrics collects disk metrics
func (c *SystemMetricsCollector) collectDiskMetrics(ctx context.Context) (models.DiskMetrics, error) {
	metrics := models.DiskMetrics{
		Filesystems: []models.FilesystemMetrics{},
	}

	// Get partitions
	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return metrics, err
	}

	// Get usage for each partition
	for _, partition := range partitions {
		usage, err := disk.UsageWithContext(ctx, partition.Mountpoint)
		if err != nil {
			continue // Skip on error
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

// collectNetworkMetrics collects network metrics
func (c *SystemMetricsCollector) collectNetworkMetrics(ctx context.Context) (models.NetworkMetrics, error) {
	metrics := models.NetworkMetrics{
		Interfaces: []models.InterfaceMetrics{},
	}

	// Get network interfaces
	interfaces, err := net.InterfacesWithContext(ctx)
	if err != nil {
		return metrics, err
	}

	// Get IO counters for each interface
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return metrics, err
	}

	for _, counter := range counters {
		for _, iface := range interfaces {
			if iface.Name == counter.Name {
				metrics.Interfaces = append(metrics.Interfaces, models.InterfaceMetrics{
					Name:        counter.Name,
					BytesSent:   counter.BytesSent,
					BytesRecv:   counter.BytesRecv,
					PacketsSent: counter.PacketsSent,
					PacketsRecv: counter.PacketsRecv,
				})
				break
			}
		}
	}

	return metrics, nil
}

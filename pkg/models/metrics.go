package models

import "time"

// SystemMetrics represents system-level metrics
type SystemMetrics struct {
	CPU         CPUMetrics     `json:"cpu"`
	Memory      MemoryMetrics  `json:"memory"`
	Disk        DiskMetrics    `json:"disk"`
	Network     NetworkMetrics `json:"network"`
	CollectedAt time.Time      `json:"collected_at"`
}

// CPUMetrics represents CPU metrics
type CPUMetrics struct {
	UsagePercent float64            `json:"usage_percent"`
	CoreUsage    []float64          `json:"core_usage"`
	LoadAverage  LoadAverageMetrics `json:"load_average"`
}

// LoadAverageMetrics represents load average metrics
type LoadAverageMetrics struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// MemoryMetrics represents memory metrics
type MemoryMetrics struct {
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	UsagePercent float64 `json:"usage_percent"`
	SwapTotal    uint64  `json:"swap_total"`
	SwapUsed     uint64  `json:"swap_used"`
	SwapFree     uint64  `json:"swap_free"`
}

// DiskMetrics represents disk metrics
type DiskMetrics struct {
	Filesystems []FilesystemMetrics `json:"filesystems"`
}

// FilesystemMetrics represents filesystem metrics
type FilesystemMetrics struct {
	MountPoint   string  `json:"mount_point"`
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	UsagePercent float64 `json:"usage_percent"`
}

// NetworkMetrics represents network metrics
type NetworkMetrics struct {
	Interfaces []InterfaceMetrics `json:"interfaces"`
}

// InterfaceMetrics represents network interface metrics
type InterfaceMetrics struct {
	Name        string `json:"name"`
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_recv"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_recv"`
}

// EndpointMetrics represents HTTP endpoint metrics
type EndpointMetrics struct {
	Name         string        `json:"name"`
	URL          string        `json:"url"`
	StatusCode   int           `json:"status_code"`
	ResponseTime time.Duration `json:"response_time"`
	IsUp         bool          `json:"is_up"`
	LastChecked  time.Time     `json:"last_checked"`
}

// GitRepoMetrics represents Git repository metrics
type GitRepoMetrics struct {
	Name           string    `json:"name"`
	Branch         string    `json:"branch"`
	CommitCount    int       `json:"commit_count"`
	LastCommit     time.Time `json:"last_commit"`
	PendingCommits int       `json:"pending_commits"`
	ModifiedFiles  int       `json:"modified_files"`
}

// ServiceStatus represents the status of a service
type ServiceStatus struct {
	Name      string        `json:"name"`
	Status    string        `json:"status"`
	Uptime    time.Duration `json:"uptime"`
	IsHealthy bool          `json:"is_healthy"`
	LastCheck time.Time     `json:"last_check"`
}

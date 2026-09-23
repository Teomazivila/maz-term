package models

import "time"

// SystemMetrics represents system-level metrics
type SystemMetrics struct {
	CPU         CPUMetrics       `json:"cpu"`
	Memory      MemoryMetrics    `json:"memory"`
	Disk        DiskMetrics      `json:"disk"`
	Network     NetworkMetrics   `json:"network"`
	Processes   []ProcessMetrics `json:"processes"`
	CollectedAt time.Time        `json:"collected_at"`
}

// ProcessMetrics represents a single running process, as shown in the process
// table. Only the highest-consuming processes are collected.
type ProcessMetrics struct {
	PID           int32   `json:"pid"`
	Name          string  `json:"name"`
	Command       string  `json:"command"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	MemoryBytes   uint64  `json:"memory_bytes"`
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

	// Error holds the transport-level failure for the most recent check, if
	// any. It is empty when the endpoint responded, whatever the status code.
	Error string `json:"error,omitempty"`

	// Availability is the percentage of successful checks over the collector's
	// rolling window, and ChecksInWindow is how many checks that window holds.
	// They are zero until at least one check has completed.
	Availability   float64 `json:"availability"`
	ChecksInWindow int     `json:"checks_in_window"`
}

// GitRepoMetrics represents Git repository metrics
type GitRepoMetrics struct {
	Name           string       `json:"name"`
	Path           string       `json:"path"`
	Branch         string       `json:"branch"`
	Branches       []string     `json:"branches"`
	CommitCount    int          `json:"commit_count"`
	LastCommit     time.Time    `json:"last_commit"`
	PendingCommits int          `json:"pending_commits"`
	ModifiedFiles  int          `json:"modified_files"`
	UntrackedFiles int          `json:"untracked_files"`
	ChangedFiles   []GitChange  `json:"changed_files"`
	CommitHistory  []CommitInfo `json:"commit_history"`

	// Remote and RemoteURL come from the repository's own configuration, so the
	// operator's git config, SSH agent and credential helpers are what apply.
	Remote    string `json:"remote"`
	RemoteURL string `json:"remote_url"`

	// HasUpstream reports whether the current branch tracks a remote branch.
	// Without one, PendingCommits cannot be computed.
	HasUpstream bool `json:"has_upstream"`

	// IsRepository reports whether Path is a Git work tree. Error holds the
	// reason collection failed, so the UI can say so instead of displaying
	// zeroes that look like a clean repository.
	IsRepository bool   `json:"is_repository"`
	Error        string `json:"error,omitempty"`
}

// GitChange is a single entry from git status --porcelain.
type GitChange struct {
	// Status is the two-character porcelain code, for example " M", "??" or "A ".
	Status string `json:"status"`
	Path   string `json:"path"`
}

// CommitInfo represents a single git commit
type CommitInfo struct {
	Hash      string    `json:"hash"`
	Author    string    `json:"author"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// ServiceStatus represents the status of a service
type ServiceStatus struct {
	Name      string        `json:"name"`
	Status    string        `json:"status"`
	Uptime    time.Duration `json:"uptime"`
	IsHealthy bool          `json:"is_healthy"`
	LastCheck time.Time     `json:"last_check"`
}

// Metric represents a generic metric from any data source
type Metric struct {
	// Name is the unique name of the metric
	Name string `json:"name"`

	// Value is the current value of the metric
	Value float64 `json:"value"`

	// Unit is the unit of measurement for the metric
	Unit string `json:"unit"`

	// Timestamp is when the metric was collected
	Timestamp time.Time `json:"timestamp"`

	// Source is where the metric came from (e.g., system, plugin name)
	Source string `json:"source"`

	// Tags are additional metadata for the metric
	Tags map[string]string `json:"tags,omitempty"`

	// ThresholdWarning is the warning threshold for this metric, if applicable
	ThresholdWarning *float64 `json:"threshold_warning,omitempty"`

	// ThresholdCritical is the critical threshold for this metric, if applicable
	ThresholdCritical *float64 `json:"threshold_critical,omitempty"`
}

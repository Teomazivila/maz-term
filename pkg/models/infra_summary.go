package models

import "time"

// The summary types below are what gets persisted for the infrastructure
// providers. Detailed current state — every instance, pod and workflow run — is
// held in memory for the provider tabs; only these aggregates go to storage,
// because the History tab plots time series and nothing plots per-resource
// datapoints. See docs/adr/0001-infrastructure-integrations.md.

// CloudSummary is one point-in-time aggregate of a cloud provider account.
type CloudSummary struct {
	Timestamp    time.Time `json:"timestamp"`
	ProviderType string    `json:"provider_type"`

	InstancesTotal   int `json:"instances_total"`
	InstancesRunning int `json:"instances_running"`
	InstancesStopped int `json:"instances_stopped"`

	// MeanCPUUtilization averages the running instances that reported a figure.
	MeanCPUUtilization float64 `json:"mean_cpu_utilization"`

	BucketsTotal   int `json:"buckets_total"`
	DatabasesTotal int `json:"databases_total"`

	// ErrorCount is how many regions or services failed to report. A non-zero
	// value means the other figures describe a partial view.
	ErrorCount int `json:"error_count"`
}

// CloudSummaryFrom aggregates a provider sample.
func CloudSummaryFrom(m CloudProviderMetrics) CloudSummary {
	summary := CloudSummary{
		Timestamp:      timeOrNow(m.LastUpdated),
		ProviderType:   m.ProviderType,
		InstancesTotal: len(m.InstanceMetrics),
		BucketsTotal:   len(m.StorageMetrics),
		DatabasesTotal: len(m.DatabaseMetrics),
		ErrorCount:     len(m.Errors),
	}

	var (
		cpuSum   float64
		cpuCount int
	)
	for _, instance := range m.InstanceMetrics {
		switch instance.Status {
		case ResourceStatusRunning:
			summary.InstancesRunning++
		case ResourceStatusStopped:
			summary.InstancesStopped++
		}
		// Only instances that actually reported a datapoint contribute, so an
		// account without CloudWatch permission does not skew the mean to zero.
		if instance.CPUUtilization > 0 {
			cpuSum += instance.CPUUtilization
			cpuCount++
		}
	}
	if cpuCount > 0 {
		summary.MeanCPUUtilization = cpuSum / float64(cpuCount)
	}

	return summary
}

// KubernetesSummary is one point-in-time aggregate of a cluster.
type KubernetesSummary struct {
	Timestamp   time.Time `json:"timestamp"`
	ClusterName string    `json:"cluster_name"`

	NodesTotal int `json:"nodes_total"`
	NodesReady int `json:"nodes_ready"`

	PodsTotal     int `json:"pods_total"`
	PodsRunning   int `json:"pods_running"`
	PodsPending   int `json:"pods_pending"`
	PodsFailed    int `json:"pods_failed"`
	PodsSucceeded int `json:"pods_succeeded"`

	DeploymentsTotal     int `json:"deployments_total"`
	DeploymentsAvailable int `json:"deployments_available"`

	RestartsTotal int `json:"restarts_total"`

	ErrorCount int `json:"error_count"`
}

// KubernetesSummaryFrom aggregates a cluster sample.
func KubernetesSummaryFrom(m KubernetesMetrics) KubernetesSummary {
	summary := KubernetesSummary{
		Timestamp:        timeOrNow(m.LastUpdated),
		ClusterName:      m.ClusterName,
		NodesTotal:       len(m.Nodes),
		PodsTotal:        len(m.Pods),
		DeploymentsTotal: len(m.Deployments),
		ErrorCount:       len(m.Errors),
	}

	for _, node := range m.Nodes {
		if node.Status == "Ready" {
			summary.NodesReady++
		}
	}

	for _, pod := range m.Pods {
		switch pod.Phase {
		case "Running":
			summary.PodsRunning++
		case "Pending":
			summary.PodsPending++
		case "Failed":
			summary.PodsFailed++
		case "Succeeded":
			summary.PodsSucceeded++
		}
		summary.RestartsTotal += pod.RestartCount
	}

	for _, deployment := range m.Deployments {
		if deployment.AvailableReplicas > 0 && deployment.AvailableReplicas >= deployment.DesiredReplicas {
			summary.DeploymentsAvailable++
		}
	}

	return summary
}

// CICDSummary is one point-in-time aggregate of a CI/CD provider.
type CICDSummary struct {
	Timestamp    time.Time `json:"timestamp"`
	ProviderType string    `json:"provider_type"`

	WorkflowsTotal  int `json:"workflows_total"`
	WorkflowsFailed int `json:"workflows_failed"`
	RunsRunning     int `json:"runs_running"`

	// SuccessRate is the percentage of recent runs that succeeded.
	SuccessRate float64 `json:"success_rate"`

	// MeanDurationSeconds is the mean duration of recent completed runs.
	MeanDurationSeconds float64 `json:"mean_duration_seconds"`

	ErrorCount int `json:"error_count"`
}

// CICDSummaryFrom aggregates a CI/CD sample.
func CICDSummaryFrom(m CICDMetrics) CICDSummary {
	return CICDSummary{
		Timestamp:           timeOrNow(m.LastUpdated),
		ProviderType:        string(m.ProviderType),
		WorkflowsTotal:      m.Summary.TotalWorkflows,
		WorkflowsFailed:     m.Summary.FailedWorkflows,
		RunsRunning:         m.Summary.RunningJobs,
		SuccessRate:         m.Summary.SuccessRate,
		MeanDurationSeconds: m.Summary.AverageDuration.Seconds(),
		ErrorCount:          len(m.Errors),
	}
}

// timeOrNow substitutes the current time for a zero timestamp, so a sample can
// never be written with a year-1 timestamp that time-window queries exclude.
func timeOrNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

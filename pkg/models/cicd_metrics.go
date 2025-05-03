package models

import (
	"time"
)

// CICDProviderType represents a type of CI/CD provider
type CICDProviderType string

const (
	// CICDProviderGitHubActions represents GitHub Actions
	CICDProviderGitHubActions CICDProviderType = "github_actions"

	// CICDProviderJenkins represents Jenkins
	CICDProviderJenkins CICDProviderType = "jenkins"

	// CICDProviderGitLab represents GitLab CI
	CICDProviderGitLab CICDProviderType = "gitlab_ci"
)

// CICDStatus represents the status of a CI/CD workflow/job/pipeline
type CICDStatus string

const (
	// CICDStatusSuccess represents a successful run
	CICDStatusSuccess CICDStatus = "success"

	// CICDStatusFailure represents a failed run
	CICDStatusFailure CICDStatus = "failure"

	// CICDStatusRunning represents a currently running job
	CICDStatusRunning CICDStatus = "running"

	// CICDStatusPending represents a pending job
	CICDStatusPending CICDStatus = "pending"

	// CICDStatusCancelled represents a cancelled job
	CICDStatusCancelled CICDStatus = "cancelled"

	// CICDStatusSkipped represents a skipped job
	CICDStatusSkipped CICDStatus = "skipped"
)

// CICDMetrics represents metrics for CI/CD systems
type CICDMetrics struct {
	// ProviderType is the type of CI/CD provider
	ProviderType CICDProviderType `json:"provider_type"`

	// ProviderName is a friendly name for the provider
	ProviderName string `json:"provider_name"`

	// Workflows is the list of workflow metrics
	Workflows []CICDWorkflowMetrics `json:"workflows"`

	// LastUpdated is when the metrics were last collected
	LastUpdated time.Time `json:"last_updated"`

	// Summary contains summary metrics
	Summary CICDSummaryMetrics `json:"summary"`

	// Errors contains any errors encountered during collection
	Errors []string `json:"errors,omitempty"`
}

// CICDSummaryMetrics represents summary metrics for CI/CD systems
type CICDSummaryMetrics struct {
	// TotalWorkflows is the total number of workflows monitored
	TotalWorkflows int `json:"total_workflows"`

	// ActiveWorkflows is the number of workflows with recent runs
	ActiveWorkflows int `json:"active_workflows"`

	// RunningJobs is the number of currently running jobs
	RunningJobs int `json:"running_jobs"`

	// SuccessRate is the percentage of successful runs in the recent history
	SuccessRate float64 `json:"success_rate"`

	// AverageDuration is the average duration of workflow runs
	AverageDuration time.Duration `json:"average_duration"`

	// FailedWorkflows is the number of workflows with recent failures
	FailedWorkflows int `json:"failed_workflows"`
}

// CICDWorkflowMetrics represents metrics for a CI/CD workflow
type CICDWorkflowMetrics struct {
	// ID is the unique identifier for the workflow
	ID string `json:"id"`

	// Name is the name of the workflow
	Name string `json:"name"`

	// Repository is the associated repository
	Repository string `json:"repository"`

	// RecentRuns contains metrics for recent workflow runs
	RecentRuns []CICDRunMetrics `json:"recent_runs"`

	// Enabled indicates whether the workflow is enabled
	Enabled bool `json:"enabled"`

	// AverageDuration is the average duration of workflow runs
	AverageDuration time.Duration `json:"average_duration"`

	// SuccessRate is the percentage of successful runs in the recent history
	SuccessRate float64 `json:"success_rate"`

	// LastRunStatus is the status of the most recent run
	LastRunStatus CICDStatus `json:"last_run_status"`

	// LastRunTime is when the most recent run occurred
	LastRunTime time.Time `json:"last_run_time"`

	// LastSuccessTime is when the last successful run occurred
	LastSuccessTime time.Time `json:"last_success_time"`

	// RunCount is the total number of runs
	RunCount int `json:"run_count"`

	// Tags are the workflow tags
	Tags map[string]string `json:"tags,omitempty"`
}

// CICDRunMetrics represents metrics for a single CI/CD run
type CICDRunMetrics struct {
	// ID is the unique identifier for the run
	ID string `json:"id"`

	// Status is the status of the run
	Status CICDStatus `json:"status"`

	// StartTime is when the run started
	StartTime time.Time `json:"start_time"`

	// EndTime is when the run ended
	EndTime time.Time `json:"end_time"`

	// Duration is the duration of the run
	Duration time.Duration `json:"duration"`

	// Trigger is what triggered the run
	Trigger string `json:"trigger"`

	// Branch is the branch the run was for
	Branch string `json:"branch"`

	// Commit is the commit hash associated with the run
	Commit string `json:"commit"`

	// Jobs is the list of job metrics for this run
	Jobs []CICDJobMetrics `json:"jobs"`

	// URL is the URL to view the run
	URL string `json:"url"`
}

// CICDJobMetrics represents metrics for a single CI/CD job
type CICDJobMetrics struct {
	// ID is the unique identifier for the job
	ID string `json:"id"`

	// Name is the name of the job
	Name string `json:"name"`

	// Status is the status of the job
	Status CICDStatus `json:"status"`

	// StartTime is when the job started
	StartTime time.Time `json:"start_time"`

	// EndTime is when the job ended
	EndTime time.Time `json:"end_time"`

	// Duration is the duration of the job
	Duration time.Duration `json:"duration"`

	// Steps is the list of steps in the job
	Steps []CICDStepMetrics `json:"steps"`

	// URL is the URL to view the job
	URL string `json:"url"`
}

// CICDStepMetrics represents metrics for a single CI/CD step
type CICDStepMetrics struct {
	// Name is the name of the step
	Name string `json:"name"`

	// Status is the status of the step
	Status CICDStatus `json:"status"`

	// Duration is the duration of the step
	Duration time.Duration `json:"duration"`

	// Output is the output of the step
	Output string `json:"output,omitempty"`
}

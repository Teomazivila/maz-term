package collector

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/config"
	"github.com/Teomazivila/maz-term/pkg/models"
)

// GitHubActionsCollector collects metrics from GitHub Actions
type GitHubActionsCollector struct {
	*BaseCollector
	config  config.GitHubConfig
	metrics models.CICDMetrics
	mu      sync.RWMutex
}

// NewGitHubActionsCollector creates a new GitHub Actions collector
func NewGitHubActionsCollector(cfg config.GitHubConfig) *GitHubActionsCollector {
	return &GitHubActionsCollector{
		BaseCollector: NewBaseCollector("github_actions"),
		config:        cfg,
		metrics: models.CICDMetrics{
			ProviderType: models.CICDProviderGitHubActions,
			ProviderName: "GitHub Actions",
			LastUpdated:  time.Now(),
		},
	}
}

// Name returns the name of the collector
func (c *GitHubActionsCollector) Name() string {
	return "GitHub Actions Collector"
}

// Collect gathers metrics from GitHub Actions
func (c *GitHubActionsCollector) Collect(ctx context.Context) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// For the MVP, we'll simulate GitHub Actions data collection with demo data
	// In a real implementation, we would use the GitHub API to collect actual metrics

	// Reset errors
	c.metrics.Errors = []string{}

	// Update collection timestamp
	c.metrics.LastUpdated = time.Now()

	// Collect workflows
	workflows, err := c.collectWorkflows(ctx)
	if err != nil {
		c.metrics.Errors = append(c.metrics.Errors, fmt.Sprintf("Workflows error: %v", err))
	} else {
		c.metrics.Workflows = workflows
	}

	// Calculate summary metrics
	c.metrics.Summary = c.calculateSummary(workflows)

	return c.metrics, nil
}

// GetLatestMetrics returns the most recently collected metrics
func (c *GitHubActionsCollector) GetLatestMetrics() models.CICDMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// Start starts the collector
func (c *GitHubActionsCollector) Start(ctx context.Context, interval time.Duration) error {
	// Call the parent Start method
	if err := c.BaseCollector.Start(ctx, interval); err != nil {
		return err
	}

	if !c.running {
		return nil
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Collect initial data
		data, err := c.Collect(ctx)
		if err == nil {
			c.UpdateData(data)
		}

		for {
			select {
			case <-ticker.C:
				data, err := c.Collect(ctx)
				if err == nil {
					c.UpdateData(data)
				}
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop stops the collector
func (c *GitHubActionsCollector) Stop() error {
	return c.BaseCollector.Stop()
}

// collectWorkflows collects metrics from GitHub Actions workflows
func (c *GitHubActionsCollector) collectWorkflows(ctx context.Context) ([]models.CICDWorkflowMetrics, error) {
	// In a real implementation, we would use the GitHub API to collect actual metrics
	// For the MVP, we'll return demo data

	workflows := []models.CICDWorkflowMetrics{
		{
			ID:              "ci-workflow",
			Name:            "CI Workflow",
			Repository:      fmt.Sprintf("%s/my-project", c.config.Owner),
			Enabled:         true,
			AverageDuration: 3*time.Minute + 45*time.Second,
			SuccessRate:     92.5,
			LastRunStatus:   models.CICDStatusSuccess,
			LastRunTime:     time.Now().Add(-15 * time.Minute),
			LastSuccessTime: time.Now().Add(-15 * time.Minute),
			RunCount:        156,
			Tags: map[string]string{
				"type": "ci",
			},
			RecentRuns: generateRecentRuns("ci", 10, 92.5),
		},
		{
			ID:              "deploy-workflow",
			Name:            "Deploy to Production",
			Repository:      fmt.Sprintf("%s/my-project", c.config.Owner),
			Enabled:         true,
			AverageDuration: 12*time.Minute + 20*time.Second,
			SuccessRate:     98.0,
			LastRunStatus:   models.CICDStatusSuccess,
			LastRunTime:     time.Now().Add(-2 * time.Hour),
			LastSuccessTime: time.Now().Add(-2 * time.Hour),
			RunCount:        87,
			Tags: map[string]string{
				"type":        "deploy",
				"environment": "production",
			},
			RecentRuns: generateRecentRuns("deploy", 10, 98.0),
		},
		{
			ID:              "nightly-tests",
			Name:            "Nightly Integration Tests",
			Repository:      fmt.Sprintf("%s/my-project", c.config.Owner),
			Enabled:         true,
			AverageDuration: 28*time.Minute + 45*time.Second,
			SuccessRate:     85.0,
			LastRunStatus:   models.CICDStatusFailure,
			LastRunTime:     time.Now().Add(-8 * time.Hour),
			LastSuccessTime: time.Now().Add(-32 * time.Hour),
			RunCount:        65,
			Tags: map[string]string{
				"type":     "test",
				"schedule": "nightly",
			},
			RecentRuns: generateRecentRuns("test", 10, 85.0),
		},
	}

	return workflows, nil
}

// calculateSummary calculates summary metrics for GitHub Actions
func (c *GitHubActionsCollector) calculateSummary(workflows []models.CICDWorkflowMetrics) models.CICDSummaryMetrics {
	summary := models.CICDSummaryMetrics{
		TotalWorkflows:  len(workflows),
		ActiveWorkflows: 0,
		RunningJobs:     0,
		SuccessRate:     0,
		FailedWorkflows: 0,
	}

	if len(workflows) == 0 {
		return summary
	}

	// Calculate active workflows, running jobs, and failed workflows
	var totalSuccessRate float64
	var totalDuration time.Duration

	for _, workflow := range workflows {
		// A workflow is active if it has run in the past week
		if time.Since(workflow.LastRunTime) < 7*24*time.Hour {
			summary.ActiveWorkflows++
		}

		// Count running jobs
		for _, run := range workflow.RecentRuns {
			for _, job := range run.Jobs {
				if job.Status == models.CICDStatusRunning {
					summary.RunningJobs++
				}
			}
		}

		// Count failed workflows
		if workflow.LastRunStatus == models.CICDStatusFailure {
			summary.FailedWorkflows++
		}

		totalSuccessRate += workflow.SuccessRate
		totalDuration += workflow.AverageDuration
	}

	// Calculate overall success rate and average duration
	summary.SuccessRate = totalSuccessRate / float64(len(workflows))
	summary.AverageDuration = totalDuration / time.Duration(len(workflows))

	return summary
}

// generateRecentRuns generates demo data for recent workflow runs
func generateRecentRuns(prefix string, count int, successRate float64) []models.CICDRunMetrics {
	runs := make([]models.CICDRunMetrics, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		// Generate start time for the run (more recent for lower indices)
		startTime := now.Add(-time.Duration(i*6) * time.Hour)

		// Determine run status based on success rate
		status := models.CICDStatusSuccess
		if rand.Float64()*100 > successRate {
			status = models.CICDStatusFailure
		}

		// For the first run, occasionally make it running
		if i == 0 && rand.Intn(10) < 2 {
			status = models.CICDStatusRunning
		}

		// Generate a realistic duration
		var duration time.Duration
		var endTime time.Time

		if status == models.CICDStatusRunning {
			duration = time.Duration(rand.Intn(600)) * time.Second // 0-10 minutes so far
			endTime = time.Time{}                                  // No end time for running jobs
		} else {
			duration = time.Duration(3+rand.Intn(25)) * time.Minute
			endTime = startTime.Add(duration)
		}

		// Generate jobs for this run
		jobCount := 2 + rand.Intn(3) // 2-4 jobs
		jobs := make([]models.CICDJobMetrics, jobCount)

		for j := 0; j < jobCount; j++ {
			jobStartTime := startTime.Add(time.Duration(j) * time.Minute)
			jobDuration := time.Duration(1+rand.Intn(10)) * time.Minute
			jobEndTime := jobStartTime.Add(jobDuration)

			jobStatus := status
			if status == models.CICDStatusRunning && j == jobCount-1 {
				// Last job is running
				jobEndTime = time.Time{}
			} else if status == models.CICDStatusFailure && j == jobCount-1 {
				// Last job failed
				jobStatus = models.CICDStatusFailure
			} else {
				jobStatus = models.CICDStatusSuccess
			}

			// Generate steps for this job
			stepCount := 3 + rand.Intn(3) // 3-5 steps
			steps := make([]models.CICDStepMetrics, stepCount)

			for s := 0; s < stepCount; s++ {
				stepDuration := time.Duration(30+rand.Intn(300)) * time.Second

				stepStatus := models.CICDStatusSuccess
				if jobStatus == models.CICDStatusRunning && s == stepCount-1 {
					stepStatus = models.CICDStatusRunning
				} else if jobStatus == models.CICDStatusFailure && s == stepCount-1 {
					stepStatus = models.CICDStatusFailure
				}

				steps[s] = models.CICDStepMetrics{
					Name:     fmt.Sprintf("Step %d", s+1),
					Status:   stepStatus,
					Duration: stepDuration,
				}
			}

			jobs[j] = models.CICDJobMetrics{
				ID:        fmt.Sprintf("%s-job-%d-%d", prefix, i, j),
				Name:      fmt.Sprintf("%s-job-%d", prefix, j+1),
				Status:    jobStatus,
				StartTime: jobStartTime,
				EndTime:   jobEndTime,
				Duration:  jobDuration,
				Steps:     steps,
				URL:       fmt.Sprintf("https://github.com/actions/runs/%d", rand.Intn(1000000)),
			}
		}

		runs[i] = models.CICDRunMetrics{
			ID:        fmt.Sprintf("%s-run-%d", prefix, i),
			Status:    status,
			StartTime: startTime,
			EndTime:   endTime,
			Duration:  duration,
			Trigger:   randTrigger(),
			Branch:    randBranch(),
			Commit:    fmt.Sprintf("%x", rand.Uint64()),
			Jobs:      jobs,
			URL:       fmt.Sprintf("https://github.com/actions/runs/%d", rand.Intn(1000000)),
		}
	}

	return runs
}

// randTrigger returns a random trigger type
func randTrigger() string {
	triggers := []string{"push", "pull_request", "workflow_dispatch", "schedule", "repository_dispatch"}
	return triggers[rand.Intn(len(triggers))]
}

// randBranch returns a random branch name
func randBranch() string {
	branches := []string{"main", "develop", "feature/user-auth", "feature/api-v2", "bugfix/issue-123"}
	return branches[rand.Intn(len(branches))]
}

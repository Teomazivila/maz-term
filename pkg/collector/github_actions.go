package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

const (
	// githubAPIBase is the REST API root.
	githubAPIBase = "https://api.github.com"

	// githubCollectTimeout bounds one full sweep across every repository.
	githubCollectTimeout = 45 * time.Second

	// githubRunsPerWorkflow is how many recent runs are examined per workflow.
	// Success rate and mean duration are computed over this window.
	githubRunsPerWorkflow = 20

	// githubMaxResponseBytes caps a response body so a malformed or hostile
	// response cannot exhaust memory.
	githubMaxResponseBytes = 8 << 20
)

// GitHubConfig configures the GitHub Actions collector.
type GitHubConfig struct {
	Owner        string
	Repositories []string

	// Workflows optionally restricts collection to these workflow file names,
	// for example "ci.yml". Empty means every workflow in the repository.
	Workflows []string

	// Token authenticates the API calls. It is read from the environment by the
	// caller and never from a configuration file.
	Token string

	// BaseURL overrides the API root, for GitHub Enterprise and for tests.
	BaseURL string
}

// wants reports whether a workflow file should be collected.
func (c GitHubConfig) wants(path string) bool {
	if len(c.Workflows) == 0 {
		return true
	}

	name := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		name = path[idx+1:]
	}

	for _, want := range c.Workflows {
		if strings.EqualFold(strings.TrimSpace(want), name) {
			return true
		}
	}
	return false
}

// GitHubActionsCollector collects workflow run history from the GitHub REST API.
//
// It uses net/http directly: two read-only endpoints do not justify a dependency.
// See docs/adr/0001-infrastructure-integrations.md.
type GitHubActionsCollector struct {
	*BaseCollector

	config GitHubConfig
	client *http.Client

	mu        sync.RWMutex
	metrics   models.CICDMetrics
	rateLimit githubRateLimit
}

// githubRateLimit is the quota state from the most recent response.
type githubRateLimit struct {
	Remaining int
	Limit     int
	Reset     time.Time
}

// NewGitHubActionsCollector creates a GitHub Actions collector.
func NewGitHubActionsCollector(cfg GitHubConfig) *GitHubActionsCollector {
	if cfg.BaseURL == "" {
		cfg.BaseURL = githubAPIBase
	}

	return &GitHubActionsCollector{
		BaseCollector: NewBaseCollector("github_actions"),
		config:        cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     60 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		metrics: models.CICDMetrics{
			ProviderType: models.CICDProviderGitHubActions,
			ProviderName: "GitHub Actions",
		},
	}
}

// RateLimit reports the quota state from the most recent API response.
func (c *GitHubActionsCollector) RateLimit() (remaining, limit int, reset time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rateLimit.Remaining, c.rateLimit.Limit, c.rateLimit.Reset
}

// Collect gathers workflows and their recent runs.
func (c *GitHubActionsCollector) Collect(ctx context.Context) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, githubCollectTimeout)
	defer cancel()

	// A configuration problem is still published, so the history records the
	// outage rather than leaving a gap indistinguishable from a collector that
	// was never running. The other providers behave the same way.
	if reason := c.configProblem(); reason != "" {
		result := models.CICDMetrics{
			ProviderType: models.CICDProviderGitHubActions,
			ProviderName: "GitHub Actions",
			LastUpdated:  time.Now(),
			Errors:       []string{reason},
		}
		c.publish(result)
		return result, errors.New(reason)
	}

	result := models.CICDMetrics{
		ProviderType: models.CICDProviderGitHubActions,
		ProviderName: "GitHub Actions",
		LastUpdated:  time.Now(),
	}

	for _, repo := range c.config.Repositories {
		workflows, err := c.collectRepository(ctx, repo)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", c.config.Owner, repo, err))
			continue
		}
		result.Workflows = append(result.Workflows, workflows...)
	}

	result.Summary = summariseWorkflows(result.Workflows)
	c.publish(result)

	if len(result.Errors) > 0 && len(result.Workflows) == 0 {
		return result, fmt.Errorf("github: %s", strings.Join(result.Errors, "; "))
	}

	return result, nil
}

// configProblem returns why collection cannot be attempted, or "" when it can.
func (c *GitHubActionsCollector) configProblem() string {
	switch {
	case c.config.Owner == "":
		return "github: cicd.github.owner is required"
	case len(c.config.Repositories) == 0:
		return "github: cicd.github.repositories is empty"
	case c.config.Token == "":
		return "github: no API token (set MAZTERM_GITHUB_TOKEN or GITHUB_TOKEN)"
	default:
		return ""
	}
}

// publish records the sample and persists its summary.
func (c *GitHubActionsCollector) publish(result models.CICDMetrics) {
	c.mu.Lock()
	c.metrics = result
	c.mu.Unlock()

	c.UpdateData(result)

	store := c.Storage()
	if store == nil {
		return
	}
	if err := store.StoreCICDSummary(models.CICDSummaryFrom(result)); err != nil {
		c.Logger().Error("failed to store CI/CD summary", "error", err)
	}
}

// collectRepository gathers one repository's workflows and their recent runs.
func (c *GitHubActionsCollector) collectRepository(ctx context.Context, repo string) ([]models.CICDWorkflowMetrics, error) {
	var listing struct {
		TotalCount int `json:"total_count"`
		Workflows  []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Path  string `json:"path"`
			State string `json:"state"`
		} `json:"workflows"`
	}

	path := fmt.Sprintf("/repos/%s/%s/actions/workflows",
		url.PathEscape(c.config.Owner), url.PathEscape(repo))
	if err := c.get(ctx, path, url.Values{"per_page": {"100"}}, &listing); err != nil {
		return nil, err
	}

	var workflows []models.CICDWorkflowMetrics
	for _, workflow := range listing.Workflows {
		if !c.config.wants(workflow.Path) {
			continue
		}

		entry := models.CICDWorkflowMetrics{
			ID:         strconv.FormatInt(workflow.ID, 10),
			Name:       workflow.Name,
			Repository: fmt.Sprintf("%s/%s", c.config.Owner, repo),
			Enabled:    workflow.State == "active",
		}

		runs, err := c.collectRuns(ctx, repo, workflow.ID)
		if err != nil {
			// One workflow's runs failing must not lose the rest of the
			// repository; the workflow is still reported, without run history.
			c.Logger().Warn("failed to list workflow runs",
				"repo", repo, "workflow", workflow.Name, "error", err)
			workflows = append(workflows, entry)
			continue
		}

		entry.RecentRuns = runs
		entry.RunCount = len(runs)
		applyRunAggregates(&entry, runs)

		workflows = append(workflows, entry)
	}

	return workflows, nil
}

// collectRuns lists the recent runs of one workflow.
func (c *GitHubActionsCollector) collectRuns(ctx context.Context, repo string, workflowID int64) ([]models.CICDRunMetrics, error) {
	var listing struct {
		WorkflowRuns []struct {
			ID         int64     `json:"id"`
			Status     string    `json:"status"`
			Conclusion string    `json:"conclusion"`
			Event      string    `json:"event"`
			HeadBranch string    `json:"head_branch"`
			HeadSHA    string    `json:"head_sha"`
			HTMLURL    string    `json:"html_url"`
			RunStarted time.Time `json:"run_started_at"`
			CreatedAt  time.Time `json:"created_at"`
			UpdatedAt  time.Time `json:"updated_at"`
		} `json:"workflow_runs"`
	}

	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%d/runs",
		url.PathEscape(c.config.Owner), url.PathEscape(repo), workflowID)
	query := url.Values{"per_page": {strconv.Itoa(githubRunsPerWorkflow)}}

	if err := c.get(ctx, path, query, &listing); err != nil {
		return nil, err
	}

	runs := make([]models.CICDRunMetrics, 0, len(listing.WorkflowRuns))
	for _, run := range listing.WorkflowRuns {
		start := run.RunStarted
		if start.IsZero() {
			start = run.CreatedAt
		}

		entry := models.CICDRunMetrics{
			ID:        strconv.FormatInt(run.ID, 10),
			Status:    githubStatus(run.Status, run.Conclusion),
			StartTime: start,
			Trigger:   run.Event,
			Branch:    run.HeadBranch,
			Commit:    run.HeadSHA,
			URL:       run.HTMLURL,
		}

		// A run still in progress has no meaningful end time or duration.
		if entry.Status != models.CICDStatusRunning && entry.Status != models.CICDStatusPending {
			entry.EndTime = run.UpdatedAt
			if !start.IsZero() && run.UpdatedAt.After(start) {
				entry.Duration = run.UpdatedAt.Sub(start)
			}
		}

		runs = append(runs, entry)
	}

	return runs, nil
}

// get issues an authenticated GET and decodes the JSON response.
func (c *GitHubActionsCollector) get(ctx context.Context, path string, query url.Values, out any) error {
	endpoint := strings.TrimSuffix(c.config.BaseURL, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "maz-term")
	req.Header.Set("Authorization", "Bearer "+c.config.Token)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.recordRateLimit(resp.Header)

	if resp.StatusCode != http.StatusOK {
		// The body may carry a useful message, but it is attacker-influenced, so
		// only a bounded, single-line excerpt is surfaced.
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		message := strings.TrimSpace(strings.ReplaceAll(string(excerpt), "\n", " "))

		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return errors.New("unauthorised: the API token is invalid or expired")
		case http.StatusForbidden:
			if remaining, _, reset := c.RateLimit(); remaining == 0 && !reset.IsZero() {
				return fmt.Errorf("rate limit exhausted, resets at %s", reset.Format(time.Kitchen))
			}
			return fmt.Errorf("forbidden: the token lacks the actions:read scope (%s)", message)
		case http.StatusNotFound:
			return errors.New("not found: check the owner and repository names")
		default:
			return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, message)
		}
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, githubMaxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

// recordRateLimit stores the quota headers so the UI can show remaining budget.
func (c *GitHubActionsCollector) recordRateLimit(header http.Header) {
	limit := githubRateLimit{Remaining: -1, Limit: -1}

	if v, err := strconv.Atoi(header.Get("X-RateLimit-Remaining")); err == nil {
		limit.Remaining = v
	}
	if v, err := strconv.Atoi(header.Get("X-RateLimit-Limit")); err == nil {
		limit.Limit = v
	}
	if v, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64); err == nil && v > 0 {
		limit.Reset = time.Unix(v, 0)
	}

	c.mu.Lock()
	c.rateLimit = limit
	c.mu.Unlock()
}

// githubStatus maps a run's status and conclusion to the shared vocabulary.
//
// GitHub reports these in two fields: an in-flight run has a status and no
// conclusion, a finished one has status "completed" and the outcome in
// conclusion.
func githubStatus(status, conclusion string) models.CICDStatus {
	if status != "completed" {
		switch status {
		case "queued", "waiting", "pending", "requested":
			return models.CICDStatusPending
		default:
			return models.CICDStatusRunning
		}
	}

	switch conclusion {
	case "success":
		return models.CICDStatusSuccess
	case "failure", "timed_out", "startup_failure":
		return models.CICDStatusFailure
	case "cancelled":
		return models.CICDStatusCancelled
	case "skipped", "neutral":
		return models.CICDStatusSkipped
	default:
		return models.CICDStatusFailure
	}
}

// applyRunAggregates computes a workflow's success rate and timings.
func applyRunAggregates(workflow *models.CICDWorkflowMetrics, runs []models.CICDRunMetrics) {
	if len(runs) == 0 {
		return
	}

	// Runs arrive newest-first.
	workflow.LastRunStatus = runs[0].Status
	workflow.LastRunTime = runs[0].StartTime

	var (
		succeeded     int
		completed     int
		durationTotal time.Duration
		durationCount int
	)

	for _, run := range runs {
		switch run.Status {
		case models.CICDStatusRunning, models.CICDStatusPending:
			continue
		case models.CICDStatusSuccess:
			succeeded++
			if workflow.LastSuccessTime.IsZero() {
				workflow.LastSuccessTime = run.StartTime
			}
		}

		completed++
		if run.Duration > 0 {
			durationTotal += run.Duration
			durationCount++
		}
	}

	// Skipped and cancelled runs count as completed but not successful, which
	// keeps the rate honest rather than flattering.
	if completed > 0 {
		workflow.SuccessRate = float64(succeeded) / float64(completed) * 100
	}
	if durationCount > 0 {
		workflow.AverageDuration = durationTotal / time.Duration(durationCount)
	}
}

// summariseWorkflows aggregates across every workflow.
func summariseWorkflows(workflows []models.CICDWorkflowMetrics) models.CICDSummaryMetrics {
	summary := models.CICDSummaryMetrics{TotalWorkflows: len(workflows)}

	var (
		rateTotal     float64
		rateCount     int
		durationTotal time.Duration
		durationCount int
	)

	for _, workflow := range workflows {
		if workflow.RunCount > 0 {
			summary.ActiveWorkflows++
		}
		if workflow.LastRunStatus == models.CICDStatusFailure {
			summary.FailedWorkflows++
		}
		for _, run := range workflow.RecentRuns {
			if run.Status == models.CICDStatusRunning || run.Status == models.CICDStatusPending {
				summary.RunningJobs++
			}
		}
		if workflow.RunCount > 0 {
			rateTotal += workflow.SuccessRate
			rateCount++
		}
		if workflow.AverageDuration > 0 {
			durationTotal += workflow.AverageDuration
			durationCount++
		}
	}

	if rateCount > 0 {
		summary.SuccessRate = rateTotal / float64(rateCount)
	}
	if durationCount > 0 {
		summary.AverageDuration = durationTotal / time.Duration(durationCount)
	}

	return summary
}

// GetLatestMetrics returns the most recent sample.
func (c *GitHubActionsCollector) GetLatestMetrics() models.CICDMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// Start begins periodic collection.
func (c *GitHubActionsCollector) Start(ctx context.Context, interval time.Duration) error {
	return c.start(ctx, interval, func(ctx context.Context) {
		if _, err := c.Collect(ctx); err != nil && ctx.Err() == nil {
			c.Logger().Warn("github actions collection failed", "error", err)
		}
	})
}

package collector

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// githubServer serves canned workflow and run responses.
func githubServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// newGitHub builds a collector pointed at a test server.
func newGitHub(t *testing.T, server *httptest.Server, cfg GitHubConfig) *GitHubActionsCollector {
	t.Helper()

	cfg.BaseURL = server.URL
	if cfg.Token == "" {
		cfg.Token = "test-token"
	}
	if cfg.Owner == "" {
		cfg.Owner = "acme"
	}
	if len(cfg.Repositories) == 0 {
		cfg.Repositories = []string{"widget"}
	}
	return NewGitHubActionsCollector(cfg)
}

// standardHandler serves one workflow with a mix of run outcomes.
func standardHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4987")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))

		switch {
		case strings.HasSuffix(r.URL.Path, "/actions/workflows"):
			fmt.Fprint(w, `{"total_count":1,"workflows":[
				{"id":42,"name":"CI","path":".github/workflows/ci.yml","state":"active"}
			]}`)

		case strings.Contains(r.URL.Path, "/actions/workflows/42/runs"):
			now := time.Now().UTC()
			fmt.Fprintf(w, `{"workflow_runs":[
				{"id":9,"status":"in_progress","conclusion":null,"event":"push",
				 "head_branch":"main","head_sha":"aaa","html_url":"https://x/9",
				 "run_started_at":%q,"created_at":%q,"updated_at":%q},
				{"id":8,"status":"completed","conclusion":"success","event":"push",
				 "head_branch":"main","head_sha":"bbb","html_url":"https://x/8",
				 "run_started_at":%q,"created_at":%q,"updated_at":%q},
				{"id":7,"status":"completed","conclusion":"failure","event":"pull_request",
				 "head_branch":"feature","head_sha":"ccc","html_url":"https://x/7",
				 "run_started_at":%q,"created_at":%q,"updated_at":%q}
			]}`,
				now.Add(-time.Minute).Format(time.RFC3339), now.Add(-time.Minute).Format(time.RFC3339), now.Format(time.RFC3339),
				now.Add(-10*time.Minute).Format(time.RFC3339), now.Add(-10*time.Minute).Format(time.RFC3339), now.Add(-8*time.Minute).Format(time.RFC3339),
				now.Add(-30*time.Minute).Format(time.RFC3339), now.Add(-30*time.Minute).Format(time.RFC3339), now.Add(-26*time.Minute).Format(time.RFC3339),
			)

		default:
			http.NotFound(w, r)
		}
	}
}

func TestGitHubCollectMapsWorkflowsAndRuns(t *testing.T) {
	server := githubServer(t, standardHandler(t))

	store := &recordingStore{}
	c := newGitHub(t, server, GitHubConfig{})
	c.SetStorageProvider(store)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metrics := c.GetLatestMetrics()
	assert.Empty(t, metrics.Errors)
	require.Len(t, metrics.Workflows, 1)

	workflow := metrics.Workflows[0]
	assert.Equal(t, "CI", workflow.Name)
	assert.Equal(t, "acme/widget", workflow.Repository)
	assert.True(t, workflow.Enabled)
	require.Len(t, workflow.RecentRuns, 3)

	// Runs arrive newest-first, so the in-progress run is the last status.
	assert.Equal(t, models.CICDStatusRunning, workflow.LastRunStatus)

	assert.Equal(t, models.CICDStatusRunning, workflow.RecentRuns[0].Status)
	assert.Zero(t, workflow.RecentRuns[0].Duration, "an in-flight run has no duration")

	assert.Equal(t, models.CICDStatusSuccess, workflow.RecentRuns[1].Status)
	assert.Equal(t, 2*time.Minute, workflow.RecentRuns[1].Duration.Round(time.Minute))
	assert.Equal(t, "main", workflow.RecentRuns[1].Branch)
	assert.Equal(t, "push", workflow.RecentRuns[1].Trigger)

	assert.Equal(t, models.CICDStatusFailure, workflow.RecentRuns[2].Status)
	assert.Equal(t, "pull_request", workflow.RecentRuns[2].Trigger)

	// One success out of two completed runs; the in-flight run is excluded.
	assert.InDelta(t, 50.0, workflow.SuccessRate, 0.001)

	assert.Equal(t, 1, metrics.Summary.RunningJobs)
	assert.Equal(t, 1, metrics.Summary.TotalWorkflows)

	_, _, cicd := store.infraCounts()
	assert.Equal(t, 1, cicd)
}

func TestGitHubSendsAuthAndVersionHeaders(t *testing.T) {
	var gotAuth, gotVersion, gotAccept atomic.Value

	server := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		gotVersion.Store(r.Header.Get("X-GitHub-Api-Version"))
		gotAccept.Store(r.Header.Get("Accept"))
		fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
	})

	c := newGitHub(t, server, GitHubConfig{Token: "secret-token"})
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "Bearer secret-token", gotAuth.Load())
	assert.Equal(t, "2022-11-28", gotVersion.Load())
	assert.Equal(t, "application/vnd.github+json", gotAccept.Load())
}

func TestGitHubRecordsRateLimit(t *testing.T) {
	server := githubServer(t, standardHandler(t))

	c := newGitHub(t, server, GitHubConfig{})
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	remaining, limit, reset := c.RateLimit()
	assert.Equal(t, 4987, remaining)
	assert.Equal(t, 5000, limit)
	assert.False(t, reset.IsZero())
}

// TestGitHubAuthFailuresAreActionable pins the error messages: an operator needs
// to know which of token, scope or name is wrong.
func TestGitHubAuthFailuresAreActionable(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantMsg string
	}{
		{name: "unauthorised", status: http.StatusUnauthorized, wantMsg: "token is invalid"},
		{name: "forbidden", status: http.StatusForbidden, wantMsg: "actions:read"},
		{name: "not found", status: http.StatusNotFound, wantMsg: "owner and repository"},
		{name: "server error", status: http.StatusInternalServerError, wantMsg: "unexpected status 500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := githubServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, `{"message":"nope"}`)
			})

			c := newGitHub(t, server, GitHubConfig{})

			_, err := c.Collect(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.NotEmpty(t, c.GetLatestMetrics().Errors)
		})
	}
}

// TestGitHubRateLimitExhaustionIsNamed distinguishes a spent quota from a missing
// scope, which are both 403.
func TestGitHubRateLimitExhaustionIsNamed(t *testing.T) {
	reset := time.Now().Add(30 * time.Minute)

	server := githubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	})

	c := newGitHub(t, server, GitHubConfig{})

	_, err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exhausted")
}

func TestGitHubRequiresConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		cfg     GitHubConfig
		wantMsg string
	}{
		{name: "no owner", cfg: GitHubConfig{Repositories: []string{"r"}, Token: "t"}, wantMsg: "owner"},
		{name: "no repositories", cfg: GitHubConfig{Owner: "o", Token: "t"}, wantMsg: "repositories"},
		{name: "no token", cfg: GitHubConfig{Owner: "o", Repositories: []string{"r"}}, wantMsg: "GITHUB_TOKEN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewGitHubActionsCollector(tt.cfg)

			_, err := c.Collect(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// TestGitHubWorkflowFilter checks the workflow allowlist.
func TestGitHubWorkflowFilter(t *testing.T) {
	server := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/actions/workflows") {
			fmt.Fprint(w, `{"total_count":2,"workflows":[
				{"id":1,"name":"CI","path":".github/workflows/ci.yml","state":"active"},
				{"id":2,"name":"Release","path":".github/workflows/release.yml","state":"active"}
			]}`)
			return
		}
		fmt.Fprint(w, `{"workflow_runs":[]}`)
	})

	c := newGitHub(t, server, GitHubConfig{Workflows: []string{"ci.yml"}})
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	workflows := c.GetLatestMetrics().Workflows
	require.Len(t, workflows, 1)
	assert.Equal(t, "CI", workflows[0].Name)
}

// TestGitHubRunListingFailureKeepsWorkflow ensures one failing run listing does
// not lose the rest of the repository.
func TestGitHubRunListingFailureKeepsWorkflow(t *testing.T) {
	server := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/actions/workflows") {
			fmt.Fprint(w, `{"total_count":1,"workflows":[
				{"id":1,"name":"CI","path":".github/workflows/ci.yml","state":"active"}
			]}`)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := newGitHub(t, server, GitHubConfig{})
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	workflows := c.GetLatestMetrics().Workflows
	require.Len(t, workflows, 1, "the workflow is still reported without run history")
	assert.Empty(t, workflows[0].RecentRuns)
	assert.Zero(t, workflows[0].RunCount)
}

func TestGitHubMultipleRepositoriesPartialFailure(t *testing.T) {
	server := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/broken/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/actions/workflows") {
			fmt.Fprint(w, `{"total_count":1,"workflows":[
				{"id":1,"name":"CI","path":".github/workflows/ci.yml","state":"active"}
			]}`)
			return
		}
		fmt.Fprint(w, `{"workflow_runs":[]}`)
	})

	c := newGitHub(t, server, GitHubConfig{Repositories: []string{"good", "broken"}})

	_, err := c.Collect(context.Background())
	require.NoError(t, err, "one bad repository is partial data, not a failed sweep")

	metrics := c.GetLatestMetrics()
	assert.Len(t, metrics.Workflows, 1)
	require.Len(t, metrics.Errors, 1)
	assert.Contains(t, metrics.Errors[0], "broken")
}

func TestGitHubStatusMapping(t *testing.T) {
	tests := []struct {
		status     string
		conclusion string
		want       models.CICDStatus
	}{
		{status: "completed", conclusion: "success", want: models.CICDStatusSuccess},
		{status: "completed", conclusion: "failure", want: models.CICDStatusFailure},
		{status: "completed", conclusion: "timed_out", want: models.CICDStatusFailure},
		{status: "completed", conclusion: "startup_failure", want: models.CICDStatusFailure},
		{status: "completed", conclusion: "cancelled", want: models.CICDStatusCancelled},
		{status: "completed", conclusion: "skipped", want: models.CICDStatusSkipped},
		{status: "completed", conclusion: "neutral", want: models.CICDStatusSkipped},
		{status: "in_progress", conclusion: "", want: models.CICDStatusRunning},
		{status: "queued", conclusion: "", want: models.CICDStatusPending},
		{status: "waiting", conclusion: "", want: models.CICDStatusPending},
		{status: "requested", conclusion: "", want: models.CICDStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.status+"/"+tt.conclusion, func(t *testing.T) {
			assert.Equal(t, tt.want, githubStatus(tt.status, tt.conclusion))
		})
	}
}

// TestSuccessRateExcludesInFlightRuns keeps the figure honest: counting a running
// run as a failure would understate it, counting it as a success would flatter it.
func TestSuccessRateExcludesInFlightRuns(t *testing.T) {
	workflow := models.CICDWorkflowMetrics{}
	applyRunAggregates(&workflow, []models.CICDRunMetrics{
		{Status: models.CICDStatusRunning},
		{Status: models.CICDStatusSuccess, Duration: 2 * time.Minute},
		{Status: models.CICDStatusSuccess, Duration: 4 * time.Minute},
		{Status: models.CICDStatusFailure, Duration: time.Minute},
	})

	assert.InDelta(t, 66.67, workflow.SuccessRate, 0.1, "2 of 3 completed runs succeeded")
	// (2m + 4m + 1m) / 3. A failed run still consumed CI time, so it counts
	// towards the mean duration even though it does not count as a success.
	assert.Equal(t, 140*time.Second, workflow.AverageDuration, "mean over completed runs only")
	assert.Equal(t, models.CICDStatusRunning, workflow.LastRunStatus)
}

func TestSuccessRateWithNoCompletedRuns(t *testing.T) {
	workflow := models.CICDWorkflowMetrics{}
	applyRunAggregates(&workflow, []models.CICDRunMetrics{{Status: models.CICDStatusRunning}})

	assert.Zero(t, workflow.SuccessRate)
	assert.Zero(t, workflow.AverageDuration)
}

func TestSummariseWorkflows(t *testing.T) {
	summary := summariseWorkflows([]models.CICDWorkflowMetrics{
		{
			RunCount: 3, SuccessRate: 100, AverageDuration: time.Minute,
			LastRunStatus: models.CICDStatusSuccess,
			RecentRuns:    []models.CICDRunMetrics{{Status: models.CICDStatusRunning}},
		},
		{
			RunCount: 2, SuccessRate: 50, AverageDuration: 3 * time.Minute,
			LastRunStatus: models.CICDStatusFailure,
		},
		{RunCount: 0},
	})

	assert.Equal(t, 3, summary.TotalWorkflows)
	assert.Equal(t, 2, summary.ActiveWorkflows, "a workflow with no runs is not active")
	assert.Equal(t, 1, summary.FailedWorkflows)
	assert.Equal(t, 1, summary.RunningJobs)
	assert.InDelta(t, 75.0, summary.SuccessRate, 0.001)
	assert.Equal(t, 2*time.Minute, summary.AverageDuration)
}

func TestGitHubRespectsCancellation(t *testing.T) {
	server := githubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprint(w, `{"total_count":0,"workflows":[]}`)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := newGitHub(t, server, GitHubConfig{})
	_, err := c.Collect(ctx)
	require.Error(t, err)
}

func TestGitHubConfigWorkflowMatching(t *testing.T) {
	cfg := GitHubConfig{Workflows: []string{"ci.yml", " Release.YML "}}

	assert.True(t, cfg.wants(".github/workflows/ci.yml"))
	assert.True(t, cfg.wants(".github/workflows/release.yml"), "matching is case-insensitive")
	assert.False(t, cfg.wants(".github/workflows/other.yml"))

	assert.True(t, GitHubConfig{}.wants("anything.yml"), "an empty filter accepts everything")
}

// TestGitHubConfigErrorIsStillRecorded pins consistency with the AWS and
// Kubernetes collectors: an enabled provider that cannot be contacted records the
// outage, so the history shows a failure rather than a gap indistinguishable from
// a collector that was never running.
func TestGitHubConfigErrorIsStillRecorded(t *testing.T) {
	store := &recordingStore{}
	c := NewGitHubActionsCollector(GitHubConfig{Owner: "acme", Repositories: []string{"widget"}})
	c.SetStorageProvider(store)

	_, err := c.Collect(context.Background())
	require.Error(t, err)

	_, _, cicd := store.infraCounts()
	assert.Equal(t, 1, cicd, "the outage must be recorded")

	metrics := c.GetLatestMetrics()
	require.Len(t, metrics.Errors, 1)
	assert.Contains(t, metrics.Errors[0], "GITHUB_TOKEN")
}

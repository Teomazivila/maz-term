package collector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

const (
	// gitCommandTimeout bounds each git invocation so a repository on an
	// unresponsive filesystem cannot stall a collection cycle.
	gitCommandTimeout = 10 * time.Second

	// commitHistoryLimit is how many recent commits are shown.
	commitHistoryLimit = 20

	// changedFileLimit caps the changed-file list; a repository mid-rebase can
	// report thousands and the table only shows a screenful.
	changedFileLimit = 200
)

// GitStatusCollector collects the status of a Git repository.
type GitStatusCollector struct {
	*BaseCollector

	repoPath string

	mu      sync.RWMutex
	metrics models.GitRepoMetrics

	subscribers *broadcaster[models.GitRepoMetrics]
}

// NewGitStatusCollector creates a collector for the repository at repoPath. An
// empty path means the current working directory. A leading ~ is expanded,
// which Go does not do and the shell cannot do for a value read from YAML.
func NewGitStatusCollector(repoPath string) *GitStatusCollector {
	resolved, err := ExpandPath(repoPath)
	if err != nil || resolved == "" {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			resolved = cwd
		} else {
			resolved = "."
		}
	}

	return &GitStatusCollector{
		BaseCollector: NewBaseCollector("git_status"),
		repoPath:      resolved,
		metrics: models.GitRepoMetrics{
			Name: filepath.Base(resolved),
			Path: resolved,
		},
		subscribers: newBroadcaster[models.GitRepoMetrics](),
	}
}

// ExpandPath resolves a leading ~ to the user's home directory and returns an
// absolute path.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding %q: %w", path, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", path, err)
	}
	return abs, nil
}

// Collect gathers the repository status.
func (c *GitStatusCollector) Collect(ctx context.Context) (any, error) {
	metrics := c.collectRepoData(ctx)

	c.mu.Lock()
	c.metrics = metrics
	c.mu.Unlock()

	c.UpdateData(metrics)
	c.subscribers.publish(metrics)
	c.store(metrics)

	if metrics.Error != "" {
		return metrics, errors.New(metrics.Error)
	}
	return metrics, nil
}

// store persists the sample, logging rather than discarding failures. Samples
// from a path that is not a repository are not recorded: writing zeroes would
// make a misconfigured path indistinguishable from a clean repository.
func (c *GitStatusCollector) store(metrics models.GitRepoMetrics) {
	store := c.Storage()
	if store == nil || !metrics.IsRepository {
		return
	}
	if err := store.StoreGitMetrics(metrics); err != nil {
		c.Logger().Error("failed to store git metrics", "repo", metrics.Name, "error", err)
	}
}

// collectRepoData runs the git queries. Failures are recorded on the returned
// value rather than dropped, so the UI can show what went wrong.
func (c *GitStatusCollector) collectRepoData(ctx context.Context) models.GitRepoMetrics {
	metrics := models.GitRepoMetrics{
		Name: filepath.Base(c.repoPath),
		Path: c.repoPath,
	}

	if _, err := os.Stat(c.repoPath); err != nil {
		metrics.Error = fmt.Sprintf("repository path unavailable: %v", err)
		return metrics
	}

	// rev-parse handles work trees, submodules and subdirectories correctly,
	// unlike checking for a .git entry.
	inside, err := c.git(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		metrics.Error = fmt.Sprintf("not a git repository: %v", err)
		return metrics
	}
	if strings.TrimSpace(inside) != "true" {
		metrics.Error = "path is not inside a git work tree"
		return metrics
	}
	metrics.IsRepository = true

	// Collect the remaining fields, accumulating problems instead of aborting:
	// a repository with no upstream or no commits is still worth displaying.
	var problems []string
	note := func(what string, err error) {
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", what, err))
		}
	}

	if root, err := c.git(ctx, "rev-parse", "--show-toplevel"); err == nil {
		if trimmed := strings.TrimSpace(root); trimmed != "" {
			metrics.Path = trimmed
			metrics.Name = filepath.Base(trimmed)
		}
	}

	branch, err := c.git(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	note("current branch", err)
	metrics.Branch = strings.TrimSpace(branch)

	branches, err := c.git(ctx, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	note("branch list", err)
	for _, line := range strings.Split(branches, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			metrics.Branches = append(metrics.Branches, name)
		}
	}

	// An empty repository has no HEAD, so rev-list fails; that is a zero commit
	// count rather than an error worth showing.
	if count, err := c.git(ctx, "rev-list", "--count", "HEAD"); err == nil {
		if parsed, convErr := strconv.Atoi(strings.TrimSpace(count)); convErr == nil {
			metrics.CommitCount = parsed
		}
	}

	if ts, err := c.git(ctx, "log", "-1", "--format=%at"); err == nil {
		if unix, convErr := strconv.ParseInt(strings.TrimSpace(ts), 10, 64); convErr == nil {
			metrics.LastCommit = time.Unix(unix, 0)
		}
	}

	// No configured upstream is normal, so a failure here means zero pending
	// commits rather than a reported problem.
	if pending, err := c.git(ctx, "rev-list", "--count", "@{u}..HEAD"); err == nil {
		if parsed, convErr := strconv.Atoi(strings.TrimSpace(pending)); convErr == nil {
			metrics.PendingCommits = parsed
		}
	}

	status, err := c.git(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	note("working tree status", err)
	if err == nil {
		metrics.ChangedFiles, metrics.ModifiedFiles, metrics.UntrackedFiles = parsePorcelain(status)
	}

	history, err := c.git(ctx, "log", fmt.Sprintf("-%d", commitHistoryLimit), "--format=%H%x1f%an%x1f%at%x1f%s")
	if err == nil {
		metrics.CommitHistory = parseCommitHistory(history)
	}

	if len(problems) > 0 {
		metrics.Error = strings.Join(problems, "; ")
	}

	return metrics
}

// parsePorcelain parses NUL-separated `git status --porcelain=v1 -z` output.
// Using -z avoids git's path quoting, so filenames containing spaces, quotes or
// newlines are handled correctly.
//
// It returns the changed entries plus counts of tracked modifications and
// untracked files, which are reported separately: counting untracked files as
// "modified" overstates how dirty a tree is.
func parsePorcelain(out string) (changes []models.GitChange, modified, untracked int) {
	entries := strings.Split(out, "\x00")

	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}

		status := entry[:2]
		path := entry[3:]

		// Renames and copies are followed by a second record holding the
		// original path; consume it so it is not parsed as its own entry.
		if status[0] == 'R' || status[0] == 'C' {
			i++
		}

		if status == "??" {
			untracked++
		} else {
			modified++
		}

		if len(changes) < changedFileLimit {
			changes = append(changes, models.GitChange{Status: status, Path: path})
		}
	}

	return changes, modified, untracked
}

// parseCommitHistory parses unit-separated git log output.
func parseCommitHistory(out string) []models.CommitInfo {
	lines := strings.Split(out, "\n")
	commits := make([]models.CommitInfo, 0, len(lines))

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, "\x1f")
		if len(parts) < 4 {
			continue
		}

		commit := models.CommitInfo{
			Hash:    parts[0],
			Author:  parts[1],
			Message: parts[3],
		}
		if unix, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
			commit.Timestamp = time.Unix(unix, 0)
		}

		commits = append(commits, commit)
	}

	return commits
}

// git runs a git command in the repository and returns its stdout.
//
// Only stdout is returned: an earlier revision used CombinedOutput, which mixed
// git's warnings into the values being parsed and inflated counts.
func (c *GitStatusCollector) git(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = c.repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}

	return stdout.String(), nil
}

// GetLatestMetrics returns the most recent sample.
func (c *GitStatusCollector) GetLatestMetrics() models.GitRepoMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// Subscribe returns a channel receiving every new sample, plus an id for
// Unsubscribe. The channel is owned and closed by the collector.
func (c *GitStatusCollector) Subscribe() (<-chan models.GitRepoMetrics, string) {
	return c.subscribers.subscribe(subscriberBuffer)
}

// Unsubscribe releases a subscription.
func (c *GitStatusCollector) Unsubscribe(id string) {
	c.subscribers.unsubscribe(id)
}

// Start begins periodic collection.
func (c *GitStatusCollector) Start(ctx context.Context, interval time.Duration) error {
	return c.start(ctx, interval, func(ctx context.Context) {
		if _, err := c.Collect(ctx); err != nil && ctx.Err() == nil {
			c.Logger().Warn("git status collection failed", "repo", c.repoPath, "error", err)
		}
	})
}

// Stop halts collection and releases all subscribers.
func (c *GitStatusCollector) Stop() error {
	err := c.BaseCollector.Stop()
	c.subscribers.closeAll()
	return err
}

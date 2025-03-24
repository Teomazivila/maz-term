package collector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// GitStatusCollector collects Git repository status
type GitStatusCollector struct {
	*BaseCollector
	repoPath     string
	metrics      models.GitRepoMetrics
	mutex        sync.RWMutex
	subscription []chan models.GitRepoMetrics
}

// NewGitStatusCollector creates a new Git repository status collector
func NewGitStatusCollector(repoPath string) *GitStatusCollector {
	// If repoPath is empty, use the current directory
	if repoPath == "" {
		repoPath, _ = os.Getwd()
	}

	repoName := filepath.Base(repoPath)
	return &GitStatusCollector{
		BaseCollector: NewBaseCollector("git_status"),
		repoPath:      repoPath,
		metrics:       models.GitRepoMetrics{Name: repoName},
		subscription:  []chan models.GitRepoMetrics{},
	}
}

// Collect gathers Git repository status
func (c *GitStatusCollector) Collect(ctx context.Context) (interface{}, error) {
	// Initialize metrics with repo name
	repoName := filepath.Base(c.repoPath)
	metrics := models.GitRepoMetrics{
		Name: repoName,
	}

	// Check if the repo exists and get status
	if err := c.collectRepoData(ctx, &metrics); err != nil {
		return metrics, err
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

	return metrics, nil
}

// GetLatestMetrics returns the latest metrics
func (c *GitStatusCollector) GetLatestMetrics() models.GitRepoMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.metrics
}

// Subscribe returns a channel that will receive metrics updates
func (c *GitStatusCollector) Subscribe() chan models.GitRepoMetrics {
	ch := make(chan models.GitRepoMetrics, 10)
	c.mutex.Lock()
	c.subscription = append(c.subscription, ch)
	c.mutex.Unlock()
	return ch
}

// Unsubscribe removes a subscription channel
func (c *GitStatusCollector) Unsubscribe(ch chan models.GitRepoMetrics) {
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
func (c *GitStatusCollector) Start(ctx context.Context, interval time.Duration) error {
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

// collectRepoData collects data about the Git repository
func (c *GitStatusCollector) collectRepoData(ctx context.Context, metrics *models.GitRepoMetrics) error {
	// Check if it's a git repository
	gitDir := filepath.Join(c.repoPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil
	}

	// Get the current branch
	branch, err := c.runGitCommand(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		metrics.Branch = strings.TrimSpace(branch)
	}

	// Get commit count
	commitCount, err := c.runGitCommand(ctx, "rev-list", "--count", "HEAD")
	if err == nil {
		count := 0
		fmt.Sscanf(strings.TrimSpace(commitCount), "%d", &count)
		metrics.CommitCount = count
	}

	// Get last commit time
	lastCommitTime, err := c.runGitCommand(ctx, "log", "-1", "--format=%ct")
	if err == nil {
		timestamp := int64(0)
		fmt.Sscanf(strings.TrimSpace(lastCommitTime), "%d", &timestamp)
		if timestamp > 0 {
			metrics.LastCommit = time.Unix(timestamp, 0)
		}
	}

	// Get uncommitted changes
	status, err := c.runGitCommand(ctx, "status", "--porcelain")
	if err == nil {
		lines := strings.Split(strings.TrimSpace(status), "\n")
		metrics.ModifiedFiles = 0
		if len(lines) > 0 && lines[0] != "" {
			metrics.ModifiedFiles = len(lines)
		}
	}

	// Get unpushed commits
	unpushedCommits, err := c.runGitCommand(ctx, "log", "@{u}..", "--oneline")
	if err == nil {
		lines := strings.Split(strings.TrimSpace(unpushedCommits), "\n")
		metrics.PendingCommits = 0
		if len(lines) > 0 && lines[0] != "" {
			metrics.PendingCommits = len(lines)
		}
	}

	return nil
}

// runGitCommand runs a git command and returns its output
func (c *GitStatusCollector) runGitCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = c.repoPath

	// Combine stderr with stdout for error handling
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/collector"
	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestSystemMetricsCollector(t *testing.T) {
	ctx := context.Background()
	c := collector.NewSystemMetricsCollector()

	// Test collecting metrics
	metrics, err := c.Collect(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)

	sysMetrics, ok := metrics.(models.SystemMetrics)
	assert.True(t, ok)

	// Check CPU metrics
	assert.True(t, sysMetrics.CPU.UsagePercent >= 0)
	assert.NotEmpty(t, sysMetrics.CPU.CoreUsage)

	// Check memory metrics
	assert.True(t, sysMetrics.Memory.Total > 0)
	assert.True(t, sysMetrics.Memory.Used > 0)
	assert.True(t, sysMetrics.Memory.UsagePercent > 0)

	// Test the start and stop methods
	err = c.Start(ctx, 1*time.Second)
	assert.NoError(t, err)
	assert.True(t, c.IsRunning())

	// Wait for at least one collection
	time.Sleep(1100 * time.Millisecond)

	// Get latest metrics
	latest := c.GetLatestMetrics()
	assert.NotEqual(t, time.Time{}, latest.CollectedAt)

	// Stop the collector
	err = c.Stop()
	assert.NoError(t, err)
	assert.False(t, c.IsRunning())
}

func TestHTTPHealthChecker(t *testing.T) {
	ctx := context.Background()
	endpoints := []models.EndpointConfig{
		{
			Name:   "Google",
			URL:    "https://www.google.com",
			Method: "GET",
		},
		{
			Name:           "Invalid URL",
			URL:            "https://invalid.url.that.does.not.exist",
			Method:         "GET",
			ExpectedStatus: 200, // This should fail
		},
	}

	c := collector.NewHTTPHealthChecker(endpoints)

	// Test collecting metrics
	metrics, err := c.Collect(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)

	endpointMetrics, ok := metrics.(map[string]models.EndpointMetrics)
	assert.True(t, ok)

	// Check Google endpoint
	googleMetrics, exists := endpointMetrics["Google"]
	assert.True(t, exists)
	assert.True(t, googleMetrics.IsUp)
	assert.Equal(t, 200, googleMetrics.StatusCode)
	assert.NotEqual(t, time.Duration(0), googleMetrics.ResponseTime)

	// Check invalid endpoint
	invalidMetrics, exists := endpointMetrics["Invalid URL"]
	assert.True(t, exists)
	assert.False(t, invalidMetrics.IsUp)

	// Test the start and stop methods
	err = c.Start(ctx, 1*time.Second)
	assert.NoError(t, err)
	assert.True(t, c.IsRunning())

	// Wait for at least one collection
	time.Sleep(1100 * time.Millisecond)

	// Get latest metrics
	latest := c.GetLatestMetrics()
	assert.NotEmpty(t, latest)

	// Stop the collector
	err = c.Stop()
	assert.NoError(t, err)
	assert.False(t, c.IsRunning())
}

func TestGitStatusCollector(t *testing.T) {
	ctx := context.Background()
	c := collector.NewGitStatusCollector("") // Current directory

	// Test collecting metrics
	metrics, err := c.Collect(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)

	gitMetrics, ok := metrics.(models.GitRepoMetrics)
	assert.True(t, ok)

	// Since we're running in a test environment, we don't know if it's in a git repo
	// Just check that we got a valid metrics object with the name field populated
	assert.NotEmpty(t, gitMetrics.Name)

	// If this is a git repository, verify commit history structure
	if gitMetrics.Branch != "" {
		// May have commits or not, but the CommitHistory field should be initialized
		assert.NotNil(t, gitMetrics.CommitHistory)

		// If we have commits, verify their structure
		for _, commit := range gitMetrics.CommitHistory {
			if commit.Hash != "" {
				// Commit hash should be a 40-character hex string if present
				assert.Len(t, commit.Hash, 40)

				// Author should be non-empty
				assert.NotEmpty(t, commit.Author)

				// Message should be non-empty
				assert.NotEmpty(t, commit.Message)

				// If timestamp is set, it shouldn't be zero
				if !commit.Timestamp.IsZero() {
					// Timestamp should be in the past
					assert.True(t, commit.Timestamp.Before(time.Now()) ||
						commit.Timestamp.Equal(time.Now()))
				}
			}
		}
	}

	// Test the start and stop methods
	err = c.Start(ctx, 1*time.Second)
	assert.NoError(t, err)
	assert.True(t, c.IsRunning())

	// Wait for at least one collection
	time.Sleep(1100 * time.Millisecond)

	// Get latest metrics
	latest := c.GetLatestMetrics()
	assert.NotEmpty(t, latest.Name)

	// Stop the collector
	err = c.Stop()
	assert.NoError(t, err)
	assert.False(t, c.IsRunning())
}

func TestSubscription(t *testing.T) {
	ctx := context.Background()
	c := collector.NewSystemMetricsCollector()

	// Create a subscription with context
	ch, id := c.Subscribe(ctx)

	// Start the collector
	err := c.Start(ctx, 500*time.Millisecond)
	assert.NoError(t, err)

	// Wait for metrics to be received
	select {
	case metrics := <-ch:
		assert.NotEqual(t, time.Time{}, metrics.CollectedAt)
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for metrics")
	}

	// Unsubscribe
	c.Unsubscribe(id)

	// Stop the collector
	err = c.Stop()
	assert.NoError(t, err)
}

func TestHTTPSubscription(t *testing.T) {
	ctx := context.Background()
	endpoints := []models.EndpointConfig{
		{
			Name:   "Google",
			URL:    "https://www.google.com",
			Method: "GET",
		},
	}

	c := collector.NewHTTPHealthChecker(endpoints)

	// Create a subscription with context
	ch, id := c.Subscribe(ctx)

	// Start the collector
	err := c.Start(ctx, 500*time.Millisecond)
	assert.NoError(t, err)

	// Wait for metrics to be received
	select {
	case metrics := <-ch:
		assert.NotEmpty(t, metrics)
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for HTTP metrics")
	}

	// Unsubscribe
	c.Unsubscribe(id)

	// Stop the collector
	err = c.Stop()
	assert.NoError(t, err)
}

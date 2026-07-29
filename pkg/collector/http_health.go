package collector

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

const (
	// availabilityWindow is how many recent checks the availability percentage
	// is computed over, per endpoint.
	availabilityWindow = 100

	// maxConcurrentChecks bounds in-flight requests so a large endpoint list
	// cannot open an unbounded number of sockets at once.
	maxConcurrentChecks = 8

	// defaultCheckTimeout applies to endpoints that do not set their own.
	defaultCheckTimeout = 10 * time.Second
)

// HTTPHealthChecker checks the health of HTTP endpoints.
type HTTPHealthChecker struct {
	*BaseCollector

	endpoints []models.EndpointConfig
	client    *http.Client

	mu      sync.RWMutex
	metrics map[string]models.EndpointMetrics
	history map[string][]bool

	subscribers *broadcaster[map[string]models.EndpointMetrics]
}

// NewHTTPHealthChecker creates a health checker for the given endpoints.
func NewHTTPHealthChecker(endpoints []models.EndpointConfig) *HTTPHealthChecker {
	client := &http.Client{
		Timeout: defaultCheckTimeout,
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       60 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	return &HTTPHealthChecker{
		BaseCollector: NewBaseCollector("http_health"),
		endpoints:     endpoints,
		client:        client,
		metrics:       make(map[string]models.EndpointMetrics, len(endpoints)),
		history:       make(map[string][]bool, len(endpoints)),
		subscribers:   newBroadcaster[map[string]models.EndpointMetrics](),
	}
}

// Collect checks every configured endpoint concurrently, bounded by
// maxConcurrentChecks.
func (c *HTTPHealthChecker) Collect(ctx context.Context) (any, error) {
	results := make(map[string]models.EndpointMetrics, len(c.endpoints))

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		sema = make(chan struct{}, maxConcurrentChecks)
	)

	for _, endpoint := range c.endpoints {
		wg.Add(1)
		go func(ep models.EndpointConfig) {
			defer wg.Done()

			select {
			case sema <- struct{}{}:
				defer func() { <-sema }()
			case <-ctx.Done():
				return
			}

			metric := c.checkEndpoint(ctx, ep)

			mu.Lock()
			results[ep.Name] = metric
			mu.Unlock()
		}(endpoint)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Fold in the rolling availability before publishing, so every consumer
	// sees the same figures.
	c.mu.Lock()
	for name, metric := range results {
		window := append(c.history[name], metric.IsUp)
		if len(window) > availabilityWindow {
			window = window[len(window)-availabilityWindow:]
		}
		c.history[name] = window

		up := 0
		for _, ok := range window {
			if ok {
				up++
			}
		}
		metric.ChecksInWindow = len(window)
		if len(window) > 0 {
			metric.Availability = float64(up) / float64(len(window)) * 100
		}
		results[name] = metric
	}
	c.metrics = results
	c.mu.Unlock()

	c.UpdateData(results)
	c.subscribers.publish(results)
	c.store(results)

	return results, nil
}

// store persists each endpoint sample, logging rather than discarding failures.
func (c *HTTPHealthChecker) store(results map[string]models.EndpointMetrics) {
	store := c.Storage()
	if store == nil {
		return
	}
	for name, metric := range results {
		if err := store.StoreHTTPMetrics(name, metric); err != nil {
			c.Logger().Error("failed to store HTTP metrics", "endpoint", name, "error", err)
		}
	}
}

// checkEndpoint performs a single health check. It never returns an error: an
// unreachable endpoint is a result, recorded in the metric's Error field.
func (c *HTTPHealthChecker) checkEndpoint(ctx context.Context, endpoint models.EndpointConfig) models.EndpointMetrics {
	metric := models.EndpointMetrics{
		Name:        endpoint.Name,
		URL:         endpoint.URL,
		LastChecked: time.Now(),
	}

	// Only http(s) is checked. Anything else in the configuration is a mistake
	// worth surfacing rather than handing to the transport.
	parsed, err := url.Parse(endpoint.URL)
	if err != nil {
		metric.Error = fmt.Sprintf("invalid URL: %v", err)
		return metric
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		metric.Error = fmt.Sprintf("unsupported URL scheme %q", parsed.Scheme)
		return metric
	}

	method := endpoint.Method
	if method == "" {
		method = http.MethodGet
	}

	checkCtx := ctx
	if endpoint.Timeout > 0 {
		var cancel context.CancelFunc
		checkCtx, cancel = context.WithTimeout(ctx, endpoint.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(checkCtx, method, endpoint.URL, nil)
	if err != nil {
		metric.Error = fmt.Sprintf("building request: %v", err)
		return metric
	}

	for key, value := range endpoint.Headers {
		req.Header.Add(key, value)
	}
	req.Header.Set("User-Agent", "maz-term")

	start := time.Now()
	resp, err := c.client.Do(req)
	metric.ResponseTime = time.Since(start)
	if err != nil {
		metric.Error = err.Error()
		return metric
	}
	defer resp.Body.Close()

	metric.StatusCode = resp.StatusCode
	if endpoint.ExpectedStatus != 0 {
		metric.IsUp = resp.StatusCode == endpoint.ExpectedStatus
	} else {
		metric.IsUp = resp.StatusCode >= 200 && resp.StatusCode < 300
	}

	return metric
}

// GetLatestMetrics returns the most recent results keyed by endpoint name.
func (c *HTTPHealthChecker) GetLatestMetrics() map[string]models.EndpointMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// A copy is returned because the caller renders it on another goroutine
	// while the next collection cycle replaces the internal map.
	out := make(map[string]models.EndpointMetrics, len(c.metrics))
	for name, metric := range c.metrics {
		out[name] = metric
	}
	return out
}

// GetEndpoints returns the configured endpoints.
func (c *HTTPHealthChecker) GetEndpoints() []models.EndpointConfig {
	return c.endpoints
}

// Subscribe returns a channel receiving every new result set, plus an id for
// Unsubscribe. The channel is owned and closed by the collector.
func (c *HTTPHealthChecker) Subscribe() (<-chan map[string]models.EndpointMetrics, string) {
	return c.subscribers.subscribe(subscriberBuffer)
}

// Unsubscribe releases a subscription.
func (c *HTTPHealthChecker) Unsubscribe(id string) {
	c.subscribers.unsubscribe(id)
}

// Start begins periodic checking.
func (c *HTTPHealthChecker) Start(ctx context.Context, interval time.Duration) error {
	return c.start(ctx, interval, func(ctx context.Context) {
		if _, err := c.Collect(ctx); err != nil && ctx.Err() == nil {
			c.Logger().Warn("HTTP health collection failed", "error", err)
		}
	})
}

// Stop halts checking and releases all subscribers.
func (c *HTTPHealthChecker) Stop() error {
	err := c.BaseCollector.Stop()
	c.subscribers.closeAll()
	return err
}

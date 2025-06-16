package collector

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// HTTPSubscriber represents a subscription with context for cleanup
type HTTPSubscriber struct {
	ch     chan map[string]models.EndpointMetrics
	ctx    context.Context
	cancel context.CancelFunc
}

// HTTPHealthChecker checks the health of HTTP endpoints
type HTTPHealthChecker struct {
	*BaseCollector
	endpoints   []models.EndpointConfig
	metrics     map[string]models.EndpointMetrics
	client      *http.Client
	mutex       sync.RWMutex
	subscribers map[string]*HTTPSubscriber
	subMutex    sync.RWMutex
}

// NewHTTPHealthChecker creates a new HTTP health checker
func NewHTTPHealthChecker(endpoints []models.EndpointConfig) *HTTPHealthChecker {
	// Create a custom HTTP client with proper timeouts and connection pooling
	client := &http.Client{
		Timeout: 10 * time.Second,
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
		metrics:       make(map[string]models.EndpointMetrics),
		client:        client,
		subscribers:   make(map[string]*HTTPSubscriber),
	}
}

// Collect checks the health of all HTTP endpoints
func (c *HTTPHealthChecker) Collect(ctx context.Context) (interface{}, error) {
	metrics := make(map[string]models.EndpointMetrics)
	var wg sync.WaitGroup
	var mutex sync.Mutex // To protect concurrent writes to metrics map

	// Create a context with timeout for all endpoint checks
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Start a goroutine for each endpoint
	for _, endpoint := range c.endpoints {
		wg.Add(1)
		go func(ep models.EndpointConfig) {
			defer wg.Done()
			metric := c.checkEndpoint(checkCtx, ep)

			mutex.Lock()
			metrics[ep.Name] = metric
			mutex.Unlock()

			// Store metrics for this endpoint
			c.StoreData(ep.Name, metric, "http")
		}(endpoint)
	}

	// Wait for all checks to complete
	wg.Wait()

	// Update the metrics
	c.mutex.Lock()
	c.metrics = metrics
	c.mutex.Unlock()

	// Notify subscribers with proper cleanup
	c.notifySubscribers(metrics)

	return metrics, nil
}

// notifySubscribers sends metrics to all active subscribers
func (c *HTTPHealthChecker) notifySubscribers(metrics map[string]models.EndpointMetrics) {
	c.subMutex.RLock()
	subscribers := make([]*HTTPSubscriber, 0, len(c.subscribers))
	for _, sub := range c.subscribers {
		subscribers = append(subscribers, sub)
	}
	c.subMutex.RUnlock()

	// Send to subscribers in parallel to avoid blocking
	var wg sync.WaitGroup
	for _, sub := range subscribers {
		wg.Add(1)
		go func(s *HTTPSubscriber) {
			defer wg.Done()
			select {
			case s.ch <- metrics:
				// Successfully sent
			case <-s.ctx.Done():
				// Subscriber context cancelled, will be cleaned up
			case <-time.After(100 * time.Millisecond):
				// Channel is blocked, skip this subscriber
			}
		}(sub)
	}
	wg.Wait()

	// Clean up cancelled subscribers
	c.cleanupDeadSubscribers()
}

// cleanupDeadSubscribers removes subscribers with cancelled contexts
func (c *HTTPHealthChecker) cleanupDeadSubscribers() {
	c.subMutex.Lock()
	defer c.subMutex.Unlock()

	for id, sub := range c.subscribers {
		select {
		case <-sub.ctx.Done():
			close(sub.ch)
			delete(c.subscribers, id)
		default:
			// Subscriber is still active
		}
	}
}

// GetLatestMetrics returns the latest metrics
func (c *HTTPHealthChecker) GetLatestMetrics() map[string]models.EndpointMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.metrics
}

// Subscribe returns a channel that will receive metrics updates
func (c *HTTPHealthChecker) Subscribe(ctx context.Context) (chan map[string]models.EndpointMetrics, string) {
	subCtx, cancel := context.WithCancel(ctx)
	ch := make(chan map[string]models.EndpointMetrics, 10)

	// Generate unique ID for this subscriber
	id := fmt.Sprintf("http_sub_%d_%d", time.Now().UnixNano(), len(c.subscribers))

	subscriber := &HTTPSubscriber{
		ch:     ch,
		ctx:    subCtx,
		cancel: cancel,
	}

	c.subMutex.Lock()
	c.subscribers[id] = subscriber
	c.subMutex.Unlock()

	return ch, id
}

// Unsubscribe removes a subscription channel
func (c *HTTPHealthChecker) Unsubscribe(id string) {
	c.subMutex.Lock()
	defer c.subMutex.Unlock()

	if sub, exists := c.subscribers[id]; exists {
		sub.cancel()
		close(sub.ch)
		delete(c.subscribers, id)
	}
}

// Start starts the collector
func (c *HTTPHealthChecker) Start(ctx context.Context, interval time.Duration) error {
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
				if collectorCtx := c.Context(); collectorCtx != nil {
					_, _ = c.Collect(collectorCtx) // Ignore errors during background collection
				}
			case <-c.Context().Done():
				// Clean up all subscribers when collector stops
				c.cleanupAllSubscribers()
				return
			}
		}
	}()

	return nil
}

// cleanupAllSubscribers closes all subscriber channels
func (c *HTTPHealthChecker) cleanupAllSubscribers() {
	c.subMutex.Lock()
	defer c.subMutex.Unlock()

	for id, sub := range c.subscribers {
		sub.cancel()
		close(sub.ch)
		delete(c.subscribers, id)
	}
}

// checkEndpoint checks the health of a single HTTP endpoint with proper error handling
func (c *HTTPHealthChecker) checkEndpoint(ctx context.Context, endpoint models.EndpointConfig) models.EndpointMetrics {
	metric := models.EndpointMetrics{
		Name:        endpoint.Name,
		URL:         endpoint.URL,
		LastChecked: time.Now(),
		IsUp:        false,
	}

	// Create a new request with context
	req, err := http.NewRequestWithContext(ctx, endpoint.Method, endpoint.URL, nil)
	if err != nil {
		return metric
	}

	// Add headers if provided
	if endpoint.Headers != nil {
		for key, value := range endpoint.Headers {
			req.Header.Add(key, value)
		}
	}

	// Add User-Agent header
	req.Header.Set("User-Agent", "maz-term/1.0")

	// Measure response time
	startTime := time.Now()
	resp, err := c.client.Do(req)
	responseTime := time.Since(startTime)
	metric.ResponseTime = responseTime

	// Check for errors
	if err != nil {
		return metric
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	// Record status code
	metric.StatusCode = resp.StatusCode

	// Check if the status code is considered healthy
	if endpoint.ExpectedStatus != 0 {
		metric.IsUp = resp.StatusCode == endpoint.ExpectedStatus
	} else {
		// Default: consider 2xx as healthy
		metric.IsUp = resp.StatusCode >= 200 && resp.StatusCode < 300
	}

	// Status is determined by the IsUp field and StatusCode

	return metric
}

// GetEndpoints returns the configured endpoints for the health checker
func (c *HTTPHealthChecker) GetEndpoints() []models.EndpointConfig {
	return c.endpoints
}

// Name returns the collector name
func (c *HTTPHealthChecker) Name() string {
	return c.GetName()
}

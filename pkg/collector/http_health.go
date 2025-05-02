package collector

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
)

// HTTPHealthChecker checks the health of HTTP endpoints
type HTTPHealthChecker struct {
	*BaseCollector
	endpoints    []models.EndpointConfig
	metrics      map[string]models.EndpointMetrics
	client       *http.Client
	mutex        sync.RWMutex
	subscription []chan map[string]models.EndpointMetrics
}

// NewHTTPHealthChecker creates a new HTTP health checker
func NewHTTPHealthChecker(endpoints []models.EndpointConfig) *HTTPHealthChecker {
	// Create a custom HTTP client with timeouts
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     60 * time.Second,
		},
	}

	return &HTTPHealthChecker{
		BaseCollector: NewBaseCollector("http_health"),
		endpoints:     endpoints,
		metrics:       make(map[string]models.EndpointMetrics),
		client:        client,
		subscription:  []chan map[string]models.EndpointMetrics{},
	}
}

// Collect checks the health of all HTTP endpoints
func (c *HTTPHealthChecker) Collect(ctx context.Context) (interface{}, error) {
	metrics := make(map[string]models.EndpointMetrics)
	var wg sync.WaitGroup
	var mutex sync.Mutex // To protect concurrent writes to metrics map

	// Start a goroutine for each endpoint
	for _, endpoint := range c.endpoints {
		wg.Add(1)
		go func(ep models.EndpointConfig) {
			defer wg.Done()
			metric := c.checkEndpoint(ctx, ep)

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
func (c *HTTPHealthChecker) GetLatestMetrics() map[string]models.EndpointMetrics {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.metrics
}

// Subscribe returns a channel that will receive metrics updates
func (c *HTTPHealthChecker) Subscribe() chan map[string]models.EndpointMetrics {
	ch := make(chan map[string]models.EndpointMetrics, 10)
	c.mutex.Lock()
	c.subscription = append(c.subscription, ch)
	c.mutex.Unlock()
	return ch
}

// Unsubscribe removes a subscription channel
func (c *HTTPHealthChecker) Unsubscribe(ch chan map[string]models.EndpointMetrics) {
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

// checkEndpoint checks the health of a single HTTP endpoint
func (c *HTTPHealthChecker) checkEndpoint(ctx context.Context, endpoint models.EndpointConfig) models.EndpointMetrics {
	metric := models.EndpointMetrics{
		Name:        endpoint.Name,
		URL:         endpoint.URL,
		LastChecked: time.Now(),
		IsUp:        false,
	}

	// Create a new request
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

	// Measure response time
	startTime := time.Now()
	resp, err := c.client.Do(req)
	responseTime := time.Since(startTime)
	metric.ResponseTime = responseTime

	// Check for errors
	if err != nil {
		return metric
	}
	defer resp.Body.Close()

	// Record status code
	metric.StatusCode = resp.StatusCode

	// Check if the status code is considered healthy
	if endpoint.ExpectedStatus != 0 {
		metric.IsUp = resp.StatusCode == endpoint.ExpectedStatus
	} else {
		// Default: consider 2xx as healthy
		metric.IsUp = resp.StatusCode >= 200 && resp.StatusCode < 300
	}

	return metric
}

// GetEndpoints returns the configured endpoints for the health checker
func (h *HTTPHealthChecker) GetEndpoints() []models.EndpointConfig {
	return h.endpoints
}

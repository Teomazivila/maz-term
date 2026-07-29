package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPCheckerStatusHandling(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		expectedStatus int
		wantUp         bool
	}{
		{name: "200 is up", status: http.StatusOK, wantUp: true},
		{name: "204 is up", status: http.StatusNoContent, wantUp: true},
		{name: "301 is down by default", status: http.StatusMovedPermanently, wantUp: false},
		{name: "404 is down", status: http.StatusNotFound, wantUp: false},
		{name: "500 is down", status: http.StatusInternalServerError, wantUp: false},
		{name: "explicit expectation matched", status: http.StatusTeapot, expectedStatus: http.StatusTeapot, wantUp: true},
		{name: "explicit expectation missed", status: http.StatusOK, expectedStatus: http.StatusTeapot, wantUp: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			c := NewHTTPHealthChecker([]models.EndpointConfig{{
				Name:           "test",
				URL:            server.URL,
				Method:         http.MethodGet,
				ExpectedStatus: tt.expectedStatus,
			}})

			_, err := c.Collect(context.Background())
			require.NoError(t, err)

			metric := c.GetLatestMetrics()["test"]
			assert.Equal(t, tt.status, metric.StatusCode)
			assert.Equal(t, tt.wantUp, metric.IsUp)
			assert.Empty(t, metric.Error)
			assert.Positive(t, metric.ResponseTime)
		})
	}
}

// TestHTTPCheckerRecordsTransportErrors pins the Error field: an unreachable
// endpoint is a result to display, not a silent zero row.
func TestHTTPCheckerRecordsTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close() // nothing is listening now

	c := NewHTTPHealthChecker([]models.EndpointConfig{{Name: "gone", URL: url}})

	_, err := c.Collect(context.Background())
	require.NoError(t, err, "an unreachable endpoint is a result, not a collection failure")

	metric := c.GetLatestMetrics()["gone"]
	assert.False(t, metric.IsUp)
	assert.NotEmpty(t, metric.Error, "the transport failure must be recorded")
	assert.Zero(t, metric.StatusCode)
}

// TestHTTPCheckerRejectsNonHTTPSchemes keeps non-web URLs out of the transport.
func TestHTTPCheckerRejectsNonHTTPSchemes(t *testing.T) {
	for _, url := range []string{"file:///etc/passwd", "ftp://example.com", "://bad"} {
		c := NewHTTPHealthChecker([]models.EndpointConfig{{Name: "bad", URL: url}})

		_, err := c.Collect(context.Background())
		require.NoError(t, err)

		metric := c.GetLatestMetrics()["bad"]
		assert.False(t, metric.IsUp)
		assert.NotEmpty(t, metric.Error, "%s must be refused with a reason", url)
	}
}

// TestHTTPCheckerAvailabilityIsARealPercentage pins M-011: the column used to
// render the boolean IsUp under an "Availability" heading.
func TestHTTPCheckerAvailabilityIsARealPercentage(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewHTTPHealthChecker([]models.EndpointConfig{{Name: "api", URL: server.URL}})

	// Three successes.
	for range 3 {
		_, err := c.Collect(context.Background())
		require.NoError(t, err)
	}
	metric := c.GetLatestMetrics()["api"]
	assert.Equal(t, 3, metric.ChecksInWindow)
	assert.InDelta(t, 100.0, metric.Availability, 0.001)

	// One failure out of four.
	fail.Store(true)
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metric = c.GetLatestMetrics()["api"]
	assert.Equal(t, 4, metric.ChecksInWindow)
	assert.InDelta(t, 75.0, metric.Availability, 0.001)
}

func TestHTTPCheckerAvailabilityWindowIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewHTTPHealthChecker([]models.EndpointConfig{{Name: "api", URL: server.URL}})

	for range availabilityWindow + 25 {
		_, err := c.Collect(context.Background())
		require.NoError(t, err)
	}

	metric := c.GetLatestMetrics()["api"]
	assert.Equal(t, availabilityWindow, metric.ChecksInWindow, "the window must not grow without bound")
}

func TestHTTPCheckerDefaultsMethodToGet(t *testing.T) {
	var gotMethod atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod.Store(r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewHTTPHealthChecker([]models.EndpointConfig{{Name: "api", URL: server.URL}})
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	assert.Equal(t, http.MethodGet, gotMethod.Load())
}

func TestHTTPCheckerSendsConfiguredHeaders(t *testing.T) {
	var gotHeader, gotAgent atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader.Store(r.Header.Get("X-Probe"))
		gotAgent.Store(r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewHTTPHealthChecker([]models.EndpointConfig{{
		Name:    "api",
		URL:     server.URL,
		Headers: map[string]string{"X-Probe": "maz-term"},
	}})
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "maz-term", gotHeader.Load())
	assert.Equal(t, "maz-term", gotAgent.Load())
}

func TestHTTPCheckerPerEndpointTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewHTTPHealthChecker([]models.EndpointConfig{{
		Name:    "slow",
		URL:     server.URL,
		Timeout: 30 * time.Millisecond,
	}})

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	metric := c.GetLatestMetrics()["slow"]
	assert.False(t, metric.IsUp)
	assert.NotEmpty(t, metric.Error)
}

func TestHTTPCheckerPersistsEveryEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := &recordingStore{}
	c := NewHTTPHealthChecker([]models.EndpointConfig{
		{Name: "one", URL: server.URL},
		{Name: "two", URL: server.URL},
	})
	c.SetStorageProvider(store)

	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	_, httpCount, _ := store.counts()
	assert.Equal(t, 2, httpCount)
}

// TestHTTPCheckerConcurrencyIsBounded checks the semaphore keeps in-flight
// requests capped regardless of how many endpoints are configured.
func TestHTTPCheckerConcurrencyIsBounded(t *testing.T) {
	var inFlight, peak atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := inFlight.Add(1)
		for {
			observed := peak.Load()
			if current <= observed || peak.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoints := make([]models.EndpointConfig, 0, 40)
	for i := range 40 {
		endpoints = append(endpoints, models.EndpointConfig{
			Name: string(rune('a'+i%26)) + string(rune('0'+i/26)),
			URL:  server.URL,
		})
	}

	c := NewHTTPHealthChecker(endpoints)
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	assert.LessOrEqual(t, peak.Load(), int64(maxConcurrentChecks),
		"in-flight requests must stay within the configured bound")
}

func TestHTTPCheckerRespectsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	c := NewHTTPHealthChecker([]models.EndpointConfig{{Name: "slow", URL: server.URL}})

	_, err := c.Collect(ctx)
	require.Error(t, err, "a cancelled collection must report the cancellation")
}

func TestHTTPCheckerWithNoEndpoints(t *testing.T) {
	c := NewHTTPHealthChecker(nil)

	result, err := c.Collect(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Empty(t, c.GetLatestMetrics())
}

// TestGetLatestMetricsReturnsACopy prevents a caller from mutating the
// collector's internal map while the next cycle replaces it.
func TestGetLatestMetricsReturnsACopy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewHTTPHealthChecker([]models.EndpointConfig{{Name: "api", URL: server.URL}})
	_, err := c.Collect(context.Background())
	require.NoError(t, err)

	first := c.GetLatestMetrics()
	first["api"] = models.EndpointMetrics{Name: "tampered"}
	delete(first, "api")

	second := c.GetLatestMetrics()
	require.Contains(t, second, "api")
	assert.Equal(t, "api", second["api"].Name)
}

func TestHTTPCheckerGetEndpoints(t *testing.T) {
	endpoints := []models.EndpointConfig{{Name: "a", URL: "https://a.example"}}
	c := NewHTTPHealthChecker(endpoints)

	assert.Equal(t, endpoints, c.GetEndpoints())
	assert.Equal(t, "http_health", c.Name())
}

package models

import "time"

// EndpointConfig represents configuration for an HTTP endpoint to check
type EndpointConfig struct {
	Name           string            `json:"name"`
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	Headers        map[string]string `json:"headers,omitempty"`
	ExpectedStatus int               `json:"expected_status,omitempty"`
	Timeout        time.Duration     `json:"timeout,omitempty"`
	Interval       time.Duration     `json:"interval,omitempty"`
}

// EndpointMetrics represents metrics for an HTTP endpoint check
type EndpointMetrics struct {
	Name         string        `json:"name"`
	URL          string        `json:"url"`
	StatusCode   int           `json:"status_code"`
	ResponseTime time.Duration `json:"response_time"`
	IsUp         bool          `json:"is_up"`
	LastChecked  time.Time     `json:"last_checked"`
}

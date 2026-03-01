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

package models

import "time"

// EndpointConfig represents configuration for an HTTP endpoint to check.
//
// The mapstructure tags are required, not decorative. This struct is decoded
// straight from the configuration file, and mapstructure matches field names
// case-insensitively but does not convert snake_case: without an explicit tag,
// "expected_status" never reaches ExpectedStatus.
type EndpointConfig struct {
	Name           string            `json:"name"                      mapstructure:"name"`
	URL            string            `json:"url"                       mapstructure:"url"`
	Method         string            `json:"method"                    mapstructure:"method"`
	Headers        map[string]string `json:"headers,omitempty"         mapstructure:"headers"`
	ExpectedStatus int               `json:"expected_status,omitempty" mapstructure:"expected_status"`
	Timeout        time.Duration     `json:"timeout,omitempty"         mapstructure:"timeout"`
	Interval       time.Duration     `json:"interval,omitempty"        mapstructure:"interval"`
}

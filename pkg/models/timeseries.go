package models

import (
	"time"
)

// TimeSeriesPoint represents a single data point in a time series
type TimeSeriesPoint struct {
	Timestamp time.Time
	Value     float64
}

// TimeSeriesData represents a collection of time series points with metadata
type TimeSeriesData struct {
	Name        string
	Description string
	Unit        string
	Points      []TimeSeriesPoint
}

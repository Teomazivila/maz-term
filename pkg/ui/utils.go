package ui

import (
	"sync"
	"time"
)

// MaxInt returns the maximum of two integers
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// TruncateString truncates a string to the given length and adds "..." if truncated
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Debouncer provides debounced function execution to prevent too many UI redraws
type Debouncer struct {
	mu             sync.Mutex
	duration       time.Duration
	timer          *time.Timer
	lastUpdateTime time.Time
}

// NewDebouncer creates a new debouncer with specified delay
func NewDebouncer(duration time.Duration) *Debouncer {
	return &Debouncer{
		duration:       duration,
		lastUpdateTime: time.Now(),
	}
}

// Debounce executes the provided function once after the debounce duration
// has passed since the last call
func (d *Debouncer) Debounce(f func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// If timer exists, stop it
	if d.timer != nil {
		d.timer.Stop()
	}

	// Only execute if enough time has passed since last update
	now := time.Now()
	if now.Sub(d.lastUpdateTime) > d.duration {
		d.lastUpdateTime = now
		f()
		return
	}

	// Create a new timer
	d.timer = time.AfterFunc(d.duration, func() {
		d.mu.Lock()
		d.lastUpdateTime = time.Now()
		d.mu.Unlock()
		f()
	})
}

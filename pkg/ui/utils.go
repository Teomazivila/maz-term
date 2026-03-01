package ui

import (
	"fmt"
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

// FormatBytes formats a number of bytes into a human-readable string
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatDurationSince formats the duration since a given time
func FormatDurationSince(t time.Time) string {
	return FormatDuration(time.Since(t))
}

// FormatDuration formats a duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d.Hours() > 24 {
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	} else if d.Hours() > 1 {
		return fmt.Sprintf("%.1fh", d.Hours())
	} else if d.Minutes() > 1 {
		return fmt.Sprintf("%.1fm", d.Minutes())
	} else {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

// FormatTime formats a time in a user-friendly way
func FormatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		minutes := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", minutes)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%dh ago", hours)
	} else if diff < 48*time.Hour {
		return "yesterday"
	} else if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	} else if now.Year() == t.Year() {
		return t.Format("Jan 2")
	} else {
		return t.Format("Jan 2, 2006")
	}
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

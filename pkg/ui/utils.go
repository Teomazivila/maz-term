package ui

import (
	"fmt"
	"time"
)

// ellipsis is appended to a truncated string.
const ellipsis = "..."

// TruncateString shortens s to at most maxLen characters, marking the cut with an
// ellipsis when there is room for one.
//
// Truncation is measured in runes, so a multi-byte character is never split into
// invalid UTF-8. A maxLen smaller than the ellipsis no longer panics: the
// previous implementation sliced s[:maxLen-3] unconditionally, which is a
// negative index for any maxLen below three.
func TruncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	if maxLen <= len(ellipsis) {
		return string(runes[:maxLen])
	}

	return string(runes[:maxLen-len(ellipsis)]) + ellipsis
}

// FormatBytes renders a byte count in binary units.
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatDurationSince renders how long ago t was.
func FormatDurationSince(t time.Time) string {
	return FormatDuration(time.Since(t))
}

// FormatDuration renders a duration at a single significant unit.
func FormatDuration(d time.Duration) string {
	switch {
	case d.Hours() >= 24:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d.Hours() >= 1:
		return fmt.Sprintf("%.1fh", d.Hours())
	case d.Minutes() >= 1:
		return fmt.Sprintf("%.1fm", d.Minutes())
	default:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

// FormatTime renders a timestamp relative to now, for use in tables.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}

	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < 0:
		return t.Format("15:04:05")
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 48*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	case now.Year() == t.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}

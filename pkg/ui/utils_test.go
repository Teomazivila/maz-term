package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestTruncateStringShortLimits pins the panic: the previous implementation
// sliced s[:maxLen-3] unconditionally, a negative index below maxLen 3.
func TestTruncateStringShortLimits(t *testing.T) {
	tests := []struct {
		in     string
		maxLen int
		want   string
	}{
		{in: "hello", maxLen: 0, want: ""},
		{in: "hello", maxLen: -1, want: ""},
		{in: "hello", maxLen: 1, want: "h"},
		{in: "hello", maxLen: 2, want: "he"},
		{in: "hello", maxLen: 3, want: "hel"},
		{in: "hello", maxLen: 4, want: "h..."},
		{in: "hello", maxLen: 5, want: "hello"},
		{in: "hello", maxLen: 99, want: "hello"},
		{in: "", maxLen: 5, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.in+"/"+time.Duration(tt.maxLen).String(), func(t *testing.T) {
			var got string
			assert.NotPanics(t, func() { got = TruncateString(tt.in, tt.maxLen) })

			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len([]rune(got)), max(tt.maxLen, 0),
				"the result must respect the limit")
		})
	}
}

// TestTruncateStringDoesNotSplitRunes guards against producing invalid UTF-8.
func TestTruncateStringDoesNotSplitRunes(t *testing.T) {
	// Each of these is multiple bytes but one rune.
	const s = "héllo wörld ünicode"

	for maxLen := 1; maxLen <= len(s)+2; maxLen++ {
		got := TruncateString(s, maxLen)
		assert.True(t, utf8ValidString(got), "maxLen=%d produced invalid UTF-8: %q", maxLen, got)
		assert.LessOrEqual(t, len([]rune(got)), maxLen)
	}
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{in: 0, want: "0 B"},
		{in: 512, want: "512 B"},
		{in: 1024, want: "1.0 KiB"},
		{in: 1536, want: "1.5 KiB"},
		{in: 1 << 20, want: "1.0 MiB"},
		{in: 1 << 30, want: "1.0 GiB"},
		{in: 1 << 40, want: "1.0 TiB"},
		{in: 1 << 50, want: "1.0 PiB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatBytes(tt.in))
		})
	}
}

// TestFormatBytesAtMaxUint64 checks the unit index cannot run off the end of the
// suffix string.
func TestFormatBytesAtMaxUint64(t *testing.T) {
	assert.NotPanics(t, func() {
		got := FormatBytes(^uint64(0))
		assert.Contains(t, got, "iB")
	})
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: 500 * time.Millisecond, want: "0.5s"},
		{in: 30 * time.Second, want: "30.0s"},
		{in: 90 * time.Second, want: "1.5m"},
		{in: 2 * time.Hour, want: "2.0h"},
		{in: 48 * time.Hour, want: "2d"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatDuration(tt.in))
		})
	}
}

func TestFormatTime(t *testing.T) {
	now := time.Now()

	assert.Equal(t, "-", FormatTime(time.Time{}), "a zero time must not render as year 1")
	assert.Equal(t, "just now", FormatTime(now.Add(-10*time.Second)))
	assert.Equal(t, "5m ago", FormatTime(now.Add(-5*time.Minute)))
	assert.Equal(t, "3h ago", FormatTime(now.Add(-3*time.Hour)))
	assert.Equal(t, "yesterday", FormatTime(now.Add(-30*time.Hour)))
	assert.Equal(t, "4d ago", FormatTime(now.Add(-4*24*time.Hour)))

	// A clock skew into the future must not produce a negative duration string.
	assert.NotContains(t, FormatTime(now.Add(time.Hour)), "-")
}

func TestFormatDurationSince(t *testing.T) {
	assert.Contains(t, FormatDurationSince(time.Now().Add(-90*time.Second)), "m")
}

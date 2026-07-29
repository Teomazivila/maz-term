package ui

import (
	"image"
	"testing"

	"github.com/Teomazivila/maz-term/pkg/config"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestApp returns an app with chrome constructed but termui uninitialised, so
// tests run without a terminal.
func newTestApp(t *testing.T) *App {
	t.Helper()

	app := NewApp(config.DefaultConfig())
	app.TermWidth, app.TermHeight = 120, 40
	app.StatusBar = widgets.NewParagraph()
	app.TabBar = widgets.NewTabPane(app.getTabNames()...)

	return app
}

// TestBuildFrameIncludesTabContentAndChrome is the regression test for the defect
// that made the whole dashboard invisible: the event loop refreshed widget state
// but never produced anything to draw.
func TestBuildFrameIncludesTabContentAndChrome(t *testing.T) {
	app := newTestApp(t)
	app.updateData()

	frame := app.buildFrame()

	require.NotEmpty(t, frame, "a frame must contain drawables; an empty frame renders a blank terminal")

	var sawStatusBar, sawTabBar, sawContent bool
	for _, drawable := range frame {
		switch drawable {
		case ui.Drawable(app.StatusBar):
			sawStatusBar = true
		case ui.Drawable(app.TabBar):
			sawTabBar = true
		default:
			sawContent = true
		}
	}

	assert.True(t, sawStatusBar, "status bar must be drawn")
	assert.True(t, sawTabBar, "tab bar must be drawn")
	assert.True(t, sawContent, "the active tab's widgets must be drawn")
}

// TestBuildFrameForEveryTab checks that no tab produces an empty frame or panics,
// which also exercises every layout function.
func TestBuildFrameForEveryTab(t *testing.T) {
	app := newTestApp(t)
	app.updateData()

	for i, tab := range app.Tabs {
		t.Run(tab.Name, func(t *testing.T) {
			app.ActiveTabIndex = i
			app.updateData()

			frame := app.buildFrame()
			assert.NotEmpty(t, frame, "tab %s produced nothing to draw", tab.Name)

			for _, drawable := range frame {
				rect := drawable.GetRect()
				assert.GreaterOrEqual(t, rect.Dx(), minContentWidth/2,
					"widget on %s is too narrow to draw", tab.Name)

				// The status and tab bars are borderless single rows; every
				// content widget is bordered and needs an interior.
				isChrome := drawable == ui.Drawable(app.StatusBar) || drawable == ui.Drawable(app.TabBar)
				if isChrome {
					assert.Equal(t, 1, rect.Dy(), "chrome bars occupy exactly one row")
					continue
				}
				assert.GreaterOrEqual(t, rect.Dy(), minWidgetExtent,
					"widget on %s has no drawable interior", tab.Name)
			}

			// Content must never overlap the chrome at the bottom of the screen.
			contentBottom := app.TermHeight - statusBarHeight - tabBarHeight
			for _, drawable := range frame {
				if drawable == ui.Drawable(app.StatusBar) || drawable == ui.Drawable(app.TabBar) {
					continue
				}
				assert.LessOrEqual(t, drawable.GetRect().Max.Y, contentBottom,
					"widget on %s overlaps the status bar", tab.Name)
			}
		})
	}
}

// TestLayoutTabToleratesFewerWidgetsThanExpected pins the bounds guard: every
// layout previously indexed a fixed number of widgets and panicked on any tab
// holding fewer.
func TestLayoutTabToleratesFewerWidgetsThanExpected(t *testing.T) {
	app := newTestApp(t)
	rect := image.Rect(0, 0, 100, 30)

	for _, name := range []string{"System", "HTTP", "Git", "History", "Notifications", "Plugins", "Unknown"} {
		for widgetCount := 0; widgetCount <= 6; widgetCount++ {
			t.Run(name, func(t *testing.T) {
				tab := &Tab{Name: name}
				for i := 0; i < widgetCount; i++ {
					tab.Widgets = append(tab.Widgets, widgets.NewParagraph())
				}

				assert.NotPanics(t, func() {
					got := app.layoutTab(tab, rect)
					assert.LessOrEqual(t, len(got), widgetCount)
				}, "%s with %d widgets panicked", name, widgetCount)
			})
		}
	}
}

// TestLayoutTabRejectsDegenerateRect ensures a collapsed terminal produces no
// drawables rather than zero-area widgets.
func TestLayoutTabRejectsDegenerateRect(t *testing.T) {
	app := newTestApp(t)
	tab := &Tab{Name: "System", Widgets: []ui.Drawable{widgets.NewParagraph()}}

	for _, rect := range []image.Rectangle{
		image.Rect(0, 0, 0, 0),
		image.Rect(0, 0, 1, 1),
		image.Rect(0, 0, 100, 1),
	} {
		assert.Empty(t, app.layoutTab(tab, rect))
	}
}

func TestSplitRowsCoversRectangleExactly(t *testing.T) {
	rect := image.Rect(0, 0, 80, 37)
	parts := splitRows(rect, 0.2, 0.3, 0.5)

	require.Len(t, parts, 3)
	assert.Equal(t, rect.Min.Y, parts[0].Min.Y)
	assert.Equal(t, rect.Max.Y, parts[2].Max.Y, "the final row must absorb the rounding remainder")

	for i := 1; i < len(parts); i++ {
		assert.Equal(t, parts[i-1].Max.Y, parts[i].Min.Y, "rows must be contiguous")
	}
}

func TestSplitColsCoversRectangleExactly(t *testing.T) {
	rect := image.Rect(0, 0, 81, 20)
	parts := splitCols(rect, 1, 1)

	require.Len(t, parts, 2)
	assert.Equal(t, rect.Min.X, parts[0].Min.X)
	assert.Equal(t, rect.Max.X, parts[1].Max.X)
	assert.Equal(t, parts[0].Max.X, parts[1].Min.X)
}

func TestSplitHandlesDegenerateInput(t *testing.T) {
	assert.Empty(t, splitRows(image.Rect(0, 0, 10, 10)))
	assert.Len(t, splitRows(image.Rect(0, 0, 10, 0), 1, 1), 2)
	assert.Len(t, splitRows(image.Rect(0, 0, 10, 10), 0, 0), 2)
}

func TestTerminalTooSmall(t *testing.T) {
	app := newTestApp(t)

	tests := []struct {
		name          string
		width, height int
		want          bool
	}{
		{"usable", 120, 40, false},
		{"narrow", 10, 40, true},
		{"short", 120, 3, true},
		{"minimum usable", minContentWidth, minContentHeight + statusBarHeight + tabBarHeight, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app.TermWidth, app.TermHeight = tt.width, tt.height
			assert.Equal(t, tt.want, app.terminalTooSmall())
		})
	}
}

// TestClampedRangeIndexHandlesCustomRange covers the zoom path, which sets the
// index to -1 to mark a window that matches no preset.
func TestClampedRangeIndexHandlesCustomRange(t *testing.T) {
	app := newTestApp(t)

	app.HistoryRangeIdx = -1
	assert.Equal(t, 0, app.clampedRangeIndex())

	app.HistoryRangeIdx = len(historyRanges) + 5
	assert.Equal(t, len(historyRanges)-1, app.clampedRangeIndex())

	app.HistoryRangeIdx = 2
	assert.Equal(t, 2, app.clampedRangeIndex())
}

// TestDefaultHistoryRangeIsConsistent guards against the constant and the value
// drifting apart, which previously left the app claiming 24h while highlighting
// the 12h entry.
func TestDefaultHistoryRangeIsConsistent(t *testing.T) {
	app := NewApp(config.DefaultConfig())

	require.Less(t, defaultHistoryRangeIndex, len(historyRanges))
	assert.Equal(t, historyRanges[defaultHistoryRangeIndex].Value, app.HistoryRange)
	assert.Equal(t, defaultHistoryRangeIndex, app.HistoryRangeIdx)
	assert.Equal(t, "24h", historyRanges[app.HistoryRangeIdx].Label)
}

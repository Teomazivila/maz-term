package ui

import (
	"testing"
	"time"

	"github.com/Teomazivila/maz-term/pkg/config"
	ui "github.com/gizak/termui/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func key(id string) ui.Event {
	return ui.Event{ID: id, Type: ui.KeyboardEvent}
}

// TestAnnotationFormAccumulatesInput pins the defect where the draft lived in a
// function-local struct and was reset on every keystroke, so the title could
// never exceed one character.
func TestAnnotationFormAccumulatesInput(t *testing.T) {
	app := newTestApp(t)
	app.beginAnnotation()

	require.True(t, app.addingAnnotation)

	for _, k := range []string{"d", "e", "p", "l", "o", "y"} {
		app.handleEvent(key(k))
	}
	assert.Equal(t, "deploy", app.annotation.Title, "title must accumulate across keystrokes")

	app.handleEvent(key("<Space>"))
	app.handleEvent(key("v"))
	assert.Equal(t, "deploy v", app.annotation.Title)

	app.handleEvent(key("<Backspace>"))
	assert.Equal(t, "deploy ", app.annotation.Title)

	// Enter advances to the description field rather than submitting.
	app.handleEvent(key("<Enter>"))
	require.Equal(t, 1, app.annotation.Field)
	assert.True(t, app.addingAnnotation)

	for _, k := range []string{"o", "k"} {
		app.handleEvent(key(k))
	}
	assert.Equal(t, "ok", app.annotation.Description)
}

// TestAnnotationFormRequiresTitle checks the empty-title guard.
func TestAnnotationFormRequiresTitle(t *testing.T) {
	app := newTestApp(t)
	app.beginAnnotation()

	app.handleEvent(key("<Enter>"))

	assert.Equal(t, 0, app.annotation.Field, "must stay on the title field")
	assert.True(t, app.addingAnnotation)
}

func TestAnnotationFormCancels(t *testing.T) {
	app := newTestApp(t)
	app.beginAnnotation()
	app.handleEvent(key("x"))

	app.handleEvent(key("<Escape>"))

	assert.False(t, app.addingAnnotation)
	assert.Empty(t, app.annotation.Title, "the draft must be discarded on cancel")
}

// TestAnnotationModeSwallowsGlobalKeys checks that typing "q" into the form does
// not quit the application.
func TestAnnotationModeSwallowsGlobalKeys(t *testing.T) {
	app := newTestApp(t)
	app.beginAnnotation()

	app.handleEvent(key("q"))

	assert.False(t, app.quit, "q must be text while the form is open")
	assert.Equal(t, "q", app.annotation.Title)
}

// TestHistoryRangeKeysChangeTheWindow pins the defect where [ and ] moved the
// index without ever assigning HistoryRange, so the window never changed.
func TestHistoryRangeKeysChangeTheWindow(t *testing.T) {
	app := newTestApp(t)
	app.selectTabByName(t, "History")

	start := app.HistoryRange
	startIdx := app.HistoryRangeIdx

	app.handleEvent(key("["))
	assert.Equal(t, startIdx-1, app.HistoryRangeIdx)
	assert.Equal(t, historyRanges[startIdx-1].Value, app.HistoryRange)
	assert.NotEqual(t, start, app.HistoryRange, "the range must actually change")

	app.handleEvent(key("]"))
	assert.Equal(t, startIdx, app.HistoryRangeIdx)
	assert.Equal(t, start, app.HistoryRange)
}

func TestHistoryRangeKeysClampAtBounds(t *testing.T) {
	app := newTestApp(t)
	app.selectTabByName(t, "History")

	for range len(historyRanges) + 3 {
		app.handleEvent(key("["))
	}
	assert.Equal(t, 0, app.HistoryRangeIdx)
	assert.Equal(t, historyRanges[0].Value, app.HistoryRange)

	for range len(historyRanges) + 3 {
		app.handleEvent(key("]"))
	}
	assert.Equal(t, len(historyRanges)-1, app.HistoryRangeIdx)
	assert.Equal(t, historyRanges[len(historyRanges)-1].Value, app.HistoryRange)
}

// TestHistoryRangeKeysOnlyApplyToHistoryTab confirms the keys do not leak.
func TestHistoryRangeKeysOnlyApplyToHistoryTab(t *testing.T) {
	app := newTestApp(t)
	app.selectTabByName(t, "System")

	before := app.HistoryRange
	app.handleEvent(key("["))

	assert.Equal(t, before, app.HistoryRange)
}

func TestQuitKeys(t *testing.T) {
	for _, id := range []string{"q", "<C-c>"} {
		app := newTestApp(t)
		app.handleEvent(key(id))
		assert.True(t, app.quit, "%s must quit", id)
	}
}

// TestResizeWithWrongPayloadDoesNotPanic pins the unchecked type assertion.
func TestResizeWithWrongPayloadDoesNotPanic(t *testing.T) {
	app := newTestApp(t)

	assert.NotPanics(t, func() {
		app.handleEvent(ui.Event{ID: "<Resize>", Payload: "not a resize"})
	})

	app.handleEvent(ui.Event{ID: "<Resize>", Payload: ui.Resize{Width: 90, Height: 30}})
	assert.Equal(t, 90, app.TermWidth)
	assert.Equal(t, 30, app.TermHeight)
}

func TestTabNavigationWraps(t *testing.T) {
	app := newTestApp(t)
	require.GreaterOrEqual(t, len(app.Tabs), 2)

	app.ActiveTabIndex = 0
	app.changeTab(-1)
	assert.Equal(t, len(app.Tabs)-1, app.ActiveTabIndex, "must wrap backwards")

	app.changeTab(1)
	assert.Equal(t, 0, app.ActiveTabIndex, "must wrap forwards")
}

func TestSelectTabIgnoresOutOfRange(t *testing.T) {
	app := newTestApp(t)
	app.ActiveTabIndex = 1

	app.selectTab(-1)
	assert.Equal(t, 1, app.ActiveTabIndex)

	app.selectTab(len(app.Tabs) + 10)
	assert.Equal(t, 1, app.ActiveTabIndex)
}

// TestDigitKeysSelectTabsNotHistoryRanges documents the resolved conflict: the
// in-app help once told operators to press 1-7 to pick a history range while
// those keys switched tabs.
func TestDigitKeysSelectTabsNotHistoryRanges(t *testing.T) {
	app := newTestApp(t)
	app.selectTabByName(t, "History")
	before := app.HistoryRange

	app.handleEvent(key("2"))

	assert.Equal(t, 1, app.ActiveTabIndex, "digits select tabs")
	assert.Equal(t, before, app.HistoryRange, "digits must not change the history range")
}

func TestExportWithoutStorageReportsClearly(t *testing.T) {
	app := newTestApp(t)
	app.Storage = nil

	app.exportData()

	assert.Contains(t, app.statusMessage, "storage")
}

func TestApplyZoomSetsCustomRange(t *testing.T) {
	app := newTestApp(t)
	app.selectTabByName(t, "History")

	app.enterZoomMode()
	require.True(t, app.ZoomMode)

	app.handleEvent(key("<Enter>"))

	assert.False(t, app.ZoomMode)
	assert.Equal(t, -1, app.HistoryRangeIdx, "a zoomed window matches no preset")
	assert.Positive(t, app.HistoryRange)
	assert.Less(t, app.HistoryRange, historyRanges[defaultHistoryRangeIndex].Value)
}

func TestZoomModeCancels(t *testing.T) {
	app := newTestApp(t)
	app.selectTabByName(t, "History")
	app.enterZoomMode()

	app.handleEvent(key("<Escape>"))

	assert.False(t, app.ZoomMode)
}

func TestHelpTextMatchesImplementedBindings(t *testing.T) {
	app := newTestApp(t)
	help := app.helpText()

	// Each documented key must be handled somewhere, so the help cannot drift
	// from behaviour the way the previous static panel had.
	for _, binding := range []string{"?", "r", "e", "q", "[", "]", "a", "A", "z", "m", "D", "C", "d", "f", "o"} {
		assert.Contains(t, help, binding, "help must document %q", binding)
	}

	assert.NotContains(t, help, "1-7", "history ranges are selected with [ and ], not digits")
	assert.NotContains(t, help, "h: Toggle", "h moves to the previous tab")
}

func TestSelectedHelper(t *testing.T) {
	order := []string{"a", "b", "c"}
	chosen := map[string]bool{"c": true, "a": true, "b": false}

	assert.Equal(t, []string{"a", "c"}, selected(chosen, order), "must follow the given order")
	assert.Nil(t, selected(map[string]bool{}, order))
}

func TestFormatTimeZeroValue(t *testing.T) {
	assert.Equal(t, "-", formatTime(time.Time{}))
	assert.NotEqual(t, "-", formatTime(time.Now()))
}

// selectTabByName activates a tab by name, failing the test when absent.
func (a *App) selectTabByName(t *testing.T, name string) {
	t.Helper()

	for i, tab := range a.Tabs {
		if tab.Name == name {
			a.ActiveTabIndex = i
			return
		}
	}
	t.Fatalf("tab %q not found", name)
}

var _ = config.DefaultConfig

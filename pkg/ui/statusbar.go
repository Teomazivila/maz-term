package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// StatusItem represents an item in the status bar
type StatusItem struct {
	Key   string
	Value string
	Style lipgloss.Style
}

// StatusBar is a component that renders a status bar
type StatusBar struct {
	items      []StatusItem
	width      int
	clock      bool
	keyStyle   lipgloss.Style
	valueStyle lipgloss.Style
}

// NewStatusBar creates a new status bar
func NewStatusBar(showClock bool) *StatusBar {
	return &StatusBar{
		items:      []StatusItem{},
		clock:      showClock,
		keyStyle:   lipgloss.NewStyle().Bold(true),
		valueStyle: lipgloss.NewStyle(),
	}
}

// SetWidth sets the width of the status bar
func (s *StatusBar) SetWidth(width int) {
	s.width = width
}

// AddItem adds a new item to the status bar
func (s *StatusBar) AddItem(key, value string, style lipgloss.Style) {
	s.items = append(s.items, StatusItem{
		Key:   key,
		Value: value,
		Style: style,
	})
}

// UpdateItem updates an existing item in the status bar
func (s *StatusBar) UpdateItem(key, value string) {
	for i, item := range s.items {
		if item.Key == key {
			s.items[i].Value = value
			return
		}
	}

	// Item not found, add it
	s.AddItem(key, value, s.valueStyle)
}

// UpdateItemWithStyle updates an existing item in the status bar with a specific style
func (s *StatusBar) UpdateItemWithStyle(key, value string, style lipgloss.Style) {
	for i, item := range s.items {
		if item.Key == key {
			s.items[i].Value = value
			s.items[i].Style = style
			return
		}
	}

	// Item not found, add it
	s.AddItem(key, value, style)
}

// RemoveItem removes an item from the status bar
func (s *StatusBar) RemoveItem(key string) {
	for i, item := range s.items {
		if item.Key == key {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return
		}
	}
}

// Render renders the status bar
func (s *StatusBar) Render() string {
	// Create a base style for the status bar
	statusStyle := Theme.StatusBar.Copy().Width(s.width)

	// Render items
	renderedItems := []string{}

	for _, item := range s.items {
		key := s.keyStyle.Render(item.Key)
		value := item.Style.Render(item.Value)
		renderedItems = append(renderedItems, fmt.Sprintf("%s: %s", key, value))
	}

	// Add clock if enabled
	if s.clock {
		timeStr := time.Now().Format("15:04:05")
		renderedItems = append(renderedItems, s.keyStyle.Render("Time: ")+timeStr)
	}

	// Join items with separator
	content := lipgloss.JoinHorizontal(lipgloss.Top, renderedItems...)

	// Render the status bar
	return statusStyle.Render(content)
}

// Update updates the status bar (e.g., clock)
func (s *StatusBar) Update() {
	// This is called periodically to update the status bar
	// Currently, only the clock needs updating which happens in Render()
}

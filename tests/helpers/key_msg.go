package helpers

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Key constants for testing
const (
	KeyLeft  = tea.KeyLeft
	KeyRight = tea.KeyRight
	KeyTab   = tea.KeyTab
)

// CreateKeyMsg creates a tea.KeyMsg for testing
func CreateKeyMsg(key string) tea.KeyMsg {
	switch key {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return tea.KeyMsg{Runes: []rune{rune(key[0])}}
	case "left":
		return tea.KeyMsg{Type: KeyLeft}
	case "right":
		return tea.KeyMsg{Type: KeyRight}
	case "tab":
		return tea.KeyMsg{Type: KeyTab}
	default:
		return tea.KeyMsg{Runes: []rune(key)}
	}
}

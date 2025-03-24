package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// KeyBinding represents a keyboard shortcut
type KeyBinding struct {
	Key         string
	Description string
	Action      func() tea.Cmd
}

// KeyMap contains all keybindings for the application
type KeyMap struct {
	Global   []KeyBinding
	TabLevel map[string][]KeyBinding
}

// NewKeyMap creates a new keymap
func NewKeyMap() KeyMap {
	return KeyMap{
		Global:   []KeyBinding{},
		TabLevel: make(map[string][]KeyBinding),
	}
}

// AddGlobalBinding adds a global keyboard shortcut
func (k *KeyMap) AddGlobalBinding(key, description string, action func() tea.Cmd) {
	k.Global = append(k.Global, KeyBinding{
		Key:         key,
		Description: description,
		Action:      action,
	})
}

// AddTabBinding adds a tab-specific keyboard shortcut
func (k *KeyMap) AddTabBinding(tab, key, description string, action func() tea.Cmd) {
	if _, ok := k.TabLevel[tab]; !ok {
		k.TabLevel[tab] = []KeyBinding{}
	}

	k.TabLevel[tab] = append(k.TabLevel[tab], KeyBinding{
		Key:         key,
		Description: description,
		Action:      action,
	})
}

// HandleKeyPress handles a key press
func (k *KeyMap) HandleKeyPress(msg tea.KeyMsg, activeTab string) (bool, tea.Cmd) {
	// Convert key to string
	key := msg.String()

	// Check global bindings first
	for _, binding := range k.Global {
		if binding.Key == key {
			return true, binding.Action()
		}
	}

	// Check tab-specific bindings
	if bindings, ok := k.TabLevel[activeTab]; ok {
		for _, binding := range bindings {
			if binding.Key == key {
				return true, binding.Action()
			}
		}
	}

	return false, nil
}

// RenderHelpView renders a help view with all keybindings
func (k *KeyMap) RenderHelpView(width, height int, activeTab string) string {
	// Collect all relevant keybindings
	allBindings := []KeyBinding{}

	// Add global bindings
	for _, binding := range k.Global {
		allBindings = append(allBindings, binding)
	}

	// Add tab-specific bindings if available
	if bindings, ok := k.TabLevel[activeTab]; ok {
		allBindings = append(allBindings, bindings...)
	}

	// If no bindings, return a message
	if len(allBindings) == 0 {
		return "No keyboard shortcuts available."
	}

	// Create two columns: key and description
	keys := []string{}
	descriptions := []string{}

	// Find the maximum length of key for alignment
	maxKeyLen := 0
	for _, binding := range allBindings {
		if len(binding.Key) > maxKeyLen {
			maxKeyLen = len(binding.Key)
		}
	}

	// Format keys and descriptions
	for _, binding := range allBindings {
		keys = append(keys, fmt.Sprintf("%-*s", maxKeyLen, binding.Key))
		descriptions = append(descriptions, binding.Description)
	}

	// Join columns
	lines := []string{}
	for i := 0; i < len(keys); i++ {
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left,
			Theme.Selected.Render(keys[i]),
			"  ",
			Theme.Text.Render(descriptions[i]),
		))
	}

	// Create help panel
	title := Theme.Title.Render("Keyboard Shortcuts")
	content := strings.Join(lines, "\n")

	// Render the help panel with lipgloss
	panel := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2).
		Width(width - 4).
		Height(height - 4).
		Render(title + "\n\n" + content)

	return panel
}

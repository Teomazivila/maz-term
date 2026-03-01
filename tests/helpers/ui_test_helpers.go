package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockTermDimensions represents mocked terminal dimensions
type MockTermDimensions struct {
	Width  int
	Height int
}

// NewMockTermDimensions creates a new mock terminal dimensions
func NewMockTermDimensions(width, height int) *MockTermDimensions {
	return &MockTermDimensions{
		Width:  width,
		Height: height,
	}
}

// GetDimensions returns the mock dimensions
func (m *MockTermDimensions) GetDimensions() (int, int) {
	return m.Width, m.Height
}

// AssertUIComponents performs assertions on UI components
func AssertUIComponents(t *testing.T, componentsCount, expectedCount int) {
	assert.Equal(t, expectedCount, componentsCount, "Expected %d components but got %d", expectedCount, componentsCount)
}

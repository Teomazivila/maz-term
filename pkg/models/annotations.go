package models

import "time"

// EventType represents the type of system event
type EventType string

const (
	// Event types
	EventTypeDeployment  EventType = "deployment"
	EventTypeRestart     EventType = "restart"
	EventTypeAlert       EventType = "alert"
	EventTypeIncident    EventType = "incident"
	EventTypeMaintenance EventType = "maintenance"
	EventTypeOther       EventType = "other"
)

// EventSeverity represents the severity level of an event
type EventSeverity string

const (
	// Severity levels
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityCritical EventSeverity = "critical"
)

// EventAnnotation represents a significant event to annotate on metrics charts
type EventAnnotation struct {
	ID          string        `json:"id"`          // Unique identifier
	Timestamp   time.Time     `json:"timestamp"`   // When the event occurred
	Title       string        `json:"title"`       // Short title for the event
	Description string        `json:"description"` // Detailed description
	Type        EventType     `json:"type"`        // Type of event
	Severity    EventSeverity `json:"severity"`    // Severity level
	Source      string        `json:"source"`      // Source system that generated the event
	Tags        []string      `json:"tags"`        // Tags for categorization
}

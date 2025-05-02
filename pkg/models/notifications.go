package models

import (
	"time"
)

// NotificationSeverity represents the severity level of a notification
type NotificationSeverity string

const (
	// NotificationInfo indicates an informational notification
	NotificationInfo NotificationSeverity = "info"

	// NotificationWarning indicates a warning notification
	NotificationWarning NotificationSeverity = "warning"

	// NotificationCritical indicates a critical notification
	NotificationCritical NotificationSeverity = "critical"
)

// NotificationSource represents the source of a notification
type NotificationSource string

const (
	// SourceSystem indicates a system-generated notification
	SourceSystem NotificationSource = "system"

	// SourceHTTP indicates an HTTP endpoint-related notification
	SourceHTTP NotificationSource = "http"

	// SourceGit indicates a Git repository-related notification
	SourceGit NotificationSource = "git"

	// SourceCloud indicates a cloud provider-related notification
	SourceCloud NotificationSource = "cloud"

	// SourceCI indicates a CI/CD pipeline-related notification
	SourceCI NotificationSource = "ci"
)

// Notification represents a notification in the system
type Notification struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Message     string               `json:"message"`
	Timestamp   time.Time            `json:"timestamp"`
	Severity    NotificationSeverity `json:"severity"`
	Source      NotificationSource   `json:"source"`
	Read        bool                 `json:"read"`
	Dismissed   bool                 `json:"dismissed"`
	ActionURL   string               `json:"action_url,omitempty"`
	ActionLabel string               `json:"action_label,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
}

// NotificationManager interface for handling notifications
type NotificationManager interface {
	AddNotification(notification Notification) error
	GetNotifications(count int, includeRead bool) ([]Notification, error)
	MarkAsRead(id string) error
	DismissNotification(id string) error
	ClearAllNotifications() error
}

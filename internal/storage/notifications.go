package storage

import (
	"fmt"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/google/uuid"
)

// InitNotificationsSchema initializes the notifications schema
func (d *Database) InitNotificationsSchema() error {
	// Create the notifications table
	_, err := d.db.Exec(`
	CREATE TABLE IF NOT EXISTS notifications (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		message TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		severity TEXT NOT NULL,
		source TEXT NOT NULL,
		read INTEGER NOT NULL DEFAULT 0,
		dismissed INTEGER NOT NULL DEFAULT 0,
		action_url TEXT,
		action_label TEXT,
		tags TEXT
	)`)
	if err != nil {
		return fmt.Errorf("failed to create notifications table: %w", err)
	}

	// Create index for timestamp
	_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_notifications_timestamp ON notifications(timestamp)`)
	if err != nil {
		return fmt.Errorf("failed to create notifications index: %w", err)
	}

	return nil
}

// AddNotification adds a new notification to the database
func (d *Database) AddNotification(notification models.Notification) error {
	// Generate ID if not provided
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	// Convert tags to string
	tags := ""
	if len(notification.Tags) > 0 {
		tags = fmt.Sprintf("%v", notification.Tags)
	}

	// Insert notification
	_, err := d.db.Exec(
		`INSERT INTO notifications (
			id, title, message, timestamp, severity, source, read, dismissed, action_url, action_label, tags
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		notification.ID,
		notification.Title,
		notification.Message,
		notification.Timestamp.Unix(),
		notification.Severity,
		notification.Source,
		boolToInt(notification.Read),
		boolToInt(notification.Dismissed),
		notification.ActionURL,
		notification.ActionLabel,
		tags,
	)

	if err != nil {
		return fmt.Errorf("failed to insert notification: %w", err)
	}

	return nil
}

// GetNotifications retrieves notifications from the database
func (d *Database) GetNotifications(count int, includeRead bool) ([]models.Notification, error) {
	// Prepare query
	query := `
		SELECT id, title, message, timestamp, severity, source, read, dismissed, action_url, action_label, tags
		FROM notifications
		WHERE dismissed = 0
	`

	// Add read filter if needed
	if !includeRead {
		query += " AND read = 0"
	}

	// Add order and limit
	query += " ORDER BY timestamp DESC"
	if count > 0 {
		query += fmt.Sprintf(" LIMIT %d", count)
	}

	// Execute query
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer rows.Close()

	// Parse results
	notifications := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		var timestamp int64
		var read, dismissed int
		var tags string

		err := rows.Scan(
			&n.ID,
			&n.Title,
			&n.Message,
			&timestamp,
			&n.Severity,
			&n.Source,
			&read,
			&dismissed,
			&n.ActionURL,
			&n.ActionLabel,
			&tags,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification row: %w", err)
		}

		// Convert values
		n.Timestamp = time.Unix(timestamp, 0)
		n.Read = intToBool(read)
		n.Dismissed = intToBool(dismissed)

		// Parse tags if not empty
		if tags != "" {
			n.Tags = parseTags(tags)
		}

		notifications = append(notifications, n)
	}

	return notifications, nil
}

// MarkAsRead marks a notification as read
func (d *Database) MarkAsRead(id string) error {
	_, err := d.db.Exec("UPDATE notifications SET read = 1 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}
	return nil
}

// DismissNotification marks a notification as dismissed
func (d *Database) DismissNotification(id string) error {
	_, err := d.db.Exec("UPDATE notifications SET dismissed = 1 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to dismiss notification: %w", err)
	}
	return nil
}

// ClearAllNotifications marks all notifications as dismissed
func (d *Database) ClearAllNotifications() error {
	_, err := d.db.Exec("UPDATE notifications SET dismissed = 1")
	if err != nil {
		return fmt.Errorf("failed to clear all notifications: %w", err)
	}
	return nil
}

// GetUnreadNotificationCount returns the count of unread notifications
func (d *Database) GetUnreadNotificationCount() (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM notifications WHERE read = 0 AND dismissed = 0").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get unread notification count: %w", err)
	}
	return count, nil
}

// Helper functions
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}

func parseTags(tagsStr string) []string {
	// Simple implementation - in a real system you'd want better parsing
	tagsStr = strings.TrimPrefix(tagsStr, "[")
	tagsStr = strings.TrimSuffix(tagsStr, "]")
	if tagsStr == "" {
		return []string{}
	}
	return strings.Split(tagsStr, " ")
}

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Teomazivila/maz-term/pkg/models"
	"github.com/google/uuid"
)

// notificationColumns is the shared projection for every notification read, so
// the scan helper and the queries cannot drift apart.
const notificationColumns = `id, title, message, timestamp, severity, source, read, dismissed, action_url, action_label, tags`

// InitNotificationsSchema initializes the notifications schema.
func (d *Database) InitNotificationsSchema() error {
	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	// action_url and action_label default to '' rather than being nullable, so
	// reads can scan straight into a string without a sql.NullString dance.
	if _, err := d.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS notifications (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		message TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		severity TEXT NOT NULL,
		source TEXT NOT NULL,
		read INTEGER NOT NULL DEFAULT 0,
		dismissed INTEGER NOT NULL DEFAULT 0,
		action_url TEXT NOT NULL DEFAULT '',
		action_label TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]'
	)`); err != nil {
		return fmt.Errorf("failed to create notifications table: %w", err)
	}

	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_notifications_timestamp ON notifications(timestamp)",
		// Matches the (dismissed, read) predicate every list query applies
		// before ordering by time.
		"CREATE INDEX IF NOT EXISTS idx_notifications_state_timestamp ON notifications(dismissed, read, timestamp)",
	}
	for _, stmt := range indexes {
		if _, err := d.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to create notifications index: %w", err)
		}
	}

	return nil
}

// AddNotification adds a new notification to the database, generating an ID and
// timestamp when they are not supplied.
func (d *Database) AddNotification(notification models.Notification) error {
	if notification.Title == "" {
		return errors.New("storage: notification title is required")
	}
	if notification.ID == "" {
		notification.ID = uuid.New().String()
	}
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	// Tags are stored as JSON. The previous format was fmt.Sprintf("%v", tags),
	// which produced "[a b c]" and could not round-trip a tag containing a
	// space.
	tags, err := json.Marshal(notification.Tags)
	if err != nil {
		return fmt.Errorf("encoding tags for notification %s: %w", notification.ID, err)
	}

	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	if _, err := d.db.ExecContext(ctx, `
	INSERT INTO notifications (`+notificationColumns+`)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		notification.ID,
		notification.Title,
		notification.Message,
		notification.Timestamp.Unix(),
		string(notification.Severity),
		string(notification.Source),
		notification.Read,
		notification.Dismissed,
		notification.ActionURL,
		notification.ActionLabel,
		string(tags),
	); err != nil {
		return fmt.Errorf("inserting notification %s: %w", notification.ID, err)
	}

	return nil
}

// GetNotifications retrieves the most recent undismissed notifications.
func (d *Database) GetNotifications(count int, includeRead bool) ([]models.Notification, error) {
	return d.GetFilteredNotifications(count, includeRead, nil, nil)
}

// GetFilteredNotifications retrieves undismissed notifications, optionally
// restricted to the given sources and severities.
//
// Both filters are applied in SQL with bound parameters. An earlier revision
// interpolated the values directly into an IN clause, which broke on any value
// containing a quote and allowed statement injection from notification data.
// The storage adapter additionally re-filtered in Go over a bounded row window,
// which silently hid matches beyond it.
func (d *Database) GetFilteredNotifications(count int, includeRead bool, sources, severities []string) ([]models.Notification, error) {
	var (
		conditions = []string{"dismissed = 0"}
		args       []any
	)

	if !includeRead {
		conditions = append(conditions, "read = 0")
	}
	if clause, values := inClause("source", sources); clause != "" {
		conditions = append(conditions, clause)
		args = append(args, values...)
	}
	if clause, values := inClause("severity", severities); clause != "" {
		conditions = append(conditions, clause)
		args = append(args, values...)
	}

	query := `SELECT ` + notificationColumns + ` FROM notifications WHERE ` +
		strings.Join(conditions, " AND ") + ` ORDER BY timestamp DESC`

	if count > 0 {
		query += " LIMIT ?"
		args = append(args, count)
	}

	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying notifications: %w", err)
	}
	defer rows.Close()

	return scanNotifications(rows)
}

// inClause builds a parameterised IN predicate for column, returning an empty
// clause when there are no values to match.
func inClause(column string, values []string) (string, []any) {
	if len(values) == 0 {
		return "", nil
	}

	args := make([]any, 0, len(values))
	for _, v := range values {
		args = append(args, v)
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
	return fmt.Sprintf("%s IN (%s)", column, placeholders), args
}

// scanNotifications reads a notification result set.
func scanNotifications(rows *sql.Rows) ([]models.Notification, error) {
	notifications := []models.Notification{}

	for rows.Next() {
		var (
			n         models.Notification
			timestamp int64
			tagsJSON  string
		)

		if err := rows.Scan(
			&n.ID,
			&n.Title,
			&n.Message,
			&timestamp,
			&n.Severity,
			&n.Source,
			&n.Read,
			&n.Dismissed,
			&n.ActionURL,
			&n.ActionLabel,
			&tagsJSON,
		); err != nil {
			return nil, fmt.Errorf("scanning notification: %w", err)
		}

		n.Timestamp = time.Unix(timestamp, 0)
		if tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &n.Tags); err != nil {
				return nil, fmt.Errorf("decoding tags for notification %s: %w", n.ID, err)
			}
		}

		notifications = append(notifications, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notifications: %w", err)
	}

	return notifications, nil
}

// MarkAsRead marks a notification as read. It returns an error wrapping
// ErrNotFound when no notification has the given ID.
func (d *Database) MarkAsRead(id string) error {
	return d.updateNotificationFlag(id, "read")
}

// DismissNotification marks a notification as dismissed. It returns an error
// wrapping ErrNotFound when no notification has the given ID.
func (d *Database) DismissNotification(id string) error {
	return d.updateNotificationFlag(id, "dismissed")
}

// updateNotificationFlag sets one boolean column on a single notification.
// column is supplied only by this package's own callers, never by user input.
func (d *Database) updateNotificationFlag(id, column string) error {
	if id == "" {
		return errors.New("storage: notification ID is required")
	}

	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	result, err := d.db.ExecContext(ctx,
		fmt.Sprintf("UPDATE notifications SET %s = 1 WHERE id = ?", column), id)
	if err != nil {
		return fmt.Errorf("updating %s on notification %s: %w", column, id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("counting updated notifications: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("notification %s: %w", id, ErrNotFound)
	}

	return nil
}

// ClearAllNotifications marks every notification as dismissed and reports how
// many were affected.
func (d *Database) ClearAllNotifications() error {
	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	result, err := d.db.ExecContext(ctx, "UPDATE notifications SET dismissed = 1 WHERE dismissed = 0")
	if err != nil {
		return fmt.Errorf("clearing notifications: %w", err)
	}

	if affected, err := result.RowsAffected(); err == nil {
		d.logger.Debug("notifications cleared", "count", affected)
	}

	return nil
}

// GetUnreadNotificationCount returns the number of unread, undismissed
// notifications.
func (d *Database) GetUnreadNotificationCount() (int, error) {
	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	var count int
	if err := d.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notifications WHERE read = 0 AND dismissed = 0").Scan(&count); err != nil {
		return 0, fmt.Errorf("counting unread notifications: %w", err)
	}

	return count, nil
}

// GetNotificationSources returns the distinct sources present in the store, for
// populating the filter UI.
func (d *Database) GetNotificationSources() ([]string, error) {
	return d.distinctNotificationColumn("source")
}

// GetNotificationSeverities returns the distinct severities present in the
// store, for populating the filter UI.
func (d *Database) GetNotificationSeverities() ([]string, error) {
	return d.distinctNotificationColumn("severity")
}

// distinctNotificationColumn returns the distinct non-empty values of one
// column. column is supplied only by this package's own callers.
func (d *Database) distinctNotificationColumn(column string) ([]string, error) {
	ctx, cancel := context.WithTimeout(d.ctx, queryTimeout)
	defer cancel()

	rows, err := d.db.QueryContext(ctx, fmt.Sprintf(
		"SELECT DISTINCT %s FROM notifications WHERE %s != '' ORDER BY %s", column, column, column))
	if err != nil {
		return nil, fmt.Errorf("listing notification %s values: %w", column, err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("scanning notification %s: %w", column, err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notification %s values: %w", column, err)
	}

	return values, nil
}

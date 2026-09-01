package models

import (
	"database/sql"
	"errors"
)

// Notification categories — one per automated reminder type a player can
// set a channel for.
const (
	CategoryRSVPReminder           = "rsvp_reminder"
	CategoryCaptainMessageReminder = "captain_message_reminder"
)

// Notification channels. ChannelEmail is the implicit default whenever a
// player has never set a preference for a category — today's
// email-everyone behavior, unchanged for anyone who never visits their
// notification settings.
const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"
	ChannelBoth  = "both"
	ChannelOff   = "off"
)

type NotificationPreferenceModel struct {
	DB *sql.DB
}

// GetChannel returns playerID's channel for category, defaulting to
// ChannelEmail if they've never set one.
func (m *NotificationPreferenceModel) GetChannel(playerID int, category string) (string, error) {
	var channel string
	stmt := `select channel from playerNotificationPreferences where playerID = ? and category = ?`
	err := m.DB.QueryRow(stmt, playerID, category).Scan(&channel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChannelEmail, nil
		}
		return "", err
	}
	return channel, nil
}

// SetChannel upserts playerID's channel for category.
func (m *NotificationPreferenceModel) SetChannel(playerID int, category, channel string) error {
	stmt := `insert into playerNotificationPreferences (playerID, category, channel) values (?, ?, ?)
		on duplicate key update channel = values(channel)`
	_, err := m.DB.Exec(stmt, playerID, category, channel)
	return err
}

// ListForPlayer returns every category playerID has an explicit
// preference row for, keyed by category — a category missing from the
// result should be treated as ChannelEmail.
func (m *NotificationPreferenceModel) ListForPlayer(playerID int) (map[string]string, error) {
	stmt := `select category, channel from playerNotificationPreferences where playerID = ?`
	rows, err := m.DB.Query(stmt, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefs := map[string]string{}
	for rows.Next() {
		var category, channel string
		if err := rows.Scan(&category, &channel); err != nil {
			return nil, err
		}
		prefs[category] = channel
	}
	return prefs, nil
}

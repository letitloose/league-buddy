package models

import (
	"database/sql"
	"time"
)

// MatchCaptainMessageReminder records that a team's captain was nudged to
// add a captain's message for a match, so MatchReminderService never
// re-sends the same one-time nudge twice.
type MatchCaptainMessageReminder struct {
	ID      int
	MatchID int
	TeamID  int
	SentAt  time.Time
}

type MatchCaptainMessageReminderModel struct {
	DB *sql.DB
}

// WasSent reports whether the captain's-message reminder for (matchID,
// teamID) has already gone out.
func (m *MatchCaptainMessageReminderModel) WasSent(matchID, teamID int) (bool, error) {
	stmt := `SELECT EXISTS(SELECT 1 FROM matchCaptainMessageReminders WHERE matchID = ? AND teamID = ?)`

	var exists bool
	err := m.DB.QueryRow(stmt, matchID, teamID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// MarkSent records that the captain's-message reminder for (matchID,
// teamID) was just sent.
func (m *MatchCaptainMessageReminderModel) MarkSent(matchID, teamID int) error {
	stmt := `INSERT INTO matchCaptainMessageReminders (matchID, teamID, sentAt) VALUES (?, ?, UTC_TIMESTAMP())`

	_, err := m.DB.Exec(stmt, matchID, teamID)
	return err
}

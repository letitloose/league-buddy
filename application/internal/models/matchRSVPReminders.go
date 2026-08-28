package models

import (
	"database/sql"
	"time"
)

// MatchRSVPReminder records that a player was reminded to RSVP for a match
// at a given days-out mark, so MatchReminderService never re-sends the same
// day's reminder twice (a restart, manual trigger, or retry is safe).
type MatchRSVPReminder struct {
	ID       int
	MatchID  int
	PlayerID int
	DaysOut  int
	SentAt   time.Time
}

type MatchRSVPReminderModel struct {
	DB *sql.DB
}

// WasSent reports whether daysOut's reminder for (matchID, playerID) has
// already gone out.
func (m *MatchRSVPReminderModel) WasSent(matchID, playerID, daysOut int) (bool, error) {
	stmt := `SELECT EXISTS(SELECT 1 FROM matchRSVPReminders WHERE matchID = ? AND playerID = ? AND daysOut = ?)`

	var exists bool
	err := m.DB.QueryRow(stmt, matchID, playerID, daysOut).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// MarkSent records that daysOut's reminder for (matchID, playerID) was just
// sent — called only after a successful send, so a failed send can still
// be retried on the next run.
func (m *MatchRSVPReminderModel) MarkSent(matchID, playerID, daysOut int) error {
	stmt := `INSERT INTO matchRSVPReminders (matchID, playerID, daysOut, sentAt) VALUES (?, ?, ?, UTC_TIMESTAMP())`

	_, err := m.DB.Exec(stmt, matchID, playerID, daysOut)
	return err
}

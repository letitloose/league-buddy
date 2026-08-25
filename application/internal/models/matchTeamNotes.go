package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// MatchTeamNote is one team's own Player of the Match designation and
// free-text captain's notes for one match — distinct from Match.Notes (a
// single shared note on the match record itself). Each team manages only
// its own row (see canManageMatchSide in cmd/web).
type MatchTeamNote struct {
	ID              int
	MatchID         int
	TeamID          int
	PlayerOfMatchID sql.NullInt32
	Notes           sql.NullString
	UpdatedAt       time.Time
}

type MatchTeamNoteModel struct {
	DB *sql.DB
}

// Upsert inserts note, or updates the existing (matchID, teamID) row if one
// already exists — a captain/scorekeeper can revise their pick or notes.
func (m *MatchTeamNoteModel) Upsert(note *MatchTeamNote) error {
	statement := `INSERT INTO matchTeamNotes (matchID, teamID, playerOfMatchID, notes, updatedAt) VALUES (?, ?, ?, ?, ?)`

	_, err := m.DB.Exec(statement, note.MatchID, note.TeamID, note.PlayerOfMatchID, note.Notes, note.UpdatedAt)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) && mySQLError.Number == 1062 {
			update := `UPDATE matchTeamNotes SET playerOfMatchID = ?, notes = ?, updatedAt = ? WHERE matchID = ? AND teamID = ?`
			_, err = m.DB.Exec(update, note.PlayerOfMatchID, note.Notes, note.UpdatedAt, note.MatchID, note.TeamID)
			return err
		}
		return err
	}
	return nil
}

// GetByMatchAndTeam returns teamID's note row for matchID, or ErrNoRecord if
// neither a Player of the Match nor notes has been set yet.
func (m *MatchTeamNoteModel) GetByMatchAndTeam(matchID, teamID int) (*MatchTeamNote, error) {
	stmt := `SELECT id, matchID, teamID, playerOfMatchID, notes, updatedAt FROM matchTeamNotes WHERE matchID = ? AND teamID = ?`

	note := &MatchTeamNote{}
	err := m.DB.QueryRow(stmt, matchID, teamID).Scan(&note.ID, &note.MatchID, &note.TeamID, &note.PlayerOfMatchID, &note.Notes, &note.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return note, nil
}

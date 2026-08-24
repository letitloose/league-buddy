package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// RSVP is one player's yes/no response (plus an optional message) to one
// match. TeamID is denormalized (not derived via a join to matches), same
// rationale as PlayerMatchStat.TeamID — it records which team the player
// was responding on behalf of for that specific match.
type RSVP struct {
	ID          int
	MatchID     int
	PlayerID    int
	TeamID      int
	Status      string // "yes" or "no"
	Message     sql.NullString
	RespondedAt time.Time
}

type RSVPModel struct {
	DB *sql.DB
}

// Upsert inserts rsvp, or updates the existing (matchID, playerID) row if
// one already exists — a player can change their mind and resubmit.
func (m *RSVPModel) Upsert(rsvp *RSVP) error {
	statement := `INSERT INTO rsvps (matchID, playerID, teamID, status, message, respondedAt) VALUES (?, ?, ?, ?, ?, ?)`

	_, err := m.DB.Exec(statement, rsvp.MatchID, rsvp.PlayerID, rsvp.TeamID, rsvp.Status, rsvp.Message, rsvp.RespondedAt)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) && mySQLError.Number == 1062 {
			update := `UPDATE rsvps SET teamID = ?, status = ?, message = ?, respondedAt = ? WHERE matchID = ? AND playerID = ?`
			_, err = m.DB.Exec(update, rsvp.TeamID, rsvp.Status, rsvp.Message, rsvp.RespondedAt, rsvp.MatchID, rsvp.PlayerID)
			return err
		}
		return err
	}
	return nil
}

// ListByMatch returns every RSVP recorded for matchID.
func (m *RSVPModel) ListByMatch(matchID int) ([]*RSVP, error) {
	stmt := `SELECT id, matchID, playerID, teamID, status, message, respondedAt FROM rsvps WHERE matchID = ?`

	rows, err := m.DB.Query(stmt, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rsvps := []*RSVP{}
	for rows.Next() {
		rsvp := &RSVP{}
		err := rows.Scan(&rsvp.ID, &rsvp.MatchID, &rsvp.PlayerID, &rsvp.TeamID, &rsvp.Status, &rsvp.Message, &rsvp.RespondedAt)
		if err != nil {
			return nil, err
		}
		rsvps = append(rsvps, rsvp)
	}
	return rsvps, nil
}

// GetByMatchAndPlayer returns playerID's RSVP for matchID, or ErrNoRecord if
// they haven't responded yet.
func (m *RSVPModel) GetByMatchAndPlayer(matchID, playerID int) (*RSVP, error) {
	stmt := `SELECT id, matchID, playerID, teamID, status, message, respondedAt FROM rsvps WHERE matchID = ? AND playerID = ?`

	rsvp := &RSVP{}
	err := m.DB.QueryRow(stmt, matchID, playerID).Scan(&rsvp.ID, &rsvp.MatchID, &rsvp.PlayerID, &rsvp.TeamID, &rsvp.Status, &rsvp.Message, &rsvp.RespondedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return rsvp, nil
}

// CountsByMatchAndTeam tallies matchID's "yes"/"no" responses among teamID's
// roster — the schedule table's in/out column.
func (m *RSVPModel) CountsByMatchAndTeam(matchID, teamID int) (in, out int, err error) {
	stmt := `SELECT status, COUNT(*) FROM rsvps WHERE matchID = ? AND teamID = ? GROUP BY status`

	rows, err := m.DB.Query(stmt, matchID, teamID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return 0, 0, err
		}
		if status == "yes" {
			in = count
		} else if status == "no" {
			out = count
		}
	}
	return in, out, nil
}

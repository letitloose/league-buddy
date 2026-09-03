package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// MatchAttendance is an explicit override of whether playerID attended
// matchID, for teamID's side — see the matchAttendance migration's comment
// for how this interacts with the RSVP-derived default.
type MatchAttendance struct {
	ID        int
	MatchID   int
	PlayerID  int
	TeamID    int
	Attended  bool
	UpdatedAt time.Time
}

type MatchAttendanceModel struct {
	DB *sql.DB
}

// Upsert inserts a, or updates the existing (matchID, playerID) row if one
// already exists — a captain/scorekeeper revising a prior mark.
func (m *MatchAttendanceModel) Upsert(a *MatchAttendance) error {
	statement := `INSERT INTO matchAttendance (matchID, playerID, teamID, attended, updatedAt) VALUES (?, ?, ?, ?, ?)`

	_, err := m.DB.Exec(statement, a.MatchID, a.PlayerID, a.TeamID, a.Attended, a.UpdatedAt)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) && mySQLError.Number == 1062 {
			update := `UPDATE matchAttendance SET teamID = ?, attended = ?, updatedAt = ? WHERE matchID = ? AND playerID = ?`
			_, err = m.DB.Exec(update, a.TeamID, a.Attended, a.UpdatedAt, a.MatchID, a.PlayerID)
			return err
		}
		return err
	}
	return nil
}

// ListByMatch returns every attendance override recorded for matchID (both
// teams) — the match view page's attendance section uses this to show
// which roster players have been explicitly marked.
func (m *MatchAttendanceModel) ListByMatch(matchID int) ([]*MatchAttendance, error) {
	stmt := `SELECT id, matchID, playerID, teamID, attended, updatedAt FROM matchAttendance WHERE matchID = ?`

	rows, err := m.DB.Query(stmt, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []*MatchAttendance{}
	for rows.Next() {
		a := &MatchAttendance{}
		err := rows.Scan(&a.ID, &a.MatchID, &a.PlayerID, &a.TeamID, &a.Attended, &a.UpdatedAt)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

// MatchesPlayedByTeamSeason returns, for teamID's players, how many of
// seasonID's already-played matches (MatchDate before asOf — pass the
// start of today, not a live instant, so a match happening later today
// doesn't count as played yet) they're considered to have attended: RSVP
// "yes" by default, or this table's value when an override exists for
// that (match, player) — a row here always wins over the RSVP-derived
// default, in either direction. Un-overridden matches with no RSVP at all
// count as not attended (no signal defaults to "didn't play," same as an
// explicit "no"). Only players with at least one attended match appear in
// the result.
func (m *MatchAttendanceModel) MatchesPlayedByTeamSeason(teamID, seasonID int, asOf time.Time) (map[int]int, error) {
	stmt := `SELECT playerID, COUNT(*) FROM (
			SELECT r.playerID AS playerID, (r.status = 'yes') AS attended
			FROM rsvps r
			JOIN matches m ON m.id = r.matchID
			WHERE m.seasonID = ? AND r.teamID = ? AND m.matchDate < ?
				AND NOT EXISTS (SELECT 1 FROM matchAttendance ma WHERE ma.matchID = r.matchID AND ma.playerID = r.playerID)
			UNION ALL
			SELECT ma.playerID, ma.attended
			FROM matchAttendance ma
			JOIN matches m ON m.id = ma.matchID
			WHERE m.seasonID = ? AND ma.teamID = ? AND m.matchDate < ?
		) combined
		WHERE attended = 1
		GROUP BY playerID`

	rows, err := m.DB.Query(stmt, seasonID, teamID, asOf, seasonID, teamID, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[int]int{}
	for rows.Next() {
		var playerID, count int
		if err := rows.Scan(&playerID, &count); err != nil {
			return nil, err
		}
		result[playerID] = count
	}
	return result, nil
}

// MatchesPlayedByPlayerBySeason is MatchesPlayedByTeamSeason's all-time,
// player-scoped counterpart, grouped by season instead of scoped to one —
// for each season playerID has an already-played match (MatchDate before
// asOf) in, across every team they've ever been on, how many they're
// considered to have attended: RSVP "yes" by default, or this table's
// value when an override exists, resolved the same override-wins way.
// Backs the season-by-season table on a player's profile page.
func (m *MatchAttendanceModel) MatchesPlayedByPlayerBySeason(playerID int, asOf time.Time) (map[int]int, error) {
	stmt := `SELECT seasonID, COUNT(*) FROM (
			SELECT r.matchID AS matchID, m.seasonID AS seasonID, (r.status = 'yes') AS attended
			FROM rsvps r
			JOIN matches m ON m.id = r.matchID
			WHERE r.playerID = ? AND m.matchDate < ?
				AND NOT EXISTS (SELECT 1 FROM matchAttendance ma WHERE ma.matchID = r.matchID AND ma.playerID = r.playerID)
			UNION ALL
			SELECT ma.matchID, m.seasonID, ma.attended
			FROM matchAttendance ma
			JOIN matches m ON m.id = ma.matchID
			WHERE ma.playerID = ? AND m.matchDate < ?
		) combined
		WHERE attended = 1
		GROUP BY seasonID`

	rows, err := m.DB.Query(stmt, playerID, asOf, playerID, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[int]int{}
	for rows.Next() {
		var seasonID, count int
		if err := rows.Scan(&seasonID, &count); err != nil {
			return nil, err
		}
		result[seasonID] = count
	}
	return result, nil
}

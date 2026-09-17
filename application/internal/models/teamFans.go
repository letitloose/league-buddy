package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TeamFan is one user following one team as a fan — no roster/RSVP/captain
// semantics at all, unlike TeamMember. A Fan account (a User with no
// PlayerID) sees the same public content a roster member does, minus
// player demographic info and match-day internal details (see
// cmd/web/handlers_matches.go's ShowHomeRoster/ShowAwayRoster).
type TeamFan struct {
	ID         int
	UserID     int
	TeamID     int
	FollowedAt time.Time
}

// FanListRow is one row of a team's fan list — just enough for the
// manager-facing "Fans (N)" panel on team-view.html.
type FanListRow struct {
	UserID     int
	Email      string
	FollowedAt time.Time
}

type TeamFanModel struct {
	DB *sql.DB
}

// Follow adds userID as a fan of teamID. Fails with ErrDuplicateFollow if
// they already follow that team.
func (m *TeamFanModel) Follow(userID, teamID int) error {
	statement := `INSERT INTO teamFans (userID, teamID, followedAt) VALUES (?, ?, UTC_TIMESTAMP())`

	_, err := m.DB.Exec(statement, userID, teamID)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 {
				return ErrDuplicateFollow
			}
		}
		return err
	}
	return nil
}

// Unfollow removes userID's fan relationship with teamID, if any — a no-op
// (no error) if they weren't following it.
func (m *TeamFanModel) Unfollow(userID, teamID int) error {
	statement := `DELETE FROM teamFans WHERE userID = ? AND teamID = ?`

	_, err := m.DB.Exec(statement, userID, teamID)
	return err
}

func (m *TeamFanModel) IsFollowing(userID, teamID int) (bool, error) {
	stmt := `SELECT EXISTS(SELECT 1 FROM teamFans WHERE userID = ? AND teamID = ?)`

	var exists bool
	err := m.DB.QueryRow(stmt, userID, teamID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// GetFollowedTeams returns every team userID follows, ordered by team name
// — used for the home page's "Teams You Follow" cards and the fan
// calendar feed.
func (m *TeamFanModel) GetFollowedTeams(userID int) ([]*Team, error) {
	stmt := `SELECT t.id, t.leagueID, t.name, t.motto, t.establishedDate, t.captainPlayerID, t.locationID, t.created
		FROM teams t
		JOIN teamFans tf ON tf.teamID = t.id
		WHERE tf.userID = ?
		ORDER BY t.name ASC`

	rows, err := m.DB.Query(stmt, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := []*Team{}
	for rows.Next() {
		team := &Team{}
		err := rows.Scan(&team.ID, &team.LeagueID, &team.Name, &team.Motto, &team.EstablishedDate, &team.CaptainPlayerID, &team.LocationID, &team.Created)
		if err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}
	return teams, nil
}

// ListFansForTeam returns every fan of teamID, most-recently-followed
// first, for the manager-facing "Fans (N)" panel.
func (m *TeamFanModel) ListFansForTeam(teamID int) ([]*FanListRow, error) {
	stmt := `SELECT u.id, u.email, tf.followedAt
		FROM teamFans tf
		JOIN users u ON u.id = tf.userID
		WHERE tf.teamID = ?
		ORDER BY tf.followedAt DESC`

	rows, err := m.DB.Query(stmt, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fans := []*FanListRow{}
	for rows.Next() {
		fan := &FanListRow{}
		if err := rows.Scan(&fan.UserID, &fan.Email, &fan.FollowedAt); err != nil {
			return nil, err
		}
		fans = append(fans, fan)
	}
	return fans, nil
}

// CountForTeam returns how many fans follow teamID.
func (m *TeamFanModel) CountForTeam(teamID int) (int, error) {
	stmt := `SELECT COUNT(*) FROM teamFans WHERE teamID = ?`

	var count int
	err := m.DB.QueryRow(stmt, teamID).Scan(&count)
	return count, err
}

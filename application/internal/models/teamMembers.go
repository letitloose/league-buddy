package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

type TeamMember struct {
	ID       int
	PlayerID int
	TeamID   int
	JoinedAt time.Time
}

type TeamMemberModel struct {
	DB *sql.DB
}

// AddMembership adds playerID to teamID's roster. Fails with
// ErrDuplicateMembership if the player is already a member of that team.
func (m *TeamMemberModel) AddMembership(playerID, teamID int) error {
	statement := `INSERT INTO teamMembers (playerID, teamID, joinedAt) VALUES (?, ?, UTC_TIMESTAMP())`

	_, err := m.DB.Exec(statement, playerID, teamID)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 {
				return ErrDuplicateMembership
			}
		}
		return err
	}
	return nil
}

// RemoveMembership drops playerID from teamID's roster only — the player
// record itself, and any of their other team memberships, are untouched.
func (m *TeamMemberModel) RemoveMembership(playerID, teamID int) error {
	statement := `DELETE FROM teamMembers WHERE playerID = ? AND teamID = ?`

	_, err := m.DB.Exec(statement, playerID, teamID)
	return err
}

// DeleteAllForPlayer removes every membership row for playerID — used when
// fully deleting a player record (no FK cascade is configured), never by the
// lightweight per-team "remove from roster" action.
func (m *TeamMemberModel) DeleteAllForPlayer(playerID int) error {
	statement := `DELETE FROM teamMembers WHERE playerID = ?`

	_, err := m.DB.Exec(statement, playerID)
	return err
}

func (m *TeamMemberModel) IsMember(playerID, teamID int) (bool, error) {
	stmt := `SELECT EXISTS(SELECT 1 FROM teamMembers WHERE playerID = ? AND teamID = ?)`

	var exists bool
	err := m.DB.QueryRow(stmt, playerID, teamID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// HasTeamInLeague reports whether playerID already belongs to some team in
// leagueID — the check behind the "one team per league" rule.
func (m *TeamMemberModel) HasTeamInLeague(playerID, leagueID int) (bool, error) {
	stmt := `SELECT EXISTS(
		SELECT 1 FROM teamMembers tm
		JOIN teams t ON t.id = tm.teamID
		WHERE tm.playerID = ? AND t.leagueID = ?
	)`

	var exists bool
	err := m.DB.QueryRow(stmt, playerID, leagueID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// GetTeamsForPlayer returns every team playerID is a member of, ordered by
// team name — used for the nav "My Teams" list and the home page cards.
func (m *TeamMemberModel) GetTeamsForPlayer(playerID int) ([]*Team, error) {
	stmt := `SELECT t.id, t.leagueID, t.name, t.motto, t.establishedDate, t.captainPlayerID, t.locationID, t.created
		FROM teams t
		JOIN teamMembers tm ON tm.teamID = t.id
		WHERE tm.playerID = ?
		ORDER BY t.name ASC`

	rows, err := m.DB.Query(stmt, playerID)
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

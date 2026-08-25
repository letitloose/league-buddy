package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TeamScorekeeper grants a player match-editing rights (score, goals,
// cards) for one team, without the full team-management rights a captain
// or league admin has (roster, invites, team info). Captains designate
// scorekeepers themselves — see canManageMatch, which checks this
// alongside captaincy/league-admin/system-admin.
type TeamScorekeeper struct {
	ID        int
	PlayerID  int
	TeamID    int
	CreatedAt time.Time
}

type TeamScorekeeperModel struct {
	DB *sql.DB
}

// AddScorekeeper makes playerID a scorekeeper of teamID. Fails with
// ErrDuplicateScorekeeper if the player already is one.
func (m *TeamScorekeeperModel) AddScorekeeper(playerID, teamID int) error {
	statement := `INSERT INTO teamScorekeepers (playerID, teamID, createdAt) VALUES (?, ?, UTC_TIMESTAMP())`

	_, err := m.DB.Exec(statement, playerID, teamID)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 {
				return ErrDuplicateScorekeeper
			}
		}
		return err
	}
	return nil
}

// RemoveScorekeeper revokes playerID's scorekeeper rights over teamID only.
func (m *TeamScorekeeperModel) RemoveScorekeeper(playerID, teamID int) error {
	statement := `DELETE FROM teamScorekeepers WHERE playerID = ? AND teamID = ?`

	_, err := m.DB.Exec(statement, playerID, teamID)
	return err
}

// DeleteAllForPlayer removes every scorekeeper row for playerID — used when
// fully deleting a player record, since fk_teamscorekeepers_player would
// otherwise block it.
func (m *TeamScorekeeperModel) DeleteAllForPlayer(playerID int) error {
	statement := `DELETE FROM teamScorekeepers WHERE playerID = ?`

	_, err := m.DB.Exec(statement, playerID)
	return err
}

func (m *TeamScorekeeperModel) IsScorekeeper(playerID, teamID int) (bool, error) {
	stmt := `SELECT EXISTS(SELECT 1 FROM teamScorekeepers WHERE playerID = ? AND teamID = ?)`

	var exists bool
	err := m.DB.QueryRow(stmt, playerID, teamID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// GetTeamIDsForPlayer returns every team playerID is a scorekeeper of — used
// for AuthContext.
func (m *TeamScorekeeperModel) GetTeamIDsForPlayer(playerID int) ([]int, error) {
	stmt := `SELECT teamID FROM teamScorekeepers WHERE playerID = ?`

	rows, err := m.DB.Query(stmt, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teamIDs := []int{}
	for rows.Next() {
		var teamID int
		if err := rows.Scan(&teamID); err != nil {
			return nil, err
		}
		teamIDs = append(teamIDs, teamID)
	}
	return teamIDs, nil
}

// ListForTeam returns every player who is a scorekeeper of teamID, ordered
// by name — used by the team-view scorekeepers panel.
func (m *TeamScorekeeperModel) ListForTeam(teamID int) ([]*Player, error) {
	stmt := `SELECT p.id, p.firstname, p.lastname, p.dateOfBirth, p.addressID, p.email, p.phonenumber, p.created
		FROM players p
		JOIN teamScorekeepers ts ON ts.playerID = p.id
		WHERE ts.teamID = ?
		ORDER BY p.firstname ASC, p.lastname ASC`

	rows, err := m.DB.Query(stmt, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := []*Player{}
	for rows.Next() {
		player := &Player{}
		err := rows.Scan(&player.ID, &player.FirstName, &player.LastName, &player.DateOfBirth,
			&player.AddressID, &player.Email, &player.PhoneNumber, &player.Created)
		if err != nil {
			return nil, err
		}
		players = append(players, player)
	}
	return players, nil
}

package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

type LeagueAdmin struct {
	ID        int
	PlayerID  int
	LeagueID  int
	CreatedAt time.Time
}

type LeagueAdminModel struct {
	DB *sql.DB
}

// AddAdmin makes playerID an admin of leagueID. Fails with
// ErrDuplicateLeagueAdmin if the player already administers that league.
func (m *LeagueAdminModel) AddAdmin(playerID, leagueID int) error {
	statement := `INSERT INTO leagueAdmins (playerID, leagueID, createdAt) VALUES (?, ?, UTC_TIMESTAMP())`

	_, err := m.DB.Exec(statement, playerID, leagueID)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 {
				return ErrDuplicateLeagueAdmin
			}
		}
		return err
	}
	return nil
}

// RemoveAdmin revokes playerID's admin rights over leagueID only.
func (m *LeagueAdminModel) RemoveAdmin(playerID, leagueID int) error {
	statement := `DELETE FROM leagueAdmins WHERE playerID = ? AND leagueID = ?`

	_, err := m.DB.Exec(statement, playerID, leagueID)
	return err
}

// DeleteAllForPlayer removes every league-admin row for playerID — used when
// fully deleting a player record, since fk_leagueadmins_player would
// otherwise block it.
func (m *LeagueAdminModel) DeleteAllForPlayer(playerID int) error {
	statement := `DELETE FROM leagueAdmins WHERE playerID = ?`

	_, err := m.DB.Exec(statement, playerID)
	return err
}

func (m *LeagueAdminModel) IsLeagueAdmin(playerID, leagueID int) (bool, error) {
	stmt := `SELECT EXISTS(SELECT 1 FROM leagueAdmins WHERE playerID = ? AND leagueID = ?)`

	var exists bool
	err := m.DB.QueryRow(stmt, playerID, leagueID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// GetLeaguesForPlayer returns every league playerID administers, ordered by
// name — used for the nav "My Leagues (Admin)" list and AuthContext.
func (m *LeagueAdminModel) GetLeaguesForPlayer(playerID int) ([]*League, error) {
	stmt := `SELECT l.id, l.name, l.motto, l.establishedDate, l.created
		FROM leagues l
		JOIN leagueAdmins la ON la.leagueID = l.id
		WHERE la.playerID = ?
		ORDER BY l.name ASC`

	rows, err := m.DB.Query(stmt, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leagues := []*League{}
	for rows.Next() {
		league := &League{}
		err := rows.Scan(&league.ID, &league.Name, &league.Motto, &league.EstablishedDate, &league.Created)
		if err != nil {
			return nil, err
		}
		leagues = append(leagues, league)
	}
	return leagues, nil
}

// ListAdminsForLeague returns every player who administers leagueID, ordered
// by name — used by the league-view admin panel.
func (m *LeagueAdminModel) ListAdminsForLeague(leagueID int) ([]*Player, error) {
	stmt := `SELECT p.id, p.firstname, p.lastname, p.dateOfBirth, p.addressID, p.email, p.phonenumber, p.created
		FROM players p
		JOIN leagueAdmins la ON la.playerID = p.id
		WHERE la.leagueID = ?
		ORDER BY p.firstname ASC, p.lastname ASC`

	rows, err := m.DB.Query(stmt, leagueID)
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

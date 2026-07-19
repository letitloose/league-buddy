package models

import (
	"database/sql"
	"errors"
	"time"
)

// Placeholder name given to a player record auto-created on signup/activation
// before the person has filled in their real name.
const (
	PlaceholderFirstName = "Update"
	PlaceholderLastName  = "Your Profile"
)

type Player struct {
	ID          int
	FirstName   string
	LastName    string
	DateOfBirth sql.NullTime
	AddressID   sql.NullInt32
	Email       sql.NullString
	PhoneNumber sql.NullString
	Created     time.Time
}

type PlayerModel struct {
	DB *sql.DB
}

func (m *PlayerModel) Insert(player *Player) (int, error) {

	statement := `INSERT INTO players (firstname, lastname, dateOfBirth, addressID, email, phonenumber, created)
    VALUES(?, ?, ?, ?, ?, ?, UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, player.FirstName, player.LastName,
		player.DateOfBirth, player.AddressID, player.Email, player.PhoneNumber)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(id), nil
}

func (m *PlayerModel) Update(player *Player) error {

	statement := `update players set
		firstname = ?,
		lastname = ?,
		dateOfBirth = ?,
		addressID = ?,
		email = ?,
		phonenumber = ?
	where id = ?`

	_, err := m.DB.Exec(statement, player.FirstName, player.LastName, player.DateOfBirth,
		player.AddressID, player.Email, player.PhoneNumber, player.ID)

	return err
}

func (m *PlayerModel) Delete(id int) error {
	statement := "delete from players where id = ?"

	_, err := m.DB.Exec(statement, id)

	return err
}

func (m *PlayerModel) Get(id int) (*Player, error) {

	stmt := `select id, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created
		from players where id = ?`

	result := m.DB.QueryRow(stmt, id)

	player := &Player{}
	err := result.Scan(&player.ID, &player.FirstName, &player.LastName,
		&player.DateOfBirth, &player.AddressID, &player.Email, &player.PhoneNumber, &player.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		} else {
			return nil, err
		}
	}
	return player, nil
}

func (m *PlayerModel) GetByEmail(email string) (*Player, error) {

	stmt := `select id, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created
		from players where email = ?`

	result := m.DB.QueryRow(stmt, email)

	player := &Player{}
	err := result.Scan(&player.ID, &player.FirstName, &player.LastName,
		&player.DateOfBirth, &player.AddressID, &player.Email, &player.PhoneNumber, &player.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		} else {
			return nil, err
		}
	}
	return player, nil
}

// GetByTeam returns a team's roster, joined through teamMembers now that a
// player can belong to more than one team.
func (m *PlayerModel) GetByTeam(teamID int) ([]*Player, error) {

	stmt := `select p.id, p.firstname, p.lastname, p.dateOfBirth, p.addressID, p.email, p.phonenumber, p.created
		from players p
		join teamMembers tm on tm.playerID = p.id
		where tm.teamID = ?
		order by p.lastname asc, p.firstname asc`

	rows, err := m.DB.Query(stmt, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := []*Player{}
	for rows.Next() {
		player := &Player{}
		err := rows.Scan(&player.ID, &player.FirstName, &player.LastName,
			&player.DateOfBirth, &player.AddressID, &player.Email, &player.PhoneNumber, &player.Created)
		if err != nil {
			return nil, err
		}
		players = append(players, player)
	}

	return players, nil
}

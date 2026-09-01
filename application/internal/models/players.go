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
	ID                         int
	FirstName                  string
	LastName                   string
	DateOfBirth                sql.NullTime
	AddressID                  sql.NullInt32
	Email                      sql.NullString
	PhoneNumber                sql.NullString
	Created                    time.Time
	PhoneVerifiedAt            sql.NullTime
	PhoneVerificationCode      sql.NullString
	PhoneVerificationExpiresAt sql.NullTime
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

// Update saves player's fields. The phone-verification columns are set
// FIRST in the SET list, before phonenumber itself — MySQL/MariaDB
// evaluates a multi-column UPDATE's assignments left to right, and a
// later assignment sees values already written by an earlier one in the
// same statement, so referencing the (still-old) `phonenumber` column
// ahead of its own reassignment is what makes the `<=>` comparison below
// compare the new number against the row's *previous* one, not against
// itself. This is the single place every phone-number write in the app
// goes through (self-edit, captain/admin edit, CSV re-import all call
// this), so it's the one place the "changing the number clears
// verification" consent rule needs to be enforced — a phone can never end
// up marked verified for a number its owner didn't actually confirm.
func (m *PlayerModel) Update(player *Player) error {

	statement := `update players set
		phoneVerifiedAt = CASE WHEN phonenumber <=> ? THEN phoneVerifiedAt ELSE NULL END,
		phoneVerificationCode = CASE WHEN phonenumber <=> ? THEN phoneVerificationCode ELSE NULL END,
		phoneVerificationExpiresAt = CASE WHEN phonenumber <=> ? THEN phoneVerificationExpiresAt ELSE NULL END,
		firstname = ?,
		lastname = ?,
		dateOfBirth = ?,
		addressID = ?,
		email = ?,
		phonenumber = ?
	where id = ?`

	_, err := m.DB.Exec(statement,
		player.PhoneNumber, player.PhoneNumber, player.PhoneNumber,
		player.FirstName, player.LastName, player.DateOfBirth,
		player.AddressID, player.Email, player.PhoneNumber, player.ID)

	return err
}

// SetPhoneVerificationCode stores a fresh verification code and its
// expiry for playerID, independent of Update (which touches the whole
// row) — used once PlayerService.RequestPhoneVerification has already
// saved the number itself via Update.
func (m *PlayerModel) SetPhoneVerificationCode(playerID int, code string, expiresAt time.Time) error {
	statement := `update players set phoneVerificationCode = ?, phoneVerificationExpiresAt = ? where id = ?`
	_, err := m.DB.Exec(statement, code, expiresAt, playerID)
	return err
}

// ConfirmPhoneVerified marks playerID's phone verified right now and
// clears the pending code/expiry.
func (m *PlayerModel) ConfirmPhoneVerified(playerID int) error {
	statement := `update players set phoneVerifiedAt = UTC_TIMESTAMP(), phoneVerificationCode = NULL, phoneVerificationExpiresAt = NULL where id = ?`
	_, err := m.DB.Exec(statement, playerID)
	return err
}

func (m *PlayerModel) Delete(id int) error {
	statement := "delete from players where id = ?"

	_, err := m.DB.Exec(statement, id)

	return err
}

func (m *PlayerModel) Get(id int) (*Player, error) {

	stmt := `select id, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created, phoneVerifiedAt, phoneVerificationCode, phoneVerificationExpiresAt
		from players where id = ?`

	result := m.DB.QueryRow(stmt, id)

	player := &Player{}
	err := result.Scan(&player.ID, &player.FirstName, &player.LastName,
		&player.DateOfBirth, &player.AddressID, &player.Email, &player.PhoneNumber, &player.Created,
		&player.PhoneVerifiedAt, &player.PhoneVerificationCode, &player.PhoneVerificationExpiresAt)
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

	stmt := `select id, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created, phoneVerifiedAt, phoneVerificationCode, phoneVerificationExpiresAt
		from players where email = ?`

	result := m.DB.QueryRow(stmt, email)

	player := &Player{}
	err := result.Scan(&player.ID, &player.FirstName, &player.LastName,
		&player.DateOfBirth, &player.AddressID, &player.Email, &player.PhoneNumber, &player.Created,
		&player.PhoneVerifiedAt, &player.PhoneVerificationCode, &player.PhoneVerificationExpiresAt)
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

	stmt := `select p.id, p.firstname, p.lastname, p.dateOfBirth, p.addressID, p.email, p.phonenumber, p.created, p.phoneVerifiedAt, p.phoneVerificationCode, p.phoneVerificationExpiresAt
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
			&player.DateOfBirth, &player.AddressID, &player.Email, &player.PhoneNumber, &player.Created,
			&player.PhoneVerifiedAt, &player.PhoneVerificationCode, &player.PhoneVerificationExpiresAt)
		if err != nil {
			return nil, err
		}
		players = append(players, player)
	}

	return players, nil
}

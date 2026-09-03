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

// DismissCaptainGuideBanner permanently hides the home page's "New captains
// start here!" banner for playerID.
func (m *PlayerModel) DismissCaptainGuideBanner(playerID int) error {
	statement := `update players set captainGuideDismissedAt = UTC_TIMESTAMP() where id = ?`
	_, err := m.DB.Exec(statement, playerID)
	return err
}

// HasDismissedCaptainGuideBanner reports whether playerID has already
// dismissed the home page's captain guide banner (see
// DismissCaptainGuideBanner) — a dedicated single-column lookup rather than
// adding this field to every Player-fetching query below, since nothing
// else needs it.
func (m *PlayerModel) HasDismissedCaptainGuideBanner(playerID int) (bool, error) {
	var dismissedAt sql.NullTime
	err := m.DB.QueryRow(`select captainGuideDismissedAt from players where id = ?`, playerID).Scan(&dismissedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNoRecord
		}
		return false, err
	}
	return dismissedAt.Valid, nil
}

// GetCalendarToken returns playerID's calendar-feed token, if one's been
// generated yet — see CalendarService.EnsureToken. A dedicated
// single-column lookup rather than adding this field to every
// Player-fetching query below, since nothing else needs it (same
// rationale as HasDismissedCaptainGuideBanner above).
func (m *PlayerModel) GetCalendarToken(playerID int) (sql.NullString, error) {
	var token sql.NullString
	err := m.DB.QueryRow(`select calendarToken from players where id = ?`, playerID).Scan(&token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.NullString{}, ErrNoRecord
		}
		return sql.NullString{}, err
	}
	return token, nil
}

// SetCalendarToken stores playerID's newly (re)generated calendar-feed
// token — see CalendarService.EnsureToken/RegenerateToken, which
// generate the token itself; this just persists it.
func (m *PlayerModel) SetCalendarToken(playerID int, token string) error {
	statement := `update players set calendarToken = ? where id = ?`
	_, err := m.DB.Exec(statement, token, playerID)
	return err
}

// GetByCalendarToken resolves a calendar-feed URL's secret token back to
// its player — the same full column list as Get/GetByEmail (just a
// different WHERE clause), so it returns a fully-populated *Player like
// every other Get* here. ErrNoRecord for an unknown or revoked token.
func (m *PlayerModel) GetByCalendarToken(token string) (*Player, error) {

	stmt := `select id, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created, phoneVerifiedAt, phoneVerificationCode, phoneVerificationExpiresAt
		from players where calendarToken = ?`

	result := m.DB.QueryRow(stmt, token)

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

// GetActiveByTeam is GetByTeam narrowed to the active roster — excludes
// Legends (see TeamMemberModel.SetLegendStatus). Used only by the team
// page's Active tab. GetByTeam itself is deliberately left unfiltered and
// unchanged everywhere else it's already used (match-day rosters, RSVP
// reminders, roster export, etc.) — a Legend is still meant to be able to
// show up, RSVP, and get stats recorded like anyone else; "Legend" is a
// roster-page organizing status, not a functional restriction.
func (m *PlayerModel) GetActiveByTeam(teamID int) ([]*Player, error) {
	stmt := `select p.id, p.firstname, p.lastname, p.dateOfBirth, p.addressID, p.email, p.phonenumber, p.created, p.phoneVerifiedAt, p.phoneVerificationCode, p.phoneVerificationExpiresAt
		from players p
		join teamMembers tm on tm.playerID = p.id
		where tm.teamID = ? and tm.isLegend = 0
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

// GetLegendsByTeam returns teamID's Legends — former roster members a
// captain has moved off the active roster but who remain linked to the
// team, still showing their career stats on their own profile (see
// TeamMemberModel.SetLegendStatus).
func (m *PlayerModel) GetLegendsByTeam(teamID int) ([]*Player, error) {
	stmt := `select p.id, p.firstname, p.lastname, p.dateOfBirth, p.addressID, p.email, p.phonenumber, p.created, p.phoneVerifiedAt, p.phoneVerificationCode, p.phoneVerificationExpiresAt
		from players p
		join teamMembers tm on tm.playerID = p.id
		where tm.teamID = ? and tm.isLegend = 1
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

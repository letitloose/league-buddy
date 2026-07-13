package models

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Player struct {
	ID          int
	TeamID      int
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

	statement := `INSERT INTO players (teamID, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created)
    VALUES(?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, player.TeamID, player.FirstName, player.LastName,
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

	stmt := `select id, teamID, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created
		from players where id = ?`

	result := m.DB.QueryRow(stmt, id)

	player := &Player{}
	err := result.Scan(&player.ID, &player.TeamID, &player.FirstName, &player.LastName,
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

	stmt := `select id, teamID, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created
		from players where email = ?`

	result := m.DB.QueryRow(stmt, email)

	player := &Player{}
	err := result.Scan(&player.ID, &player.TeamID, &player.FirstName, &player.LastName,
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

func (m *PlayerModel) GetByTeam(teamID int) ([]*Player, error) {

	stmt := `select id, teamID, firstname, lastname, dateOfBirth, addressID, email, phonenumber, created
		from players where teamID = ? order by lastname asc, firstname asc`

	rows, err := m.DB.Query(stmt, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := []*Player{}
	for rows.Next() {
		player := &Player{}
		err := rows.Scan(&player.ID, &player.TeamID, &player.FirstName, &player.LastName,
			&player.DateOfBirth, &player.AddressID, &player.Email, &player.PhoneNumber, &player.Created)
		if err != nil {
			return nil, err
		}
		players = append(players, player)
	}

	return players, nil
}

type PlayerSearchCriteria struct {
	FirstName string
	LastName  string
	Email     string
	TeamID    int
	Sort      string
	Order     string
	Offset    int
	Limit     int
}

type PlayerSearchResult struct {
	ID          int
	FirstName   string
	LastName    string
	Email       sql.NullString
	PhoneNumber sql.NullString
	DateOfBirth sql.NullTime
	FullCount   sql.NullInt32
}

func (m *PlayerModel) Search(criteria *PlayerSearchCriteria) ([]*PlayerSearchResult, error) {
	statement, queryParams := buildPlayerSearchStatement(criteria)
	rows, err := m.DB.Query(statement, queryParams...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := []*PlayerSearchResult{}
	for rows.Next() {
		player := &PlayerSearchResult{}
		err := rows.Scan(&player.ID, &player.FirstName, &player.LastName, &player.Email,
			&player.PhoneNumber, &player.DateOfBirth, &player.FullCount)
		if err != nil {
			return nil, err
		}
		players = append(players, player)
	}
	return players, nil
}

func buildPlayerSearchStatement(criteria *PlayerSearchCriteria) (string, []any) {
	allowedSorts := map[string]string{
		"firstname": "firstname",
		"lastname":  "lastname",
		"email":     "email",
	}

	base := `SELECT id, firstname, lastname, email, phonenumber, dateOfBirth, COUNT(*) OVER() AS fullCount
		FROM players WHERE teamID = ?`

	queryParams := []any{criteria.TeamID}
	where := ""

	if criteria.FirstName != "" {
		where += " AND UPPER(firstname) LIKE CONCAT('%', ?, '%')"
		queryParams = append(queryParams, strings.ToUpper(criteria.FirstName))
	}
	if criteria.LastName != "" {
		where += " AND UPPER(lastname) LIKE CONCAT('%', ?, '%')"
		queryParams = append(queryParams, strings.ToUpper(criteria.LastName))
	}
	if criteria.Email != "" {
		where += " AND UPPER(email) LIKE CONCAT('%', ?, '%')"
		queryParams = append(queryParams, strings.ToUpper(criteria.Email))
	}

	orderBy := "ORDER BY lastname ASC"
	if sortCol, ok := allowedSorts[criteria.Sort]; ok {
		order := "ASC"
		if criteria.Order == "DESC" {
			order = "DESC"
		}
		orderBy = fmt.Sprintf("ORDER BY %s %s", sortCol, order)
	}

	limit := fmt.Sprintf("LIMIT %d OFFSET %d", criteria.Limit, criteria.Offset)
	return fmt.Sprintf("%s%s %s %s;", base, where, orderBy, limit), queryParams
}

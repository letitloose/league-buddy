package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

type League struct {
	ID              int
	Name            string
	Motto           sql.NullString
	EstablishedDate sql.NullTime
	Created         time.Time
}

type LeagueModel struct {
	DB *sql.DB
}

func (m *LeagueModel) Insert(league *League) (int, error) {
	statement := `INSERT INTO leagues (name, motto, establishedDate, created) VALUES (?, ?, ?, UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, league.Name, league.Motto, league.EstablishedDate)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (m *LeagueModel) Get(id int) (*League, error) {
	stmt := `SELECT id, name, motto, establishedDate, created FROM leagues WHERE id = ?`

	league := &League{}
	err := m.DB.QueryRow(stmt, id).Scan(&league.ID, &league.Name, &league.Motto, &league.EstablishedDate, &league.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return league, nil
}

func (m *LeagueModel) Update(league *League) error {
	statement := `UPDATE leagues SET name = ?, motto = ?, establishedDate = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, league.Name, league.Motto, league.EstablishedDate, league.ID)
	return err
}

func (m *LeagueModel) Delete(id int) error {
	statement := `DELETE FROM leagues WHERE id = ?`

	_, err := m.DB.Exec(statement, id)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1451 {
				return ErrHasDependents
			}
		}
		return err
	}
	return nil
}

func (m *LeagueModel) List() ([]*League, error) {
	statement := `SELECT id, name, motto, establishedDate, created FROM leagues ORDER BY name ASC`

	rows, err := m.DB.Query(statement)
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

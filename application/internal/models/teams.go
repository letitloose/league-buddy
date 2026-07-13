package models

import (
	"database/sql"
	"errors"
	"time"
)

type Team struct {
	ID      int
	Name    string
	Created time.Time
}

type TeamModel struct {
	DB *sql.DB
}

func (m *TeamModel) Insert(name string) (int, error) {
	statement := `INSERT INTO teams (name, created) VALUES (?, UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, name)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(id), nil
}

func (m *TeamModel) Get(id int) (*Team, error) {
	stmt := `select id, name, created from teams where id = ?`

	team := &Team{}
	err := m.DB.QueryRow(stmt, id).Scan(&team.ID, &team.Name, &team.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return team, nil
}

// GetDefault returns the single team used by the current single-team UI.
// This is the seam multi-team/League support will replace later — callers
// that need "the current team" go through here rather than hardcoding an ID.
func (m *TeamModel) GetDefault() (*Team, error) {
	stmt := `select id, name, created from teams order by id asc limit 1`

	team := &Team{}
	err := m.DB.QueryRow(stmt).Scan(&team.ID, &team.Name, &team.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return team, nil
}

package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Team struct {
	ID              int
	LeagueID        int
	Name            string
	Motto           sql.NullString
	EstablishedDate sql.NullTime
	CaptainPlayerID sql.NullInt32
	LocationID      sql.NullInt32
	Created         time.Time

	// RemindersEnabled/ReminderDaysOut/ReminderTime configure this team's
	// automated RSVP reminder cascade (see
	// services.MatchReminderService.SendDueRSVPReminders) — reminders go
	// out once a day, starting ReminderDaysOut days before a match and
	// counting down to 1, to any roster player who hasn't RSVP'd yet,
	// timed to ReminderTime (a "15:04:05" Eastern time-of-day). Defaults
	// (true/3/"09:00:00") match the fixed global schedule every team used
	// before this was made configurable. Does not affect the separate
	// captain's-message nudge except via RemindersEnabled, which silences
	// it too — a nudge to write a message for a reminder that will never
	// be sent doesn't make sense either.
	RemindersEnabled bool
	ReminderDaysOut  int
	ReminderTime     string
}

type TeamModel struct {
	DB *sql.DB
}

func (m *TeamModel) Insert(team *Team) (int, error) {
	statement := `INSERT INTO teams (leagueID, name, motto, establishedDate, locationID, created) VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, team.LeagueID, team.Name, team.Motto, team.EstablishedDate, team.LocationID)
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
	stmt := `SELECT id, leagueID, name, motto, establishedDate, captainPlayerID, locationID, created, remindersEnabled, reminderDaysOut, reminderTime FROM teams WHERE id = ?`

	team := &Team{}
	err := m.DB.QueryRow(stmt, id).Scan(&team.ID, &team.LeagueID, &team.Name, &team.Motto, &team.EstablishedDate, &team.CaptainPlayerID, &team.LocationID, &team.Created, &team.RemindersEnabled, &team.ReminderDaysOut, &team.ReminderTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return team, nil
}

func (m *TeamModel) Update(team *Team) error {
	statement := `UPDATE teams SET leagueID = ?, name = ?, motto = ?, establishedDate = ?, locationID = ?, remindersEnabled = ?, reminderDaysOut = ?, reminderTime = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, team.LeagueID, team.Name, team.Motto, team.EstablishedDate, team.LocationID, team.RemindersEnabled, team.ReminderDaysOut, team.ReminderTime, team.ID)
	return err
}

func (m *TeamModel) Delete(id int) error {
	statement := `DELETE FROM teams WHERE id = ?`

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

func (m *TeamModel) GetByLeague(leagueID int) ([]*Team, error) {
	stmt := `SELECT id, leagueID, name, motto, establishedDate, captainPlayerID, locationID, created, remindersEnabled, reminderDaysOut, reminderTime FROM teams WHERE leagueID = ? ORDER BY name ASC`

	rows, err := m.DB.Query(stmt, leagueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := []*Team{}
	for rows.Next() {
		team := &Team{}
		err := rows.Scan(&team.ID, &team.LeagueID, &team.Name, &team.Motto, &team.EstablishedDate, &team.CaptainPlayerID, &team.LocationID, &team.Created, &team.RemindersEnabled, &team.ReminderDaysOut, &team.ReminderTime)
		if err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}
	return teams, nil
}

// GetByLocation returns every team whose home field is locationID — used to
// name the teams blocking a location delete (fk_teams_location).
func (m *TeamModel) GetByLocation(locationID int) ([]*Team, error) {
	stmt := `SELECT id, leagueID, name, motto, establishedDate, captainPlayerID, locationID, created, remindersEnabled, reminderDaysOut, reminderTime FROM teams WHERE locationID = ? ORDER BY name ASC`

	rows, err := m.DB.Query(stmt, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := []*Team{}
	for rows.Next() {
		team := &Team{}
		err := rows.Scan(&team.ID, &team.LeagueID, &team.Name, &team.Motto, &team.EstablishedDate, &team.CaptainPlayerID, &team.LocationID, &team.Created, &team.RemindersEnabled, &team.ReminderDaysOut, &team.ReminderTime)
		if err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}
	return teams, nil
}

// SetCaptain assigns playerID as teamID's captain. A zero playerID clears
// the captain instead.
func (m *TeamModel) SetCaptain(teamID int, playerID sql.NullInt32) error {
	statement := `UPDATE teams SET captainPlayerID = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, playerID, teamID)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 {
				return ErrDuplicateCaptain
			}
		}
		return err
	}
	return nil
}

// ClearCaptainByPlayer removes playerID as captain of whatever team (if any)
// currently has them, so the player can be safely deleted despite the
// fk_teams_captain foreign key.
func (m *TeamModel) ClearCaptainByPlayer(playerID int) error {
	statement := `UPDATE teams SET captainPlayerID = NULL WHERE captainPlayerID = ?`

	_, err := m.DB.Exec(statement, playerID)
	return err
}

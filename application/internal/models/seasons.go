package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Season is a league-scoped block of time (e.g. "Spring 2024") containing a
// round-robin of matches. StartDate/EndDate are optional and used only to
// pick a sensible "current" season when a league has several (see
// GetCurrent).
type Season struct {
	ID        int
	LeagueID  int
	Name      string
	StartDate sql.NullTime
	EndDate   sql.NullTime
	Created   time.Time
}

type SeasonModel struct {
	DB *sql.DB
}

func (m *SeasonModel) Insert(season *Season) (int, error) {
	statement := `INSERT INTO seasons (leagueID, name, startDate, endDate, created) VALUES (?, ?, ?, ?, UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, season.LeagueID, season.Name, season.StartDate, season.EndDate)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (m *SeasonModel) Get(id int) (*Season, error) {
	stmt := `SELECT id, leagueID, name, startDate, endDate, created FROM seasons WHERE id = ?`

	season := &Season{}
	err := m.DB.QueryRow(stmt, id).Scan(&season.ID, &season.LeagueID, &season.Name, &season.StartDate, &season.EndDate, &season.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return season, nil
}

func (m *SeasonModel) Update(season *Season) error {
	statement := `UPDATE seasons SET leagueID = ?, name = ?, startDate = ?, endDate = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, season.LeagueID, season.Name, season.StartDate, season.EndDate, season.ID)
	return err
}

func (m *SeasonModel) Delete(id int) error {
	statement := `DELETE FROM seasons WHERE id = ?`

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

// GetMostRecentWithResults returns leagueID's most recently started season
// that has at least one scored match — used to pick which season's
// standings/leaders the league home page shows. ErrNoRecord if no season in
// the league has any recorded result yet.
func (m *SeasonModel) GetMostRecentWithResults(leagueID int) (*Season, error) {
	stmt := `SELECT s.id, s.leagueID, s.name, s.startDate, s.endDate, s.created
		FROM seasons s
		JOIN matches m ON m.seasonID = s.id
		WHERE s.leagueID = ? AND m.homeScore IS NOT NULL AND m.awayScore IS NOT NULL
		GROUP BY s.id, s.leagueID, s.name, s.startDate, s.endDate, s.created
		ORDER BY s.startDate DESC, s.created DESC
		LIMIT 1`

	season := &Season{}
	err := m.DB.QueryRow(stmt, leagueID).Scan(&season.ID, &season.LeagueID, &season.Name, &season.StartDate, &season.EndDate, &season.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return season, nil
}

// GetByLeague returns every season in leagueID, most recently started first.
func (m *SeasonModel) GetByLeague(leagueID int) ([]*Season, error) {
	stmt := `SELECT id, leagueID, name, startDate, endDate, created FROM seasons WHERE leagueID = ? ORDER BY startDate DESC, created DESC`

	rows, err := m.DB.Query(stmt, leagueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seasons := []*Season{}
	for rows.Next() {
		season := &Season{}
		err := rows.Scan(&season.ID, &season.LeagueID, &season.Name, &season.StartDate, &season.EndDate, &season.Created)
		if err != nil {
			return nil, err
		}
		seasons = append(seasons, season)
	}
	return seasons, nil
}

// GetCurrent picks the season a team/league page should default to when
// several exist: the one whose date range contains asOf, else the most
// recently ended one, else the earliest not-yet-started one. Returns
// ErrNoRecord if the league has no seasons at all.
func (m *SeasonModel) GetCurrent(leagueID int, asOf time.Time) (*Season, error) {
	seasons, err := m.GetByLeague(leagueID)
	if err != nil {
		return nil, err
	}
	if len(seasons) == 0 {
		return nil, ErrNoRecord
	}

	var mostRecentlyEnded *Season
	var earliestUpcoming *Season
	for _, season := range seasons {
		started := !season.StartDate.Valid || !season.StartDate.Time.After(asOf)
		ended := season.EndDate.Valid && season.EndDate.Time.Before(asOf)

		if started && !ended {
			return season, nil
		}
		if ended && (mostRecentlyEnded == nil || season.EndDate.Time.After(mostRecentlyEnded.EndDate.Time)) {
			mostRecentlyEnded = season
		}
		if !started && (earliestUpcoming == nil || season.StartDate.Time.Before(earliestUpcoming.StartDate.Time)) {
			earliestUpcoming = season
		}
	}

	if mostRecentlyEnded != nil {
		return mostRecentlyEnded, nil
	}
	if earliestUpcoming != nil {
		return earliestUpcoming, nil
	}
	return seasons[0], nil
}

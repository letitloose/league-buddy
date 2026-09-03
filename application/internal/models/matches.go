package models

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Match is one game within a season. HomeScore/AwayScore are nullable — a
// scheduled-but-not-yet-played (or historically unrecorded) match has no
// score. Notes covers free-text asides and, for historical rows where only
// a win/loss/draw outcome is known but not the exact score, a
// human-readable stand-in for the missing number.
type Match struct {
	ID         int
	SeasonID   int
	HomeTeamID int
	AwayTeamID int
	MatchDate  time.Time
	LocationID sql.NullInt32
	HomeScore  sql.NullInt32
	AwayScore  sql.NullInt32
	Notes      sql.NullString
	Created    time.Time
}

type MatchModel struct {
	DB *sql.DB
}

func (m *MatchModel) Insert(match *Match) (int, error) {
	statement := `INSERT INTO matches (seasonID, homeTeamID, awayTeamID, matchDate, locationID, homeScore, awayScore, notes, created)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, match.SeasonID, match.HomeTeamID, match.AwayTeamID, match.MatchDate,
		match.LocationID, match.HomeScore, match.AwayScore, match.Notes)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (m *MatchModel) Get(id int) (*Match, error) {
	stmt := `SELECT id, seasonID, homeTeamID, awayTeamID, matchDate, locationID, homeScore, awayScore, notes, created FROM matches WHERE id = ?`

	match := &Match{}
	err := m.DB.QueryRow(stmt, id).Scan(&match.ID, &match.SeasonID, &match.HomeTeamID, &match.AwayTeamID, &match.MatchDate,
		&match.LocationID, &match.HomeScore, &match.AwayScore, &match.Notes, &match.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return match, nil
}

func (m *MatchModel) Update(match *Match) error {
	statement := `UPDATE matches SET seasonID = ?, homeTeamID = ?, awayTeamID = ?, matchDate = ?, locationID = ?, homeScore = ?, awayScore = ?, notes = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, match.SeasonID, match.HomeTeamID, match.AwayTeamID, match.MatchDate,
		match.LocationID, match.HomeScore, match.AwayScore, match.Notes, match.ID)
	return err
}

// Delete removes match id along with every row in every table that
// exists only in relation to it — stats, goals, cards, RSVPs, captain's
// notes, and reminder-tracking rows have no life outside their match,
// unlike teams/locations, which are independently referenced elsewhere
// and so block deletion instead (see ErrHasDependents). Each of these
// tables has a foreign key on matchID with no ON DELETE CASCADE, so
// leaving any of them out here would surface as an opaque FK-constraint
// error instead of a clean delete.
func (m *MatchModel) Delete(id int) error {
	for _, table := range []string{
		"playerMatchStats",
		"matchGoals",
		"matchCards",
		"rsvps",
		"matchAttendance",
		"matchTeamNotes",
		"matchRSVPReminders",
		"matchCaptainMessageReminders",
	} {
		if _, err := m.DB.Exec(`DELETE FROM `+table+` WHERE matchID = ?`, id); err != nil {
			return err
		}
	}

	_, err := m.DB.Exec(`DELETE FROM matches WHERE id = ?`, id)
	return err
}

// GetBySeason returns every match in seasonID, earliest first.
func (m *MatchModel) GetBySeason(seasonID int) ([]*Match, error) {
	stmt := `SELECT id, seasonID, homeTeamID, awayTeamID, matchDate, locationID, homeScore, awayScore, notes, created
		FROM matches WHERE seasonID = ? ORDER BY matchDate ASC, id ASC`

	return m.queryMatches(stmt, seasonID)
}

// GetByTeamAndSeason returns teamID's matches (home or away) within
// seasonID, earliest first.
func (m *MatchModel) GetByTeamAndSeason(teamID, seasonID int) ([]*Match, error) {
	stmt := `SELECT id, seasonID, homeTeamID, awayTeamID, matchDate, locationID, homeScore, awayScore, notes, created
		FROM matches WHERE seasonID = ? AND (homeTeamID = ? OR awayTeamID = ?) ORDER BY matchDate ASC, id ASC`

	return m.queryMatches(stmt, seasonID, teamID, teamID)
}

// GetByDate returns every match scheduled anywhere within the calendar
// day starting at date — a half-open range, not exact equality, since
// matchDate now carries a real kickoff time. date must already be the
// start of that day in the caller's intended time zone (dateNDaysOut in
// internal/services/matchReminders.go anchors this in Eastern, so a
// match late in the evening still lands on its own calendar day rather
// than rolling into the next) — used by MatchReminderService to find
// matches N days out.
func (m *MatchModel) GetByDate(date time.Time) ([]*Match, error) {
	stmt := `SELECT id, seasonID, homeTeamID, awayTeamID, matchDate, locationID, homeScore, awayScore, notes, created
		FROM matches WHERE matchDate >= ? AND matchDate < ? ORDER BY id ASC`

	return m.queryMatches(stmt, date, date.AddDate(0, 0, 1))
}

// TeamMatchAggregate is one team's win/loss/draw/goal tally across a
// season's scored matches — the raw numbers standings are built from.
// Points aren't included here since they're a display-layer rule (3/1/0),
// not a fact about the matches themselves.
type TeamMatchAggregate struct {
	TeamID       int
	Wins         int
	Losses       int
	Draws        int
	GoalsFor     int
	GoalsAgainst int
}

// GetSeasonAggregatesByTeam tallies wins/losses/draws/goals for every team
// that has at least one scored match in seasonID — unscored/unplayed
// matches don't count. Teams with no scored matches simply don't appear;
// callers wanting a complete standings table (0s for winless teams) merge
// this against their own team list.
func (m *MatchModel) GetSeasonAggregatesByTeam(seasonID int) ([]*TeamMatchAggregate, error) {
	stmt := `SELECT teamID,
			SUM(CASE WHEN goalsFor > goalsAgainst THEN 1 ELSE 0 END) AS wins,
			SUM(CASE WHEN goalsFor < goalsAgainst THEN 1 ELSE 0 END) AS losses,
			SUM(CASE WHEN goalsFor = goalsAgainst THEN 1 ELSE 0 END) AS draws,
			SUM(goalsFor) AS goalsFor,
			SUM(goalsAgainst) AS goalsAgainst
		FROM (
			SELECT homeTeamID AS teamID, homeScore AS goalsFor, awayScore AS goalsAgainst
			FROM matches WHERE seasonID = ? AND homeScore IS NOT NULL AND awayScore IS NOT NULL
			UNION ALL
			SELECT awayTeamID AS teamID, awayScore AS goalsFor, homeScore AS goalsAgainst
			FROM matches WHERE seasonID = ? AND homeScore IS NOT NULL AND awayScore IS NOT NULL
		) AS results
		GROUP BY teamID`

	rows, err := m.DB.Query(stmt, seasonID, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aggregates := []*TeamMatchAggregate{}
	for rows.Next() {
		agg := &TeamMatchAggregate{}
		err := rows.Scan(&agg.TeamID, &agg.Wins, &agg.Losses, &agg.Draws, &agg.GoalsFor, &agg.GoalsAgainst)
		if err != nil {
			return nil, err
		}
		aggregates = append(aggregates, agg)
	}
	return aggregates, nil
}

// NextMatchForTeam returns teamID's earliest match (home or away) on or
// after asOf, across any season. ErrNoRecord if there isn't one.
func (m *MatchModel) NextMatchForTeam(teamID int, asOf time.Time) (*Match, error) {
	stmt := `SELECT id, seasonID, homeTeamID, awayTeamID, matchDate, locationID, homeScore, awayScore, notes, created
		FROM matches WHERE (homeTeamID = ? OR awayTeamID = ?) AND matchDate >= ? ORDER BY matchDate ASC, id ASC LIMIT 1`

	match := &Match{}
	err := m.DB.QueryRow(stmt, teamID, teamID, asOf).Scan(&match.ID, &match.SeasonID, &match.HomeTeamID, &match.AwayTeamID,
		&match.MatchDate, &match.LocationID, &match.HomeScore, &match.AwayScore, &match.Notes, &match.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return match, nil
}

// GetUpcomingByTeamIDs returns every match (home or away) for any of
// teamIDs whose matchDate is on/after asOf, across every season, earliest
// first — the multi-team version of NextMatchForTeam, used to build a
// player's calendar feed across every team they belong to. Empty (not an
// error) for an empty teamIDs.
func (m *MatchModel) GetUpcomingByTeamIDs(teamIDs []int, asOf time.Time) ([]*Match, error) {
	if len(teamIDs) == 0 {
		return []*Match{}, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(teamIDs)), ",")
	stmt := fmt.Sprintf(`SELECT id, seasonID, homeTeamID, awayTeamID, matchDate, locationID, homeScore, awayScore, notes, created
		FROM matches WHERE (homeTeamID IN (%s) OR awayTeamID IN (%s)) AND matchDate >= ? ORDER BY matchDate ASC, id ASC`,
		placeholders, placeholders)

	args := make([]any, 0, len(teamIDs)*2+1)
	for _, id := range teamIDs {
		args = append(args, id)
	}
	for _, id := range teamIDs {
		args = append(args, id)
	}
	args = append(args, asOf)

	return m.queryMatches(stmt, args...)
}

func (m *MatchModel) queryMatches(stmt string, args ...any) ([]*Match, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := []*Match{}
	for rows.Next() {
		match := &Match{}
		err := rows.Scan(&match.ID, &match.SeasonID, &match.HomeTeamID, &match.AwayTeamID, &match.MatchDate,
			&match.LocationID, &match.HomeScore, &match.AwayScore, &match.Notes, &match.Created)
		if err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	return matches, nil
}

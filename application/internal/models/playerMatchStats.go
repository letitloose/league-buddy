package models

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// PlayerMatchStat is one player's stat line for one match. TeamID is
// denormalized (not derived via a join to matches) since a player's team
// can differ season to season — this records which team they were on for
// that specific match.
type PlayerMatchStat struct {
	ID          int
	MatchID     int
	PlayerID    int
	TeamID      int
	Goals       int
	Assists     int
	YellowCards int
	RedCards    int
}

// StatLine is one player's aggregated stats across a team's matches in a
// season — the shape the roster leaderboard renders.
type StatLine struct {
	PlayerID    int
	Name        string
	Goals       int
	Assists     int
	YellowCards int
	RedCards    int
}

// LeagueLeaderLine is one player's total for a single stat (goals or
// assists) across every match in a season, league-wide rather than scoped
// to one team — the shape the league/season "leaders" tables render.
type LeagueLeaderLine struct {
	PlayerID int
	Name     string
	TeamName string
	Total    int
}

type PlayerMatchStatModel struct {
	DB *sql.DB
}

// Upsert inserts stat, or updates the existing (matchID, playerID) row if
// one already exists.
func (m *PlayerMatchStatModel) Upsert(stat *PlayerMatchStat) error {
	statement := `INSERT INTO playerMatchStats (matchID, playerID, teamID, goals, assists, yellowCards, redCards) VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := m.DB.Exec(statement, stat.MatchID, stat.PlayerID, stat.TeamID, stat.Goals, stat.Assists, stat.YellowCards, stat.RedCards)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) && mySQLError.Number == 1062 {
			update := `UPDATE playerMatchStats SET teamID = ?, goals = ?, assists = ?, yellowCards = ?, redCards = ? WHERE matchID = ? AND playerID = ?`
			_, err = m.DB.Exec(update, stat.TeamID, stat.Goals, stat.Assists, stat.YellowCards, stat.RedCards, stat.MatchID, stat.PlayerID)
			return err
		}
		return err
	}
	return nil
}

// ListByMatch returns every stat line recorded for matchID.
func (m *PlayerMatchStatModel) ListByMatch(matchID int) ([]*PlayerMatchStat, error) {
	stmt := `SELECT id, matchID, playerID, teamID, goals, assists, yellowCards, redCards FROM playerMatchStats WHERE matchID = ?`

	rows, err := m.DB.Query(stmt, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []*PlayerMatchStat{}
	for rows.Next() {
		stat := &PlayerMatchStat{}
		err := rows.Scan(&stat.ID, &stat.MatchID, &stat.PlayerID, &stat.TeamID, &stat.Goals, &stat.Assists, &stat.YellowCards, &stat.RedCards)
		if err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

// LeaderboardByTeamSeason aggregates every player's stats for teamID across
// seasonID's matches, ordered by goals then assists descending.
func (m *PlayerMatchStatModel) LeaderboardByTeamSeason(teamID, seasonID int) ([]*StatLine, error) {
	stmt := `SELECT p.id, p.firstname, p.lastname,
			SUM(pms.goals) AS goals, SUM(pms.assists) AS assists,
			SUM(pms.yellowCards) AS yellowCards, SUM(pms.redCards) AS redCards
		FROM playerMatchStats pms
		JOIN matches mt ON mt.id = pms.matchID
		JOIN players p ON p.id = pms.playerID
		WHERE pms.teamID = ? AND mt.seasonID = ?
		GROUP BY p.id, p.firstname, p.lastname
		ORDER BY goals DESC, assists DESC, p.lastname ASC, p.firstname ASC`

	rows, err := m.DB.Query(stmt, teamID, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []*StatLine{}
	for rows.Next() {
		var firstName, lastName string
		line := &StatLine{}
		err := rows.Scan(&line.PlayerID, &firstName, &lastName, &line.Goals, &line.Assists, &line.YellowCards, &line.RedCards)
		if err != nil {
			return nil, err
		}
		line.Name = firstName + " " + lastName
		lines = append(lines, line)
	}
	return lines, nil
}

// TopScorersForSeason returns the top limit goal-scorers across every team
// in seasonID, ordered by goals descending — the league (and season) page's
// "Goal Leaders" table.
func (m *PlayerMatchStatModel) TopScorersForSeason(seasonID, limit int) ([]*LeagueLeaderLine, error) {
	return m.topStatForSeason(seasonID, limit, "goals")
}

// TopAssistersForSeason is TopScorersForSeason's assists counterpart —
// the "Assist Leaders" table.
func (m *PlayerMatchStatModel) TopAssistersForSeason(seasonID, limit int) ([]*LeagueLeaderLine, error) {
	return m.topStatForSeason(seasonID, limit, "assists")
}

// topStatForSeason is TopScorersForSeason/TopAssistersForSeason's shared
// query. statColumn is interpolated directly into the SQL rather than bound
// as a parameter (column names can't be placeholders) — safe only because
// every call site passes one of the two literal constants above, never
// anything derived from a request.
func (m *PlayerMatchStatModel) topStatForSeason(seasonID, limit int, statColumn string) ([]*LeagueLeaderLine, error) {
	stmt := fmt.Sprintf(`SELECT p.id, p.firstname, p.lastname, t.name, SUM(pms.%s) AS total
		FROM playerMatchStats pms
		JOIN matches mt ON mt.id = pms.matchID
		JOIN players p ON p.id = pms.playerID
		JOIN teams t ON t.id = pms.teamID
		WHERE mt.seasonID = ?
		GROUP BY p.id, p.firstname, p.lastname, t.name
		HAVING total > 0
		ORDER BY total DESC
		LIMIT ?`, statColumn)

	rows, err := m.DB.Query(stmt, seasonID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []*LeagueLeaderLine{}
	for rows.Next() {
		var firstName, lastName string
		line := &LeagueLeaderLine{}
		err := rows.Scan(&line.PlayerID, &firstName, &lastName, &line.TeamName, &line.Total)
		if err != nil {
			return nil, err
		}
		line.Name = firstName + " " + lastName
		lines = append(lines, line)
	}
	return lines, nil
}

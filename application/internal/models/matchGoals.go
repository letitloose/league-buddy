package models

import "database/sql"

// MatchGoal is one goal scored in a match. ScorerPlayerID/AssisterPlayerID
// are nullable — a goal is always attributed to a team (always observable
// from the sideline), but who exactly scored or assisted may not be known,
// especially for the opposing team.
type MatchGoal struct {
	ID               int
	MatchID          int
	TeamID           int
	ScorerPlayerID   sql.NullInt32
	AssisterPlayerID sql.NullInt32
}

type MatchGoalModel struct {
	DB DBTX
}

// ReplaceForMatch replaces every goal recorded for matchID with goals —
// the match-edit form always submits its complete current set of rows, so
// a plain wipe-and-reinsert (rather than a diff/upsert against a natural
// key, which these plain event-log rows don't have) is simplest.
func (m *MatchGoalModel) ReplaceForMatch(matchID int, goals []MatchGoal) error {
	if _, err := m.DB.Exec("DELETE FROM matchGoals WHERE matchID = ?", matchID); err != nil {
		return err
	}

	statement := "INSERT INTO matchGoals (matchID, teamID, scorerPlayerID, assisterPlayerID) VALUES (?, ?, ?, ?)"
	for _, g := range goals {
		if _, err := m.DB.Exec(statement, matchID, g.TeamID, g.ScorerPlayerID, g.AssisterPlayerID); err != nil {
			return err
		}
	}
	return nil
}

// ListByMatch returns every goal recorded for matchID, in the order they
// were saved — used to prefill the match-edit form's existing rows.
func (m *MatchGoalModel) ListByMatch(matchID int) ([]*MatchGoal, error) {
	stmt := "SELECT id, matchID, teamID, scorerPlayerID, assisterPlayerID FROM matchGoals WHERE matchID = ? ORDER BY id"

	rows, err := m.DB.Query(stmt, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := []*MatchGoal{}
	for rows.Next() {
		g := &MatchGoal{}
		if err := rows.Scan(&g.ID, &g.MatchID, &g.TeamID, &g.ScorerPlayerID, &g.AssisterPlayerID); err != nil {
			return nil, err
		}
		goals = append(goals, g)
	}
	return goals, nil
}

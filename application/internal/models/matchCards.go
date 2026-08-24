package models

import "database/sql"

// MatchCard is one yellow/red card shown in a match. PlayerID is
// nullable — a card is always attributed to a team, but who exactly was
// carded may not be known, especially for the opposing team.
type MatchCard struct {
	ID       int
	MatchID  int
	TeamID   int
	PlayerID sql.NullInt32
	CardType string // "yellow" or "red"
}

type MatchCardModel struct {
	DB DBTX
}

// ReplaceForMatch replaces every card recorded for matchID with cards —
// same wipe-and-reinsert rationale as MatchGoalModel.ReplaceForMatch.
func (m *MatchCardModel) ReplaceForMatch(matchID int, cards []MatchCard) error {
	if _, err := m.DB.Exec("DELETE FROM matchCards WHERE matchID = ?", matchID); err != nil {
		return err
	}

	statement := "INSERT INTO matchCards (matchID, teamID, playerID, cardType) VALUES (?, ?, ?, ?)"
	for _, c := range cards {
		if _, err := m.DB.Exec(statement, matchID, c.TeamID, c.PlayerID, c.CardType); err != nil {
			return err
		}
	}
	return nil
}

// ListByMatch returns every card recorded for matchID, in the order they
// were saved — used to prefill the match-edit form's existing rows.
func (m *MatchCardModel) ListByMatch(matchID int) ([]*MatchCard, error) {
	stmt := "SELECT id, matchID, teamID, playerID, cardType FROM matchCards WHERE matchID = ? ORDER BY id"

	rows, err := m.DB.Query(stmt, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := []*MatchCard{}
	for rows.Next() {
		c := &MatchCard{}
		if err := rows.Scan(&c.ID, &c.MatchID, &c.TeamID, &c.PlayerID, &c.CardType); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, nil
}

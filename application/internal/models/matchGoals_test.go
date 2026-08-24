package models

import (
	"database/sql"
	"testing"
	"time"
)

func TestReplaceAndListMatchGoals(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	mgm := MatchGoalModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	scorer, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	assister, err := pm.Insert(&Player{FirstName: "Ana", LastName: "Winger"})
	if err != nil {
		t.Fatal(err)
	}

	goals := []MatchGoal{
		{TeamID: 1, ScorerPlayerID: sql.NullInt32{Int32: int32(scorer), Valid: true}, AssisterPlayerID: sql.NullInt32{Int32: int32(assister), Valid: true}},
		{TeamID: opponentID}, // unattributed goal for the other team
	}
	if err := mgm.ReplaceForMatch(matchID, goals); err != nil {
		t.Fatal(err)
	}

	got, err := mgm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 goals, got %d", len(got))
	}
	if got[0].TeamID != 1 || !got[0].ScorerPlayerID.Valid || int(got[0].ScorerPlayerID.Int32) != scorer {
		t.Fatalf("expected first goal attributed to the scorer, got %+v", got[0])
	}
	if !got[0].AssisterPlayerID.Valid || int(got[0].AssisterPlayerID.Int32) != assister {
		t.Fatalf("expected first goal's assister recorded, got %+v", got[0])
	}
	if got[1].TeamID != opponentID || got[1].ScorerPlayerID.Valid {
		t.Fatalf("expected second goal unattributed for the opponent, got %+v", got[1])
	}
}

func TestReplaceForMatchWipesPreviousGoals(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	mgm := MatchGoalModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	if err := mgm.ReplaceForMatch(matchID, []MatchGoal{{TeamID: 1}, {TeamID: 1}, {TeamID: opponentID}}); err != nil {
		t.Fatal(err)
	}
	// Re-saving with fewer goals should leave exactly the new set, not
	// accumulate on top of the old one.
	if err := mgm.ReplaceForMatch(matchID, []MatchGoal{{TeamID: 1}}); err != nil {
		t.Fatal(err)
	}

	got, err := mgm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TeamID != 1 {
		t.Fatalf("expected exactly the latest single goal, got %+v", got)
	}
}

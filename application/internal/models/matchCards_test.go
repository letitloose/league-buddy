package models

import (
	"database/sql"
	"testing"
	"time"
)

func TestReplaceAndListMatchCards(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	mcm := MatchCardModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&Player{FirstName: "Rough", LastName: "Tackle"})
	if err != nil {
		t.Fatal(err)
	}

	cards := []MatchCard{
		{TeamID: 1, PlayerID: sql.NullInt32{Int32: int32(playerID), Valid: true}, CardType: "yellow"},
		{TeamID: opponentID, CardType: "red"}, // unattributed card for the other team
	}
	if err := mcm.ReplaceForMatch(matchID, cards); err != nil {
		t.Fatal(err)
	}

	got, err := mcm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(got))
	}
	if got[0].TeamID != 1 || got[0].CardType != "yellow" || !got[0].PlayerID.Valid || int(got[0].PlayerID.Int32) != playerID {
		t.Fatalf("expected first card attributed yellow for the player, got %+v", got[0])
	}
	if got[1].TeamID != opponentID || got[1].CardType != "red" || got[1].PlayerID.Valid {
		t.Fatalf("expected second card unattributed red for the opponent, got %+v", got[1])
	}
}

func TestReplaceForMatchWipesPreviousCards(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	mcm := MatchCardModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	if err := mcm.ReplaceForMatch(matchID, []MatchCard{{TeamID: 1, CardType: "yellow"}, {TeamID: 1, CardType: "yellow"}}); err != nil {
		t.Fatal(err)
	}
	if err := mcm.ReplaceForMatch(matchID, []MatchCard{{TeamID: 1, CardType: "red"}}); err != nil {
		t.Fatal(err)
	}

	got, err := mcm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CardType != "red" {
		t.Fatalf("expected exactly the latest single card, got %+v", got)
	}
}

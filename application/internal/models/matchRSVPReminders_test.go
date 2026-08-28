package models

import (
	"testing"
	"time"
)

func TestMatchRSVPReminderWasSentAndMarkSent(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	mrrm := MatchRSVPReminderModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}

	wasSent, err := mrrm.WasSent(matchID, playerID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if wasSent {
		t.Fatal("expected WasSent false before MarkSent")
	}

	if err := mrrm.MarkSent(matchID, playerID, 3); err != nil {
		t.Fatal(err)
	}

	wasSent, err = mrrm.WasSent(matchID, playerID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSent {
		t.Fatal("expected WasSent true after MarkSent")
	}

	// A different daysOut for the same match/player is tracked separately.
	wasSentDifferentDay, err := mrrm.WasSent(matchID, playerID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if wasSentDifferentDay {
		t.Fatal("expected WasSent false for a daysOut that hasn't been marked")
	}
}

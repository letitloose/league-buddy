package models

import (
	"testing"
	"time"
)

func TestMatchCaptainMessageReminderWasSentAndMarkSent(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	mcrm := MatchCaptainMessageReminderModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	wasSent, err := mcrm.WasSent(matchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if wasSent {
		t.Fatal("expected WasSent false before MarkSent")
	}

	if err := mcrm.MarkSent(matchID, 1); err != nil {
		t.Fatal(err)
	}

	wasSent, err = mcrm.WasSent(matchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSent {
		t.Fatal("expected WasSent true after MarkSent")
	}

	// The other team's reminder for the same match is tracked separately.
	wasSentOtherTeam, err := mcrm.WasSent(matchID, opponentID)
	if err != nil {
		t.Fatal(err)
	}
	if wasSentOtherTeam {
		t.Fatal("expected WasSent false for the other team")
	}
}

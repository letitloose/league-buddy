package models

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestUpsertRSVPInsertAndUpdate(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	rm := RSVPModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}

	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: playerID, TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: playerID, TeamID: 1, Status: "no", Message: sql.NullString{String: "can't make it", Valid: true}, RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	rsvps, err := rm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rsvps) != 1 {
		t.Fatalf("expected upsert to update in place, got %d rows", len(rsvps))
	}
	if rsvps[0].Status != "no" || rsvps[0].Message.String != "can't make it" {
		t.Fatalf("expected updated rsvp status=no message=\"can't make it\", got %+v", rsvps[0])
	}
}

func TestGetRSVPByMatchAndPlayer(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	rm := RSVPModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := rm.GetByMatchAndPlayer(matchID, playerID); !errors.Is(err, ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord before responding, got %v", err)
	}

	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: playerID, TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	rsvp, err := rm.GetByMatchAndPlayer(matchID, playerID)
	if err != nil {
		t.Fatal(err)
	}
	if rsvp.Status != "yes" {
		t.Fatalf("expected status=yes, got %+v", rsvp)
	}
}

func TestCountsByMatchAndTeam(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	rm := RSVPModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	playerIDs := make([]int, 3)
	for i := range playerIDs {
		playerID, err := pm.Insert(&Player{FirstName: "Player", LastName: "Test"})
		if err != nil {
			t.Fatal(err)
		}
		playerIDs[i] = playerID
	}

	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: playerIDs[0], TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: playerIDs[1], TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: playerIDs[2], TeamID: 1, Status: "no", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// A response for the opposing team shouldn't be counted in team 1's tally.
	opponentPlayerID, err := pm.Insert(&Player{FirstName: "Rory", LastName: "Rival"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: opponentPlayerID, TeamID: opponentID, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	in, out, err := rm.CountsByMatchAndTeam(matchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if in != 2 || out != 1 {
		t.Fatalf("expected in=2 out=1, got in=%d out=%d", in, out)
	}

	opponentIn, opponentOut, err := rm.CountsByMatchAndTeam(matchID, opponentID)
	if err != nil {
		t.Fatal(err)
	}
	if opponentIn != 1 || opponentOut != 0 {
		t.Fatalf("expected opponent in=1 out=0, got in=%d out=%d", opponentIn, opponentOut)
	}
}

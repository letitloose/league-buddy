package models

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestUpsertMatchTeamNoteInsertAndUpdate(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	mtnm := MatchTeamNoteModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}

	if err := mtnm.Upsert(&MatchTeamNote{MatchID: matchID, TeamID: 1, Notes: sql.NullString{String: "great effort all around", Valid: true}, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := mtnm.Upsert(&MatchTeamNote{MatchID: matchID, TeamID: 1, PlayerOfMatchID: sql.NullInt32{Int32: int32(playerID), Valid: true}, Notes: sql.NullString{String: "Sam was everywhere", Valid: true}, CaptainMessage: sql.NullString{String: "Meet at 6:15, wear white", Valid: true}, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	note, err := mtnm.GetByMatchAndTeam(matchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !note.PlayerOfMatchID.Valid || int(note.PlayerOfMatchID.Int32) != playerID {
		t.Fatalf("expected upsert to update in place with playerOfMatchID=%d, got %+v", playerID, note.PlayerOfMatchID)
	}
	if note.Notes.String != "Sam was everywhere" {
		t.Fatalf("expected updated notes, got %+v", note.Notes)
	}
	if note.CaptainMessage.String != "Meet at 6:15, wear white" {
		t.Fatalf("expected updated captainMessage, got %+v", note.CaptainMessage)
	}
}

func TestGetMatchTeamNoteByMatchAndTeam(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	mtnm := MatchTeamNoteModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mtnm.GetByMatchAndTeam(matchID, 1); !errors.Is(err, ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord before any note is saved, got %v", err)
	}

	if err := mtnm.Upsert(&MatchTeamNote{MatchID: matchID, TeamID: 1, Notes: sql.NullString{String: "solid win", Valid: true}, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	note, err := mtnm.GetByMatchAndTeam(matchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if note.Notes.String != "solid win" {
		t.Fatalf("expected notes=\"solid win\", got %+v", note)
	}

	// The away team's note is a separate row entirely.
	if _, err := mtnm.GetByMatchAndTeam(matchID, opponentID); !errors.Is(err, ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord for the other team's note, got %v", err)
	}
}

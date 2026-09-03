package models

import (
	"testing"
	"time"
)

func TestMatchAttendanceUpsertInsertAndUpdate(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	mam := MatchAttendanceModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}

	if err := mam.Upsert(&MatchAttendance{MatchID: matchID, PlayerID: playerID, TeamID: 1, Attended: true, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := mam.Upsert(&MatchAttendance{MatchID: matchID, PlayerID: playerID, TeamID: 1, Attended: false, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	list, err := mam.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected upsert to update in place, got %d rows", len(list))
	}
	if list[0].Attended {
		t.Fatalf("expected the second upsert's attended=false to win, got %+v", list[0])
	}
}

// A roster player's attendance defaults to their RSVP: "yes" counts, "no"
// and no-response don't.
func TestMatchesPlayedByTeamSeasonUsesRSVPByDefault(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	rm := RSVPModel{DB: db}
	mam := MatchAttendanceModel{DB: db}

	past := time.Now().AddDate(0, 0, -3)
	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: past})
	if err != nil {
		t.Fatal(err)
	}

	yesID, err := pm.Insert(&Player{FirstName: "Said", LastName: "Yes"})
	if err != nil {
		t.Fatal(err)
	}
	noID, err := pm.Insert(&Player{FirstName: "Said", LastName: "No"})
	if err != nil {
		t.Fatal(err)
	}
	silentID, err := pm.Insert(&Player{FirstName: "Said", LastName: "Nothing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: yesID, TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: noID, TeamID: 1, Status: "no", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	played, err := mam.MatchesPlayedByTeamSeason(1, seasonID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if played[yesID] != 1 {
		t.Errorf("expected the yes-RSVP player to have 1 match played, got %d", played[yesID])
	}
	if _, ok := played[noID]; ok {
		t.Errorf("expected the no-RSVP player to have no matches played, got %d", played[noID])
	}
	if _, ok := played[silentID]; ok {
		t.Errorf("expected the non-responding player to have no matches played, got %d", played[silentID])
	}
}

// An explicit attendance override always wins over the RSVP-derived
// default, in either direction.
func TestMatchesPlayedByTeamSeasonOverrideWinsOverRSVP(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	rm := RSVPModel{DB: db}
	mam := MatchAttendanceModel{DB: db}

	past := time.Now().AddDate(0, 0, -3)
	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: past})
	if err != nil {
		t.Fatal(err)
	}

	// RSVP'd yes but a no-show, corrected after the fact.
	noShowID, err := pm.Insert(&Player{FirstName: "No", LastName: "Show"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: matchID, PlayerID: noShowID, TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := mam.Upsert(&MatchAttendance{MatchID: matchID, PlayerID: noShowID, TeamID: 1, Attended: false, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	// Never RSVP'd (or said no) but showed up as a walk-on.
	walkOnID, err := pm.Insert(&Player{FirstName: "Walk", LastName: "On"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mam.Upsert(&MatchAttendance{MatchID: matchID, PlayerID: walkOnID, TeamID: 1, Attended: true, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	played, err := mam.MatchesPlayedByTeamSeason(1, seasonID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := played[noShowID]; ok {
		t.Errorf("expected the overridden no-show to not count, got %d", played[noShowID])
	}
	if played[walkOnID] != 1 {
		t.Errorf("expected the overridden walk-on to count, got %d", played[walkOnID])
	}
}

// A match that hasn't happened yet (MatchDate on/after asOf) never counts
// toward MP, even if the player already RSVP'd yes.
func TestMatchesPlayedByTeamSeasonExcludesFutureMatches(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	rm := RSVPModel{DB: db}
	mam := MatchAttendanceModel{DB: db}

	asOf := time.Now()
	future, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: asOf.AddDate(0, 0, 3)})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&Player{FirstName: "Future", LastName: "Match"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: future, PlayerID: playerID, TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	played, err := mam.MatchesPlayedByTeamSeason(1, seasonID, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := played[playerID]; ok {
		t.Errorf("expected an upcoming match to never count toward MP, got %d", played[playerID])
	}
}

// MatchesPlayedByPlayerBySeason is the all-time, cross-team counterpart to
// MatchesPlayedByTeamSeason, grouped by season — it must find a player's
// attended matches across every team/season they've ever been on, honoring
// the same override-wins-over-RSVP rule, and tally them per season rather
// than as one combined total.
func TestMatchesPlayedByPlayerBySeason(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	laterSeasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	newTeamID := newTestOpponentTeam(t, db, 1, "New Team FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	rm := RSVPModel{DB: db}
	mam := MatchAttendanceModel{DB: db}

	past := time.Now().AddDate(0, 0, -3)
	playerID, err := pm.Insert(&Player{FirstName: "Journeyman", LastName: "Player"})
	if err != nil {
		t.Fatal(err)
	}

	// Attended (RSVP yes) on the original team, in the earlier season.
	oldTeamMatchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: past})
	if err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: oldTeamMatchID, PlayerID: playerID, TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	// After transferring, a no-show corrected via override on the new
	// team, in the later season.
	noShowMatchID, err := mm.Insert(&Match{SeasonID: laterSeasonID, HomeTeamID: newTeamID, AwayTeamID: opponentID, MatchDate: past})
	if err != nil {
		t.Fatal(err)
	}
	if err := rm.Upsert(&RSVP{MatchID: noShowMatchID, PlayerID: playerID, TeamID: newTeamID, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := mam.Upsert(&MatchAttendance{MatchID: noShowMatchID, PlayerID: playerID, TeamID: newTeamID, Attended: false, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	played, err := mam.MatchesPlayedByPlayerBySeason(playerID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if played[seasonID] != 1 {
		t.Errorf("expected 1 attended match in the earlier season, got %d", played[seasonID])
	}
	if _, ok := played[laterSeasonID]; ok {
		t.Errorf("expected the later season's corrected no-show to not count, got %d", played[laterSeasonID])
	}
}

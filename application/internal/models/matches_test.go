package models

import (
	"database/sql"
	"testing"
	"time"
)

func newTestSeason(t *testing.T, db *sql.DB, leagueID int) int {
	t.Helper()
	sm := SeasonModel{DB: db}
	id, err := sm.Insert(&Season{LeagueID: leagueID, Name: "Test Season"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newTestOpponentTeam(t *testing.T, db *sql.DB, leagueID int, name string) int {
	t.Helper()
	tm := TeamModel{DB: db}
	id, err := tm.Insert(&Team{LeagueID: leagueID, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInsertAndGetMatch(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}

	matchDate := time.Date(2024, 5, 5, 0, 0, 0, 0, time.UTC)
	id, err := mm.Insert(&Match{
		SeasonID:   seasonID,
		HomeTeamID: opponentID,
		AwayTeamID: 1,
		MatchDate:  matchDate,
		HomeScore:  sql.NullInt32{Int32: 2, Valid: true},
		AwayScore:  sql.NullInt32{Int32: 1, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	match, err := mm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if match.HomeTeamID != opponentID || match.AwayTeamID != 1 {
		t.Fatalf("wrong teams! got home=%d away=%d", match.HomeTeamID, match.AwayTeamID)
	}
	if !match.HomeScore.Valid || match.HomeScore.Int32 != 2 {
		t.Fatalf("expected homeScore 2, got %+v", match.HomeScore)
	}
	if !match.MatchDate.Equal(matchDate) {
		t.Fatalf("expected matchDate %v, got %v", matchDate, match.MatchDate)
	}
}

func TestUpdateMatch(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}

	id, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	if err := mm.Update(&Match{
		ID:         id,
		SeasonID:   seasonID,
		HomeTeamID: 1,
		AwayTeamID: opponentID,
		MatchDate:  time.Now(),
		HomeScore:  sql.NullInt32{Int32: 3, Valid: true},
		AwayScore:  sql.NullInt32{Int32: 0, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	match, err := mm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !match.HomeScore.Valid || match.HomeScore.Int32 != 3 {
		t.Fatalf("expected updated homeScore 3, got %+v", match.HomeScore)
	}
}

func TestDeleteMatchCascadesStats(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	pmsm := PlayerMatchStatModel{DB: db}

	matchID, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: matchID, PlayerID: playerID, TeamID: 1, Goals: 2}); err != nil {
		t.Fatal(err)
	}

	if err := mm.Delete(matchID); err != nil {
		t.Fatal(err)
	}

	if _, err := mm.Get(matchID); err != ErrNoRecord {
		t.Fatalf("expected match to be gone, got %v", err)
	}
	stats, err := pmsm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Fatalf("expected stats to cascade-delete, got %d", len(stats))
	}
}

func TestGetBySeasonAndGetByTeamAndSeason(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	otherSeasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	otherTeamID := newTestOpponentTeam(t, db, 1, "Third FC")
	mm := MatchModel{DB: db}

	if _, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: opponentID, AwayTeamID: otherTeamID, MatchDate: time.Date(2024, 5, 8, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&Match{SeasonID: otherSeasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}

	seasonMatches, err := mm.GetBySeason(seasonID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seasonMatches) != 2 {
		t.Fatalf("expected 2 matches in season, got %d", len(seasonMatches))
	}

	teamMatches, err := mm.GetByTeamAndSeason(1, seasonID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teamMatches) != 1 {
		t.Fatalf("expected 1 match for team 1 in season, got %d", len(teamMatches))
	}
}

func TestNextMatchForTeam(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}

	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	future1 := time.Date(2099, 5, 1, 0, 0, 0, 0, time.UTC)
	future2 := time.Date(2099, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: past}); err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: opponentID, AwayTeamID: 1, MatchDate: future2}); err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: future1}); err != nil {
		t.Fatal(err)
	}

	asOf := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := mm.NextMatchForTeam(1, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if !next.MatchDate.Equal(future1) {
		t.Fatalf("expected earliest future match %v, got %v", future1, next.MatchDate)
	}
}

func TestNextMatchForTeamNoneExist(t *testing.T) {
	db := NewTestDB(t)

	mm := MatchModel{DB: db}

	_, err := mm.NextMatchForTeam(1, time.Now())
	if err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestGetSeasonAggregatesByTeam(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentA := newTestOpponentTeam(t, db, 1, "Opponent A")
	opponentB := newTestOpponentTeam(t, db, 1, "Opponent B")
	mm := MatchModel{DB: db}

	// Team 1 (home) beats Opponent A (away) 3-1.
	if _, err := mm.Insert(&Match{
		SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentA, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 3, Valid: true}, AwayScore: sql.NullInt32{Int32: 1, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	// Opponent A (home) draws Team 1 (away) 2-2.
	if _, err := mm.Insert(&Match{
		SeasonID: seasonID, HomeTeamID: opponentA, AwayTeamID: 1, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 2, Valid: true}, AwayScore: sql.NullInt32{Int32: 2, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	// Team 1 (home) loses to Opponent B (away) 0-2.
	if _, err := mm.Insert(&Match{
		SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentB, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 0, Valid: true}, AwayScore: sql.NullInt32{Int32: 2, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	// Unplayed match — must not affect the tally.
	if _, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentB, MatchDate: time.Now()}); err != nil {
		t.Fatal(err)
	}

	aggregates, err := mm.GetSeasonAggregatesByTeam(seasonID)
	if err != nil {
		t.Fatal(err)
	}
	byTeam := map[int]*TeamMatchAggregate{}
	for _, agg := range aggregates {
		byTeam[agg.TeamID] = agg
	}

	team1 := byTeam[1]
	if team1 == nil {
		t.Fatal("expected an aggregate for team 1")
	}
	if team1.Wins != 1 || team1.Losses != 1 || team1.Draws != 1 || team1.GoalsFor != 5 || team1.GoalsAgainst != 5 {
		t.Fatalf("wrong team 1 aggregate: %+v", team1)
	}

	oppA := byTeam[opponentA]
	if oppA == nil || oppA.Wins != 0 || oppA.Losses != 1 || oppA.Draws != 1 || oppA.GoalsFor != 3 || oppA.GoalsAgainst != 5 {
		t.Fatalf("wrong opponent A aggregate: %+v", oppA)
	}

	oppB := byTeam[opponentB]
	if oppB == nil || oppB.Wins != 1 || oppB.Losses != 0 || oppB.Draws != 0 || oppB.GoalsFor != 2 || oppB.GoalsAgainst != 0 {
		t.Fatalf("wrong opponent B aggregate: %+v", oppB)
	}
}

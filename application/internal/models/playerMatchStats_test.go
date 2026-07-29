package models

import (
	"testing"
	"time"
)

func TestUpsertPlayerMatchStatInsertAndUpdate(t *testing.T) {
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

	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: matchID, PlayerID: playerID, TeamID: 1, Goals: 1, Assists: 0}); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: matchID, PlayerID: playerID, TeamID: 1, Goals: 2, Assists: 1}); err != nil {
		t.Fatal(err)
	}

	stats, err := pmsm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected upsert to update in place, got %d rows", len(stats))
	}
	if stats[0].Goals != 2 || stats[0].Assists != 1 {
		t.Fatalf("expected updated stats goals=2 assists=1, got %+v", stats[0])
	}
}

func TestLeaderboardByTeamSeason(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	otherSeasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	pmsm := PlayerMatchStatModel{DB: db}

	striker, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	winger, err := pm.Insert(&Player{FirstName: "Ana", LastName: "Winger"})
	if err != nil {
		t.Fatal(err)
	}

	match1, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	match2, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: opponentID, AwayTeamID: 1, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	otherSeasonMatch, err := mm.Insert(&Match{SeasonID: otherSeasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: match1, PlayerID: striker, TeamID: 1, Goals: 2, Assists: 0}); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: match2, PlayerID: striker, TeamID: 1, Goals: 1, Assists: 1}); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: match1, PlayerID: winger, TeamID: 1, Goals: 0, Assists: 3}); err != nil {
		t.Fatal(err)
	}
	// A different season's stat line should not be included.
	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: otherSeasonMatch, PlayerID: striker, TeamID: 1, Goals: 99, Assists: 99}); err != nil {
		t.Fatal(err)
	}

	board, err := pmsm.LeaderboardByTeamSeason(1, seasonID)
	if err != nil {
		t.Fatal(err)
	}
	if len(board) != 2 {
		t.Fatalf("expected 2 players on the leaderboard, got %d", len(board))
	}
	if board[0].PlayerID != striker || board[0].Goals != 3 {
		t.Fatalf("expected striker leading with 3 goals, got %+v", board[0])
	}
	if board[1].PlayerID != winger || board[1].Assists != 3 {
		t.Fatalf("expected winger with 3 assists, got %+v", board[1])
	}
}

func TestTopScorersAndAssistersForSeason(t *testing.T) {
	db := NewTestDB(t)

	seasonID := newTestSeason(t, db, 1)
	opponentID := newTestOpponentTeam(t, db, 1, "Rival FC")
	mm := MatchModel{DB: db}
	pm := PlayerModel{DB: db}
	pmsm := PlayerMatchStatModel{DB: db}

	striker, err := pm.Insert(&Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	winger, err := pm.Insert(&Player{FirstName: "Ana", LastName: "Winger"})
	if err != nil {
		t.Fatal(err)
	}
	rivalPlayer, err := pm.Insert(&Player{FirstName: "Rory", LastName: "Rival"})
	if err != nil {
		t.Fatal(err)
	}
	quietPlayer, err := pm.Insert(&Player{FirstName: "Quiet", LastName: "Bench"})
	if err != nil {
		t.Fatal(err)
	}

	match1, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: match1, PlayerID: striker, TeamID: 1, Goals: 3, Assists: 0}); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: match1, PlayerID: winger, TeamID: 1, Goals: 1, Assists: 2}); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: match1, PlayerID: rivalPlayer, TeamID: opponentID, Goals: 2, Assists: 1}); err != nil {
		t.Fatal(err)
	}
	// A player with no goals/assists shouldn't clutter either leaders table.
	if err := pmsm.Upsert(&PlayerMatchStat{MatchID: match1, PlayerID: quietPlayer, TeamID: 1, Goals: 0, Assists: 0}); err != nil {
		t.Fatal(err)
	}

	scorers, err := pmsm.TopScorersForSeason(seasonID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(scorers) != 3 {
		t.Fatalf("expected 3 scorers (quiet bench excluded), got %d: %+v", len(scorers), scorers)
	}
	if scorers[0].PlayerID != striker || scorers[0].Total != 3 || scorers[0].TeamName != "Test Team" {
		t.Fatalf("expected striker leading with 3 goals for Test Team, got %+v", scorers[0])
	}

	assisters, err := pmsm.TopAssistersForSeason(seasonID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(assisters) != 2 {
		t.Fatalf("expected 2 assisters, got %d: %+v", len(assisters), assisters)
	}
	if assisters[0].PlayerID != winger || assisters[0].Total != 2 {
		t.Fatalf("expected winger leading with 2 assists, got %+v", assisters[0])
	}

	limited, err := pmsm.TopScorersForSeason(seasonID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected limit to cap results at 1, got %d", len(limited))
	}
}

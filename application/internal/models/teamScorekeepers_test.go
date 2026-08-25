package models

import "testing"

func TestAddAndRemoveScorekeeper(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tsm := TeamScorekeeperModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := tsm.AddScorekeeper(playerID, 1); err != nil {
		t.Fatal(err)
	}

	isScorekeeper, err := tsm.IsScorekeeper(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isScorekeeper {
		t.Fatal("expected player to be a scorekeeper of team 1")
	}

	if err := tsm.RemoveScorekeeper(playerID, 1); err != nil {
		t.Fatal(err)
	}

	isScorekeeper, err = tsm.IsScorekeeper(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if isScorekeeper {
		t.Fatal("expected scorekeeper rights to be removed")
	}
}

func TestAddScorekeeperDuplicate(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tsm := TeamScorekeeperModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := tsm.AddScorekeeper(playerID, 1); err != nil {
		t.Fatal(err)
	}

	if err := tsm.AddScorekeeper(playerID, 1); err != ErrDuplicateScorekeeper {
		t.Fatalf("expected ErrDuplicateScorekeeper, got %v", err)
	}
}

func TestGetTeamIDsForPlayer(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tm := TeamModel{DB: db}
	tsm := TeamScorekeeperModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	secondTeamID, err := tm.Insert(&Team{LeagueID: 1, Name: "Second Team"})
	if err != nil {
		t.Fatal(err)
	}

	if err := tsm.AddScorekeeper(playerID, 1); err != nil {
		t.Fatal(err)
	}
	if err := tsm.AddScorekeeper(playerID, secondTeamID); err != nil {
		t.Fatal(err)
	}

	teamIDs, err := tsm.GetTeamIDsForPlayer(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teamIDs) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teamIDs))
	}
}

func TestListForTeam(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tsm := TeamScorekeeperModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	scorekeepers, err := tsm.ListForTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(scorekeepers) != 0 {
		t.Fatalf("expected 0 scorekeepers before AddScorekeeper, got %d", len(scorekeepers))
	}

	if err := tsm.AddScorekeeper(playerID, 1); err != nil {
		t.Fatal(err)
	}

	scorekeepers, err = tsm.ListForTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(scorekeepers) != 1 || scorekeepers[0].ID != playerID {
		t.Fatalf("expected [%d], got %v", playerID, scorekeepers)
	}
}

func TestDeleteAllScorekeepersForPlayer(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tm := TeamModel{DB: db}
	tsm := TeamScorekeeperModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	secondTeamID, err := tm.Insert(&Team{LeagueID: 1, Name: "Second Team"})
	if err != nil {
		t.Fatal(err)
	}

	if err := tsm.AddScorekeeper(playerID, 1); err != nil {
		t.Fatal(err)
	}
	if err := tsm.AddScorekeeper(playerID, secondTeamID); err != nil {
		t.Fatal(err)
	}

	if err := tsm.DeleteAllForPlayer(playerID); err != nil {
		t.Fatal(err)
	}

	teamIDs, err := tsm.GetTeamIDsForPlayer(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teamIDs) != 0 {
		t.Fatalf("expected 0 teams after DeleteAllForPlayer, got %d", len(teamIDs))
	}
}

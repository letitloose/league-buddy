package models

import "testing"

func TestAddAndRemoveMembership(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tmm := TeamMemberModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	isMember, err := tmm.IsMember(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected player to be a member of team 1")
	}

	if err := tmm.RemoveMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	isMember, err = tmm.IsMember(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if isMember {
		t.Fatal("expected membership to be removed")
	}
}

func TestAddMembershipDuplicate(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tmm := TeamMemberModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	if err := tmm.AddMembership(playerID, 1); err != ErrDuplicateMembership {
		t.Fatalf("expected ErrDuplicateMembership, got %v", err)
	}
}

func TestHasTeamInLeague(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tm := TeamModel{DB: db}
	tmm := TeamMemberModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	hasTeam, err := tmm.HasTeamInLeague(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if hasTeam {
		t.Fatal("expected no team in league 1 yet")
	}

	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	hasTeam, err = tmm.HasTeamInLeague(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTeam {
		t.Fatal("expected player to have a team in league 1")
	}

	// A second league should be unaffected by membership in the first.
	secondLeagueID, err := (&LeagueModel{DB: db}).Insert(&League{Name: "Second League"})
	if err != nil {
		t.Fatal(err)
	}
	hasTeam, err = tmm.HasTeamInLeague(playerID, secondLeagueID)
	if err != nil {
		t.Fatal(err)
	}
	if hasTeam {
		t.Fatal("expected no team in the second league")
	}

	// Sanity check that team 1 actually belongs to league 1, matching the
	// assumption above.
	team, err := tm.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if team.LeagueID != 1 {
		t.Fatalf("expected seeded team 1 to be in league 1, got league %d", team.LeagueID)
	}
}

func TestSetLegendStatus(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tmm := TeamMemberModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Old", LastName: "Timer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	if err := tmm.SetLegendStatus(playerID, 1, true); err != nil {
		t.Fatal(err)
	}
	active, err := pm.GetActiveByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range active {
		if p.ID == playerID {
			t.Fatal("expected the player to no longer appear in the active roster after being made a Legend")
		}
	}
	legends, err := pm.GetLegendsByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(legends) != 1 || legends[0].ID != playerID {
		t.Fatalf("expected the player to appear as the sole Legend, got %+v", legends)
	}

	// Reactivating moves them back.
	if err := tmm.SetLegendStatus(playerID, 1, false); err != nil {
		t.Fatal(err)
	}
	active, err = pm.GetActiveByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range active {
		if p.ID == playerID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the player to be back on the active roster after reactivation")
	}
	legends, err = pm.GetLegendsByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(legends) != 0 {
		t.Fatalf("expected no Legends left after reactivation, got %+v", legends)
	}

	// Membership itself (IsMember) is unaffected by Legend status either way.
	isMember, err := tmm.IsMember(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected Legend status to never affect underlying team membership")
	}
}

func TestGetTeamsForPlayer(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	lm := LeagueModel{DB: db}
	tm := TeamModel{DB: db}
	tmm := TeamMemberModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	secondLeagueID, err := lm.Insert(&League{Name: "Second League"})
	if err != nil {
		t.Fatal(err)
	}
	secondTeamID, err := tm.Insert(&Team{LeagueID: secondLeagueID, Name: "Second Team"})
	if err != nil {
		t.Fatal(err)
	}

	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, secondTeamID); err != nil {
		t.Fatal(err)
	}

	teams, err := tmm.GetTeamsForPlayer(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
}

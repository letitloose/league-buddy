package models

import "testing"

func TestAddAndRemoveLeagueAdmin(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	lam := LeagueAdminModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := lam.AddAdmin(playerID, 1); err != nil {
		t.Fatal(err)
	}

	isAdmin, err := lam.IsLeagueAdmin(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isAdmin {
		t.Fatal("expected player to administer league 1")
	}

	if err := lam.RemoveAdmin(playerID, 1); err != nil {
		t.Fatal(err)
	}

	isAdmin, err = lam.IsLeagueAdmin(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if isAdmin {
		t.Fatal("expected admin rights to be removed")
	}
}

func TestAddLeagueAdminDuplicate(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	lam := LeagueAdminModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := lam.AddAdmin(playerID, 1); err != nil {
		t.Fatal(err)
	}

	if err := lam.AddAdmin(playerID, 1); err != ErrDuplicateLeagueAdmin {
		t.Fatalf("expected ErrDuplicateLeagueAdmin, got %v", err)
	}
}

func TestGetLeaguesForPlayer(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	lm := LeagueModel{DB: db}
	lam := LeagueAdminModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	secondLeagueID, err := lm.Insert(&League{Name: "Second League"})
	if err != nil {
		t.Fatal(err)
	}

	if err := lam.AddAdmin(playerID, 1); err != nil {
		t.Fatal(err)
	}
	if err := lam.AddAdmin(playerID, secondLeagueID); err != nil {
		t.Fatal(err)
	}

	leagues, err := lam.GetLeaguesForPlayer(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(leagues) != 2 {
		t.Fatalf("expected 2 leagues, got %d", len(leagues))
	}
}

func TestListAdminsForLeague(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	lam := LeagueAdminModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	admins, err := lam.ListAdminsForLeague(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(admins) != 0 {
		t.Fatalf("expected 0 admins before AddAdmin, got %d", len(admins))
	}

	if err := lam.AddAdmin(playerID, 1); err != nil {
		t.Fatal(err)
	}

	admins, err = lam.ListAdminsForLeague(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(admins) != 1 || admins[0].ID != playerID {
		t.Fatalf("expected [%d], got %v", playerID, admins)
	}
}

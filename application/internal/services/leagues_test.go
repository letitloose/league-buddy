package services

import (
	"database/sql"
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestAddLeagueAdmin(t *testing.T) {
	db := models.NewTestDB(t)

	leagues := &models.LeagueModel{DB: db}
	leagueService := LeagueService{LeagueModel: leagues, DB: db}
	players := &models.PlayerModel{DB: db}
	lam := &models.LeagueAdminModel{DB: db}

	playerID, err := players.Insert(&models.Player{FirstName: "League", LastName: "Admin", Email: sql.NullString{String: "league-admin-svc@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	if err := leagueService.AddLeagueAdmin(1, "league-admin-svc@example.com", "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	isAdmin, err := lam.IsLeagueAdmin(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isAdmin {
		t.Fatal("expected player to administer league 1")
	}
}

func TestAddLeagueAdminUnknownEmail(t *testing.T) {
	db := models.NewTestDB(t)

	leagues := &models.LeagueModel{DB: db}
	leagueService := LeagueService{LeagueModel: leagues, DB: db}

	err := leagueService.AddLeagueAdmin(1, "no-such-player@example.com", "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestRemoveLeagueAdmin(t *testing.T) {
	db := models.NewTestDB(t)

	leagues := &models.LeagueModel{DB: db}
	leagueService := LeagueService{LeagueModel: leagues, DB: db}
	players := &models.PlayerModel{DB: db}
	lam := &models.LeagueAdminModel{DB: db}

	playerID, err := players.Insert(&models.Player{FirstName: "League", LastName: "Admin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := lam.AddAdmin(playerID, 1); err != nil {
		t.Fatal(err)
	}

	if err := leagueService.RemoveLeagueAdmin(1, playerID, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	isAdmin, err := lam.IsLeagueAdmin(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if isAdmin {
		t.Fatal("expected admin rights to be removed")
	}
}

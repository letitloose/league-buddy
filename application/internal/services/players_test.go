package services

import (
	"database/sql"
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestAddPlayer(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	form := &PlayerForm{
		FirstName: "Lou",
		LastName:  "Garwood",
		Address1:  "123 Main St",
		City:      "Troy",
		Email:     "lou@example.com",
	}

	id, err := playerService.AddPlayer(1, form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	player, err := players.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Lou"
	if player.FirstName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, player.FirstName)
	}
	if !player.AddressID.Valid {
		t.Fatal("expected an address to have been created")
	}
	if !player.TeamID.Valid || player.TeamID.Int32 != 1 {
		t.Fatalf("wrong teamID! expected 1 but got %+v", player.TeamID)
	}
}

func TestAddPlayerMissingRequiredFields(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	form := &PlayerForm{}

	_, err := playerService.AddPlayer(1, form, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestAddPlayerBadTeam(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	form := &PlayerForm{FirstName: "Lou", LastName: "Garwood"}

	_, err := playerService.AddPlayer(9999, form, "admin@example.com")
	if err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestUpdatePlayer(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Lou", LastName: "Garwood"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	form := &PlayerForm{ID: id, FirstName: "Lou", LastName: "Buddy", Address1: "456 Oak Ave", City: "Albany"}
	if err := playerService.UpdatePlayer(form, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	player, err := players.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Buddy"
	if player.LastName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, player.LastName)
	}
	if !player.AddressID.Valid {
		t.Fatal("expected an address to have been created on update")
	}
}

func TestDeletePlayer(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Lou", LastName: "Garwood"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := playerService.DeletePlayer(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	_, err = players.Get(id)
	if err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestDeletePlayerClearsCaptaincy(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}
	teams := &models.TeamModel{DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Cap", LastName: "Tain"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := teams.SetCaptain(1, sql.NullInt32{Int32: int32(id), Valid: true}); err != nil {
		t.Fatal(err)
	}

	if err := playerService.DeletePlayer(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	team, err := teams.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if team.CaptainPlayerID.Valid {
		t.Fatalf("expected captain cleared after delete, got %+v", team.CaptainPlayerID)
	}
}

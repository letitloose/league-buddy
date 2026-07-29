package models

import (
	"database/sql"
	"testing"
	"time"
)

func TestInsertTeam(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}

	id, err := tm.Insert(&Team{LeagueID: 1, Name: "Second Team"})
	if err != nil {
		t.Fatal(err)
	}

	team, err := tm.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Second Team"
	if team.Name != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, team.Name)
	}
	if team.LeagueID != 1 {
		t.Fatalf("wrong league! expected 1 but got %d", team.LeagueID)
	}
}

func TestGetTeam(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}

	team, err := tm.Get(1)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Test Team"
	if team.Name != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, team.Name)
	}
}

func TestUpdateTeamWithMottoAndEstablishedDate(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}

	established := sql.NullTime{Time: time.Date(2001, 5, 6, 0, 0, 0, 0, time.UTC), Valid: true}

	err := tm.Update(&Team{
		ID:              1,
		LeagueID:        1,
		Name:            "Test Team",
		Motto:           sql.NullString{String: "Go get 'em", Valid: true},
		EstablishedDate: established,
	})
	if err != nil {
		t.Fatal(err)
	}

	team, err := tm.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if !team.Motto.Valid || team.Motto.String != "Go get 'em" {
		t.Fatalf("expected motto to be set, got %+v", team.Motto)
	}
	if !team.EstablishedDate.Valid {
		t.Fatal("expected establishedDate to be set")
	}
}

func TestSetCaptain(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}
	pm := PlayerModel{DB: db}
	tmm := TeamMemberModel{DB: db}

	player := &Player{
		FirstName: "Cap",
		LastName:  "Tain",
	}
	playerID, err := pm.Insert(player)
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	if err := tm.SetCaptain(1, sql.NullInt32{Int32: int32(playerID), Valid: true}); err != nil {
		t.Fatal(err)
	}

	team, err := tm.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if !team.CaptainPlayerID.Valid || int(team.CaptainPlayerID.Int32) != playerID {
		t.Fatalf("expected captain %d, got %+v", playerID, team.CaptainPlayerID)
	}

	if err := tm.ClearCaptainByPlayer(playerID); err != nil {
		t.Fatal(err)
	}

	team, err = tm.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if team.CaptainPlayerID.Valid {
		t.Fatalf("expected captain cleared, got %+v", team.CaptainPlayerID)
	}
}

func TestGetByLocation(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}
	lm := LocationModel{DB: db}
	tm := TeamModel{DB: db}

	addressID, err := am.Insert(&Address{
		Address1: sql.NullString{String: "1 Main St", Valid: true},
		City:     sql.NullString{String: "Troy", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	locationID, err := lm.Insert(&Location{Name: "Shared Field", AddressID: addressID, AddressKey: "1 main st||troy||"})
	if err != nil {
		t.Fatal(err)
	}

	teams, err := tm.GetByLocation(locationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 0 {
		t.Fatalf("expected 0 teams before assignment, got %d", len(teams))
	}

	if err := tm.Update(&Team{ID: 1, LeagueID: 1, Name: "Test Team", LocationID: sql.NullInt32{Int32: int32(locationID), Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tm.Insert(&Team{LeagueID: 1, Name: "Second Team", LocationID: sql.NullInt32{Int32: int32(locationID), Valid: true}}); err != nil {
		t.Fatal(err)
	}

	teams, err = tm.GetByLocation(locationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams sharing the location, got %d", len(teams))
	}
	names := map[string]bool{teams[0].Name: true, teams[1].Name: true}
	if !names["Test Team"] || !names["Second Team"] {
		t.Fatalf("expected Test Team and Second Team, got %v", teams)
	}
}

func TestDeleteTeamWithDependentsFails(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}
	pm := PlayerModel{DB: db}
	tmm := TeamMemberModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	err = tm.Delete(1)
	if err != ErrHasDependents {
		t.Fatalf("expected ErrHasDependents, got %v", err)
	}
}

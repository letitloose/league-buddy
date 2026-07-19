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

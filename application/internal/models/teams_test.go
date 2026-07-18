package models

import (
	"database/sql"
	"testing"
)

func TestInsertTeam(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}

	id, err := tm.Insert(1, "Second Team")
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

func TestSetCaptain(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}
	pm := PlayerModel{DB: db}

	player := &Player{
		TeamID:    sql.NullInt32{Int32: 1, Valid: true},
		FirstName: "Cap",
		LastName:  "Tain",
	}
	playerID, err := pm.Insert(player)
	if err != nil {
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

	if _, err := pm.Insert(&Player{TeamID: sql.NullInt32{Int32: 1, Valid: true}, FirstName: "Lou", LastName: "Garwood"}); err != nil {
		t.Fatal(err)
	}

	err := tm.Delete(1)
	if err != ErrHasDependents {
		t.Fatalf("expected ErrHasDependents, got %v", err)
	}
}

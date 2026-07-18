package models

import (
	"database/sql"
	"testing"
)

func TestInsertPlayer(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{
		TeamID:    sql.NullInt32{Int32: 1, Valid: true},
		FirstName: "Lou",
		LastName:  "Garwood",
		Email:     sql.NullString{String: "lou@example.com", Valid: true},
	}

	id, err := pm.Insert(player)
	if err != nil {
		t.Fatal(err)
	}

	got, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Lou"
	if got.FirstName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, got.FirstName)
	}
	if !got.TeamID.Valid || got.TeamID.Int32 != 1 {
		t.Fatalf("wrong teamID! expected 1 but got %+v", got.TeamID)
	}
}

func TestInsertPlayerWithoutTeam(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{
		FirstName: "Unaffiliated",
		LastName:  "Player",
		Email:     sql.NullString{String: "unaffiliated@example.com", Valid: true},
	}

	id, err := pm.Insert(player)
	if err != nil {
		t.Fatal(err)
	}

	got, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	if got.TeamID.Valid {
		t.Fatalf("expected no team, got %+v", got.TeamID)
	}
}

func TestSetTeam(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{FirstName: "Lou", LastName: "Garwood"}
	id, err := pm.Insert(player)
	if err != nil {
		t.Fatal(err)
	}

	if err := pm.SetTeam(id, 1); err != nil {
		t.Fatal(err)
	}

	got, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.TeamID.Valid || got.TeamID.Int32 != 1 {
		t.Fatalf("wrong teamID! expected 1 but got %+v", got.TeamID)
	}
}

func TestUpdatePlayer(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{TeamID: sql.NullInt32{Int32: 1, Valid: true}, FirstName: "Lou", LastName: "Garwood"}
	id, err := pm.Insert(player)
	if err != nil {
		t.Fatal(err)
	}

	player.ID = id
	player.LastName = "Buddy"
	if err := pm.Update(player); err != nil {
		t.Fatal(err)
	}

	got, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Buddy"
	if got.LastName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, got.LastName)
	}
}

func TestGetPlayerByEmail(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{
		TeamID:    sql.NullInt32{Int32: 1, Valid: true},
		FirstName: "Lou",
		LastName:  "Garwood",
		Email:     sql.NullString{String: "lou@example.com", Valid: true},
	}
	if _, err := pm.Insert(player); err != nil {
		t.Fatal(err)
	}

	got, err := pm.GetByEmail("lou@example.com")
	if err != nil {
		t.Fatal(err)
	}

	expected := "Lou"
	if got.FirstName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, got.FirstName)
	}

	_, err = pm.GetByEmail("nope@example.com")
	if err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestGetPlayersByTeam(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	if _, err := pm.Insert(&Player{TeamID: sql.NullInt32{Int32: 1, Valid: true}, FirstName: "Lou", LastName: "Garwood"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.Insert(&Player{TeamID: sql.NullInt32{Int32: 1, Valid: true}, FirstName: "Ada", LastName: "Lovelace"}); err != nil {
		t.Fatal(err)
	}

	players, err := pm.GetByTeam(1)
	if err != nil {
		t.Fatal(err)
	}

	expected := 2
	if len(players) != expected {
		t.Fatalf("wrong number of results. expecting %d, got %d", expected, len(players))
	}
}

func TestSearchPlayers(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	if _, err := pm.Insert(&Player{TeamID: sql.NullInt32{Int32: 1, Valid: true}, FirstName: "Lou", LastName: "Garwood"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.Insert(&Player{TeamID: sql.NullInt32{Int32: 1, Valid: true}, FirstName: "Ada", LastName: "Lovelace"}); err != nil {
		t.Fatal(err)
	}

	results, err := pm.Search(&PlayerSearchCriteria{TeamID: 1, FirstName: "lou", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}

	expected := 1
	if len(results) != expected {
		t.Fatalf("wrong number of results. expecting %d, got %d", expected, len(results))
	}

	expectedName := "Lou"
	if results[0].FirstName != expectedName {
		t.Fatalf("wrong player returned. expecting %s, got %s", expectedName, results[0].FirstName)
	}
}

package models

import (
	"database/sql"
	"testing"
)

func TestInsertPlayer(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}
	team, err := tm.GetDefault()
	if err != nil {
		t.Fatal(err)
	}

	pm := PlayerModel{DB: db}
	player := &Player{
		TeamID:    team.ID,
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
}

func TestUpdatePlayer(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}
	team, err := tm.GetDefault()
	if err != nil {
		t.Fatal(err)
	}

	pm := PlayerModel{DB: db}
	player := &Player{TeamID: team.ID, FirstName: "Lou", LastName: "Garwood"}
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

	tm := TeamModel{DB: db}
	team, err := tm.GetDefault()
	if err != nil {
		t.Fatal(err)
	}

	pm := PlayerModel{DB: db}
	player := &Player{
		TeamID:    team.ID,
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

	tm := TeamModel{DB: db}
	team, err := tm.GetDefault()
	if err != nil {
		t.Fatal(err)
	}

	pm := PlayerModel{DB: db}
	if _, err := pm.Insert(&Player{TeamID: team.ID, FirstName: "Lou", LastName: "Garwood"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.Insert(&Player{TeamID: team.ID, FirstName: "Ada", LastName: "Lovelace"}); err != nil {
		t.Fatal(err)
	}

	players, err := pm.GetByTeam(team.ID)
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

	tm := TeamModel{DB: db}
	team, err := tm.GetDefault()
	if err != nil {
		t.Fatal(err)
	}

	pm := PlayerModel{DB: db}
	if _, err := pm.Insert(&Player{TeamID: team.ID, FirstName: "Lou", LastName: "Garwood"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.Insert(&Player{TeamID: team.ID, FirstName: "Ada", LastName: "Lovelace"}); err != nil {
		t.Fatal(err)
	}

	results, err := pm.Search(&PlayerSearchCriteria{TeamID: team.ID, FirstName: "lou", Limit: 20})
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

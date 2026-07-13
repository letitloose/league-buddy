package models

import (
	"testing"
)

func TestInsertTeam(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}

	id, err := tm.Insert("Second Team")
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
}

func TestGetDefaultTeam(t *testing.T) {
	db := NewTestDB(t)

	tm := TeamModel{DB: db}

	team, err := tm.GetDefault()
	if err != nil {
		t.Fatal(err)
	}

	expected := "Test Team"
	if team.Name != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, team.Name)
	}
}

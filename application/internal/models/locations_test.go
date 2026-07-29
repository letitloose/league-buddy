package models

import (
	"database/sql"
	"testing"
)

func TestInsertAndGetLocation(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}
	lm := LocationModel{DB: db}

	addressID, err := am.Insert(&Address{
		Address1: sql.NullString{String: "100 Phillips Rd", Valid: true},
		City:     sql.NullString{String: "East Greenbush", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	id, err := lm.Insert(&Location{Name: "East Greenbush Soccer Club", AddressID: addressID, AddressKey: "100 phillips rd||east greenbush||"})
	if err != nil {
		t.Fatal(err)
	}

	location, err := lm.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "East Greenbush Soccer Club"
	if location.Name != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, location.Name)
	}
	if location.AddressID != addressID {
		t.Fatalf("wrong addressID! expected %d but got %d", addressID, location.AddressID)
	}
}

func TestInsertLocationDuplicateAddressKeyFails(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}
	lm := LocationModel{DB: db}

	addressID, err := am.Insert(&Address{Address1: sql.NullString{String: "1 Main St", Valid: true}, City: sql.NullString{String: "Troy", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lm.Insert(&Location{Name: "First", AddressID: addressID, AddressKey: "1 main st||troy||"}); err != nil {
		t.Fatal(err)
	}

	secondAddressID, err := am.Insert(&Address{Address1: sql.NullString{String: "1 Main St", Valid: true}, City: sql.NullString{String: "Troy", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = lm.Insert(&Location{Name: "Duplicate", AddressID: secondAddressID, AddressKey: "1 main st||troy||"})
	if err != ErrDuplicateLocation {
		t.Fatalf("expected ErrDuplicateLocation, got %v", err)
	}
}

func TestGetByAddressKey(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}
	lm := LocationModel{DB: db}

	addressID, err := am.Insert(&Address{Address1: sql.NullString{String: "1 Main St", Valid: true}, City: sql.NullString{String: "Troy", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := lm.Insert(&Location{Name: "Field", AddressID: addressID, AddressKey: "1 main st||troy||"})
	if err != nil {
		t.Fatal(err)
	}

	found, err := lm.GetByAddressKey("1 main st||troy||")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != id {
		t.Fatalf("expected location %d, got %d", id, found.ID)
	}

	_, err = lm.GetByAddressKey("no such key")
	if err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestUpdateLocation(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}
	lm := LocationModel{DB: db}

	addressID, err := am.Insert(&Address{
		Address1: sql.NullString{String: "1 Main St", Valid: true},
		City:     sql.NullString{String: "Troy", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := lm.Insert(&Location{Name: "Old Name", AddressID: addressID, AddressKey: "1 main st||troy||"})
	if err != nil {
		t.Fatal(err)
	}

	if err := lm.Update(&Location{ID: id, Name: "New Name", AddressID: addressID, AddressKey: "1 main st||troy||"}); err != nil {
		t.Fatal(err)
	}

	location, err := lm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if location.Name != "New Name" {
		t.Fatalf("expected updated name, got %s", location.Name)
	}
}

func TestUpdateLocationDuplicateAddressKeyFails(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}
	lm := LocationModel{DB: db}

	addressID1, err := am.Insert(&Address{Address1: sql.NullString{String: "1 Main St", Valid: true}, City: sql.NullString{String: "Troy", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lm.Insert(&Location{Name: "First", AddressID: addressID1, AddressKey: "1 main st||troy||"}); err != nil {
		t.Fatal(err)
	}

	addressID2, err := am.Insert(&Address{Address1: sql.NullString{String: "2 Main St", Valid: true}, City: sql.NullString{String: "Troy", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := lm.Insert(&Location{Name: "Second", AddressID: addressID2, AddressKey: "2 main st||troy||"})
	if err != nil {
		t.Fatal(err)
	}

	// Editing the second location to the first location's address should fail.
	err = lm.Update(&Location{ID: secondID, Name: "Second", AddressID: addressID2, AddressKey: "1 main st||troy||"})
	if err != ErrDuplicateLocation {
		t.Fatalf("expected ErrDuplicateLocation, got %v", err)
	}
}

func TestDeleteLocationWithDependentsFails(t *testing.T) {
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
	id, err := lm.Insert(&Location{Name: "Busy Field", AddressID: addressID, AddressKey: "1 main st||troy||"})
	if err != nil {
		t.Fatal(err)
	}

	if err := tm.Update(&Team{ID: 1, LeagueID: 1, Name: "Test Team", LocationID: sql.NullInt32{Int32: int32(id), Valid: true}}); err != nil {
		t.Fatal(err)
	}

	err = lm.Delete(id)
	if err != ErrHasDependents {
		t.Fatalf("expected ErrHasDependents, got %v", err)
	}
}

func TestListLocations(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}
	lm := LocationModel{DB: db}

	locations, err := lm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 0 {
		t.Fatalf("expected 0 locations initially, got %d", len(locations))
	}

	addressID, err := am.Insert(&Address{
		Address1: sql.NullString{String: "1 Main St", Valid: true},
		City:     sql.NullString{String: "Troy", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lm.Insert(&Location{Name: "A Field", AddressID: addressID, AddressKey: "1 main st||troy||"}); err != nil {
		t.Fatal(err)
	}

	locations, err = lm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locations))
	}
}

package models

import (
	"database/sql"
	"testing"
)

func TestInsertAddress(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}

	address := &Address{
		Address1: sql.NullString{String: "123 Main St", Valid: true},
		City:     sql.NullString{String: "Troy", Valid: true},
		Country:  sql.NullString{String: "USA", Valid: true},
	}

	id, err := am.Insert(address)
	if err != nil {
		t.Fatal(err)
	}

	got, err := am.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "123 Main St"
	if got.Address1.String != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, got.Address1.String)
	}
}

func TestUpdateAddress(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}

	address := &Address{
		Address1: sql.NullString{String: "123 Main St", Valid: true},
		City:     sql.NullString{String: "Troy", Valid: true},
	}

	id, err := am.Insert(address)
	if err != nil {
		t.Fatal(err)
	}

	address.ID = id
	address.City = sql.NullString{String: "Albany", Valid: true}
	if _, err := am.Update(address); err != nil {
		t.Fatal(err)
	}

	got, err := am.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Albany"
	if got.City.String != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, got.City.String)
	}
}

func TestDeleteAddress(t *testing.T) {
	db := NewTestDB(t)

	am := AddressModel{DB: db}

	address := &Address{
		Address1: sql.NullString{String: "123 Main St", Valid: true},
		City:     sql.NullString{String: "Troy", Valid: true},
	}
	id, err := am.Insert(address)
	if err != nil {
		t.Fatal(err)
	}

	if err := am.Delete(id); err != nil {
		t.Fatal(err)
	}

	_, err = am.Get(id)
	if err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

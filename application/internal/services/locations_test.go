package services

import (
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestCreateLocation(t *testing.T) {
	db := models.NewTestDB(t)

	locations := &models.LocationModel{DB: db}
	locationService := LocationService{LocationModel: locations, DB: db}

	form := &LocationForm{
		Name:          "East Greenbush Soccer Club",
		Address1:      "100 Phillips Rd",
		City:          "East Greenbush",
		StateProvince: "NY",
	}

	id, err := locationService.CreateLocation(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	location, err := locations.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "East Greenbush Soccer Club"
	if location.Name != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, location.Name)
	}

	addresses := &models.AddressModel{DB: db}
	address, err := addresses.Get(location.AddressID)
	if err != nil {
		t.Fatal(err)
	}
	if address.Address1.String != "100 Phillips Rd" {
		t.Fatalf("wrong address! expected 100 Phillips Rd, got %s", address.Address1.String)
	}
}

// TestCreateLocationDedupesExistingAddress is the core of the "team captains
// can add a new home field without creating duplicates" feature: creating a
// second location at the same address (different case/whitespace, and even
// a different name) must transparently resolve to the first location's ID,
// not create a new row, and must not leave the speculatively-inserted
// address row behind.
func TestCreateLocationDedupesExistingAddress(t *testing.T) {
	db := models.NewTestDB(t)

	locations := &models.LocationModel{DB: db}
	locationService := LocationService{LocationModel: locations, DB: db}

	firstID, err := locationService.CreateLocation(&LocationForm{
		Name: "East Greenbush Soccer Club", Address1: "100 Phillips Rd", City: "East Greenbush", StateProvince: "NY",
	}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	secondID, err := locationService.CreateLocation(&LocationForm{
		Name: "Some Other Name", Address1: "  100 PHILLIPS RD  ", City: "  East Greenbush ", StateProvince: "ny",
	}, "captain@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if secondID != firstID {
		t.Fatalf("expected the duplicate address to resolve to location %d, got %d", firstID, secondID)
	}

	all, err := locations.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected exactly 1 location after the dedup, got %d", len(all))
	}
	if all[0].Name != "East Greenbush Soccer Club" {
		t.Fatalf("expected the original location's name to win, got %q", all[0].Name)
	}
}

func TestCreateLocationMissingRequiredFields(t *testing.T) {
	db := models.NewTestDB(t)

	locations := &models.LocationModel{DB: db}
	locationService := LocationService{LocationModel: locations, DB: db}

	_, err := locationService.CreateLocation(&LocationForm{}, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestUpdateLocation(t *testing.T) {
	db := models.NewTestDB(t)

	locations := &models.LocationModel{DB: db}
	locationService := LocationService{LocationModel: locations, DB: db}

	id, err := locationService.CreateLocation(&LocationForm{Name: "Old Field", Address1: "1 Main St", City: "Troy"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	err = locationService.UpdateLocation(&LocationForm{ID: id, Name: "New Field", Address1: "2 Main St", City: "Troy"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	location, err := locations.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if location.Name != "New Field" {
		t.Fatalf("expected updated name, got %s", location.Name)
	}

	addresses := &models.AddressModel{DB: db}
	address, err := addresses.Get(location.AddressID)
	if err != nil {
		t.Fatal(err)
	}
	if address.Address1.String != "2 Main St" {
		t.Fatalf("expected updated address, got %s", address.Address1.String)
	}
}

func TestUpdateLocationRejectsCollidingAddress(t *testing.T) {
	db := models.NewTestDB(t)

	locations := &models.LocationModel{DB: db}
	locationService := LocationService{LocationModel: locations, DB: db}

	if _, err := locationService.CreateLocation(&LocationForm{Name: "Field A", Address1: "1 Main St", City: "Troy"}, "admin@example.com"); err != nil {
		t.Fatal(err)
	}
	fieldBID, err := locationService.CreateLocation(&LocationForm{Name: "Field B", Address1: "2 Main St", City: "Troy"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	form := &LocationForm{ID: fieldBID, Name: "Field B", Address1: "1 Main St", City: "Troy"}
	err = locationService.UpdateLocation(form, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
	if len(form.NonFieldErrors) == 0 {
		t.Fatal("expected a non-field error explaining the collision")
	}
}

func TestDeleteLocation(t *testing.T) {
	db := models.NewTestDB(t)

	locations := &models.LocationModel{DB: db}
	locationService := LocationService{LocationModel: locations, DB: db}

	id, err := locationService.CreateLocation(&LocationForm{Name: "Temp Field", Address1: "1 Main St", City: "Troy"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := locationService.DeleteLocation(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	_, err = locations.Get(id)
	if err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

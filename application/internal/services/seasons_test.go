package services

import (
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestCreateSeason(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	form := &SeasonForm{LeagueID: 1, Name: "Spring 2024", StartDate: "2024-05-01", EndDate: "2024-06-30"}

	id, err := seasonService.CreateSeason(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	season, err := seasons.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if season.Name != "Spring 2024" {
		t.Fatalf("wrong name! expected Spring 2024, got %s", season.Name)
	}
	if !season.StartDate.Valid {
		t.Fatal("expected startDate to be set")
	}
}

func TestCreateSeasonMissingRequiredFields(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	_, err := seasonService.CreateSeason(&SeasonForm{}, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestUpdateSeason(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	id, err := seasonService.CreateSeason(&SeasonForm{LeagueID: 1, Name: "Old Name"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	err = seasonService.UpdateSeason(&SeasonForm{ID: id, LeagueID: 1, Name: "New Name"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	season, err := seasons.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if season.Name != "New Name" {
		t.Fatalf("expected updated name, got %s", season.Name)
	}
}

func TestDeleteSeason(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	id, err := seasonService.CreateSeason(&SeasonForm{LeagueID: 1, Name: "Spring 2024"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := seasonService.DeleteSeason(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	if _, err := seasons.Get(id); err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

package models

import (
	"database/sql"
	"testing"
	"time"
)

func TestInsertAndGetSeason(t *testing.T) {
	db := NewTestDB(t)

	sm := SeasonModel{DB: db}

	start := sql.NullTime{Time: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	end := sql.NullTime{Time: time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC), Valid: true}

	id, err := sm.Insert(&Season{LeagueID: 1, Name: "Spring 2024", StartDate: start, EndDate: end})
	if err != nil {
		t.Fatal(err)
	}

	season, err := sm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if season.Name != "Spring 2024" {
		t.Fatalf("wrong name! expected Spring 2024 but got %s", season.Name)
	}
	if season.LeagueID != 1 {
		t.Fatalf("wrong league! expected 1 but got %d", season.LeagueID)
	}
	if !season.StartDate.Valid || !season.StartDate.Time.Equal(start.Time) {
		t.Fatalf("expected startDate %v, got %+v", start.Time, season.StartDate)
	}
}

func TestUpdateSeason(t *testing.T) {
	db := NewTestDB(t)

	sm := SeasonModel{DB: db}

	id, err := sm.Insert(&Season{LeagueID: 1, Name: "Old Name"})
	if err != nil {
		t.Fatal(err)
	}

	if err := sm.Update(&Season{ID: id, LeagueID: 1, Name: "New Name"}); err != nil {
		t.Fatal(err)
	}

	season, err := sm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if season.Name != "New Name" {
		t.Fatalf("expected updated name, got %s", season.Name)
	}
}

func TestGetByLeagueSeasons(t *testing.T) {
	db := NewTestDB(t)

	sm := SeasonModel{DB: db}

	early := sql.NullTime{Time: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	late := sql.NullTime{Time: time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	if _, err := sm.Insert(&Season{LeagueID: 1, Name: "Spring 2024", StartDate: early}); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Insert(&Season{LeagueID: 1, Name: "Spring 2025", StartDate: late}); err != nil {
		t.Fatal(err)
	}

	seasons, err := sm.GetByLeague(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(seasons) != 2 {
		t.Fatalf("expected 2 seasons, got %d", len(seasons))
	}
	if seasons[0].Name != "Spring 2025" {
		t.Fatalf("expected most recent season first, got %s", seasons[0].Name)
	}
}

func TestGetCurrentSeason(t *testing.T) {
	db := NewTestDB(t)

	sm := SeasonModel{DB: db}

	past := &Season{
		LeagueID:  1,
		Name:      "Fall 2023",
		StartDate: sql.NullTime{Time: time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		EndDate:   sql.NullTime{Time: time.Date(2023, 10, 30, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	current := &Season{
		LeagueID:  1,
		Name:      "Spring 2024",
		StartDate: sql.NullTime{Time: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		EndDate:   sql.NullTime{Time: time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	future := &Season{
		LeagueID:  1,
		Name:      "Fall 2024",
		StartDate: sql.NullTime{Time: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		EndDate:   sql.NullTime{Time: time.Date(2024, 10, 30, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	if _, err := sm.Insert(past); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Insert(current); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Insert(future); err != nil {
		t.Fatal(err)
	}

	asOf := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	got, err := sm.GetCurrent(1, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Spring 2024" {
		t.Fatalf("expected Spring 2024 to be current, got %s", got.Name)
	}

	asOf = time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC)
	got, err = sm.GetCurrent(1, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Spring 2024" {
		t.Fatalf("expected most-recently-ended Spring 2024 between seasons, got %s", got.Name)
	}
}

func TestGetCurrentSeasonNoneExist(t *testing.T) {
	db := NewTestDB(t)

	sm := SeasonModel{DB: db}

	_, err := sm.GetCurrent(1, time.Now())
	if err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

// GetCurrentOrNext's tie-break between seasons is the opposite of
// GetCurrent's: it should surface the upcoming season, not the one that
// just wrapped up — see TestGetCurrentSeason's second assertion for the
// GetCurrent behavior this is intentionally reversing.
func TestGetCurrentOrNextSeason(t *testing.T) {
	db := NewTestDB(t)

	sm := SeasonModel{DB: db}

	past := &Season{
		LeagueID:  1,
		Name:      "Fall 2023",
		StartDate: sql.NullTime{Time: time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		EndDate:   sql.NullTime{Time: time.Date(2023, 10, 30, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	current := &Season{
		LeagueID:  1,
		Name:      "Spring 2024",
		StartDate: sql.NullTime{Time: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		EndDate:   sql.NullTime{Time: time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	future := &Season{
		LeagueID:  1,
		Name:      "Fall 2024",
		StartDate: sql.NullTime{Time: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		EndDate:   sql.NullTime{Time: time.Date(2024, 10, 30, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	if _, err := sm.Insert(past); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Insert(current); err != nil {
		t.Fatal(err)
	}
	if _, err := sm.Insert(future); err != nil {
		t.Fatal(err)
	}

	asOf := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	got, err := sm.GetCurrentOrNext(1, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Spring 2024" {
		t.Fatalf("expected in-progress Spring 2024, got %s", got.Name)
	}

	asOf = time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC)
	got, err = sm.GetCurrentOrNext(1, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Fall 2024" {
		t.Fatalf("expected upcoming Fall 2024 between seasons, got %s", got.Name)
	}
}

func TestGetCurrentOrNextSeasonNoneExist(t *testing.T) {
	db := NewTestDB(t)

	sm := SeasonModel{DB: db}

	_, err := sm.GetCurrentOrNext(1, time.Now())
	if err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestDeleteSeasonWithDependentsFails(t *testing.T) {
	db := NewTestDB(t)

	sm := SeasonModel{DB: db}
	mm := MatchModel{DB: db}

	seasonID, err := sm.Insert(&Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: 1, MatchDate: time.Now()}); err != nil {
		t.Fatal(err)
	}

	err = sm.Delete(seasonID)
	if err != ErrHasDependents {
		t.Fatalf("expected ErrHasDependents, got %v", err)
	}
}

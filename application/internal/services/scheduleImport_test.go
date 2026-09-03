package services

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

// newScheduleImportFixtures sets up a league with the teams/season needed
// by the tests below, deliberately including a team name spelled
// differently than the CSV rows will use it (mirrors the real
// "Chatham SOMA" vs. CSV "Chatham - SOMA" mismatch found in the user's
// actual schedule file).
func newScheduleImportFixtures(t *testing.T, db *sql.DB) (seasonID, colonialID, chathamID, bethlehemID int) {
	t.Helper()

	lm := &models.LeagueModel{DB: db}
	leagueID, err := lm.Insert(&models.League{Name: "Test League"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: db}
	seasonID, err = sm.Insert(&models.Season{LeagueID: leagueID, Name: "Fall 2026"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: db}
	colonialID, err = tm.Insert(&models.Team{LeagueID: leagueID, Name: "Colonial FC"})
	if err != nil {
		t.Fatal(err)
	}
	chathamID, err = tm.Insert(&models.Team{LeagueID: leagueID, Name: "Chatham SOMA"})
	if err != nil {
		t.Fatal(err)
	}
	bethlehemID, err = tm.Insert(&models.Team{LeagueID: leagueID, Name: "Bethlehem FC"})
	if err != nil {
		t.Fatal(err)
	}

	return seasonID, colonialID, chathamID, bethlehemID
}

func newTestLocation(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	am := &models.AddressModel{DB: db}
	street := "1 " + name + " St"
	addressID, err := am.Insert(&models.Address{Address1: sql.NullString{String: street, Valid: true}, City: sql.NullString{String: "Anytown", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	locm := &models.LocationModel{DB: db}
	id, err := locm.Insert(&models.Location{Name: name, AddressID: addressID, AddressKey: addressKey(street, "", "Anytown", "", "")})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestImportScheduleCSVHandlesRealWorldMismatches(t *testing.T) {
	db := models.NewTestDB(t)
	seasonID, colonialID, chathamID, bethlehemID := newScheduleImportFixtures(t, db)
	crellinID := newTestLocation(t, db, "Crellin Park")
	cliftonID := newTestLocation(t, db, "Clifton Commons")

	service := &ScheduleImportService{DB: db}

	csvBody := `Date,Time,Home,Away,Location
9/8/2026,,Chatham - SOMA,Colonial FC,"Crellin Park, Chatham"
9/15/2026,09:30:00 AM,Colonial FC,Bethlehem FC,"Clifton Commons, Clifton Park"
9/22/2026,9:30 AM,Nonexistent FC,Colonial FC,
9/29/2026,9:30 AM,Colonial FC,Bethlehem FC,"Somewhere Nobody Has Heard Of"`

	result, err := service.ImportCSV(seasonID, strings.NewReader(csvBody), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 3 {
		t.Fatalf("expected 3 matches added (row 3 skipped for the unknown team), got %+v", result)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 row skipped, got %+v", result.Skipped)
	}
	if !strings.Contains(result.Skipped[0].Reason, "Nonexistent FC") {
		t.Fatalf("expected the skip reason to name the unmatched team, got %q", result.Skipped[0].Reason)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning (the unresolvable location), got %+v", result.Warnings)
	}

	mm := &models.MatchModel{DB: db}
	matches, err := mm.GetBySeason(seasonID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches created, got %d", len(matches))
	}

	var chathamMatch, bethlehemMatch, warningMatch *models.Match
	for _, m := range matches {
		switch {
		case m.HomeTeamID == chathamID || m.AwayTeamID == chathamID:
			chathamMatch = m
		case m.MatchDate.Day() == 15:
			bethlehemMatch = m
		case m.MatchDate.Day() == 29:
			warningMatch = m
		}
	}

	if chathamMatch == nil {
		t.Fatal("expected the 'Chatham - SOMA' row to resolve to the Chatham SOMA team despite the punctuation mismatch")
	}
	if !chathamMatch.LocationID.Valid || int(chathamMatch.LocationID.Int32) != crellinID {
		t.Fatalf("expected the location to resolve via the text before the first comma, got %+v", chathamMatch.LocationID)
	}
	if chathamMatch.MatchDate.Hour() != 0 || chathamMatch.MatchDate.Minute() != 0 {
		t.Fatalf("expected a blank Time cell to leave no time recorded, got %s", chathamMatch.MatchDate)
	}

	if bethlehemMatch == nil || bethlehemMatch.HomeTeamID != colonialID || bethlehemMatch.AwayTeamID != bethlehemID {
		t.Fatalf("expected the 9/15 Colonial vs Bethlehem match to exist, got %+v", bethlehemMatch)
	}
	// 09:30 AM Eastern (EDT, UTC-4 in September) stored as its UTC-equivalent
	// instant, same as parseRequiredMatchDateTime does for manual match entry.
	if bethlehemMatch.MatchDate.Hour() != 13 || bethlehemMatch.MatchDate.Minute() != 30 {
		t.Fatalf("expected the real-world '09:30:00 AM' time format (with seconds) to parse as 09:30 AM Eastern, got %s", bethlehemMatch.MatchDate)
	}
	if !bethlehemMatch.LocationID.Valid || int(bethlehemMatch.LocationID.Int32) != cliftonID {
		t.Fatalf("expected the location to resolve via the text before the first comma, got %+v", bethlehemMatch.LocationID)
	}

	if warningMatch == nil {
		t.Fatal("expected the row with an unresolvable location to still create a match")
	}
	if warningMatch.LocationID.Valid {
		t.Fatal("expected the unresolvable location to leave the match's location unset")
	}
}

func TestImportScheduleCSVReimportUpdatesInsteadOfDuplicating(t *testing.T) {
	db := models.NewTestDB(t)
	seasonID, _, _, _ := newScheduleImportFixtures(t, db)
	crellinID := newTestLocation(t, db, "Crellin Park")

	service := &ScheduleImportService{DB: db}

	first := `Date,Time,Home,Away,Location
9/15/2026,9:30 AM,Colonial FC,Bethlehem FC,`
	if _, err := service.ImportCSV(seasonID, strings.NewReader(first), "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	second := `Date,Time,Home,Away,Location
9/15/2026,10:00 AM,Colonial FC,Bethlehem FC,Crellin Park`
	result, err := service.ImportCSV(seasonID, strings.NewReader(second), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 0 || len(result.Updated) != 1 {
		t.Fatalf("expected the second import to update, not add, got %+v", result)
	}

	mm := &models.MatchModel{DB: db}
	matches, err := mm.GetBySeason(seasonID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 match after the re-import, got %d", len(matches))
	}
	if !matches[0].LocationID.Valid || int(matches[0].LocationID.Int32) != crellinID {
		t.Fatalf("expected the re-import to set the location, got %+v", matches[0].LocationID)
	}
	if matches[0].MatchDate.Hour() != 14 { // 10:00 AM Eastern (EDT, UTC-4) in September
		t.Fatalf("expected the re-import to update the time, got %s", matches[0].MatchDate)
	}
}

func TestImportScheduleCSVRejectsMissingRequiredColumn(t *testing.T) {
	db := models.NewTestDB(t)
	seasonID, _, _, _ := newScheduleImportFixtures(t, db)

	service := &ScheduleImportService{DB: db}
	csvBody := "Date,Home\n9/8/2026,Colonial FC"
	if _, err := service.ImportCSV(seasonID, strings.NewReader(csvBody), "admin@example.com"); err == nil {
		t.Fatal("expected an error for a CSV missing the Away column")
	}
}

package services

import (
	"bytes"
	"database/sql"
	"fmt"
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

// newTestRosterTeam builds a league, a season (picked up by
// SeasonModel.GetCurrentOrNext since it has no start/end dates), a home
// field, a captain (with an email), and rosterSize additional players on
// the roster — everything BuildRosterPDF needs. One player is left with no
// address/DOB/phone at all, to prove missing optional data renders as
// blank cells rather than an error.
func newTestRosterTeam(t *testing.T, db *sql.DB, rosterSize int) int {
	t.Helper()

	lm := &models.LeagueModel{DB: db}
	leagueID, err := lm.Insert(&models.League{Name: "Capital District Over 30 Recreational Soccer League"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: db}
	if _, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "2025 Fall & 2026 Spring Seasons and Activities"}); err != nil {
		t.Fatal(err)
	}

	am := &models.AddressModel{DB: db}
	addressID, err := am.Insert(&models.Address{
		Address1:      sql.NullString{String: "10 Andrew Ct", Valid: true},
		City:          sql.NullString{String: "Troy", Valid: true},
		StateProvince: sql.NullString{String: "NY", Valid: true},
		ZipCode:       sql.NullString{String: "12182", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	locm := &models.LocationModel{DB: db}
	locationID, err := locm.Insert(&models.Location{
		Name:       "Clifton Commons",
		AddressID:  addressID,
		AddressKey: addressKey("10 Andrew Ct", "", "Troy", "NY", "12182"),
	})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: db}
	teamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Colonial FC", LocationID: sql.NullInt32{Int32: int32(locationID), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}

	captainID, err := pm.Insert(&models.Player{
		FirstName: "Lou",
		LastName:  "Garwood",
		Email:     sql.NullString{String: "louis.garwood@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(captainID, teamID); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetCaptain(teamID, sql.NullInt32{Int32: int32(captainID), Valid: true}); err != nil {
		t.Fatal(err)
	}

	// A player with no address/phone/DOB on file at all.
	bareID, err := pm.Insert(&models.Player{FirstName: "Bare", LastName: "Profile"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(bareID, teamID); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < rosterSize; i++ {
		playerID, err := pm.Insert(&models.Player{
			FirstName:   fmt.Sprintf("Player%d", i),
			LastName:    fmt.Sprintf("Roster%d", i),
			AddressID:   sql.NullInt32{Int32: int32(addressID), Valid: true},
			PhoneNumber: sql.NullString{String: "518-555-0100", Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := tmm.AddMembership(playerID, teamID); err != nil {
			t.Fatal(err)
		}
	}

	return teamID
}

func testBuildRosterPDF(t *testing.T, rosterSize int) []byte {
	t.Helper()
	db := models.NewTestDB(t)
	teamID := newTestRosterTeam(t, db, rosterSize)

	service := &RosterExportService{DB: db}
	pdfBytes, err := service.BuildRosterPDF(teamID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfBytes) == 0 {
		t.Fatal("expected non-empty PDF output")
	}

	pageCount := bytes.Count(pdfBytes, []byte("/Type /Page")) - bytes.Count(pdfBytes, []byte("/Type /Pages"))
	if pageCount != 1 {
		t.Fatalf("expected exactly 1 page, found %d", pageCount)
	}

	return pdfBytes
}

func TestBuildRosterPDFNominalRoster(t *testing.T) {
	testBuildRosterPDF(t, 5)
}

func TestBuildRosterPDFAtLimit(t *testing.T) {
	testBuildRosterPDF(t, 25)
}

func TestBuildRosterPDFOverLimit(t *testing.T) {
	testBuildRosterPDF(t, 30)
}

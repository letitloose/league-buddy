package services

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestImportCSVHandlesFullAndMinimalRows(t *testing.T) {
	db := models.NewTestDB(t)
	service := &RosterImportService{DB: db}

	csvBody := `Last Name,First Name,Email,Address1,Address2,City,State,Zip,Phone,DOB
Garwood,Lou,import-lou@example.com,10 Andrew Ct,,Troy,NY,12182,518-495-2003,03/22/1978
Alund,Rob,import-rob@example.com,,,,,,,`

	result, err := service.ImportCSV(1, strings.NewReader(csvBody), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 2 {
		t.Fatalf("expected 2 players added, got %+v", result)
	}
	if len(result.Skipped) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("expected no skips/warnings, got %+v", result)
	}

	pm := &models.PlayerModel{DB: db}
	lou, err := pm.GetByEmail("import-lou@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !lou.PhoneNumber.Valid || lou.PhoneNumber.String != "518-495-2003" {
		t.Fatalf("expected normalized phone 5184952003, got %+v", lou.PhoneNumber)
	}
	if !lou.DateOfBirth.Valid {
		t.Fatal("expected a parsed DOB for Lou")
	}
	if !lou.AddressID.Valid {
		t.Fatal("expected an address to have been created for Lou")
	}

	rob, err := pm.GetByEmail("import-rob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if rob.AddressID.Valid || rob.PhoneNumber.Valid || rob.DateOfBirth.Valid {
		t.Fatalf("expected Rob's optional fields to stay blank, got %+v", rob)
	}

	tmm := &models.TeamMemberModel{DB: db}
	for _, playerID := range []int{lou.ID, rob.ID} {
		isMember, err := tmm.IsMember(playerID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !isMember {
			t.Fatalf("expected player %d to be on team 1's roster", playerID)
		}
	}
}

func TestImportCSVSkipsRowsMissingRequiredFields(t *testing.T) {
	db := models.NewTestDB(t)
	service := &RosterImportService{DB: db}

	csvBody := `Last Name,First Name,Email
Noemail,Player,
Bademail,Player,not-an-email`

	result, err := service.ImportCSV(1, strings.NewReader(csvBody), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 0 {
		t.Fatalf("expected nothing added, got %+v", result.Added)
	}
	if len(result.Skipped) != 2 {
		t.Fatalf("expected 2 rows skipped, got %+v", result.Skipped)
	}
}

func TestImportCSVDropsBadOptionalFieldsWithWarning(t *testing.T) {
	db := models.NewTestDB(t)
	service := &RosterImportService{DB: db}

	csvBody := `Last Name,First Name,Email,Address1,City,State,Zip,DOB
Zippy,Bad,bad-zip@example.com,1 Main St,Troy,NY,notazip,
Datey,Bad,bad-dob@example.com,,,,,not-a-date`

	result, err := service.ImportCSV(1, strings.NewReader(csvBody), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 2 {
		t.Fatalf("expected both rows to still be added despite bad optional fields, got %+v", result)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("expected 2 warnings (bad zip, bad dob), got %+v", result.Warnings)
	}

	pm := &models.PlayerModel{DB: db}
	zippy, err := pm.GetByEmail("bad-zip@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if zippy.AddressID.Valid {
		t.Fatal("expected the whole address to be dropped when the zip was invalid")
	}

	datey, err := pm.GetByEmail("bad-dob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if datey.DateOfBirth.Valid {
		t.Fatal("expected DOB to be left blank when unparseable")
	}
}

func TestImportCSVReimportUpdatesInsteadOfDuplicating(t *testing.T) {
	db := models.NewTestDB(t)
	service := &RosterImportService{DB: db}

	first := `Last Name,First Name,Email,Phone
Regular,Player,reimport@example.com,518-555-0100`
	if _, err := service.ImportCSV(1, strings.NewReader(first), "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	second := `Last Name,First Name,Email,Phone
Regular,Player,reimport@example.com,518-555-0199`
	result, err := service.ImportCSV(1, strings.NewReader(second), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 0 || len(result.Updated) != 1 {
		t.Fatalf("expected the second import to update, not add, got %+v", result)
	}

	pm := &models.PlayerModel{DB: db}
	player, err := pm.GetByEmail("reimport@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if player.PhoneNumber.String != "518-555-0199" {
		t.Fatalf("expected phone to be refreshed to 5185550199, got %s", player.PhoneNumber.String)
	}

	tmm := &models.TeamMemberModel{DB: db}
	isMember, err := tmm.IsMember(player.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected the player to still be on the roster exactly once")
	}
}

func TestImportCSVSkipsEmailAlreadyOnAnotherTeamInLeague(t *testing.T) {
	db := models.NewTestDB(t)
	service := &RosterImportService{DB: db}

	tm := &models.TeamModel{DB: db}
	otherTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Other League Team"})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	elsewhereID, err := pm.Insert(&models.Player{FirstName: "Else", LastName: "Where", Email: sql.NullString{String: "elsewhere-import@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(elsewhereID, otherTeamID); err != nil {
		t.Fatal(err)
	}

	csvBody := `Last Name,First Name,Email
Where,Else,elsewhere-import@example.com`
	result, err := service.ImportCSV(1, strings.NewReader(csvBody), "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 0 || len(result.Skipped) != 1 {
		t.Fatalf("expected the row to be skipped as a league conflict, got %+v", result)
	}

	isMember, err := tmm.IsMember(elsewhereID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if isMember {
		t.Fatal("expected the conflicting player not to be added to team 1's roster")
	}
}

func TestImportCSVRejectsMissingRequiredColumn(t *testing.T) {
	db := models.NewTestDB(t)
	service := &RosterImportService{DB: db}

	csvBody := `First Name,Email
Player,noemail-column@example.com`

	if _, err := service.ImportCSV(1, strings.NewReader(csvBody), "admin@example.com"); err == nil {
		t.Fatal("expected an error for a CSV missing the Last Name column")
	}
}

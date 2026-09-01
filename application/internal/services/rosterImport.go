package services

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

// RosterImportRowResult describes one CSV row's outcome — used for both
// Skipped (the row wasn't imported at all) and Warnings (the row was
// imported, but an optional field was dropped for failing validation).
type RosterImportRowResult struct {
	RowNumber int // 1-based, counting the header row as row 1, matching what a spreadsheet shows
	Name      string
	Reason    string
}

// RosterImportResult is ImportCSV's full report: names of players
// added/updated, plus every row that was skipped entirely or imported with
// a dropped optional field.
type RosterImportResult struct {
	Added    []string
	Updated  []string
	Skipped  []RosterImportRowResult
	Warnings []RosterImportRowResult
}

// RosterImportService bulk-imports a team's roster from a CSV shaped like
// the league's own roster spreadsheet.
type RosterImportService struct {
	DB *sql.DB
}

// SampleRosterCSV is served as a downloadable template from the Import
// Roster page so a captain can see the expected columns — two rows drawn
// from the real Colonial FC roster already in the system, one fully
// filled in and one showing that only name+email is required.
const SampleRosterCSV = `Last Name,First Name,Email,Address1,Address2,City,State,Zip,Phone,DOB
Garwood,Lou,louis.garwood@example.com,10 Andrew Ct,,Troy,NY,12182,518-495-2003,03/22/1978
Alund,Rob,rob.alund@example.com,,,,,,,
`

// normalizeHeader strips spaces and lowercases a CSV header cell so
// "Last Name", "LASTNAME", and "lastname" all match the same column.
func normalizeHeader(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// dobLayouts are tried in order against a DOB cell — covers both a
// machine-written ISO date and the mixed formats real rosters actually use
// (the source Colonial FC roster has both "07/18/1983" and "5/16/88").
var dobLayouts = []string{"2006-01-02", "01/02/2006", "1/2/2006", "01/02/06", "1/2/06"}

// parseFlexibleDate tries each of dobLayouts in turn, returning the value
// reformatted as "2006-01-02" (what PlayerForm.DateOfBirth/parseOptionalDate
// expect) and true on the first match. A blank value is valid and parses
// to ("", true) — DOB is optional.
func parseFlexibleDate(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	for _, layout := range dobLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// ImportCSV reads file as a roster CSV and imports it onto teamID's
// roster, row by row. A malformed file (unreadable CSV, or missing one of
// the required Last Name/First Name/Email columns) is rejected outright
// with a plain error — everything else is handled per-row, so one bad row
// never aborts the rest of the file.
func (s *RosterImportService) ImportCSV(teamID int, file io.Reader, actorEmail string) (*RosterImportResult, error) {
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("could not read the CSV header row: %w", err)
	}

	colIndex := map[string]int{}
	for i, cell := range header {
		colIndex[normalizeHeader(cell)] = i
	}
	for _, required := range []string{"lastname", "firstname", "email"} {
		if _, ok := colIndex[required]; !ok {
			return nil, fmt.Errorf("missing required column %q", required)
		}
	}
	cell := func(record []string, key string) string {
		i, ok := colIndex[key]
		if !ok || i >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[i])
	}

	result := &RosterImportResult{}
	pm := &models.PlayerModel{DB: s.DB}
	tmm := &models.TeamMemberModel{DB: s.DB}
	tm := &models.TeamModel{DB: s.DB}
	playerService := &PlayerService{PlayerModel: pm, DB: s.DB}

	team, err := tm.Get(teamID)
	if err != nil {
		return nil, err
	}

	rowNumber := 1
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("could not read row %d: %w", rowNumber+1, err)
		}
		rowNumber++

		firstName := cell(record, "firstname")
		lastName := cell(record, "lastname")
		email := cell(record, "email")
		name := strings.TrimSpace(firstName + " " + lastName)
		if name == "" {
			name = email
		}

		if firstName == "" || lastName == "" || email == "" {
			result.Skipped = append(result.Skipped, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "First Name, Last Name, and Email are all required."})
			continue
		}
		if !validator.ValidEmail(email) {
			result.Skipped = append(result.Skipped, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "Email is not a valid address: " + email})
			continue
		}

		form := &PlayerForm{
			FirstName: firstName,
			LastName:  lastName,
			Email:     email,
		}

		phone := cell(record, "phone")
		if phone != "" {
			if normalized, ok := normalizePhoneNumber(phone); ok {
				form.PhoneNumber = normalized
			} else {
				result.Warnings = append(result.Warnings, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "Phone number wasn't 10 digits and was left blank: " + phone})
			}
		}

		dob := cell(record, "dob")
		if isoDOB, ok := parseFlexibleDate(dob); ok {
			form.DateOfBirth = isoDOB
		} else {
			result.Warnings = append(result.Warnings, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "Date of birth wasn't recognized and was left blank: " + dob})
		}

		address1 := cell(record, "address1")
		if address1 != "" {
			city := cell(record, "city")
			state := cell(record, "state")
			zip := cell(record, "zip")
			okState := state == "" || models.IsValidStateCode(strings.ToUpper(state))
			okZip := true
			normalizedZip := ""
			if zip != "" {
				normalizedZip, okZip = normalizeZip(zip)
			}
			switch {
			case city == "":
				result.Warnings = append(result.Warnings, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "Address was given without a City and was left blank."})
			case !okState:
				result.Warnings = append(result.Warnings, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "State \"" + state + "\" isn't a valid state code, so the address was left blank."})
			case !okZip:
				result.Warnings = append(result.Warnings, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "Zip \"" + zip + "\" isn't 5 digits, so the address was left blank."})
			default:
				form.Address1 = address1
				form.Address2 = cell(record, "address2")
				form.City = city
				form.StateProvince = strings.ToUpper(state)
				form.ZipCode = normalizedZip
			}
		}

		existing, err := pm.GetByEmail(email)
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return nil, err
		}

		switch {
		case err == nil:
			isMember, err := tmm.IsMember(existing.ID, teamID)
			if err != nil {
				return nil, err
			}
			if !isMember {
				hasTeamInLeague, err := tmm.HasTeamInLeague(existing.ID, team.LeagueID)
				if err != nil {
					return nil, err
				}
				if hasTeamInLeague {
					result.Skipped = append(result.Skipped, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "This email already belongs to a player on another team in this league."})
					continue
				}
			}

			form.ID = existing.ID
			if err := playerService.UpdatePlayer(form, actorEmail); err != nil {
				if errors.Is(err, models.ErrBadData) {
					result.Skipped = append(result.Skipped, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "Row failed validation and was skipped."})
					continue
				}
				return nil, err
			}
			if !isMember {
				if err := tmm.AddMembership(existing.ID, teamID); err != nil {
					return nil, err
				}
				result.Added = append(result.Added, name)
			} else {
				result.Updated = append(result.Updated, name)
			}

		case errors.Is(err, models.ErrNoRecord):
			if _, err := playerService.AddPlayer(teamID, form, actorEmail); err != nil {
				if errors.Is(err, models.ErrBadData) {
					result.Skipped = append(result.Skipped, RosterImportRowResult{RowNumber: rowNumber, Name: name, Reason: "Row failed validation and was skipped."})
					continue
				}
				return nil, err
			}
			result.Added = append(result.Added, name)
		}
	}

	return result, nil
}

package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/letitloose/league-buddy/internal/models"
)

// RosterExportService renders the league's official roster-registration
// form for a team as a downloadable PDF, populated from that team's live
// roster data.
type RosterExportService struct {
	DB *sql.DB
}

// pageWidthMM/pageHeightMM are landscape US Letter, matching the source
// form's paper size.
const (
	pageWidthMM  = 279.4
	pageHeightMM = 215.9
	marginMM     = 10.0
)

// rosterColumn is one column of the player table: its header label, its
// width, and how to pull that column's text out of a roster row.
type rosterColumn struct {
	label string
	width float64
}

var rosterColumns = []rosterColumn{
	{"LAST NAME", 32},
	{"FIRST NAME", 28},
	{"ADDRESS", 100},
	{"ZIP", 20},
	{"PHONE", 45},
	{"DOB", 34},
}

// BuildRosterPDF renders teamID's roster as a single-page landscape PDF
// matching the league's official roster-registration form. Missing
// optional data (no captain assigned, no home field, a player with no
// address on file, etc.) is rendered as a blank field rather than an
// error — the export should always succeed for whatever the team's roster
// actually has on file.
func (s *RosterExportService) BuildRosterPDF(teamID int) ([]byte, error) {
	tm := &models.TeamModel{DB: s.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		return nil, err
	}

	lm := &models.LeagueModel{DB: s.DB}
	league, err := lm.Get(team.LeagueID)
	if err != nil {
		return nil, err
	}

	sm := &models.SeasonModel{DB: s.DB}
	season, err := sm.GetCurrentOrNext(team.LeagueID, time.Now())
	if err != nil && !errors.Is(err, models.ErrNoRecord) {
		return nil, err
	}

	var captain *models.Player
	if team.CaptainPlayerID.Valid {
		pm := &models.PlayerModel{DB: s.DB}
		captain, err = pm.Get(int(team.CaptainPlayerID.Int32))
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return nil, err
		}
	}

	var location *models.Location
	if team.LocationID.Valid {
		locm := &models.LocationModel{DB: s.DB}
		location, err = locm.Get(int(team.LocationID.Int32))
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return nil, err
		}
	}

	pm := &models.PlayerModel{DB: s.DB}
	roster, err := pm.GetByTeam(teamID)
	if err != nil {
		return nil, err
	}

	am := &models.AddressModel{DB: s.DB}
	addressesByPlayer := map[int]*models.Address{}
	for _, player := range roster {
		if !player.AddressID.Valid {
			continue
		}
		address, err := am.Get(int(player.AddressID.Int32))
		if err != nil {
			if errors.Is(err, models.ErrNoRecord) {
				continue
			}
			return nil, err
		}
		addressesByPlayer[player.ID] = address
	}

	return renderRosterPDF(league, season, team, location, captain, roster, addressesByPlayer)
}

func renderRosterPDF(league *models.League, season *models.Season, team *models.Team, location *models.Location, captain *models.Player, roster []*models.Player, addressesByPlayer map[int]*models.Address) ([]byte, error) {
	// fpdf's Size field is expected in portrait terms and gets swapped
	// internally when OrientationStr is "L" — passing landscape dimensions
	// directly here would cause fpdf to swap them a second time, producing
	// a narrow/tall page. The standard "Letter" size string already
	// encodes the portrait dimensions fpdf expects, so orientation alone
	// produces the intended 279.4mm x 215.9mm landscape page.
	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "L",
		UnitStr:        "mm",
		SizeStr:        "Letter",
	})
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetMargins(marginMM, marginMM, marginMM)
	pdf.AddPage()

	usableWidth := pageWidthMM - 2*marginMM

	// Header block.
	pdf.SetFont("Helvetica", "B", 14)
	pdf.CellFormat(usableWidth, 7, league.Name, "", 1, "C", false, 0, "")

	pdf.SetFont("Helvetica", "", 11)
	seasonName := ""
	if season != nil {
		seasonName = season.Name
	}
	pdf.CellFormat(usableWidth, 6, seasonName, "", 1, "C", false, 0, "")

	captainName := ""
	captainEmail := ""
	if captain != nil {
		captainName = captain.FirstName + " " + captain.LastName
		if captain.Email.Valid {
			captainEmail = captain.Email.String
		}
	}
	homeField := ""
	if location != nil {
		homeField = location.Name
	}
	pdf.SetFont("Helvetica", "", 10)
	teamInfoLine := fmt.Sprintf("Team Name %s   Home field: %s   Team Coordinator %s   E-mail: %s",
		team.Name, homeField, captainName, captainEmail)
	pdf.CellFormat(usableWidth, 6, teamInfoLine, "", 1, "L", false, 0, "")
	pdf.Ln(2)

	headerBlockBottom := pdf.GetY()

	// Footer (signature lines) height, reserved up front so the table body
	// row-height math below always leaves room for it.
	const footerHeight = 12.0
	footerTop := pageHeightMM - marginMM - footerHeight

	const tableHeaderRowHeight = 7.0
	tableTop := headerBlockBottom
	bodyTop := tableTop + tableHeaderRowHeight
	availableBodyHeight := footerTop - bodyTop

	rowCount := len(roster)
	rowHeight := 7.0
	if rowCount > 0 {
		rowHeight = availableBodyHeight / float64(rowCount)
		if rowHeight > 7.0 {
			rowHeight = 7.0
		}
	}
	// Font size is always in points regardless of the document's mm unit
	// (fpdf.SetFont), so convert the row height to points before deriving
	// a font size from it — sized to leave comfortable padding inside each
	// bordered cell rather than filling it edge to edge.
	const mmToPt = 2.83465
	fontSize := rowHeight * mmToPt * 0.5
	if fontSize > 10 {
		fontSize = 10
	}
	if fontSize < 6 {
		fontSize = 6
	}

	// Table header row.
	pdf.SetY(tableTop)
	pdf.SetX(marginMM)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(220, 220, 220)
	for _, col := range rosterColumns {
		pdf.CellFormat(col.width, tableHeaderRowHeight, col.label, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Table body.
	pdf.SetFont("Helvetica", "", fontSize)
	for i, player := range roster {
		pdf.SetX(marginMM)
		shaded := i%2 == 1

		address := addressesByPlayer[player.ID]
		addressText, zipText := formatRosterAddress(address)
		phoneText := ""
		if player.PhoneNumber.Valid {
			phoneText = player.PhoneNumber.String
		}
		dobText := ""
		if player.DateOfBirth.Valid {
			dobText = player.DateOfBirth.Time.Format("01/02/2006")
		}

		values := []string{player.LastName, player.FirstName, addressText, zipText, phoneText, dobText}
		for j, col := range rosterColumns {
			pdf.CellFormat(col.width, rowHeight, values[j], "1", 0, "L", shaded, 0, "")
		}
		pdf.Ln(-1)
	}

	// Footer signature lines.
	pdf.SetY(footerTop + 4)
	pdf.SetX(marginMM)
	pdf.SetFont("Helvetica", "", 10)
	halfWidth := usableWidth / 2
	pdf.CellFormat(halfWidth, 6, "Team Coordinator's Signature ________________________ Date __________", "", 0, "L", false, 0, "")
	pdf.CellFormat(halfWidth, 6, "Registrar's signature ________________________ Date __________", "", 1, "L", false, 0, "")

	var buf strings.Builder
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// formatRosterAddress builds the combined "street, city, state" address
// cell and the separate zip cell the source form uses, tolerating a
// missing address entirely or missing individual fields on it.
func formatRosterAddress(address *models.Address) (addressText, zipText string) {
	if address == nil {
		return "", ""
	}
	var parts []string
	if address.Address1.Valid && address.Address1.String != "" {
		parts = append(parts, address.Address1.String)
	}
	if address.Address2.Valid && address.Address2.String != "" {
		parts = append(parts, address.Address2.String)
	}
	if address.City.Valid && address.City.String != "" {
		parts = append(parts, address.City.String)
	}
	if address.StateProvince.Valid && address.StateProvince.String != "" {
		parts = append(parts, address.StateProvince.String)
	}
	addressText = strings.Join(parts, ", ")
	if address.ZipCode.Valid {
		zipText = address.ZipCode.String
	}
	return addressText, zipText
}

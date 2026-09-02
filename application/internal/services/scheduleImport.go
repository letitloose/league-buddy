package services

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

// ScheduleImportRowResult describes one CSV row's outcome — used for both
// Skipped (the row wasn't imported at all) and Warnings (the row was
// imported, but an optional field — currently just Location — couldn't
// be resolved).
type ScheduleImportRowResult struct {
	RowNumber int
	Matchup   string // "Home vs Away", for display
	Reason    string
}

// ScheduleImportResult is ImportCSV's full report.
type ScheduleImportResult struct {
	Added    []string
	Updated  []string
	Skipped  []ScheduleImportRowResult
	Warnings []ScheduleImportRowResult
}

// ScheduleImportService bulk-imports a season's match schedule from a CSV
// shaped like the league's own fixture spreadsheet.
type ScheduleImportService struct {
	DB *sql.DB
}

// SampleScheduleCSV is served as a downloadable template from the Import
// Schedule page — drawn from the real Fall 2026 fixtures already in the
// system, one row with a time and one without, showing Time is optional.
const SampleScheduleCSV = `Date,Time,Home,Away,Location
9/8/2026,,Chatham SOMA,Colonial FC,"Crellin Park"
9/15/2026,9:30 AM,Colonial FC,Bethlehem FC,"Clifton Commons"
`

// normalizeName strips everything but letters/digits and lowercases —
// used to match a CSV's free-text team/location name against what's on
// file, tolerating punctuation/spacing/case differences (e.g. the real
// difference between a CSV's "Chatham - SOMA" and the team actually named
// "Chatham SOMA") without doing genuine fuzzy/edit-distance matching,
// which risks silently picking the wrong team or location.
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return strings.ToLower(b.String())
}

var scheduleDateLayouts = []string{"1/2/2006", "2006-01-02"}
var scheduleTimeLayouts = []string{"3:04 PM", "15:04"}

func parseScheduleDate(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range scheduleDateLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// parseScheduleTime returns ("", true) for a blank input (no time
// recorded is valid) and ("15:04"-formatted, true) for a recognized time.
func parseScheduleTime(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	for _, layout := range scheduleTimeLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("15:04"), true
		}
	}
	return "", false
}

// existingMatchForm builds a MatchForm that preserves everything about an
// existing match except SeasonID/HomeTeamID/AwayTeamID (unchanged by
// definition, since that's how it was matched) and MatchDate/MatchTime/
// LocationID (the fields the caller is about to overwrite) — used so a
// re-import that only means to fix a match's date/time/location doesn't
// accidentally wipe out a score, notes, or recorded goals/cards someone
// already entered.
func existingMatchForm(db *sql.DB, match *models.Match) (*MatchForm, error) {
	form := &MatchForm{
		ID:         match.ID,
		SeasonID:   match.SeasonID,
		HomeTeamID: match.HomeTeamID,
		AwayTeamID: match.AwayTeamID,
		Notes:      match.Notes.String,
	}
	if match.HomeScore.Valid {
		form.HomeScore = strconv.Itoa(int(match.HomeScore.Int32))
	}
	if match.AwayScore.Valid {
		form.AwayScore = strconv.Itoa(int(match.AwayScore.Int32))
	}

	goalModel := &models.MatchGoalModel{DB: db}
	goals, err := goalModel.ListByMatch(match.ID)
	if err != nil {
		return nil, err
	}
	for _, g := range goals {
		input := GoalInput{TeamID: g.TeamID}
		if g.ScorerPlayerID.Valid {
			input.ScorerPlayerID = int(g.ScorerPlayerID.Int32)
		}
		if g.AssisterPlayerID.Valid {
			input.AssisterPlayerID = int(g.AssisterPlayerID.Int32)
		}
		if g.Minute.Valid {
			input.Minute = int(g.Minute.Int32)
		}
		form.Goals = append(form.Goals, input)
	}

	cardModel := &models.MatchCardModel{DB: db}
	cards, err := cardModel.ListByMatch(match.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range cards {
		input := CardInput{TeamID: c.TeamID, CardType: c.CardType}
		if c.PlayerID.Valid {
			input.PlayerID = int(c.PlayerID.Int32)
		}
		form.Cards = append(form.Cards, input)
	}

	return form, nil
}

// ImportCSV reads file as a schedule CSV and imports it onto seasonID —
// every row becomes a match in that season; the season itself is never
// created or changed by this. Header row required (Date/Home/Away;
// Time/Location optional). A malformed file (unreadable CSV, missing a
// required column) is rejected outright; everything else is handled
// per-row.
func (s *ScheduleImportService) ImportCSV(seasonID int, file io.Reader, actorEmail string) (*ScheduleImportResult, error) {
	sm := &models.SeasonModel{DB: s.DB}
	season, err := sm.Get(seasonID)
	if err != nil {
		return nil, err
	}

	tm := &models.TeamModel{DB: s.DB}
	teams, err := tm.GetByLeague(season.LeagueID)
	if err != nil {
		return nil, err
	}
	teamsByName := map[string]*models.Team{}
	for _, team := range teams {
		teamsByName[normalizeName(team.Name)] = team
	}

	locm := &models.LocationModel{DB: s.DB}
	locations, err := locm.List()
	if err != nil {
		return nil, err
	}
	locationsByName := map[string]*models.Location{}
	for _, loc := range locations {
		locationsByName[normalizeName(loc.Name)] = loc
	}

	mm := &models.MatchModel{DB: s.DB}
	existingMatches, err := mm.GetBySeason(seasonID)
	if err != nil {
		return nil, err
	}
	// Keyed by "homeTeamID|awayTeamID|2006-01-02", checked both team
	// orders when looking a row up (see below) — a schedule CSV always
	// gives a specific home/away, but re-matching should still recognize
	// the same fixture even if the order somehow differs between runs.
	existingByKey := map[string]*models.Match{}
	for _, match := range existingMatches {
		key := fmt.Sprintf("%d|%d|%s", match.HomeTeamID, match.AwayTeamID, match.MatchDate.Format("2006-01-02"))
		existingByKey[key] = match
	}

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
	for _, required := range []string{"date", "home", "away"} {
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

	result := &ScheduleImportResult{}
	matchService := &MatchService{MatchModel: mm, DB: s.DB}

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

		homeName := cell(record, "home")
		awayName := cell(record, "away")
		matchup := fmt.Sprintf("%s vs %s", homeName, awayName)

		matchDate, dateOK := parseScheduleDate(cell(record, "date"))
		if !dateOK {
			result.Skipped = append(result.Skipped, ScheduleImportRowResult{RowNumber: rowNumber, Matchup: matchup, Reason: "missing or unrecognized date"})
			continue
		}

		homeTeam, homeOK := teamsByName[normalizeName(homeName)]
		if !homeOK {
			result.Skipped = append(result.Skipped, ScheduleImportRowResult{RowNumber: rowNumber, Matchup: matchup, Reason: "team not found: " + homeName})
			continue
		}
		awayTeam, awayOK := teamsByName[normalizeName(awayName)]
		if !awayOK {
			result.Skipped = append(result.Skipped, ScheduleImportRowResult{RowNumber: rowNumber, Matchup: matchup, Reason: "team not found: " + awayName})
			continue
		}
		if homeTeam.ID == awayTeam.ID {
			result.Skipped = append(result.Skipped, ScheduleImportRowResult{RowNumber: rowNumber, Matchup: matchup, Reason: "home and away team are the same"})
			continue
		}

		matchTime, timeOK := parseScheduleTime(cell(record, "time"))
		if !timeOK {
			result.Warnings = append(result.Warnings, ScheduleImportRowResult{RowNumber: rowNumber, Matchup: matchup, Reason: "time wasn't recognized and was left blank: " + cell(record, "time")})
		}

		locationID := 0
		if locationText := cell(record, "location"); locationText != "" {
			loc, ok := locationsByName[normalizeName(locationText)]
			if !ok {
				if beforeComma, _, found := strings.Cut(locationText, ","); found {
					loc, ok = locationsByName[normalizeName(beforeComma)]
				}
			}
			if ok {
				locationID = loc.ID
			} else {
				result.Warnings = append(result.Warnings, ScheduleImportRowResult{RowNumber: rowNumber, Matchup: matchup, Reason: "location not found and left blank: " + locationText})
			}
		}

		key := fmt.Sprintf("%d|%d|%s", homeTeam.ID, awayTeam.ID, matchDate)
		existing := existingByKey[key]
		if existing == nil {
			reverseKey := fmt.Sprintf("%d|%d|%s", awayTeam.ID, homeTeam.ID, matchDate)
			existing = existingByKey[reverseKey]
		}

		if existing != nil {
			form, err := existingMatchForm(s.DB, existing)
			if err != nil {
				return nil, err
			}
			form.LocationID = locationID
			form.MatchDate = matchDate
			form.MatchTime = matchTime
			if err := matchService.UpdateMatch(form, actorEmail); err != nil {
				if errors.Is(err, models.ErrBadData) {
					result.Skipped = append(result.Skipped, ScheduleImportRowResult{RowNumber: rowNumber, Matchup: matchup, Reason: "row failed validation and was skipped"})
					continue
				}
				return nil, err
			}
			result.Updated = append(result.Updated, matchup)
			continue
		}

		form := &MatchForm{
			SeasonID:   seasonID,
			HomeTeamID: homeTeam.ID,
			AwayTeamID: awayTeam.ID,
			LocationID: locationID,
			MatchDate:  matchDate,
			MatchTime:  matchTime,
		}
		if _, err := matchService.CreateMatch(form, actorEmail); err != nil {
			if errors.Is(err, models.ErrBadData) {
				result.Skipped = append(result.Skipped, ScheduleImportRowResult{RowNumber: rowNumber, Matchup: matchup, Reason: "row failed validation and was skipped"})
				continue
			}
			return nil, err
		}
		result.Added = append(result.Added, matchup)
	}

	return result, nil
}

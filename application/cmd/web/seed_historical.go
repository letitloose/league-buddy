package main

import (
	"database/sql"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/services"
)

// seedHistoricalSeasons populates ~2 years of real Colonial FC results (5
// seasons, 34 matches, 18 opponent teams) provided by the site owner, so the
// dev database has real schedule/results data instead of starting empty.
// Dev-only — called from reset() behind RESETDB=true, never during tests
// (cmd/web's and internal/models' test harnesses build the test DB straight
// from teardown-test.sql/setup.sql/reset-test.sql and never call reset()).
//
// The source spreadsheet's numeric score column was inconsistently ordered
// (sometimes Home-Away, sometimes Away-Home) while its W/L/D column was
// consistent for every row, so every score below is resolved by trusting
// the letter and assigning the two given numbers to home/away by magnitude
// (the winner gets the larger number) rather than trusting position. No
// per-player stats are seeded — the source spreadsheet only has team
// scores.
func (app *application) seedHistoricalSeasons() error {
	db := app.playerService.DB

	teamIDs, err := seedHistoricalTeams(db)
	if err != nil {
		return err
	}
	teamIDs["Colonial FC"] = 1 // seeded by setup.sql

	locationIDs, err := seedHistoricalLocations(app.locationService)
	if err != nil {
		return err
	}

	seasonIDs, err := seedHistoricalSeasonRows(db)
	if err != nil {
		return err
	}

	return seedHistoricalMatchRows(db, teamIDs, locationIDs, seasonIDs)
}

// "Fulmont FC" isn't listed separately — per the site owner, it's the same
// team as "GSL FC - Active Ingredient Brewing Co" under an old name, so the
// 2026-06-14 match against "Fulmont FC" below is recorded against GSL.
var historicalOpponentTeams = []string{
	"Latham Almost United",
	"Hudson FC",
	"GSL FC - Active Ingredient Brewing Co",
	"Corona FC",
	"Guilderland FC - Marisa's Lace Pizzeria Rest.",
	"Strikers FC",
	"Saratoga FC",
	"Chatham SOMA",
	"Bethlehem FC",
	"Adirondack",
	"Albany - Parkitects",
	"Brunswick FC",
	"Crazy Dawg T Shirts",
	"Greene FC - I.T.S. Inc",
	"Atletico Clifton Park",
	"Knick91SC",
	"Legends",
}

// seedHistoricalTeams creates a team shell (no captain/roster — their
// player data isn't part of the source spreadsheet) for every opponent
// Colonial FC has faced, all in league 1 ("CapReg over 30").
func seedHistoricalTeams(db *sql.DB) (map[string]int, error) {
	tm := &models.TeamModel{DB: db}
	ids := make(map[string]int, len(historicalOpponentTeams))
	for _, name := range historicalOpponentTeams {
		id, err := tm.Insert(&models.Team{LeagueID: 1, Name: name})
		if err != nil {
			return nil, err
		}
		ids[name] = id
	}
	return ids, nil
}

type historicalLocation struct {
	Key           string // short lookup key used by historicalMatches below
	Name          string
	Address1      string
	City          string
	StateProvince string
	ZipCode       string
}

// historicalLocationDefs are best-effort addresses parsed from the source
// spreadsheet's free-text venue column — good enough for a Maps search link
// even where imprecise. "eastgreenbush" intentionally matches the address
// setup.sql already seeded for Colonial FC's current home field ("East
// Greenbush Soccer Club", 100 Phillips Rd) — LocationService.CreateLocation
// dedupes on address, so this resolves to that same existing location
// rather than creating a duplicate.
var historicalLocationDefs = []historicalLocation{
	{"niskayuna", "Niskayuna Soccer Complex", "Niskayuna Soccer Complex", "Niskayuna", "NY", ""},
	{"greenport", "Greenport Town Park", "Greenport Town Park", "Hudson", "NY", ""},
	{"clifton", "Clifton Commons", "Clifton Commons", "Clifton Park", "NY", ""},
	{"nott", "Nott Road Fields", "Nott Road", "Guilderland", "NY", ""},
	{"wilton", "Wilton Town Gavin Park", "10 Lewis Rd", "Saratoga Springs", "NY", "12866"},
	{"crellin", "Crellin Park", "Crellin Park", "Chatham", "NY", ""},
	{"jenkinsville", "Jenkinsville Rd Fields", "Jenkinsville Rd", "Fort Ann", "NY", ""},
	{"gloversville", "Gloversville High School", "Gloversville High School", "Gloversville", "NY", ""},
	{"bethlehemcomplex", "Bethlehem Soccer Complex", "450 Wemple Road", "Glenmont", "NY", "12077"},
	{"rte2", "Rte. 2 Soccer Complex", "Rte. 2 Soccer Complex", "Brunswick", "NY", ""},
	{"malwyck", "Malwyck", "Malwyck", "Scotia", "NY", ""},
	{"eastgreenbush", "East Greenbush Soccer Club", "100 Phillips Rd", "East Greenbush", "NY", ""},
}

func seedHistoricalLocations(locationService *services.LocationService) (map[string]int, error) {
	ids := make(map[string]int, len(historicalLocationDefs))
	for _, loc := range historicalLocationDefs {
		id, err := locationService.CreateLocation(&services.LocationForm{
			Name:          loc.Name,
			Address1:      loc.Address1,
			City:          loc.City,
			StateProvince: loc.StateProvince,
			ZipCode:       loc.ZipCode,
		}, "system")
		if err != nil {
			return nil, err
		}
		ids[loc.Key] = id
	}
	return ids, nil
}

type historicalSeasonDef struct {
	Key       string
	Name      string
	StartDate string
	EndDate   string
}

// historicalSeasonDefs' boundaries follow the blank-row breaks in the
// source spreadsheet — a spring and a fall season per year.
var historicalSeasonDefs = []historicalSeasonDef{
	{"spring2024", "Spring 2024", "2024-05-05", "2024-06-30"},
	{"fall2024", "Fall 2024", "2024-09-08", "2024-10-27"},
	{"spring2025", "Spring 2025", "2025-05-04", "2025-06-29"},
	{"fall2025", "Fall 2025", "2025-09-07", "2025-10-26"},
	{"spring2026", "Spring 2026", "2026-05-03", "2026-06-28"},
}

func seedHistoricalSeasonRows(db *sql.DB) (map[string]int, error) {
	sm := &models.SeasonModel{DB: db}
	ids := make(map[string]int, len(historicalSeasonDefs))
	for _, s := range historicalSeasonDefs {
		start, err := time.Parse("2006-01-02", s.StartDate)
		if err != nil {
			return nil, err
		}
		end, err := time.Parse("2006-01-02", s.EndDate)
		if err != nil {
			return nil, err
		}

		id, err := sm.Insert(&models.Season{
			LeagueID:  1,
			Name:      s.Name,
			StartDate: sql.NullTime{Time: start, Valid: true},
			EndDate:   sql.NullTime{Time: end, Valid: true},
		})
		if err != nil {
			return nil, err
		}
		ids[s.Key] = id
	}
	return ids, nil
}

type historicalMatch struct {
	Season    string
	Date      string
	Home      string
	Away      string
	Location  string
	HomeScore *int
	AwayScore *int
	Notes     string
}

func intp(v int) *int { return &v }

var historicalMatches = []historicalMatch{
	// Spring 2024 — only a win/loss letter was recorded, no scores.
	{"spring2024", "2024-05-05", "Latham Almost United", "Colonial FC", "niskayuna", nil, nil, "Colonial FC won (score not recorded)"},
	{"spring2024", "2024-05-19", "Hudson FC", "Colonial FC", "greenport", nil, nil, "Colonial FC lost (score not recorded)"},
	{"spring2024", "2024-06-02", "Colonial FC", "GSL FC - Active Ingredient Brewing Co", "clifton", nil, nil, "Colonial FC won (score not recorded)"},
	{"spring2024", "2024-06-09", "Colonial FC", "Corona FC", "clifton", nil, nil, "Colonial FC lost (score not recorded)"},
	{"spring2024", "2024-06-16", "Guilderland FC - Marisa's Lace Pizzeria Rest.", "Colonial FC", "nott", nil, nil, "Colonial FC won (score not recorded)"},
	{"spring2024", "2024-06-23", "Colonial FC", "Strikers FC", "clifton", nil, nil, "Colonial FC lost (score not recorded)"},
	{"spring2024", "2024-06-30", "Saratoga FC", "Colonial FC", "wilton", nil, nil, "Colonial FC won (score not recorded)"},

	// Fall 2024
	{"fall2024", "2024-09-08", "Chatham SOMA", "Colonial FC", "crellin", intp(1), intp(3), ""},
	{"fall2024", "2024-09-15", "Colonial FC", "Bethlehem FC", "clifton", intp(7), intp(4), ""},
	{"fall2024", "2024-09-22", "Adirondack", "Colonial FC", "jenkinsville", intp(5), intp(3), ""},
	{"fall2024", "2024-09-29", "GSL FC - Active Ingredient Brewing Co", "Colonial FC", "gloversville", intp(4), intp(3), ""},
	{"fall2024", "2024-10-06", "Colonial FC", "Strikers FC", "clifton", intp(3), intp(5), ""},
	{"fall2024", "2024-10-20", "Colonial FC", "Albany - Parkitects", "clifton", intp(2), intp(4), ""},
	{"fall2024", "2024-10-27", "Colonial FC", "Hudson FC", "clifton", nil, nil, "Colonial FC lost (score not recorded)"},

	// Spring 2025
	{"spring2025", "2025-05-04", "Corona FC", "Colonial FC", "clifton", intp(7), intp(2), ""},
	{"spring2025", "2025-05-18", "Colonial FC", "Adirondack", "clifton", intp(3), intp(2), ""},
	{"spring2025", "2025-06-01", "Albany - Parkitects", "Colonial FC", "bethlehemcomplex", intp(5), intp(1), ""},
	{"spring2025", "2025-06-08", "Brunswick FC", "Colonial FC", "rte2", intp(5), intp(0), ""},
	{"spring2025", "2025-06-15", "Guilderland FC - Marisa's Lace Pizzeria Rest.", "Colonial FC", "nott", intp(2), intp(5), ""},
	{"spring2025", "2025-06-22", "Colonial FC", "Crazy Dawg T Shirts", "clifton", intp(9), intp(2), ""},
	{"spring2025", "2025-06-29", "Greene FC - I.T.S. Inc", "Colonial FC", "eastgreenbush", intp(2), intp(5), ""},

	// Fall 2025
	{"fall2025", "2025-09-07", "Strikers FC", "Colonial FC", "clifton", intp(4), intp(1), ""},
	{"fall2025", "2025-09-14", "Brunswick FC", "Colonial FC", "rte2", intp(0), intp(2), ""},
	{"fall2025", "2025-09-21", "Chatham SOMA", "Colonial FC", "crellin", intp(5), intp(6), ""},
	{"fall2025", "2025-09-28", "GSL FC - Active Ingredient Brewing Co", "Colonial FC", "gloversville", intp(7), intp(1), ""},
	{"fall2025", "2025-10-05", "Bethlehem FC", "Colonial FC", "nott", nil, nil, "Result not recorded"},
	{"fall2025", "2025-10-19", "Colonial FC", "Atletico Clifton Park", "malwyck", nil, nil, "Result not recorded"},
	{"fall2025", "2025-10-26", "Colonial FC", "Knick91SC", "malwyck", nil, nil, "Result not recorded"},

	// Spring 2026
	{"spring2026", "2026-05-03", "Colonial FC", "Albany - Parkitects", "eastgreenbush", intp(5), intp(2), ""},
	{"spring2026", "2026-05-17", "Brunswick FC", "Colonial FC", "rte2", intp(4), intp(8), ""},
	{"spring2026", "2026-06-07", "Strikers FC", "Colonial FC", "clifton", intp(2), intp(5), "Strikers only had 10 players"},
	{"spring2026", "2026-06-14", "Colonial FC", "GSL FC - Active Ingredient Brewing Co", "eastgreenbush", intp(4), intp(2), "GSL only had 8 players but we loaned them some"},
	{"spring2026", "2026-06-21", "Colonial FC", "Legends", "eastgreenbush", intp(1), intp(1), ""},
	{"spring2026", "2026-06-28", "Colonial FC", "Hudson FC", "eastgreenbush", intp(2), intp(2), ""},
}

func seedHistoricalMatchRows(db *sql.DB, teamIDs, locationIDs, seasonIDs map[string]int) error {
	mm := &models.MatchModel{DB: db}
	for _, hm := range historicalMatches {
		matchDate, err := time.Parse("2006-01-02", hm.Date)
		if err != nil {
			return err
		}

		match := &models.Match{
			SeasonID:   seasonIDs[hm.Season],
			HomeTeamID: teamIDs[hm.Home],
			AwayTeamID: teamIDs[hm.Away],
			MatchDate:  matchDate,
			LocationID: sql.NullInt32{Int32: int32(locationIDs[hm.Location]), Valid: true},
			Notes:      sql.NullString{String: hm.Notes, Valid: hm.Notes != ""},
		}
		if hm.HomeScore != nil && hm.AwayScore != nil {
			match.HomeScore = sql.NullInt32{Int32: int32(*hm.HomeScore), Valid: true}
			match.AwayScore = sql.NullInt32{Int32: int32(*hm.AwayScore), Valid: true}
		}

		if _, err := mm.Insert(match); err != nil {
			return err
		}
	}
	return nil
}

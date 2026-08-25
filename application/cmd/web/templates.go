package main

import (
	"html/template"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/ui"
)

// Breadcrumb is one entry in a page's breadcrumb trail. URL is empty for the
// current page, which templates render as plain text instead of a link.
type Breadcrumb struct {
	Label string
	URL   string
}

// NavTeamInfo is the lightweight {ID, Name} shape powering the nav's "My
// Teams" dropdown — populated once per request in newTemplateData.
type NavTeamInfo struct {
	ID   int
	Name string
}

// NavLeagueInfo is the same shape as NavTeamInfo, powering the nav's "My
// Leagues (Admin)" dropdown.
type NavLeagueInfo struct {
	ID   int
	Name string
}

type templateData struct {
	CurrentYear     int
	LastUpdate      string
	Form            any
	Data            any
	SupportData     any
	Flash           string
	IsAuthenticated bool
	IsActive        bool
	IsAdmin         bool
	UserName        string
	PlayerID        int
	MyTeams         []NavTeamInfo
	MyAdminLeagues  []NavLeagueInfo
	CSRFToken       string
	Breadcrumbs     []Breadcrumb
}

func pickerDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// Create a humanDate function which returns a nicely formatted string
// representation of a time.Time object.
func humanDate(t time.Time) string {
	return t.Format("01/02/2006")
}

// humanDateShort drops the year humanDate includes — for tables already
// scoped to a single season (a team's schedule, a season's match list),
// where the season name already conveys the year and repeating it in every
// row just crowds out room mobile screens need for the columns that
// actually vary (opponent, score).
func humanDateShort(t time.Time) string {
	return t.Format("01/02")
}

// easternLocation is loaded once at startup; falling back to UTC (rather
// than failing to start) if the container's tzdata is ever missing, since
// both this session's dev and production images already install it.
var easternLocation = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// humanDateTime formats t (stored as UTC, e.g. via UTC_TIMESTAMP()) in US
// Eastern time with both date and time-of-day, plus the zone abbreviation
// (EST/EDT) so it stays unambiguous across the daylight-saving switch —
// used where the exact moment matters (Last Login), unlike humanDate's
// date-only formatting for everything else.
func humanDateTime(t time.Time) string {
	return t.In(easternLocation).Format("01/02/2006 3:04 PM MST")
}

// templateRowWrapper bundles the root template data alongside one row for
// match-update.html's goal-row/card-row sub-templates — Go's {{template}}
// action only takes one pipeline argument, so goalRowData/cardRowData pack
// both into a single value. Row is `any` (not *goalFormRow/*cardFormRow
// directly) so the same wrapper works for both, and so the blank
// <template> row can pass literal `nil` — a nil pointer is falsy to
// {{if .Row}}, distinguishing "no row" from "a real row whose fields all
// happen to be zero" (an unattributed goal/card).
type templateRowWrapper struct {
	Root any
	Row  any
}

func goalRowData(root, row any) templateRowWrapper { return templateRowWrapper{Root: root, Row: row} }
func cardRowData(root, row any) templateRowWrapper { return templateRowWrapper{Root: root, Row: row} }
func teamNoteSideData(root, row any) templateRowWrapper {
	return templateRowWrapper{Root: root, Row: row}
}

// mapsURL builds a Google Maps search link for an address — no API key
// needed, just the documented search-by-query URL scheme.
func mapsURL(a *models.Address) string {
	if a == nil {
		return ""
	}

	parts := []string{}
	if a.Address1.Valid && a.Address1.String != "" {
		parts = append(parts, a.Address1.String)
	}
	if a.Address2.Valid && a.Address2.String != "" {
		parts = append(parts, a.Address2.String)
	}
	if a.City.Valid && a.City.String != "" {
		parts = append(parts, a.City.String)
	}
	if a.StateProvince.Valid && a.StateProvince.String != "" {
		parts = append(parts, a.StateProvince.String)
	}
	if a.ZipCode.Valid && a.ZipCode.String != "" {
		parts = append(parts, a.ZipCode.String)
	}
	if len(parts) == 0 {
		return ""
	}

	return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(strings.Join(parts, ", "))
}

var functions = template.FuncMap{
	"pickerDate":       pickerDate,
	"humanDate":        humanDate,
	"humanDateShort":   humanDateShort,
	"humanDateTime":    humanDateTime,
	"mapsURL":          mapsURL,
	"goalRowData":      goalRowData,
	"cardRowData":      cardRowData,
	"teamNoteSideData": teamNoteSideData,
}

func newTemplateCache() (map[string]*template.Template, error) {
	cache := map[string]*template.Template{}

	pages, err := fs.Glob(ui.Files, "html/pages/*.html")
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		name := filepath.Base(page)

		patterns := []string{
			"html/base.html",
			"html/partials/*.html",
			page,
		}

		ts, err := template.New(name).Funcs(functions).ParseFS(ui.Files, patterns...)
		if err != nil {
			return nil, err
		}

		cache[name] = ts
	}

	return cache, nil
}

func getTemplateSet(page string) (*template.Template, error) {
	patterns := []string{
		"./ui/html/base.html",
		"./ui/html/partials/nav.html",
		"./ui/html/partials/breadcrumbs.html",
		"./ui/html/partials/player-form-fields.html",
		"./ui/html/partials/team-form-fields.html",
		"./ui/html/partials/league-form-fields.html",
		"./ui/html/partials/location-form-fields.html",
		"./ui/html/partials/season-form-fields.html",
		"./ui/html/partials/match-form-fields.html",
		"./ui/html/partials/leader-tables.html",
		"./ui/html/pages/" + page,
	}

	ts, err := template.New(page).Funcs(functions).ParseFiles(patterns...)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

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
	CurrentYear       int
	LastUpdate        string
	Form              any
	Data              any
	SupportData       any
	Flash             string
	IsAuthenticated   bool
	IsActive          bool
	IsAdmin           bool
	IsRealAdmin       bool
	ViewingAsPlayer   bool
	UserName          string
	PlayerID          int
	MyTeams           []NavTeamInfo
	MyAdminLeagues    []NavLeagueInfo
	CSRFToken         string
	Breadcrumbs       []Breadcrumb
	NextURL           string
	SMSFeatureEnabled bool
}

func pickerDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// hasMatchTime reports whether t carries a real kickoff time, as opposed
// to the "no time recorded" sentinel: exactly midnight in whatever zone
// the value was actually stored in (UTC, both for a fresh DB read and for
// every match created before this feature or seeded via time.Parse's
// implicit UTC). Deliberately checked on the raw value, NOT converted to
// Eastern first — a bare historical date widened from DATE to DATETIME
// really is UTC midnight, and converting it to Eastern first would land
// on ~7-8pm the *previous* day (whichever DST offset applies) and look
// like a bogus real time rather than "no time recorded". A genuine
// Eastern kickoff time (from parseRequiredMatchDateTime) only collides
// with this sentinel in the rare case of a match scheduled for exactly
// 7pm/8pm Eastern (whichever is UTC midnight that day) — an accepted,
// narrow edge case, far better than every historical match display being
// wrong.
func hasMatchTime(t time.Time) bool {
	return !(t.Hour() == 0 && t.Minute() == 0)
}

// matchPickerDate/matchPickerTime split a match's stored kickoff instant
// back into the two <input type=date>/<input type=time> values the edit
// form needs. Only converted to Eastern when a real time is on file (see
// hasMatchTime) — a sentinel value is shown/left exactly as stored
// (matchPickerTime blank), so re-saving an old match without touching the
// time field never accidentally invents a fake kickoff time for it.
func matchPickerDate(t time.Time) string {
	if !hasMatchTime(t) {
		return t.Format("2006-01-02")
	}
	return t.In(easternLocation).Format("2006-01-02")
}

func matchPickerTime(t time.Time) string {
	if !hasMatchTime(t) {
		return ""
	}
	return t.In(easternLocation).Format("15:04")
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

// matchDateTime/matchDateTimeShort format a match's own date for display,
// appending the Eastern kickoff time when one's on file (see
// hasMatchTime) — used specifically for a match's own date, not other
// unrelated dates on the same page. The date and time are always derived
// from the *same* converted instant, never mixing a raw-UTC date with an
// Eastern-converted time (that mismatch is exactly what produced a wrong
// date and a fabricated time for historical matches during development).
func matchDateTime(t time.Time) string {
	if !hasMatchTime(t) {
		return humanDate(t)
	}
	e := t.In(easternLocation)
	return e.Format("01/02/2006") + " " + e.Format("3:04 PM")
}

func matchDateTimeShort(t time.Time) string {
	if !hasMatchTime(t) {
		return humanDateShort(t)
	}
	e := t.In(easternLocation)
	return e.Format("01/02") + " " + e.Format("3:04 PM")
}

// matchKickoffTime formats just the Eastern kickoff time, no date — for
// display alongside a date that's already shown once elsewhere (a matchday
// heading grouping several match cards under one date).
func matchKickoffTime(t time.Time) string {
	if !hasMatchTime(t) {
		return ""
	}
	return t.In(easternLocation).Format("3:04 PM")
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
	"pickerDate":         pickerDate,
	"humanDate":          humanDate,
	"humanDateShort":     humanDateShort,
	"humanDateTime":      humanDateTime,
	"matchDateTime":      matchDateTime,
	"matchDateTimeShort": matchDateTimeShort,
	"matchKickoffTime":   matchKickoffTime,
	"mapsURL":            mapsURL,
	"goalRowData":        goalRowData,
	"cardRowData":        cardRowData,
	"teamNoteSideData":   teamNoteSideData,
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

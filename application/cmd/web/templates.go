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
	"pickerDate": pickerDate,
	"humanDate":  humanDate,
	"mapsURL":    mapsURL,
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

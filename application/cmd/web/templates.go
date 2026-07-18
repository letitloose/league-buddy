package main

import (
	"html/template"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/letitloose/league-buddy/ui"
)

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
	TeamID          int
	IsCaptain       bool
	CSRFToken       string
}

func pickerDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// Create a humanDate function which returns a nicely formatted string
// representation of a time.Time object.
func humanDate(t time.Time) string {
	return t.Format("01/02/2006")
}

var functions = template.FuncMap{
	"pickerDate": pickerDate,
	"humanDate":  humanDate,
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
		"./ui/html/partials/player-form-fields.html",
		"./ui/html/partials/team-form-fields.html",
		"./ui/html/partials/league-form-fields.html",
		"./ui/html/pages/" + page,
	}

	ts, err := template.New(page).Funcs(functions).ParseFiles(patterns...)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

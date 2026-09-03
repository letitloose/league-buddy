package main

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/letitloose/league-buddy/internal/models"
)

// calendarFeed serves one player's personal iCalendar subscription feed —
// deliberately unauthenticated (see routes.go: registered directly on the
// bare router, not the dynamic/session chain), since a phone's calendar
// app fetches this with no cookies at all. The secret :token is the only
// access control (see CalendarService.BuildFeed/RegenerateToken).
func (app *application) calendarFeed(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())
	token := params.ByName("token")

	feed, err := app.calendarService.BuildFeed(token)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
			return
		}
		app.serverError(w, err)
		return
	}

	// No Content-Disposition — this must read as a live, refetchable feed
	// to a calendar app, not a one-time file download.
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(feed)
}

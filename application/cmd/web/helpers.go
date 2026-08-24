package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/justinas/nosurf"
	"github.com/letitloose/league-buddy/internal/models"
)

func (app *application) newTemplateData(r *http.Request) *templateData {
	data := &templateData{
		CurrentYear:     time.Now().Year(),
		LastUpdate:      os.Getenv("SOFTWARE_LAST_UPDATE"),
		Flash:           app.sessionManager.PopString(r.Context(), "flash"),
		IsAuthenticated: app.isAuthenticated(r),
		IsActive:        app.isActive(r),
		IsAdmin:         app.isAdmin(r),
		PlayerID:        app.getPlayerID(r),
		UserName:        app.getUserName(r),
		CSRFToken:       nosurf.Token(r),
	}

	if playerID := app.getPlayerID(r); app.isActive(r) && playerID > 0 {
		tmm := &models.TeamMemberModel{DB: app.playerService.DB}
		if teams, err := tmm.GetTeamsForPlayer(playerID); err == nil {
			for _, team := range teams {
				data.MyTeams = append(data.MyTeams, NavTeamInfo{ID: team.ID, Name: team.Name})
			}
		} else {
			app.errorLog.Println(err)
		}

		lam := &models.LeagueAdminModel{DB: app.playerService.DB}
		if leagues, err := lam.GetLeaguesForPlayer(playerID); err == nil {
			for _, league := range leagues {
				data.MyAdminLeagues = append(data.MyAdminLeagues, NavLeagueInfo{ID: league.ID, Name: league.Name})
			}
		} else {
			app.errorLog.Println(err)
		}
	}

	return data
}

func (app *application) serverError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	app.errorLog.Output(2, trace)

	app.renderErrorPage(w, http.StatusInternalServerError)
}

func (app *application) clientError(w http.ResponseWriter, status int) {
	app.renderErrorPage(w, status)
}

func (app *application) notFound(w http.ResponseWriter) {
	app.clientError(w, http.StatusNotFound)
}

// errorPageData is the shape error.html renders: the status code plus a
// friendly headline/message pair, looked up in errorPageCopy (falling back
// to the bare status text for anything not listed there).
type errorPageData struct {
	StatusCode int
	Headline   string
	Message    string
}

var errorPageCopy = map[int]struct{ Headline, Message string }{
	http.StatusBadRequest:          {"Bad request", "Something about that request didn't look right."},
	http.StatusForbidden:           {"Access denied", "You don't have permission to view this page."},
	http.StatusNotFound:            {"Page not found", "The page you're looking for doesn't exist, or may have moved."},
	http.StatusConflict:            {"That changed under you", "Someone else's change got there first — refresh and try again."},
	http.StatusUnprocessableEntity: {"Couldn't process that", "Double-check the form and try again."},
	http.StatusInternalServerError: {"Something went wrong", "An unexpected error occurred on our end. Try again, or head back home."},
}

// renderErrorPage renders the branded error.html page for status, in place
// of Go's bare-text http.Error. Deliberately doesn't go through the shared
// render() (which itself calls serverError on failure) — a broken
// error.html would otherwise recurse back into this function forever, so
// any failure here falls back to a plain http.Error instead.
func (app *application) renderErrorPage(w http.ResponseWriter, status int) {
	entry, ok := errorPageCopy[status]
	if !ok {
		entry = struct{ Headline, Message string }{http.StatusText(status), "Try again, or head back home."}
	}

	data := &templateData{
		CurrentYear: time.Now().Year(),
		LastUpdate:  os.Getenv("SOFTWARE_LAST_UPDATE"),
		Data: &errorPageData{
			StatusCode: status,
			Headline:   entry.Headline,
			Message:    entry.Message,
		},
	}

	var ts *template.Template
	var err error
	if app.useTemplateCache {
		var found bool
		ts, found = app.templateCache["error.html"]
		if !found {
			err = fmt.Errorf("the template error.html does not exist")
		}
	} else {
		ts, err = getTemplateSet("error.html")
	}
	if err != nil {
		http.Error(w, http.StatusText(status), status)
		return
	}

	buf := new(bytes.Buffer)
	if err := ts.ExecuteTemplate(buf, "base", data); err != nil {
		http.Error(w, http.StatusText(status), status)
		return
	}

	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (app *application) render(w http.ResponseWriter, status int, page string, data *templateData) {
	var ts *template.Template
	var ok bool
	var err error

	if !app.useTemplateCache {
		ts, err = getTemplateSet(page)
		if err != nil {
			app.serverError(w, fmt.Errorf("the template %s does not exist", page))
			return
		}
	} else {
		ts, ok = app.templateCache[page]
		if !ok {
			err := fmt.Errorf("the template %s does not exist", page)
			app.serverError(w, err)
			return
		}
	}

	buf := new(bytes.Buffer)
	err = ts.ExecuteTemplate(buf, "base", data)
	if err != nil {
		app.serverError(w, err)
		return
	}

	w.WriteHeader(status)

	buf.WriteTo(w)
}

func (app *application) isAuthenticated(r *http.Request) bool {
	isAuthenticated, ok := r.Context().Value(isAuthenticatedContextKey).(bool)
	if !ok {
		return false
	}

	return isAuthenticated
}

func (app *application) isActive(r *http.Request) bool {
	isActive, ok := r.Context().Value(isActiveContextKey).(bool)
	if !ok {
		return false
	}

	return isActive
}

func (app *application) isAdmin(r *http.Request) bool {
	isAdmin, ok := r.Context().Value(isAdminContextKey).(bool)
	if !ok {
		return false
	}

	return isAdmin
}

func (app *application) getUserName(r *http.Request) string {
	userName, ok := r.Context().Value(userNameContextKey).(string)
	if !ok {
		return ""
	}

	return userName
}

func (app *application) getPlayerID(r *http.Request) int {
	playerID, ok := r.Context().Value(playerIDContextKey).(int)
	if !ok {
		return 0
	}

	return playerID
}

func (app *application) getTeamIDs(r *http.Request) []int {
	teamIDs, ok := r.Context().Value(teamIDsContextKey).([]int)
	if !ok {
		return nil
	}

	return teamIDs
}

func (app *application) isMemberOfTeam(r *http.Request, teamID int) bool {
	for _, id := range app.getTeamIDs(r) {
		if id == teamID {
			return true
		}
	}
	return false
}

func (app *application) isCaptainOfTeam(r *http.Request, teamID int) bool {
	captainTeamIDs, ok := r.Context().Value(captainTeamIDsContextKey).([]int)
	if !ok {
		return false
	}
	for _, id := range captainTeamIDs {
		if id == teamID {
			return true
		}
	}
	return false
}

func (app *application) getLeagueAdminLeagueIDs(r *http.Request) []int {
	leagueIDs, ok := r.Context().Value(leagueAdminLeagueIDsContextKey).([]int)
	if !ok {
		return nil
	}

	return leagueIDs
}

func (app *application) isLeagueAdminOfLeague(r *http.Request, leagueID int) bool {
	for _, id := range app.getLeagueAdminLeagueIDs(r) {
		if id == leagueID {
			return true
		}
	}
	return false
}

func (app *application) isLeagueAdminOfTeam(r *http.Request, teamID int) bool {
	leagueAdminTeamIDs, ok := r.Context().Value(leagueAdminTeamIDsContextKey).([]int)
	if !ok {
		return false
	}
	for _, id := range leagueAdminTeamIDs {
		if id == teamID {
			return true
		}
	}
	return false
}

// canManageTeam reports whether the current request's user may edit teamID's
// info, manage its roster, or reassign its captain: an Admin, the team's own
// captain, or a league admin of the team's league.
func (app *application) canManageTeam(r *http.Request, teamID int) bool {
	return app.isAdmin(r) || app.isCaptainOfTeam(r, teamID) || app.isLeagueAdminOfTeam(r, teamID)
}

// canDeleteTeam reports whether the current request's user may delete
// teamID: an Admin or a league admin of the team's league. Deliberately
// excludes plain captains — deleting a team is too destructive to leave to a
// single unilateral captain.
func (app *application) canDeleteTeam(r *http.Request, teamID int) bool {
	return app.isAdmin(r) || app.isLeagueAdminOfTeam(r, teamID)
}

// canManageLeague reports whether the current request's user may create a
// team in leagueID: an Admin or a league admin of that league.
func (app *application) canManageLeague(r *http.Request, leagueID int) bool {
	return app.isAdmin(r) || app.isLeagueAdminOfLeague(r, leagueID)
}

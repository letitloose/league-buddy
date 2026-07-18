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
)

func (app *application) newTemplateData(r *http.Request) *templateData {
	return &templateData{
		CurrentYear:     time.Now().Year(),
		LastUpdate:      os.Getenv("SOFTWARE_LAST_UPDATE"),
		Flash:           app.sessionManager.PopString(r.Context(), "flash"),
		IsAuthenticated: app.isAuthenticated(r),
		IsActive:        app.isActive(r),
		IsAdmin:         app.isAdmin(r),
		PlayerID:        app.getPlayerID(r),
		TeamID:          app.getTeamID(r),
		IsCaptain:       app.isCaptain(r),
		UserName:        app.getUserName(r),
		CSRFToken:       nosurf.Token(r),
	}
}

func (app *application) serverError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	app.errorLog.Output(2, trace)

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (app *application) clientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

func (app *application) notFound(w http.ResponseWriter) {
	app.clientError(w, http.StatusNotFound)
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

func (app *application) getTeamID(r *http.Request) int {
	teamID, ok := r.Context().Value(teamIDContextKey).(int)
	if !ok {
		return 0
	}

	return teamID
}

func (app *application) isCaptain(r *http.Request) bool {
	isCaptain, ok := r.Context().Value(isCaptainContextKey).(bool)
	if !ok {
		return false
	}

	return isCaptain
}

package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/services"
)

func (app *application) leagueList(w http.ResponseWriter, r *http.Request) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	leagues, err := lm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = leagues

	app.render(w, http.StatusOK, "league-list.html", data)
}

// leagueViewData wraps a league alongside its teams for the detail page.
// CanManage gates the Add Team button and per-team Edit/Delete links —
// admins and league admins of this league. Admins gates the admin-only
// panel where CanManage of the admin themselves does not — Admins is only
// populated for system admins, who alone can assign/revoke league admins.
type leagueViewData struct {
	League    *models.League
	Teams     []*models.Team
	CanManage bool
	Admins    []*models.Player
}

func (app *application) leagueView(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	tm := &models.TeamModel{DB: app.playerService.DB}
	teams, err := tm.GetByLeague(id)
	if err != nil {
		app.serverError(w, err)
		return
	}

	var admins []*models.Player
	if app.isAdmin(r) {
		lam := &models.LeagueAdminModel{DB: app.playerService.DB}
		admins, err = lam.ListAdminsForLeague(id)
		if err != nil {
			app.serverError(w, err)
			return
		}
	}

	data := app.newTemplateData(r)
	data.Data = &leagueViewData{
		League:    league,
		Teams:     teams,
		CanManage: app.canManageLeague(r, id),
		Admins:    admins,
	}
	data.Breadcrumbs = []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: league.Name},
	}

	app.render(w, http.StatusOK, "league-view.html", data)
}

// leagueFormBreadcrumbs is the shared "Leagues / Add League" trail for the
// create form and its validation-error re-render.
func leagueFormBreadcrumbs() []Breadcrumb {
	return []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: "Add League"},
	}
}

func (app *application) leagueForm(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = services.LeagueForm{}
	data.Breadcrumbs = leagueFormBreadcrumbs()
	app.render(w, http.StatusOK, "league-create.html", data)
}

func (app *application) leagueCreate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := &services.LeagueForm{
		Name:            r.PostForm.Get("name"),
		Motto:           r.PostForm.Get("motto"),
		EstablishedDate: r.PostForm.Get("establisheddate"),
	}

	id, err := app.leagueService.CreateLeague(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			data.Breadcrumbs = leagueFormBreadcrumbs()
			app.render(w, http.StatusUnprocessableEntity, "league-create.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been created!")
	http.Redirect(w, r, fmt.Sprintf("/league/%d", id), http.StatusSeeOther)
}

func (app *application) leagueUpdate(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	form := &services.LeagueForm{ID: league.ID, Name: league.Name, Motto: league.Motto.String}
	if league.EstablishedDate.Valid {
		form.EstablishedDate = pickerDate(league.EstablishedDate.Time)
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.Breadcrumbs = []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)},
		{Label: "Edit"},
	}

	app.render(w, http.StatusOK, "league-update.html", data)
}

func (app *application) leagueUpdatePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PostForm.Get("league-id"))
	if err != nil || id < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := &services.LeagueForm{
		ID:              id,
		Name:            r.PostForm.Get("name"),
		Motto:           r.PostForm.Get("motto"),
		EstablishedDate: r.PostForm.Get("establisheddate"),
	}

	err = app.leagueService.UpdateLeague(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			data.Breadcrumbs = []Breadcrumb{
				{Label: "Leagues", URL: "/league"},
				{Label: form.Name, URL: fmt.Sprintf("/league/%d", form.ID)},
				{Label: "Edit"},
			}
			app.render(w, http.StatusUnprocessableEntity, "league-update.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been updated!")
	http.Redirect(w, r, fmt.Sprintf("/league/%d", form.ID), http.StatusSeeOther)
}

func (app *application) leagueDelete(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	err = app.leagueService.DeleteLeague(id, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrHasDependents) {
			app.clientError(w, http.StatusConflict)
			return
		}
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// leagueAddAdmin and leagueRemoveAdmin are plain form POSTs (leagueID plus
// email/playerID) rather than the JSON+fetch toggle pattern used elsewhere,
// mirroring teamSetCaptain's "plain form POST for a one-off admin action".
// Both are admin-tier routes — see routes.go for why assigning league admins
// stays system-admin-only.
func (app *application) leagueAddAdmin(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	leagueID, err := strconv.Atoi(r.PostForm.Get("leagueID"))
	if err != nil || leagueID < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	email := r.PostForm.Get("email")

	err = app.leagueService.AddLeagueAdmin(leagueID, email, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			app.sessionManager.Put(r.Context(), "flash", "No player found with that email.")
		} else if errors.Is(err, models.ErrDuplicateLeagueAdmin) {
			app.sessionManager.Put(r.Context(), "flash", "That player already administers this league.")
		} else {
			app.serverError(w, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/league/%d", leagueID), http.StatusSeeOther)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "League admin added.")
	http.Redirect(w, r, fmt.Sprintf("/league/%d", leagueID), http.StatusSeeOther)
}

func (app *application) leagueRemoveAdmin(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	leagueID, err := strconv.Atoi(r.PostForm.Get("leagueID"))
	if err != nil || leagueID < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	playerID, err := strconv.Atoi(r.PostForm.Get("playerID"))
	if err != nil || playerID < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	if err := app.leagueService.RemoveLeagueAdmin(leagueID, playerID, app.getUserName(r)); err != nil {
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "League admin removed.")
	http.Redirect(w, r, fmt.Sprintf("/league/%d", leagueID), http.StatusSeeOther)
}

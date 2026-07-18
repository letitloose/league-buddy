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
type leagueViewData struct {
	League *models.League
	Teams  []*models.Team
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

	data := app.newTemplateData(r)
	data.Data = &leagueViewData{League: league, Teams: teams}

	app.render(w, http.StatusOK, "league-view.html", data)
}

func (app *application) leagueForm(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = services.LeagueForm{}
	app.render(w, http.StatusOK, "league-create.html", data)
}

func (app *application) leagueCreate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := &services.LeagueForm{
		Name: r.PostForm.Get("name"),
	}

	id, err := app.leagueService.CreateLeague(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
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

	data := app.newTemplateData(r)
	data.Form = &services.LeagueForm{ID: league.ID, Name: league.Name}

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
		ID:   id,
		Name: r.PostForm.Get("name"),
	}

	err = app.leagueService.UpdateLeague(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
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

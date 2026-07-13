package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/services"
)

func (app *application) playerForm(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = services.PlayerForm{}
	app.render(w, http.StatusOK, "player-create.html", data)
}

func (app *application) playerCreate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := &services.PlayerForm{
		FirstName:     r.PostForm.Get("firstname"),
		LastName:      r.PostForm.Get("lastname"),
		DateOfBirth:   r.PostForm.Get("dateofbirth"),
		Address1:      r.PostForm.Get("address1"),
		Address2:      r.PostForm.Get("address2"),
		City:          r.PostForm.Get("city"),
		StateProvince: r.PostForm.Get("stateprovince"),
		ZipCode:       r.PostForm.Get("zipcode"),
		Country:       r.PostForm.Get("country"),
		Email:         r.PostForm.Get("email"),
		PhoneNumber:   r.PostForm.Get("phonenumber"),
	}

	playerID, err := app.playerService.AddPlayer(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "player-create.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	if app.emailService != nil && form.Email != "" {
		body := fmt.Sprintf(
			`<html>
				<body>
					<p>You've been added to the League Buddy roster. <a href="https://%s/user/signup">Sign up here</a> to manage your profile.<p>
				</body>
			</html>`, os.Getenv("VIRTUAL_HOST"))
		_ = app.emailService.SendEmailV2("You've been added to the League Buddy roster", "", body, form.Email)
	}

	app.sessionManager.Put(r.Context(), "flash", form.FirstName+" "+form.LastName+" has been added to the roster!")
	http.Redirect(w, r, fmt.Sprintf("/player/view/%d", playerID), http.StatusSeeOther)
}

func (app *application) playerList(w http.ResponseWriter, r *http.Request) {
	tm := &models.TeamModel{DB: app.playerService.DB}
	team, err := tm.GetDefault()
	if err != nil {
		app.serverError(w, err)
		return
	}

	players, err := app.playerService.GetByTeam(team.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = players

	app.render(w, http.StatusOK, "player-list.html", data)
}

func (app *application) playerSearch(w http.ResponseWriter, r *http.Request) {
	tm := &models.TeamModel{DB: app.playerService.DB}
	team, err := tm.GetDefault()
	if err != nil {
		app.serverError(w, err)
		return
	}

	criteria := &models.PlayerSearchCriteria{TeamID: team.ID, Limit: 20}

	params := r.URL.Query()
	criteria.FirstName = params.Get("firstname")
	criteria.LastName = params.Get("lastname")
	criteria.Email = params.Get("email")
	criteria.Sort = params.Get("sort")
	criteria.Order = params.Get("order")

	if offset := params.Get("offset"); offset != "" {
		criteria.Offset, err = strconv.Atoi(offset)
		if err != nil {
			app.clientError(w, http.StatusBadRequest)
			return
		}
	}

	if limit := params.Get("limit"); limit != "" {
		criteria.Limit, err = strconv.Atoi(limit)
		if err != nil {
			app.clientError(w, http.StatusBadRequest)
			return
		}
	}

	results, err := app.playerService.Search(criteria)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = results

	app.render(w, http.StatusOK, "player-list.html", data)
}

func (app *application) playerView(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	player, err := app.playerService.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	data := app.newTemplateData(r)
	data.Data = player

	app.render(w, http.StatusOK, "player-view.html", data)
}

func (app *application) playerUpdate(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	if !app.isAdmin(r) && app.getPlayerID(r) != id {
		http.Redirect(w, r, "/player", http.StatusSeeOther)
		return
	}

	player, err := app.playerService.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	form := &services.PlayerForm{
		ID:        player.ID,
		FirstName: player.FirstName,
		LastName:  player.LastName,
		Email:     player.Email.String,
	}
	if player.DateOfBirth.Valid {
		form.DateOfBirth = pickerDate(player.DateOfBirth.Time)
	}
	form.PhoneNumber = player.PhoneNumber.String

	if player.AddressID.Valid {
		am := &models.AddressModel{DB: app.playerService.DB}
		address, err := am.Get(int(player.AddressID.Int32))
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			app.serverError(w, err)
			return
		}
		if address != nil {
			form.Address1 = address.Address1.String
			form.Address2 = address.Address2.String
			form.City = address.City.String
			form.StateProvince = address.StateProvince.String
			form.ZipCode = address.ZipCode.String
			form.Country = address.Country.String
		}
	}

	data := app.newTemplateData(r)
	data.Form = form

	app.render(w, http.StatusOK, "player-update.html", data)
}

func (app *application) playerUpdatePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PostForm.Get("player-id"))
	if err != nil || id < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	if !app.isAdmin(r) && app.getPlayerID(r) != id {
		http.Redirect(w, r, "/player", http.StatusSeeOther)
		return
	}

	form := &services.PlayerForm{
		ID:            id,
		FirstName:     r.PostForm.Get("firstname"),
		LastName:      r.PostForm.Get("lastname"),
		DateOfBirth:   r.PostForm.Get("dateofbirth"),
		Address1:      r.PostForm.Get("address1"),
		Address2:      r.PostForm.Get("address2"),
		City:          r.PostForm.Get("city"),
		StateProvince: r.PostForm.Get("stateprovince"),
		ZipCode:       r.PostForm.Get("zipcode"),
		Country:       r.PostForm.Get("country"),
		Email:         r.PostForm.Get("email"),
		PhoneNumber:   r.PostForm.Get("phonenumber"),
	}

	err = app.playerService.UpdatePlayer(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "player-update.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.FirstName+" "+form.LastName+" has been updated!")
	http.Redirect(w, r, fmt.Sprintf("/player/view/%d", form.ID), http.StatusSeeOther)
}

func (app *application) playerDelete(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	err = app.playerService.DeletePlayer(id, app.getUserName(r))
	if err != nil {
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

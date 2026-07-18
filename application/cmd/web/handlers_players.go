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

// teamRosterData wraps a team alongside its roster (or search results) so
// templates can show team-scoped headings/links without a second lookup.
type teamRosterData struct {
	Team    *models.Team
	Players any
}

func (app *application) getRouteTeam(w http.ResponseWriter, r *http.Request) (*models.Team, bool) {
	params := httprouter.ParamsFromContext(r.Context())

	teamID, err := strconv.Atoi(params.ByName("teamID"))
	if err != nil || teamID < 1 {
		app.notFound(w)
		return nil, false
	}

	tm := &models.TeamModel{DB: app.playerService.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return nil, false
	}

	return team, true
}

func (app *application) playerForm(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	data := app.newTemplateData(r)
	data.Data = team
	data.Form = services.PlayerForm{}
	data.SupportData = models.USStates
	app.render(w, http.StatusOK, "player-create.html", data)
}

func (app *application) playerCreate(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

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
		Email:         r.PostForm.Get("email"),
		PhoneNumber:   r.PostForm.Get("phonenumber"),
	}

	playerID, err := app.playerService.AddPlayer(team.ID, form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Data = team
			data.Form = form
			data.SupportData = models.USStates
			app.render(w, http.StatusUnprocessableEntity, "player-create.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	if form.Email != "" {
		signupLink := fmt.Sprintf("https://%s/user/signup", os.Getenv("VIRTUAL_HOST"))
		if app.emailService != nil {
			body := fmt.Sprintf(
				`<html>
					<body>
						<p>You've been added to the League Buddy roster. <a href="%s">Sign up here</a> to manage your profile.<p>
					</body>
				</html>`, signupLink)
			_ = app.emailService.SendEmailV2("You've been added to the League Buddy roster", "", body, form.Email)
		} else {
			app.infoLog.Printf("no email configured -- roster invite for %s: %s", form.Email, signupLink)
		}
	}

	app.sessionManager.Put(r.Context(), "flash", form.FirstName+" "+form.LastName+" has been added to the roster!")
	http.Redirect(w, r, fmt.Sprintf("/player/view/%d", playerID), http.StatusSeeOther)
}

func (app *application) playerList(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	players, err := app.playerService.GetByTeam(team.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = &teamRosterData{Team: team, Players: players}

	app.render(w, http.StatusOK, "player-list.html", data)
}

func (app *application) playerSearch(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	criteria := &models.PlayerSearchCriteria{TeamID: team.ID, Limit: 20}

	var err error
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
	data.Data = &teamRosterData{Team: team, Players: results}

	app.render(w, http.StatusOK, "player-list.html", data)
}

// playerProfile combines a player with its linked address (if any) for the
// read-only view page. Player's fields are promoted, so templates can
// reference them directly (e.g. .FirstName) alongside .Address.
type playerProfile struct {
	*models.Player
	Address *models.Address
}

// getPlayerAddress fetches the address linked to a player, if any. Returns
// nil (not an error) when the player has no linked address.
func (app *application) getPlayerAddress(player *models.Player) (*models.Address, error) {
	if !player.AddressID.Valid {
		return nil, nil
	}

	am := &models.AddressModel{DB: app.playerService.DB}
	address, err := am.Get(int(player.AddressID.Int32))
	if err != nil && !errors.Is(err, models.ErrNoRecord) {
		return nil, err
	}
	return address, nil
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

	address, err := app.getPlayerAddress(player)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = &playerProfile{Player: player, Address: address}

	app.render(w, http.StatusOK, "player-view.html", data)
}

// canManagePlayer reports whether the current request's user may edit or
// delete the given player: an Admin, the player themself, or the captain of
// the player's own team.
func (app *application) canManagePlayer(r *http.Request, player *models.Player) bool {
	if app.isAdmin(r) {
		return true
	}
	if app.getPlayerID(r) == player.ID {
		return true
	}
	return player.TeamID.Valid && app.isCaptain(r) && app.getTeamID(r) == int(player.TeamID.Int32)
}

func (app *application) playerUpdate(w http.ResponseWriter, r *http.Request) {
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

	if !app.canManagePlayer(r, player) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	address, err := app.getPlayerAddress(player)
	if err != nil {
		app.serverError(w, err)
		return
	}

	form := &services.PlayerForm{
		ID:          player.ID,
		FirstName:   player.FirstName,
		LastName:    player.LastName,
		Email:       player.Email.String,
		PhoneNumber: player.PhoneNumber.String,
	}
	if form.FirstName == models.PlaceholderFirstName && form.LastName == models.PlaceholderLastName {
		// Prompt for a real name instead of making them clear the placeholder first.
		form.FirstName = ""
		form.LastName = ""
	}
	if player.DateOfBirth.Valid {
		form.DateOfBirth = pickerDate(player.DateOfBirth.Time)
	}
	if address != nil {
		form.Address1 = address.Address1.String
		form.Address2 = address.Address2.String
		form.City = address.City.String
		form.StateProvince = address.StateProvince.String
		form.ZipCode = address.ZipCode.String
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.SupportData = models.USStates

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

	player, err := app.playerService.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	if !app.canManagePlayer(r, player) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
		Email:         r.PostForm.Get("email"),
		PhoneNumber:   r.PostForm.Get("phonenumber"),
	}

	err = app.playerService.UpdatePlayer(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			data.SupportData = models.USStates
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

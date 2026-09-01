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

	breadcrumbs, ok := app.teamHierarchyBreadcrumbs(w, team, Breadcrumb{Label: "Add Player"})
	if !ok {
		return
	}

	data := app.newTemplateData(r)
	data.Data = team
	data.Form = services.PlayerForm{}
	data.SupportData = models.USStates
	data.Breadcrumbs = breadcrumbs
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
			breadcrumbs, ok := app.teamHierarchyBreadcrumbs(w, team, Breadcrumb{Label: "Add Player"})
			if !ok {
				return
			}
			data := app.newTemplateData(r)
			data.Data = team
			data.Form = form
			data.SupportData = models.USStates
			data.Breadcrumbs = breadcrumbs
			app.render(w, http.StatusUnprocessableEntity, "player-create.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	if form.Email != "" {
		signupLink := fmt.Sprintf("https://%s/user/signup", os.Getenv("PUBLIC_HOST"))
		if app.emailService != nil {
			body := fmt.Sprintf(
				`<html>
					<body>
						<p>You've been added to the Blame the Ball roster. <a href="%s">Sign up here</a> to manage your profile.<p>
					</body>
				</html>`, signupLink)
			_ = app.emailService.SendEmailV2("You've been added to the Blame the Ball roster", "", body, form.Email)
		} else {
			app.infoLog.Printf("no email configured -- roster invite for %s: %s", form.Email, signupLink)
		}
	}

	app.sessionManager.Put(r.Context(), "flash", form.FirstName+" "+form.LastName+" has been added to the roster!")
	http.Redirect(w, r, fmt.Sprintf("/player/view/%d", playerID), http.StatusSeeOther)
}

// teamHierarchyBreadcrumbs returns "Leagues / {League} / {Team} / Roster"
// plus any trailing crumbs (e.g. "Add Player"). With no extra crumbs, Roster
// is left unlinked as the current page; otherwise it links back to the team
// page, which now shows the roster inline.
func (app *application) teamHierarchyBreadcrumbs(w http.ResponseWriter, team *models.Team, extra ...Breadcrumb) ([]Breadcrumb, bool) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(team.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return nil, false
	}
	rosterURL := ""
	if len(extra) > 0 {
		rosterURL = fmt.Sprintf("/team/%d", team.ID)
	}
	crumbs := append(app.teamBreadcrumbs(team, league, false), Breadcrumb{Label: "Roster", URL: rosterURL})
	return append(crumbs, extra...), true
}

// playerProfile combines a player with its linked address (if any) for the
// read-only view page. Player's fields are promoted, so templates can
// reference them directly (e.g. .FirstName) alongside .Address. CanManage
// says whether the viewer may edit this profile (see canManagePlayer) —
// deletion is a separate, admin-only action shown independently.
type playerProfile struct {
	*models.Player
	Address   *models.Address
	CanManage bool
	IsSelf    bool
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

// playerBreadcrumbTeam loads the team and league behind a player's
// breadcrumb trail. Returns nil, nil, nil when the player belongs to zero or
// more than one team — a flat profile page has no single unambiguous parent
// to show a hierarchical trail through in those cases, so callers fall back
// to a flatter breadcrumb (or none at all).
func (app *application) playerBreadcrumbTeam(player *models.Player) (*models.Team, *models.League, error) {
	tmm := &models.TeamMemberModel{DB: app.playerService.DB}
	teams, err := tmm.GetTeamsForPlayer(player.ID)
	if err != nil {
		return nil, nil, err
	}
	if len(teams) != 1 {
		return nil, nil, nil
	}

	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(teams[0].LeagueID)
	if err != nil {
		return nil, nil, err
	}

	return teams[0], league, nil
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

	team, league, err := app.playerBreadcrumbTeam(player)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = &playerProfile{Player: player, Address: address, CanManage: app.canManagePlayer(r, player), IsSelf: app.getPlayerID(r) == player.ID}
	if team != nil {
		data.Breadcrumbs = append(app.teamBreadcrumbs(team, league, false),
			Breadcrumb{Label: "Roster", URL: fmt.Sprintf("/team/%d", team.ID)},
			Breadcrumb{Label: player.FirstName + " " + player.LastName},
		)
	}

	app.render(w, http.StatusOK, "player-view.html", data)
}

// canManagePlayer reports whether the current request's user may edit this
// player's profile: an Admin, the player themself, or a captain/league
// admin of any team the player is also a member of (a player can belong to
// several teams, so this checks for overlap rather than a single shared
// team).
func (app *application) canManagePlayer(r *http.Request, player *models.Player) bool {
	if app.isAdmin(r) {
		return true
	}
	if app.getPlayerID(r) == player.ID {
		return true
	}

	tmm := &models.TeamMemberModel{DB: app.playerService.DB}
	teams, err := tmm.GetTeamsForPlayer(player.ID)
	if err != nil {
		return false
	}
	for _, team := range teams {
		if app.canManageTeam(r, team.ID) {
			return true
		}
	}
	return false
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

	team, league, err := app.playerBreadcrumbTeam(player)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.SupportData = models.USStates
	playerLabel := player.FirstName + " " + player.LastName
	if team != nil {
		data.Breadcrumbs = append(app.teamBreadcrumbs(team, league, false),
			Breadcrumb{Label: "Roster", URL: fmt.Sprintf("/team/%d", team.ID)},
			Breadcrumb{Label: playerLabel, URL: fmt.Sprintf("/player/view/%d", player.ID)},
			Breadcrumb{Label: "Edit"},
		)
	} else {
		data.Breadcrumbs = []Breadcrumb{
			{Label: playerLabel, URL: fmt.Sprintf("/player/view/%d", player.ID)},
			{Label: "Edit"},
		}
	}

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

// playerDelete wipes a player's bio, address, and every team membership —
// the full destructive action. Admin-only, lives on the flat profile page
// (not per-team) since it's no longer a per-roster action.
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

// playerRemoveFromTeam drops a player from just this team's roster — the
// lightweight, captain-usable action. Their bio, address, and any other
// team memberships are untouched.
func (app *application) playerRemoveFromTeam(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	params := httprouter.ParamsFromContext(r.Context())
	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	err = app.playerService.RemoveFromRoster(id, team.ID, app.getUserName(r))
	if err != nil {
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

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

// teamViewData is the shape rendered by team-view.html: the team, its
// league, the captain's name (blank if unassigned), and whether the current
// user is allowed to request to join (active, has a player, currently on no
// team at all).
type teamViewData struct {
	Team             *models.Team
	League           *models.League
	CaptainName      string
	CanRequestToJoin bool
	Roster           []*models.Player
}

func (app *application) teamView(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(team.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	captainName := ""
	if team.CaptainPlayerID.Valid {
		pm := &models.PlayerModel{DB: app.playerService.DB}
		captain, err := pm.Get(int(team.CaptainPlayerID.Int32))
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			app.serverError(w, err)
			return
		}
		if captain != nil {
			captainName = captain.FirstName + " " + captain.LastName
		}
	}

	canRequestToJoin := app.isActive(r) && app.getPlayerID(r) > 0 && app.getTeamID(r) == 0

	var roster []*models.Player
	if app.isAdmin(r) {
		roster, err = app.playerService.GetByTeam(team.ID)
		if err != nil {
			app.serverError(w, err)
			return
		}
	}

	data := app.newTemplateData(r)
	data.Data = &teamViewData{
		Team:             team,
		League:           league,
		CaptainName:      captainName,
		CanRequestToJoin: canRequestToJoin,
		Roster:           roster,
	}

	app.render(w, http.StatusOK, "team-view.html", data)
}

func (app *application) teamForm(w http.ResponseWriter, r *http.Request) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	leagues, err := lm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = services.TeamForm{}
	data.SupportData = leagues

	app.render(w, http.StatusOK, "team-create.html", data)
}

func (app *application) teamCreate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	leagueID, _ := strconv.Atoi(r.PostForm.Get("leagueID"))
	form := &services.TeamForm{
		LeagueID: leagueID,
		Name:     r.PostForm.Get("name"),
	}

	id, err := app.teamService.CreateTeam(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) || errors.Is(err, models.ErrNoRecord) {
			lm := &models.LeagueModel{DB: app.playerService.DB}
			leagues, lerr := lm.List()
			if lerr != nil {
				app.serverError(w, lerr)
				return
			}
			data := app.newTemplateData(r)
			data.Form = form
			data.SupportData = leagues
			app.render(w, http.StatusUnprocessableEntity, "team-create.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been created!")
	http.Redirect(w, r, fmt.Sprintf("/team/%d", id), http.StatusSeeOther)
}

func (app *application) teamUpdate(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	tm := &models.TeamModel{DB: app.playerService.DB}
	team, err := tm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	lm := &models.LeagueModel{DB: app.playerService.DB}
	leagues, err := lm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = &services.TeamForm{ID: team.ID, LeagueID: team.LeagueID, Name: team.Name}
	data.SupportData = leagues

	app.render(w, http.StatusOK, "team-update.html", data)
}

func (app *application) teamUpdatePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PostForm.Get("team-id"))
	if err != nil || id < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	leagueID, _ := strconv.Atoi(r.PostForm.Get("leagueID"))
	form := &services.TeamForm{
		ID:       id,
		LeagueID: leagueID,
		Name:     r.PostForm.Get("name"),
	}

	err = app.teamService.UpdateTeam(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) || errors.Is(err, models.ErrNoRecord) {
			lm := &models.LeagueModel{DB: app.playerService.DB}
			leagues, lerr := lm.List()
			if lerr != nil {
				app.serverError(w, lerr)
				return
			}
			data := app.newTemplateData(r)
			data.Form = form
			data.SupportData = leagues
			app.render(w, http.StatusUnprocessableEntity, "team-update.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been updated!")
	http.Redirect(w, r, fmt.Sprintf("/team/%d", form.ID), http.StatusSeeOther)
}

func (app *application) teamDelete(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	err = app.teamService.DeleteTeam(id, app.getUserName(r))
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

// teamSetCaptain is a plain form POST (teamID, playerID — 0 clears) rather
// than the JSON+fetch toggle pattern used elsewhere, since it's a one-off
// admin action, not a per-row checkbox.
func (app *application) teamSetCaptain(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	teamID, err := strconv.Atoi(r.PostForm.Get("teamID"))
	if err != nil || teamID < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	playerID, _ := strconv.Atoi(r.PostForm.Get("playerID"))

	err = app.teamService.SetCaptain(teamID, playerID, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			app.sessionManager.Put(r.Context(), "flash", "That player isn't on this team's roster.")
			http.Redirect(w, r, fmt.Sprintf("/team/%d", teamID), http.StatusSeeOther)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Team captain updated.")
	http.Redirect(w, r, fmt.Sprintf("/team/%d", teamID), http.StatusSeeOther)
}

func (app *application) joinRequestSubmit(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	playerID := app.getPlayerID(r)
	if playerID < 1 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	err := app.joinRequestService.RequestToJoin(playerID, team.ID, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrDuplicateRequest) {
			app.sessionManager.Put(r.Context(), "flash", "You already have a pending join request.")
		} else if errors.Is(err, models.ErrBadData) {
			app.sessionManager.Put(r.Context(), "flash", "You're already on a team.")
		} else {
			app.serverError(w, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/team/%d", team.ID), http.StatusSeeOther)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Your request to join "+team.Name+" has been submitted.")
	http.Redirect(w, r, fmt.Sprintf("/team/%d", team.ID), http.StatusSeeOther)
}

func (app *application) teamInviteForm(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	data := app.newTemplateData(r)
	data.Data = team
	data.Form = services.InviteForm{}

	app.render(w, http.StatusOK, "team-invite.html", data)
}

func (app *application) teamInviteSend(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := &services.InviteForm{
		Emails: r.PostForm.Get("emails"),
	}

	loggedInUserID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")

	invited, err := app.inviteService.SendInvites(team.ID, loggedInUserID, app.getUserName(r), form)
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Data = team
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "team-invite.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", fmt.Sprintf("Invited %d player(s) to %s.", len(invited), team.Name))
	http.Redirect(w, r, fmt.Sprintf("/team/%d", team.ID), http.StatusSeeOther)
}

// joinRequestListData wraps join-request rows alongside the team they're
// scoped to (nil for the cross-team admin view, whose template shows a Team
// column instead of a page-level team heading).
type joinRequestListData struct {
	Team     *models.Team
	Requests []*models.JoinRequestListItem
}

func (app *application) joinRequestList(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	requests, err := app.joinRequestService.ListPendingByTeam(team.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = &joinRequestListData{Team: team, Requests: requests}

	app.render(w, http.StatusOK, "team-join-requests.html", data)
}

func (app *application) adminJoinRequestList(w http.ResponseWriter, r *http.Request) {
	requests, err := app.joinRequestService.ListPendingAll()
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = &joinRequestListData{Team: nil, Requests: requests}

	app.render(w, http.StatusOK, "team-join-requests.html", data)
}

func (app *application) getRouteJoinRequest(w http.ResponseWriter, r *http.Request) (*models.TeamJoinRequest, int, bool) {
	params := httprouter.ParamsFromContext(r.Context())

	teamID, err := strconv.Atoi(params.ByName("teamID"))
	if err != nil || teamID < 1 {
		app.notFound(w)
		return nil, 0, false
	}

	requestID, err := strconv.Atoi(params.ByName("requestID"))
	if err != nil || requestID < 1 {
		app.notFound(w)
		return nil, 0, false
	}

	jr, err := app.joinRequestService.Get(requestID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return nil, 0, false
	}

	// Defense in depth: a captain guessing another team's request id should
	// still 404, not act on it, even though requireTeamManager already
	// confirmed they manage :teamID.
	if jr.TeamID != teamID {
		app.notFound(w)
		return nil, 0, false
	}

	return jr, teamID, true
}

func (app *application) joinRequestApprove(w http.ResponseWriter, r *http.Request) {
	jr, teamID, ok := app.getRouteJoinRequest(w, r)
	if !ok {
		return
	}

	loggedInUserID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")

	err := app.joinRequestService.Approve(jr.ID, loggedInUserID, app.getUserName(r))
	if err != nil {
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Join request approved.")
	http.Redirect(w, r, fmt.Sprintf("/team/%d/joinRequests", teamID), http.StatusSeeOther)
}

func (app *application) joinRequestReject(w http.ResponseWriter, r *http.Request) {
	jr, teamID, ok := app.getRouteJoinRequest(w, r)
	if !ok {
		return
	}

	loggedInUserID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")

	err := app.joinRequestService.Reject(jr.ID, loggedInUserID, app.getUserName(r))
	if err != nil {
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Join request rejected.")
	http.Redirect(w, r, fmt.Sprintf("/team/%d/joinRequests", teamID), http.StatusSeeOther)
}

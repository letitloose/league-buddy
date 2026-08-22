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

// teamBreadcrumbs returns the "Leagues / {League} / {Team}" trail shared by
// every page scoped under a team. Pass teamIsCurrent true when the team
// itself is the page being rendered, so its crumb is left unlinked.
func (app *application) teamBreadcrumbs(team *models.Team, league *models.League, teamIsCurrent bool) []Breadcrumb {
	teamURL := fmt.Sprintf("/team/%d", team.ID)
	if teamIsCurrent {
		teamURL = ""
	}
	return []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)},
		{Label: team.Name, URL: teamURL},
	}
}

// teamViewData is the shape rendered by team-view.html: the team, its
// league, the captain's name (blank if unassigned), whether the current
// user may manage this team (admin or its captain), and whether they're
// allowed to request to join (active, has a player, not already on a team
// in this league). PendingInviteCount/PendingJoinRequestCount power the
// small badge counts on the Invite Players/Join Requests buttons — both are
// left at zero (and unqueried) for viewers who can't manage the team.
type teamViewData struct {
	Team                    *models.Team
	League                  *models.League
	CaptainName             string
	CanManage               bool
	CanDelete               bool
	CanRequestToJoin        bool
	Roster                  []*models.Player
	PendingInviteCount      int
	PendingJoinRequestCount int
	Location                *models.Location
	LocationAddress         *models.Address
	CurrentSeason           *models.Season
	Leaders                 []*models.StatLine
	LeadersByPlayer         map[int]*models.StatLine
	LeadingScorer           *models.StatLine
	LeadingAssister         *models.StatLine
	Schedule                []*seasonMatchRow
}

// teamFormSupportData is the SupportData shape for team-create.html and
// team-update.html: the leagues a manager may pick from (already filtered
// for non-admins) and every location, for the optional home-field dropdown.
type teamFormSupportData struct {
	Leagues   []*models.League
	Locations []*models.Location
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

	canManage := app.canManageTeam(r, team.ID)
	canDelete := app.canDeleteTeam(r, team.ID)

	canRequestToJoin := false
	if app.isActive(r) && app.getPlayerID(r) > 0 {
		tmm := &models.TeamMemberModel{DB: app.playerService.DB}
		isMember, err := tmm.IsMember(app.getPlayerID(r), team.ID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		hasTeamInLeague, err := tmm.HasTeamInLeague(app.getPlayerID(r), team.LeagueID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		canRequestToJoin = !isMember && !hasTeamInLeague
	}

	// The roster is shown inline on the team page to every active viewer,
	// not just managers.
	roster, err := app.playerService.GetByTeam(team.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	var location *models.Location
	var locationAddress *models.Address
	if team.LocationID.Valid {
		locm := &models.LocationModel{DB: app.playerService.DB}
		location, err = locm.Get(int(team.LocationID.Int32))
		if err != nil {
			app.serverError(w, err)
			return
		}
		am := &models.AddressModel{DB: app.playerService.DB}
		locationAddress, err = am.Get(location.AddressID)
		if err != nil {
			app.serverError(w, err)
			return
		}
	}

	pendingInviteCount := 0
	pendingJoinRequestCount := 0
	if canManage {
		im := &models.InviteModel{DB: app.playerService.DB}
		pendingInvites, err := im.ListPendingByTeam(team.ID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		pendingInviteCount = len(pendingInvites)

		pendingRequests, err := app.joinRequestService.ListPendingByTeam(team.ID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		pendingJoinRequestCount = len(pendingRequests)
	}

	// A fresh league with no results yet renders exactly as before — the
	// roster table's stat columns and the schedule are only shown when
	// CurrentSeason is set. Same "most recent season with results" pick the
	// league page's standings use (SeasonModel.GetMostRecentWithResults),
	// rather than the date-based GetCurrent, so the whole app defaults to
	// one consistent notion of "the season we're showing right now".
	var currentSeason *models.Season
	var leaders []*models.StatLine
	var schedule []*seasonMatchRow
	leadersByPlayer := map[int]*models.StatLine{}
	sm := &models.SeasonModel{DB: app.playerService.DB}
	currentSeason, err = sm.GetMostRecentWithResults(team.LeagueID)
	if err != nil && !errors.Is(err, models.ErrNoRecord) {
		app.serverError(w, err)
		return
	}
	if currentSeason != nil {
		pmsm := &models.PlayerMatchStatModel{DB: app.playerService.DB}
		leaders, err = pmsm.LeaderboardByTeamSeason(team.ID, currentSeason.ID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		for _, line := range leaders {
			leadersByPlayer[line.PlayerID] = line
		}

		mm := &models.MatchModel{DB: app.playerService.DB}
		matches, err := mm.GetByTeamAndSeason(team.ID, currentSeason.ID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		schedule, err = app.buildSeasonMatchRows(matches)
		if err != nil {
			app.serverError(w, err)
			return
		}
	}

	var leadingScorer, leadingAssister *models.StatLine
	for _, line := range leaders {
		if line.Goals > 0 && (leadingScorer == nil || line.Goals > leadingScorer.Goals) {
			leadingScorer = line
		}
		if line.Assists > 0 && (leadingAssister == nil || line.Assists > leadingAssister.Assists) {
			leadingAssister = line
		}
	}

	data := app.newTemplateData(r)
	data.Data = &teamViewData{
		Team:                    team,
		League:                  league,
		CaptainName:             captainName,
		CanManage:               canManage,
		CanDelete:               canDelete,
		CanRequestToJoin:        canRequestToJoin,
		Roster:                  roster,
		PendingInviteCount:      pendingInviteCount,
		PendingJoinRequestCount: pendingJoinRequestCount,
		Location:                location,
		LocationAddress:         locationAddress,
		CurrentSeason:           currentSeason,
		Leaders:                 leaders,
		LeadersByPlayer:         leadersByPlayer,
		LeadingScorer:           leadingScorer,
		LeadingAssister:         leadingAssister,
		Schedule:                schedule,
	}
	data.Breadcrumbs = app.teamBreadcrumbs(team, league, true)

	app.render(w, http.StatusOK, "team-view.html", data)
}

// teamFormBreadcrumbs is the shared "Leagues / Add Team" trail for the
// create form and its validation-error re-render.
func teamFormBreadcrumbs() []Breadcrumb {
	return []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: "Add Team"},
	}
}

// filterManageableLeagues returns leagues unchanged for admins, or narrowed
// down to just the leagues the current request's user administers otherwise
// — used so the team-create league dropdown never offers a league admin a
// league they don't actually manage.
func (app *application) filterManageableLeagues(r *http.Request, leagues []*models.League) []*models.League {
	if app.isAdmin(r) {
		return leagues
	}

	manageable := []*models.League{}
	for _, league := range leagues {
		if app.isLeagueAdminOfLeague(r, league.ID) {
			manageable = append(manageable, league)
		}
	}
	return manageable
}

// renderTeamCreateForm re-derives everything team-create.html needs
// (leagues filtered to what this viewer can manage, every location) and
// renders it — shared by the initial GET and every validation-failure
// re-render (bad team fields, or a rejected inline new-location).
func (app *application) renderTeamCreateForm(w http.ResponseWriter, r *http.Request, form *services.TeamForm, status int) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	leagues, err := lm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	locm := &models.LocationModel{DB: app.playerService.DB}
	locations, err := locm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.SupportData = &teamFormSupportData{Leagues: app.filterManageableLeagues(r, leagues), Locations: locations}
	data.Breadcrumbs = teamFormBreadcrumbs()
	app.render(w, status, "team-create.html", data)
}

// resolveTeamLocationID figures out which locationID a team create/update
// submission should use. The inline "add a new home field" fields take
// priority over the <select> dropdown when any of them are filled in —
// LocationService.CreateLocation either creates a new location or
// transparently reuses an existing one at the same address, so a team
// manager typing in a field that already exists just gets associated with
// it instead of creating a duplicate. Returns ok=false (having already
// attached a form error) when the new-location fields are only partially
// filled in, or fail their own validation.
func (app *application) resolveTeamLocationID(r *http.Request, form *services.TeamForm, actorEmail string) (int, bool) {
	form.NewLocationName = r.PostForm.Get("newlocationname")
	form.NewLocationAddress1 = r.PostForm.Get("newlocationaddress1")
	form.NewLocationAddress2 = r.PostForm.Get("newlocationaddress2")
	form.NewLocationCity = r.PostForm.Get("newlocationcity")
	form.NewLocationStateProvince = r.PostForm.Get("newlocationstateprovince")
	form.NewLocationZipCode = r.PostForm.Get("newlocationzipcode")

	if form.NewLocationName == "" && form.NewLocationAddress1 == "" && form.NewLocationCity == "" {
		locationID, _ := strconv.Atoi(r.PostForm.Get("locationID"))
		return locationID, true
	}

	if form.NewLocationName == "" || form.NewLocationAddress1 == "" || form.NewLocationCity == "" {
		form.AddNonFieldError("To add a new home field, enter at least a name, address, and city.")
		return 0, false
	}

	locationID, err := app.locationService.CreateLocation(&services.LocationForm{
		Name:          form.NewLocationName,
		Address1:      form.NewLocationAddress1,
		Address2:      form.NewLocationAddress2,
		City:          form.NewLocationCity,
		StateProvince: form.NewLocationStateProvince,
		ZipCode:       form.NewLocationZipCode,
	}, actorEmail)
	if err != nil {
		form.AddNonFieldError("The new home field's address couldn't be saved — check it and try again.")
		return 0, false
	}

	return locationID, true
}

func (app *application) teamForm(w http.ResponseWriter, r *http.Request) {
	if !app.isAdmin(r) && len(app.getLeagueAdminLeagueIDs(r)) == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	app.renderTeamCreateForm(w, r, &services.TeamForm{}, http.StatusOK)
}

func (app *application) teamCreate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	leagueID, _ := strconv.Atoi(r.PostForm.Get("leagueID"))

	if !app.canManageLeague(r, leagueID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	form := &services.TeamForm{
		LeagueID:        leagueID,
		Name:            r.PostForm.Get("name"),
		Motto:           r.PostForm.Get("motto"),
		EstablishedDate: r.PostForm.Get("establisheddate"),
	}

	locationID, ok := app.resolveTeamLocationID(r, form, app.getUserName(r))
	if !ok {
		app.renderTeamCreateForm(w, r, form, http.StatusUnprocessableEntity)
		return
	}
	form.LocationID = locationID

	id, err := app.teamService.CreateTeam(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) || errors.Is(err, models.ErrNoRecord) {
			app.renderTeamCreateForm(w, r, form, http.StatusUnprocessableEntity)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been created!")
	http.Redirect(w, r, fmt.Sprintf("/team/%d", id), http.StatusSeeOther)
}

// teamUpdateData wraps the team (for its current CaptainPlayerID, to
// preselect the Captain <select>) alongside its roster (the options for
// that <select>).
type teamUpdateData struct {
	Team   *models.Team
	Roster []*models.Player
}

// renderTeamUpdateForm re-derives everything team-update.html needs (the
// team + its roster, every league, every location) and renders it — shared
// by the initial GET and every validation-failure re-render (bad team
// fields, a rejected inline new-location, or an unassignable captain).
// The leagueID <select> stays populated with every league (not just the
// leagues this viewer administers) so a captain — who can edit their team's
// info but not reassign it to another league — still sees a dropdown
// containing their own team's current league. Reassignment to a league they
// don't manage is rejected server-side in teamUpdatePost. The template only
// renders this <select> at all for admins — everyone else gets a hidden
// input carrying the unchanged value instead.
func (app *application) renderTeamUpdateForm(w http.ResponseWriter, r *http.Request, form *services.TeamForm, team *models.Team, status int) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	leagues, err := lm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	locm := &models.LocationModel{DB: app.playerService.DB}
	locations, err := locm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	roster, err := app.playerService.GetByTeam(team.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.Data = &teamUpdateData{Team: team, Roster: roster}
	data.SupportData = &teamFormSupportData{Leagues: leagues, Locations: locations}

	breadcrumbs := []Breadcrumb{{Label: "Leagues", URL: "/league"}}
	for _, league := range leagues {
		if league.ID == form.LeagueID {
			breadcrumbs = append(breadcrumbs, Breadcrumb{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)})
			break
		}
	}
	breadcrumbs = append(breadcrumbs,
		Breadcrumb{Label: form.Name, URL: fmt.Sprintf("/team/%d", form.ID)},
		Breadcrumb{Label: "Edit"},
	)
	data.Breadcrumbs = breadcrumbs

	app.render(w, status, "team-update.html", data)
}

func (app *application) teamUpdate(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("teamID"))
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

	form := &services.TeamForm{ID: team.ID, LeagueID: team.LeagueID, Name: team.Name, Motto: team.Motto.String, LocationID: int(team.LocationID.Int32)}
	if team.EstablishedDate.Valid {
		form.EstablishedDate = pickerDate(team.EstablishedDate.Time)
	}

	app.renderTeamUpdateForm(w, r, form, team, http.StatusOK)
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

	if !app.canManageTeam(r, id) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	leagueID, _ := strconv.Atoi(r.PostForm.Get("leagueID"))

	// Reassigning a team to a different league is beyond what a plain
	// captain or a league admin of only the team's current league can do —
	// clamp back to the team's existing league rather than rejecting the
	// whole edit, so captains editing motto/name aren't blocked by this.
	if !app.canManageLeague(r, leagueID) {
		tm := &models.TeamModel{DB: app.playerService.DB}
		existing, terr := tm.Get(id)
		if terr != nil {
			if errors.Is(terr, models.ErrNoRecord) {
				app.notFound(w)
			} else {
				app.serverError(w, terr)
			}
			return
		}
		leagueID = existing.LeagueID
	}

	form := &services.TeamForm{
		ID:              id,
		LeagueID:        leagueID,
		Name:            r.PostForm.Get("name"),
		Motto:           r.PostForm.Get("motto"),
		EstablishedDate: r.PostForm.Get("establisheddate"),
	}

	// getRouteTeamByID re-fetches the team fresh (needed both to build the
	// error re-render and because the row must still exist).
	getRouteTeamByID := func() (*models.Team, bool) {
		tm := &models.TeamModel{DB: app.playerService.DB}
		team, terr := tm.Get(id)
		if terr != nil {
			if errors.Is(terr, models.ErrNoRecord) {
				app.notFound(w)
			} else {
				app.serverError(w, terr)
			}
			return nil, false
		}
		return team, true
	}

	locationID, ok := app.resolveTeamLocationID(r, form, app.getUserName(r))
	if !ok {
		team, ok := getRouteTeamByID()
		if !ok {
			return
		}
		app.renderTeamUpdateForm(w, r, form, team, http.StatusUnprocessableEntity)
		return
	}
	form.LocationID = locationID

	err = app.teamService.UpdateTeam(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) || errors.Is(err, models.ErrNoRecord) {
			team, ok := getRouteTeamByID()
			if !ok {
				return
			}
			app.renderTeamUpdateForm(w, r, form, team, http.StatusUnprocessableEntity)
			return
		}
		app.serverError(w, err)
		return
	}

	// Captain reassignment rides along with the team-info edit now, rather
	// than a separate form. The <select> always posts a value ("0" for
	// "-- None --", or a player ID) — captainPlayerID is only absent when a
	// caller other than team-update.html's form posts here without it, in
	// which case the captain is left untouched.
	if captainStr := r.PostForm.Get("captainPlayerID"); captainStr != "" {
		captainPlayerID, _ := strconv.Atoi(captainStr)
		if err := app.teamService.SetCaptain(id, captainPlayerID, app.getUserName(r)); err != nil {
			if errors.Is(err, models.ErrBadData) {
				app.sessionManager.Put(r.Context(), "flash", form.Name+" updated, but that player isn't on the roster — captain unchanged.")
				http.Redirect(w, r, fmt.Sprintf("/team/%d", form.ID), http.StatusSeeOther)
				return
			}
			app.serverError(w, err)
			return
		}
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been updated!")
	http.Redirect(w, r, fmt.Sprintf("/team/%d", form.ID), http.StatusSeeOther)
}

func (app *application) teamDelete(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("teamID"))
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
// admin/captain/league-admin action, not a per-row checkbox.
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

	if !app.canManageTeam(r, teamID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
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

// teamActionBreadcrumbs returns "Leagues / {League} / {Team}" plus trailing
// crumbs for team-manager pages that sit directly under the team, not the
// roster (invite, join requests).
func (app *application) teamActionBreadcrumbs(w http.ResponseWriter, team *models.Team, extra ...Breadcrumb) ([]Breadcrumb, bool) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(team.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return nil, false
	}
	return append(app.teamBreadcrumbs(team, league, false), extra...), true
}

// teamInviteData wraps a team alongside its outstanding (not-yet-accepted)
// invites, shown below the send form so captains/admins can see who's
// already been invited.
type teamInviteData struct {
	Team           *models.Team
	PendingInvites []*models.Invite
}

func (app *application) teamInviteForm(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	im := &models.InviteModel{DB: app.playerService.DB}
	pending, err := im.ListPendingByTeam(team.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	breadcrumbs, ok := app.teamActionBreadcrumbs(w, team, Breadcrumb{Label: "Invite Players"})
	if !ok {
		return
	}

	data := app.newTemplateData(r)
	data.Data = &teamInviteData{Team: team, PendingInvites: pending}
	data.Form = services.InviteForm{}
	data.Breadcrumbs = breadcrumbs

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
			im := &models.InviteModel{DB: app.playerService.DB}
			pending, perr := im.ListPendingByTeam(team.ID)
			if perr != nil {
				app.serverError(w, perr)
				return
			}
			breadcrumbs, ok := app.teamActionBreadcrumbs(w, team, Breadcrumb{Label: "Invite Players"})
			if !ok {
				return
			}
			data := app.newTemplateData(r)
			data.Data = &teamInviteData{Team: team, PendingInvites: pending}
			data.Form = form
			data.Breadcrumbs = breadcrumbs
			app.render(w, http.StatusUnprocessableEntity, "team-invite.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", fmt.Sprintf("Invited %d player(s) to %s.", len(invited), team.Name))
	http.Redirect(w, r, fmt.Sprintf("/team/%d", team.ID), http.StatusSeeOther)
}

func (app *application) inviteCancel(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	teamID, err := strconv.Atoi(params.ByName("teamID"))
	if err != nil || teamID < 1 {
		app.notFound(w)
		return
	}
	inviteID, err := strconv.Atoi(params.ByName("inviteID"))
	if err != nil || inviteID < 1 {
		app.notFound(w)
		return
	}

	im := &models.InviteModel{DB: app.playerService.DB}
	invite, err := im.Get(inviteID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}
	// Defense in depth: a captain guessing another team's invite id should
	// still 404, not act on it, even though requireTeamManager already
	// confirmed they manage :teamID.
	if invite.TeamID != teamID {
		app.notFound(w)
		return
	}

	if err := app.inviteService.CancelInvite(inviteID, app.getUserName(r)); err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.clientError(w, http.StatusConflict)
			return
		}
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
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

	breadcrumbs, ok := app.teamActionBreadcrumbs(w, team, Breadcrumb{Label: "Join Requests"})
	if !ok {
		return
	}

	data := app.newTemplateData(r)
	data.Data = &joinRequestListData{Team: team, Requests: requests}
	data.Breadcrumbs = breadcrumbs

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

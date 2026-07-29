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

// seasonMatchRow pairs a match with its home/away team names and location —
// everything season-view.html needs per row without a template-side lookup.
// LocationAddress is loaded alongside Location since mapsURL takes an
// Address, not a Location (see team-view.html's Location/LocationAddress
// pair for the same reason).
type seasonMatchRow struct {
	Match           *models.Match
	HomeTeamName    string
	AwayTeamName    string
	Location        *models.Location
	LocationAddress *models.Address
}

type seasonViewData struct {
	Season        *models.Season
	League        *models.League
	Matches       []*seasonMatchRow
	CanManage     bool
	GoalLeaders   []*models.LeagueLeaderLine
	AssistLeaders []*models.LeagueLeaderLine
}

func (app *application) buildSeasonMatchRows(matches []*models.Match) ([]*seasonMatchRow, error) {
	tm := &models.TeamModel{DB: app.playerService.DB}
	locm := &models.LocationModel{DB: app.playerService.DB}
	am := &models.AddressModel{DB: app.playerService.DB}

	rows := make([]*seasonMatchRow, 0, len(matches))
	for _, match := range matches {
		homeTeam, err := tm.Get(match.HomeTeamID)
		if err != nil {
			return nil, err
		}
		awayTeam, err := tm.Get(match.AwayTeamID)
		if err != nil {
			return nil, err
		}

		var location *models.Location
		var locationAddress *models.Address
		if match.LocationID.Valid {
			location, err = locm.Get(int(match.LocationID.Int32))
			if err != nil {
				return nil, err
			}
			locationAddress, err = am.Get(location.AddressID)
			if err != nil {
				return nil, err
			}
		}

		rows = append(rows, &seasonMatchRow{
			Match:           match,
			HomeTeamName:    homeTeam.Name,
			AwayTeamName:    awayTeam.Name,
			Location:        location,
			LocationAddress: locationAddress,
		})
	}
	return rows, nil
}

func (app *application) seasonView(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	sm := &models.SeasonModel{DB: app.playerService.DB}
	season, err := sm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(season.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	mm := &models.MatchModel{DB: app.playerService.DB}
	matches, err := mm.GetBySeason(id)
	if err != nil {
		app.serverError(w, err)
		return
	}
	rows, err := app.buildSeasonMatchRows(matches)
	if err != nil {
		app.serverError(w, err)
		return
	}

	pmsm := &models.PlayerMatchStatModel{DB: app.playerService.DB}
	goalLeaders, err := pmsm.TopScorersForSeason(id, 5)
	if err != nil {
		app.serverError(w, err)
		return
	}
	assistLeaders, err := pmsm.TopAssistersForSeason(id, 5)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = &seasonViewData{
		Season:        season,
		League:        league,
		Matches:       rows,
		CanManage:     app.canManageLeague(r, season.LeagueID),
		GoalLeaders:   goalLeaders,
		AssistLeaders: assistLeaders,
	}
	data.Breadcrumbs = []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)},
		{Label: season.Name},
	}

	app.render(w, http.StatusOK, "season-view.html", data)
}

// seasonFormSupportData is the SupportData shape for season-create.html and
// season-update.html: the leagues a manager may pick from (already filtered
// for non-admins, same as teamFormSupportData.Leagues).
type seasonFormSupportData struct {
	Leagues []*models.League
}

func seasonFormBreadcrumbs() []Breadcrumb {
	return []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: "Add Season"},
	}
}

func (app *application) renderSeasonCreateForm(w http.ResponseWriter, r *http.Request, form *services.SeasonForm, status int) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	leagues, err := lm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.SupportData = &seasonFormSupportData{Leagues: app.filterManageableLeagues(r, leagues)}
	data.Breadcrumbs = seasonFormBreadcrumbs()
	app.render(w, status, "season-create.html", data)
}

func (app *application) seasonForm(w http.ResponseWriter, r *http.Request) {
	if !app.isAdmin(r) && len(app.getLeagueAdminLeagueIDs(r)) == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	leagueID, _ := strconv.Atoi(r.URL.Query().Get("leagueID"))
	app.renderSeasonCreateForm(w, r, &services.SeasonForm{LeagueID: leagueID}, http.StatusOK)
}

func (app *application) seasonCreate(w http.ResponseWriter, r *http.Request) {
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

	form := &services.SeasonForm{
		LeagueID:  leagueID,
		Name:      r.PostForm.Get("name"),
		StartDate: r.PostForm.Get("startdate"),
		EndDate:   r.PostForm.Get("enddate"),
	}

	id, err := app.seasonService.CreateSeason(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) || errors.Is(err, models.ErrNoRecord) {
			app.renderSeasonCreateForm(w, r, form, http.StatusUnprocessableEntity)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been created!")
	http.Redirect(w, r, fmt.Sprintf("/season/%d", id), http.StatusSeeOther)
}

func (app *application) renderSeasonUpdateForm(w http.ResponseWriter, r *http.Request, form *services.SeasonForm, status int) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	leagues, err := lm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.SupportData = &seasonFormSupportData{Leagues: leagues}

	breadcrumbs := []Breadcrumb{{Label: "Leagues", URL: "/league"}}
	for _, league := range leagues {
		if league.ID == form.LeagueID {
			breadcrumbs = append(breadcrumbs, Breadcrumb{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)})
			break
		}
	}
	breadcrumbs = append(breadcrumbs,
		Breadcrumb{Label: form.Name, URL: fmt.Sprintf("/season/%d", form.ID)},
		Breadcrumb{Label: "Edit"},
	)
	data.Breadcrumbs = breadcrumbs

	app.render(w, status, "season-update.html", data)
}

func (app *application) seasonUpdate(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	sm := &models.SeasonModel{DB: app.playerService.DB}
	season, err := sm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	if !app.canManageLeague(r, season.LeagueID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	form := &services.SeasonForm{ID: season.ID, LeagueID: season.LeagueID, Name: season.Name}
	if season.StartDate.Valid {
		form.StartDate = pickerDate(season.StartDate.Time)
	}
	if season.EndDate.Valid {
		form.EndDate = pickerDate(season.EndDate.Time)
	}

	app.renderSeasonUpdateForm(w, r, form, http.StatusOK)
}

func (app *application) seasonUpdatePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PostForm.Get("season-id"))
	if err != nil || id < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	sm := &models.SeasonModel{DB: app.playerService.DB}
	existing, err := sm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	if !app.canManageLeague(r, existing.LeagueID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	leagueID, _ := strconv.Atoi(r.PostForm.Get("leagueID"))
	// Reassigning a season to a different league is admin-only (mirrors
	// team reassignment in teamUpdatePost) — a league admin editing dates
	// isn't blocked by this, since the field is only ever posted by admins
	// in the first place (season-update.html only shows the league <select>
	// to admins; everyone else posts a hidden field carrying the unchanged
	// value).
	if !app.canManageLeague(r, leagueID) {
		leagueID = existing.LeagueID
	}

	form := &services.SeasonForm{
		ID:        id,
		LeagueID:  leagueID,
		Name:      r.PostForm.Get("name"),
		StartDate: r.PostForm.Get("startdate"),
		EndDate:   r.PostForm.Get("enddate"),
	}

	err = app.seasonService.UpdateSeason(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) || errors.Is(err, models.ErrNoRecord) {
			app.renderSeasonUpdateForm(w, r, form, http.StatusUnprocessableEntity)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been updated!")
	http.Redirect(w, r, fmt.Sprintf("/season/%d", form.ID), http.StatusSeeOther)
}

// teamSeasonMatchRow is one row of a team's own schedule for a season —
// just the opponent and whether this team was home or away, rather than
// seasonMatchRow's home/away team names (which read oddly once you already
// know which team's page you're on).
type teamSeasonMatchRow struct {
	Match           *models.Match
	Opponent        string
	IsHome          bool
	Location        *models.Location
	LocationAddress *models.Address
}

type teamSeasonViewData struct {
	Team    *models.Team
	League  *models.League
	Season  *models.Season
	Seasons []*models.Season
	Matches []*teamSeasonMatchRow
	Leaders []*models.StatLine
}

func (app *application) teamSeasonView(w http.ResponseWriter, r *http.Request) {
	team, ok := app.getRouteTeam(w, r)
	if !ok {
		return
	}

	params := httprouter.ParamsFromContext(r.Context())
	seasonID, err := strconv.Atoi(params.ByName("seasonID"))
	if err != nil || seasonID < 1 {
		app.notFound(w)
		return
	}

	sm := &models.SeasonModel{DB: app.playerService.DB}
	season, err := sm.Get(seasonID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}
	if season.LeagueID != team.LeagueID {
		app.notFound(w)
		return
	}

	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(team.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	seasons, err := sm.GetByLeague(team.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	mm := &models.MatchModel{DB: app.playerService.DB}
	matches, err := mm.GetByTeamAndSeason(team.ID, seasonID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	tm := &models.TeamModel{DB: app.playerService.DB}
	locm := &models.LocationModel{DB: app.playerService.DB}
	am := &models.AddressModel{DB: app.playerService.DB}
	rows := make([]*teamSeasonMatchRow, 0, len(matches))
	for _, match := range matches {
		isHome := match.HomeTeamID == team.ID
		opponentID := match.AwayTeamID
		if !isHome {
			opponentID = match.HomeTeamID
		}
		opponent, err := tm.Get(opponentID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		var location *models.Location
		var locationAddress *models.Address
		if match.LocationID.Valid {
			location, err = locm.Get(int(match.LocationID.Int32))
			if err != nil {
				app.serverError(w, err)
				return
			}
			locationAddress, err = am.Get(location.AddressID)
			if err != nil {
				app.serverError(w, err)
				return
			}
		}
		rows = append(rows, &teamSeasonMatchRow{Match: match, Opponent: opponent.Name, IsHome: isHome, Location: location, LocationAddress: locationAddress})
	}

	pmsm := &models.PlayerMatchStatModel{DB: app.playerService.DB}
	leaders, err := pmsm.LeaderboardByTeamSeason(team.ID, seasonID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = &teamSeasonViewData{
		Team:    team,
		League:  league,
		Season:  season,
		Seasons: seasons,
		Matches: rows,
		Leaders: leaders,
	}
	data.Breadcrumbs = append(app.teamBreadcrumbs(team, league, false), Breadcrumb{Label: "Schedule"})

	app.render(w, http.StatusOK, "team-season-view.html", data)
}

func (app *application) seasonDelete(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	sm := &models.SeasonModel{DB: app.playerService.DB}
	season, err := sm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	if !app.canManageLeague(r, season.LeagueID) {
		app.clientError(w, http.StatusForbidden)
		return
	}

	err = app.seasonService.DeleteSeason(id, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrHasDependents) {
			mm := &models.MatchModel{DB: app.playerService.DB}
			matches, merr := mm.GetBySeason(id)
			if merr != nil {
				app.serverError(w, merr)
				return
			}
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, "%s still has %d match(es) scheduled. Delete them first.", season.Name, len(matches))
			return
		}
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

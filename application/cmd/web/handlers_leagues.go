package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

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
	// Seasons lists every season in the league, most-recent-first, for the
	// season-picker <select> — see leagueView's season-resolution comment.
	Seasons   []*models.Season
	ActiveTab string
	// Season is the one being shown — standings, its leader tables, and
	// the Matches tab all key off it. Picked via ?season= if valid for
	// this league, otherwise GetCurrentOrNext (see leagueView).
	Season *models.Season
	// MatchCount is always populated whenever Season != nil (a cheap
	// models.Match count, needed for the Delete Season confirmation
	// message regardless of which tab is active) — MatchDays is the
	// expensive, fully-hydrated-per-match version, built only when
	// ActiveTab is "matches".
	MatchCount       int
	Standings        []*standingRow
	StandingsColumns []standingsColumn
	GoalLeaders      []*models.LeagueLeaderLine
	AssistLeaders    []*models.LeagueLeaderLine
	MatchDays        []*matchDayGroup
}

// matchDayGroup buckets a season's matches by calendar date (Eastern, same
// as every other match-date display — see matchDateTime) for the league
// page's Matches tab, rendered as one heading plus a grid of match cards
// per day rather than season-view.html's flat table.
type matchDayGroup struct {
	Label   string
	Matches []*seasonMatchRow
}

// matchDayKey/matchDayLabel key and label a match's calendar day. Mirrors
// matchDateTime's own branch exactly (raw date for a "no time recorded"
// sentinel, Eastern-converted date once a real time is on file) — grouping
// by a different rule than the date already shown on each card would let a
// match's heading and its own card disagree about what day it's on.
func matchDayKey(t time.Time) string {
	if !hasMatchTime(t) {
		return t.Format("2006-01-02")
	}
	return t.In(easternLocation).Format("2006-01-02")
}

func matchDayLabel(t time.Time) string {
	if !hasMatchTime(t) {
		return t.Format("Monday, January 2, 2006")
	}
	return t.In(easternLocation).Format("Monday, January 2, 2006")
}

// groupMatchesByDay buckets pre-sorted (by MatchModel.GetBySeason) match
// rows into consecutive same-day groups, preserving row order within a day.
func groupMatchesByDay(rows []*seasonMatchRow) []*matchDayGroup {
	var groups []*matchDayGroup
	var currentKey string
	for _, row := range rows {
		key := matchDayKey(row.Match.MatchDate)
		if len(groups) == 0 || key != currentKey {
			groups = append(groups, &matchDayGroup{Label: matchDayLabel(row.Match.MatchDate)})
			currentKey = key
		}
		last := groups[len(groups)-1]
		last.Matches = append(last.Matches, row)
	}
	return groups
}

// standingRow is one team's line in a season's standings table.
type standingRow struct {
	TeamID       int
	TeamName     string
	Points       int
	Wins         int
	Losses       int
	Draws        int
	GoalsFor     int
	GoalsAgainst int
}

// standingsColumn is one sortable column header on the standings table —
// URL already carries the query params clicking it should navigate to
// (toggling direction if it's already the active sort column).
type standingsColumn struct {
	Label  string
	Key    string
	URL    string
	Active bool
}

var validStandingsSortKeys = map[string]bool{
	"points": true, "wins": true, "losses": true, "draws": true, "goalsfor": true, "goalsagainst": true,
}

// buildStandings merges every team in the league (so a winless team still
// shows up with all zeros) with its season aggregate, computing Points from
// the 3/1/0 win/draw/loss rule. Unsorted — callers sort separately.
func buildStandings(db *sql.DB, teams []*models.Team, seasonID int) ([]*standingRow, error) {
	mm := &models.MatchModel{DB: db}
	aggregates, err := mm.GetSeasonAggregatesByTeam(seasonID)
	if err != nil {
		return nil, err
	}

	byTeam := make(map[int]*models.TeamMatchAggregate, len(aggregates))
	for _, agg := range aggregates {
		byTeam[agg.TeamID] = agg
	}

	rows := make([]*standingRow, 0, len(teams))
	for _, team := range teams {
		row := &standingRow{TeamID: team.ID, TeamName: team.Name}
		if agg, ok := byTeam[team.ID]; ok {
			row.Wins = agg.Wins
			row.Losses = agg.Losses
			row.Draws = agg.Draws
			row.GoalsFor = agg.GoalsFor
			row.GoalsAgainst = agg.GoalsAgainst
			row.Points = agg.Wins*3 + agg.Draws
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// sortStandings sorts rows in place by sortKey (validated against
// validStandingsSortKeys by the caller), descending unless dir is "asc".
func sortStandings(rows []*standingRow, sortKey, dir string) {
	sort.SliceStable(rows, func(i, j int) bool {
		var a, b int
		switch sortKey {
		case "wins":
			a, b = rows[i].Wins, rows[j].Wins
		case "losses":
			a, b = rows[i].Losses, rows[j].Losses
		case "draws":
			a, b = rows[i].Draws, rows[j].Draws
		case "goalsfor":
			a, b = rows[i].GoalsFor, rows[j].GoalsFor
		case "goalsagainst":
			a, b = rows[i].GoalsAgainst, rows[j].GoalsAgainst
		default:
			a, b = rows[i].Points, rows[j].Points
		}
		if dir == "asc" {
			return a < b
		}
		return a > b
	})
}

// buildStandingsColumns builds the six sortable column headers — clicking
// an inactive column sorts by it descending; clicking the already-active
// column toggles direction. leagueID/seasonID identify the league page
// and the season currently selected on it, since the standings table is
// always scoped to one season at a time.
func buildStandingsColumns(leagueID, seasonID int, currentSort, currentDir string) []standingsColumn {
	defs := []struct{ Label, Key string }{
		{"Pts", "points"},
		{"W", "wins"},
		{"L", "losses"},
		{"D", "draws"},
		{"GF", "goalsfor"},
		{"GA", "goalsagainst"},
	}

	cols := make([]standingsColumn, len(defs))
	for i, d := range defs {
		active := d.Key == currentSort
		nextDir := "desc"
		if active && currentDir == "desc" {
			nextDir = "asc"
		}
		cols[i] = standingsColumn{
			Label:  d.Label,
			Key:    d.Key,
			URL:    fmt.Sprintf("/league/%d?season=%d&sort=%s&dir=%s&tab=standings", leagueID, seasonID, d.Key, nextDir),
			Active: active,
		}
	}
	return cols
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

	sm := &models.SeasonModel{DB: app.playerService.DB}
	seasons, err := sm.GetByLeague(id)
	if err != nil {
		app.serverError(w, err)
		return
	}

	sortKey := r.URL.Query().Get("sort")
	if !validStandingsSortKeys[sortKey] {
		sortKey = "points"
	}
	dir := r.URL.Query().Get("dir")
	if dir != "asc" {
		dir = "desc"
	}

	// season is shared by all three tabs — standings, its leader tables,
	// and the Matches tab all show the same season. A ?season= query param
	// picks a specific season from this league's history (e.g. from the
	// season picker or a bookmarked/shared link); anything invalid, absent,
	// or belonging to a different league falls back to GetCurrentOrNext,
	// the same date-based pick the team page uses for its own
	// schedule/leaderboard. A newly created season is "current" (and shows
	// zeroed standings/leaders) the moment it exists, rather than the page
	// lingering on a previous season until the new one has recorded
	// results — that staleness, and standings/Matches disagreeing with
	// each other about which season was "current," were both bugs.
	var season *models.Season
	if seasonID, convErr := strconv.Atoi(r.URL.Query().Get("season")); convErr == nil && seasonID > 0 {
		if picked, pickErr := sm.Get(seasonID); pickErr == nil && picked.LeagueID == id {
			season = picked
		} else if pickErr != nil && !errors.Is(pickErr, models.ErrNoRecord) {
			app.serverError(w, pickErr)
			return
		}
	}
	if season == nil {
		season, err = sm.GetCurrentOrNext(id, time.Now())
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			app.serverError(w, err)
			return
		}
	}

	var standings []*standingRow
	var goalLeaders, assistLeaders []*models.LeagueLeaderLine
	var matchCount int

	if season != nil {
		standings, err = buildStandings(app.playerService.DB, teams, season.ID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		sortStandings(standings, sortKey, dir)

		pmsm := &models.PlayerMatchStatModel{DB: app.playerService.DB}
		goalLeaders, err = pmsm.TopScorersForSeason(season.ID, 5)
		if err != nil {
			app.serverError(w, err)
			return
		}
		assistLeaders, err = pmsm.TopAssistersForSeason(season.ID, 5)
		if err != nil {
			app.serverError(w, err)
			return
		}
	}

	activeTab := r.URL.Query().Get("tab")
	if activeTab != "matches" && activeTab != "leaders" {
		activeTab = "standings"
	}

	var matchDays []*matchDayGroup
	if season != nil {
		mm := &models.MatchModel{DB: app.playerService.DB}
		matches, err := mm.GetBySeason(season.ID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		matchCount = len(matches)

		if activeTab == "matches" {
			rows, err := app.buildSeasonMatchRows(matches)
			if err != nil {
				app.serverError(w, err)
				return
			}
			matchDays = groupMatchesByDay(rows)
		}
	}

	standingsColumns := []standingsColumn{}
	if season != nil {
		standingsColumns = buildStandingsColumns(id, season.ID, sortKey, dir)
	}

	data := app.newTemplateData(r)
	data.Data = &leagueViewData{
		League:           league,
		Teams:            teams,
		CanManage:        app.canManageLeague(r, id),
		Admins:           admins,
		Seasons:          seasons,
		ActiveTab:        activeTab,
		Season:           season,
		MatchCount:       matchCount,
		Standings:        standings,
		StandingsColumns: standingsColumns,
		GoalLeaders:      goalLeaders,
		AssistLeaders:    assistLeaders,
		MatchDays:        matchDays,
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

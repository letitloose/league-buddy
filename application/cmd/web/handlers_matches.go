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

// matchFormData is the shape match-create.html needs: the season it belongs
// to, the teams that season's league offers for the home/away dropdowns,
// and every location for the venue dropdown.
type matchFormData struct {
	Season    *models.Season
	League    *models.League
	Teams     []*models.Team
	Locations []*models.Location
}

func matchFormBreadcrumbs(season *models.Season, league *models.League) []Breadcrumb {
	return []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)},
		{Label: season.Name, URL: fmt.Sprintf("/season/%d", season.ID)},
		{Label: "Add Match"},
	}
}

func (app *application) renderMatchCreateForm(w http.ResponseWriter, r *http.Request, form *services.MatchForm, season *models.Season, status int) {
	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(season.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	tm := &models.TeamModel{DB: app.playerService.DB}
	teams, err := tm.GetByLeague(season.LeagueID)
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
	data.Data = &matchFormData{Season: season, League: league, Teams: teams, Locations: locations}
	data.Breadcrumbs = matchFormBreadcrumbs(season, league)
	app.render(w, status, "match-create.html", data)
}

func (app *application) matchForm(w http.ResponseWriter, r *http.Request) {
	seasonID, _ := strconv.Atoi(r.URL.Query().Get("seasonID"))
	if seasonID < 1 {
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

	if !app.canManageLeague(r, season.LeagueID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	app.renderMatchCreateForm(w, r, &services.MatchForm{SeasonID: seasonID}, season, http.StatusOK)
}

func (app *application) matchCreate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	seasonID, _ := strconv.Atoi(r.PostForm.Get("seasonID"))
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

	if !app.canManageLeague(r, season.LeagueID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	homeTeamID, _ := strconv.Atoi(r.PostForm.Get("hometeamid"))
	awayTeamID, _ := strconv.Atoi(r.PostForm.Get("awayteamid"))
	locationID, _ := strconv.Atoi(r.PostForm.Get("locationid"))

	form := &services.MatchForm{
		SeasonID:   seasonID,
		HomeTeamID: homeTeamID,
		AwayTeamID: awayTeamID,
		LocationID: locationID,
		MatchDate:  r.PostForm.Get("matchdate"),
		HomeScore:  r.PostForm.Get("homescore"),
		AwayScore:  r.PostForm.Get("awayscore"),
		Notes:      r.PostForm.Get("notes"),
	}

	_, err = app.matchService.CreateMatch(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			app.renderMatchCreateForm(w, r, form, season, http.StatusUnprocessableEntity)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Match added.")
	http.Redirect(w, r, fmt.Sprintf("/season/%d", seasonID), http.StatusSeeOther)
}

// matchRosterEntry is one row of the per-player stat grid on
// match-update.html — a roster player alongside their existing stat line
// for this match, if any (all zero for a player who hasn't been recorded
// yet).
type matchRosterEntry struct {
	Player *models.Player
	Stat   *models.PlayerMatchStat
}

type matchUpdateData struct {
	Match      *models.Match
	Season     *models.Season
	League     *models.League
	HomeTeam   *models.Team
	AwayTeam   *models.Team
	Locations  []*models.Location
	HomeRoster []*matchRosterEntry
	AwayRoster []*matchRosterEntry
}

func buildMatchRoster(roster []*models.Player, teamID int, statsByPlayer map[int]*models.PlayerMatchStat) []*matchRosterEntry {
	entries := make([]*matchRosterEntry, 0, len(roster))
	for _, player := range roster {
		stat := statsByPlayer[player.ID]
		if stat == nil {
			stat = &models.PlayerMatchStat{PlayerID: player.ID, TeamID: teamID}
		}
		entries = append(entries, &matchRosterEntry{Player: player, Stat: stat})
	}
	return entries
}

func (app *application) renderMatchUpdateForm(w http.ResponseWriter, r *http.Request, form *services.MatchForm, match *models.Match, status int) {
	sm := &models.SeasonModel{DB: app.playerService.DB}
	season, err := sm.Get(match.SeasonID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(season.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	tm := &models.TeamModel{DB: app.playerService.DB}
	homeTeam, err := tm.Get(match.HomeTeamID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	awayTeam, err := tm.Get(match.AwayTeamID)
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

	homeRosterPlayers, err := app.playerService.GetByTeam(match.HomeTeamID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	awayRosterPlayers, err := app.playerService.GetByTeam(match.AwayTeamID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	pmsm := &models.PlayerMatchStatModel{DB: app.playerService.DB}
	existingStats, err := pmsm.ListByMatch(match.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	statsByPlayer := make(map[int]*models.PlayerMatchStat, len(existingStats))
	for _, stat := range existingStats {
		statsByPlayer[stat.PlayerID] = stat
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.Data = &matchUpdateData{
		Match:      match,
		Season:     season,
		League:     league,
		HomeTeam:   homeTeam,
		AwayTeam:   awayTeam,
		Locations:  locations,
		HomeRoster: buildMatchRoster(homeRosterPlayers, match.HomeTeamID, statsByPlayer),
		AwayRoster: buildMatchRoster(awayRosterPlayers, match.AwayTeamID, statsByPlayer),
	}
	data.Breadcrumbs = []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)},
		{Label: season.Name, URL: fmt.Sprintf("/season/%d", season.ID)},
		{Label: fmt.Sprintf("%s vs %s", homeTeam.Name, awayTeam.Name)},
	}

	app.render(w, status, "match-update.html", data)
}

func (app *application) getRouteMatch(w http.ResponseWriter, r *http.Request) (*models.Match, bool) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return nil, false
	}

	mm := &models.MatchModel{DB: app.playerService.DB}
	match, err := mm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return nil, false
	}

	return match, true
}

// canManageMatch reports whether the current request's user may edit
// match's score/stats: an admin, a league admin of the match's league, or
// the captain of either team that played in it — captains weren't
// previously involved in match administration, but they're the ones
// actually at the games, so they get the same edit access here that
// canManageTeam already gives them over their own team's roster.
func (app *application) canManageMatch(r *http.Request, match *models.Match) (bool, error) {
	canDelete, err := app.canDeleteMatch(r, match)
	if err != nil {
		return false, err
	}
	if canDelete {
		return true, nil
	}
	return app.isCaptainOfTeam(r, match.HomeTeamID) || app.isCaptainOfTeam(r, match.AwayTeamID), nil
}

// canDeleteMatch reports whether the current request's user may delete
// match: an admin or a league admin of the match's league. Deliberately
// excludes captains — same "delete is too destructive for a single
// unilateral captain" rule already applied to canDeleteTeam.
func (app *application) canDeleteMatch(r *http.Request, match *models.Match) (bool, error) {
	sm := &models.SeasonModel{DB: app.playerService.DB}
	season, err := sm.Get(match.SeasonID)
	if err != nil {
		return false, err
	}
	return app.canManageLeague(r, season.LeagueID), nil
}

// matchStatDisplayRow is one player's recorded stat line on the read-only
// match view — zero-stat roster rows (a player who didn't record anything)
// are omitted, unlike the edit grid, which shows every roster player so
// stats can be entered for the first time.
type matchStatDisplayRow struct {
	PlayerName  string
	Goals       int
	Assists     int
	YellowCards int
	RedCards    int
}

type matchViewData struct {
	Match           *models.Match
	Season          *models.Season
	League          *models.League
	HomeTeam        *models.Team
	AwayTeam        *models.Team
	Location        *models.Location
	LocationAddress *models.Address
	HomeStats       []*matchStatDisplayRow
	AwayStats       []*matchStatDisplayRow
	CanManage       bool
}

// matchView is the read-only "match screen" any active user can reach by
// clicking a schedule row (team home page, season page) — score, location,
// and every recorded player stat. CanManage gates the Edit link to
// /admin/match/update/:id (admins, league admins, or a captain of either
// team — see canManageMatch).
func (app *application) matchView(w http.ResponseWriter, r *http.Request) {
	match, ok := app.getRouteMatch(w, r)
	if !ok {
		return
	}

	sm := &models.SeasonModel{DB: app.playerService.DB}
	season, err := sm.Get(match.SeasonID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(season.LeagueID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	tm := &models.TeamModel{DB: app.playerService.DB}
	homeTeam, err := tm.Get(match.HomeTeamID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	awayTeam, err := tm.Get(match.AwayTeamID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	var location *models.Location
	var locationAddress *models.Address
	if match.LocationID.Valid {
		locm := &models.LocationModel{DB: app.playerService.DB}
		location, err = locm.Get(int(match.LocationID.Int32))
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

	pmsm := &models.PlayerMatchStatModel{DB: app.playerService.DB}
	stats, err := pmsm.ListByMatch(match.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	pm := &models.PlayerModel{DB: app.playerService.DB}
	var homeStats, awayStats []*matchStatDisplayRow
	for _, stat := range stats {
		if stat.Goals == 0 && stat.Assists == 0 && stat.YellowCards == 0 && stat.RedCards == 0 {
			continue
		}
		player, err := pm.Get(stat.PlayerID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		row := &matchStatDisplayRow{
			PlayerName:  player.FirstName + " " + player.LastName,
			Goals:       stat.Goals,
			Assists:     stat.Assists,
			YellowCards: stat.YellowCards,
			RedCards:    stat.RedCards,
		}
		if stat.TeamID == match.HomeTeamID {
			homeStats = append(homeStats, row)
		} else {
			awayStats = append(awayStats, row)
		}
	}

	canManage, err := app.canManageMatch(r, match)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = &matchViewData{
		Match:           match,
		Season:          season,
		League:          league,
		HomeTeam:        homeTeam,
		AwayTeam:        awayTeam,
		Location:        location,
		LocationAddress: locationAddress,
		HomeStats:       homeStats,
		AwayStats:       awayStats,
		CanManage:       canManage,
	}
	data.Breadcrumbs = []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)},
		{Label: season.Name, URL: fmt.Sprintf("/season/%d", season.ID)},
		{Label: fmt.Sprintf("%s vs %s", homeTeam.Name, awayTeam.Name)},
	}

	app.render(w, http.StatusOK, "match-view.html", data)
}

func (app *application) matchUpdate(w http.ResponseWriter, r *http.Request) {
	match, ok := app.getRouteMatch(w, r)
	if !ok {
		return
	}

	canManage, err := app.canManageMatch(r, match)
	if err != nil {
		app.serverError(w, err)
		return
	}
	if !canManage {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	form := &services.MatchForm{
		ID:         match.ID,
		SeasonID:   match.SeasonID,
		HomeTeamID: match.HomeTeamID,
		AwayTeamID: match.AwayTeamID,
		LocationID: int(match.LocationID.Int32),
		MatchDate:  pickerDate(match.MatchDate),
		Notes:      match.Notes.String,
	}
	if match.HomeScore.Valid {
		form.HomeScore = strconv.Itoa(int(match.HomeScore.Int32))
	}
	if match.AwayScore.Valid {
		form.AwayScore = strconv.Itoa(int(match.AwayScore.Int32))
	}

	app.renderMatchUpdateForm(w, r, form, match, http.StatusOK)
}

// parseMatchStats reads the per-player stat grid match-update.html submits:
// one hidden "statPlayerIDs" value per roster row, paired with
// teamID_<id>/goals_<id>/assists_<id>/yellow_<id>/red_<id> fields.
func parseMatchStats(r *http.Request) []services.PlayerStatInput {
	ids := r.PostForm["statPlayerIDs"]
	stats := make([]services.PlayerStatInput, 0, len(ids))
	for _, idStr := range ids {
		playerID, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}
		teamID, _ := strconv.Atoi(r.PostForm.Get("teamID_" + idStr))
		goals, _ := strconv.Atoi(r.PostForm.Get("goals_" + idStr))
		assists, _ := strconv.Atoi(r.PostForm.Get("assists_" + idStr))
		yellow, _ := strconv.Atoi(r.PostForm.Get("yellow_" + idStr))
		red, _ := strconv.Atoi(r.PostForm.Get("red_" + idStr))
		stats = append(stats, services.PlayerStatInput{
			PlayerID: playerID, TeamID: teamID, Goals: goals, Assists: assists, YellowCards: yellow, RedCards: red,
		})
	}
	return stats
}

func (app *application) matchUpdatePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PostForm.Get("match-id"))
	if err != nil || id < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	mm := &models.MatchModel{DB: app.playerService.DB}
	existing, err := mm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	canManage, err := app.canManageMatch(r, existing)
	if err != nil {
		app.serverError(w, err)
		return
	}
	if !canManage {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	locationID, _ := strconv.Atoi(r.PostForm.Get("locationid"))

	// Home/away teams are fixed once a match exists (see match-team-fields'
	// doc comment) — reuse the existing values rather than trusting the
	// (nonexistent) form fields.
	form := &services.MatchForm{
		ID:         id,
		SeasonID:   existing.SeasonID,
		HomeTeamID: existing.HomeTeamID,
		AwayTeamID: existing.AwayTeamID,
		LocationID: locationID,
		MatchDate:  r.PostForm.Get("matchdate"),
		HomeScore:  r.PostForm.Get("homescore"),
		AwayScore:  r.PostForm.Get("awayscore"),
		Notes:      r.PostForm.Get("notes"),
		Stats:      parseMatchStats(r),
	}

	err = app.matchService.UpdateMatch(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			app.renderMatchUpdateForm(w, r, form, existing, http.StatusUnprocessableEntity)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Match updated.")
	http.Redirect(w, r, fmt.Sprintf("/season/%d", existing.SeasonID), http.StatusSeeOther)
}

func (app *application) matchDelete(w http.ResponseWriter, r *http.Request) {
	match, ok := app.getRouteMatch(w, r)
	if !ok {
		return
	}

	canDelete, err := app.canDeleteMatch(r, match)
	if err != nil {
		app.serverError(w, err)
		return
	}
	if !canDelete {
		app.clientError(w, http.StatusForbidden)
		return
	}

	if err := app.matchService.DeleteMatch(match.ID, app.getUserName(r)); err != nil {
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

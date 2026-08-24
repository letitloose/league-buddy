package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

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

// goalFormRow/cardFormRow are match-update.html's template-friendly view of
// a saved goal/card: plain ints (0 = unattributed) rather than
// sql.NullInt32, since Go templates can't {{eq}} an int32 against an int
// without a helper function, and the form's dropdowns just need "is this
// the selected option" comparisons against plain ints.
type goalFormRow struct {
	TeamID           int
	ScorerPlayerID   int
	AssisterPlayerID int
}

type cardFormRow struct {
	TeamID   int
	PlayerID int
	CardType string
}

func newGoalFormRow(g *models.MatchGoal) *goalFormRow {
	row := &goalFormRow{TeamID: g.TeamID}
	if g.ScorerPlayerID.Valid {
		row.ScorerPlayerID = int(g.ScorerPlayerID.Int32)
	}
	if g.AssisterPlayerID.Valid {
		row.AssisterPlayerID = int(g.AssisterPlayerID.Int32)
	}
	return row
}

func newCardFormRow(c *models.MatchCard) *cardFormRow {
	row := &cardFormRow{TeamID: c.TeamID, CardType: c.CardType}
	if c.PlayerID.Valid {
		row.PlayerID = int(c.PlayerID.Int32)
	}
	return row
}

type matchUpdateData struct {
	Match      *models.Match
	Season     *models.Season
	League     *models.League
	HomeTeam   *models.Team
	AwayTeam   *models.Team
	Locations  []*models.Location
	HomeRoster []*models.Player
	AwayRoster []*models.Player
	Goals      []*goalFormRow
	Cards      []*cardFormRow
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

	mgm := &models.MatchGoalModel{DB: app.playerService.DB}
	savedGoals, err := mgm.ListByMatch(match.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	mcm := &models.MatchCardModel{DB: app.playerService.DB}
	savedCards, err := mcm.ListByMatch(match.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	goals := make([]*goalFormRow, len(savedGoals))
	for i, g := range savedGoals {
		goals[i] = newGoalFormRow(g)
	}
	cards := make([]*cardFormRow, len(savedCards))
	for i, c := range savedCards {
		cards[i] = newCardFormRow(c)
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
		HomeRoster: homeRosterPlayers,
		AwayRoster: awayRosterPlayers,
		Goals:      goals,
		Cards:      cards,
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

// rsvpDisplayRow is one roster player who RSVP'd with a given status
// ("yes" or "no") — a no-response roster player never gets a row, since the
// point of these lists is "who actually answered," not a full roll call.
type rsvpDisplayRow struct {
	PlayerName string
	Message    string
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
	CanRSVP         bool
	IsPast          bool
	HomeRSVPsIn     []*rsvpDisplayRow
	AwayRSVPsIn     []*rsvpDisplayRow
	HomeRSVPsOut    []*rsvpDisplayRow
	AwayRSVPsOut    []*rsvpDisplayRow
}

// buildRSVPRows returns a row for every roster player who RSVP'd status
// ("yes" or "no"), skipping anyone who hasn't responded.
func buildRSVPRows(roster []*models.Player, rsvpsByPlayer map[int]*models.RSVP, status string) []*rsvpDisplayRow {
	rows := make([]*rsvpDisplayRow, 0, len(roster))
	for _, player := range roster {
		rsvp := rsvpsByPlayer[player.ID]
		if rsvp == nil || rsvp.Status != status {
			continue
		}
		rows = append(rows, &rsvpDisplayRow{
			PlayerName: player.FirstName + " " + player.LastName,
			Message:    rsvp.Message.String,
		})
	}
	return rows
}

// matchIsPast reports whether match's date is strictly before today — used
// to close RSVPing once a match has already happened. Compared at day
// granularity (today itself still counts as open) rather than exact
// kickoff time, since MatchDate is a plain DATE with no time component.
// "Now" is converted into MatchDate's own location before extracting the
// year/month/day, rather than the server process's local location, so a
// same-calendar-day match is never miscounted as past by a few hours of
// UTC/local skew.
func matchIsPast(match *models.Match) bool {
	now := time.Now().In(match.MatchDate.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, match.MatchDate.Location())
	return match.MatchDate.Before(today)
}

// buildMatchViewData assembles everything match-view.html needs. Split out
// of matchView so a failed RSVP submission can re-render the same page
// (with the just-submitted, possibly-invalid form re-attached) without
// duplicating this lookup logic.
func (app *application) buildMatchViewData(r *http.Request, match *models.Match) (*matchViewData, error) {
	sm := &models.SeasonModel{DB: app.playerService.DB}
	season, err := sm.Get(match.SeasonID)
	if err != nil {
		return nil, err
	}
	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(season.LeagueID)
	if err != nil {
		return nil, err
	}
	tm := &models.TeamModel{DB: app.playerService.DB}
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
		locm := &models.LocationModel{DB: app.playerService.DB}
		location, err = locm.Get(int(match.LocationID.Int32))
		if err != nil {
			return nil, err
		}
		am := &models.AddressModel{DB: app.playerService.DB}
		locationAddress, err = am.Get(location.AddressID)
		if err != nil {
			return nil, err
		}
	}

	pmsm := &models.PlayerMatchStatModel{DB: app.playerService.DB}
	stats, err := pmsm.ListByMatch(match.ID)
	if err != nil {
		return nil, err
	}
	pm := &models.PlayerModel{DB: app.playerService.DB}
	var homeStats, awayStats []*matchStatDisplayRow
	for _, stat := range stats {
		if stat.Goals == 0 && stat.Assists == 0 && stat.YellowCards == 0 && stat.RedCards == 0 {
			continue
		}
		player, err := pm.Get(stat.PlayerID)
		if err != nil {
			return nil, err
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

	homeRoster, err := app.playerService.GetByTeam(match.HomeTeamID)
	if err != nil {
		return nil, err
	}
	awayRoster, err := app.playerService.GetByTeam(match.AwayTeamID)
	if err != nil {
		return nil, err
	}
	rm := &models.RSVPModel{DB: app.playerService.DB}
	rsvps, err := rm.ListByMatch(match.ID)
	if err != nil {
		return nil, err
	}
	rsvpsByPlayer := make(map[int]*models.RSVP, len(rsvps))
	for _, rsvp := range rsvps {
		rsvpsByPlayer[rsvp.PlayerID] = rsvp
	}

	canManage, err := app.canManageMatch(r, match)
	if err != nil {
		return nil, err
	}

	playerID := app.getPlayerID(r)
	isPast := matchIsPast(match)
	canRSVP := playerID > 0 && !isPast && (app.isMemberOfTeam(r, match.HomeTeamID) || app.isMemberOfTeam(r, match.AwayTeamID))

	return &matchViewData{
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
		CanRSVP:         canRSVP,
		IsPast:          isPast,
		HomeRSVPsIn:     buildRSVPRows(homeRoster, rsvpsByPlayer, "yes"),
		AwayRSVPsIn:     buildRSVPRows(awayRoster, rsvpsByPlayer, "yes"),
		HomeRSVPsOut:    buildRSVPRows(homeRoster, rsvpsByPlayer, "no"),
		AwayRSVPsOut:    buildRSVPRows(awayRoster, rsvpsByPlayer, "no"),
	}, nil
}

func matchViewBreadcrumbs(league *models.League, season *models.Season, homeTeam, awayTeam *models.Team) []Breadcrumb {
	return []Breadcrumb{
		{Label: "Leagues", URL: "/league"},
		{Label: league.Name, URL: fmt.Sprintf("/league/%d", league.ID)},
		{Label: season.Name, URL: fmt.Sprintf("/season/%d", season.ID)},
		{Label: fmt.Sprintf("%s vs %s", homeTeam.Name, awayTeam.Name)},
	}
}

// renderMatchView renders match-view.html for viewData, with form attached
// as the sticky RSVP widget's contents (the viewer's existing response on a
// plain GET, or a just-submitted-but-invalid one on a failed POST).
func (app *application) renderMatchView(w http.ResponseWriter, r *http.Request, viewData *matchViewData, form *services.RSVPForm, status int) {
	data := app.newTemplateData(r)
	data.Data = viewData
	data.Form = form
	data.Breadcrumbs = matchViewBreadcrumbs(viewData.League, viewData.Season, viewData.HomeTeam, viewData.AwayTeam)

	app.render(w, status, "match-view.html", data)
}

// matchView is the read-only "match screen" any active user can reach by
// clicking a schedule row (team home page, season page) — score, location,
// every recorded player stat, and (for a roster member of either team) an
// RSVP widget plus the full in/out roll call. CanManage gates the Edit link
// to /admin/match/update/:id (admins, league admins, or a captain of either
// team — see canManageMatch).
func (app *application) matchView(w http.ResponseWriter, r *http.Request) {
	match, ok := app.getRouteMatch(w, r)
	if !ok {
		return
	}

	viewData, err := app.buildMatchViewData(r, match)
	if err != nil {
		app.serverError(w, err)
		return
	}

	form := &services.RSVPForm{}
	if viewData.CanRSVP {
		rm := &models.RSVPModel{DB: app.playerService.DB}
		existing, err := rm.GetByMatchAndPlayer(match.ID, app.getPlayerID(r))
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			app.serverError(w, err)
			return
		}
		if err == nil {
			form.Status = existing.Status
			form.Message = existing.Message.String
		}
	}

	app.renderMatchView(w, r, viewData, form, http.StatusOK)
}

// matchRSVPSubmit records the current player's yes/no response (plus an
// optional message) to a match. Eligibility is "on the roster of either the
// home or away team" (isMemberOfTeam), the same bar joinRequestSubmit uses
// for its own in-handler check rather than a dedicated middleware tier.
func (app *application) matchRSVPSubmit(w http.ResponseWriter, r *http.Request) {
	match, ok := app.getRouteMatch(w, r)
	if !ok {
		return
	}

	playerID := app.getPlayerID(r)
	isHome := app.isMemberOfTeam(r, match.HomeTeamID)
	isAway := app.isMemberOfTeam(r, match.AwayTeamID)
	if playerID <= 0 || !(isHome || isAway) || matchIsPast(match) {
		http.Redirect(w, r, fmt.Sprintf("/match/%d", match.ID), http.StatusSeeOther)
		return
	}
	// A player on both rosters (a rare cross-team case) defaults to the
	// home side — there's no team selector in the RSVP widget itself.
	teamID := match.AwayTeamID
	if isHome {
		teamID = match.HomeTeamID
	}

	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	form := &services.RSVPForm{
		Status:  r.PostForm.Get("status"),
		Message: r.PostForm.Get("message"),
	}

	err := app.rsvpService.SubmitRSVP(match.ID, playerID, teamID, form)
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			viewData, buildErr := app.buildMatchViewData(r, match)
			if buildErr != nil {
				app.serverError(w, buildErr)
				return
			}
			app.renderMatchView(w, r, viewData, form, http.StatusUnprocessableEntity)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "RSVP saved.")
	http.Redirect(w, r, fmt.Sprintf("/match/%d", match.ID), http.StatusSeeOther)
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

// parseGoalsAndCards reads the dynamic goal/card row builder
// match-update.html submits: one set of same-named fields per row (Go's
// r.PostForm collects repeated field names into an ordered slice, so the
// i-th value of each name belongs to the same row). A blank select (the
// "unattributed" option) submits as "", which parses to 0.
func parseGoalsAndCards(r *http.Request) ([]services.GoalInput, []services.CardInput) {
	goalTeamIDs := r.PostForm["goalTeamID"]
	goalScorerIDs := r.PostForm["goalScorerID"]
	goalAssisterIDs := r.PostForm["goalAssisterID"]
	goals := make([]services.GoalInput, 0, len(goalTeamIDs))
	for i, teamIDStr := range goalTeamIDs {
		teamID, err := strconv.Atoi(teamIDStr)
		if err != nil {
			continue
		}
		scorerID, _ := strconv.Atoi(valueAt(goalScorerIDs, i))
		assisterID, _ := strconv.Atoi(valueAt(goalAssisterIDs, i))
		goals = append(goals, services.GoalInput{TeamID: teamID, ScorerPlayerID: scorerID, AssisterPlayerID: assisterID})
	}

	cardTeamIDs := r.PostForm["cardTeamID"]
	cardPlayerIDs := r.PostForm["cardPlayerID"]
	cardTypes := r.PostForm["cardType"]
	cards := make([]services.CardInput, 0, len(cardTeamIDs))
	for i, teamIDStr := range cardTeamIDs {
		teamID, err := strconv.Atoi(teamIDStr)
		if err != nil {
			continue
		}
		playerID, _ := strconv.Atoi(valueAt(cardPlayerIDs, i))
		cards = append(cards, services.CardInput{TeamID: teamID, PlayerID: playerID, CardType: valueAt(cardTypes, i)})
	}

	return goals, cards
}

// valueAt returns values[i], or "" if i is out of range — guards against a
// malformed submission where the parallel slices' lengths don't line up.
func valueAt(values []string, i int) string {
	if i < 0 || i >= len(values) {
		return ""
	}
	return values[i]
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
	goals, cards := parseGoalsAndCards(r)
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
		Goals:      goals,
		Cards:      cards,
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
	http.Redirect(w, r, fmt.Sprintf("/match/%d", existing.ID), http.StatusSeeOther)
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

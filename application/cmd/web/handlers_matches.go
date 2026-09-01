package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/julienschmidt/httprouter"
	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/services"
	"github.com/letitloose/league-buddy/internal/validator"
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
	Minute           int
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
	if g.Minute.Valid {
		row.Minute = int(g.Minute.Int32)
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
// match's score/stats: an admin, a league admin of the match's league, the
// captain of either team that played in it, or a scorekeeper either
// captain has designated for their team — captains weren't previously
// involved in match administration, but they (and whoever they delegate
// to) are the ones actually at the games, so they get the same edit access
// here that canManageTeam already gives captains over their own team's
// roster. Scorekeepers get only this, not canManageTeam's wider rights.
func (app *application) canManageMatch(r *http.Request, match *models.Match) (bool, error) {
	canDelete, err := app.canDeleteMatch(r, match)
	if err != nil {
		return false, err
	}
	if canDelete {
		return true, nil
	}
	if app.isCaptainOfTeam(r, match.HomeTeamID) || app.isCaptainOfTeam(r, match.AwayTeamID) {
		return true, nil
	}
	return app.isScorekeeperOfTeam(r, match.HomeTeamID) || app.isScorekeeperOfTeam(r, match.AwayTeamID), nil
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

// goalBoxScoreRow is one goal on the read-only match view — a real box
// score entry, not an aggregated count. Rows arrive from
// MatchGoalModel.ListByMatch already in chronological (minute, then
// insertion) order. ScorerName/AssisterName are blank when unattributed.
type goalBoxScoreRow struct {
	Minute       int // 0 = not recorded
	ScorerName   string
	AssisterName string
	IsOwnGoal    bool // scorer is on the *other* team's roster, not the one credited with the goal
}

// cardBoxScoreRow is one card on the read-only match view. PlayerName is
// blank when unattributed.
type cardBoxScoreRow struct {
	PlayerName string
	CardType   string // "yellow" or "red"
}

// rsvpDisplayRow is one roster player who RSVP'd with a given status
// ("yes" or "no") — a no-response roster player never gets a row, since the
// point of these lists is "who actually answered," not a full roll call.
type rsvpDisplayRow struct {
	PlayerName string
	Message    string
}

// matchTeamNoteView is one team's Player of the Match pick and captain's
// notes, template-facing — Form carries the sticky (possibly invalid) value
// on a failed submission, or the currently saved value otherwise.
type matchTeamNoteView struct {
	TeamID                 int
	Roster                 []*models.Player
	PlayerOfMatchName      string
	Notes                  string
	CaptainMessage         string
	CanManage              bool
	Form                   *services.MatchTeamNoteForm
	CanSendTestReminder    bool
	ActivatedTeammates     []*teammateOption
	VerifiedPhoneTeammates []*teammateOption
}

// teammateOption is one roster player offered on the match test-reminder
// tool's "select from a list of teammates" pickers (matchTestReminderSubmit
// for email/ActivatedTeammates, matchTestReminderSMSSubmit for
// text/VerifiedPhoneTeammates) — mirroring the invite screen's
// roster-picker pattern (RosterWithoutAccount in handlers_teams.go) but
// for the opposite case: someone reachable enough to send a test to.
// Only one of Email/Phone is populated depending on which list this came
// from.
type teammateOption struct {
	PlayerID int
	Name     string
	Email    string
	Phone    string
}

type matchViewData struct {
	Match           *models.Match
	Season          *models.Season
	League          *models.League
	HomeTeam        *models.Team
	AwayTeam        *models.Team
	Location        *models.Location
	LocationAddress *models.Address
	HomeGoals       []*goalBoxScoreRow
	AwayGoals       []*goalBoxScoreRow
	HomeCards       []*cardBoxScoreRow
	AwayCards       []*cardBoxScoreRow
	CanManage       bool
	CanRSVP         bool
	IsPast          bool
	HomeRSVPsIn     []*rsvpDisplayRow
	AwayRSVPsIn     []*rsvpDisplayRow
	HomeRSVPsOut    []*rsvpDisplayRow
	AwayRSVPsOut    []*rsvpDisplayRow
	HomeNote        *matchTeamNoteView
	AwayNote        *matchTeamNoteView
	ShowHomeBox     bool
	ShowAwayBox     bool
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

	homeRoster, err := app.playerService.GetByTeam(match.HomeTeamID)
	if err != nil {
		return nil, err
	}
	awayRoster, err := app.playerService.GetByTeam(match.AwayTeamID)
	if err != nil {
		return nil, err
	}
	playerName := make(map[int]string, len(homeRoster)+len(awayRoster))
	homeRosterIDs := make(map[int]bool, len(homeRoster))
	for _, player := range homeRoster {
		playerName[player.ID] = player.FirstName + " " + player.LastName
		homeRosterIDs[player.ID] = true
	}
	awayRosterIDs := make(map[int]bool, len(awayRoster))
	for _, player := range awayRoster {
		playerName[player.ID] = player.FirstName + " " + player.LastName
		awayRosterIDs[player.ID] = true
	}
	// isOnCreditedRoster reports whether playerID belongs to the roster of
	// teamID — used to flag an own goal (a goal's scorer credited to a team
	// they're not actually on).
	isOnCreditedRoster := func(playerID, teamID int) bool {
		if teamID == match.HomeTeamID {
			return homeRosterIDs[playerID]
		}
		return awayRosterIDs[playerID]
	}
	// nameFor falls back to a direct lookup for a player who scored/was
	// carded but has since left the roster shown above — rare, but a
	// roster removal shouldn't make historical box-score rows blank.
	pm := &models.PlayerModel{DB: app.playerService.DB}
	nameFor := func(playerID sql.NullInt32) (string, error) {
		if !playerID.Valid {
			return "", nil
		}
		id := int(playerID.Int32)
		if name, ok := playerName[id]; ok {
			return name, nil
		}
		player, err := pm.Get(id)
		if err != nil {
			return "", err
		}
		return player.FirstName + " " + player.LastName, nil
	}

	mgm := &models.MatchGoalModel{DB: app.playerService.DB}
	goals, err := mgm.ListByMatch(match.ID)
	if err != nil {
		return nil, err
	}
	var homeGoals, awayGoals []*goalBoxScoreRow
	for _, g := range goals {
		scorerName, err := nameFor(g.ScorerPlayerID)
		if err != nil {
			return nil, err
		}
		assisterName, err := nameFor(g.AssisterPlayerID)
		if err != nil {
			return nil, err
		}
		row := &goalBoxScoreRow{ScorerName: scorerName, AssisterName: assisterName}
		if g.ScorerPlayerID.Valid && !isOnCreditedRoster(int(g.ScorerPlayerID.Int32), g.TeamID) {
			row.IsOwnGoal = true
		}
		if g.Minute.Valid {
			row.Minute = int(g.Minute.Int32)
		}
		if g.TeamID == match.HomeTeamID {
			homeGoals = append(homeGoals, row)
		} else {
			awayGoals = append(awayGoals, row)
		}
	}

	mcm := &models.MatchCardModel{DB: app.playerService.DB}
	cards, err := mcm.ListByMatch(match.ID)
	if err != nil {
		return nil, err
	}
	var homeCards, awayCards []*cardBoxScoreRow
	for _, c := range cards {
		name, err := nameFor(c.PlayerID)
		if err != nil {
			return nil, err
		}
		row := &cardBoxScoreRow{PlayerName: name, CardType: c.CardType}
		if c.TeamID == match.HomeTeamID {
			homeCards = append(homeCards, row)
		} else {
			awayCards = append(awayCards, row)
		}
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

	// Before the match has a recorded result, a non-manager only sees their
	// own team's box — not the opponent's roster/RSVP roll call, notes,
	// etc. — so a team can't scout who's confirmed to play for the other
	// side ahead of the match. Once the score is in, that concern is moot
	// and everyone sees both boxes. Keyed off the score rather than
	// goal-by-goal detail specifically, matching the "Score not recorded"
	// convention already used just above on this same page — some
	// (especially older) matches only ever get a final score, never
	// goal-by-goal rows, and should still unlock.
	hasResult := match.HomeScore.Valid && match.AwayScore.Valid
	showHomeBox := canManage || hasResult || app.isMemberOfTeam(r, match.HomeTeamID)
	showAwayBox := canManage || hasResult || app.isMemberOfTeam(r, match.AwayTeamID)

	mtnm := &models.MatchTeamNoteModel{DB: app.playerService.DB}
	um := &models.UserModel{DB: app.playerService.DB}
	buildNoteView := func(teamID int, roster []*models.Player, canManageSide, canSendTestReminder bool) (*matchTeamNoteView, error) {
		note, err := mtnm.GetByMatchAndTeam(match.ID, teamID)
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return nil, err
		}
		view := &matchTeamNoteView{TeamID: teamID, Roster: roster, CanManage: canManageSide, Form: &services.MatchTeamNoteForm{}, CanSendTestReminder: canSendTestReminder}
		if canSendTestReminder {
			playerIDs := make([]int, len(roster))
			for i, p := range roster {
				playerIDs[i] = p.ID
			}
			activatedEmails, err := um.GetActivatedEmailsForPlayers(playerIDs)
			if err != nil {
				return nil, err
			}
			for _, p := range roster {
				if email, ok := activatedEmails[p.ID]; ok {
					view.ActivatedTeammates = append(view.ActivatedTeammates, &teammateOption{PlayerID: p.ID, Name: p.FirstName + " " + p.LastName, Email: email})
				}
				if p.PhoneVerifiedAt.Valid {
					view.VerifiedPhoneTeammates = append(view.VerifiedPhoneTeammates, &teammateOption{PlayerID: p.ID, Name: p.FirstName + " " + p.LastName, Phone: p.PhoneNumber.String})
				}
			}
		}
		if note != nil {
			if note.PlayerOfMatchID.Valid {
				view.Form.PlayerOfMatchID = int(note.PlayerOfMatchID.Int32)
			}
			view.Form.Notes = note.Notes.String
			view.Notes = note.Notes.String
			view.Form.CaptainMessage = note.CaptainMessage.String
			view.CaptainMessage = note.CaptainMessage.String
			name, err := nameFor(note.PlayerOfMatchID)
			if err != nil {
				return nil, err
			}
			view.PlayerOfMatchName = name
		}
		return view, nil
	}
	// A captain of one of this match's two teams sees edit controls for
	// only their own side — even if they're also an admin/league admin
	// (whose access would otherwise reach both sides), since being "the
	// captain" here is a stronger, narrower identity than the broader
	// manage-everything roles. Doesn't apply to a viewer who isn't captain
	// of either side, or (rare) captains both — admin/league-admin/
	// scorekeeper access then works exactly as it does everywhere else.
	isCaptainOfHome := app.isCaptainOfTeam(r, match.HomeTeamID)
	isCaptainOfAway := app.isCaptainOfTeam(r, match.AwayTeamID)
	suppressHomeControls := isCaptainOfAway && !isCaptainOfHome
	suppressAwayControls := isCaptainOfHome && !isCaptainOfAway

	homeNote, err := buildNoteView(match.HomeTeamID, homeRoster,
		app.canManageMatchSide(r, match.HomeTeamID) && !suppressHomeControls,
		(app.isAdmin(r) || isCaptainOfHome) && !suppressHomeControls)
	if err != nil {
		return nil, err
	}
	awayNote, err := buildNoteView(match.AwayTeamID, awayRoster,
		app.canManageMatchSide(r, match.AwayTeamID) && !suppressAwayControls,
		(app.isAdmin(r) || isCaptainOfAway) && !suppressAwayControls)
	if err != nil {
		return nil, err
	}

	return &matchViewData{
		Match:           match,
		Season:          season,
		League:          league,
		HomeTeam:        homeTeam,
		AwayTeam:        awayTeam,
		Location:        location,
		LocationAddress: locationAddress,
		HomeGoals:       homeGoals,
		AwayGoals:       awayGoals,
		HomeCards:       homeCards,
		AwayCards:       awayCards,
		CanManage:       canManage,
		CanRSVP:         canRSVP,
		IsPast:          isPast,
		HomeRSVPsIn:     buildRSVPRows(homeRoster, rsvpsByPlayer, "yes"),
		AwayRSVPsIn:     buildRSVPRows(awayRoster, rsvpsByPlayer, "yes"),
		HomeRSVPsOut:    buildRSVPRows(homeRoster, rsvpsByPlayer, "no"),
		AwayRSVPsOut:    buildRSVPRows(awayRoster, rsvpsByPlayer, "no"),
		HomeNote:        homeNote,
		AwayNote:        awayNote,
		ShowHomeBox:     showHomeBox,
		ShowAwayBox:     showAwayBox,
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

// matchTeamNoteSubmit saves one team's own Player of the Match pick and
// captain's notes for a match. Unlike matchUpdatePost (either team's
// manager may edit the shared score/goals/cards), this is scoped to the
// team named in the submitted teamID field — canManageMatchSide only knows
// how to check "does this user manage this team" in isolation, so teamID is
// verified against the match's own home/away teams first, before that check
// even runs.
func (app *application) matchTeamNoteSubmit(w http.ResponseWriter, r *http.Request) {
	match, ok := app.getRouteMatch(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	teamID, err := strconv.Atoi(r.PostForm.Get("teamID"))
	if err != nil || (teamID != match.HomeTeamID && teamID != match.AwayTeamID) {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if !app.canManageMatchSide(r, teamID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	playerOfMatchID, _ := strconv.Atoi(r.PostForm.Get("playerOfMatchID"))
	form := &services.MatchTeamNoteForm{
		PlayerOfMatchID: playerOfMatchID,
		Notes:           r.PostForm.Get("notes"),
		CaptainMessage:  r.PostForm.Get("captainMessage"),
	}

	err = app.matchTeamNoteService.SaveNote(match.ID, teamID, form)
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			viewData, buildErr := app.buildMatchViewData(r, match)
			if buildErr != nil {
				app.serverError(w, buildErr)
				return
			}
			if teamID == match.HomeTeamID {
				viewData.HomeNote.Form = form
			} else {
				viewData.AwayNote.Form = form
			}
			app.renderMatchView(w, r, viewData, &services.RSVPForm{}, http.StatusUnprocessableEntity)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Notes saved.")
	http.Redirect(w, r, fmt.Sprintf("/match/%d", match.ID), http.StatusSeeOther)
}

// matchTestReminderSubmit sends an on-demand preview of the RSVP reminder
// email for one team's side of a match, to addresses the caller chooses —
// either typed in directly or picked from that team's roster members who
// already have an activated account (mirroring the invite screen's
// "type an email, or pick from the roster" pattern — see team-invite.html
// and teamInviteFromRoster). A temporary validation tool while the
// reminder feature is new: visible only to that team's captain, or an
// admin — deliberately narrower than canManageMatchSide (no scorekeepers)
// since this bypasses the real send schedule and shouldn't be casually
// spammable. Records nothing — see MatchReminderService.SendTestReminder.
func (app *application) matchTestReminderSubmit(w http.ResponseWriter, r *http.Request) {
	match, ok := app.getRouteMatch(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	teamID, err := strconv.Atoi(r.PostForm.Get("teamID"))
	if err != nil || (teamID != match.HomeTeamID && teamID != match.AwayTeamID) {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if !app.isAdmin(r) && !app.isCaptainOfTeam(r, teamID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	seen := map[string]bool{}
	var addresses []string
	addAddress := func(addr string) {
		addr = strings.TrimSpace(addr)
		if addr == "" || seen[addr] || !validator.ValidEmail(addr) {
			return
		}
		seen[addr] = true
		addresses = append(addresses, addr)
	}

	for _, field := range strings.FieldsFunc(r.PostForm.Get("emails"), func(c rune) bool { return c == ',' || unicode.IsSpace(c) }) {
		addAddress(field)
	}

	if playerIDStrs := r.PostForm["playerIDs"]; len(playerIDStrs) > 0 {
		playerIDs := make([]int, 0, len(playerIDStrs))
		for _, s := range playerIDStrs {
			if id, err := strconv.Atoi(s); err == nil {
				playerIDs = append(playerIDs, id)
			}
		}
		um := &models.UserModel{DB: app.playerService.DB}
		activatedEmails, err := um.GetActivatedEmailsForPlayers(playerIDs)
		if err != nil {
			app.serverError(w, err)
			return
		}
		for _, email := range activatedEmails {
			addAddress(email)
		}
	}

	if len(addresses) == 0 {
		app.sessionManager.Put(r.Context(), "flash", "Enter at least one valid email address or select a teammate.")
		http.Redirect(w, r, fmt.Sprintf("/match/%d", match.ID), http.StatusSeeOther)
		return
	}

	if err := app.matchReminderService.SendTestReminder(match.ID, teamID, addresses); err != nil {
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", fmt.Sprintf("Test reminder sent to %d address(es).", len(addresses)))
	http.Redirect(w, r, fmt.Sprintf("/match/%d", match.ID), http.StatusSeeOther)
}

// matchTestReminderSMSSubmit is matchTestReminderSubmit's SMS counterpart
// — same admin-or-that-team's-captain gate, but with no free-text option
// at all: playerIDs must come from the VerifiedPhoneTeammates picker
// (SendTestReminderSMS re-checks roster membership and verification
// itself regardless of what's posted here, so this handler doesn't need
// to duplicate that check).
func (app *application) matchTestReminderSMSSubmit(w http.ResponseWriter, r *http.Request) {
	match, ok := app.getRouteMatch(w, r)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	teamID, err := strconv.Atoi(r.PostForm.Get("teamID"))
	if err != nil || (teamID != match.HomeTeamID && teamID != match.AwayTeamID) {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if !app.isAdmin(r) && !app.isCaptainOfTeam(r, teamID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var playerIDs []int
	for _, s := range r.PostForm["smsPlayerIDs"] {
		if id, err := strconv.Atoi(s); err == nil {
			playerIDs = append(playerIDs, id)
		}
	}

	if len(playerIDs) == 0 {
		app.sessionManager.Put(r.Context(), "flash", "Select at least one teammate with a verified phone number.")
		http.Redirect(w, r, fmt.Sprintf("/match/%d", match.ID), http.StatusSeeOther)
		return
	}

	sent, err := app.matchReminderService.SendTestReminderSMS(match.ID, teamID, playerIDs)
	if err != nil {
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", fmt.Sprintf("Test text sent to %d teammate(s).", sent))
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
	goalMinutes := r.PostForm["goalMinute"]
	goals := make([]services.GoalInput, 0, len(goalTeamIDs))
	for i, teamIDStr := range goalTeamIDs {
		teamID, err := strconv.Atoi(teamIDStr)
		if err != nil {
			continue
		}
		scorerID, _ := strconv.Atoi(valueAt(goalScorerIDs, i))
		assisterID, _ := strconv.Atoi(valueAt(goalAssisterIDs, i))
		minute, _ := strconv.Atoi(valueAt(goalMinutes, i))
		goals = append(goals, services.GoalInput{TeamID: teamID, ScorerPlayerID: scorerID, AssisterPlayerID: assisterID, Minute: minute})
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

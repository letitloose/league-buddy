package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

// homeTeamCard is one entry in the home page's "My Teams" area.
// NextMatch/NextMatchOpponent power the separate "Upcoming Matches"
// section — NextMatch is nil when the team has no scheduled match on or
// after today.
type homeTeamCard struct {
	Team                     *models.Team
	League                   *models.League
	CaptainName              string
	NextMatch                *models.Match
	NextMatchOpponent        string
	NextMatchIsHome          bool
	NextMatchLocation        *models.Location
	NextMatchLocationAddress *models.Address
}

// homeLeagueCard is one entry in the home page's "My Leagues" area — shown
// to system admins (every league) and league admins (just the leagues they
// administer).
type homeLeagueCard struct {
	League    *models.League
	TeamCount int
}

// homeData is the home page's role-dependent content: Leagues (system
// admins see every league, league admins see just the ones they administer),
// Teams (any team the player is a member of, regardless of role), and
// whether to show the system-admin-only quick links. A player who is both a
// league admin and a team member sees both sections.
type homeData struct {
	Leagues          []*homeLeagueCard
	Teams            []*homeTeamCard
	HasUpcomingMatch bool
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.notFound(w)
		return
	}

	data := app.newTemplateData(r)

	if app.isActive(r) {
		hd := &homeData{}

		for _, teamID := range app.getTeamIDs(r) {
			card, err := app.buildHomeTeamCard(teamID)
			if err != nil {
				app.serverError(w, err)
				return
			}
			hd.Teams = append(hd.Teams, card)
			if card.NextMatch != nil {
				hd.HasUpcomingMatch = true
			}
		}

		var leagues []*models.League
		var err error
		if app.isAdmin(r) {
			leagues, err = (&models.LeagueModel{DB: app.playerService.DB}).List()
		} else if playerID := app.getPlayerID(r); playerID > 0 {
			leagues, err = (&models.LeagueAdminModel{DB: app.playerService.DB}).GetLeaguesForPlayer(playerID)
		}
		if err != nil {
			app.serverError(w, err)
			return
		}
		for _, league := range leagues {
			card, err := app.buildHomeLeagueCard(league)
			if err != nil {
				app.serverError(w, err)
				return
			}
			hd.Leagues = append(hd.Leagues, card)
		}

		data.Data = hd
	}

	app.render(w, http.StatusOK, "home.html", data)
}

// buildHomeLeagueCard loads the display data for one "My Leagues" card: the
// league and how many teams it has.
func (app *application) buildHomeLeagueCard(league *models.League) (*homeLeagueCard, error) {
	tm := &models.TeamModel{DB: app.playerService.DB}
	teams, err := tm.GetByLeague(league.ID)
	if err != nil {
		return nil, err
	}

	return &homeLeagueCard{League: league, TeamCount: len(teams)}, nil
}

// buildHomeTeamCard loads the display data for one "My Teams" card: the
// team, its league, and the captain's name (blank if unassigned).
func (app *application) buildHomeTeamCard(teamID int) (*homeTeamCard, error) {
	tm := &models.TeamModel{DB: app.playerService.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		return nil, err
	}

	lm := &models.LeagueModel{DB: app.playerService.DB}
	league, err := lm.Get(team.LeagueID)
	if err != nil {
		return nil, err
	}

	captainName := ""
	if team.CaptainPlayerID.Valid {
		pm := &models.PlayerModel{DB: app.playerService.DB}
		captain, err := pm.Get(int(team.CaptainPlayerID.Int32))
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return nil, err
		}
		if captain != nil {
			captainName = captain.FirstName + " " + captain.LastName
		}
	}

	card := &homeTeamCard{
		Team:        team,
		League:      league,
		CaptainName: captainName,
	}

	mm := &models.MatchModel{DB: app.playerService.DB}
	next, err := mm.NextMatchForTeam(teamID, time.Now())
	if err != nil {
		if !errors.Is(err, models.ErrNoRecord) {
			return nil, err
		}
		return card, nil
	}

	isHome := next.HomeTeamID == teamID
	opponentID := next.HomeTeamID
	if isHome {
		opponentID = next.AwayTeamID
	}
	opponent, err := tm.Get(opponentID)
	if err != nil {
		return nil, err
	}

	card.NextMatch = next
	card.NextMatchOpponent = opponent.Name
	card.NextMatchIsHome = isHome

	if next.LocationID.Valid {
		locm := &models.LocationModel{DB: app.playerService.DB}
		location, err := locm.Get(int(next.LocationID.Int32))
		if err != nil {
			return nil, err
		}
		am := &models.AddressModel{DB: app.playerService.DB}
		address, err := am.Get(location.AddressID)
		if err != nil {
			return nil, err
		}
		card.NextMatchLocation = location
		card.NextMatchLocationAddress = address
	}

	return card, nil
}

package main

import (
	"errors"
	"net/http"

	"github.com/letitloose/league-buddy/internal/models"
)

// homeTeamCard is one entry in the home page's "My Teams" area.
type homeTeamCard struct {
	Team        *models.Team
	League      *models.League
	CaptainName string
	RosterCount int
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.notFound(w)
		return
	}

	data := app.newTemplateData(r)

	if app.isActive(r) {
		teamIDs := app.getTeamIDs(r)
		cards := make([]*homeTeamCard, 0, len(teamIDs))
		for _, teamID := range teamIDs {
			card, err := app.buildHomeTeamCard(teamID)
			if err != nil {
				app.serverError(w, err)
				return
			}
			cards = append(cards, card)
		}
		if len(cards) > 0 {
			data.Data = cards
		}
	}

	app.render(w, http.StatusOK, "home.html", data)
}

// buildHomeTeamCard loads the display data for one "My Teams" card: the
// team, its league, the captain's name (blank if unassigned), and roster
// size.
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

	roster, err := app.playerService.GetByTeam(teamID)
	if err != nil {
		return nil, err
	}

	return &homeTeamCard{
		Team:        team,
		League:      league,
		CaptainName: captainName,
		RosterCount: len(roster),
	}, nil
}

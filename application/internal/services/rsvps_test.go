package services

import (
	"testing"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestSubmitRSVPRejectsInvalidStatus(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	rm := &models.RSVPModel{DB: db}
	rsvpService := RSVPService{RSVPModel: rm}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	form := &RSVPForm{Status: "maybe"}
	err = rsvpService.SubmitRSVP(matchID, 1, 1, form)
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
	if form.FieldErrors["status"] == "" {
		t.Fatal("expected a field error on status")
	}
}

func TestSubmitRSVPRejectsOverlongMessage(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	rm := &models.RSVPModel{DB: db}
	rsvpService := RSVPService{RSVPModel: rm}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	longMessage := ""
	for i := 0; i < 256; i++ {
		longMessage += "a"
	}

	form := &RSVPForm{Status: "yes", Message: longMessage}
	err = rsvpService.SubmitRSVP(matchID, 1, 1, form)
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
	if form.FieldErrors["message"] == "" {
		t.Fatal("expected a field error on message")
	}
}

func TestSubmitRSVPRoundTripAndResubmit(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	rm := &models.RSVPModel{DB: db}
	rsvpService := RSVPService{RSVPModel: rm}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&models.Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}

	if err := rsvpService.SubmitRSVP(matchID, playerID, 1, &RSVPForm{Status: "yes", Message: "count me in"}); err != nil {
		t.Fatal(err)
	}
	if err := rsvpService.SubmitRSVP(matchID, playerID, 1, &RSVPForm{Status: "no", Message: "can't make it anymore"}); err != nil {
		t.Fatal(err)
	}

	rsvps, err := rm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rsvps) != 1 {
		t.Fatalf("expected resubmitting to update the same row, got %d rows", len(rsvps))
	}
	if rsvps[0].Status != "no" || rsvps[0].Message.String != "can't make it anymore" {
		t.Fatalf("expected the latest response to have won, got %+v", rsvps[0])
	}
}

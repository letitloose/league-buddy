package services

import (
	"testing"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestSaveNoteRejectsOverlongNotes(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	mtnm := &models.MatchTeamNoteModel{DB: db}
	noteService := MatchTeamNoteService{MatchTeamNoteModel: mtnm, DB: db}

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

	longNotes := ""
	for i := 0; i < 2001; i++ {
		longNotes += "a"
	}

	form := &MatchTeamNoteForm{Notes: longNotes}
	err = noteService.SaveNote(matchID, 1, form)
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
	if form.FieldErrors["notes"] == "" {
		t.Fatal("expected a field error on notes")
	}
}

func TestSaveNoteRejectsOverlongCaptainMessage(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	mtnm := &models.MatchTeamNoteModel{DB: db}
	noteService := MatchTeamNoteService{MatchTeamNoteModel: mtnm, DB: db}

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
	for i := 0; i < 2001; i++ {
		longMessage += "a"
	}

	form := &MatchTeamNoteForm{CaptainMessage: longMessage}
	err = noteService.SaveNote(matchID, 1, form)
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
	if form.FieldErrors["captainMessage"] == "" {
		t.Fatal("expected a field error on captainMessage")
	}
}

func TestSaveNoteRejectsPlayerOfMatchNotOnRoster(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	mtnm := &models.MatchTeamNoteModel{DB: db}
	noteService := MatchTeamNoteService{MatchTeamNoteModel: mtnm, DB: db}

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
	// A player who is never added to team 1's roster.
	playerID, err := pm.Insert(&models.Player{FirstName: "Not", LastName: "Rostered"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchTeamNoteForm{PlayerOfMatchID: playerID}
	err = noteService.SaveNote(matchID, 1, form)
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
	if form.FieldErrors["playerOfMatchID"] == "" {
		t.Fatal("expected a field error on playerOfMatchID")
	}
}

func TestSaveNoteRoundTripAndResubmit(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	mtnm := &models.MatchTeamNoteModel{DB: db}
	noteService := MatchTeamNoteService{MatchTeamNoteModel: mtnm, DB: db}

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
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	if err := noteService.SaveNote(matchID, 1, &MatchTeamNoteForm{PlayerOfMatchID: playerID, Notes: "Sam ran the show", CaptainMessage: "Meet at 6:15"}); err != nil {
		t.Fatal(err)
	}
	if err := noteService.SaveNote(matchID, 1, &MatchTeamNoteForm{Notes: "actually a team effort", CaptainMessage: "Meet at 6:00 instead"}); err != nil {
		t.Fatal(err)
	}

	note, err := mtnm.GetByMatchAndTeam(matchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if note.PlayerOfMatchID.Valid {
		t.Fatalf("expected the resubmission to clear the Player of the Match pick, got %+v", note.PlayerOfMatchID)
	}
	if note.CaptainMessage.String != "Meet at 6:00 instead" {
		t.Fatalf("expected the latest captainMessage to have won, got %+v", note.CaptainMessage)
	}
	if note.Notes.String != "actually a team effort" {
		t.Fatalf("expected the latest notes to have won, got %+v", note.Notes)
	}
}

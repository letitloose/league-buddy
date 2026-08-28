package services

import (
	"database/sql"
	"testing"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestSendDueRSVPRemindersOnlyEmailsNonResponders(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	rm := &models.RSVPModel{DB: db}
	mrrm := &models.MatchRSVPReminderModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	asOf := time.Now()
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, 3)})
	if err != nil {
		t.Fatal(err)
	}

	responderID, err := pm.Insert(&models.Player{FirstName: "Responder", LastName: "One", Email: sql.NullString{String: "responder@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	nonResponderID, err := pm.Insert(&models.Player{FirstName: "Non", LastName: "Responder", Email: sql.NullString{String: "nonresponder@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	noEmailID, err := pm.Insert(&models.Player{FirstName: "No", LastName: "Email"})
	if err != nil {
		t.Fatal(err)
	}
	for _, playerID := range []int{responderID, nonResponderID, noEmailID} {
		if err := tmm.AddMembership(playerID, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := rm.Upsert(&models.RSVP{MatchID: matchID, PlayerID: responderID, TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	sent, err := reminderService.SendDueRSVPReminders(asOf)
	if err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatalf("expected 1 reminder sent (only the non-responder with an email), got %d", sent)
	}

	wasSentToNonResponder, err := mrrm.WasSent(matchID, nonResponderID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSentToNonResponder {
		t.Fatal("expected the non-responder to be marked reminded")
	}
	wasSentToResponder, err := mrrm.WasSent(matchID, responderID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if wasSentToResponder {
		t.Fatal("expected the responder to never be reminded (they already RSVP'd)")
	}
	wasSentToNoEmail, err := mrrm.WasSent(matchID, noEmailID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if wasSentToNoEmail {
		t.Fatal("expected the player with no email to be skipped, not marked reminded")
	}

	// Re-running the same asOf is idempotent — nothing more to send.
	sentAgain, err := reminderService.SendDueRSVPReminders(asOf)
	if err != nil {
		t.Fatal(err)
	}
	if sentAgain != 0 {
		t.Fatalf("expected 0 on a second run for the same day, got %d", sentAgain)
	}
}

func TestSendDueRSVPRemindersSkipsMatchesOutsideTheSchedule(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	asOf := time.Now()
	// 4 days out isn't part of the RSVP schedule (3/2/1 only).
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, 4)}); err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&models.Player{FirstName: "Some", LastName: "Player", Email: sql.NullString{String: "player@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	sent, err := reminderService.SendDueRSVPReminders(asOf)
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("expected 0 reminders for a match outside the 3/2/1-day schedule, got %d", sent)
	}
}

func TestSendDueCaptainMessageReminders(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	mcrm := &models.MatchCaptainMessageReminderModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	// Team 1 (the seeded team) gets a captain below; opponent has none.
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	asOf := time.Now()
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, captainMessageDaysOut)})
	if err != nil {
		t.Fatal(err)
	}

	captainID, err := pm.Insert(&models.Player{FirstName: "Cap", LastName: "Tain", Email: sql.NullString{String: "captain@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(captainID, 1); err != nil {
		t.Fatal(err)
	}
	homeTeam := &models.TeamModel{DB: db}
	if err := homeTeam.SetCaptain(1, sql.NullInt32{Int32: int32(captainID), Valid: true}); err != nil {
		t.Fatal(err)
	}

	sent, err := reminderService.SendDueCaptainMessageReminders(asOf)
	if err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatalf("expected 1 reminder sent (home team's captain; away team has no captain), got %d", sent)
	}

	wasSentHome, err := mcrm.WasSent(matchID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSentHome {
		t.Fatal("expected the home team's reminder to be marked sent")
	}
	wasSentAway, err := mcrm.WasSent(matchID, opponentID)
	if err != nil {
		t.Fatal(err)
	}
	if wasSentAway {
		t.Fatal("expected no reminder for the away team, which has no captain")
	}

	// Re-running is idempotent.
	sentAgain, err := reminderService.SendDueCaptainMessageReminders(asOf)
	if err != nil {
		t.Fatal(err)
	}
	if sentAgain != 0 {
		t.Fatalf("expected 0 on a second run, got %d", sentAgain)
	}
}

func TestSendDueCaptainMessageRemindersSkipsWhenMessageAlreadySet(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	mtnm := &models.MatchTeamNoteModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	asOf := time.Now()
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, captainMessageDaysOut)})
	if err != nil {
		t.Fatal(err)
	}

	captainID, err := pm.Insert(&models.Player{FirstName: "Cap", LastName: "Tain", Email: sql.NullString{String: "captain@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(captainID, 1); err != nil {
		t.Fatal(err)
	}
	homeTeam := &models.TeamModel{DB: db}
	if err := homeTeam.SetCaptain(1, sql.NullInt32{Int32: int32(captainID), Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := mtnm.Upsert(&models.MatchTeamNote{MatchID: matchID, TeamID: 1, CaptainMessage: sql.NullString{String: "Already set this early", Valid: true}, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	sent, err := reminderService.SendDueCaptainMessageReminders(asOf)
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("expected 0 reminders when the captain's message is already set, got %d", sent)
	}
}

// SendTestReminder is the on-demand preview a captain triggers from the
// match screen — unlike the real scheduled sends, it must work for a match
// far outside the 3/2/1-day RSVP window and must never touch
// matchRSVPReminders, since it isn't a real scheduled send.
func TestSendTestReminderIgnoresScheduleAndRecordsNothing(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	mrrm := &models.MatchRSVPReminderModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	// 30 days out — nowhere near the real 3/2/1-day RSVP schedule.
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now().AddDate(0, 0, 30)})
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

	if err := reminderService.SendTestReminder(matchID, 1, []string{"tester@example.com"}); err != nil {
		t.Fatal(err)
	}

	wasSent, err := mrrm.WasSent(matchID, playerID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if wasSent {
		t.Fatal("expected SendTestReminder to record nothing to matchRSVPReminders")
	}
}

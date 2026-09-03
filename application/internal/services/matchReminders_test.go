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

	// Fixed, DST-uneventful, safely-after-any-default-ReminderTime moment —
	// SendDueRSVPReminders now gates on each team's own ReminderTime
	// (default 9am Eastern), so a bare time.Now() would make these tests
	// flaky depending on what time of day they happen to run.
	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
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

// verifyPlayerPhone drives a player's phone through the real
// request-code/confirm-code model methods (rather than hand-writing SQL),
// so tests exercise the same path the app actually uses to mark a phone
// verified.
func verifyPlayerPhone(t *testing.T, pm *models.PlayerModel, playerID int) {
	t.Helper()
	if err := pm.SetPhoneVerificationCode(playerID, "123456", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := pm.ConfirmPhoneVerified(playerID); err != nil {
		t.Fatal(err)
	}
}

func TestSendDueRSVPRemindersRespectsChannelPreference(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	mrrm := &models.MatchRSVPReminderModel{DB: db}
	npm := &models.NotificationPreferenceModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	// Fixed, DST-uneventful, safely-after-any-default-ReminderTime moment —
	// SendDueRSVPReminders now gates on each team's own ReminderTime
	// (default 9am Eastern), so a bare time.Now() would make these tests
	// flaky depending on what time of day they happen to run.
	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, 3)})
	if err != nil {
		t.Fatal(err)
	}

	// Verified phone, SMS-only preference, no email at all — still counts
	// as sent (proves SMS alone can deliver without falling back).
	smsOnlyID, err := pm.Insert(&models.Player{FirstName: "Sms", LastName: "Only", PhoneNumber: sql.NullString{String: "518-555-0100", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	verifyPlayerPhone(t, pm, smsOnlyID)
	if err := npm.SetChannel(smsOnlyID, models.CategoryRSVPReminder, models.ChannelSMS); err != nil {
		t.Fatal(err)
	}

	// SMS preference but phone never verified — must fall back to email,
	// not be silently dropped.
	unverifiedID, err := pm.Insert(&models.Player{FirstName: "Unverified", LastName: "Phone", Email: sql.NullString{String: "unverified@example.com", Valid: true}, PhoneNumber: sql.NullString{String: "518-555-0101", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := npm.SetChannel(unverifiedID, models.CategoryRSVPReminder, models.ChannelSMS); err != nil {
		t.Fatal(err)
	}

	// Explicit "off" — must not be counted or recorded as sent at all.
	offID, err := pm.Insert(&models.Player{FirstName: "Opted", LastName: "Out", Email: sql.NullString{String: "opted-out@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := npm.SetChannel(offID, models.CategoryRSVPReminder, models.ChannelOff); err != nil {
		t.Fatal(err)
	}

	for _, playerID := range []int{smsOnlyID, unverifiedID, offID} {
		if err := tmm.AddMembership(playerID, 1); err != nil {
			t.Fatal(err)
		}
	}

	sent, err := reminderService.SendDueRSVPReminders(asOf)
	if err != nil {
		t.Fatal(err)
	}
	if sent != 2 {
		t.Fatalf("expected 2 reminders sent (sms-only and the unverified-fallback-to-email player, not the opted-out one), got %d", sent)
	}

	wasSentSMSOnly, err := mrrm.WasSent(matchID, smsOnlyID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSentSMSOnly {
		t.Fatal("expected the verified SMS-only player to be marked reminded")
	}
	wasSentUnverified, err := mrrm.WasSent(matchID, unverifiedID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSentUnverified {
		t.Fatal("expected the unverified-but-has-email player to be marked reminded via the email fallback")
	}
	wasSentOff, err := mrrm.WasSent(matchID, offID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if wasSentOff {
		t.Fatal("expected the opted-out player to never be marked reminded")
	}
}

func TestSendTestReminderSMSOnlyTextsVerifiedRosterMembers(t *testing.T) {
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
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now().AddDate(0, 0, 3)})
	if err != nil {
		t.Fatal(err)
	}

	verifiedID, err := pm.Insert(&models.Player{FirstName: "Verified", LastName: "Teammate", PhoneNumber: sql.NullString{String: "518-555-0100", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	verifyPlayerPhone(t, pm, verifiedID)
	if err := tmm.AddMembership(verifiedID, 1); err != nil {
		t.Fatal(err)
	}

	unverifiedID, err := pm.Insert(&models.Player{FirstName: "Unverified", LastName: "Teammate", PhoneNumber: sql.NullString{String: "518-555-0101", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(unverifiedID, 1); err != nil {
		t.Fatal(err)
	}

	notOnRosterID, err := pm.Insert(&models.Player{FirstName: "Not", LastName: "OnRoster", PhoneNumber: sql.NullString{String: "518-555-0102", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	verifyPlayerPhone(t, pm, notOnRosterID)

	sent, err := reminderService.SendTestReminderSMS(matchID, 1, []int{verifiedID, unverifiedID, notOnRosterID})
	if err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatalf("expected exactly 1 test text sent (only the verified roster member), got %d", sent)
	}
}

// A match scheduled late in the Eastern evening must still be found on
// its own Eastern calendar day — proves dateNDaysOut/GetByDate's range
// query end to end through the real reminder pipeline, not just the
// model-level range logic (see TestGetByDate in internal/models).
func TestSendDueRSVPRemindersFindsLateEveningMatch(t *testing.T) {
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

	// A fixed, DST-uneventful date so the test is deterministic.
	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
	lateEvening := dateNDaysOut(asOf, 3).Add(23*time.Hour + 30*time.Minute)
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: lateEvening})
	if err != nil {
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
	if sent != 1 {
		t.Fatalf("expected the 11:30pm Eastern match to still be found 3 days out, got %d sent", sent)
	}

	wasSent, err := mrrm.WasSent(matchID, playerID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSent {
		t.Fatal("expected the late-evening match's reminder to be recorded")
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

	// Fixed, DST-uneventful, safely-after-any-default-ReminderTime moment —
	// SendDueRSVPReminders now gates on each team's own ReminderTime
	// (default 9am Eastern), so a bare time.Now() would make these tests
	// flaky depending on what time of day they happen to run.
	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
	// 4 days out exceeds team 1's default ReminderDaysOut of 3.
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
		t.Fatalf("expected 0 reminders for a match beyond the team's configured reminder cascade, got %d", sent)
	}
}

// A team can opt out of RSVP reminders entirely via
// models.Team.RemindersEnabled (set on the team's edit page) — proves the
// opt-out actually suppresses a reminder that would otherwise be due.
func TestSendDueRSVPRemindersRespectsPerTeamOptOut(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	mrrm := &models.MatchRSVPReminderModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	team, err := teams.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	team.RemindersEnabled = false
	if err := teams.Update(team); err != nil {
		t.Fatal(err)
	}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, 3)})
	if err != nil {
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
		t.Fatalf("expected 0 reminders for an opted-out team, got %d", sent)
	}
	wasSent, err := mrrm.WasSent(matchID, playerID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if wasSent {
		t.Fatal("expected the opted-out team's player to never be marked reminded")
	}
}

// A team's ReminderDaysOut shortens or lengthens the cascade — proves a
// team configured for 1 day out doesn't get reminded 3 days out (the old
// global default), while a match 1 day out for that same team still goes
// through.
func TestSendDueRSVPRemindersRespectsPerTeamDaysOut(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	mrrm := &models.MatchRSVPReminderModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	team, err := teams.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	team.ReminderDaysOut = 1
	if err := teams.Update(team); err != nil {
		t.Fatal(err)
	}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
	tooEarlyMatchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, 3)})
	if err != nil {
		t.Fatal(err)
	}
	dueMatchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, 1)})
	if err != nil {
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
	if sent != 1 {
		t.Fatalf("expected exactly 1 reminder (the 1-day-out match only), got %d", sent)
	}
	if wasSent, _ := mrrm.WasSent(tooEarlyMatchID, playerID, 3); wasSent {
		t.Fatal("expected the 3-day-out match to be skipped — beyond the team's configured 1-day cascade")
	}
	if wasSent, _ := mrrm.WasSent(dueMatchID, playerID, 1); !wasSent {
		t.Fatal("expected the 1-day-out match to be reminded")
	}
}

// A team's ReminderTime gates *when* today a due reminder actually goes
// out — proves a team configured for a later time isn't reminded before
// that time, but is once asOf reaches it.
func TestSendDueRSVPRemindersRespectsPerTeamReminderTime(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	mrrm := &models.MatchRSVPReminderModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	team, err := teams.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	team.ReminderTime = "17:00:00"
	if err := teams.Update(team); err != nil {
		t.Fatal(err)
	}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	before := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(before, 3)})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&models.Player{FirstName: "Some", LastName: "Player", Email: sql.NullString{String: "player@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	sentBefore, err := reminderService.SendDueRSVPReminders(before)
	if err != nil {
		t.Fatal(err)
	}
	if sentBefore != 0 {
		t.Fatalf("expected 0 reminders before the team's configured 5pm reminder time, got %d", sentBefore)
	}

	after := time.Date(2024, 7, 1, 17, 30, 0, 0, easternLocation)
	sentAfter, err := reminderService.SendDueRSVPReminders(after)
	if err != nil {
		t.Fatal(err)
	}
	if sentAfter != 1 {
		t.Fatalf("expected 1 reminder once asOf reaches the team's configured time, got %d", sentAfter)
	}
	if wasSent, _ := mrrm.WasSent(matchID, playerID, 3); !wasSent {
		t.Fatal("expected the match to be marked reminded once sent")
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

	// Fixed, DST-uneventful, safely-after-any-default-ReminderTime moment —
	// SendDueRSVPReminders now gates on each team's own ReminderTime
	// (default 9am Eastern), so a bare time.Now() would make these tests
	// flaky depending on what time of day they happen to run.
	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
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

	// Fixed, DST-uneventful, safely-after-any-default-ReminderTime moment —
	// SendDueRSVPReminders now gates on each team's own ReminderTime
	// (default 9am Eastern), so a bare time.Now() would make these tests
	// flaky depending on what time of day they happen to run.
	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
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

// A team that's opted out of RSVP reminders altogether (RemindersEnabled
// false) shouldn't get nudged to write a message for reminders that will
// never go out.
func TestSendDueCaptainMessageRemindersSkipsWhenTeamOptedOut(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	mm := &models.MatchModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	reminderService := MatchReminderService{DB: db}

	team, err := teams.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	team.RemindersEnabled = false
	if err := teams.Update(team); err != nil {
		t.Fatal(err)
	}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	asOf := time.Date(2024, 7, 1, 12, 0, 0, 0, easternLocation)
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: dateNDaysOut(asOf, captainMessageDaysOut)}); err != nil {
		t.Fatal(err)
	}

	captainID, err := pm.Insert(&models.Player{FirstName: "Cap", LastName: "Tain", Email: sql.NullString{String: "captain@example.com", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(captainID, 1); err != nil {
		t.Fatal(err)
	}
	if err := teams.SetCaptain(1, sql.NullInt32{Int32: int32(captainID), Valid: true}); err != nil {
		t.Fatal(err)
	}

	sent, err := reminderService.SendDueCaptainMessageReminders(asOf)
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("expected 0 reminders for an opted-out team, got %d", sent)
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

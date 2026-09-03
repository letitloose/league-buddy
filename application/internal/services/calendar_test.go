package services

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

// newCalendarFixtures sets up a league with two teams and a player who's
// an active member of both, plus a season to hang matches off of.
func newCalendarFixtures(t *testing.T, db *sql.DB) (playerID, teamAID, teamBID, seasonID int) {
	t.Helper()

	lm := &models.LeagueModel{DB: db}
	leagueID, err := lm.Insert(&models.League{Name: "Test League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: db}
	teamAID, err = tm.Insert(&models.Team{LeagueID: leagueID, Name: "Team A"})
	if err != nil {
		t.Fatal(err)
	}
	teamBID, err = tm.Insert(&models.Team{LeagueID: leagueID, Name: "Team B"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: db}
	seasonID, err = sm.Insert(&models.Season{LeagueID: leagueID, Name: "Test Season"})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: db}
	playerID, err = pm.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	tmm := &models.TeamMemberModel{DB: db}
	if err := tmm.AddMembership(playerID, teamAID); err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, teamBID); err != nil {
		t.Fatal(err)
	}

	return playerID, teamAID, teamBID, seasonID
}

func TestEnsureAndRegenerateCalendarToken(t *testing.T) {
	db := models.NewTestDB(t)
	playerID, _, _, _ := newCalendarFixtures(t, db)
	service := &CalendarService{DB: db}

	token1, err := service.EnsureToken(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if token1 == "" {
		t.Fatal("expected a non-empty token")
	}

	token2, err := service.EnsureToken(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if token1 != token2 {
		t.Fatalf("expected EnsureToken to be idempotent, got %q then %q", token1, token2)
	}

	token3, err := service.RegenerateToken(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if token3 == token1 {
		t.Fatal("expected RegenerateToken to issue a different token")
	}

	pm := &models.PlayerModel{DB: db}
	if _, err := pm.GetByCalendarToken(token1); err != models.ErrNoRecord {
		t.Fatalf("expected the old token to no longer resolve, got %v", err)
	}
}

func TestBuildFeedUnknownToken(t *testing.T) {
	db := models.NewTestDB(t)
	service := &CalendarService{DB: db}

	_, err := service.BuildFeed("no-such-token")
	if err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestBuildFeedNoActiveTeams(t *testing.T) {
	db := models.NewTestDB(t)
	pm := &models.PlayerModel{DB: db}
	playerID, err := pm.Insert(&models.Player{FirstName: "Solo", LastName: "Player"})
	if err != nil {
		t.Fatal(err)
	}
	service := &CalendarService{DB: db}

	token, err := service.EnsureToken(playerID)
	if err != nil {
		t.Fatal(err)
	}
	feed, err := service.BuildFeed(token)
	if err != nil {
		t.Fatal(err)
	}
	body := string(feed)
	if !strings.Contains(body, "BEGIN:VCALENDAR") || !strings.Contains(body, "END:VCALENDAR") {
		t.Fatalf("expected a valid (if empty) calendar, got:\n%s", body)
	}
	if strings.Contains(body, "BEGIN:VEVENT") {
		t.Fatalf("expected no events for a player on no active team, got:\n%s", body)
	}
}

func TestBuildFeedAcrossTeamsAndTimedVsAllDay(t *testing.T) {
	db := models.NewTestDB(t)
	playerID, teamAID, teamBID, seasonID := newCalendarFixtures(t, db)

	mm := &models.MatchModel{DB: db}
	timed := time.Date(2099, 5, 1, 14, 30, 0, 0, time.UTC)
	timedID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamAID, AwayTeamID: teamBID, MatchDate: timed})
	if err != nil {
		t.Fatal(err)
	}
	allDay := time.Date(2099, 5, 8, 0, 0, 0, 0, time.UTC)
	allDayID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamBID, AwayTeamID: teamAID, MatchDate: allDay})
	if err != nil {
		t.Fatal(err)
	}
	// A past match — must be excluded from the feed.
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamAID, AwayTeamID: teamBID, MatchDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}

	service := &CalendarService{DB: db}
	token, err := service.EnsureToken(playerID)
	if err != nil {
		t.Fatal(err)
	}
	feed, err := service.BuildFeed(token)
	if err != nil {
		t.Fatal(err)
	}
	body := string(feed)

	if strings.Count(body, "BEGIN:VEVENT") != 2 {
		t.Fatalf("expected exactly 2 VEVENTs (past match excluded), got:\n%s", body)
	}
	if !strings.Contains(body, "SUMMARY:Team A vs Team B") {
		t.Fatalf("expected a Team A vs Team B SUMMARY, got:\n%s", body)
	}
	if !strings.Contains(body, "UID:match-"+strconv.Itoa(timedID)+"@blametheball") {
		t.Fatalf("expected a stable UID for the timed match, got:\n%s", body)
	}
	if !strings.Contains(body, "DTSTART:20990501T143000Z") {
		t.Fatalf("expected a UTC timed DTSTART, got:\n%s", body)
	}
	timedEventStart := strings.Index(body, "UID:match-"+strconv.Itoa(timedID)+"@blametheball")
	timedEventEnd := strings.Index(body[timedEventStart:], "END:VEVENT") + timedEventStart
	if strings.Contains(body[timedEventStart:timedEventEnd], "DTEND") {
		t.Fatalf("expected no DTEND on the timed match's own VEVENT (no stored duration), got:\n%s", body)
	}
	if !strings.Contains(body, "UID:match-"+strconv.Itoa(allDayID)+"@blametheball") {
		t.Fatalf("expected a stable UID for the all-day match, got:\n%s", body)
	}
	if !strings.Contains(body, "DTSTART;VALUE=DATE:20990508") || !strings.Contains(body, "DTEND;VALUE=DATE:20990509") {
		t.Fatalf("expected an all-day VALUE=DATE event spanning just its own day, got:\n%s", body)
	}
}

func TestBuildFeedExcludesLegendTeams(t *testing.T) {
	db := models.NewTestDB(t)
	playerID, teamAID, teamBID, seasonID := newCalendarFixtures(t, db)

	tmm := &models.TeamMemberModel{DB: db}
	if err := tmm.SetLegendStatus(playerID, teamBID, true); err != nil {
		t.Fatal(err)
	}

	mm := &models.MatchModel{DB: db}
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamAID, AwayTeamID: teamBID, MatchDate: time.Date(2099, 5, 1, 14, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	// A match belonging only to the now-Legend team — must be excluded.
	otherTeamID := mustInsertTeam(t, db, "Team C")
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamBID, AwayTeamID: otherTeamID, MatchDate: time.Date(2099, 5, 8, 14, 30, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}

	service := &CalendarService{DB: db}
	token, err := service.EnsureToken(playerID)
	if err != nil {
		t.Fatal(err)
	}
	feed, err := service.BuildFeed(token)
	if err != nil {
		t.Fatal(err)
	}
	body := string(feed)

	// Team A vs Team B still shows (playerID is still active on Team A),
	// but the Team B vs Team C match must not, since playerID's only tie
	// to Team B is now a Legend membership.
	if strings.Count(body, "BEGIN:VEVENT") != 1 {
		t.Fatalf("expected exactly 1 VEVENT (Legend team's own match excluded), got:\n%s", body)
	}
	if strings.Contains(body, "Team C") {
		t.Fatalf("expected no matches from the Legend-only team, got:\n%s", body)
	}
}

func mustInsertTeam(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	tm := &models.TeamModel{DB: db}
	league, err := (&models.LeagueModel{DB: db}).Insert(&models.League{Name: name + " League"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := tm.Insert(&models.Team{LeagueID: league, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

package services

import (
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestCreateMatch(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{
		SeasonID:   seasonID,
		HomeTeamID: 1,
		AwayTeamID: opponentID,
		MatchDate:  "2024-05-05",
		HomeScore:  "3",
		AwayScore:  "1",
	}

	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	match, err := matches.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !match.HomeScore.Valid || match.HomeScore.Int32 != 3 {
		t.Fatalf("expected homeScore 3, got %+v", match.HomeScore)
	}
}

func TestCreateMatchSameHomeAndAwayTeamFails(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: 1, MatchDate: "2024-05-05"}
	_, err = matchService.CreateMatch(form, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestCreateMatchTeamOutsideSeasonLeagueFails(t *testing.T) {
	db := models.NewTestDB(t)

	leagues := &models.LeagueModel{DB: db}
	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	otherLeagueID, err := leagues.Insert(&models.League{Name: "Other League"})
	if err != nil {
		t.Fatal(err)
	}
	outsiderID, err := teams.Insert(&models.Team{LeagueID: otherLeagueID, Name: "Outsider FC"})
	if err != nil {
		t.Fatal(err)
	}
	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: outsiderID, MatchDate: "2024-05-05"}
	_, err = matchService.CreateMatch(form, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

// Entering only one side's score is common (a team logs their own result
// without knowing the opponent's exact number) — the blank side defaults to
// 0 rather than blocking the save.
func TestCreateMatchOneSidedScoreDefaultsOtherToZero(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05", HomeScore: "3"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	match, err := matches.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !match.HomeScore.Valid || match.HomeScore.Int32 != 3 {
		t.Fatalf("expected homeScore=3, got %+v", match.HomeScore)
	}
	if !match.AwayScore.Valid || match.AwayScore.Int32 != 0 {
		t.Fatalf("expected awayScore to default to 0, got %+v", match.AwayScore)
	}
}

func TestUpdateMatchSavesGoalsAndCards(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	players := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	matches := &models.MatchModel{DB: db}
	pmsm := &models.PlayerMatchStatModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := players.Insert(&models.Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Two goals (the second also assisted by the same player, purely to
	// exercise both tallies at once) and one yellow card, all for playerID.
	// The second goal's earlier minute means it should sort first.
	updateForm := &MatchForm{
		ID: id, SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05",
		HomeScore: "2", AwayScore: "0",
		Goals: []GoalInput{
			{TeamID: 1, ScorerPlayerID: playerID, Minute: 70},
			{TeamID: 1, ScorerPlayerID: playerID, AssisterPlayerID: playerID, Minute: 10},
		},
		Cards: []CardInput{
			{TeamID: 1, PlayerID: playerID, CardType: "yellow"},
		},
	}
	if err := matchService.UpdateMatch(updateForm, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	stats, err := pmsm.ListByMatch(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Goals != 2 || stats[0].Assists != 1 || stats[0].YellowCards != 1 {
		t.Fatalf("expected saved stat line goals=2 assists=1 yellowCards=1, got %+v", stats)
	}

	mgm := &models.MatchGoalModel{DB: db}
	goals, err := mgm.ListByMatch(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 2 {
		t.Fatalf("expected 2 goal rows saved, got %d", len(goals))
	}
	if !goals[0].Minute.Valid || goals[0].Minute.Int32 != 10 || !goals[1].Minute.Valid || goals[1].Minute.Int32 != 70 {
		t.Fatalf("expected the goals saved with their minutes and sorted earliest-first, got %+v then %+v", goals[0], goals[1])
	}
}

// A goal's minute must be a plausible number, not an obviously bad value.
func TestUpdateMatchRejectsUnreasonableMinute(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	updateForm := &MatchForm{
		ID: id, SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05",
		Goals: []GoalInput{{TeamID: 1, Minute: 999}},
	}
	err = matchService.UpdateMatch(updateForm, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

// Resubmitting a match's stats replaces the previous set rather than
// accumulating on top of it — the recomputed playerMatchStats cache must
// reflect only the latest save.
func TestUpdateMatchResubmitReplacesStats(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	players := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	matches := &models.MatchModel{DB: db}
	pmsm := &models.PlayerMatchStatModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := players.Insert(&models.Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	first := &MatchForm{
		ID: id, SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05",
		HomeScore: "2", AwayScore: "0",
		Goals: []GoalInput{{TeamID: 1, ScorerPlayerID: playerID}, {TeamID: 1, ScorerPlayerID: playerID}},
	}
	if err := matchService.UpdateMatch(first, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	second := &MatchForm{
		ID: id, SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05",
		HomeScore: "1", AwayScore: "0",
		Goals: []GoalInput{{TeamID: 1, ScorerPlayerID: playerID}},
	}
	if err := matchService.UpdateMatch(second, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	stats, err := pmsm.ListByMatch(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Goals != 1 {
		t.Fatalf("expected the resubmit to replace, leaving goals=1, got %+v", stats)
	}
}

// A team's goal rows can fall short of its recorded score (an own goal has
// no scorer to credit) but must never exceed it.
func TestUpdateMatchGoalRowsExceedingScoreFails(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	players := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	scorer1, err := players.Insert(&models.Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(scorer1, 1); err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Home team recorded a 2-0 win, but 3 goal rows are recorded for them.
	updateForm := &MatchForm{
		ID: id, SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05",
		HomeScore: "2", AwayScore: "0",
		Goals: []GoalInput{
			{TeamID: 1, ScorerPlayerID: scorer1},
			{TeamID: 1, ScorerPlayerID: scorer1},
			{TeamID: 1},
		},
	}
	err = matchService.UpdateMatch(updateForm, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

// A scorer/assister/carded player must actually belong to that row's
// team's roster.
func TestUpdateMatchRejectsPlayerNotOnRowsTeam(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	players := &models.PlayerModel{DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	// Not added to any roster.
	outsiderID, err := players.Insert(&models.Player{FirstName: "Not", LastName: "OnRoster"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	updateForm := &MatchForm{
		ID: id, SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05",
		Goals: []GoalInput{{TeamID: 1, ScorerPlayerID: outsiderID}},
	}
	err = matchService.UpdateMatch(updateForm, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestDeleteMatch(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := MatchService{MatchModel: matches, DB: db}

	seasonID, err := seasons.Insert(&models.Season{LeagueID: 1, Name: "Spring 2024"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := teams.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := matchService.DeleteMatch(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	if _, err := matches.Get(id); err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

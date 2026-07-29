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

func TestCreateMatchOneSidedScoreFails(t *testing.T) {
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
	_, err = matchService.CreateMatch(form, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestUpdateMatchSavesPlayerStats(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	teams := &models.TeamModel{DB: db}
	players := &models.PlayerModel{DB: db}
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

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	updateForm := &MatchForm{
		ID: id, SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05",
		HomeScore: "2", AwayScore: "0",
		Stats: []PlayerStatInput{{PlayerID: playerID, TeamID: 1, Goals: 2, Assists: 1}},
	}
	if err := matchService.UpdateMatch(updateForm, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	stats, err := pmsm.ListByMatch(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Goals != 2 || stats[0].Assists != 1 {
		t.Fatalf("expected saved stat line goals=2 assists=1, got %+v", stats)
	}
}

// A team's player goals can fall short of its recorded score (an own goal
// has no scorer to credit) but must never exceed it.
func TestUpdateMatchPlayerGoalsExceedingScoreFails(t *testing.T) {
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
	scorer1, err := players.Insert(&models.Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	scorer2, err := players.Insert(&models.Player{FirstName: "Ana", LastName: "Winger"})
	if err != nil {
		t.Fatal(err)
	}

	form := &MatchForm{SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05"}
	id, err := matchService.CreateMatch(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Home team recorded a 2-0 win, but two players are each credited with
	// 2 goals — 4 total, more than the team actually scored.
	updateForm := &MatchForm{
		ID: id, SeasonID: seasonID, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: "2024-05-05",
		HomeScore: "2", AwayScore: "0",
		Stats: []PlayerStatInput{
			{PlayerID: scorer1, TeamID: 1, Goals: 2},
			{PlayerID: scorer2, TeamID: 1, Goals: 2},
		},
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

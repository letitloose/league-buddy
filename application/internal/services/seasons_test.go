package services

import (
	"database/sql"
	"testing"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestCreateSeason(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	form := &SeasonForm{LeagueID: 1, Name: "Spring 2024", StartDate: "2024-05-01", EndDate: "2024-06-30"}

	id, err := seasonService.CreateSeason(form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	season, err := seasons.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if season.Name != "Spring 2024" {
		t.Fatalf("wrong name! expected Spring 2024, got %s", season.Name)
	}
	if !season.StartDate.Valid {
		t.Fatal("expected startDate to be set")
	}
}

func TestCreateSeasonMissingRequiredFields(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	_, err := seasonService.CreateSeason(&SeasonForm{}, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestUpdateSeason(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	id, err := seasonService.CreateSeason(&SeasonForm{LeagueID: 1, Name: "Old Name"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	err = seasonService.UpdateSeason(&SeasonForm{ID: id, LeagueID: 1, Name: "New Name"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	season, err := seasons.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if season.Name != "New Name" {
		t.Fatalf("expected updated name, got %s", season.Name)
	}
}

func TestDeleteSeason(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	id, err := seasonService.CreateSeason(&SeasonForm{LeagueID: 1, Name: "Spring 2024"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := seasonService.DeleteSeason(id, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 matches deleted for an empty season, got %d", deleted)
	}

	if _, err := seasons.Get(id); err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

// Deleting a season with scheduled matches is a bulk delete, not a
// "remove them yourself first" block — this proves both the match count
// returned and that everything attached to a match (goals, RSVPs) is
// cleaned up too, not just left dangling or causing an FK error.
func TestDeleteSeasonBulkDeletesMatchesAndDependents(t *testing.T) {
	db := models.NewTestDB(t)

	seasons := &models.SeasonModel{DB: db}
	seasonService := SeasonService{SeasonModel: seasons, DB: db}

	id, err := seasonService.CreateSeason(&SeasonForm{LeagueID: 1, Name: "Spring 2024"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: db}
	opponentID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Rival FC"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: db}
	matchID, err := mm.Insert(&models.Match{SeasonID: id, HomeTeamID: 1, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: db}
	playerID, err := pm.Insert(&models.Player{FirstName: "Sam", LastName: "Striker"})
	if err != nil {
		t.Fatal(err)
	}
	tmm := &models.TeamMemberModel{DB: db}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}
	rm := &models.RSVPModel{DB: db}
	if err := rm.Upsert(&models.RSVP{MatchID: matchID, PlayerID: playerID, TeamID: 1, Status: "yes", RespondedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	goalModel := &models.MatchGoalModel{DB: db}
	if err := goalModel.ReplaceForMatch(matchID, []models.MatchGoal{{TeamID: 1, ScorerPlayerID: sql.NullInt32{Int32: int32(playerID), Valid: true}}}); err != nil {
		t.Fatal(err)
	}

	deleted, err := seasonService.DeleteSeason(id, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 match deleted, got %d", deleted)
	}

	if _, err := seasons.Get(id); err != models.ErrNoRecord {
		t.Fatalf("expected the season to be gone, got %v", err)
	}
	if _, err := mm.Get(matchID); err != models.ErrNoRecord {
		t.Fatalf("expected the match to be gone, got %v", err)
	}
	goals, err := goalModel.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 0 {
		t.Fatalf("expected the match's goals to be cleaned up too, got %d", len(goals))
	}
}

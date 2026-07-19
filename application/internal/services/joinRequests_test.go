package services

import (
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestRequestToJoin(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	jrm := &models.JoinRequestModel{DB: db}
	service := JoinRequestService{JoinRequestModel: jrm, DB: db}

	playerID, err := pm.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := service.RequestToJoin(playerID, 1, "lou@example.com"); err != nil {
		t.Fatal(err)
	}

	jr, err := jrm.GetPendingByPlayerAndLeague(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if jr.TeamID != 1 {
		t.Fatalf("expected pending request for team 1, got team %d", jr.TeamID)
	}
}

func TestRequestToJoinBlockedBySameLeagueMembership(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	tm := &models.TeamModel{DB: db}
	jrm := &models.JoinRequestModel{DB: db}
	service := JoinRequestService{JoinRequestModel: jrm, DB: db}

	playerID, err := pm.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	otherTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Other Team, Same League"})
	if err != nil {
		t.Fatal(err)
	}

	err = service.RequestToJoin(playerID, otherTeamID, "lou@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestRequestToJoinAllowedAcrossDifferentLeagues(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	lm := &models.LeagueModel{DB: db}
	tm := &models.TeamModel{DB: db}
	jrm := &models.JoinRequestModel{DB: db}
	service := JoinRequestService{JoinRequestModel: jrm, DB: db}

	playerID, err := pm.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	secondLeagueID, err := lm.Insert(&models.League{Name: "Second League"})
	if err != nil {
		t.Fatal(err)
	}
	secondTeamID, err := tm.Insert(&models.Team{LeagueID: secondLeagueID, Name: "Second League Team"})
	if err != nil {
		t.Fatal(err)
	}

	if err := service.RequestToJoin(playerID, secondTeamID, "lou@example.com"); err != nil {
		t.Fatalf("expected request to a different league to be allowed, got %v", err)
	}
}

func TestRequestToJoinBlockedByDuplicatePendingRequest(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	tm := &models.TeamModel{DB: db}
	jrm := &models.JoinRequestModel{DB: db}
	service := JoinRequestService{JoinRequestModel: jrm, DB: db}

	playerID, err := pm.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	otherTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Another Team"})
	if err != nil {
		t.Fatal(err)
	}

	if err := service.RequestToJoin(playerID, 1, "lou@example.com"); err != nil {
		t.Fatal(err)
	}

	err = service.RequestToJoin(playerID, otherTeamID, "lou@example.com")
	if err != models.ErrDuplicateRequest {
		t.Fatalf("expected ErrDuplicateRequest, got %v", err)
	}
}

func TestApproveJoinRequest(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	jrm := &models.JoinRequestModel{DB: db}
	users := &models.UserModel{DB: db}
	service := JoinRequestService{JoinRequestModel: jrm, DB: db}

	playerID, err := pm.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RequestToJoin(playerID, 1, "lou@example.com"); err != nil {
		t.Fatal(err)
	}
	jr, err := jrm.GetPendingByPlayerAndLeague(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}

	admin, err := users.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Approve(jr.ID, admin.UserID, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	isMember, err := tmm.IsMember(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected player to be added to team 1")
	}
}

func TestApproveRejectsOtherPendingInSameLeagueOnly(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	lm := &models.LeagueModel{DB: db}
	tm := &models.TeamModel{DB: db}
	jrm := &models.JoinRequestModel{DB: db}
	users := &models.UserModel{DB: db}
	service := JoinRequestService{JoinRequestModel: jrm, DB: db}

	playerID, err := pm.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	// A second team in league 1 (competing request) and a team in a
	// different league (unrelated request that must survive).
	competingTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Competing Team"})
	if err != nil {
		t.Fatal(err)
	}
	secondLeagueID, err := lm.Insert(&models.League{Name: "Second League"})
	if err != nil {
		t.Fatal(err)
	}
	unrelatedTeamID, err := tm.Insert(&models.Team{LeagueID: secondLeagueID, Name: "Unrelated Team"})
	if err != nil {
		t.Fatal(err)
	}

	if err := service.RequestToJoin(playerID, 1, "lou@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := service.RequestToJoin(playerID, unrelatedTeamID, "lou@example.com"); err != nil {
		t.Fatal(err)
	}

	// Simulate a competing request for the other team in league 1, inserted
	// directly (RequestToJoin itself would now block a second same-league
	// request from this player, which is exactly the scenario being guarded
	// against here — a stale competing request from before this player had
	// team 1's pending request).
	competingRequestID, err := jrm.Insert(&models.TeamJoinRequest{PlayerID: playerID, TeamID: competingTeamID})
	if err != nil {
		t.Fatal(err)
	}

	teamOneRequest, err := jrm.GetPendingByPlayerAndLeague(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}

	admin, err := users.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Approve(teamOneRequest.ID, admin.UserID, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	competing, err := jrm.Get(competingRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if competing.Status != "REJECTED" {
		t.Fatalf("expected competing same-league request to be rejected, got %s", competing.Status)
	}

	unrelated, err := jrm.GetPendingByPlayerAndLeague(playerID, secondLeagueID)
	if err != nil {
		t.Fatalf("expected unrelated different-league request to still be pending, got %v", err)
	}
	if unrelated.TeamID != unrelatedTeamID {
		t.Fatalf("expected unrelated pending request for team %d, got %d", unrelatedTeamID, unrelated.TeamID)
	}
}

func TestRejectJoinRequest(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	jrm := &models.JoinRequestModel{DB: db}
	users := &models.UserModel{DB: db}
	service := JoinRequestService{JoinRequestModel: jrm, DB: db}

	playerID, err := pm.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RequestToJoin(playerID, 1, "lou@example.com"); err != nil {
		t.Fatal(err)
	}
	jr, err := jrm.GetPendingByPlayerAndLeague(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}

	admin, err := users.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Reject(jr.ID, admin.UserID, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	rejected, err := jrm.Get(jr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "REJECTED" {
		t.Fatalf("expected status REJECTED, got %s", rejected.Status)
	}
}

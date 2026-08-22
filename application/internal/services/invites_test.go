package services

import (
	"database/sql"
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestSendInvitesRejectsExistingRosterMember(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	im := &models.InviteModel{DB: db}
	inviteService := InviteService{InviteModel: im, DB: db}

	playerID, err := pm.Insert(&models.Player{
		FirstName: "Already",
		LastName:  "OnRoster",
		Email:     sql.NullString{String: "already-on-roster@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	form := &InviteForm{Emails: "already-on-roster@example.com"}
	_, err = inviteService.SendInvites(1, 1, "admin@example.com", form)
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
	if form.FieldErrors["emails"] == "" {
		t.Fatal("expected a field error on emails")
	}
}

func TestSendInvitesAllowsNewEmail(t *testing.T) {
	db := models.NewTestDB(t)

	im := &models.InviteModel{DB: db}
	inviteService := InviteService{InviteModel: im, DB: db}

	form := &InviteForm{Emails: "brand-new-invitee@example.com"}
	invited, err := inviteService.SendInvites(1, 1, "admin@example.com", form)
	if err != nil {
		t.Fatal(err)
	}
	if len(invited) != 1 || invited[0] != "brand-new-invitee@example.com" {
		t.Fatalf("expected to invite brand-new-invitee@example.com, got %v", invited)
	}
}

func TestInviteRosterPlayers(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	im := &models.InviteModel{DB: db}
	inviteService := InviteService{InviteModel: im, DB: db}

	playerID, err := pm.Insert(&models.Player{
		FirstName: "Roster",
		LastName:  "Placeholder",
		Email:     sql.NullString{String: "roster-placeholder@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}

	invited, err := inviteService.InviteRosterPlayers(1, []int{playerID}, 1, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(invited) != 1 || invited[0] != "roster-placeholder@example.com" {
		t.Fatalf("expected to invite roster-placeholder@example.com, got %v", invited)
	}

	pending, err := im.ListPendingByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Email != "roster-placeholder@example.com" {
		t.Fatalf("expected the invite to be recorded, got %v", pending)
	}
}

func TestInviteRosterPlayersSkipsPlayerWithAccount(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	um := &models.UserModel{DB: db}
	im := &models.InviteModel{DB: db}
	inviteService := InviteService{InviteModel: im, DB: db}

	playerID, err := pm.Insert(&models.Player{
		FirstName: "Has",
		LastName:  "Account",
		Email:     sql.NullString{String: "has-account-roster@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}
	userID, err := um.Insert("has-account-roster@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}

	invited, err := inviteService.InviteRosterPlayers(1, []int{playerID}, 1, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(invited) != 0 {
		t.Fatalf("expected a player who already has an account to be skipped, got %v", invited)
	}
}

func TestInviteRosterPlayersSkipsNonMember(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	im := &models.InviteModel{DB: db}
	inviteService := InviteService{InviteModel: im, DB: db}

	// Not added to any team's roster.
	playerID, err := pm.Insert(&models.Player{
		FirstName: "Not",
		LastName:  "OnRoster",
		Email:     sql.NullString{String: "not-on-roster@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	invited, err := inviteService.InviteRosterPlayers(1, []int{playerID}, 1, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(invited) != 0 {
		t.Fatalf("expected a non-roster player to be skipped, got %v", invited)
	}
}

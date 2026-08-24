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

func TestSendInvitesAddsExistingAccountWithPlayerDirectly(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	um := &models.UserModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	im := &models.InviteModel{DB: db}
	inviteService := InviteService{InviteModel: im, DB: db}

	playerID, err := pm.Insert(&models.Player{
		FirstName: "Already",
		LastName:  "HasAccount",
		Email:     sql.NullString{String: "already-has-account@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := um.Insert("already-has-account@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}

	form := &InviteForm{Emails: "already-has-account@example.com"}
	invited, err := inviteService.SendInvites(1, 1, "admin@example.com", form)
	if err != nil {
		t.Fatal(err)
	}
	if len(invited) != 1 || invited[0] != "already-has-account@example.com" {
		t.Fatalf("expected already-has-account@example.com to be added, got %v", invited)
	}

	isMember, err := tmm.IsMember(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected the existing account's player to be added to the team roster immediately")
	}

	pending, err := im.ListPendingByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no dangling invite to be created for an existing account, got %v", pending)
	}
}

func TestSendInvitesCreatesPlaceholderPlayerForAccountWithNone(t *testing.T) {
	db := models.NewTestDB(t)

	um := &models.UserModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	im := &models.InviteModel{DB: db}
	inviteService := InviteService{InviteModel: im, DB: db}

	_, err := um.Insert("no-player-yet@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	form := &InviteForm{Emails: "no-player-yet@example.com"}
	invited, err := inviteService.SendInvites(1, 1, "admin@example.com", form)
	if err != nil {
		t.Fatal(err)
	}
	if len(invited) != 1 {
		t.Fatalf("expected one address added, got %v", invited)
	}

	user, err := um.GetUserByEmail("no-player-yet@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.PlayerID.Valid {
		t.Fatal("expected a placeholder player to be created and linked")
	}
	isMember, err := tmm.IsMember(int(user.PlayerID.Int32), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected the new placeholder player to be added to the team roster")
	}
}

func TestSendInvitesSkipsExistingAccountAlreadyOnATeamInLeague(t *testing.T) {
	db := models.NewTestDB(t)

	tm := &models.TeamModel{DB: db}
	pm := &models.PlayerModel{DB: db}
	um := &models.UserModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	im := &models.InviteModel{DB: db}
	inviteService := InviteService{InviteModel: im, DB: db}

	otherTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Other Team In League"})
	if err != nil {
		t.Fatal(err)
	}
	playerID, err := pm.Insert(&models.Player{
		FirstName: "Already",
		LastName:  "OnOtherTeam",
		Email:     sql.NullString{String: "already-on-other-team@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, otherTeamID); err != nil {
		t.Fatal(err)
	}
	userID, err := um.Insert("already-on-other-team@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}

	form := &InviteForm{Emails: "already-on-other-team@example.com"}
	_, err = inviteService.SendInvites(1, 1, "admin@example.com", form)
	if err != nil {
		t.Fatal(err)
	}

	isMemberOfTeam1, err := tmm.IsMember(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if isMemberOfTeam1 {
		t.Fatal("expected the player to NOT be added to team 1 while already on a team in the same league")
	}
	isMemberOfOtherTeam, err := tmm.IsMember(playerID, otherTeamID)
	if err != nil {
		t.Fatal(err)
	}
	if !isMemberOfOtherTeam {
		t.Fatal("expected the player's original team membership to be untouched")
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

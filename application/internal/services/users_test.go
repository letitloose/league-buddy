package services

import (
	"database/sql"
	"testing"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestInsertUser(t *testing.T) {
	db := models.NewTestDB(t)

	users := &models.UserModel{DB: db}
	userService := UserService{UserModel: users} // Email left nil: signup must not require it

	form := &UserForm{Email: "new-signup@example.com", Password: "validpassword123", ConfirmPassword: "validpassword123"}
	if err := userService.InsertUser(form); err != nil {
		t.Fatal(err)
	}

	user, err := users.GetUserByEmail("new-signup@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if user.Active {
		t.Fatal("expected new user to be inactive")
	}
}

// InsertSeedUser is InsertUser minus the activation email — used only by
// the dev RESETDB bootstrap seed (cmd/web/main.go), which force-activates
// every account it creates immediately afterward regardless of any link
// being clicked.
func TestInsertSeedUser(t *testing.T) {
	db := models.NewTestDB(t)

	users := &models.UserModel{DB: db}
	userService := UserService{UserModel: users}

	form := &UserForm{Email: "seed-user@example.com", Password: "validpassword123", ConfirmPassword: "validpassword123"}
	if err := userService.InsertSeedUser(form); err != nil {
		t.Fatal(err)
	}

	user, err := users.GetUserByEmail("seed-user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if user.Active {
		t.Fatal("expected new user to be inactive (reset() activates it separately)")
	}
}

func TestInsertUserBadData(t *testing.T) {
	db := models.NewTestDB(t)

	users := &models.UserModel{DB: db}
	userService := UserService{UserModel: users}

	form := &UserForm{Email: "not-an-email", Password: "short", ConfirmPassword: "different"}
	err := userService.InsertUser(form)
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestActivateUserCreatesPlaceholderPlayer(t *testing.T) {
	db := models.NewTestDB(t)

	users := &models.UserModel{DB: db}
	userService := UserService{UserModel: users}

	form := &UserForm{Email: "activate-me@example.com", Password: "validpassword123", ConfirmPassword: "validpassword123"}
	if err := userService.InsertUser(form); err != nil {
		t.Fatal(err)
	}

	hash, err := userService.GetVerificationHashByEmail("activate-me@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := userService.ActivateUser(hash); err != nil {
		t.Fatal(err)
	}

	user, err := users.GetUserByEmail("activate-me@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.Active {
		t.Fatal("expected user to be active after activation")
	}
	if !user.PlayerID.Valid {
		t.Fatal("expected a placeholder player to have been linked")
	}
}

func TestActivateUserLinksExistingPlayerByEmail(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	playerID, err := pm.Insert(&models.Player{
		FirstName: "Pre",
		LastName:  "Added",
		Email:     sql.NullString{String: "pre-added@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	users := &models.UserModel{DB: db}
	userService := UserService{UserModel: users}

	form := &UserForm{Email: "pre-added@example.com", Password: "validpassword123", ConfirmPassword: "validpassword123"}
	if err := userService.InsertUser(form); err != nil {
		t.Fatal(err)
	}

	hash, err := userService.GetVerificationHashByEmail("pre-added@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := userService.ActivateUser(hash); err != nil {
		t.Fatal(err)
	}

	user, err := users.GetUserByEmail("pre-added@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.PlayerID.Valid || int(user.PlayerID.Int32) != playerID {
		t.Fatalf("expected user to be linked to pre-added player %d, got %v", playerID, user.PlayerID)
	}
}

func TestActivateUserWithInviteAutoJoinsTeam(t *testing.T) {
	db := models.NewTestDB(t)

	tm := &models.TeamModel{DB: db}
	secondTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Invited Team"})
	if err != nil {
		t.Fatal(err)
	}

	users := &models.UserModel{DB: db}
	creator, err := users.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	im := &models.InviteModel{DB: db}
	inviteID, err := im.Insert(&models.Invite{
		Token:           "test-invite-token",
		TeamID:          secondTeamID,
		Email:           "invitee@example.com",
		CreatedByUserID: creator.UserID,
	})
	if err != nil {
		t.Fatal(err)
	}

	userService := UserService{UserModel: users}

	form := &UserForm{
		Email:           "invitee@example.com",
		Password:        "validpassword123",
		ConfirmPassword: "validpassword123",
		InviteToken:     "test-invite-token",
	}
	if err := userService.InsertUser(form); err != nil {
		t.Fatal(err)
	}

	invitedUserSummary, err := users.GetUserByEmail("invitee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	invitedUser, err := users.GetUser(invitedUserSummary.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if !invitedUser.PendingInviteID.Valid || int(invitedUser.PendingInviteID.Int32) != inviteID {
		t.Fatalf("expected pendingInviteID %d, got %v", inviteID, invitedUser.PendingInviteID)
	}

	hash, err := userService.GetVerificationHashByEmail("invitee@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := userService.ActivateUser(hash); err != nil {
		t.Fatal(err)
	}

	activatedUser, err := users.GetUser(invitedUserSummary.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if !activatedUser.PlayerID.Valid {
		t.Fatal("expected a player to have been linked")
	}
	if activatedUser.PendingInviteID.Valid {
		t.Fatal("expected pendingInviteID to be cleared after activation")
	}

	tmm := &models.TeamMemberModel{DB: db}
	isMember, err := tmm.IsMember(int(activatedUser.PlayerID.Int32), secondTeamID)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatalf("expected player auto-joined to team %d", secondTeamID)
	}

	invite, err := im.Get(inviteID)
	if err != nil {
		t.Fatal(err)
	}
	if !invite.UsedAt.Valid {
		t.Fatal("expected invite to be marked used")
	}
}

func TestActivateUserWithCaptainInviteSetsCaptain(t *testing.T) {
	db := models.NewTestDB(t)

	tm := &models.TeamModel{DB: db}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Captain Invite Team"})
	if err != nil {
		t.Fatal(err)
	}

	users := &models.UserModel{DB: db}
	creator, err := users.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	im := &models.InviteModel{DB: db}
	if _, err := im.Insert(&models.Invite{
		Token:           "captain-invite-token",
		TeamID:          teamID,
		Email:           "new-captain@example.com",
		CreatedByUserID: creator.UserID,
		AsCaptain:       true,
	}); err != nil {
		t.Fatal(err)
	}

	userService := UserService{UserModel: users}

	form := &UserForm{
		Email:           "new-captain@example.com",
		Password:        "validpassword123",
		ConfirmPassword: "validpassword123",
		InviteToken:     "captain-invite-token",
	}
	if err := userService.InsertUser(form); err != nil {
		t.Fatal(err)
	}

	hash, err := userService.GetVerificationHashByEmail("new-captain@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := userService.ActivateUser(hash); err != nil {
		t.Fatal(err)
	}

	invitedUserSummary, err := users.GetUserByEmail("new-captain@example.com")
	if err != nil {
		t.Fatal(err)
	}
	activatedUser, err := users.GetUser(invitedUserSummary.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if !activatedUser.PlayerID.Valid {
		t.Fatal("expected a player to have been linked")
	}

	team, err := tm.Get(teamID)
	if err != nil {
		t.Fatal(err)
	}
	if !team.CaptainPlayerID.Valid || int(team.CaptainPlayerID.Int32) != int(activatedUser.PlayerID.Int32) {
		t.Fatalf("expected team %d's captain to be player %d, got %+v", teamID, activatedUser.PlayerID.Int32, team.CaptainPlayerID)
	}
}

func TestActivateUserWithInviteSkipsAutoJoinOnLeagueConflict(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}
	tm := &models.TeamModel{DB: db}
	users := &models.UserModel{DB: db}

	// An existing (unlinked) player already on team 1, in league 1.
	existingPlayerID, err := pm.Insert(&models.Player{
		FirstName: "Already",
		LastName:  "OnATeam",
		Email:     sql.NullString{String: "conflict@example.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(existingPlayerID, 1); err != nil {
		t.Fatal(err)
	}

	// A second team, same league (1), that they're about to be invited to.
	otherTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Conflicting Team"})
	if err != nil {
		t.Fatal(err)
	}

	creator, err := users.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}
	im := &models.InviteModel{DB: db}
	inviteID, err := im.Insert(&models.Invite{
		Token:           "conflict-invite-token",
		TeamID:          otherTeamID,
		Email:           "conflict@example.com",
		CreatedByUserID: creator.UserID,
	})
	if err != nil {
		t.Fatal(err)
	}

	userService := UserService{UserModel: users}
	form := &UserForm{
		Email:           "conflict@example.com",
		Password:        "validpassword123",
		ConfirmPassword: "validpassword123",
		InviteToken:     "conflict-invite-token",
	}
	if err := userService.InsertUser(form); err != nil {
		t.Fatal(err)
	}

	hash, err := userService.GetVerificationHashByEmail("conflict@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := userService.ActivateUser(hash); err != nil {
		t.Fatal(err)
	}

	// The user should be linked to the existing player (by email)...
	user, err := users.GetUserByEmail("conflict@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.PlayerID.Valid || int(user.PlayerID.Int32) != existingPlayerID {
		t.Fatalf("expected link to existing player %d, got %v", existingPlayerID, user.PlayerID)
	}

	// ...but NOT auto-joined to the conflicting team...
	isMember, err := tmm.IsMember(existingPlayerID, otherTeamID)
	if err != nil {
		t.Fatal(err)
	}
	if isMember {
		t.Fatal("expected auto-join to be skipped due to league conflict")
	}

	// ...their original team-1 membership stays untouched...
	isMember, err = tmm.IsMember(existingPlayerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected original team membership to be unaffected")
	}

	// ...and the invite is still consumed, not left dangling.
	invite, err := im.Get(inviteID)
	if err != nil {
		t.Fatal(err)
	}
	if !invite.UsedAt.Valid {
		t.Fatal("expected invite to be marked used despite the skipped auto-join")
	}
}

func TestAuthenticateUser(t *testing.T) {
	db := models.NewTestDB(t)

	users := &models.UserModel{DB: db}
	userService := UserService{UserModel: users}

	id, err := userService.AuthenticateUser(&UserForm{Email: "player@example.com", Password: "testpassword"})
	if err != nil {
		t.Fatal(err)
	}
	if id < 1 {
		t.Fatal("expected a valid user id")
	}

	_, err = userService.AuthenticateUser(&UserForm{Email: "player@example.com", Password: "wrongpassword"})
	if err != models.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestToggleAdmin(t *testing.T) {
	db := models.NewTestDB(t)

	users := &models.UserModel{DB: db}
	userService := UserService{UserModel: users}

	targetID, err := users.Insert("toggle-admin@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	adminID, err := userService.AuthenticateUser(&UserForm{Email: "player@example.com", Password: "testpassword"})
	if err != nil {
		t.Fatal(err)
	}

	if err := userService.ToggleAdmin(targetID, adminID); err != nil {
		t.Fatal(err)
	}

	isAdmin, err := users.UserHasRole(targetID, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if !isAdmin {
		t.Fatal("expected target user to have ADMIN role after toggle")
	}
}

package models

import (
	"database/sql"
	"testing"
	"time"
)

func TestInsertUser(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}

	id, err := um.Insert("new-user@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	user, err := um.GetUser(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "new-user@example.com"
	if user.Email != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, user.Email)
	}
	if user.Active {
		t.Fatal("expected new user to be inactive")
	}
}

func TestInsertUserDuplicateEmail(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}

	if _, err := um.Insert("dup@example.com", "validpassword123"); err != nil {
		t.Fatal(err)
	}

	_, err := um.Insert("dup@example.com", "validpassword123")
	if err != ErrDuplicateEmail {
		t.Fatalf("expected ErrDuplicateEmail, got %v", err)
	}
}

// Deleting a user who created an invite, was recorded as having used
// someone else's invite, or reviewed a join request must not fail with an
// FK violation — createdByUserID is cleaned up (the invite is deleted
// outright, since that column is NOT NULL), while usedByUserID and
// teamJoinRequests.respondedByUserID are just orphaned to NULL, since those
// rows still carry real meaning after the actor's own login is gone.
func TestDeleteUserCleansUpDependentInvitesAndJoinRequests(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}
	im := InviteModel{DB: db}
	jrm := JoinRequestModel{DB: db}
	pm := PlayerModel{DB: db}

	deletedUserID, err := um.Insert("about-to-be-deleted@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	otherUserID, err := um.Insert("other-user@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	// An invite the deleted user sent -- createdByUserID is NOT NULL, so
	// this row can't survive; it must be deleted, not just orphaned.
	sentInviteID, err := im.Insert(&Invite{Token: "sent-token", TeamID: 1, Email: "invitee@example.com", CreatedByUserID: deletedUserID})
	if err != nil {
		t.Fatal(err)
	}

	// An invite someone ELSE sent, that the deleted user was recorded as
	// having used -- usedByUserID is nullable, so this row should survive
	// with usedByUserID cleared.
	usedInviteID, err := im.Insert(&Invite{Token: "used-token", TeamID: 1, Email: "about-to-be-deleted@example.com", CreatedByUserID: otherUserID})
	if err != nil {
		t.Fatal(err)
	}
	if err := im.MarkUsed(usedInviteID, deletedUserID); err != nil {
		t.Fatal(err)
	}

	// A join request the deleted user reviewed -- respondedByUserID is
	// nullable, so this row should survive with it cleared.
	requesterID, err := pm.Insert(&Player{FirstName: "Wants", LastName: "ToJoin"})
	if err != nil {
		t.Fatal(err)
	}
	joinRequestID, err := jrm.Insert(&TeamJoinRequest{PlayerID: requesterID, TeamID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := jrm.UpdateStatus(joinRequestID, "APPROVED", deletedUserID); err != nil {
		t.Fatal(err)
	}

	if err := um.Delete(deletedUserID); err != nil {
		t.Fatal(err)
	}

	if _, err := um.GetUser(deletedUserID); err != ErrNoRecord {
		t.Fatalf("expected the user to be gone, got %v", err)
	}

	if _, err := im.Get(sentInviteID); err != ErrNoRecord {
		t.Fatalf("expected the invite the deleted user created to be gone, got %v", err)
	}

	usedInvite, err := im.Get(usedInviteID)
	if err != nil {
		t.Fatal(err)
	}
	if usedInvite.UsedByUserID.Valid {
		t.Fatalf("expected usedByUserID to be cleared, got %+v", usedInvite.UsedByUserID)
	}

	joinRequest, err := jrm.Get(joinRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if joinRequest.RespondedByUserID.Valid {
		t.Fatalf("expected respondedByUserID to be cleared, got %+v", joinRequest.RespondedByUserID)
	}
}

func TestAuthenticate(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}

	id, err := um.Authenticate("player@example.com", "testpassword")
	if err != nil {
		t.Fatal(err)
	}
	if id < 1 {
		t.Fatal("expected a valid user id")
	}

	_, err = um.Authenticate("player@example.com", "wrongpassword")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestActivate(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}

	id, err := um.Insert("activate-me@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	if err := um.Activate(id); err != nil {
		t.Fatal(err)
	}

	active, err := um.Active(id)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("expected user to be active")
	}
}

func TestGetAuthContext(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}

	ac, err := um.GetAuthContext(1)
	if err != nil {
		t.Fatal(err)
	}

	if !ac.Active {
		t.Fatal("expected seeded test user to be active")
	}
	if ac.IsAdmin {
		t.Fatal("expected seeded test user to not be admin")
	}
}

func TestGetAuthContextWithTeams(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tmm := TeamMemberModel{DB: db}
	tm := TeamModel{DB: db}
	um := UserModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "Cap", LastName: "Tain"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, 1); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetCaptain(1, sql.NullInt32{Int32: int32(playerID), Valid: true}); err != nil {
		t.Fatal(err)
	}

	userID, err := um.Insert("captain-auth@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}

	ac, err := um.GetAuthContext(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ac.TeamIDs) != 1 || ac.TeamIDs[0] != 1 {
		t.Fatalf("expected TeamIDs [1], got %v", ac.TeamIDs)
	}
	if len(ac.CaptainTeamIDs) != 1 || ac.CaptainTeamIDs[0] != 1 {
		t.Fatalf("expected CaptainTeamIDs [1], got %v", ac.CaptainTeamIDs)
	}
}

func TestGetAuthContextWithLeagueAdmin(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	lam := LeagueAdminModel{DB: db}
	um := UserModel{DB: db}

	playerID, err := pm.Insert(&Player{FirstName: "League", LastName: "Admin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := lam.AddAdmin(playerID, 1); err != nil {
		t.Fatal(err)
	}

	userID, err := um.Insert("league-admin-auth@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}

	ac, err := um.GetAuthContext(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ac.LeagueAdminLeagueIDs) != 1 || ac.LeagueAdminLeagueIDs[0] != 1 {
		t.Fatalf("expected LeagueAdminLeagueIDs [1], got %v", ac.LeagueAdminLeagueIDs)
	}
	if len(ac.LeagueAdminTeamIDs) != 1 || ac.LeagueAdminTeamIDs[0] != 1 {
		t.Fatalf("expected LeagueAdminTeamIDs [1] (seeded team 1 is in league 1), got %v", ac.LeagueAdminTeamIDs)
	}
}

func TestSetPlayerID(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	playerID, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	um := UserModel{DB: db}
	userID, err := um.Insert("link-me@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}

	user, err := um.GetUser(userID)
	if err != nil {
		t.Fatal(err)
	}

	if !user.PlayerID.Valid || int(user.PlayerID.Int32) != playerID {
		t.Fatalf("expected playerID %d, got %v", playerID, user.PlayerID)
	}
}

func TestListPlayerIDsWithAccounts(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	um := UserModel{DB: db}

	linkedPlayerID, err := pm.Insert(&Player{FirstName: "Linked", LastName: "Player"})
	if err != nil {
		t.Fatal(err)
	}
	unlinkedPlayerID, err := pm.Insert(&Player{FirstName: "Unlinked", LastName: "Player"})
	if err != nil {
		t.Fatal(err)
	}

	userID, err := um.Insert("has-account@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, linkedPlayerID); err != nil {
		t.Fatal(err)
	}

	accounts, err := um.ListPlayerIDsWithAccounts([]int{linkedPlayerID, unlinkedPlayerID})
	if err != nil {
		t.Fatal(err)
	}
	if !accounts[linkedPlayerID] {
		t.Errorf("expected %d to have an account", linkedPlayerID)
	}
	if accounts[unlinkedPlayerID] {
		t.Errorf("expected %d to NOT have an account", unlinkedPlayerID)
	}

	empty, err := um.ListPlayerIDsWithAccounts([]int{})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected an empty map for an empty input slice, got %v", empty)
	}
}

func TestToggleActive(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}

	id, err := um.Insert("toggle-me@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	if err := um.ToggleActive(id); err != nil {
		t.Fatal(err)
	}

	active, err := um.Active(id)
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("expected user to be active after toggle")
	}
}

func TestUserHasRole(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}

	id, err := um.Insert("role-test@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	hasRole, err := um.UserHasRole(id, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if hasRole {
		t.Fatal("expected new user to not have ADMIN role")
	}

	if err := um.InsertUserRole(id, "ADMIN"); err != nil {
		t.Fatal(err)
	}

	hasRole, err = um.UserHasRole(id, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if !hasRole {
		t.Fatal("expected user to have ADMIN role after insert")
	}
}

// With no sort specified, the user list defaults to most-recent-login
// first — the admin's usual "who's been active" view.
func TestSearchUsersDefaultSortsByLastLoginDescending(t *testing.T) {
	db := NewTestDB(t)
	um := UserModel{DB: db}

	olderID, err := um.Insert("older-login@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	newerID, err := um.Insert("newer-login@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE users SET lastlogin = ? WHERE id = ?", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), olderID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE users SET lastlogin = ? WHERE id = ?", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), newerID); err != nil {
		t.Fatal(err)
	}

	results, err := um.Search(&UserSearchCriteria{Email: "-login@example.com", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].UserID != newerID || results[1].UserID != olderID {
		t.Fatalf("expected the more recent login first by default, got %+v then %+v", results[0], results[1])
	}
}

// An explicit sort/order pair overrides the lastlogin-descending default.
func TestSearchUsersSortByColumnAndOrder(t *testing.T) {
	db := NewTestDB(t)
	um := UserModel{DB: db}

	aID, err := um.Insert("aaa-sort-test@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	zID, err := um.Insert("zzz-sort-test@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}

	asc, err := um.Search(&UserSearchCriteria{Email: "-sort-test@example.com", Sort: "email", Order: "ASC", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(asc) != 2 || asc[0].UserID != aID || asc[1].UserID != zID {
		t.Fatalf("expected aaa then zzz ascending, got %+v", asc)
	}

	desc, err := um.Search(&UserSearchCriteria{Email: "-sort-test@example.com", Sort: "email", Order: "DESC", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(desc) != 2 || desc[0].UserID != zID || desc[1].UserID != aID {
		t.Fatalf("expected zzz then aaa descending, got %+v", desc)
	}
}

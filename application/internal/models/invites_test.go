package models

import "testing"

func TestListPendingByTeam(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}
	im := InviteModel{DB: db}

	creator, err := um.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	pending, err := im.ListPendingByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending invites before any are sent, got %d", len(pending))
	}

	inviteID, err := im.Insert(&Invite{
		Token:           "list-pending-token",
		TeamID:          1,
		Email:           "invitee@example.com",
		CreatedByUserID: creator.UserID,
	})
	if err != nil {
		t.Fatal(err)
	}

	pending, err = im.ListPendingByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Email != "invitee@example.com" {
		t.Fatalf("expected 1 pending invite for invitee@example.com, got %v", pending)
	}

	if err := im.MarkUsed(inviteID, creator.UserID); err != nil {
		t.Fatal(err)
	}

	pending, err = im.ListPendingByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending invites once used, got %d", len(pending))
	}
}

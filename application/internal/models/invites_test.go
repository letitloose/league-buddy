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

func TestCancelInvite(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}
	im := InviteModel{DB: db}

	creator, err := um.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	inviteID, err := im.Insert(&Invite{
		Token:           "cancel-token",
		TeamID:          1,
		Email:           "invitee@example.com",
		CreatedByUserID: creator.UserID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := im.Cancel(inviteID); err != nil {
		t.Fatal(err)
	}

	invite, err := im.Get(inviteID)
	if err != nil {
		t.Fatal(err)
	}
	if !invite.CanceledAt.Valid {
		t.Fatal("expected canceledAt to be set")
	}

	pending, err := im.ListPendingByTeam(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected a canceled invite to drop off the pending list, got %d", len(pending))
	}

	// Canceling again (already canceled) is a no-op, not an error state to
	// silently succeed at — nothing left to cancel.
	if err := im.Cancel(inviteID); err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord canceling an already-canceled invite, got %v", err)
	}
}

func TestCancelUsedInviteFails(t *testing.T) {
	db := NewTestDB(t)

	um := UserModel{DB: db}
	im := InviteModel{DB: db}

	creator, err := um.GetUserByEmail("player@example.com")
	if err != nil {
		t.Fatal(err)
	}

	inviteID, err := im.Insert(&Invite{
		Token:           "used-token",
		TeamID:          1,
		Email:           "invitee@example.com",
		CreatedByUserID: creator.UserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := im.MarkUsed(inviteID, creator.UserID); err != nil {
		t.Fatal(err)
	}

	if err := im.Cancel(inviteID); err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord canceling an already-used invite, got %v", err)
	}
}

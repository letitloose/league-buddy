package models

import (
	"database/sql"
	"testing"
	"time"
)

func TestInsertPlayer(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{
		FirstName: "Lou",
		LastName:  "Garwood",
		Email:     sql.NullString{String: "lou@example.com", Valid: true},
	}

	id, err := pm.Insert(player)
	if err != nil {
		t.Fatal(err)
	}

	got, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Lou"
	if got.FirstName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, got.FirstName)
	}
}

func TestUpdatePlayer(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{FirstName: "Lou", LastName: "Garwood"}
	id, err := pm.Insert(player)
	if err != nil {
		t.Fatal(err)
	}

	player.ID = id
	player.LastName = "Buddy"
	if err := pm.Update(player); err != nil {
		t.Fatal(err)
	}

	got, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Buddy"
	if got.LastName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, got.LastName)
	}
}

func TestGetPlayerByEmail(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{
		FirstName: "Lou",
		LastName:  "Garwood",
		Email:     sql.NullString{String: "lou@example.com", Valid: true},
	}
	if _, err := pm.Insert(player); err != nil {
		t.Fatal(err)
	}

	got, err := pm.GetByEmail("lou@example.com")
	if err != nil {
		t.Fatal(err)
	}

	expected := "Lou"
	if got.FirstName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, got.FirstName)
	}

	_, err = pm.GetByEmail("nope@example.com")
	if err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestGetPlayersByTeam(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	tmm := TeamMemberModel{DB: db}

	id1, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := pm.Insert(&Player{FirstName: "Ada", LastName: "Lovelace"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(id1, 1); err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(id2, 1); err != nil {
		t.Fatal(err)
	}

	players, err := pm.GetByTeam(1)
	if err != nil {
		t.Fatal(err)
	}

	expected := 2
	if len(players) != expected {
		t.Fatalf("wrong number of results. expecting %d, got %d", expected, len(players))
	}
}

func TestCalendarToken(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	id, err := pm.Insert(&Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	token, err := pm.GetCalendarToken(id)
	if err != nil {
		t.Fatal(err)
	}
	if token.Valid {
		t.Fatalf("expected no calendar token yet, got %+v", token)
	}

	if err := pm.SetCalendarToken(id, "abc123"); err != nil {
		t.Fatal(err)
	}
	token, err = pm.GetCalendarToken(id)
	if err != nil {
		t.Fatal(err)
	}
	if !token.Valid || token.String != "abc123" {
		t.Fatalf("expected token abc123, got %+v", token)
	}

	got, err := pm.GetByCalendarToken("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id {
		t.Fatalf("expected GetByCalendarToken to resolve player %d, got %d", id, got.ID)
	}

	_, err = pm.GetByCalendarToken("no-such-token")
	if err != ErrNoRecord {
		t.Fatalf("expected ErrNoRecord for an unknown token, got %v", err)
	}
}

// Changing phonenumber via Update must clear any existing phone
// verification — this is the one place every phone-number write in the
// app funnels through, so it's the one place the "changing the number
// clears verification" consent rule can be enforced.
func TestUpdatePlayerClearsPhoneVerificationOnNumberChange(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	player := &Player{FirstName: "Lou", LastName: "Garwood", PhoneNumber: sql.NullString{String: "518-555-0100", Valid: true}}
	id, err := pm.Insert(player)
	if err != nil {
		t.Fatal(err)
	}
	if err := pm.SetPhoneVerificationCode(id, "123456", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := pm.ConfirmPhoneVerified(id); err != nil {
		t.Fatal(err)
	}

	verified, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.PhoneVerifiedAt.Valid {
		t.Fatal("expected phone to be verified before the update")
	}

	// Updating with the SAME phone number must leave verification intact.
	verified.FirstName = "Louis"
	if err := pm.Update(verified); err != nil {
		t.Fatal(err)
	}
	stillVerified, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !stillVerified.PhoneVerifiedAt.Valid {
		t.Fatal("expected verification to survive an update that didn't change the phone number")
	}

	// Updating with a DIFFERENT phone number must clear verification.
	stillVerified.PhoneNumber = sql.NullString{String: "518-555-0199", Valid: true}
	if err := pm.Update(stillVerified); err != nil {
		t.Fatal(err)
	}
	cleared, err := pm.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.PhoneVerifiedAt.Valid {
		t.Fatal("expected verification to be cleared after the phone number changed")
	}
	if cleared.PhoneVerificationCode.Valid || cleared.PhoneVerificationExpiresAt.Valid {
		t.Fatal("expected any pending verification code to be cleared too")
	}
}

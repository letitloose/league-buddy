package models

import (
	"database/sql"
	"testing"
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

func TestSetPlayerID(t *testing.T) {
	db := NewTestDB(t)

	pm := PlayerModel{DB: db}
	playerID, err := pm.Insert(&Player{TeamID: sql.NullInt32{Int32: 1, Valid: true}, FirstName: "Lou", LastName: "Garwood"})
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

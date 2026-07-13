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

	tm := &models.TeamModel{DB: db}
	team, err := tm.GetDefault()
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: db}
	playerID, err := pm.Insert(&models.Player{
		TeamID:    team.ID,
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

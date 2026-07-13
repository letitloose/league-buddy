package services

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type UserService struct {
	*models.UserModel
	*Email
}

type UserForm struct {
	Email               string `form:"email"`
	Password            string `form:"password"`
	ConfirmPassword     string
	validator.Validator `form:"-"`
}

type UserSearchForm struct {
	*models.UserSearchCriteria
	validator.Validator
}

type UserPost struct {
	ID        int    `json:"userID,string"`
	CSRFToken string `json:"csrf_token"`
}

func (service *UserService) ForgotPassword(uf *UserForm) error {
	uf.CheckField(validator.NotBlank(uf.Email), "email", "This field cannot be blank")
	if !uf.Valid() {
		return models.ErrBadData
	}

	verificationHash, err := service.GetVerificationHashByEmail(uf.Email)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			return nil
		} else {
			return err
		}
	}

	if service.Email != nil {
		siteAddress := os.Getenv("VIRTUAL_HOST")
		body := fmt.Sprintf(
			`<html>
				<body>
					<p>Please <a href="https://%s/user/resetPassword?hash=%s">click here</a> to reset your password.<p>
				</body>
			</html>`, siteAddress, verificationHash)
		err = service.SendEmailV2("League Buddy Password Reset", "", body, uf.Email)
		if err != nil {
			return err
		}
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(uf.Email, time.Now(), "password reset email sent: "+uf.Email)
	if err != nil {
		return err
	}

	return nil
}

func (service *UserService) InsertUser(uf *UserForm) error {

	// Validate the form contents using our helper functions.
	uf.CheckField(validator.NotBlank(uf.Email), "email", "This field cannot be blank")
	uf.CheckField(validator.ValidEmail(uf.Email), "email", "You must enter a valid email: name@domain.ext")
	uf.CheckField(validator.NotBlank(uf.Password), "password", "This field cannot be blank")
	uf.CheckField(validator.MinChars(uf.Password, 8), "password", "This field must be at least 8 characters long")
	uf.CheckField(validator.Equals(uf.Password, uf.ConfirmPassword), "confirmPassword", "Passwords must match!")

	if !uf.Valid() {
		return models.ErrBadData
	}

	_, err := service.Insert(uf.Email, uf.Password)
	if err != nil {
		return err
	}

	//user created successfully,  send an email with the validation link
	if service.Email != nil {
		verificationHash, err := service.GetVerificationHashByEmail(uf.Email)
		if err != nil {
			return err
		}
		siteAddress := os.Getenv("VIRTUAL_HOST")
		body := fmt.Sprintf(
			`<html>
				<body>
					<h1>Hello!</h1>
					<p>Please <a href="https://%s/user/activate?hash=%s">click here</a> to validate your email and activate your account.<p>
				</body>
			</html>`, siteAddress, verificationHash)
		err = service.SendEmailV2("Activate your League Buddy account", "", body, uf.Email)
		if err != nil {
			return err
		}
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(uf.Email, time.Now(), "user record created: "+uf.Email)
	if err != nil {
		return err
	}

	return nil
}

func (service *UserService) ResetPassword(uf *UserForm) error {

	// Validate the form contents using our helper functions.
	uf.CheckField(validator.MinChars(uf.Password, 8), "password", "This field must be at least 8 characters long")
	uf.CheckField(validator.Equals(uf.Password, uf.ConfirmPassword), "confirmPassword", "Passwords must match!")

	if !uf.Valid() {
		return models.ErrBadData
	}

	err := service.UserModel.ResetPassword(uf.Email, uf.Password)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(uf.Email, time.Now(), "user password reset: "+uf.Email)
	if err != nil {
		return err
	}

	return nil
}

func (service *UserService) AuthenticateUser(uf *UserForm) (int, error) {

	uf.CheckField(validator.NotBlank(uf.Email), "email", "Please enter your email to login")

	if !uf.Valid() {
		return 0, models.ErrBadData
	}

	id, err := service.Authenticate(uf.Email, uf.Password)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (service *UserService) ToggleActive(userID, loggedInUserID int) error {

	loggedInUser, err := service.GetUser(loggedInUserID)
	if err != nil {
		return err
	}
	user, err := service.GetUser(userID)
	if err != nil {
		return err
	}

	description := " activated user record: "
	if user.Active {
		description = " deactivated user record: "
	}

	err = service.UserModel.ToggleActive(userID)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(loggedInUser.Email, time.Now(), loggedInUser.Email+description+user.Email)
	if err != nil {
		return err
	}

	if !user.PlayerID.Valid {
		err := service.linkOrCreatePlayer(user.UserID, user.Email)
		if err != nil {
			return err
		}
	}

	return nil
}

func (service *UserService) ActivateUser(hash string) error {
	user, err := service.GetByVerificationHash(hash)
	if err != nil {
		return err
	}

	err = service.Activate(user.UserID)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(user.Email, time.Now(), "user record activated: "+user.Email)
	if err != nil {
		return err
	}

	return service.linkOrCreatePlayer(user.UserID, user.Email)
}

// linkOrCreatePlayer links a user account to a roster player record: it
// first tries to claim an existing unlinked player row with a matching email
// (covers an admin pre-adding a player before they sign up), otherwise
// creates a placeholder player on the default team. Safe to call more than
// once for the same user — re-linking to the same player is a no-op.
func (service *UserService) linkOrCreatePlayer(userID int, email string) error {
	pm := models.PlayerModel{DB: service.DB}

	if existing, err := pm.GetByEmail(email); err == nil {
		return service.UserModel.SetPlayerID(userID, existing.ID)
	} else if !errors.Is(err, models.ErrNoRecord) {
		return err
	}

	tm := models.TeamModel{DB: service.DB}
	team, err := tm.GetDefault()
	if err != nil {
		return err
	}

	newPlayer := &models.Player{
		TeamID:    team.ID,
		FirstName: "",
		LastName:  "",
		Email:     sql.NullString{String: email, Valid: true},
	}
	playerID, err := pm.Insert(newPlayer)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(email, time.Now(), "player record created: "+email)
	if err != nil {
		return err
	}

	return service.UserModel.SetPlayerID(userID, playerID)
}

func (service *UserService) ToggleAdmin(userID, loggedInUserID int) error {
	isAdmin, err := service.UserHasRole(userID, "ADMIN")
	if err != nil {
		return err
	}
	if isAdmin {
		service.DeleteUserRole(userID, "ADMIN")
	} else {
		service.InsertUserRole(userID, "ADMIN")
	}

	loggedInUser, err := service.GetUser(loggedInUserID)
	if err != nil {
		return err
	}
	user, err := service.GetUser(userID)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(loggedInUser.Email, time.Now(), "toggled admin for user: "+user.Email)
	if err != nil {
		return err
	}

	return nil
}

func (service *UserService) DeleteUser(userID, loggedInUserID int) error {

	user, err := service.GetUser(userID)
	if err != nil {
		return err
	}

	err = service.Delete(userID)
	if err != nil {
		return err
	}

	loggedInUser, err := service.GetUser(loggedInUserID)
	if err != nil {
		return err
	}
	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(loggedInUser.Email, time.Now(), "deleted user: "+user.Email)
	if err != nil {
		return err
	}

	return nil
}

func (service *UserService) GetAuthContext(id int) (*models.AuthContext, error) {
	return service.UserModel.GetAuthContext(id)
}

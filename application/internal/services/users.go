package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type UserService struct {
	*models.UserModel
	*Email
	InfoLog *log.Logger // used to log activation/reset links when Email is nil (no creds configured)
}

type UserForm struct {
	Email               string `form:"email"`
	Password            string `form:"password"`
	ConfirmPassword     string
	InviteToken         string `form:"-"` // from ?invite= at signup, threaded through as a hidden field
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

	resetLink := fmt.Sprintf("https://%s/user/resetPassword?hash=%s", os.Getenv("PUBLIC_HOST"), verificationHash)

	if service.Email != nil {
		body := fmt.Sprintf(
			`<html>
				<body>
					<p>Please <a href="%s">click here</a> to reset your password.<p>
				</body>
			</html>`, resetLink)
		err = service.SendEmailV2("Blame the Ball Password Reset", "", body, uf.Email)
		if err != nil {
			return err
		}
	} else if service.InfoLog != nil {
		service.InfoLog.Printf("no email configured -- password reset link for %s: %s", uf.Email, resetLink)
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(uf.Email, time.Now(), "password reset email sent: "+uf.Email)
	if err != nil {
		return err
	}

	return nil
}

func (service *UserService) InsertUser(uf *UserForm) error {
	return service.insertUser(uf, true)
}

// InsertSeedUser creates a login exactly like InsertUser, but never sends
// the activation email — for the dev RESETDB bootstrap seed only
// (cmd/web/main.go's reset/seedRoleUser), which force-activates every
// account it creates immediately afterward via ActivateUser regardless of
// whether any link is ever clicked. Sending a real "activate your account"
// email in that path is always pointless — and since RESETDB reseeds on
// every dev-server restart (and Air restarts the process on every file
// save), a configured Email sender turned that into a real inbox getting
// spammed dozens of times in a single session.
func (service *UserService) InsertSeedUser(uf *UserForm) error {
	return service.insertUser(uf, false)
}

func (service *UserService) insertUser(uf *UserForm, sendEmail bool) error {

	// Validate the form contents using our helper functions.
	uf.CheckField(validator.NotBlank(uf.Email), "email", "This field cannot be blank")
	uf.CheckField(validator.ValidEmail(uf.Email), "email", "You must enter a valid email: name@domain.ext")
	uf.CheckField(validator.NotBlank(uf.Password), "password", "This field cannot be blank")
	uf.CheckField(validator.MinChars(uf.Password, 8), "password", "This field must be at least 8 characters long")
	uf.CheckField(validator.Equals(uf.Password, uf.ConfirmPassword), "confirmPassword", "Passwords must match!")

	if !uf.Valid() {
		return models.ErrBadData
	}

	userID, err := service.Insert(uf.Email, uf.Password)
	if err != nil {
		return err
	}

	if uf.InviteToken != "" {
		im := models.InviteModel{DB: service.DB}
		invite, err := im.GetByToken(uf.InviteToken)
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return err
		}
		// A stale/unknown/already-used/canceled token is treated as "no
		// invite" — never block signup because of a bad URL param.
		if err == nil && !invite.UsedAt.Valid && !invite.CanceledAt.Valid {
			if err := service.UserModel.SetPendingInvite(userID, invite.ID); err != nil {
				return err
			}
		}
	}

	//user created successfully, send (or log, in dev) the activation link
	if sendEmail {
		verificationHash, err := service.GetVerificationHashByEmail(uf.Email)
		if err != nil {
			return err
		}
		activationLink := fmt.Sprintf("https://%s/user/activate?hash=%s", os.Getenv("PUBLIC_HOST"), verificationHash)

		if service.Email != nil {
			body := fmt.Sprintf(
				`<html>
					<body>
						<h1>Hello!</h1>
						<p>Please <a href="%s">click here</a> to validate your email and activate your account.<p>
					</body>
				</html>`, activationLink)
			err = service.SendEmailV2("Activate your Blame the Ball account", "", body, uf.Email)
			if err != nil {
				return err
			}
		} else if service.InfoLog != nil {
			service.InfoLog.Printf("no email configured -- activation link for %s: %s", uf.Email, activationLink)
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
// creates a placeholder player. If the signup carried a valid invite token
// (users.pendingInviteID, set at signup time — see InsertUser), the linked
// or newly-created player is added to the invited team's membership, no
// approval needed — unless they already belong to a different team in that
// same league, in which case the auto-join is skipped (a player may hold at
// most one team per league) but the invite is still consumed, so it never
// dangles. A player with no invite is left unaffiliated until an invite or
// an approved join request adds a membership. Safe to call more than once
// for the same user — re-linking to the same player, or re-consuming an
// already-used invite, is a no-op.
func (service *UserService) linkOrCreatePlayer(userID int, email string) error {
	pm := models.PlayerModel{DB: service.DB}
	im := models.InviteModel{DB: service.DB}
	tmm := models.TeamMemberModel{DB: service.DB}

	user, err := service.GetUser(userID)
	if err != nil {
		return err
	}

	var invite *models.Invite
	if user.PendingInviteID.Valid {
		invite, err = im.Get(int(user.PendingInviteID.Int32))
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return err
		}
	}

	if existing, err := pm.GetByEmail(email); err == nil {
		if invite != nil {
			team, err := (&models.TeamModel{DB: service.DB}).Get(invite.TeamID)
			if err != nil {
				return err
			}
			hasTeamInLeague, err := tmm.HasTeamInLeague(existing.ID, team.LeagueID)
			if err != nil {
				return err
			}
			if !hasTeamInLeague {
				if err := tmm.AddMembership(existing.ID, invite.TeamID); err != nil && !errors.Is(err, models.ErrDuplicateMembership) {
					return err
				}
				if invite.AsCaptain {
					if err := (&models.TeamModel{DB: service.DB}).SetCaptain(invite.TeamID, sql.NullInt32{Int32: int32(existing.ID), Valid: true}); err != nil {
						return err
					}
				}
			} else if service.InfoLog != nil {
				service.InfoLog.Printf("invite auto-join skipped for %s: already on a team in league %d", email, team.LeagueID)
			}
		}
		if err := service.UserModel.SetPlayerID(userID, existing.ID); err != nil {
			return err
		}
	} else if errors.Is(err, models.ErrNoRecord) {
		newPlayer := &models.Player{
			FirstName: models.PlaceholderFirstName,
			LastName:  models.PlaceholderLastName,
			Email:     sql.NullString{String: email, Valid: true},
		}
		playerID, err := pm.Insert(newPlayer)
		if err != nil {
			return err
		}
		cs := &CommonService{DB: service.DB}
		if err := cs.InsertAuditLog(email, time.Now(), "player record created: "+email); err != nil {
			return err
		}
		if invite != nil {
			// Brand new player, zero existing memberships — no league
			// conflict is possible.
			if err := tmm.AddMembership(playerID, invite.TeamID); err != nil {
				return err
			}
			if invite.AsCaptain {
				if err := (&models.TeamModel{DB: service.DB}).SetCaptain(invite.TeamID, sql.NullInt32{Int32: int32(playerID), Valid: true}); err != nil {
					return err
				}
			}
		}
		if err := service.UserModel.SetPlayerID(userID, playerID); err != nil {
			return err
		}
	} else {
		return err
	}

	if invite != nil {
		if err := im.MarkUsed(invite.ID, userID); err != nil {
			return err
		}
		if err := service.UserModel.ClearPendingInvite(userID); err != nil {
			return err
		}
	}

	return nil
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

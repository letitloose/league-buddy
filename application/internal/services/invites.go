package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type InviteForm struct {
	Emails    string // raw textarea, newline/comma separated
	AsCaptain bool
	validator.Validator
}

type InviteService struct {
	*models.InviteModel
	DB *sql.DB
	*Email
	InfoLog *log.Logger
}

func generateInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// parseEmailList splits raw input on commas/newlines, trims whitespace,
// drops blanks, and de-duplicates while preserving order.
func parseEmailList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})

	seen := map[string]bool{}
	emails := []string{}
	for _, f := range fields {
		addr := strings.TrimSpace(f)
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		emails = append(emails, addr)
	}
	return emails
}

// SendInvites parses form.Emails, validates each address (including
// rejecting any address that already belongs to a player on this team's
// roster — see the field error text for why), then either adds the address
// straight to the roster (if it already belongs to a User account — see
// addExistingAccountToRoster) or creates an invite token and emails (or
// logs, in dev mode without EMAIL_USER configured) a signup link. Returns
// the addresses actually invited/added.
func (service *InviteService) SendInvites(teamID, createdByUserID int, actorEmail string, form *InviteForm) ([]string, error) {
	emails := parseEmailList(form.Emails)
	if len(emails) == 0 {
		form.AddFieldError("emails", "You must enter at least one email address.")
	}
	if form.AsCaptain && len(emails) > 1 {
		form.AddFieldError("emails", "Only one person can be invited as team captain at a time.")
	}

	pm := &models.PlayerModel{DB: service.DB}
	tmm := &models.TeamMemberModel{DB: service.DB}
	for _, addr := range emails {
		if !validator.ValidEmail(addr) {
			form.AddFieldError("emails", "You must enter a valid email: name@domain.ext")
			break
		}
		existing, err := pm.GetByEmail(addr)
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return nil, err
		}
		if err == nil {
			isMember, err := tmm.IsMember(existing.ID, teamID)
			if err != nil {
				return nil, err
			}
			if isMember {
				form.AddFieldError("emails", addr+" is already on this team's roster — use the roster list below to invite them instead.")
				break
			}
		}
	}

	if !form.Valid() {
		return nil, models.ErrBadData
	}

	tm := &models.TeamModel{DB: service.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		return nil, err
	}

	um := &models.UserModel{DB: service.DB}
	invited := []string{}
	for _, addr := range emails {
		// An address that already belongs to a User account will never sign
		// up again, so a token-based invite for it would just dangle
		// forever — add them to the roster immediately instead.
		user, err := um.GetUserByEmail(addr)
		if err != nil && !errors.Is(err, models.ErrNoRecord) {
			return invited, err
		}
		if err == nil {
			if err := service.addExistingAccountToRoster(team, user, actorEmail, form.AsCaptain); err != nil {
				return invited, err
			}
			invited = append(invited, addr)
			continue
		}

		if err := service.sendOneInvite(team, createdByUserID, addr, actorEmail, form.AsCaptain); err != nil {
			return invited, err
		}
		invited = append(invited, addr)
	}

	return invited, nil
}

// InviteRosterPlayers invites specific existing roster members by player ID
// — the picker on the invite page for roster placeholders who haven't
// signed up yet. Unlike SendInvites, these players are *expected* to already
// be on the roster, so that check doesn't apply here. Silently skips any
// playerID that turns out not to be on this team, already has an account, or
// has no email on file (a tampered request, since the picker itself never
// offers those) rather than failing the whole batch. Returns the addresses
// actually invited.
func (service *InviteService) InviteRosterPlayers(teamID int, playerIDs []int, createdByUserID int, actorEmail string) ([]string, error) {
	tm := &models.TeamModel{DB: service.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		return nil, err
	}

	pm := &models.PlayerModel{DB: service.DB}
	tmm := &models.TeamMemberModel{DB: service.DB}
	um := &models.UserModel{DB: service.DB}

	invited := []string{}
	for _, playerID := range playerIDs {
		player, err := pm.Get(playerID)
		if err != nil {
			if errors.Is(err, models.ErrNoRecord) {
				continue
			}
			return invited, err
		}
		if !player.Email.Valid || player.Email.String == "" {
			continue
		}

		isMember, err := tmm.IsMember(playerID, teamID)
		if err != nil {
			return invited, err
		}
		if !isMember {
			continue
		}

		hasAccount, err := um.ListPlayerIDsWithAccounts([]int{playerID})
		if err != nil {
			return invited, err
		}
		if hasAccount[playerID] {
			continue
		}

		addr := player.Email.String
		if err := service.sendOneInvite(team, createdByUserID, addr, actorEmail, false); err != nil {
			return invited, err
		}
		invited = append(invited, addr)
	}

	return invited, nil
}

// sendOneInvite generates a token, records the invite, sends (or logs) the
// signup email, and audit-logs it — the part shared by SendInvites and
// InviteRosterPlayers regardless of how the recipient's address was chosen.
// asCaptain marks the invite so accepting it makes the recipient this
// team's captain (see linkOrCreatePlayer in users.go) and changes the
// email copy accordingly.
func (service *InviteService) sendOneInvite(team *models.Team, createdByUserID int, addr, actorEmail string, asCaptain bool) error {
	token, err := generateInviteToken()
	if err != nil {
		return err
	}

	_, err = service.Insert(&models.Invite{
		Token:           token,
		TeamID:          team.ID,
		Email:           addr,
		CreatedByUserID: createdByUserID,
		AsCaptain:       asCaptain,
	})
	if err != nil {
		return err
	}

	invitationText := fmt.Sprintf("You've been invited to join %s on Blame the Ball.", team.Name)
	if asCaptain {
		invitationText = fmt.Sprintf("You've been invited to become the team captain of %s on Blame the Ball.", team.Name)
	}

	signupLink := fmt.Sprintf("https://%s/user/signup?invite=%s", os.Getenv("PUBLIC_HOST"), token)
	if service.Email != nil {
		body := fmt.Sprintf(
			`<html>
				<body>
					<p>%s <a href="%s">Sign up here</a>.</p>
				</body>
			</html>`, invitationText, signupLink)
		if err := service.SendEmailV2(fmt.Sprintf("You're invited to join %s", team.Name), "", body, addr); err != nil {
			return err
		}
	} else if service.InfoLog != nil {
		service.InfoLog.Printf("no email configured -- invite link for %s (team %d): %s", addr, team.ID, signupLink)
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "invited "+addr+" to team: "+team.Name)
}

// addExistingAccountToRoster adds an existing User account straight onto
// team's roster, skipping the token/signup-link flow entirely — that flow
// only ever gets consumed by a brand-new signup, so inviting an address
// that already has an account would otherwise leave the invite dangling
// forever (see SendInvites). Creates a placeholder player first if the
// account somehow has none yet (e.g. an admin-created login that was never
// linked). Skips the membership add (but still reports success and emails
// nothing further) if the player already holds a team in this league,
// mirroring linkOrCreatePlayer's identical one-team-per-league rule — for
// the same reason, asCaptain is skipped along with it, since they were
// never actually added to team.
func (service *InviteService) addExistingAccountToRoster(team *models.Team, user *models.User, actorEmail string, asCaptain bool) error {
	pm := &models.PlayerModel{DB: service.DB}
	tmm := &models.TeamMemberModel{DB: service.DB}
	um := &models.UserModel{DB: service.DB}

	playerID := int(user.PlayerID.Int32)
	if !user.PlayerID.Valid {
		newPlayer := &models.Player{
			FirstName: models.PlaceholderFirstName,
			LastName:  models.PlaceholderLastName,
			Email:     sql.NullString{String: user.Email, Valid: true},
		}
		id, err := pm.Insert(newPlayer)
		if err != nil {
			return err
		}
		playerID = id
		if err := um.SetPlayerID(user.UserID, playerID); err != nil {
			return err
		}
	}

	hasTeamInLeague, err := tmm.HasTeamInLeague(playerID, team.LeagueID)
	if err != nil {
		return err
	}
	if hasTeamInLeague {
		if service.InfoLog != nil {
			service.InfoLog.Printf("invite auto-add skipped for %s: already on a team in league %d", user.Email, team.LeagueID)
		}
		return nil
	}

	if err := tmm.AddMembership(playerID, team.ID); err != nil && !errors.Is(err, models.ErrDuplicateMembership) {
		return err
	}

	if asCaptain {
		tm := &models.TeamModel{DB: service.DB}
		if err := tm.SetCaptain(team.ID, sql.NullInt32{Int32: int32(playerID), Valid: true}); err != nil {
			return err
		}
	}

	teamLink := fmt.Sprintf("https://%s/team/%d", os.Getenv("PUBLIC_HOST"), team.ID)
	if service.Email != nil {
		body := fmt.Sprintf(
			`<html>
				<body>
					<p>You've been added to %s on Blame the Ball. <a href="%s">View your team</a>.</p>
				</body>
			</html>`, team.Name, teamLink)
		if err := service.SendEmailV2(fmt.Sprintf("You've been added to %s", team.Name), "", body, user.Email); err != nil {
			return err
		}
	} else if service.InfoLog != nil {
		service.InfoLog.Printf("no email configured -- added %s to team %d (%s): %s", user.Email, team.ID, team.Name, teamLink)
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "added existing account "+user.Email+" to team: "+team.Name)
}

// CancelInvite revokes an outstanding invite so its signup link no longer
// works. No-ops (returns ErrNoRecord) if it was already used or canceled.
func (service *InviteService) CancelInvite(id int, actorEmail string) error {
	invite, err := service.Get(id)
	if err != nil {
		return err
	}

	if err := service.Cancel(id); err != nil {
		return err
	}

	tm := &models.TeamModel{DB: service.DB}
	team, err := tm.Get(invite.TeamID)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "canceled invite for "+invite.Email+" to team: "+team.Name)
}

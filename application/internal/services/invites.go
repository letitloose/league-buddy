package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type InviteForm struct {
	Emails string // raw textarea, newline/comma separated
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

// SendInvites parses form.Emails, validates each address, creates one
// invite token per address, and emails (or logs, in dev mode without
// EMAIL_USER configured) a signup link for each. Returns the addresses
// actually invited.
func (service *InviteService) SendInvites(teamID, createdByUserID int, actorEmail string, form *InviteForm) ([]string, error) {
	emails := parseEmailList(form.Emails)
	if len(emails) == 0 {
		form.AddFieldError("emails", "You must enter at least one email address.")
	}
	for _, addr := range emails {
		if !validator.ValidEmail(addr) {
			form.AddFieldError("emails", "You must enter a valid email: name@domain.ext")
			break
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

	cs := &CommonService{DB: service.DB}
	invited := []string{}

	for _, addr := range emails {
		token, err := generateInviteToken()
		if err != nil {
			return invited, err
		}

		_, err = service.Insert(&models.Invite{
			Token:           token,
			TeamID:          teamID,
			Email:           addr,
			CreatedByUserID: createdByUserID,
		})
		if err != nil {
			return invited, err
		}

		signupLink := fmt.Sprintf("https://%s/user/signup?invite=%s", os.Getenv("VIRTUAL_HOST"), token)
		if service.Email != nil {
			body := fmt.Sprintf(
				`<html>
					<body>
						<p>You've been invited to join %s on League Buddy. <a href="%s">Sign up here</a>.</p>
					</body>
				</html>`, team.Name, signupLink)
			if err := service.SendEmailV2(fmt.Sprintf("You're invited to join %s", team.Name), "", body, addr); err != nil {
				return invited, err
			}
		} else if service.InfoLog != nil {
			service.InfoLog.Printf("no email configured -- invite link for %s (team %d): %s", addr, teamID, signupLink)
		}

		if err := cs.InsertAuditLog(actorEmail, time.Now(), "invited "+addr+" to team: "+team.Name); err != nil {
			return invited, err
		}

		invited = append(invited, addr)
	}

	return invited, nil
}

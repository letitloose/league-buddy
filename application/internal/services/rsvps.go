package services

import (
	"database/sql"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type RSVPForm struct {
	Status  string // "yes" or "no"
	Message string
	validator.Validator
}

type RSVPService struct {
	*models.RSVPModel
}

// SubmitRSVP validates and upserts a player's response to a match. Callers
// are responsible for eligibility (is this player actually on the match's
// home or away roster) — that's a session-context check done in the web
// layer, not something worth re-verifying here since matchID/playerID never
// come from attacker-controlled form data.
func (service *RSVPService) SubmitRSVP(matchID, playerID, teamID int, form *RSVPForm) error {
	form.CheckField(form.Status == "yes" || form.Status == "no", "status", "You must choose Yes or No.")
	form.CheckField(validator.MaxChars(form.Message, 255), "message", "Message must be at most 255 characters.")

	if !form.Valid() {
		return models.ErrBadData
	}

	var message sql.NullString
	if trimmed := strings.TrimSpace(form.Message); trimmed != "" {
		message = sql.NullString{String: trimmed, Valid: true}
	}

	return service.Upsert(&models.RSVP{
		MatchID:     matchID,
		PlayerID:    playerID,
		TeamID:      teamID,
		Status:      form.Status,
		Message:     message,
		RespondedAt: time.Now(),
	})
}

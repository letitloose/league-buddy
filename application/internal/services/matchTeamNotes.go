package services

import (
	"database/sql"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type MatchTeamNoteForm struct {
	PlayerOfMatchID int // 0 = none
	Notes           string
	validator.Validator
}

type MatchTeamNoteService struct {
	*models.MatchTeamNoteModel
	DB *sql.DB
}

// SaveNote validates and upserts teamID's Player of the Match / captain's
// notes for matchID. Callers are responsible for authorization (does this
// user manage teamID) — a web-layer check via canManageMatchSide, same
// division of labor as RSVPService.SubmitRSVP.
func (service *MatchTeamNoteService) SaveNote(matchID, teamID int, form *MatchTeamNoteForm) error {
	form.CheckField(validator.MaxChars(form.Notes, 2000), "notes", "Notes must be at most 2000 characters.")

	if form.PlayerOfMatchID > 0 {
		tmm := &models.TeamMemberModel{DB: service.DB}
		isMember, err := tmm.IsMember(form.PlayerOfMatchID, teamID)
		if err != nil {
			return err
		}
		if !isMember {
			form.AddFieldError("playerOfMatchID", "That player isn't on this team's roster.")
		}
	}

	if !form.Valid() {
		return models.ErrBadData
	}

	var playerOfMatchID sql.NullInt32
	if form.PlayerOfMatchID > 0 {
		playerOfMatchID = sql.NullInt32{Int32: int32(form.PlayerOfMatchID), Valid: true}
	}
	var notes sql.NullString
	if trimmed := strings.TrimSpace(form.Notes); trimmed != "" {
		notes = sql.NullString{String: trimmed, Valid: true}
	}

	return service.Upsert(&models.MatchTeamNote{
		MatchID:         matchID,
		TeamID:          teamID,
		PlayerOfMatchID: playerOfMatchID,
		Notes:           notes,
		UpdatedAt:       time.Now(),
	})
}

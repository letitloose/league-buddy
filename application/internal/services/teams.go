package services

import (
	"database/sql"
	"errors"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type TeamForm struct {
	ID              int
	LeagueID        int
	Name            string
	Motto           string
	EstablishedDate string // "2006-01-02" from <input type=date>
	validator.Validator
}

type TeamService struct {
	*models.TeamModel
	DB *sql.DB
}

func (service *TeamService) CreateTeam(form *TeamForm, actorEmail string) (int, error) {
	form.CheckField(validator.NotBlank(form.Name), "name", "You must enter a name.")
	if form.LeagueID <= 0 {
		form.AddFieldError("leagueID", "You must choose a league.")
	}

	if !form.Valid() {
		return 0, models.ErrBadData
	}

	lm := &models.LeagueModel{DB: service.DB}
	if _, err := lm.Get(form.LeagueID); err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			return 0, models.ErrNoRecord
		}
		return 0, err
	}

	id, err := service.Insert(&models.Team{
		LeagueID:        form.LeagueID,
		Name:            form.Name,
		Motto:           sql.NullString{String: form.Motto, Valid: form.Motto != ""},
		EstablishedDate: parseOptionalDate(form.EstablishedDate),
	})
	if err != nil {
		return 0, err
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(actorEmail, time.Now(), "team created: "+form.Name)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (service *TeamService) UpdateTeam(form *TeamForm, actorEmail string) error {
	form.CheckField(validator.NotBlank(form.Name), "name", "You must enter a name.")
	if form.LeagueID <= 0 {
		form.AddFieldError("leagueID", "You must choose a league.")
	}

	if !form.Valid() {
		return models.ErrBadData
	}

	lm := &models.LeagueModel{DB: service.DB}
	if _, err := lm.Get(form.LeagueID); err != nil {
		return err
	}

	err := service.Update(&models.Team{
		ID:              form.ID,
		LeagueID:        form.LeagueID,
		Name:            form.Name,
		Motto:           sql.NullString{String: form.Motto, Valid: form.Motto != ""},
		EstablishedDate: parseOptionalDate(form.EstablishedDate),
	})
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "team updated: "+form.Name)
}

func (service *TeamService) DeleteTeam(id int, actorEmail string) error {
	team, err := service.Get(id)
	if err != nil {
		return err
	}

	err = service.Delete(id)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "team deleted: "+team.Name)
}

// SetCaptain assigns playerID (0 clears) as teamID's captain. The target
// player must already be a member of that team.
func (service *TeamService) SetCaptain(teamID, playerID int, actorEmail string) error {
	if playerID == 0 {
		if err := service.TeamModel.SetCaptain(teamID, sql.NullInt32{}); err != nil {
			return err
		}
		cs := &CommonService{DB: service.DB}
		return cs.InsertAuditLog(actorEmail, time.Now(), "team captain cleared")
	}

	pm := &models.PlayerModel{DB: service.DB}
	player, err := pm.Get(playerID)
	if err != nil {
		return err
	}

	tmm := &models.TeamMemberModel{DB: service.DB}
	isMember, err := tmm.IsMember(playerID, teamID)
	if err != nil {
		return err
	}
	if !isMember {
		return models.ErrBadData
	}

	if err := service.TeamModel.SetCaptain(teamID, sql.NullInt32{Int32: int32(playerID), Valid: true}); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "team captain set: "+player.FirstName+" "+player.LastName)
}

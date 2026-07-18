package services

import (
	"database/sql"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type LeagueForm struct {
	ID   int
	Name string
	validator.Validator
}

type LeagueService struct {
	*models.LeagueModel
	DB *sql.DB
}

func (service *LeagueService) CreateLeague(form *LeagueForm, actorEmail string) (int, error) {
	form.CheckField(validator.NotBlank(form.Name), "name", "You must enter a name.")
	if !form.Valid() {
		return 0, models.ErrBadData
	}

	id, err := service.Insert(form.Name)
	if err != nil {
		return 0, err
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(actorEmail, time.Now(), "league created: "+form.Name)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (service *LeagueService) UpdateLeague(form *LeagueForm, actorEmail string) error {
	form.CheckField(validator.NotBlank(form.Name), "name", "You must enter a name.")
	if !form.Valid() {
		return models.ErrBadData
	}

	err := service.Update(form.ID, form.Name)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "league updated: "+form.Name)
}

func (service *LeagueService) DeleteLeague(id int, actorEmail string) error {
	league, err := service.Get(id)
	if err != nil {
		return err
	}

	err = service.Delete(id)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "league deleted: "+league.Name)
}

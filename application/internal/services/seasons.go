package services

import (
	"database/sql"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type SeasonForm struct {
	ID        int
	LeagueID  int
	Name      string
	StartDate string // "2006-01-02" from <input type=date>
	EndDate   string
	validator.Validator
}

type SeasonService struct {
	*models.SeasonModel
	DB *sql.DB
}

func (service *SeasonService) CreateSeason(form *SeasonForm, actorEmail string) (int, error) {
	form.CheckField(validator.NotBlank(form.Name), "name", "You must enter a name.")
	if form.LeagueID <= 0 {
		form.AddFieldError("leagueID", "You must choose a league.")
	}

	if !form.Valid() {
		return 0, models.ErrBadData
	}

	lm := &models.LeagueModel{DB: service.DB}
	if _, err := lm.Get(form.LeagueID); err != nil {
		return 0, err
	}

	id, err := service.Insert(&models.Season{
		LeagueID:  form.LeagueID,
		Name:      form.Name,
		StartDate: parseOptionalDate(form.StartDate),
		EndDate:   parseOptionalDate(form.EndDate),
	})
	if err != nil {
		return 0, err
	}

	cs := &CommonService{DB: service.DB}
	if err := cs.InsertAuditLog(actorEmail, time.Now(), "season created: "+form.Name); err != nil {
		return 0, err
	}

	return id, nil
}

func (service *SeasonService) UpdateSeason(form *SeasonForm, actorEmail string) error {
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

	err := service.Update(&models.Season{
		ID:        form.ID,
		LeagueID:  form.LeagueID,
		Name:      form.Name,
		StartDate: parseOptionalDate(form.StartDate),
		EndDate:   parseOptionalDate(form.EndDate),
	})
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "season updated: "+form.Name)
}

func (service *SeasonService) DeleteSeason(id int, actorEmail string) error {
	season, err := service.Get(id)
	if err != nil {
		return err
	}

	if err := service.Delete(id); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "season deleted: "+season.Name)
}

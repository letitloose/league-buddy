package services

import (
	"database/sql"
	"fmt"
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

// DeleteSeason deletes id along with every match scheduled in it — a
// bulk delete, not a "remove the matches first yourself" block, since a
// season with even a handful of matches (each carrying goals, cards,
// RSVPs, etc.) would otherwise mean deleting every one of them by hand
// first. MatchModel.Delete already cleans up everything attached to each
// match, so this just needs to do that once per match before deleting
// the season row itself. Returns how many matches were removed, so the
// caller can report that back to the user.
func (service *SeasonService) DeleteSeason(id int, actorEmail string) (int, error) {
	season, err := service.Get(id)
	if err != nil {
		return 0, err
	}

	mm := &models.MatchModel{DB: service.DB}
	matches, err := mm.GetBySeason(id)
	if err != nil {
		return 0, err
	}
	for _, match := range matches {
		if err := mm.Delete(match.ID); err != nil {
			return 0, err
		}
	}

	if err := service.Delete(id); err != nil {
		return 0, err
	}

	cs := &CommonService{DB: service.DB}
	if err := cs.InsertAuditLog(actorEmail, time.Now(), fmt.Sprintf("season deleted (with %d match(es)): %s", len(matches), season.Name)); err != nil {
		return 0, err
	}
	return len(matches), nil
}

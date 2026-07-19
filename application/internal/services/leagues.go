package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type LeagueForm struct {
	ID              int
	Name            string
	Motto           string
	EstablishedDate string // "2006-01-02" from <input type=date>
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

	id, err := service.Insert(&models.League{
		Name:            form.Name,
		Motto:           sql.NullString{String: form.Motto, Valid: form.Motto != ""},
		EstablishedDate: parseOptionalDate(form.EstablishedDate),
	})
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

	err := service.Update(&models.League{
		ID:              form.ID,
		Name:            form.Name,
		Motto:           sql.NullString{String: form.Motto, Valid: form.Motto != ""},
		EstablishedDate: parseOptionalDate(form.EstablishedDate),
	})
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "league updated: "+form.Name)
}

// AddLeagueAdmin makes the player with the given email an admin of leagueID.
func (service *LeagueService) AddLeagueAdmin(leagueID int, email string, actorEmail string) error {
	pm := &models.PlayerModel{DB: service.DB}
	player, err := pm.GetByEmail(email)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			return models.ErrBadData
		}
		return err
	}

	lam := &models.LeagueAdminModel{DB: service.DB}
	if err := lam.AddAdmin(player.ID, leagueID); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), fmt.Sprintf("league admin added: %s %s (league %d)", player.FirstName, player.LastName, leagueID))
}

// RemoveLeagueAdmin revokes playerID's admin rights over leagueID.
func (service *LeagueService) RemoveLeagueAdmin(leagueID, playerID int, actorEmail string) error {
	pm := &models.PlayerModel{DB: service.DB}
	player, err := pm.Get(playerID)
	if err != nil {
		return err
	}

	lam := &models.LeagueAdminModel{DB: service.DB}
	if err := lam.RemoveAdmin(playerID, leagueID); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), fmt.Sprintf("league admin removed: %s %s (league %d)", player.FirstName, player.LastName, leagueID))
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

package services

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type LocationForm struct {
	ID            int
	Name          string
	Address1      string
	Address2      string
	City          string
	StateProvince string
	ZipCode       string
	validator.Validator
}

type LocationService struct {
	*models.LocationModel
	DB *sql.DB
}

func validateLocationForm(form *LocationForm) {
	form.CheckField(validator.NotBlank(form.Name), "name", "You must enter a name.")
	form.CheckField(validator.NotBlank(form.Address1), "address1", "You must enter an address.")
	form.CheckField(validator.NotBlank(form.City), "city", "You must enter a city.")
	if form.StateProvince != "" {
		code := strings.ToUpper(strings.TrimSpace(form.StateProvince))
		form.StateProvince = code
		form.CheckField(models.IsValidStateCode(code), "stateprovince", "You must select a valid state.")
	}
}

// CreateLocation creates a new location, or — if one already exists at the
// same address (case/whitespace-insensitive match) — transparently returns
// that existing location's ID instead of creating a duplicate. This is what
// lets a team manager type in a "new" home field that happens to already
// exist and just get associated with it.
func (service *LocationService) CreateLocation(form *LocationForm, actorEmail string) (int, error) {
	validateLocationForm(form)
	if !form.Valid() {
		return 0, models.ErrBadData
	}

	addressID, err := upsertAddress(service.DB, 0, form.Address1, form.Address2, form.City, form.StateProvince, form.ZipCode)
	if err != nil {
		return 0, err
	}

	key := addressKey(form.Address1, form.Address2, form.City, form.StateProvince, form.ZipCode)
	id, err := service.Insert(&models.Location{Name: form.Name, AddressID: int(addressID.Int32), AddressKey: key})
	if err != nil {
		if errors.Is(err, models.ErrDuplicateLocation) {
			// The address row we just inserted above is now an orphan —
			// nothing else could have referenced it yet — so clean it up
			// before reusing the existing location.
			am := &models.AddressModel{DB: service.DB}
			if derr := am.Delete(int(addressID.Int32)); derr != nil {
				return 0, derr
			}

			existing, gerr := service.GetByAddressKey(key)
			if gerr != nil {
				return 0, gerr
			}
			return existing.ID, nil
		}
		return 0, err
	}

	cs := &CommonService{DB: service.DB}
	if err := cs.InsertAuditLog(actorEmail, time.Now(), "location created: "+form.Name); err != nil {
		return 0, err
	}

	return id, nil
}

// UpdateLocation edits a location in place. Unlike CreateLocation, it does
// not merge into another location if the edited address collides with one
// — merging identities on edit would mean reassigning every team pointing
// at the old row and deleting it, which is real surgery, not a "just
// associate with it" nicety — so a colliding edit is rejected as a normal
// validation error instead.
func (service *LocationService) UpdateLocation(form *LocationForm, actorEmail string) error {
	validateLocationForm(form)
	if !form.Valid() {
		return models.ErrBadData
	}

	existing, err := service.Get(form.ID)
	if err != nil {
		return err
	}

	addressID, err := upsertAddress(service.DB, existing.AddressID, form.Address1, form.Address2, form.City, form.StateProvince, form.ZipCode)
	if err != nil {
		return err
	}

	key := addressKey(form.Address1, form.Address2, form.City, form.StateProvince, form.ZipCode)
	err = service.Update(&models.Location{ID: form.ID, Name: form.Name, AddressID: int(addressID.Int32), AddressKey: key})
	if err != nil {
		if errors.Is(err, models.ErrDuplicateLocation) {
			form.AddNonFieldError("Another location already uses this address.")
			return models.ErrBadData
		}
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "location updated: "+form.Name)
}

func (service *LocationService) DeleteLocation(id int, actorEmail string) error {
	location, err := service.Get(id)
	if err != nil {
		return err
	}

	if err := service.Delete(id); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "location deleted: "+location.Name)
}

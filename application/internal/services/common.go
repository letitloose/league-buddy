package services

import (
	"database/sql"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

type CommonService struct {
	DB *sql.DB
}

func (service *CommonService) InsertAuditLog(email string, changeTime time.Time, description string) error {
	am := &models.AuditLogModel{DB: service.DB}

	logEntry := &models.AuditLog{UserEmail: email, ChangeDate: changeTime, Description: description}
	err := am.Insert(logEntry)

	return err
}

// upsertAddress inserts a new address, updates an existing one (when
// existingAddressID > 0), or leaves things alone (returning an invalid
// NullInt32) if address1 is blank and there's no existing address to
// preserve. Shared by any service that carries an optional or required
// street address on its form (players, locations).
func upsertAddress(db *sql.DB, existingAddressID int, address1, address2, city, stateProvince, zipCode string) (sql.NullInt32, error) {
	if address1 == "" {
		if existingAddressID > 0 {
			return sql.NullInt32{Int32: int32(existingAddressID), Valid: true}, nil
		}
		return sql.NullInt32{}, nil
	}

	am := &models.AddressModel{DB: db}
	address := &models.Address{
		ID:            existingAddressID,
		Address1:      sql.NullString{String: address1, Valid: true},
		Address2:      sql.NullString{String: address2, Valid: address2 != ""},
		City:          sql.NullString{String: city, Valid: city != ""},
		StateProvince: sql.NullString{String: stateProvince, Valid: stateProvince != ""},
		ZipCode:       sql.NullString{String: zipCode, Valid: zipCode != ""},
	}

	if existingAddressID > 0 {
		_, err := am.Update(address)
		if err != nil {
			return sql.NullInt32{}, err
		}
		return sql.NullInt32{Int32: int32(existingAddressID), Valid: true}, nil
	}

	id, err := am.Insert(address)
	if err != nil {
		return sql.NullInt32{}, err
	}
	return sql.NullInt32{Int32: int32(id), Valid: true}, nil
}

// addressKey builds a case/whitespace-normalized key from address fields,
// used to detect when two locations are really the same physical address —
// LocationService.CreateLocation relies on this (plus a DB uniqueness
// constraint on locations.addressKey) to transparently reuse an existing
// location instead of creating a duplicate.
func addressKey(address1, address2, city, stateProvince, zipCode string) string {
	normalize := func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return strings.Join([]string{
		normalize(address1),
		normalize(address2),
		normalize(city),
		normalize(stateProvince),
		normalize(zipCode),
	}, "|")
}

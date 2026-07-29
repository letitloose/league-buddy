package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Location is a physical field/venue a team can call its home field.
// AddressID is required — a location without an address isn't useful.
// AddressKey is a normalized form of the address (see services.addressKey)
// with a DB-level uniqueness constraint, so two locations can never be
// created for the same physical address.
type Location struct {
	ID         int
	Name       string
	AddressID  int
	AddressKey string
	Created    time.Time
}

type LocationModel struct {
	DB *sql.DB
}

func (m *LocationModel) Insert(location *Location) (int, error) {
	statement := `INSERT INTO locations (name, addressID, addressKey, created) VALUES (?, ?, ?, UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, location.Name, location.AddressID, location.AddressKey)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 {
				return 0, ErrDuplicateLocation
			}
		}
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (m *LocationModel) Get(id int) (*Location, error) {
	stmt := `SELECT id, name, addressID, addressKey, created FROM locations WHERE id = ?`

	location := &Location{}
	err := m.DB.QueryRow(stmt, id).Scan(&location.ID, &location.Name, &location.AddressID, &location.AddressKey, &location.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return location, nil
}

// GetByAddressKey looks up a location by its normalized address key — used
// to resolve an existing location when a create attempt collides with one.
func (m *LocationModel) GetByAddressKey(key string) (*Location, error) {
	stmt := `SELECT id, name, addressID, addressKey, created FROM locations WHERE addressKey = ?`

	location := &Location{}
	err := m.DB.QueryRow(stmt, key).Scan(&location.ID, &location.Name, &location.AddressID, &location.AddressKey, &location.Created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return location, nil
}

func (m *LocationModel) Update(location *Location) error {
	statement := `UPDATE locations SET name = ?, addressID = ?, addressKey = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, location.Name, location.AddressID, location.AddressKey, location.ID)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 {
				return ErrDuplicateLocation
			}
		}
		return err
	}
	return nil
}

func (m *LocationModel) Delete(id int) error {
	statement := `DELETE FROM locations WHERE id = ?`

	_, err := m.DB.Exec(statement, id)
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1451 {
				return ErrHasDependents
			}
		}
		return err
	}
	return nil
}

func (m *LocationModel) List() ([]*Location, error) {
	stmt := `SELECT id, name, addressID, addressKey, created FROM locations ORDER BY name ASC`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	locations := []*Location{}
	for rows.Next() {
		location := &Location{}
		err := rows.Scan(&location.ID, &location.Name, &location.AddressID, &location.AddressKey, &location.Created)
		if err != nil {
			return nil, err
		}
		locations = append(locations, location)
	}
	return locations, nil
}

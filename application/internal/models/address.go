package models

import (
	"database/sql"
	"errors"
)

type Address struct {
	ID            int
	Address1      sql.NullString
	Address2      sql.NullString
	City          sql.NullString
	StateProvince sql.NullString
	ZipCode       sql.NullString
	Country       sql.NullString
}

type AddressModel struct {
	DB *sql.DB
}

func (m *AddressModel) Insert(address *Address) (int, error) {

	statement := `INSERT INTO address (address1, address2, city, stateProvince, zipCode, country)
    VALUES(?, ?, ?, ?, ?, ?)`

	result, err := m.DB.Exec(statement, address.Address1, address.Address2, address.City, address.StateProvince, address.ZipCode, address.Country)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(id), nil
}

func (m *AddressModel) Get(id int) (*Address, error) {

	stmt := `select id, address1, address2, city, stateProvince, zipCode, country
		from address where id = ?`

	result := m.DB.QueryRow(stmt, id)

	address := &Address{}
	err := result.Scan(&address.ID, &address.Address1, &address.Address2, &address.City,
		&address.StateProvince, &address.ZipCode, &address.Country)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		} else {
			return nil, err
		}
	}
	return address, nil
}

func (m *AddressModel) Update(address *Address) (int, error) {

	statement := `update address set
		address1 = ?,
		address2 = ?,
		city = ?,
		stateProvince = ?,
		zipCode = ?,
		country = ?
	where id = ?`

	result, err := m.DB.Exec(statement, address.Address1, address.Address2, address.City, address.StateProvince, address.ZipCode, address.Country, address.ID)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(id), nil
}

func (m *AddressModel) Delete(id int) error {
	statement := "delete from address where id = ?"

	_, err := m.DB.Exec(statement, id)

	return err
}

func (m *AddressModel) DeleteByPlayer(playerID int) error {
	statement := "delete from address where id = (select addressID from players where id = ?)"

	_, err := m.DB.Exec(statement, playerID)

	return err
}

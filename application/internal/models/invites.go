package models

import (
	"database/sql"
	"errors"
	"time"
)

type Invite struct {
	ID              int
	Token           string
	TeamID          int
	Email           string
	CreatedByUserID int
	CreatedAt       time.Time
	UsedAt          sql.NullTime
	UsedByUserID    sql.NullInt32
	CanceledAt      sql.NullTime
	AsCaptain       bool
}

type InviteModel struct {
	DB *sql.DB
}

func (m *InviteModel) Insert(invite *Invite) (int, error) {
	statement := `INSERT INTO invites (token, teamID, email, createdByUserID, createdAt, asCaptain)
		VALUES (?, ?, ?, ?, UTC_TIMESTAMP(), ?)`

	result, err := m.DB.Exec(statement, invite.Token, invite.TeamID, invite.Email, invite.CreatedByUserID, invite.AsCaptain)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (m *InviteModel) Get(id int) (*Invite, error) {
	stmt := `SELECT id, token, teamID, email, createdByUserID, createdAt, usedAt, usedByUserID, canceledAt, asCaptain
		FROM invites WHERE id = ?`

	invite := &Invite{}
	err := m.DB.QueryRow(stmt, id).Scan(&invite.ID, &invite.Token, &invite.TeamID, &invite.Email,
		&invite.CreatedByUserID, &invite.CreatedAt, &invite.UsedAt, &invite.UsedByUserID, &invite.CanceledAt, &invite.AsCaptain)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return invite, nil
}

func (m *InviteModel) GetByToken(token string) (*Invite, error) {
	stmt := `SELECT id, token, teamID, email, createdByUserID, createdAt, usedAt, usedByUserID, canceledAt, asCaptain
		FROM invites WHERE token = ?`

	invite := &Invite{}
	err := m.DB.QueryRow(stmt, token).Scan(&invite.ID, &invite.Token, &invite.TeamID, &invite.Email,
		&invite.CreatedByUserID, &invite.CreatedAt, &invite.UsedAt, &invite.UsedByUserID, &invite.CanceledAt, &invite.AsCaptain)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return invite, nil
}

// ListPendingByTeam returns every invite for teamID that hasn't been used or
// canceled, oldest first — powers the outstanding-invites list on the invite
// page and the team page's pending-invite badge count.
func (m *InviteModel) ListPendingByTeam(teamID int) ([]*Invite, error) {
	stmt := `SELECT id, token, teamID, email, createdByUserID, createdAt, usedAt, usedByUserID, canceledAt, asCaptain
		FROM invites WHERE teamID = ? AND usedAt IS NULL AND canceledAt IS NULL ORDER BY createdAt ASC`

	rows, err := m.DB.Query(stmt, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invites := []*Invite{}
	for rows.Next() {
		invite := &Invite{}
		err := rows.Scan(&invite.ID, &invite.Token, &invite.TeamID, &invite.Email,
			&invite.CreatedByUserID, &invite.CreatedAt, &invite.UsedAt, &invite.UsedByUserID, &invite.CanceledAt, &invite.AsCaptain)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, nil
}

func (m *InviteModel) MarkUsed(id, usedByUserID int) error {
	statement := `UPDATE invites SET usedAt = UTC_TIMESTAMP(), usedByUserID = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, usedByUserID, id)
	return err
}

// Cancel revokes an outstanding invite, so its token can no longer be used
// to sign up. A no-op guarded update: only touches a row that hasn't already
// been used or canceled. Returns ErrNoRecord if there was nothing to cancel
// (already used, already canceled, or doesn't exist).
func (m *InviteModel) Cancel(id int) error {
	statement := `UPDATE invites SET canceledAt = UTC_TIMESTAMP() WHERE id = ? AND usedAt IS NULL AND canceledAt IS NULL`

	result, err := m.DB.Exec(statement, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNoRecord
	}
	return nil
}

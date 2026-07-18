package models

import (
	"database/sql"
	"errors"
	"time"
)

type TeamJoinRequest struct {
	ID                int
	PlayerID          int
	TeamID            int
	Status            string // "PENDING" | "APPROVED" | "REJECTED"
	RequestedAt       time.Time
	RespondedAt       sql.NullTime
	RespondedByUserID sql.NullInt32
}

// JoinRequestListItem is the joined shape used by the pending-requests list
// templates (both the per-team and the cross-team admin view).
type JoinRequestListItem struct {
	ID          int
	PlayerID    int
	TeamID      int
	TeamName    string
	FirstName   string
	LastName    string
	Email       sql.NullString
	RequestedAt time.Time
}

type JoinRequestModel struct {
	DB *sql.DB
}

func (m *JoinRequestModel) Insert(jr *TeamJoinRequest) (int, error) {
	statement := `INSERT INTO teamJoinRequests (playerID, teamID, status, requestedAt)
		VALUES (?, ?, 'PENDING', UTC_TIMESTAMP())`

	result, err := m.DB.Exec(statement, jr.PlayerID, jr.TeamID)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (m *JoinRequestModel) Get(id int) (*TeamJoinRequest, error) {
	stmt := `SELECT id, playerID, teamID, status, requestedAt, respondedAt, respondedByUserID
		FROM teamJoinRequests WHERE id = ?`

	jr := &TeamJoinRequest{}
	err := m.DB.QueryRow(stmt, id).Scan(&jr.ID, &jr.PlayerID, &jr.TeamID, &jr.Status,
		&jr.RequestedAt, &jr.RespondedAt, &jr.RespondedByUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return jr, nil
}

func (m *JoinRequestModel) GetPendingByPlayer(playerID int) (*TeamJoinRequest, error) {
	stmt := `SELECT id, playerID, teamID, status, requestedAt, respondedAt, respondedByUserID
		FROM teamJoinRequests WHERE playerID = ? AND status = 'PENDING'`

	jr := &TeamJoinRequest{}
	err := m.DB.QueryRow(stmt, playerID).Scan(&jr.ID, &jr.PlayerID, &jr.TeamID, &jr.Status,
		&jr.RequestedAt, &jr.RespondedAt, &jr.RespondedByUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return jr, nil
}

func (m *JoinRequestModel) UpdateStatus(id int, status string, respondedByUserID int) error {
	statement := `UPDATE teamJoinRequests SET status = ?, respondedAt = UTC_TIMESTAMP(), respondedByUserID = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, status, respondedByUserID, id)
	return err
}

// RejectOtherPending marks any other pending request by this player (besides
// excludeID) as REJECTED — used after one request gets approved.
func (m *JoinRequestModel) RejectOtherPending(playerID, excludeID, respondedByUserID int) error {
	statement := `UPDATE teamJoinRequests
		SET status = 'REJECTED', respondedAt = UTC_TIMESTAMP(), respondedByUserID = ?
		WHERE playerID = ? AND id != ? AND status = 'PENDING'`

	_, err := m.DB.Exec(statement, respondedByUserID, playerID, excludeID)
	return err
}

func (m *JoinRequestModel) ListPendingByTeam(teamID int) ([]*JoinRequestListItem, error) {
	stmt := `SELECT jr.id, jr.playerID, jr.teamID, t.name, p.firstname, p.lastname, p.email, jr.requestedAt
		FROM teamJoinRequests jr
		JOIN players p ON p.id = jr.playerID
		JOIN teams t ON t.id = jr.teamID
		WHERE jr.status = 'PENDING' AND jr.teamID = ?
		ORDER BY jr.requestedAt ASC`

	return m.listPending(stmt, teamID)
}

func (m *JoinRequestModel) ListPendingAll() ([]*JoinRequestListItem, error) {
	stmt := `SELECT jr.id, jr.playerID, jr.teamID, t.name, p.firstname, p.lastname, p.email, jr.requestedAt
		FROM teamJoinRequests jr
		JOIN players p ON p.id = jr.playerID
		JOIN teams t ON t.id = jr.teamID
		WHERE jr.status = 'PENDING'
		ORDER BY jr.requestedAt ASC`

	return m.listPending(stmt)
}

func (m *JoinRequestModel) listPending(stmt string, args ...any) ([]*JoinRequestListItem, error) {
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []*JoinRequestListItem{}
	for rows.Next() {
		item := &JoinRequestListItem{}
		err := rows.Scan(&item.ID, &item.PlayerID, &item.TeamID, &item.TeamName,
			&item.FirstName, &item.LastName, &item.Email, &item.RequestedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

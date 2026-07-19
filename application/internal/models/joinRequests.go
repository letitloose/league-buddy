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

// DeleteAllForPlayer removes every join-request row (any status) for
// playerID — used when fully deleting a player record, since
// fk_tjr_player would otherwise block it.
func (m *JoinRequestModel) DeleteAllForPlayer(playerID int) error {
	statement := `DELETE FROM teamJoinRequests WHERE playerID = ?`

	_, err := m.DB.Exec(statement, playerID)
	return err
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

// GetPendingByPlayerAndLeague finds this player's pending request (if any)
// for a team in leagueID. A player can have simultaneous pending requests in
// different leagues, so this is scoped to one league at a time, not global.
func (m *JoinRequestModel) GetPendingByPlayerAndLeague(playerID, leagueID int) (*TeamJoinRequest, error) {
	stmt := `SELECT jr.id, jr.playerID, jr.teamID, jr.status, jr.requestedAt, jr.respondedAt, jr.respondedByUserID
		FROM teamJoinRequests jr
		JOIN teams t ON t.id = jr.teamID
		WHERE jr.playerID = ? AND t.leagueID = ? AND jr.status = 'PENDING'`

	jr := &TeamJoinRequest{}
	err := m.DB.QueryRow(stmt, playerID, leagueID).Scan(&jr.ID, &jr.PlayerID, &jr.TeamID, &jr.Status,
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

// RejectOtherPending marks any other pending request by this player in the
// same league (besides excludeID) as REJECTED — used after one request gets
// approved. Scoped to leagueID so an approval in one league doesn't reject a
// player's unrelated pending request in a different league.
func (m *JoinRequestModel) RejectOtherPending(playerID, excludeID, respondedByUserID, leagueID int) error {
	statement := `UPDATE teamJoinRequests jr
		JOIN teams t ON t.id = jr.teamID
		SET jr.status = 'REJECTED', jr.respondedAt = UTC_TIMESTAMP(), jr.respondedByUserID = ?
		WHERE jr.playerID = ? AND jr.id != ? AND jr.status = 'PENDING' AND t.leagueID = ?`

	_, err := m.DB.Exec(statement, respondedByUserID, playerID, excludeID, leagueID)
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

package models

import "database/sql"

// DBTX is the subset of *sql.DB's methods a model needs, satisfied by both
// *sql.DB and *sql.Tx — lets a caller opt a model into a transaction (see
// MatchGoalModel/MatchCardModel/PlayerMatchStatModel, used together inside
// one transaction when a match's stats are recomputed) without any of the
// model's other, non-transactional call sites changing, since *sql.DB
// already implements this interface structurally.
type DBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

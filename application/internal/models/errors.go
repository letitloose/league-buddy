package models

import (
	"errors"
)

var (
	ErrNoRecord             = errors.New("models: no matching record found")
	ErrInvalidCredentials   = errors.New("models: invalid credentials")
	ErrDuplicateEmail       = errors.New("models: duplicate email")
	ErrBadData              = errors.New("models: bad data")
	ErrHasDependents        = errors.New("models: record has dependent rows and cannot be deleted")
	ErrDuplicateCaptain     = errors.New("models: player is already captain of another team")
	ErrDuplicateRequest     = errors.New("models: player already has a pending join request")
	ErrDuplicateMembership  = errors.New("models: player is already a member of this team")
	ErrDuplicateLeagueAdmin = errors.New("models: player is already an admin of this league")
)

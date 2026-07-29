package services

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

// PlayerStatInput is one roster row's stat entry on the match-edit form.
type PlayerStatInput struct {
	PlayerID    int
	TeamID      int
	Goals       int
	Assists     int
	YellowCards int
	RedCards    int
}

type MatchForm struct {
	ID         int
	SeasonID   int
	HomeTeamID int
	AwayTeamID int
	LocationID int    // 0 = no location set
	MatchDate  string // "2006-01-02" from <input type=date>
	HomeScore  string // blank = not recorded
	AwayScore  string
	Notes      string
	Stats      []PlayerStatInput
	validator.Validator
}

type MatchService struct {
	*models.MatchModel
	DB *sql.DB
}

// parseRequiredDate parses a "2006-01-02" value, returning ok=false for a
// blank or unparseable input — unlike parseOptionalDate (players.go), a
// match must have a date.
func parseRequiredDate(value string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func validateMatchForm(form *MatchForm, db *sql.DB) (time.Time, sql.NullInt32, sql.NullInt32) {
	matchDate, dateOK := parseRequiredDate(form.MatchDate)
	form.CheckField(dateOK, "matchdate", "You must enter a valid date.")

	if form.HomeTeamID <= 0 || form.AwayTeamID <= 0 {
		form.AddFieldError("hometeamid", "You must choose both teams.")
	} else if form.HomeTeamID == form.AwayTeamID {
		form.AddFieldError("awayteamid", "The home and away teams must be different.")
	}

	homeScore, awayScore, scoresOK := parseScorePair(form.HomeScore, form.AwayScore)
	form.CheckField(scoresOK, "homescore", "Enter both scores or leave both blank.")

	if !form.Valid() {
		return matchDate, homeScore, awayScore
	}

	tm := &models.TeamModel{DB: db}
	homeTeam, err := tm.Get(form.HomeTeamID)
	if err != nil {
		form.AddFieldError("hometeamid", "That team could not be found.")
		return matchDate, homeScore, awayScore
	}
	awayTeam, err := tm.Get(form.AwayTeamID)
	if err != nil {
		form.AddFieldError("awayteamid", "That team could not be found.")
		return matchDate, homeScore, awayScore
	}

	sm := &models.SeasonModel{DB: db}
	season, err := sm.Get(form.SeasonID)
	if err != nil {
		form.AddNonFieldError("That season could not be found.")
		return matchDate, homeScore, awayScore
	}

	if homeTeam.LeagueID != season.LeagueID || awayTeam.LeagueID != season.LeagueID {
		form.AddNonFieldError("Both teams must belong to this season's league.")
	}

	// Individual player goals may fall short of the team's recorded score
	// (an own goal has no scorer to credit), but can never exceed it.
	if homeScore.Valid && awayScore.Valid {
		homeGoals, awayGoals := 0, 0
		for _, s := range form.Stats {
			switch s.TeamID {
			case form.HomeTeamID:
				homeGoals += s.Goals
			case form.AwayTeamID:
				awayGoals += s.Goals
			}
		}
		if homeGoals > int(homeScore.Int32) {
			form.AddNonFieldError(fmt.Sprintf("Player goals for %s (%d) can't exceed the recorded score (%d).", homeTeam.Name, homeGoals, homeScore.Int32))
		}
		if awayGoals > int(awayScore.Int32) {
			form.AddNonFieldError(fmt.Sprintf("Player goals for %s (%d) can't exceed the recorded score (%d).", awayTeam.Name, awayGoals, awayScore.Int32))
		}
	}

	if form.LocationID > 0 {
		locm := &models.LocationModel{DB: db}
		if _, err := locm.Get(form.LocationID); err != nil {
			form.AddFieldError("locationid", "That location could not be found.")
		}
	}

	return matchDate, homeScore, awayScore
}

// parseScorePair enforces "both entered or both blank" and returns them as
// NullInt32s. ok is false if exactly one side was entered, or either side
// doesn't parse as a non-negative integer.
func parseScorePair(homeRaw, awayRaw string) (sql.NullInt32, sql.NullInt32, bool) {
	homeRaw = strings.TrimSpace(homeRaw)
	awayRaw = strings.TrimSpace(awayRaw)

	if homeRaw == "" && awayRaw == "" {
		return sql.NullInt32{}, sql.NullInt32{}, true
	}
	if homeRaw == "" || awayRaw == "" {
		return sql.NullInt32{}, sql.NullInt32{}, false
	}

	home, err := strconv.Atoi(homeRaw)
	if err != nil || home < 0 {
		return sql.NullInt32{}, sql.NullInt32{}, false
	}
	away, err := strconv.Atoi(awayRaw)
	if err != nil || away < 0 {
		return sql.NullInt32{}, sql.NullInt32{}, false
	}

	return sql.NullInt32{Int32: int32(home), Valid: true}, sql.NullInt32{Int32: int32(away), Valid: true}, true
}

func (service *MatchService) CreateMatch(form *MatchForm, actorEmail string) (int, error) {
	matchDate, homeScore, awayScore := validateMatchForm(form, service.DB)
	if !form.Valid() {
		return 0, models.ErrBadData
	}

	id, err := service.Insert(&models.Match{
		SeasonID:   form.SeasonID,
		HomeTeamID: form.HomeTeamID,
		AwayTeamID: form.AwayTeamID,
		MatchDate:  matchDate,
		LocationID: sql.NullInt32{Int32: int32(form.LocationID), Valid: form.LocationID > 0},
		HomeScore:  homeScore,
		AwayScore:  awayScore,
		Notes:      sql.NullString{String: form.Notes, Valid: form.Notes != ""},
	})
	if err != nil {
		return 0, err
	}

	if err := saveMatchStats(service.DB, id, form.Stats); err != nil {
		return 0, err
	}

	cs := &CommonService{DB: service.DB}
	if err := cs.InsertAuditLog(actorEmail, time.Now(), "match created"); err != nil {
		return 0, err
	}

	return id, nil
}

func (service *MatchService) UpdateMatch(form *MatchForm, actorEmail string) error {
	matchDate, homeScore, awayScore := validateMatchForm(form, service.DB)
	if !form.Valid() {
		return models.ErrBadData
	}

	err := service.Update(&models.Match{
		ID:         form.ID,
		SeasonID:   form.SeasonID,
		HomeTeamID: form.HomeTeamID,
		AwayTeamID: form.AwayTeamID,
		MatchDate:  matchDate,
		LocationID: sql.NullInt32{Int32: int32(form.LocationID), Valid: form.LocationID > 0},
		HomeScore:  homeScore,
		AwayScore:  awayScore,
		Notes:      sql.NullString{String: form.Notes, Valid: form.Notes != ""},
	})
	if err != nil {
		return err
	}

	if err := saveMatchStats(service.DB, form.ID, form.Stats); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "match updated")
}

// saveMatchStats upserts every roster row the match-edit page rendered
// (even all-zero ones), so re-submitting the form always reflects exactly
// what's on screen.
func saveMatchStats(db *sql.DB, matchID int, stats []PlayerStatInput) error {
	pmsm := &models.PlayerMatchStatModel{DB: db}
	for _, s := range stats {
		err := pmsm.Upsert(&models.PlayerMatchStat{
			MatchID:     matchID,
			PlayerID:    s.PlayerID,
			TeamID:      s.TeamID,
			Goals:       s.Goals,
			Assists:     s.Assists,
			YellowCards: s.YellowCards,
			RedCards:    s.RedCards,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (service *MatchService) DeleteMatch(id int, actorEmail string) error {
	if _, err := service.Get(id); err != nil {
		return err
	}

	if err := service.Delete(id); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "match deleted")
}

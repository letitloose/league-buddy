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

// GoalInput is one goal row on the match-edit form: always attributed to a
// team, optionally to a scorer and/or assister (0 = unattributed — a team
// may know the other side scored without knowing which of their players
// did).
type GoalInput struct {
	TeamID           int
	ScorerPlayerID   int
	AssisterPlayerID int
}

// CardInput is one card row on the match-edit form: always attributed to a
// team, optionally to a specific player (0 = unattributed).
type CardInput struct {
	TeamID   int
	PlayerID int
	CardType string // "yellow" or "red"
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
	Goals      []GoalInput
	Cards      []CardInput
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

	// Goal rows may fall short of the team's recorded score (an own goal
	// has no scorer to credit), but can never exceed it — each row is one
	// goal, attributed or not, so counting rows (not summing a per-player
	// tally) is the whole check now.
	if homeScore.Valid && awayScore.Valid {
		homeGoals, awayGoals := 0, 0
		for _, g := range form.Goals {
			switch g.TeamID {
			case form.HomeTeamID:
				homeGoals++
			case form.AwayTeamID:
				awayGoals++
			}
		}
		if homeGoals > int(homeScore.Int32) {
			form.AddNonFieldError(fmt.Sprintf("Recorded goals for %s (%d) can't exceed the recorded score (%d).", homeTeam.Name, homeGoals, homeScore.Int32))
		}
		if awayGoals > int(awayScore.Int32) {
			form.AddNonFieldError(fmt.Sprintf("Recorded goals for %s (%d) can't exceed the recorded score (%d).", awayTeam.Name, awayGoals, awayScore.Int32))
		}
	}

	validateMatchEvents(form, db)

	if form.LocationID > 0 {
		locm := &models.LocationModel{DB: db}
		if _, err := locm.Get(form.LocationID); err != nil {
			form.AddFieldError("locationid", "That location could not be found.")
		}
	}

	return matchDate, homeScore, awayScore
}

// validateMatchEvents checks every goal/card row's TeamID is actually one
// of the match's two teams, and — only when a specific player was picked
// (0 means unattributed, always allowed) — that they belong to that row's
// team's roster. A mismatch only happens from a tampered request, since the
// form's dropdowns only ever offer valid combinations, but it's cheap to
// catch rather than trust.
func validateMatchEvents(form *MatchForm, db *sql.DB) {
	tmm := &models.TeamMemberModel{DB: db}

	validTeam := func(teamID int) bool {
		return teamID == form.HomeTeamID || teamID == form.AwayTeamID
	}
	onRoster := func(playerID, teamID int) bool {
		isMember, err := tmm.IsMember(playerID, teamID)
		return err == nil && isMember
	}

	for _, g := range form.Goals {
		if !validTeam(g.TeamID) {
			form.AddNonFieldError("Every goal must belong to one of this match's two teams.")
			continue
		}
		if g.ScorerPlayerID > 0 && !onRoster(g.ScorerPlayerID, g.TeamID) {
			form.AddNonFieldError("A goal's scorer must be on that team's roster.")
		}
		if g.AssisterPlayerID > 0 && !onRoster(g.AssisterPlayerID, g.TeamID) {
			form.AddNonFieldError("A goal's assister must be on that team's roster.")
		}
	}

	for _, c := range form.Cards {
		if !validTeam(c.TeamID) {
			form.AddNonFieldError("Every card must belong to one of this match's two teams.")
			continue
		}
		if c.PlayerID > 0 && !onRoster(c.PlayerID, c.TeamID) {
			form.AddNonFieldError("A card's player must be on that team's roster.")
		}
		if c.CardType != "yellow" && c.CardType != "red" {
			form.AddNonFieldError("A card's type must be yellow or red.")
		}
	}
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

	if err := saveMatchEvents(service.DB, id, form.Goals, form.Cards); err != nil {
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

	if err := saveMatchEvents(service.DB, form.ID, form.Goals, form.Cards); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "match updated")
}

// saveMatchEvents replaces matchID's goal/card rows with exactly what the
// edit form submitted, then recomputes the playerMatchStats cache the
// leaderboard queries read from — every player who scored, assisted, or
// was carded in any of the just-saved rows gets one recomputed line. Runs
// inside one transaction so the events and the cache derived from them are
// never left out of step with each other: either the whole save lands, or
// none of it does.
func saveMatchEvents(db *sql.DB, matchID int, goals []GoalInput, cards []CardInput) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	mgm := &models.MatchGoalModel{DB: tx}
	mcm := &models.MatchCardModel{DB: tx}
	pmsm := &models.PlayerMatchStatModel{DB: tx}

	goalRows := make([]models.MatchGoal, len(goals))
	for i, g := range goals {
		goalRows[i] = models.MatchGoal{
			TeamID:           g.TeamID,
			ScorerPlayerID:   nullPlayerID(g.ScorerPlayerID),
			AssisterPlayerID: nullPlayerID(g.AssisterPlayerID),
		}
	}
	if err := mgm.ReplaceForMatch(matchID, goalRows); err != nil {
		return err
	}

	cardRows := make([]models.MatchCard, len(cards))
	for i, c := range cards {
		cardRows[i] = models.MatchCard{
			TeamID:   c.TeamID,
			PlayerID: nullPlayerID(c.PlayerID),
			CardType: c.CardType,
		}
	}
	if err := mcm.ReplaceForMatch(matchID, cardRows); err != nil {
		return err
	}

	tally := map[int]*models.PlayerMatchStat{}
	statFor := func(playerID, teamID int) *models.PlayerMatchStat {
		stat, ok := tally[playerID]
		if !ok {
			stat = &models.PlayerMatchStat{MatchID: matchID, PlayerID: playerID, TeamID: teamID}
			tally[playerID] = stat
		}
		return stat
	}
	for _, g := range goals {
		if g.ScorerPlayerID > 0 {
			statFor(g.ScorerPlayerID, g.TeamID).Goals++
		}
		if g.AssisterPlayerID > 0 {
			statFor(g.AssisterPlayerID, g.TeamID).Assists++
		}
	}
	for _, c := range cards {
		if c.PlayerID > 0 {
			stat := statFor(c.PlayerID, c.TeamID)
			if c.CardType == "yellow" {
				stat.YellowCards++
			} else {
				stat.RedCards++
			}
		}
	}

	if err := pmsm.DeleteByMatch(matchID); err != nil {
		return err
	}
	for _, stat := range tally {
		if err := pmsm.Upsert(stat); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// nullPlayerID converts the form's "0 = unattributed" convention to a
// NullInt32 for storage.
func nullPlayerID(playerID int) sql.NullInt32 {
	if playerID <= 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(playerID), Valid: true}
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

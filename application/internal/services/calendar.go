package services

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

// CalendarService builds a personal iCalendar (RFC 5545) feed — every
// upcoming match across every team someone is tied to — and manages the
// secret token a phone's calendar app uses to fetch it. Two parallel
// flavors: BuildFeed/EnsureToken for a player (every active-roster team
// they're on), BuildFanFeed/EnsureFanToken for a fan (every team they
// follow) — both funnel into the shared buildFeedForTeams.
type CalendarService struct {
	DB *sql.DB
}

// EnsureToken returns playerID's calendar-feed token, generating and
// saving one on first use (a player who never opens Notification
// Preferences never gets a token at all). Idempotent — a second call
// returns the same token.
func (service *CalendarService) EnsureToken(playerID int) (string, error) {
	pm := &models.PlayerModel{DB: service.DB}
	token, err := pm.GetCalendarToken(playerID)
	if err != nil {
		return "", err
	}
	if token.Valid {
		return token.String, nil
	}
	return service.RegenerateToken(playerID)
}

// RegenerateToken always issues and saves a fresh token, invalidating
// whatever URL the player previously subscribed with — the revocation
// path for a leaked link.
func (service *CalendarService) RegenerateToken(playerID int) (string, error) {
	token, err := generateSecretToken()
	if err != nil {
		return "", err
	}
	pm := &models.PlayerModel{DB: service.DB}
	if err := pm.SetCalendarToken(playerID, token); err != nil {
		return "", err
	}
	return token, nil
}

// EnsureFanToken is EnsureToken's fan counterpart, backed by
// users.fanCalendarToken instead of players.calendarToken.
func (service *CalendarService) EnsureFanToken(userID int) (string, error) {
	um := &models.UserModel{DB: service.DB}
	token, err := um.GetFanCalendarToken(userID)
	if err != nil {
		return "", err
	}
	if token.Valid {
		return token.String, nil
	}
	return service.RegenerateFanToken(userID)
}

// RegenerateFanToken is RegenerateToken's fan counterpart — always issues
// and saves a fresh token, invalidating whatever URL the fan previously
// subscribed with.
func (service *CalendarService) RegenerateFanToken(userID int) (string, error) {
	token, err := generateSecretToken()
	if err != nil {
		return "", err
	}
	um := &models.UserModel{DB: service.DB}
	if err := um.SetFanCalendarToken(userID, token); err != nil {
		return "", err
	}
	return token, nil
}

// hasMatchTime mirrors cmd/web/templates.go's helper of the same name —
// duplicated rather than imported, since internal/services can't depend
// on cmd/web. A match's stored UTC-midnight sentinel (see that file's
// comment for the full rationale) means "no real kickoff time recorded."
func hasMatchTime(t time.Time) bool {
	return !(t.Hour() == 0 && t.Minute() == 0)
}

// icsEscape escapes RFC 5545 TEXT special characters (backslash first,
// so escaping the others doesn't get re-escaped).
func icsEscape(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, "\n", `\n`)
	return replacer.Replace(s)
}

// formatICSAddress joins location's name with address's non-empty parts
// for an ICS LOCATION field — duplicates mapsURL's join logic in
// cmd/web/templates.go (same cross-package constraint as hasMatchTime
// above) rather than the maps-URL-specific query-escaping that helper
// also does.
func formatICSAddress(location *models.Location, address *models.Address) string {
	parts := []string{location.Name}
	if address != nil {
		if address.Address1.Valid && address.Address1.String != "" {
			parts = append(parts, address.Address1.String)
		}
		if address.Address2.Valid && address.Address2.String != "" {
			parts = append(parts, address.Address2.String)
		}
		if address.City.Valid && address.City.String != "" {
			parts = append(parts, address.City.String)
		}
		if address.StateProvince.Valid && address.StateProvince.String != "" {
			parts = append(parts, address.StateProvince.String)
		}
		if address.ZipCode.Valid && address.ZipCode.String != "" {
			parts = append(parts, address.ZipCode.String)
		}
	}
	return strings.Join(parts, ", ")
}

// assumedMatchDuration is used for a timed match's DTEND, since nothing
// in the schema records how long a match actually lasts — long enough to
// cover a full match plus stoppage time, short enough that the event
// still reads as "this evening," not "most of the day."
const assumedMatchDuration = 2 * time.Hour

// writeVEvent appends one match's VEVENT block to b. A match with a real
// kickoff time (hasMatchTime) gets a UTC DTSTART/DTEND assumedMatchDuration
// apart. A match with no real time becomes an all-day VALUE=DATE event
// instead, spanning just its own calendar day.
func writeVEvent(b *strings.Builder, match *models.Match, homeTeam, awayTeam *models.Team, locationLine string, publicHost string) {
	b.WriteString("BEGIN:VEVENT\r\n")
	fmt.Fprintf(b, "UID:match-%d@blametheball\r\n", match.ID)
	fmt.Fprintf(b, "DTSTAMP:%s\r\n", time.Now().UTC().Format("20060102T150405Z"))

	if hasMatchTime(match.MatchDate) {
		fmt.Fprintf(b, "DTSTART:%s\r\n", match.MatchDate.UTC().Format("20060102T150405Z"))
		fmt.Fprintf(b, "DTEND:%s\r\n", match.MatchDate.Add(assumedMatchDuration).UTC().Format("20060102T150405Z"))
	} else {
		fmt.Fprintf(b, "DTSTART;VALUE=DATE:%s\r\n", match.MatchDate.Format("20060102"))
		fmt.Fprintf(b, "DTEND;VALUE=DATE:%s\r\n", match.MatchDate.AddDate(0, 0, 1).Format("20060102"))
	}

	fmt.Fprintf(b, "SUMMARY:%s\r\n", icsEscape(homeTeam.Name+" vs "+awayTeam.Name))
	if locationLine != "" {
		fmt.Fprintf(b, "LOCATION:%s\r\n", icsEscape(locationLine))
	}
	if publicHost != "" {
		fmt.Fprintf(b, "DESCRIPTION:%s\r\n", icsEscape(fmt.Sprintf("https://%s/match/%d", publicHost, match.ID)))
	}
	b.WriteString("END:VEVENT\r\n")
}

// BuildFeed resolves token to a player and returns their upcoming-match
// schedule (across every active-roster team they belong to) as raw
// iCalendar bytes. Returns models.ErrNoRecord for an unknown or revoked
// token. A player on no active team (or with no upcoming matches) still
// gets back a valid, empty calendar rather than an error.
func (service *CalendarService) BuildFeed(token string) ([]byte, error) {
	pm := &models.PlayerModel{DB: service.DB}
	player, err := pm.GetByCalendarToken(token)
	if err != nil {
		return nil, err
	}

	tmm := &models.TeamMemberModel{DB: service.DB}
	teams, err := tmm.GetActiveTeamsForPlayer(player.ID)
	if err != nil {
		return nil, err
	}

	return service.buildFeedForTeams(teams, player.FirstName+" "+player.LastName+" - Matches")
}

// BuildFanFeed is BuildFeed's fan counterpart: resolves token to a Fan
// account (a User with no Player row) and returns the upcoming-match
// schedule across every team they follow. Returns models.ErrNoRecord for
// an unknown or revoked token.
func (service *CalendarService) BuildFanFeed(token string) ([]byte, error) {
	um := &models.UserModel{DB: service.DB}
	user, err := um.GetByFanCalendarToken(token)
	if err != nil {
		return nil, err
	}

	tfm := &models.TeamFanModel{DB: service.DB}
	teams, err := tfm.GetFollowedTeams(user.UserID)
	if err != nil {
		return nil, err
	}

	return service.buildFeedForTeams(teams, "My Followed Teams")
}

// buildFeedForTeams is the shared core of BuildFeed/BuildFanFeed: every
// upcoming match across teams, as raw iCalendar bytes under calendarName.
func (service *CalendarService) buildFeedForTeams(teams []*models.Team, calendarName string) ([]byte, error) {
	teamsByID := make(map[int]*models.Team, len(teams))
	teamIDs := make([]int, len(teams))
	for i, team := range teams {
		teamsByID[team.ID] = team
		teamIDs[i] = team.ID
	}

	mm := &models.MatchModel{DB: service.DB}
	matches, err := mm.GetUpcomingByTeamIDs(teamIDs, dateNDaysOut(time.Now(), 0))
	if err != nil {
		return nil, err
	}

	tm := &models.TeamModel{DB: service.DB}
	locm := &models.LocationModel{DB: service.DB}
	am := &models.AddressModel{DB: service.DB}
	publicHost := os.Getenv("PUBLIC_HOST")

	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Blame the Ball//Match Schedule//EN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	fmt.Fprintf(&b, "X-WR-CALNAME:%s\r\n", icsEscape(calendarName))

	for _, match := range matches {
		homeTeam := teamsByID[match.HomeTeamID]
		if homeTeam == nil {
			homeTeam, err = tm.Get(match.HomeTeamID)
			if err != nil {
				return nil, err
			}
		}
		awayTeam := teamsByID[match.AwayTeamID]
		if awayTeam == nil {
			awayTeam, err = tm.Get(match.AwayTeamID)
			if err != nil {
				return nil, err
			}
		}

		var locationLine string
		if match.LocationID.Valid {
			location, err := locm.Get(int(match.LocationID.Int32))
			if err != nil {
				return nil, err
			}
			address, err := am.Get(location.AddressID)
			if err != nil {
				return nil, err
			}
			locationLine = formatICSAddress(location, address)
		}

		writeVEvent(&b, match, homeTeam, awayTeam, locationLine, publicHost)
	}

	b.WriteString("END:VCALENDAR\r\n")
	return []byte(b.String()), nil
}

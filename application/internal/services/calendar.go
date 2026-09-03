package services

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

// CalendarService builds each player's personal iCalendar (RFC 5545) feed
// — every upcoming match across every active-roster team they're on —
// and manages the secret token a phone's calendar app uses to fetch it.
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

// writeVEvent appends one match's VEVENT block to b. A match with a real
// kickoff time (hasMatchTime) gets a UTC DTSTART with no DTEND/DURATION —
// RFC 5545 treats that as a valid zero-duration event, which is honest
// given nothing in the schema records how long a match actually lasts (no
// invented duration). A match with no real time becomes an all-day
// VALUE=DATE event instead, spanning just its own calendar day.
func writeVEvent(b *strings.Builder, match *models.Match, homeTeam, awayTeam *models.Team, locationLine string, publicHost string) {
	b.WriteString("BEGIN:VEVENT\r\n")
	fmt.Fprintf(b, "UID:match-%d@blametheball\r\n", match.ID)
	fmt.Fprintf(b, "DTSTAMP:%s\r\n", time.Now().UTC().Format("20060102T150405Z"))

	if hasMatchTime(match.MatchDate) {
		fmt.Fprintf(b, "DTSTART:%s\r\n", match.MatchDate.UTC().Format("20060102T150405Z"))
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
	fmt.Fprintf(&b, "X-WR-CALNAME:%s\r\n", icsEscape(player.FirstName+" "+player.LastName+" - Matches"))

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

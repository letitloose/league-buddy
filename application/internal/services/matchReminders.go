package services

import (
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log"
	"os"
	"strings"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

// rsvpDaysOutSchedule is the RSVP reminder countdown: 3 days before a
// match, then 2, then 1 — each day only to roster players who still
// haven't RSVP'd.
var rsvpDaysOutSchedule = []int{3, 2, 1}

// captainMessageDaysOut is when a team's captain gets a one-time nudge to
// add a captain's message for an upcoming match, if they haven't already.
const captainMessageDaysOut = 4

// easternLocation mirrors cmd/web/templates.go's loader — duplicated
// rather than imported, since internal/services can't depend on cmd/web.
var easternLocation = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// MatchReminderService sends both automated match reminders: RSVP nudges
// (SendDueRSVPReminders) and captain's-message nudges
// (SendDueCaptainMessageReminders). Kept email-agnostic in naming
// (matchRSVPReminders, not matchRSVPReminderEmails) so a future SMS
// delivery channel can slot in without renaming anything here.
type MatchReminderService struct {
	DB *sql.DB
	*Email
	SMS     *SMS
	InfoLog *log.Logger
}

// notify sends emailBody/smsBody to recipient according to their saved
// preference for category (defaulting to email if they've never set one
// — see models.NotificationPreferenceModel.GetChannel). If the preference
// wants SMS but the phone isn't currently verified (e.g. verification
// lapsed after a phone-number change since they set the preference), it
// falls back to email rather than silently dropping the reminder. Returns
// whether anything was actually dispatched — false for a player whose
// preference is ChannelOff, which callers use to decide whether this
// counts as a real send (for their "N reminders sent" count and whether
// to record it in matchRSVPReminders/matchCaptainMessageReminders).
func (service *MatchReminderService) notify(category string, recipient *models.Player, emailSubject, emailBody, smsBody string) (bool, error) {
	npm := &models.NotificationPreferenceModel{DB: service.DB}
	channel, err := npm.GetChannel(recipient.ID, category)
	if err != nil {
		return false, err
	}

	wantsEmail := channel == models.ChannelEmail || channel == models.ChannelBoth
	wantsSMS := channel == models.ChannelSMS || channel == models.ChannelBoth
	if wantsSMS && !recipient.PhoneVerifiedAt.Valid {
		wantsSMS = false
		wantsEmail = true
	}

	sent := false

	if wantsEmail && recipient.Email.Valid && recipient.Email.String != "" {
		if service.Email != nil {
			if err := service.SendEmailV2(emailSubject, "", emailBody, recipient.Email.String); err != nil {
				return sent, err
			}
		} else if service.InfoLog != nil {
			service.InfoLog.Printf("no email configured -- reminder for %s: %s", recipient.Email.String, emailSubject)
		}
		sent = true
	}

	if wantsSMS {
		if service.SMS != nil {
			if err := service.SMS.Send(recipient.PhoneNumber.String, smsBody); err != nil {
				return sent, err
			}
		} else if service.InfoLog != nil {
			service.InfoLog.Printf("no SMS provider configured -- text reminder for player %d: %s", recipient.ID, smsBody)
		}
		sent = true
	}

	return sent, nil
}

// dateNDaysOut returns the UTC-midnight date (matching how matches.matchDate
// is stored — see parseRequiredDate in matches.go) that is days days after
// asOf's Eastern calendar date. Anchoring on Eastern time, rather than the
// server's local time, matches how every other human-facing date in this
// app is displayed (cmd/web/templates.go's humanDateTime), so "3 days
// before a Saturday match" means what a player in this league would
// expect regardless of the server's own timezone.
func dateNDaysOut(asOf time.Time, days int) time.Time {
	e := asOf.In(easternLocation)
	today := time.Date(e.Year(), e.Month(), e.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, days)
}

// SendDueRSVPReminders emails every roster player who hasn't RSVP'd yet for
// a match 3/2/1 days out (from asOf's Eastern calendar date), skipping
// anyone already reminded for that day (matchRSVPReminders) and anyone with
// no email on file. Returns how many were sent. One player's send failure
// is logged and skipped, not fatal to the whole run — a background/manual
// job shouldn't abort partway through a day's matches over one bad address.
func (service *MatchReminderService) SendDueRSVPReminders(asOf time.Time) (int, error) {
	mm := &models.MatchModel{DB: service.DB}
	pm := &models.PlayerModel{DB: service.DB}
	rm := &models.RSVPModel{DB: service.DB}
	tm := &models.TeamModel{DB: service.DB}
	mtnm := &models.MatchTeamNoteModel{DB: service.DB}
	mrrm := &models.MatchRSVPReminderModel{DB: service.DB}

	sent := 0
	for _, daysOut := range rsvpDaysOutSchedule {
		matches, err := mm.GetByDate(dateNDaysOut(asOf, daysOut))
		if err != nil {
			return sent, err
		}

		for _, match := range matches {
			homeTeam, err := tm.Get(match.HomeTeamID)
			if err != nil {
				return sent, err
			}
			awayTeam, err := tm.Get(match.AwayTeamID)
			if err != nil {
				return sent, err
			}

			responses, err := rm.ListByMatch(match.ID)
			if err != nil {
				return sent, err
			}
			respondedByPlayer := make(map[int]*models.RSVP, len(responses))
			for _, resp := range responses {
				respondedByPlayer[resp.PlayerID] = resp
			}

			sides := []struct {
				teamID   int
				opponent *models.Team
			}{
				{match.HomeTeamID, awayTeam},
				{match.AwayTeamID, homeTeam},
			}

			// A player on both rosters (rare) gets one email per match, not
			// two.
			remindedThisMatch := map[int]bool{}

			for _, side := range sides {
				roster, err := pm.GetByTeam(side.teamID)
				if err != nil {
					return sent, err
				}
				note, err := mtnm.GetByMatchAndTeam(match.ID, side.teamID)
				if err != nil && !errors.Is(err, models.ErrNoRecord) {
					return sent, err
				}

				for _, player := range roster {
					if remindedThisMatch[player.ID] {
						continue
					}
					if _, responded := respondedByPlayer[player.ID]; responded {
						continue
					}
					hasEmail := player.Email.Valid && player.Email.String != ""
					hasVerifiedPhone := player.PhoneVerifiedAt.Valid
					if !hasEmail && !hasVerifiedPhone {
						continue
					}
					wasSent, err := mrrm.WasSent(match.ID, player.ID, daysOut)
					if err != nil {
						return sent, err
					}
					if wasSent {
						continue
					}

					delivered, err := service.sendRSVPReminder(match, homeTeam, awayTeam, roster, respondedByPlayer, note, player)
					if err != nil {
						if service.InfoLog != nil {
							service.InfoLog.Printf("rsvp reminder: failed to email player %d for match %d: %v", player.ID, match.ID, err)
						}
						continue
					}
					if !delivered {
						// Preference is off — nothing to record, and no
						// reason to stop the other side's roster from
						// re-checking this player's preference too.
						continue
					}
					if err := mrrm.MarkSent(match.ID, player.ID, daysOut); err != nil {
						return sent, err
					}
					remindedThisMatch[player.ID] = true
					sent++
				}
			}
		}
	}

	return sent, nil
}

// captainNameForTeam returns the display name of the captain of whichever
// of homeTeam/awayTeam matches teamID, or "" if that team has no captain
// assigned or the captain isn't found in roster (roster is always that same
// team's own roster, so no extra DB lookup is needed here).
func captainNameForTeam(teamID int, homeTeam, awayTeam *models.Team, roster []*models.Player) string {
	team := homeTeam
	if teamID == awayTeam.ID {
		team = awayTeam
	}
	if !team.CaptainPlayerID.Valid {
		return ""
	}
	for _, p := range roster {
		if p.ID == int(team.CaptainPlayerID.Int32) {
			return p.FirstName + " " + p.LastName
		}
	}
	return ""
}

// buildRSVPReminderContent builds the subject/body shared by every RSVP
// reminder send — the real scheduled ones (sendRSVPReminder) and the
// on-demand preview (SendTestReminder): their team's captain's message (if
// set), an RSVP Now! link, and who on their own team has already responded
// (with messages).
func buildRSVPReminderContent(match *models.Match, homeTeam, awayTeam *models.Team, roster []*models.Player, respondedByPlayer map[int]*models.RSVP, note *models.MatchTeamNote) (subject, body string) {
	matchURL := fmt.Sprintf("https://%s/match/%d", os.Getenv("PUBLIC_HOST"), match.ID)
	dateStr := match.MatchDate.Format("01/02/2006")
	subject = fmt.Sprintf("RSVP for %s vs %s — %s", homeTeam.Name, awayTeam.Name, dateStr)

	var b strings.Builder
	b.WriteString("<html><body>")
	if note != nil && note.CaptainMessage.Valid && note.CaptainMessage.String != "" {
		label := "Message from your captain"
		if captainName := captainNameForTeam(note.TeamID, homeTeam, awayTeam, roster); captainName != "" {
			label = "Message from Captain " + html.EscapeString(captainName)
		}
		fmt.Fprintf(&b, "<p><strong>%s:</strong> %s</p>", label, html.EscapeString(note.CaptainMessage.String))
	}
	fmt.Fprintf(&b, `<p><a href="%s">RSVP Now!</a></p>`, matchURL)

	var confirmed, notAttending []string
	for _, player := range roster {
		resp, ok := respondedByPlayer[player.ID]
		if !ok {
			continue
		}
		line := html.EscapeString(player.FirstName + " " + player.LastName)
		if resp.Message.Valid && resp.Message.String != "" {
			line += fmt.Sprintf(" &mdash; &ldquo;%s&rdquo;", html.EscapeString(resp.Message.String))
		}
		if resp.Status == "yes" {
			confirmed = append(confirmed, line)
		} else {
			notAttending = append(notAttending, line)
		}
	}
	writeList := func(heading string, lines []string) {
		if len(lines) == 0 {
			return
		}
		fmt.Fprintf(&b, "<p><strong>%s</strong></p><ul>", heading)
		for _, line := range lines {
			b.WriteString("<li>" + line + "</li>")
		}
		b.WriteString("</ul>")
	}
	writeList("Confirmed", confirmed)
	writeList("Not Attending", notAttending)
	b.WriteString("</body></html>")

	return subject, b.String()
}

// buildRSVPReminderSMSBody is the RSVP reminder's short text-message
// counterpart to buildRSVPReminderContent's HTML email body — just the
// nudge and the link, no confirmed/not-attending detail (that stays
// in-app, consistent with texting being one-way).
func buildRSVPReminderSMSBody(match *models.Match, homeTeam, awayTeam *models.Team) string {
	matchURL := fmt.Sprintf("https://%s/match/%d", os.Getenv("PUBLIC_HOST"), match.ID)
	dateStr := match.MatchDate.Format("01/02/2006")
	return fmt.Sprintf("RSVP for %s vs %s on %s: %s", homeTeam.Name, awayTeam.Name, dateStr, matchURL)
}

// sendRSVPReminder sends recipient a real, scheduled RSVP reminder for
// match, by whichever channel(s) they've chosen (see notify) — returns
// whether anything was actually sent (false if their preference is off).
func (service *MatchReminderService) sendRSVPReminder(match *models.Match, homeTeam, awayTeam *models.Team, roster []*models.Player, respondedByPlayer map[int]*models.RSVP, note *models.MatchTeamNote, recipient *models.Player) (bool, error) {
	subject, body := buildRSVPReminderContent(match, homeTeam, awayTeam, roster, respondedByPlayer, note)
	smsBody := buildRSVPReminderSMSBody(match, homeTeam, awayTeam)
	return service.notify(models.CategoryRSVPReminder, recipient, subject, body, smsBody)
}

// SendTestReminder builds the same RSVP-reminder content real reminders use
// for teamID's side of matchID, and sends it — subject prefixed "[TEST]" —
// to each of addresses. Deliberately bypasses every gate real reminders
// use (days-out schedule, already-responded, already-reminded) and records
// nothing to matchRSVPReminders, since this is a manual preview/validation
// send a captain triggers on demand, not a scheduled one. addresses need
// not belong to actual roster players — a captain may want to preview it
// in their own inbox.
func (service *MatchReminderService) SendTestReminder(matchID, teamID int, addresses []string) error {
	mm := &models.MatchModel{DB: service.DB}
	match, err := mm.Get(matchID)
	if err != nil {
		return err
	}

	tm := &models.TeamModel{DB: service.DB}
	homeTeam, err := tm.Get(match.HomeTeamID)
	if err != nil {
		return err
	}
	awayTeam, err := tm.Get(match.AwayTeamID)
	if err != nil {
		return err
	}

	pm := &models.PlayerModel{DB: service.DB}
	roster, err := pm.GetByTeam(teamID)
	if err != nil {
		return err
	}

	rm := &models.RSVPModel{DB: service.DB}
	responses, err := rm.ListByMatch(match.ID)
	if err != nil {
		return err
	}
	respondedByPlayer := make(map[int]*models.RSVP, len(responses))
	for _, resp := range responses {
		respondedByPlayer[resp.PlayerID] = resp
	}

	mtnm := &models.MatchTeamNoteModel{DB: service.DB}
	note, err := mtnm.GetByMatchAndTeam(match.ID, teamID)
	if err != nil && !errors.Is(err, models.ErrNoRecord) {
		return err
	}

	subject, body := buildRSVPReminderContent(match, homeTeam, awayTeam, roster, respondedByPlayer, note)
	subject = "[TEST] " + subject

	for _, addr := range addresses {
		if service.Email != nil {
			if err := service.SendEmailV2(subject, "", body, addr); err != nil {
				return err
			}
		} else if service.InfoLog != nil {
			service.InfoLog.Printf("no email configured -- test reminder for %s (match %d, team %d)", addr, match.ID, teamID)
		}
	}
	return nil
}

// SendTestReminderSMS is SendTestReminder's SMS counterpart. Unlike
// SendTestReminder (which accepts arbitrary addresses), every playerID is
// re-checked here against teamID's roster and PhoneVerifiedAt regardless
// of what the caller passed — this is the one place a consent mistake
// would actually text someone, so it doesn't trust the picker alone.
// Returns how many were sent.
func (service *MatchReminderService) SendTestReminderSMS(matchID, teamID int, playerIDs []int) (int, error) {
	mm := &models.MatchModel{DB: service.DB}
	match, err := mm.Get(matchID)
	if err != nil {
		return 0, err
	}

	tm := &models.TeamModel{DB: service.DB}
	homeTeam, err := tm.Get(match.HomeTeamID)
	if err != nil {
		return 0, err
	}
	awayTeam, err := tm.Get(match.AwayTeamID)
	if err != nil {
		return 0, err
	}

	smsBody := "[TEST] " + buildRSVPReminderSMSBody(match, homeTeam, awayTeam)

	pm := &models.PlayerModel{DB: service.DB}
	tmm := &models.TeamMemberModel{DB: service.DB}

	sent := 0
	for _, playerID := range playerIDs {
		isMember, err := tmm.IsMember(playerID, teamID)
		if err != nil {
			return sent, err
		}
		if !isMember {
			continue
		}
		player, err := pm.Get(playerID)
		if err != nil {
			if errors.Is(err, models.ErrNoRecord) {
				continue
			}
			return sent, err
		}
		if !player.PhoneVerifiedAt.Valid {
			continue
		}

		if service.SMS != nil {
			if err := service.SMS.Send(player.PhoneNumber.String, smsBody); err != nil {
				return sent, err
			}
		} else if service.InfoLog != nil {
			service.InfoLog.Printf("no SMS provider configured -- test text for player %d (match %d, team %d)", playerID, match.ID, teamID)
		}
		sent++
	}
	return sent, nil
}

// SendDueCaptainMessageReminders emails a team's captain, once, 4 days
// before a match, if that team hasn't yet set a captain's message for it —
// skipped entirely if the team has no captain assigned or the captain has
// no email on file.
func (service *MatchReminderService) SendDueCaptainMessageReminders(asOf time.Time) (int, error) {
	mm := &models.MatchModel{DB: service.DB}
	tm := &models.TeamModel{DB: service.DB}
	pm := &models.PlayerModel{DB: service.DB}
	mtnm := &models.MatchTeamNoteModel{DB: service.DB}
	mcrm := &models.MatchCaptainMessageReminderModel{DB: service.DB}

	matches, err := mm.GetByDate(dateNDaysOut(asOf, captainMessageDaysOut))
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, match := range matches {
		homeTeam, err := tm.Get(match.HomeTeamID)
		if err != nil {
			return sent, err
		}
		awayTeam, err := tm.Get(match.AwayTeamID)
		if err != nil {
			return sent, err
		}

		sides := []struct{ team, opponent *models.Team }{
			{homeTeam, awayTeam},
			{awayTeam, homeTeam},
		}

		for _, side := range sides {
			if !side.team.CaptainPlayerID.Valid {
				continue
			}
			wasSent, err := mcrm.WasSent(match.ID, side.team.ID)
			if err != nil {
				return sent, err
			}
			if wasSent {
				continue
			}
			note, err := mtnm.GetByMatchAndTeam(match.ID, side.team.ID)
			if err != nil && !errors.Is(err, models.ErrNoRecord) {
				return sent, err
			}
			if note != nil && note.CaptainMessage.Valid && note.CaptainMessage.String != "" {
				continue
			}
			captain, err := pm.Get(int(side.team.CaptainPlayerID.Int32))
			if err != nil {
				return sent, err
			}
			hasEmail := captain.Email.Valid && captain.Email.String != ""
			hasVerifiedPhone := captain.PhoneVerifiedAt.Valid
			if !hasEmail && !hasVerifiedPhone {
				continue
			}

			delivered, err := service.sendCaptainMessageReminder(match, side.team, side.opponent, captain)
			if err != nil {
				if service.InfoLog != nil {
					service.InfoLog.Printf("captain-message reminder: failed to email captain %d for match %d: %v", captain.ID, match.ID, err)
				}
				continue
			}
			if !delivered {
				continue
			}
			if err := mcrm.MarkSent(match.ID, side.team.ID); err != nil {
				return sent, err
			}
			sent++
		}
	}

	return sent, nil
}

// sendCaptainMessageReminder returns whether anything was actually sent
// (false if the captain's preference is off) — see notify.
func (service *MatchReminderService) sendCaptainMessageReminder(match *models.Match, team, opponent *models.Team, captain *models.Player) (bool, error) {
	matchURL := fmt.Sprintf("https://%s/match/%d", os.Getenv("PUBLIC_HOST"), match.ID)
	dateStr := match.MatchDate.Format("01/02/2006")
	subject := fmt.Sprintf("Add a message for %s's match vs %s — %s", team.Name, opponent.Name, dateStr)
	body := fmt.Sprintf(
		`<html><body><p>Your team plays %s on %s. <a href="%s">Add a message for your team</a> to include in their RSVP reminder emails.</p></body></html>`,
		html.EscapeString(opponent.Name), dateStr, matchURL)
	smsBody := fmt.Sprintf("Add a message for %s's match vs %s on %s: %s", team.Name, opponent.Name, dateStr, matchURL)

	return service.notify(models.CategoryCaptainMessageReminder, captain, subject, body, smsBody)
}

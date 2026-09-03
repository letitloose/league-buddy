package main

import "time"

// reminderPollInterval is how often runDailyReminderScheduler checks for
// due reminders. Each team now has its own configurable ReminderTime
// (see models.Team), so a single fixed daily firing no longer works —
// polling this often catches every team's chosen time within a reasonably
// tight window without meaningfully increasing load (each poll is a
// handful of cheap queries; matchRSVPReminders/matchCaptainMessageReminders
// already dedup a team/match/day so an over-frequent poll never double
// sends).
const reminderPollInterval = 15 * time.Minute

// runDailyReminderScheduler sends both match reminder jobs (RSVP nudges and
// captain's-message nudges) every reminderPollInterval. Runs for the life
// of the process as a background goroutine — one failed run is logged and
// never brings the server down, since it's purely additive to the app's
// normal request handling.
func (app *application) runDailyReminderScheduler() {
	for {
		app.sendDueMatchReminders()
		time.Sleep(reminderPollInterval)
	}
}

// sendDueMatchReminders runs both MatchReminderService jobs and logs their
// results — shared by the poller and the admin manual-trigger route
// (matchReminderTrigger), so both report the same counts.
func (app *application) sendDueMatchReminders() (rsvpCount, captainMessageCount int) {
	rsvpCount, err := app.matchReminderService.SendDueRSVPReminders(time.Now())
	if err != nil {
		app.errorLog.Println(err)
	}

	captainMessageCount, err = app.matchReminderService.SendDueCaptainMessageReminders(time.Now())
	if err != nil {
		app.errorLog.Println(err)
	}

	app.infoLog.Printf("match reminders: sent %d RSVP reminder(s), %d captain-message reminder(s)", rsvpCount, captainMessageCount)
	return rsvpCount, captainMessageCount
}

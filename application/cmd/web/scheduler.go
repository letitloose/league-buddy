package main

import "time"

// runDailyReminderScheduler sends both match reminder jobs (RSVP nudges and
// captain's-message nudges) once a day at 9am Eastern, then repeats every
// 24h. Runs for the life of the process as a background goroutine — one
// failed run is logged and never brings the server down, since it's purely
// additive to the app's normal request handling.
func (app *application) runDailyReminderScheduler() {
	for {
		time.Sleep(durationUntilNext9amEastern(time.Now()))
		app.sendDueMatchReminders()
		time.Sleep(24 * time.Hour)
	}
}

// durationUntilNext9amEastern returns how long to sleep from now until the
// next 9am US Eastern — today's if it hasn't happened yet, tomorrow's
// otherwise.
func durationUntilNext9amEastern(now time.Time) time.Duration {
	e := now.In(easternLocation)
	next := time.Date(e.Year(), e.Month(), e.Day(), 9, 0, 0, 0, easternLocation)
	if !next.After(e) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(e)
}

// sendDueMatchReminders runs both MatchReminderService jobs and logs their
// results — shared by the daily scheduler and the admin manual-trigger
// route (matchReminderTrigger), so both report the same counts.
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

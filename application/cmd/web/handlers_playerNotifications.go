package main

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"github.com/letitloose/league-buddy/internal/models"
)

// playerNotificationsData is the shape player-notifications.html needs:
// the player, their phone-verification status, their current channel for
// each notification category (defaulting to "email" — see
// models.NotificationPreferenceModel.GetChannel), and their calendar-feed
// link. SMSFeatureEnabled gates the template's Phone/Reminder-Delivery
// cards specifically — unlike before, the page itself is always
// reachable (see requireOwnPlayer) since the Calendar card underneath is
// useful with no SMS provider configured at all.
type playerNotificationsData struct {
	Player                *models.Player
	SMSFeatureEnabled     bool
	PhoneVerified         bool
	HasPendingCode        bool
	RSVPChannel           string
	CaptainMessageChannel string
	// CalendarFeedURL (webcal://) is the tap-to-subscribe link — template.URL,
	// not a plain string, because html/template's default safe-URL-scheme
	// allowlist doesn't include "webcal" and would otherwise silently
	// replace the href with the harmless-but-broken "#ZgotmplZ" sentinel
	// (a same-page anchor that does nothing when tapped). Safe to mark
	// trusted here since it's built entirely from PUBLIC_HOST and a
	// server-generated token, never user input. CalendarFeedHTTPSURL is
	// the same feed as a plain https:// link (already an allowed scheme,
	// so a plain string is fine), for copy-pasting into a calendar app's
	// "Add calendar by URL" option as a fallback.
	CalendarFeedURL      template.URL
	CalendarFeedHTTPSURL string
}

// requireOwnPlayer resolves the :id route param and fetches that player,
// but — unlike canManagePlayer, which also allows an admin or a
// captain/league-admin of any team the player is on — only lets the
// logged-in user's own linked player through. Notification consent isn't
// something anyone else can grant on a player's behalf.
func (app *application) requireOwnPlayer(w http.ResponseWriter, r *http.Request) (*models.Player, bool) {
	params := httprouter.ParamsFromContext(r.Context())
	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return nil, false
	}
	if app.getPlayerID(r) != id {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return nil, false
	}

	player, err := app.playerService.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return nil, false
	}
	return player, true
}

func (app *application) playerNotifications(w http.ResponseWriter, r *http.Request) {
	player, ok := app.requireOwnPlayer(w, r)
	if !ok {
		return
	}

	pageData := &playerNotificationsData{
		Player: player,
		// TODO(sms): forced off regardless of app.smsFeatureEnabled() —
		// phone verification/SMS reminders are hidden from this page
		// until SMS development is finished. Restore
		// `app.smsFeatureEnabled()` here (and drop this comment) once
		// that's ready; nothing else needs to change.
		SMSFeatureEnabled: false,
	}

	if pageData.SMSFeatureEnabled {
		rsvpChannel, err := app.notificationPreferenceService.GetChannel(player.ID, models.CategoryRSVPReminder)
		if err != nil {
			app.serverError(w, err)
			return
		}
		captainChannel, err := app.notificationPreferenceService.GetChannel(player.ID, models.CategoryCaptainMessageReminder)
		if err != nil {
			app.serverError(w, err)
			return
		}
		pageData.PhoneVerified = player.PhoneVerifiedAt.Valid
		pageData.HasPendingCode = player.PhoneVerificationCode.Valid
		pageData.RSVPChannel = rsvpChannel
		pageData.CaptainMessageChannel = captainChannel
	}

	token, err := app.calendarService.EnsureToken(player.ID)
	if err != nil {
		app.serverError(w, err)
		return
	}
	feedPath := fmt.Sprintf("/calendar/%s/schedule.ics", token)
	pageData.CalendarFeedURL = template.URL("webcal://" + os.Getenv("PUBLIC_HOST") + feedPath)
	pageData.CalendarFeedHTTPSURL = "https://" + os.Getenv("PUBLIC_HOST") + feedPath

	data := app.newTemplateData(r)
	data.Data = pageData
	data.Breadcrumbs = []Breadcrumb{
		{Label: player.FirstName + " " + player.LastName, URL: fmt.Sprintf("/player/view/%d", player.ID)},
		{Label: "Notification Preferences"},
	}

	app.render(w, http.StatusOK, "player-notifications.html", data)
}

// playerCalendarRegenerate reissues player's calendar-feed token,
// invalidating whatever URL they'd previously subscribed with — the
// revocation path if a link ever leaks.
func (app *application) playerCalendarRegenerate(w http.ResponseWriter, r *http.Request) {
	player, ok := app.requireOwnPlayer(w, r)
	if !ok {
		return
	}

	if _, err := app.calendarService.RegenerateToken(player.ID); err != nil {
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Calendar link regenerated — you'll need to re-subscribe on your phone.")
	http.Redirect(w, r, fmt.Sprintf("/player/notifications/%d", player.ID), http.StatusSeeOther)
}

func (app *application) playerPhoneVerificationRequest(w http.ResponseWriter, r *http.Request) {
	player, ok := app.requireOwnPlayer(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	err := app.playerService.RequestPhoneVerification(player.ID, r.PostForm.Get("phonenumber"))
	if err != nil {
		switch {
		case errors.Is(err, models.ErrBadData):
			app.sessionManager.Put(r.Context(), "flash", "Enter a valid 10-digit phone number.")
		case errors.Is(err, models.ErrVerificationCooldown):
			app.sessionManager.Put(r.Context(), "flash", "Please wait a minute before requesting another code.")
		default:
			app.serverError(w, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/player/notifications/%d", player.ID), http.StatusSeeOther)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", "A verification code has been texted to that number.")
	http.Redirect(w, r, fmt.Sprintf("/player/notifications/%d", player.ID), http.StatusSeeOther)
}

func (app *application) playerPhoneVerificationConfirm(w http.ResponseWriter, r *http.Request) {
	player, ok := app.requireOwnPlayer(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	verified, err := app.playerService.ConfirmPhoneVerification(player.ID, r.PostForm.Get("code"))
	if err != nil {
		app.serverError(w, err)
		return
	}
	if verified {
		app.sessionManager.Put(r.Context(), "flash", "Phone number verified!")
	} else {
		app.sessionManager.Put(r.Context(), "flash", "That code is incorrect or has expired.")
	}
	http.Redirect(w, r, fmt.Sprintf("/player/notifications/%d", player.ID), http.StatusSeeOther)
}

func (app *application) playerNotificationPreferencesSave(w http.ResponseWriter, r *http.Request) {
	player, ok := app.requireOwnPlayer(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	updates := map[string]string{
		models.CategoryRSVPReminder:           r.PostForm.Get("rsvpChannel"),
		models.CategoryCaptainMessageReminder: r.PostForm.Get("captainMessageChannel"),
	}
	for category, channel := range updates {
		if channel == "" {
			continue
		}
		if err := app.notificationPreferenceService.SetPreference(player.ID, category, channel); err != nil {
			if errors.Is(err, models.ErrBadData) {
				app.sessionManager.Put(r.Context(), "flash", "Verify your phone number before choosing text for a notification.")
				http.Redirect(w, r, fmt.Sprintf("/player/notifications/%d", player.ID), http.StatusSeeOther)
				return
			}
			app.serverError(w, err)
			return
		}
	}

	app.sessionManager.Put(r.Context(), "flash", "Notification preferences saved.")
	http.Redirect(w, r, fmt.Sprintf("/player/notifications/%d", player.ID), http.StatusSeeOther)
}

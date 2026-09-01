package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"github.com/letitloose/league-buddy/internal/models"
)

// playerNotificationsData is the shape player-notifications.html needs:
// the player, their phone-verification status, and their current channel
// for each notification category (defaulting to "email" — see
// models.NotificationPreferenceModel.GetChannel).
type playerNotificationsData struct {
	Player                *models.Player
	PhoneVerified         bool
	HasPendingCode        bool
	RSVPChannel           string
	CaptainMessageChannel string
}

// requireOwnPlayer resolves the :id route param and fetches that player,
// but — unlike canManagePlayer, which also allows an admin or a
// captain/league-admin of any team the player is on — only lets the
// logged-in user's own linked player through. Notification consent isn't
// something anyone else can grant on a player's behalf.
//
// Also 404s the whole notifications feature while no SMS provider is
// configured (smsFeatureEnabled) — the page's whole point is choosing a
// text-delivery channel, so there's nothing useful to show, and the link
// to it is already hidden (see player-view.html), so a direct hit here
// should look like the feature doesn't exist rather than half-work.
func (app *application) requireOwnPlayer(w http.ResponseWriter, r *http.Request) (*models.Player, bool) {
	if !app.smsFeatureEnabled() {
		app.notFound(w)
		return nil, false
	}

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

	data := app.newTemplateData(r)
	data.Data = &playerNotificationsData{
		Player:                player,
		PhoneVerified:         player.PhoneVerifiedAt.Valid,
		HasPendingCode:        player.PhoneVerificationCode.Valid,
		RSVPChannel:           rsvpChannel,
		CaptainMessageChannel: captainChannel,
	}
	data.Breadcrumbs = []Breadcrumb{
		{Label: player.FirstName + " " + player.LastName, URL: fmt.Sprintf("/player/view/%d", player.ID)},
		{Label: "Notification Preferences"},
	}

	app.render(w, http.StatusOK, "player-notifications.html", data)
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

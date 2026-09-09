package services

import (
	"testing"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestGetChannelDefaultsToEmail(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	npm := &models.NotificationPreferenceModel{DB: db}

	id, err := players.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	channel, err := npm.GetChannel(id, models.CategoryRSVPReminder)
	if err != nil {
		t.Fatal(err)
	}
	if channel != models.ChannelEmail {
		t.Fatalf("expected default channel %q, got %q", models.ChannelEmail, channel)
	}
}

func TestSetPreferenceRejectsSMSUntilVerifiedAndOptedIn(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	npm := &models.NotificationPreferenceModel{DB: db}
	prefService := NotificationPreferenceService{NotificationPreferenceModel: npm, DB: db}

	id, err := players.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelSMS); err != models.ErrBadData {
		t.Fatalf("expected ErrBadData for an unverified, non-opted-in phone, got %v", err)
	}
	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelBoth); err != models.ErrBadData {
		t.Fatalf("expected ErrBadData for ChannelBoth with an unverified, non-opted-in phone, got %v", err)
	}

	// Email/Off never require verification or opt-in.
	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelOff); err != nil {
		t.Fatal(err)
	}

	if err := players.SetPhoneVerificationCode(id, "123456", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := players.ConfirmPhoneVerified(id); err != nil {
		t.Fatal(err)
	}

	// Verified, but SMS program consent not yet given — still rejected.
	// Verification alone only proves the player controls the number, not
	// that they've agreed to be texted.
	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelSMS); err != models.ErrBadData {
		t.Fatalf("expected ErrBadData for a verified phone with no SMS opt-in, got %v", err)
	}

	if err := players.SetSMSOptIn(id, true); err != nil {
		t.Fatal(err)
	}

	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelSMS); err != nil {
		t.Fatal(err)
	}
	channel, err := npm.GetChannel(id, models.CategoryRSVPReminder)
	if err != nil {
		t.Fatal(err)
	}
	if channel != models.ChannelSMS {
		t.Fatalf("expected channel %q after verifying and opting in, got %q", models.ChannelSMS, channel)
	}
}

// Once granted, SMS opt-in can be revoked independently of phone
// verification — SetPreference must reject sms/both again immediately,
// without needing to re-verify the (still-valid) phone number.
func TestSetPreferenceRejectsSMSAfterOptOut(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	npm := &models.NotificationPreferenceModel{DB: db}
	prefService := NotificationPreferenceService{NotificationPreferenceModel: npm, DB: db}

	id, err := players.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}
	if err := players.SetPhoneVerificationCode(id, "123456", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := players.ConfirmPhoneVerified(id); err != nil {
		t.Fatal(err)
	}
	if err := players.SetSMSOptIn(id, true); err != nil {
		t.Fatal(err)
	}
	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelSMS); err != nil {
		t.Fatal(err)
	}

	if err := players.SetSMSOptIn(id, false); err != nil {
		t.Fatal(err)
	}

	player, err := players.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !player.PhoneVerifiedAt.Valid {
		t.Fatal("expected opting out to leave phone verification untouched")
	}
	if player.SMSOptInAt.Valid {
		t.Fatal("expected opting out to clear SMSOptInAt")
	}

	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelSMS); err != models.ErrBadData {
		t.Fatalf("expected ErrBadData for sms after opting out, got %v", err)
	}
}

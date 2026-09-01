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

func TestSetPreferenceRejectsSMSUntilPhoneVerified(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	npm := &models.NotificationPreferenceModel{DB: db}
	prefService := NotificationPreferenceService{NotificationPreferenceModel: npm, DB: db}

	id, err := players.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelSMS); err != models.ErrBadData {
		t.Fatalf("expected ErrBadData for an unverified phone, got %v", err)
	}
	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelBoth); err != models.ErrBadData {
		t.Fatalf("expected ErrBadData for ChannelBoth with an unverified phone, got %v", err)
	}

	// Email/Off never require verification.
	if err := prefService.SetPreference(id, models.CategoryRSVPReminder, models.ChannelOff); err != nil {
		t.Fatal(err)
	}

	if err := players.SetPhoneVerificationCode(id, "123456", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := players.ConfirmPhoneVerified(id); err != nil {
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
		t.Fatalf("expected channel %q after verifying, got %q", models.ChannelSMS, channel)
	}
}

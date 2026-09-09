package services

import (
	"database/sql"

	"github.com/letitloose/league-buddy/internal/models"
)

type NotificationPreferenceService struct {
	*models.NotificationPreferenceModel
	DB *sql.DB
}

// SetPreference upserts playerID's channel for category, rejecting
// ChannelSMS/ChannelBoth unless the player's phone is currently verified
// AND they've opted into the SMS program (see Player.SMSOptInAt) — the one
// place this consent rule is enforced server-side, regardless of what the
// form itself renders. Verification alone isn't enough: it only proves the
// player controls the number, not that they've agreed to be texted.
func (service *NotificationPreferenceService) SetPreference(playerID int, category, channel string) error {
	switch channel {
	case models.ChannelEmail, models.ChannelOff:
		// No verification or opt-in needed for these.
	case models.ChannelSMS, models.ChannelBoth:
		pm := &models.PlayerModel{DB: service.DB}
		player, err := pm.Get(playerID)
		if err != nil {
			return err
		}
		if !player.PhoneVerifiedAt.Valid || !player.SMSOptInAt.Valid {
			return models.ErrBadData
		}
	default:
		return models.ErrBadData
	}

	return service.SetChannel(playerID, category, channel)
}

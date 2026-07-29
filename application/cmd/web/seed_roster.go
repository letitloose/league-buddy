package main

import (
	"database/sql"

	"github.com/letitloose/league-buddy/internal/models"
)

// colonialRosterPlayers is Colonial FC's (team 1) real roster the site
// owner provided, minus Lou Garwood — he's handled separately in
// seedColonialRoster since he's already seeded as the system admin (see
// seedAdminProfile/LEAGUEBUDDYUSER) and additionally becomes league 1's
// admin and team 1's captain. Every player here gets a Player row plus
// roster membership only, no login — same as the historical opponent teams
// in seed_historical.go — so a future signup with a matching email
// auto-links via UserService.linkOrCreatePlayer.
var colonialRosterPlayers = []struct{ FirstName, LastName, Email string }{
	{"Rob", "Alund", "alund.robert@gmail.com"},
	{"James", "Alund", "jimithang17@aol.com"},
	{"Johnny", "Alund", "johnmalund@gmail.com"},
	{"Tucker", "Beestar", "tucker@beestera.com"},
	{"Brian", "Bould", "brian.bould@gmail.com"},
	{"Chris", "Burke", "christopherburkeesq@gmail.com"},
	{"Cheyenne", "Burke", "cheyburke10@gmail.com"},
	{"William", "Bussert", "william.c.bussert@gmail.com"},
	{"Levi", "Christman", "lmchristman10@gmail.com"},
	{"Matt", "Gasbarro", "gasbarro.matt@gmail.com"},
	{"Heath", "Heimroth", "heathheimroth@gmail.com"},
	{"Mike", "Hulse", "Mikehulse29@hotmail.com"},
	{"John", "Iapoce", "Johniapoce23@yahoo.com"},
	{"Rick", "Johnson", "rcjohnso@us.ibm.com"},
	{"Sean", "Lanza", "slanza94@gmail.com"},
	{"Justin", "Myers", "Jmyers1115@gmail.com"},
	{"Chris", "Netzbend", "netzband56@gmail.com"},
	{"Greg", "Nickson", "ggnickson@yahoo.com"},
	{"James", "Riccardi", "jimphilric@gmail.com"},
	{"Brian", "Shoemaker", "brishoemaker20@gmail.com"},
	{"Kurtis", "Smith", "kurtis@beestera.com"},
	{"Matt", "Snow", "Matthewsnw@gmail.com"},
	{"Ken", "Sochor", "ksochor1@yahoo.com"},
	{"Mike", "Story", "story20@gmail.com"},
	{"Devin", "Tomson", "dtomson80@gmail.com"},
}

// seedColonialRoster makes adminEmail's already-seeded player (Lou Garwood)
// league 1's admin and team 1's captain, then adds every other Colonial FC
// player as a plain roster member. Dev-only, called from reset() — never
// runs in tests, same as seedHistoricalSeasons.
func (app *application) seedColonialRoster(adminEmail string) error {
	if adminEmail == "" {
		return nil
	}

	db := app.playerService.DB
	pm := &models.PlayerModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}

	admin, err := pm.GetByEmail(adminEmail)
	if err != nil {
		return err
	}
	if err := tmm.AddMembership(admin.ID, 1); err != nil {
		return err
	}

	lam := &models.LeagueAdminModel{DB: db}
	if err := lam.AddAdmin(admin.ID, 1); err != nil {
		return err
	}

	tm := &models.TeamModel{DB: db}
	if err := tm.SetCaptain(1, sql.NullInt32{Int32: int32(admin.ID), Valid: true}); err != nil {
		return err
	}

	for _, p := range colonialRosterPlayers {
		playerID, err := pm.Insert(&models.Player{
			FirstName: p.FirstName,
			LastName:  p.LastName,
			Email:     sql.NullString{String: p.Email, Valid: true},
		})
		if err != nil {
			return err
		}
		if err := tmm.AddMembership(playerID, 1); err != nil {
			return err
		}
	}

	return nil
}

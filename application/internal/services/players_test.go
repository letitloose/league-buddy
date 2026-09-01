package services

import (
	"database/sql"
	"testing"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestAddPlayer(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	form := &PlayerForm{
		FirstName: "Lou",
		LastName:  "Garwood",
		Address1:  "123 Main St",
		City:      "Troy",
		Email:     "lou@example.com",
	}

	id, err := playerService.AddPlayer(1, form, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	player, err := players.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Lou"
	if player.FirstName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, player.FirstName)
	}
	if !player.AddressID.Valid {
		t.Fatal("expected an address to have been created")
	}

	tmm := &models.TeamMemberModel{DB: db}
	isMember, err := tmm.IsMember(id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected player to be a member of team 1")
	}
}

func TestAddPlayerMissingRequiredFields(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	form := &PlayerForm{}

	_, err := playerService.AddPlayer(1, form, "admin@example.com")
	if err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

func TestAddPlayerBadTeam(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	form := &PlayerForm{FirstName: "Lou", LastName: "Garwood"}

	_, err := playerService.AddPlayer(9999, form, "admin@example.com")
	if err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestUpdatePlayer(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Lou", LastName: "Garwood"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	form := &PlayerForm{ID: id, FirstName: "Lou", LastName: "Buddy", Address1: "456 Oak Ave", City: "Albany"}
	if err := playerService.UpdatePlayer(form, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	player, err := players.Get(id)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Buddy"
	if player.LastName != expected {
		t.Fatalf("wrong! expected %s but got %s", expected, player.LastName)
	}
	if !player.AddressID.Valid {
		t.Fatal("expected an address to have been created on update")
	}
}

func TestDeletePlayer(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Lou", LastName: "Garwood"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := playerService.DeletePlayer(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	_, err = players.Get(id)
	if err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

// A player with an address on file (e.g. one filled out via the full
// player-update form, or the site-owner's own seeded profile) must still
// delete cleanly. fk_players_address blocks deleting an address a player
// row still references, so the player row has to go first — this
// regression-tests that the two are deleted in the right order (an earlier
// version deleted the address first via a subquery through the still-extant
// player row, which is exactly backwards and failed with a 1451).
func TestDeletePlayerWithAddress(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}
	am := &models.AddressModel{DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{
		FirstName: "Lou", LastName: "Garwood",
		Address1: "100 Phillips Rd", City: "East Greenbush", StateProvince: "NY", ZipCode: "12061",
	}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	player, err := players.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !player.AddressID.Valid {
		t.Fatal("expected an address to have been created")
	}
	addressID := int(player.AddressID.Int32)

	if err := playerService.DeletePlayer(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	if _, err := players.Get(id); err != models.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord for the deleted player, got %v", err)
	}
	if _, err := am.Get(addressID); err != models.ErrNoRecord {
		t.Fatalf("expected the player's address to be deleted too, got %v", err)
	}
}

func TestRemoveFromRoster(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}
	tmm := &models.TeamMemberModel{DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Lou", LastName: "Garwood"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := playerService.RemoveFromRoster(id, 1, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	isMember, err := tmm.IsMember(id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if isMember {
		t.Fatal("expected membership to be removed")
	}

	// The player record itself must survive — RemoveFromRoster is not a
	// destructive action.
	if _, err := players.Get(id); err != nil {
		t.Fatalf("expected player record to still exist, got %v", err)
	}
}

func TestRemoveFromRosterClearsCaptaincy(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}
	teams := &models.TeamModel{DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Cap", LastName: "Tain"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := teams.SetCaptain(1, sql.NullInt32{Int32: int32(id), Valid: true}); err != nil {
		t.Fatal(err)
	}

	if err := playerService.RemoveFromRoster(id, 1, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	team, err := teams.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if team.CaptainPlayerID.Valid {
		t.Fatalf("expected captain cleared after removal, got %+v", team.CaptainPlayerID)
	}
}

func TestDeletePlayerOrphansUserLogin(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: pm, DB: db}
	users := &models.UserModel{DB: db}

	// fk_users_player would block deletion if the linked login weren't
	// unlinked first.
	playerID, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Has", LastName: "ALogin"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	userID, err := users.Insert("has-a-login@example.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}

	if err := playerService.DeletePlayer(playerID, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	user, err := users.GetUser(userID)
	if err != nil {
		t.Fatal(err)
	}
	if user.PlayerID.Valid {
		t.Fatalf("expected user's playerID to be cleared, got %v", user.PlayerID)
	}
}

func TestDeletePlayerRemovesJoinRequests(t *testing.T) {
	db := models.NewTestDB(t)

	pm := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: pm, DB: db}
	jrm := &models.JoinRequestModel{DB: db}
	joinRequestService := JoinRequestService{JoinRequestModel: jrm, DB: db}

	// An unaffiliated player with a resolved (approved) join-request history
	// — fk_tjr_player would block deletion if these rows weren't cleaned up.
	playerID, err := pm.Insert(&models.Player{FirstName: "Had", LastName: "Requested"})
	if err != nil {
		t.Fatal(err)
	}
	if err := joinRequestService.RequestToJoin(playerID, 1, "had-requested@example.com"); err != nil {
		t.Fatal(err)
	}
	jr, err := jrm.GetPendingByPlayerAndLeague(playerID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := joinRequestService.Approve(jr.ID, 1, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	if err := playerService.DeletePlayer(playerID, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	if _, err := pm.Get(playerID); err != models.ErrNoRecord {
		t.Fatalf("expected player to be deleted, got %v", err)
	}
}

func TestDeletePlayerClearsCaptaincy(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}
	teams := &models.TeamModel{DB: db}

	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Cap", LastName: "Tain"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := teams.SetCaptain(1, sql.NullInt32{Int32: int32(id), Valid: true}); err != nil {
		t.Fatal(err)
	}

	if err := playerService.DeletePlayer(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	team, err := teams.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if team.CaptainPlayerID.Valid {
		t.Fatalf("expected captain cleared after delete, got %+v", team.CaptainPlayerID)
	}
}

func TestDeletePlayerRemovesLeagueAdminRoles(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}
	lam := &models.LeagueAdminModel{DB: db}

	// fk_leagueadmins_player would block deletion if this row weren't
	// cleaned up.
	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "League", LastName: "Admin"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := lam.AddAdmin(id, 1); err != nil {
		t.Fatal(err)
	}

	if err := playerService.DeletePlayer(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	if _, err := players.Get(id); err != models.ErrNoRecord {
		t.Fatalf("expected player to be deleted, got %v", err)
	}
}

func TestDeletePlayerRemovesAllMemberships(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}
	leagues := &models.LeagueModel{DB: db}
	teams := &models.TeamModel{DB: db}
	tmm := &models.TeamMemberModel{DB: db}

	secondLeagueID, err := leagues.Insert(&models.League{Name: "Second League"})
	if err != nil {
		t.Fatal(err)
	}
	secondTeamID, err := teams.Insert(&models.Team{LeagueID: secondLeagueID, Name: "Second Team"})
	if err != nil {
		t.Fatal(err)
	}

	// AddPlayer already adds a team-1 membership; add a second one so the
	// player belongs to two teams before deletion.
	id, err := playerService.AddPlayer(1, &PlayerForm{FirstName: "Multi", LastName: "Team"}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(id, secondTeamID); err != nil {
		t.Fatal(err)
	}

	if err := playerService.DeletePlayer(id, "admin@example.com"); err != nil {
		t.Fatal(err)
	}

	if isMember, err := tmm.IsMember(id, 1); err != nil {
		t.Fatal(err)
	} else if isMember {
		t.Fatal("expected team 1 membership to be removed")
	}
	if isMember, err := tmm.IsMember(id, secondTeamID); err != nil {
		t.Fatal(err)
	} else if isMember {
		t.Fatal("expected second team membership to be removed")
	}
}

func TestRequestAndConfirmPhoneVerification(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	id, err := players.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := playerService.RequestPhoneVerification(id, "518-555-0100"); err != nil {
		t.Fatal(err)
	}

	player, err := players.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !player.PhoneVerificationCode.Valid {
		t.Fatal("expected a pending verification code")
	}
	if player.PhoneNumber.String != "518-555-0100" {
		t.Fatalf("expected the phone number to already be saved, got %q", player.PhoneNumber.String)
	}

	// Wrong code doesn't verify.
	ok, err := playerService.ConfirmPhoneVerification(id, "000000")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected a wrong code to fail")
	}

	// Right code verifies.
	ok, err = playerService.ConfirmPhoneVerification(id, player.PhoneVerificationCode.String)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected the correct code to verify")
	}
	verified, err := players.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.PhoneVerifiedAt.Valid {
		t.Fatal("expected the phone to be marked verified")
	}
	if verified.PhoneVerificationCode.Valid {
		t.Fatal("expected the pending code to be cleared after verifying")
	}
}

// The resend cooldown only applies while a code is still outstanding —
// ConfirmPhoneVerified clears the pending code/expiry once verified (see
// TestRequestAndConfirmPhoneVerification), so there's nothing left to
// cool down after a successful verification. This test covers the
// realistic case the cooldown actually guards: requesting a second code
// before ever confirming the first one.
func TestRequestPhoneVerificationRejectsRapidResend(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	id, err := players.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := playerService.RequestPhoneVerification(id, "518-555-0100"); err != nil {
		t.Fatal(err)
	}
	if err := playerService.RequestPhoneVerification(id, "518-555-0100"); err != models.ErrVerificationCooldown {
		t.Fatalf("expected ErrVerificationCooldown on an immediate resend, got %v", err)
	}
}

func TestConfirmPhoneVerificationRejectsExpiredCode(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	id, err := players.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}
	if err := players.SetPhoneVerificationCode(id, "123456", time.Now().Add(-1*time.Minute)); err != nil {
		t.Fatal(err)
	}

	ok, err := playerService.ConfirmPhoneVerification(id, "123456")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected an expired code to fail")
	}
}

func TestRequestPhoneVerificationRejectsInvalidNumber(t *testing.T) {
	db := models.NewTestDB(t)

	players := &models.PlayerModel{DB: db}
	playerService := PlayerService{PlayerModel: players, DB: db}

	id, err := players.Insert(&models.Player{FirstName: "Lou", LastName: "Garwood"})
	if err != nil {
		t.Fatal(err)
	}

	if err := playerService.RequestPhoneVerification(id, "not-a-phone-number"); err != models.ErrBadData {
		t.Fatalf("expected ErrBadData, got %v", err)
	}
}

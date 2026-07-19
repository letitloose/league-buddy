package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/validator"
)

type PlayerForm struct {
	ID            int
	FirstName     string
	LastName      string
	DateOfBirth   string // "2006-01-02" from <input type=date>
	Address1      string
	Address2      string
	City          string
	StateProvince string
	ZipCode       string
	Email         string
	PhoneNumber   string
	validator.Validator
}

type PlayerService struct {
	*models.PlayerModel
	DB *sql.DB // needed for AddressModel/TeamModel/CommonService side calls
}

func parseOptionalDate(value string) sql.NullTime {
	if value == "" {
		return sql.NullTime{}
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// extractDigits strips everything but the digit characters from s.
func extractDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// normalizePhoneNumber reformats a US phone number as ###-###-####,
// tolerating however the digits were originally punctuated/spaced. Returns
// the original input and false if it doesn't contain exactly 10 digits.
func normalizePhoneNumber(raw string) (string, bool) {
	digits := extractDigits(raw)
	if len(digits) != 10 {
		return raw, false
	}
	return fmt.Sprintf("%s-%s-%s", digits[0:3], digits[3:6], digits[6:10]), true
}

// normalizeZip validates a US ZIP code (exactly 5 digits, tolerating
// incidental spaces/punctuation). Returns the original input and false if
// it doesn't contain exactly 5 digits.
func normalizeZip(raw string) (string, bool) {
	digits := extractDigits(raw)
	if len(digits) != 5 {
		return raw, false
	}
	return digits, true
}

// capitalizeWords uppercases the first letter of each word, leaving the
// rest of each word untouched (so already-correct casing like "McDonald"
// or "NY" isn't clobbered).
func capitalizeWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		if unicode.IsLower(r[0]) {
			r[0] = unicode.ToUpper(r[0])
		}
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// validateAndNormalizePlayerForm runs the field validation and
// normalization (phone/zip formatting, address capitalization) shared by
// AddPlayer and UpdatePlayer.
func validateAndNormalizePlayerForm(form *PlayerForm) {
	form.CheckField(validator.NotBlank(form.FirstName), "firstname", "You must enter a first name.")
	form.CheckField(validator.NotBlank(form.LastName), "lastname", "You must enter a last name.")
	if form.Email != "" {
		form.CheckField(validator.ValidEmail(form.Email), "email", "You must enter a valid email: name@domain.ext")
	}
	if form.PhoneNumber != "" {
		normalized, ok := normalizePhoneNumber(form.PhoneNumber)
		form.PhoneNumber = normalized
		form.CheckField(ok, "phonenumber", "Phone number must be 10 digits.")
	}
	if form.ZipCode != "" {
		normalized, ok := normalizeZip(form.ZipCode)
		form.ZipCode = normalized
		form.CheckField(ok, "zipcode", "Zip code must be 5 digits.")
	}
	if form.StateProvince != "" {
		code := strings.ToUpper(strings.TrimSpace(form.StateProvince))
		form.StateProvince = code
		form.CheckField(models.IsValidStateCode(code), "stateprovince", "You must select a valid state.")
	}
	if form.Address1 != "" {
		// city is NOT NULL in the address table, so this must be enforced
		// whenever a street address is being saved at all.
		form.CheckField(validator.NotBlank(form.City), "city", "You must enter a city.")
	}

	form.Address1 = capitalizeWords(form.Address1)
	form.Address2 = capitalizeWords(form.Address2)
	form.City = capitalizeWords(form.City)
}

func (service *PlayerService) AddPlayer(teamID int, form *PlayerForm, actorEmail string) (int, error) {
	validateAndNormalizePlayerForm(form)

	if !form.Valid() {
		return 0, models.ErrBadData
	}

	tm := &models.TeamModel{DB: service.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		return 0, err
	}

	addressID, err := service.upsertAddress(0, form)
	if err != nil {
		return 0, err
	}

	player := &models.Player{
		FirstName:   form.FirstName,
		LastName:    form.LastName,
		DateOfBirth: parseOptionalDate(form.DateOfBirth),
		AddressID:   addressID,
		Email:       sql.NullString{String: form.Email, Valid: form.Email != ""},
		PhoneNumber: sql.NullString{String: form.PhoneNumber, Valid: form.PhoneNumber != ""},
	}
	playerID, err := service.Insert(player)
	if err != nil {
		return 0, err
	}

	tmm := &models.TeamMemberModel{DB: service.DB}
	if err := tmm.AddMembership(playerID, team.ID); err != nil {
		return 0, err
	}

	cs := &CommonService{DB: service.DB}
	err = cs.InsertAuditLog(actorEmail, time.Now(), "player record created: "+form.FirstName+" "+form.LastName)
	if err != nil {
		return 0, err
	}

	return playerID, nil
}

func (service *PlayerService) UpdatePlayer(form *PlayerForm, actorEmail string) error {
	validateAndNormalizePlayerForm(form)

	if !form.Valid() {
		return models.ErrBadData
	}

	existing, err := service.Get(form.ID)
	if err != nil {
		return err
	}

	existingAddressID := 0
	if existing.AddressID.Valid {
		existingAddressID = int(existing.AddressID.Int32)
	}

	addressID, err := service.upsertAddress(existingAddressID, form)
	if err != nil {
		return err
	}

	player := &models.Player{
		ID:          form.ID,
		FirstName:   form.FirstName,
		LastName:    form.LastName,
		DateOfBirth: parseOptionalDate(form.DateOfBirth),
		AddressID:   addressID,
		Email:       sql.NullString{String: form.Email, Valid: form.Email != ""},
		PhoneNumber: sql.NullString{String: form.PhoneNumber, Valid: form.PhoneNumber != ""},
	}
	err = service.Update(player)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "player record updated: "+form.FirstName+" "+form.LastName)
}

// upsertAddress inserts a new address, updates an existing one (when
// existingAddressID > 0), or leaves things alone if no address fields were
// supplied. Returns the resulting addressID to store on the player row.
func (service *PlayerService) upsertAddress(existingAddressID int, form *PlayerForm) (sql.NullInt32, error) {
	if form.Address1 == "" {
		if existingAddressID > 0 {
			return sql.NullInt32{Int32: int32(existingAddressID), Valid: true}, nil
		}
		return sql.NullInt32{}, nil
	}

	am := &models.AddressModel{DB: service.DB}
	address := &models.Address{
		ID:            existingAddressID,
		Address1:      sql.NullString{String: form.Address1, Valid: true},
		Address2:      sql.NullString{String: form.Address2, Valid: form.Address2 != ""},
		City:          sql.NullString{String: form.City, Valid: form.City != ""},
		StateProvince: sql.NullString{String: form.StateProvince, Valid: form.StateProvince != ""},
		ZipCode:       sql.NullString{String: form.ZipCode, Valid: form.ZipCode != ""},
	}

	if existingAddressID > 0 {
		_, err := am.Update(address)
		if err != nil {
			return sql.NullInt32{}, err
		}
		return sql.NullInt32{Int32: int32(existingAddressID), Valid: true}, nil
	}

	id, err := am.Insert(address)
	if err != nil {
		return sql.NullInt32{}, err
	}
	return sql.NullInt32{Int32: int32(id), Valid: true}, nil
}

// DeletePlayer wipes the player's bio, address, and every team membership,
// and orphans their login if they have one — the full destructive action,
// admin-only. For removing a player from just one team's roster, see
// RemoveFromRoster instead.
func (service *PlayerService) DeletePlayer(id int, actorEmail string) error {
	player, err := service.Get(id)
	if err != nil {
		return err
	}

	// A team's fk_teams_captain FK would otherwise block deleting its
	// current captain.
	tm := &models.TeamModel{DB: service.DB}
	if err := tm.ClearCaptainByPlayer(id); err != nil {
		return err
	}

	tmm := &models.TeamMemberModel{DB: service.DB}
	if err := tmm.DeleteAllForPlayer(id); err != nil {
		return err
	}

	// fk_leagueadmins_player would otherwise block deleting a player who
	// administers one or more leagues.
	lam := &models.LeagueAdminModel{DB: service.DB}
	if err := lam.DeleteAllForPlayer(id); err != nil {
		return err
	}

	// fk_tjr_player would otherwise block deleting a player who ever
	// submitted a join request, approved/rejected/still-pending or not.
	jrm := &models.JoinRequestModel{DB: service.DB}
	if err := jrm.DeleteAllForPlayer(id); err != nil {
		return err
	}

	// fk_users_player would otherwise block deleting a player with a login
	// — orphan the user account (playerID -> NULL) instead of deleting it.
	um := &models.UserModel{DB: service.DB}
	if err := um.ClearPlayerID(id); err != nil {
		return err
	}

	am := &models.AddressModel{DB: service.DB}
	err = am.DeleteByPlayer(id)
	if err != nil {
		return err
	}

	err = service.Delete(id)
	if err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), "player record deleted: "+player.FirstName+" "+player.LastName)
}

// RemoveFromRoster removes playerID from just teamID's roster — the
// lightweight, captain-usable action. The player's bio, address, and any
// other team memberships are untouched.
func (service *PlayerService) RemoveFromRoster(playerID, teamID int, actorEmail string) error {
	player, err := service.Get(playerID)
	if err != nil {
		return err
	}

	tm := &models.TeamModel{DB: service.DB}
	team, err := tm.Get(teamID)
	if err != nil {
		return err
	}
	if team.CaptainPlayerID.Valid && int(team.CaptainPlayerID.Int32) == playerID {
		if err := tm.SetCaptain(teamID, sql.NullInt32{}); err != nil {
			return err
		}
	}

	tmm := &models.TeamMemberModel{DB: service.DB}
	if err := tmm.RemoveMembership(playerID, teamID); err != nil {
		return err
	}

	cs := &CommonService{DB: service.DB}
	return cs.InsertAuditLog(actorEmail, time.Now(), fmt.Sprintf("removed from roster: %s %s (team %d)", player.FirstName, player.LastName, teamID))
}

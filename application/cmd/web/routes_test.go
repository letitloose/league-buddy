package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/letitloose/league-buddy/internal/models"
)

func TestPublicRoutes(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tests := []struct {
		name string
		path string
	}{
		{"home", "/"},
		{"login", "/user/login"},
		{"signup", "/user/signup"},
		{"forgot password", "/user/forgotPassword"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, _ := ts.get(t, tt.path)
			if code != http.StatusOK {
				t.Errorf("want %d; got %d", http.StatusOK, code)
			}
		})
	}
}

// 404s render the branded error page (nav, footer, "Back to Home" link)
// rather than Go's bare-text http.Error response.
func TestNotFoundRendersStyledErrorPage(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	code, _, body := ts.get(t, "/this-route-does-not-exist")
	if code != http.StatusNotFound {
		t.Fatalf("want %d; got %d", http.StatusNotFound, code)
	}
	if !strings.Contains(body, "Page not found") {
		t.Error("expected the friendly 404 headline")
	}
	if !strings.Contains(body, "Back to Home") {
		t.Error("expected a Back to Home link")
	}
	if !strings.Contains(body, "Blame the Ball") {
		t.Error("expected the normal site nav/branding to still render on a 404")
	}
}

// Unauthenticated requests to active and admin routes are redirected to /.
func TestUnauthenticatedAccessRedirects(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tests := []struct {
		name string
		path string
	}{
		{"team player create", "/team/1/player/create"},
		{"user search", "/user/search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, headers, _ := ts.get(t, tt.path)
			if code != http.StatusSeeOther {
				t.Errorf("want %d; got %d", http.StatusSeeOther, code)
			}
			if loc := headers.Get("Location"); loc != "/" {
				t.Errorf("want Location %q; got %q", "/", loc)
			}
		})
	}
}

// Active (non-admin, non-captain) users are redirected away from admin-only
// and team-manager-only routes.
func TestAdminRoutesRedirectActiveNonAdmin(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	getRoutes := []struct {
		name string
		path string
	}{
		{"user search", "/user/search"},
		{"team player create form", "/team/1/player/create"},
	}

	for _, tt := range getRoutes {
		t.Run(tt.name, func(t *testing.T) {
			code, headers, _ := ts.get(t, tt.path)
			if code != http.StatusSeeOther {
				t.Errorf("want %d; got %d", http.StatusSeeOther, code)
			}
			if loc := headers.Get("Location"); loc != "/" {
				t.Errorf("want Location %q; got %q", "/", loc)
			}
		})
	}

	t.Run("team player remove from roster", func(t *testing.T) {
		code, headers, _ := ts.delete(t, "/team/1/player/1/remove")
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("full player delete is admin-only", func(t *testing.T) {
		code, headers, _ := ts.delete(t, "/player/delete/1")
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("team create form redirected for plain active user", func(t *testing.T) {
		code, headers, _ := ts.get(t, "/admin/team/create")
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("team update form redirected for plain active user", func(t *testing.T) {
		code, headers, _ := ts.get(t, "/admin/team/update/1")
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("team delete redirected for plain active user", func(t *testing.T) {
		code, headers, _ := ts.delete(t, "/admin/team/delete/1")
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})
}

// Active users can access active-chain routes.
func TestActiveRoutes(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	tests := []struct {
		name string
		path string
	}{
		{"league list", "/league"},
		{"league view", "/league/1"},
		{"team view", "/team/1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, _ := ts.get(t, tt.path)
			if code != http.StatusOK {
				t.Errorf("want %d; got %d", http.StatusOK, code)
			}
		})
	}
}

// Admin users can access admin-chain and team-manager-chain routes.
func TestAdminRoutes(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testAdminEmail, testAdminPass)

	tests := []struct {
		name string
		path string
	}{
		{"user search", "/user/search"},
		{"team player create form", "/team/1/player/create"},
		{"league create form", "/admin/league/create"},
		{"team create form", "/admin/team/create"},
		{"admin join requests", "/admin/joinRequests"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, _ := ts.get(t, tt.path)
			if code != http.StatusOK {
				t.Errorf("want %d; got %d", http.StatusOK, code)
			}
		})
	}
}

// setupTeamCaptain creates and activates a user, links them to a new player
// on teamID, and makes that player teamID's captain. Returns the player ID.
func setupTeamCaptain(t *testing.T, teamID int, email, password string) int {
	t.Helper()

	um := &models.UserModel{DB: testDB}
	pm := &models.PlayerModel{DB: testDB}
	tm := &models.TeamModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}

	userID, err := um.Insert(email, password)
	if err != nil {
		t.Fatal(err)
	}
	if err := um.Activate(userID); err != nil {
		t.Fatal(err)
	}

	playerID, err := pm.Insert(&models.Player{
		FirstName: "Cap",
		LastName:  "Tain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, teamID); err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetCaptain(teamID, sql.NullInt32{Int32: int32(playerID), Valid: true}); err != nil {
		t.Fatal(err)
	}

	return playerID
}

// setupLeagueAdmin creates and activates a user, links them to a new player,
// and makes that player an admin of leagueID. Returns the player ID.
func setupLeagueAdmin(t *testing.T, leagueID int, email, password string) int {
	t.Helper()

	um := &models.UserModel{DB: testDB}
	pm := &models.PlayerModel{DB: testDB}
	lam := &models.LeagueAdminModel{DB: testDB}

	userID, err := um.Insert(email, password)
	if err != nil {
		t.Fatal(err)
	}
	if err := um.Activate(userID); err != nil {
		t.Fatal(err)
	}

	playerID, err := pm.Insert(&models.Player{
		FirstName: "League",
		LastName:  "Admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}
	if err := lam.AddAdmin(playerID, leagueID); err != nil {
		t.Fatal(err)
	}

	return playerID
}

// A league admin can create/edit/delete a team in their own league, manage
// its roster (same tier captains already use), but is redirected attempting
// any of that for a team in a *different* league.
func TestLeagueAdminTier(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	otherLeagueID, err := lm.Insert(&models.League{Name: "Other League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	ownLeagueTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "League Admin Own Team"})
	if err != nil {
		t.Fatal(err)
	}
	otherLeagueTeamID, err := tm.Insert(&models.Team{LeagueID: otherLeagueID, Name: "League Admin Other Team"})
	if err != nil {
		t.Fatal(err)
	}

	setupLeagueAdmin(t, 1, "league-admin@test.com", "validpassword123")

	t.Run("roster management allowed for own league's team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "league-admin@test.com", "validpassword123")
		code, _, _ := ts.get(t, fmt.Sprintf("/team/%d/player/create", ownLeagueTeamID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("roster management redirected for other league's team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "league-admin@test.com", "validpassword123")
		code, headers, _ := ts.get(t, fmt.Sprintf("/team/%d/player/create", otherLeagueTeamID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("team edit form allowed for own league's team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "league-admin@test.com", "validpassword123")
		code, _, _ := ts.get(t, fmt.Sprintf("/admin/team/update/%d", ownLeagueTeamID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("team edit form redirected for other league's team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "league-admin@test.com", "validpassword123")
		code, headers, _ := ts.get(t, fmt.Sprintf("/admin/team/update/%d", otherLeagueTeamID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("team delete allowed for own league's team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "league-admin@test.com", "validpassword123")
		code, _, _ := ts.delete(t, fmt.Sprintf("/admin/team/delete/%d", ownLeagueTeamID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("team delete redirected for other league's team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "league-admin@test.com", "validpassword123")
		code, headers, _ := ts.delete(t, fmt.Sprintf("/admin/team/delete/%d", otherLeagueTeamID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})
}

// A team captain can now edit their own team's info (name/motto/established
// date), but is still redirected attempting to delete it — deleting a team
// is reserved for admins and league admins.
func TestCaptainCanEditButNotDeleteTeam(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Captain Edit Team"})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, teamID, "captain-edit@test.com", "validpassword123")

	t.Run("team edit form allowed for own team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "captain-edit@test.com", "validpassword123")
		code, _, _ := ts.get(t, fmt.Sprintf("/admin/team/update/%d", teamID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("team delete redirected for own team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "captain-edit@test.com", "validpassword123")
		code, headers, _ := ts.delete(t, fmt.Sprintf("/admin/team/delete/%d", teamID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})
}

// A captain can add a brand-new location as their team's home field
// straight from the team edit form, without going through the admin-only
// /admin/location/create page.
func TestCaptainCanAddNewHomeField(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Home Field Team"})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, teamID, "captain-homefield@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())
	ts.login(t, "captain-homefield@test.com", "validpassword123")

	_, _, body := ts.get(t, fmt.Sprintf("/admin/team/update/%d", teamID))
	csrfToken := extractCSRFToken(t, body)

	code, headers, _ := ts.postForm(t, "/admin/team/update", url.Values{
		"team-id":             {fmt.Sprintf("%d", teamID)},
		"leagueID":            {"1"},
		"name":                {"Home Field Team"},
		"newlocationname":     {"Test Field"},
		"newlocationaddress1": {"5 Test Ln"},
		"newlocationcity":     {"Troy"},
		"csrf_token":          {csrfToken},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
	}
	if loc := headers.Get("Location"); loc != fmt.Sprintf("/team/%d", teamID) {
		t.Fatalf("want Location %q; got %q", fmt.Sprintf("/team/%d", teamID), loc)
	}

	team, err := tm.Get(teamID)
	if err != nil {
		t.Fatal(err)
	}
	if !team.LocationID.Valid {
		t.Fatal("expected team to have a location set")
	}

	lm := &models.LocationModel{DB: testDB}
	location, err := lm.Get(int(team.LocationID.Int32))
	if err != nil {
		t.Fatal(err)
	}
	if location.Name != "Test Field" {
		t.Fatalf("expected location name Test Field, got %s", location.Name)
	}
}

// The teamManager tier (requireTeamManager) must allow Admins through for
// any team, allow a captain through only for their own team, and redirect
// everyone else (including a captain of a *different* team) to /.
func TestTeamManagerTier(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	otherTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Other Manager Team"})
	if err != nil {
		t.Fatal(err)
	}
	ownTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Own Manager Team"})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, ownTeamID, "captain-own@test.com", "validpassword123")

	t.Run("captain of own team allowed", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "captain-own@test.com", "validpassword123")
		code, _, _ := ts.get(t, fmt.Sprintf("/team/%d/player/create", ownTeamID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("captain of another team redirected", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "captain-own@test.com", "validpassword123")
		code, headers, _ := ts.get(t, fmt.Sprintf("/team/%d/player/create", otherTeamID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("admin allowed on any team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testAdminEmail, testAdminPass)
		code, _, _ := ts.get(t, fmt.Sprintf("/team/%d/player/create", otherTeamID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("plain active user redirected", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)
		code, headers, _ := ts.get(t, fmt.Sprintf("/team/%d/player/create", ownTeamID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})
}

// End-to-end: an unaffiliated active player requests to join a team, and the
// team's captain can approve it, which assigns the team and clears the
// pending request.
func TestJoinRequestSubmitApproveFlow(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Join Flow Team"})
	if err != nil {
		t.Fatal(err)
	}
	setupTeamCaptain(t, teamID, "captain-jr@test.com", "validpassword123")

	um := &models.UserModel{DB: testDB}
	pm := &models.PlayerModel{DB: testDB}

	applicantUserID, err := um.Insert("applicant@test.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.Activate(applicantUserID); err != nil {
		t.Fatal(err)
	}
	applicantPlayerID, err := pm.Insert(&models.Player{FirstName: "App", LastName: "Licant"})
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(applicantUserID, applicantPlayerID); err != nil {
		t.Fatal(err)
	}

	applicantTS := newTestServer(t, app.routes())
	applicantTS.login(t, "applicant@test.com", "validpassword123")

	_, _, body := applicantTS.get(t, fmt.Sprintf("/team/%d", teamID))
	csrfToken := extractCSRFToken(t, body)

	code, headers, _ := applicantTS.postForm(t, fmt.Sprintf("/team/%d/joinRequest", teamID), url.Values{
		"csrf_token": {csrfToken},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("join request submit: want %d; got %d", http.StatusSeeOther, code)
	}
	if loc := headers.Get("Location"); loc != fmt.Sprintf("/team/%d", teamID) {
		t.Fatalf("join request submit: want Location %q; got %q", fmt.Sprintf("/team/%d", teamID), loc)
	}

	jrm := &models.JoinRequestModel{DB: testDB}
	pending, err := jrm.GetPendingByPlayerAndLeague(applicantPlayerID, 1)
	if err != nil {
		t.Fatal(err)
	}

	captainTS := newTestServer(t, app.routes())
	captainTS.login(t, "captain-jr@test.com", "validpassword123")

	_, _, body = captainTS.get(t, fmt.Sprintf("/team/%d/joinRequests", teamID))
	csrfToken = extractCSRFToken(t, body)

	code, headers, _ = captainTS.postForm(t, fmt.Sprintf("/team/%d/joinRequests/%d/approve", teamID, pending.ID), url.Values{
		"csrf_token": {csrfToken},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("join request approve: want %d; got %d", http.StatusSeeOther, code)
	}

	tmm := &models.TeamMemberModel{DB: testDB}
	isMember, err := tmm.IsMember(applicantPlayerID, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatalf("expected player joined to team %d", teamID)
	}
}

// End-to-end: signing up via a captain's invite link auto-joins the invited
// team, with no separate approval step.
func TestInviteSignupAutoJoinsTeam(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Invite Signup Team"})
	if err != nil {
		t.Fatal(err)
	}

	um := &models.UserModel{DB: testDB}
	admin, err := um.GetUserByEmail(testAdminEmail)
	if err != nil {
		t.Fatal(err)
	}

	im := &models.InviteModel{DB: testDB}
	inviteID, err := im.Insert(&models.Invite{
		Token:           "route-test-invite-token",
		TeamID:          teamID,
		Email:           "invited-signup@test.com",
		CreatedByUserID: admin.UserID,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, body := ts.get(t, "/user/signup?invite=route-test-invite-token")
	csrfToken := extractCSRFToken(t, body)

	code, headers, _ := ts.postForm(t, "/user/signup", url.Values{
		"email":           {"invited-signup@test.com"},
		"password":        {"validpassword123"},
		"confirmPassword": {"validpassword123"},
		"inviteToken":     {"route-test-invite-token"},
		"csrf_token":      {csrfToken},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("signup: want %d; got %d", http.StatusSeeOther, code)
	}
	if loc := headers.Get("Location"); loc != "/user/login" {
		t.Fatalf("signup: want Location %q; got %q", "/user/login", loc)
	}

	hash, err := app.userService.GetVerificationHashByEmail("invited-signup@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.userService.ActivateUser(hash); err != nil {
		t.Fatal(err)
	}

	user, err := um.GetUserByEmail("invited-signup@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.PlayerID.Valid {
		t.Fatal("expected a player to have been linked")
	}

	tmm := &models.TeamMemberModel{DB: testDB}
	isMember, err := tmm.IsMember(int(user.PlayerID.Int32), teamID)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatalf("expected player auto-joined to team %d", teamID)
	}

	invite, err := im.Get(inviteID)
	if err != nil {
		t.Fatal(err)
	}
	if !invite.UsedAt.Valid {
		t.Fatal("expected invite to be marked used")
	}
}

// A team manager (captain, league admin, or admin) can cancel an
// outstanding invite, which both drops it from the pending list and stops
// its signup link from working.
func TestCaptainCanCancelInvite(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Cancel Invite Team"})
	if err != nil {
		t.Fatal(err)
	}
	otherTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Cancel Invite Other Team"})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, teamID, "cancel-invite-captain@test.com", "validpassword123")

	um := &models.UserModel{DB: testDB}
	captainUser, err := um.GetUserByEmail("cancel-invite-captain@test.com")
	if err != nil {
		t.Fatal(err)
	}

	im := &models.InviteModel{DB: testDB}
	inviteID, err := im.Insert(&models.Invite{
		Token:           "cancel-route-token",
		TeamID:          teamID,
		Email:           "someone@test.com",
		CreatedByUserID: captainUser.UserID,
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, "cancel-invite-captain@test.com", "validpassword123")

	t.Run("captain of a different team cannot cancel", func(t *testing.T) {
		code, headers, _ := ts.delete(t, fmt.Sprintf("/team/%d/invite/%d/cancel", otherTeamID, inviteID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("captain of the team can cancel", func(t *testing.T) {
		code, _, _ := ts.delete(t, fmt.Sprintf("/team/%d/invite/%d/cancel", teamID, inviteID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}

		pending, err := im.ListPendingByTeam(teamID)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 0 {
			t.Fatalf("expected the canceled invite to drop off the pending list, got %d", len(pending))
		}
	})

	t.Run("canceling again reports a conflict", func(t *testing.T) {
		code, _, _ := ts.delete(t, fmt.Sprintf("/team/%d/invite/%d/cancel", teamID, inviteID))
		if code != http.StatusConflict {
			t.Errorf("want %d; got %d", http.StatusConflict, code)
		}
	})
}

// Inviting a free-text email address that already belongs to a player on
// this team's roster is rejected with a field error; inviting a roster
// player who has no account yet, via the roster picker, succeeds.
func TestTeamInviteRosterValidationAndPicker(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Roster Invite Team"})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, teamID, "roster-invite-captain@test.com", "validpassword123")

	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}
	playerID, err := pm.Insert(&models.Player{
		FirstName: "Roster",
		LastName:  "Placeholder",
		Email:     sql.NullString{String: "roster-invite-placeholder@test.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, teamID); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, "roster-invite-captain@test.com", "validpassword123")

	t.Run("free-text invite to an existing roster member's email is rejected", func(t *testing.T) {
		_, _, getBody := ts.get(t, fmt.Sprintf("/team/%d/invite", teamID))
		csrfToken := extractCSRFToken(t, getBody)

		form := url.Values{}
		form.Add("csrf_token", csrfToken)
		form.Add("emails", "roster-invite-placeholder@test.com")
		code, _, body := ts.postForm(t, fmt.Sprintf("/team/%d/invite", teamID), form)
		if code != http.StatusUnprocessableEntity {
			t.Errorf("want %d; got %d", http.StatusUnprocessableEntity, code)
		}
		if !strings.Contains(body, "already on this team") {
			t.Errorf("expected a roster-conflict error message in the response body")
		}
	})

	t.Run("inviting via the roster picker succeeds", func(t *testing.T) {
		_, _, getBody := ts.get(t, fmt.Sprintf("/team/%d/invite", teamID))
		csrfToken := extractCSRFToken(t, getBody)

		form := url.Values{}
		form.Add("csrf_token", csrfToken)
		form.Add("playerIDs", strconv.Itoa(playerID))
		code, headers, _ := ts.postForm(t, fmt.Sprintf("/team/%d/invite/roster", teamID), form)
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/team/%d", teamID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/team/%d", teamID), loc)
		}

		im := &models.InviteModel{DB: testDB}
		pending, err := im.ListPendingByTeam(teamID)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 1 || pending[0].Email != "roster-invite-placeholder@test.com" {
			t.Fatalf("expected one pending invite for roster-invite-placeholder@test.com, got %v", pending)
		}
	})
}

// Inviting an email address that already belongs to a User account (just
// not yet on this team) adds them to the roster immediately, end to end
// through the route — no dangling invite, since they'd have no reason to
// go through signup again.
func TestTeamInviteExistingAccountAddsImmediately(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Existing Account Invite Team"})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, teamID, "existing-account-invite-captain@test.com", "validpassword123")

	pm := &models.PlayerModel{DB: testDB}
	um := &models.UserModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}

	playerID, err := pm.Insert(&models.Player{
		FirstName: "Elsewhere",
		LastName:  "Already",
		Email:     sql.NullString{String: "elsewhere-already@test.com", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := um.Insert("elsewhere-already@test.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, "existing-account-invite-captain@test.com", "validpassword123")

	_, _, getBody := ts.get(t, fmt.Sprintf("/team/%d/invite", teamID))
	csrfToken := extractCSRFToken(t, getBody)

	form := url.Values{}
	form.Add("csrf_token", csrfToken)
	form.Add("emails", "elsewhere-already@test.com")
	code, headers, _ := ts.postForm(t, fmt.Sprintf("/team/%d/invite", teamID), form)
	if code != http.StatusSeeOther {
		t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
	}
	if loc := headers.Get("Location"); loc != fmt.Sprintf("/team/%d", teamID) {
		t.Errorf("want Location %q; got %q", fmt.Sprintf("/team/%d", teamID), loc)
	}

	isMember, err := tmm.IsMember(playerID, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected the existing account's player to be added to the roster immediately")
	}

	im := &models.InviteModel{DB: testDB}
	pending, err := im.ListPendingByTeam(teamID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no dangling invite for an existing account, got %v", pending)
	}
}

// A canceled invite's signup link no longer auto-joins the team — the
// account can still be created, but linkOrCreatePlayer never sees a valid
// pending invite to act on.
func TestCanceledInviteSignupDoesNotAutoJoin(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Canceled Invite Team"})
	if err != nil {
		t.Fatal(err)
	}

	um := &models.UserModel{DB: testDB}
	admin, err := um.GetUserByEmail(testAdminEmail)
	if err != nil {
		t.Fatal(err)
	}

	im := &models.InviteModel{DB: testDB}
	inviteID, err := im.Insert(&models.Invite{
		Token:           "canceled-signup-token",
		TeamID:          teamID,
		Email:           "canceled-invite-signup@test.com",
		CreatedByUserID: admin.UserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := im.Cancel(inviteID); err != nil {
		t.Fatal(err)
	}

	_, _, body := ts.get(t, "/user/signup?invite=canceled-signup-token")
	csrfToken := extractCSRFToken(t, body)

	code, _, _ := ts.postForm(t, "/user/signup", url.Values{
		"email":           {"canceled-invite-signup@test.com"},
		"password":        {"validpassword123"},
		"confirmPassword": {"validpassword123"},
		"inviteToken":     {"canceled-signup-token"},
		"csrf_token":      {csrfToken},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("signup: want %d; got %d", http.StatusSeeOther, code)
	}

	hash, err := app.userService.GetVerificationHashByEmail("canceled-invite-signup@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.userService.ActivateUser(hash); err != nil {
		t.Fatal(err)
	}

	user, err := um.GetUserByEmail("canceled-invite-signup@test.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.PlayerID.Valid {
		t.Fatal("expected a placeholder player to still be linked")
	}

	tmm := &models.TeamMemberModel{DB: testDB}
	isMember, err := tmm.IsMember(int(user.PlayerID.Int32), teamID)
	if err != nil {
		t.Fatal(err)
	}
	if isMember {
		t.Fatal("expected a canceled invite to NOT auto-join the team")
	}
}

// A league admin can create/edit/delete a season and a match within their
// own league, but is redirected attempting any of that for a different
// league — season/match administration reuses canManageLeague exactly like
// team creation does.
func TestLeagueAdminCanManageSeasonsAndMatches(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	otherLeagueID, err := lm.Insert(&models.League{Name: "Other Season League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Season Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Season Away FC"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: testDB}
	otherSeasonID, err := sm.Insert(&models.Season{LeagueID: otherLeagueID, Name: "Other League Season"})
	if err != nil {
		t.Fatal(err)
	}

	setupLeagueAdmin(t, 1, "season-league-admin@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())
	ts.login(t, "season-league-admin@test.com", "validpassword123")

	t.Run("season create form allowed for own league", func(t *testing.T) {
		code, _, _ := ts.get(t, "/admin/season/create?leagueID=1")
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	_, _, body := ts.get(t, "/admin/season/create?leagueID=1")
	csrfToken := extractCSRFToken(t, body)

	var seasonID int
	t.Run("season create allowed for own league", func(t *testing.T) {
		code, headers, _ := ts.postForm(t, "/admin/season/create", url.Values{
			"leagueID":   {"1"},
			"name":       {"Test Season"},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		loc := headers.Get("Location")
		if _, err := fmt.Sscanf(loc, "/season/%d", &seasonID); err != nil || seasonID < 1 {
			t.Fatalf("expected a /season/:id redirect, got %q", loc)
		}
	})

	t.Run("season create redirected for other league", func(t *testing.T) {
		code, headers, _ := ts.postForm(t, "/admin/season/create", url.Values{
			"leagueID":   {fmt.Sprintf("%d", otherLeagueID)},
			"name":       {"Should Not Be Created"},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("season update form redirected for other league's season", func(t *testing.T) {
		code, headers, _ := ts.get(t, fmt.Sprintf("/admin/season/update/%d", otherSeasonID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	var matchID int
	t.Run("match create allowed for own league's season", func(t *testing.T) {
		_, _, formBody := ts.get(t, fmt.Sprintf("/admin/match/create?seasonID=%d", seasonID))
		matchCSRF := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, "/admin/match/create", url.Values{
			"seasonID":   {fmt.Sprintf("%d", seasonID)},
			"hometeamid": {fmt.Sprintf("%d", homeTeamID)},
			"awayteamid": {fmt.Sprintf("%d", awayTeamID)},
			"matchdate":  {"2024-05-05"},
			"csrf_token": {matchCSRF},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/season/%d", seasonID) {
			t.Fatalf("want Location %q; got %q", fmt.Sprintf("/season/%d", seasonID), loc)
		}

		mm := &models.MatchModel{DB: testDB}
		matches, err := mm.GetBySeason(seasonID)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Fatalf("expected 1 match, got %d", len(matches))
		}
		matchID = matches[0].ID
	})

	t.Run("match update allowed for own league's match", func(t *testing.T) {
		code, _, _ := ts.get(t, fmt.Sprintf("/admin/match/update/%d", matchID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("match delete allowed for own league's match", func(t *testing.T) {
		code, _, _ := ts.delete(t, fmt.Sprintf("/admin/match/delete/%d", matchID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("season delete allowed once its matches are gone", func(t *testing.T) {
		code, _, _ := ts.delete(t, fmt.Sprintf("/admin/season/delete/%d", seasonID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})
}

// A plain captain (no league-admin rights) is redirected from every
// season/match admin route, but can still view the read-only season and
// team-schedule pages.
func TestCaptainRedirectedFromSeasonMatchAdminRoutes(t *testing.T) {
	app := newTestApplication(t)

	// A dedicated league (rather than the shared team-1/league-1 fixtures)
	// so this test's season doesn't leak into other tests in this file —
	// TestMain sets up the DB once for the whole binary, it isn't reset
	// between tests.
	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Captain Season League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Captain Season Team"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Captain View Season"})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, teamID, "captain-season@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())
	ts.login(t, "captain-season@test.com", "validpassword123")

	adminGetRoutes := []struct {
		name string
		path string
	}{
		{"season create form", "/admin/season/create?leagueID=1"},
		{"season update form", fmt.Sprintf("/admin/season/update/%d", seasonID)},
		{"match create form", fmt.Sprintf("/admin/match/create?seasonID=%d", seasonID)},
	}
	for _, tt := range adminGetRoutes {
		t.Run(tt.name+" redirected", func(t *testing.T) {
			code, headers, _ := ts.get(t, tt.path)
			if code != http.StatusSeeOther {
				t.Errorf("want %d; got %d", http.StatusSeeOther, code)
			}
			if loc := headers.Get("Location"); loc != "/" {
				t.Errorf("want Location %q; got %q", "/", loc)
			}
		})
	}

	t.Run("season delete forbidden", func(t *testing.T) {
		code, _, _ := ts.delete(t, fmt.Sprintf("/admin/season/delete/%d", seasonID))
		if code != http.StatusForbidden {
			t.Errorf("want %d; got %d", http.StatusForbidden, code)
		}
	})

	t.Run("season view allowed (read-only)", func(t *testing.T) {
		code, _, _ := ts.get(t, fmt.Sprintf("/season/%d", seasonID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("team schedule view allowed (read-only)", func(t *testing.T) {
		code, _, _ := ts.get(t, fmt.Sprintf("/team/%d/season/%d", teamID, seasonID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})
}

// A team's page and the home page must render without error when its
// league has no seasons yet (the default state for a brand-new league) —
// team 1 in the test fixtures never has a season, so these are exercised by
// every other test in this file too, but it's worth asserting directly.
func TestTeamAndHomeRenderWithoutSeason(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	code, _, body := ts.get(t, "/team/1")
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	if strings.Contains(body, "Schedule &amp; full stats") {
		t.Error("expected no Schedule link without a season")
	}

	code, _, _ = ts.get(t, "/")
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
}

// The league page's standings table defaults to points-descending, and
// clicking a column header (via its ?sort=&dir= query params) re-orders it
// by that column instead. Goals-against ranks these three teams in a
// different order than points does, by construction, so the two page
// renders are checked against two different expected orderings.
func TestLeagueStandingsSortingAndLeaderTables(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Standings Test League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	teamA, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Standings Team A"})
	if err != nil {
		t.Fatal(err)
	}
	teamB, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Standings Team B"})
	if err != nil {
		t.Fatal(err)
	}
	teamC, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Standings Team C"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Standings Test Season"})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: testDB}
	scorer, err := pm.Insert(&models.Player{FirstName: "Goalie", LastName: "Scorer"})
	if err != nil {
		t.Fatal(err)
	}
	assister, err := pm.Insert(&models.Player{FirstName: "Setup", LastName: "Assister"})
	if err != nil {
		t.Fatal(err)
	}

	mm := &models.MatchModel{DB: testDB}
	pmsm := &models.PlayerMatchStatModel{DB: testDB}

	// A beats B 5-1, B beats C 1-0, A beats C 2-0.
	// Points: A=6, B=3, C=0. GoalsAgainst: A=1, C=3, B=5 — a different order.
	match1, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamA, AwayTeamID: teamB, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 5, Valid: true}, AwayScore: sql.NullInt32{Int32: 1, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamA, AwayTeamID: teamC, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 2, Valid: true}, AwayScore: sql.NullInt32{Int32: 0, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamB, AwayTeamID: teamC, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 1, Valid: true}, AwayScore: sql.NullInt32{Int32: 0, Valid: true}}); err != nil {
		t.Fatal(err)
	}

	if err := pmsm.Upsert(&models.PlayerMatchStat{MatchID: match1, PlayerID: scorer, TeamID: teamA, Goals: 3, Assists: 0}); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&models.PlayerMatchStat{MatchID: match1, PlayerID: assister, TeamID: teamA, Goals: 1, Assists: 2}); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	t.Run("default sort is points descending", func(t *testing.T) {
		code, _, body := ts.get(t, fmt.Sprintf("/league/%d", leagueID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		posA, posB, posC := strings.Index(body, "Standings Team A"), strings.Index(body, "Standings Team B"), strings.Index(body, "Standings Team C")
		if posA < 0 || posB < 0 || posC < 0 {
			t.Fatalf("expected all three teams in the standings table, got positions %d/%d/%d", posA, posB, posC)
		}
		if !(posA < posB && posB < posC) {
			t.Fatalf("expected order A, B, C by points; got positions %d/%d/%d", posA, posB, posC)
		}
		if !strings.Contains(body, "Goal Leaders") || !strings.Contains(body, "Assist Leaders") {
			t.Error("expected both leader tables on the league page")
		}
		if !strings.Contains(body, "Goalie Scorer") || !strings.Contains(body, "Setup Assister") {
			t.Error("expected the seeded scorer/assister to appear in the leader tables")
		}
	})

	t.Run("sort=goalsagainst&dir=asc reorders the table", func(t *testing.T) {
		code, _, body := ts.get(t, fmt.Sprintf("/league/%d?sort=goalsagainst&dir=asc", leagueID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		posA, posB, posC := strings.Index(body, "Standings Team A"), strings.Index(body, "Standings Team B"), strings.Index(body, "Standings Team C")
		if !(posA < posC && posC < posB) {
			t.Fatalf("expected order A, C, B by goals-against ascending; got positions %d/%d/%d", posA, posB, posC)
		}
	})

	t.Run("season page shows the same leader tables", func(t *testing.T) {
		code, _, body := ts.get(t, fmt.Sprintf("/season/%d", seasonID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "Goal Leaders") || !strings.Contains(body, "Assist Leaders") {
			t.Error("expected both leader tables on the season page")
		}
		if !strings.Contains(body, "Goalie Scorer") || !strings.Contains(body, "Setup Assister") {
			t.Error("expected the seeded scorer/assister to appear in the leader tables")
		}
	})
}

// The match view page is open to any active user, but the Edit Match link
// (and the underlying /admin/match/update route) is only offered to an
// admin, a league admin, or a captain of one of the two teams that played —
// captains weren't previously involved in match administration at all, so
// this is new capability, not a change to the existing admin/league-admin
// access. Deleting a match stays admin/league-admin-only, same as deleting
// a team — too destructive for a single unilateral captain.
func TestMatchViewAndCaptainEditAccess(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Match View Test League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Match View Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Match View Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	otherTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Match View Bystander FC"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Match View Season"})
	if err != nil {
		t.Fatal(err)
	}

	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 1, Valid: true}, AwayScore: sql.NullInt32{Int32: 0, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, awayTeamID, "match-view-captain@test.com", "validpassword123")
	setupTeamCaptain(t, otherTeamID, "match-view-bystander-captain@test.com", "validpassword123")

	t.Run("any active user can view the match", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		code, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "Match View Home FC") || !strings.Contains(body, "Match View Away FC") {
			t.Error("expected both team names on the match view page")
		}
		if strings.Contains(body, "Edit Match") {
			t.Error("expected no Edit Match link for a plain active user")
		}
	})

	t.Run("captain of a team in the match can edit it", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "match-view-captain@test.com", "validpassword123")

		code, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "Edit Match") {
			t.Error("expected an Edit Match link for a captain of one of the teams")
		}

		_, _, formBody := ts.get(t, fmt.Sprintf("/admin/match/update/%d", matchID))
		csrfToken := extractCSRFToken(t, formBody)

		postCode, headers, _ := ts.postForm(t, "/admin/match/update", url.Values{
			"match-id":   {fmt.Sprintf("%d", matchID)},
			"matchdate":  {"2024-05-05"},
			"homescore":  {"2"},
			"awayscore":  {"0"},
			"csrf_token": {csrfToken},
		})
		if postCode != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, postCode)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
			t.Fatalf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
		}

		match, err := mm.Get(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if !match.HomeScore.Valid || match.HomeScore.Int32 != 2 {
			t.Fatalf("expected updated homeScore 2, got %+v", match.HomeScore)
		}
	})

	t.Run("captain of a team in the match still cannot delete it", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "match-view-captain@test.com", "validpassword123")

		code, _, _ := ts.delete(t, fmt.Sprintf("/admin/match/delete/%d", matchID))
		if code != http.StatusForbidden {
			t.Errorf("want %d; got %d", http.StatusForbidden, code)
		}
	})

	t.Run("captain of an uninvolved team cannot edit", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "match-view-bystander-captain@test.com", "validpassword123")

		code, headers, _ := ts.get(t, fmt.Sprintf("/admin/match/update/%d", matchID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("team home page shows the schedule linking to the match view", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		code, _, body := ts.get(t, fmt.Sprintf("/team/%d", homeTeamID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, fmt.Sprintf("/match/%d", matchID)) {
			t.Error("expected the team home page's schedule to link to the match view")
		}
		if !strings.Contains(body, "Match View Away FC") {
			t.Error("expected the opponent's name in the schedule")
		}
	})

	t.Run("away team's schedule prefixes the opponent with @", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		code, _, body := ts.get(t, fmt.Sprintf("/team/%d", awayTeamID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "@ Match View Home FC") {
			t.Error("expected the away team's schedule to show '@ <home team>' rather than separate Home/Away columns")
		}
	})
}

// Submitting the match-edit form's goal/card rows saves them as discrete
// events (queryable via MatchGoalModel/MatchCardModel exactly as
// submitted), recomputes the playerMatchStats cache the leaderboards read
// from, and a second submission with a different set of rows replaces
// rather than accumulates.
func TestMatchUpdateSavesGoalsAndCards(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Match Update Events League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Events Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Events Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Events Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 2, Valid: true}, AwayScore: sql.NullInt32{Int32: 0, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	scorerID := setupTeamCaptain(t, homeTeamID, "events-scorer-captain@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())
	ts.login(t, testAdminEmail, testAdminPass)

	postGoalsAndCards := func(t *testing.T, form url.Values) (int, http.Header) {
		t.Helper()
		_, _, formBody := ts.get(t, fmt.Sprintf("/admin/match/update/%d", matchID))
		form.Set("csrf_token", extractCSRFToken(t, formBody))
		form.Set("match-id", fmt.Sprintf("%d", matchID))
		code, headers, _ := ts.postForm(t, "/admin/match/update", form)
		return code, headers
	}

	t.Run("saves a scored goal, an unattributed goal, and a card", func(t *testing.T) {
		form := url.Values{
			"matchdate":  {"2024-05-05"},
			"homescore":  {"2"},
			"awayscore":  {"1"},
			"goalTeamID": {fmt.Sprintf("%d", homeTeamID), fmt.Sprintf("%d", awayTeamID)},
			// First goal: scored and assisted by the same test player (not
			// realistic, but exercises both tallies incrementing). Second
			// goal (away team) intentionally has no scorer/assister.
			"goalScorerID":   {fmt.Sprintf("%d", scorerID), ""},
			"goalAssisterID": {fmt.Sprintf("%d", scorerID), ""},
			"cardTeamID":     {fmt.Sprintf("%d", homeTeamID)},
			"cardPlayerID":   {fmt.Sprintf("%d", scorerID)},
			"cardType":       {"yellow"},
		}
		code, headers := postGoalsAndCards(t, form)
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
		}

		mgm := &models.MatchGoalModel{DB: testDB}
		goals, err := mgm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(goals) != 2 {
			t.Fatalf("expected 2 goal rows, got %d", len(goals))
		}
		if goals[0].TeamID != homeTeamID || !goals[0].ScorerPlayerID.Valid || int(goals[0].ScorerPlayerID.Int32) != scorerID {
			t.Fatalf("expected the first goal attributed to the scorer, got %+v", goals[0])
		}
		if goals[1].TeamID != awayTeamID || goals[1].ScorerPlayerID.Valid {
			t.Fatalf("expected the second goal unattributed for the away team, got %+v", goals[1])
		}

		mcm := &models.MatchCardModel{DB: testDB}
		cards, err := mcm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(cards) != 1 || cards[0].CardType != "yellow" {
			t.Fatalf("expected 1 yellow card, got %+v", cards)
		}

		pmsm := &models.PlayerMatchStatModel{DB: testDB}
		stats, err := pmsm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stats) != 1 || stats[0].Goals != 1 || stats[0].Assists != 1 || stats[0].YellowCards != 1 {
			t.Fatalf("expected the recomputed cache to show goals=1 assists=1 yellowCards=1, got %+v", stats)
		}
	})

	t.Run("resubmitting with fewer rows replaces rather than accumulates", func(t *testing.T) {
		form := url.Values{
			"matchdate":    {"2024-05-05"},
			"homescore":    {"2"},
			"awayscore":    {"0"},
			"goalTeamID":   {fmt.Sprintf("%d", homeTeamID)},
			"goalScorerID": {fmt.Sprintf("%d", scorerID)},
		}
		code, _ := postGoalsAndCards(t, form)
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}

		mgm := &models.MatchGoalModel{DB: testDB}
		goals, err := mgm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(goals) != 1 {
			t.Fatalf("expected the resubmit to replace, leaving 1 goal, got %d", len(goals))
		}

		mcm := &models.MatchCardModel{DB: testDB}
		cards, err := mcm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(cards) != 0 {
			t.Fatalf("expected the card to have been dropped by the resubmit, got %d", len(cards))
		}

		pmsm := &models.PlayerMatchStatModel{DB: testDB}
		stats, err := pmsm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stats) != 1 || stats[0].Goals != 1 || stats[0].Assists != 0 || stats[0].YellowCards != 0 {
			t.Fatalf("expected the recomputed cache to show only goals=1, got %+v", stats)
		}
	})
}

// A roster player on either side of a match can RSVP yes/no with an
// optional message; resubmitting updates their existing response instead of
// adding a duplicate; a plain active user with no roster tie to either team
// is redirected rather than allowed to record a response.
func TestMatchRSVP(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "RSVP Test League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "RSVP Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "RSVP Away FC"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "RSVP Season"})
	if err != nil {
		t.Fatal(err)
	}

	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now(),
		// Scored so the team page's GetMostRecentWithResults picks up this
		// season and renders the schedule table the last subtest checks.
		HomeScore: sql.NullInt32{Int32: 1, Valid: true}, AwayScore: sql.NullInt32{Int32: 0, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	rosterPlayerID := setupTeamCaptain(t, homeTeamID, "rsvp-player@test.com", "validpassword123")

	rm := &models.RSVPModel{DB: testDB}

	t.Run("a roster player can RSVP and resubmit updates the same response", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "rsvp-player@test.com", "validpassword123")

		_, _, getBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken := extractCSRFToken(t, getBody)

		code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/rsvp", matchID), url.Values{
			"csrf_token": {csrfToken},
			"status":     {"yes"},
			"message":    {"count me in"},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
		}

		_, _, viewBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(viewBody, "count me in") {
			t.Error("expected the RSVP message to appear on the match view")
		}

		_, _, getBody2 := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken2 := extractCSRFToken(t, getBody2)
		code2, _, _ := ts.postForm(t, fmt.Sprintf("/match/%d/rsvp", matchID), url.Values{
			"csrf_token": {csrfToken2},
			"status":     {"no"},
			"message":    {"can't make it"},
		})
		if code2 != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code2)
		}

		rsvps, err := rm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(rsvps) != 1 {
			t.Fatalf("expected resubmitting to update the same row, got %d rows", len(rsvps))
		}
		if rsvps[0].PlayerID != rosterPlayerID || rsvps[0].Status != "no" {
			t.Fatalf("expected the latest response (no) to have won, got %+v", rsvps[0])
		}
	})

	t.Run("a plain active user with no roster tie is redirected, not recorded", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		_, _, getBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken := extractCSRFToken(t, getBody)

		code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/rsvp", matchID), url.Values{
			"csrf_token": {csrfToken},
			"status":     {"yes"},
		})
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
		}

		rsvps, err := rm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(rsvps) != 1 {
			t.Fatalf("expected no new RSVP row from an ineligible user, got %d rows", len(rsvps))
		}
	})

	t.Run("the team schedule table reflects the recorded in/out count", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		code, _, body := ts.get(t, fmt.Sprintf("/team/%d", homeTeamID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "0 in / 1 out") {
			t.Errorf("expected the schedule row to show 0 in / 1 out for the home team, body: %s", body)
		}
	})
}

// A match that already happened can no longer be RSVP'd to: the widget
// doesn't render, and a direct POST is redirected away without recording a
// response — the same "in-handler eligibility check" enforced server-side,
// not just hidden in the UI.
func TestMatchRSVPClosedForPastMatches(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Past Match RSVP League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Past Match Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Past Match Away FC"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Past Match Season"})
	if err != nil {
		t.Fatal(err)
	}

	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID,
		MatchDate: time.Now().AddDate(0, 0, -7),
		HomeScore: sql.NullInt32{Int32: 2, Valid: true}, AwayScore: sql.NullInt32{Int32: 1, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, homeTeamID, "past-rsvp-player@test.com", "validpassword123")
	rm := &models.RSVPModel{DB: testDB}

	ts := newTestServer(t, app.routes())
	ts.login(t, "past-rsvp-player@test.com", "validpassword123")

	_, _, getBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
	if strings.Contains(getBody, "Your RSVP") {
		t.Error("expected no RSVP widget for a match that already happened")
	}
	// The match page itself has no form (no RSVP widget, no session-scoped
	// form field) to pull a token from, so grab one from another page in the
	// same session — nosurf's token isn't page-specific.
	_, _, signupBody := ts.get(t, "/user/signup")
	csrfToken := extractCSRFToken(t, signupBody)

	code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/rsvp", matchID), url.Values{
		"csrf_token": {csrfToken},
		"status":     {"yes"},
	})
	if code != http.StatusSeeOther {
		t.Errorf("want %d; got %d", http.StatusSeeOther, code)
	}
	if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
		t.Errorf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
	}

	rsvps, err := rm.ListByMatch(matchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rsvps) != 0 {
		t.Fatalf("expected no RSVP recorded for a past match, got %d rows", len(rsvps))
	}
}

// The match view's "Not Attending" list shows roster players who RSVP'd
// "no" (with their message) while the match is still upcoming, but drops
// off the page once the match has happened — only "Confirmed" (who
// actually showed) matters in hindsight.
func TestMatchRSVPNotAttendingVisibility(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Attendance Test League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Attendance Test Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Attendance Test Away FC"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Attendance Test Season"})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}
	rm := &models.RSVPModel{DB: testDB}

	comingPlayerID, err := pm.Insert(&models.Player{FirstName: "Coming", LastName: "Player"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(comingPlayerID, homeTeamID); err != nil {
		t.Fatal(err)
	}
	outPlayerID, err := pm.Insert(&models.Player{FirstName: "Out", LastName: "Player"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(outPlayerID, homeTeamID); err != nil {
		t.Fatal(err)
	}

	mm := &models.MatchModel{DB: testDB}

	t.Run("upcoming match shows both Confirmed and Not Attending", func(t *testing.T) {
		matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		if err := rm.Upsert(&models.RSVP{MatchID: matchID, PlayerID: comingPlayerID, TeamID: homeTeamID, Status: "yes", Message: sql.NullString{String: "bringing snacks", Valid: true}, RespondedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if err := rm.Upsert(&models.RSVP{MatchID: matchID, PlayerID: outPlayerID, TeamID: homeTeamID, Status: "no", Message: sql.NullString{String: "out of town", Valid: true}, RespondedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}

		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		code, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "Confirmed") || !strings.Contains(body, "bringing snacks") {
			t.Error("expected the Confirmed list with the yes-RSVP's message")
		}
		if !strings.Contains(body, "Not Attending") || !strings.Contains(body, "out of town") {
			t.Error("expected the Not Attending list with the no-RSVP's message for an upcoming match")
		}
	})

	t.Run("past match hides Not Attending, keeps Confirmed", func(t *testing.T) {
		pastMatchID, err := mm.Insert(&models.Match{
			SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID,
			MatchDate: time.Now().AddDate(0, 0, -3),
			HomeScore: sql.NullInt32{Int32: 3, Valid: true}, AwayScore: sql.NullInt32{Int32: 1, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := rm.Upsert(&models.RSVP{MatchID: pastMatchID, PlayerID: comingPlayerID, TeamID: homeTeamID, Status: "yes", RespondedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if err := rm.Upsert(&models.RSVP{MatchID: pastMatchID, PlayerID: outPlayerID, TeamID: homeTeamID, Status: "no", Message: sql.NullString{String: "out of town", Valid: true}, RespondedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}

		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		code, _, body := ts.get(t, fmt.Sprintf("/match/%d", pastMatchID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "Confirmed") {
			t.Error("expected the Confirmed list to still show for a past match")
		}
		if strings.Contains(body, "Not Attending") {
			t.Error("expected no Not Attending list for a match that already happened")
		}
	})
}

// The roster's "has an account" indicator is only visible to team managers
// (admin, league admin, or captain) — a plain active viewer sees the roster
// with no such column at all.
func TestRosterAccountIndicatorVisibleOnlyToManagers(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Account Indicator Team"})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: testDB}
	linkedPlayerID, err := pm.Insert(&models.Player{FirstName: "Linked", LastName: "Guy"})
	if err != nil {
		t.Fatal(err)
	}
	unlinkedPlayerID, err := pm.Insert(&models.Player{FirstName: "Unlinked", LastName: "Guy"})
	if err != nil {
		t.Fatal(err)
	}
	tmm := &models.TeamMemberModel{DB: testDB}
	if err := tmm.AddMembership(linkedPlayerID, teamID); err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(unlinkedPlayerID, teamID); err != nil {
		t.Fatal(err)
	}

	um := &models.UserModel{DB: testDB}
	linkedUserID, err := um.Insert("roster-account-linked@test.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(linkedUserID, linkedPlayerID); err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, teamID, "roster-account-captain@test.com", "validpassword123")

	t.Run("captain sees the account column", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "roster-account-captain@test.com", "validpassword123")

		code, _, body := ts.get(t, fmt.Sprintf("/team/%d", teamID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "Has signed up") {
			t.Error("expected the linked player to show a signed-up indicator")
		}
		if !strings.Contains(body, "Hasn't signed up yet") {
			t.Error("expected the unlinked player to show a not-signed-up indicator")
		}
	})

	t.Run("plain active viewer sees no account column", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		code, _, body := ts.get(t, fmt.Sprintf("/team/%d", teamID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if strings.Contains(body, "Has signed up") || strings.Contains(body, "Hasn't signed up yet") {
			t.Error("expected no account indicator for a plain active viewer")
		}
	})
}

// The admin user list defaults to most-recent-login-first, with the Last
// Login column header marked as the active sort and linking to toggle the
// direction; picking a different column via query params re-sorts and
// updates which header shows as active.
func TestUserListDefaultSortAndToggle(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testAdminEmail, testAdminPass)

	t.Run("a plain page load defaults to Last Login descending", func(t *testing.T) {
		code, _, body := ts.get(t, "/user/search")
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "sort=lastlogin&order=ASC") {
			t.Error("expected the active Last Login header to link to toggle into ASC")
		}
	})

	t.Run("sorting by email ascending shows email as the active column", func(t *testing.T) {
		code, _, body := ts.get(t, "/user/search?sort=email&order=ASC")
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "sort=email&order=DESC") {
			t.Error("expected the active Email header to link to toggle into DESC")
		}
	})
}

// The home page's Upcoming Matches table only shows which of the player's
// teams a row belongs to when they're on more than one team with an
// upcoming match — a single-team player just sees Date/Opponent/Location,
// no redundant team name.
func TestHomeUpcomingMatchesTeamDisambiguation(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	pm := &models.PlayerModel{DB: testDB}
	um := &models.UserModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}
	sm := &models.SeasonModel{DB: testDB}
	mm := &models.MatchModel{DB: testDB}

	leagueA, err := (&models.LeagueModel{DB: testDB}).Insert(&models.League{Name: "Home Upcoming League A"})
	if err != nil {
		t.Fatal(err)
	}
	leagueB, err := (&models.LeagueModel{DB: testDB}).Insert(&models.League{Name: "Home Upcoming League B"})
	if err != nil {
		t.Fatal(err)
	}
	teamA, err := tm.Insert(&models.Team{LeagueID: leagueA, Name: "Home Upcoming Team A"})
	if err != nil {
		t.Fatal(err)
	}
	teamB, err := tm.Insert(&models.Team{LeagueID: leagueB, Name: "Home Upcoming Team B"})
	if err != nil {
		t.Fatal(err)
	}
	opponentA, err := tm.Insert(&models.Team{LeagueID: leagueA, Name: "Home Upcoming Opponent A"})
	if err != nil {
		t.Fatal(err)
	}
	opponentB, err := tm.Insert(&models.Team{LeagueID: leagueB, Name: "Home Upcoming Opponent B"})
	if err != nil {
		t.Fatal(err)
	}

	seasonA, err := sm.Insert(&models.Season{LeagueID: leagueA, Name: "Home Upcoming Season A"})
	if err != nil {
		t.Fatal(err)
	}
	seasonB, err := sm.Insert(&models.Season{LeagueID: leagueB, Name: "Home Upcoming Season B"})
	if err != nil {
		t.Fatal(err)
	}

	future := time.Now().AddDate(0, 0, 7)
	if _, err := mm.Insert(&models.Match{SeasonID: seasonA, HomeTeamID: teamA, AwayTeamID: opponentA, MatchDate: future}); err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&models.Match{SeasonID: seasonB, HomeTeamID: teamB, AwayTeamID: opponentB, MatchDate: future}); err != nil {
		t.Fatal(err)
	}

	playerID, err := pm.Insert(&models.Player{FirstName: "Multi", LastName: "Team"})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := um.Insert("multi-team-home@test.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.Activate(userID); err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, teamA); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, "multi-team-home@test.com", "validpassword123")

	t.Run("single team shows no team disambiguation", func(t *testing.T) {
		code, _, body := ts.get(t, "/")
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "Home Upcoming Opponent A") {
			t.Error("expected the opponent's name in the Upcoming Matches table")
		}
		if strings.Contains(body, "(Home Upcoming Team A)") {
			t.Error("expected no team-name disambiguation for a single-team player")
		}
	})

	t.Run("two teams with upcoming matches each show their own team name", func(t *testing.T) {
		if err := tmm.AddMembership(playerID, teamB); err != nil {
			t.Fatal(err)
		}

		code, _, body := ts.get(t, "/")
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		if !strings.Contains(body, "(Home Upcoming Team A)") || !strings.Contains(body, "(Home Upcoming Team B)") {
			t.Error("expected both teams' names to disambiguate their rows once the player has two upcoming matches")
		}
	})
}

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
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
// A logged-out hit on an active-tier route (chained through requireActive)
// now carries its own URL through login via `next` (see
// TestLoginRedirectsToNextAfterAuth). Admin-tier routes (requireAdmin only,
// no requireActive in the chain) are untouched by that change and still
// just land on the homepage — not a target for email deep links anyway.
func TestUnauthenticatedAccessRedirects(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tests := []struct {
		name         string
		path         string
		wantLocation string
	}{
		{"team player create", "/team/1/player/create", "/user/login?next=" + url.QueryEscape("/team/1/player/create")},
		{"user search", "/user/search", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, headers, _ := ts.get(t, tt.path)
			if code != http.StatusSeeOther {
				t.Errorf("want %d; got %d", http.StatusSeeOther, code)
			}
			if loc := headers.Get("Location"); loc != tt.wantLocation {
				t.Errorf("want Location %q; got %q", tt.wantLocation, loc)
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

// The captain guide is a plain public page (same tier as /privacy and
// /terms) — reachable with no login. The home page's link to it is gated
// to actual team captains only — a plain active user (on no team, or on a
// team as a regular player) shouldn't see it.
func TestCaptainGuidePage(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Captain Guide Test League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Captain Guide Test Team"})
	if err != nil {
		t.Fatal(err)
	}
	setupTeamCaptain(t, teamID, "captain-guide-captain@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())

	code, _, body := ts.get(t, "/captains")
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	for _, want := range []string{"New Captain's Guide", "Import Roster", "Send Test Reminder"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the captain guide to mention %q", want)
		}
	}

	t.Run("shown to a team captain", func(t *testing.T) {
		ts.login(t, "captain-guide-captain@test.com", "validpassword123")
		_, _, homeBody := ts.get(t, "/")
		if !strings.Contains(homeBody, `href="/captains"`) {
			t.Error("expected the home page to link to the captain guide for a team captain")
		}
	})

	t.Run("hidden from a plain active user", func(t *testing.T) {
		ts.login(t, testActiveEmail, testActivePass)
		_, _, homeBody := ts.get(t, "/")
		if strings.Contains(homeBody, `href="/captains"`) {
			t.Error("expected the home page not to link to the captain guide for a non-captain")
		}
	})
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

// setupRosterMember creates and activates a user, links them to a new
// player, and adds that player to teamID's roster — a plain member, not a
// captain or scorekeeper. Returns the player ID.
func setupRosterMember(t *testing.T, teamID int, email, password string) int {
	t.Helper()

	um := &models.UserModel{DB: testDB}
	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}

	userID, err := um.Insert(email, password)
	if err != nil {
		t.Fatal(err)
	}
	if err := um.Activate(userID); err != nil {
		t.Fatal(err)
	}

	playerID, err := pm.Insert(&models.Player{
		FirstName: "Roster",
		LastName:  "Member",
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

// The Export Roster button's route renders a downloadable PDF for the
// team's captain. Gating itself (admin-or-captain-only) is already covered
// by TestTeamManagerTier above since this route sits on the same
// teamManager tier — this just proves the route itself works.
func TestTeamRosterExport(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Roster Export Team"})
	if err != nil {
		t.Fatal(err)
	}
	setupTeamCaptain(t, teamID, "roster-export-captain@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())
	ts.login(t, "roster-export-captain@test.com", "validpassword123")

	code, headers, body := ts.get(t, fmt.Sprintf("/team/%d/rosterExport", teamID))
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	if ct := headers.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("want Content-Type application/pdf; got %q", ct)
	}
	if len(body) == 0 {
		t.Error("expected non-empty PDF body")
	}
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

// Only a system admin or a league admin of the team's league can mark an
// invite "as captain" — a team's own captain posting asCaptain=on has it
// silently dropped (not an error, just treated as a normal invite), and
// more than one email with asCaptain is rejected regardless of who sends
// it, since a team can only have one captain.
func TestTeamInviteAsCaptainGating(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Captain Invite Gating Team"})
	if err != nil {
		t.Fatal(err)
	}
	setupTeamCaptain(t, teamID, "gating-captain@test.com", "validpassword123")

	im := &models.InviteModel{DB: testDB}

	t.Run("team's own captain cannot mark an invite as captain", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "gating-captain@test.com", "validpassword123")

		_, _, getBody := ts.get(t, fmt.Sprintf("/team/%d/invite", teamID))
		csrfToken := extractCSRFToken(t, getBody)

		form := url.Values{}
		form.Add("csrf_token", csrfToken)
		form.Add("emails", "captain-attempt@test.com")
		form.Add("asCaptain", "on")
		code, _, _ := ts.postForm(t, fmt.Sprintf("/team/%d/invite", teamID), form)
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}

		pending, err := im.ListPendingByTeam(teamID)
		if err != nil {
			t.Fatal(err)
		}
		var found *models.Invite
		for _, invite := range pending {
			if invite.Email == "captain-attempt@test.com" {
				found = invite
			}
		}
		if found == nil {
			t.Fatal("expected the invite to still be sent")
		}
		if found.AsCaptain {
			t.Fatal("expected a non-admin's asCaptain request to be ignored")
		}
	})

	t.Run("admin can mark an invite as captain", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testAdminEmail, testAdminPass)

		_, _, getBody := ts.get(t, fmt.Sprintf("/team/%d/invite", teamID))
		csrfToken := extractCSRFToken(t, getBody)

		form := url.Values{}
		form.Add("csrf_token", csrfToken)
		form.Add("emails", "admin-appointed-captain@test.com")
		form.Add("asCaptain", "on")
		code, _, _ := ts.postForm(t, fmt.Sprintf("/team/%d/invite", teamID), form)
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}

		pending, err := im.ListPendingByTeam(teamID)
		if err != nil {
			t.Fatal(err)
		}
		var found *models.Invite
		for _, invite := range pending {
			if invite.Email == "admin-appointed-captain@test.com" {
				found = invite
			}
		}
		if found == nil {
			t.Fatal("expected the invite to be sent")
		}
		if !found.AsCaptain {
			t.Fatal("expected an admin's asCaptain request to be honored")
		}
	})

	t.Run("multiple emails with asCaptain is rejected", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testAdminEmail, testAdminPass)

		_, _, getBody := ts.get(t, fmt.Sprintf("/team/%d/invite", teamID))
		csrfToken := extractCSRFToken(t, getBody)

		form := url.Values{}
		form.Add("csrf_token", csrfToken)
		form.Add("emails", "one@test.com, two@test.com")
		form.Add("asCaptain", "on")
		code, _, body := ts.postForm(t, fmt.Sprintf("/team/%d/invite", teamID), form)
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("want %d; got %d", http.StatusUnprocessableEntity, code)
		}
		if !strings.Contains(body, "Only one person can be invited as team captain") {
			t.Fatal("expected the multi-email/asCaptain field error to be rendered")
		}
	})
}

// A captain can upload a CSV on the Import Roster page and see the new
// players show up on the roster. The teamManager tier gating itself is
// already covered generically by TestTeamManagerTier, so this only proves
// the route/handler/service wiring works end to end.
func TestTeamRosterImportSubmit(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Roster Import Team"})
	if err != nil {
		t.Fatal(err)
	}
	setupTeamCaptain(t, teamID, "roster-import-captain@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())
	ts.login(t, "roster-import-captain@test.com", "validpassword123")

	_, _, getBody := ts.get(t, fmt.Sprintf("/team/%d/rosterImport", teamID))
	csrfToken := extractCSRFToken(t, getBody)

	csvBody := "Last Name,First Name,Email\nRoute,Test,route-import@test.com\n"
	code, _, body := ts.postMultipart(t, fmt.Sprintf("/team/%d/rosterImport", teamID), csrfToken, "file", "roster.csv", []byte(csvBody))
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	if !strings.Contains(body, "1") || !strings.Contains(body, "player(s) added") {
		t.Fatalf("expected the result page to report 1 player added, got body: %s", body)
	}

	pm := &models.PlayerModel{DB: testDB}
	player, err := pm.GetByEmail("route-import@test.com")
	if err != nil {
		t.Fatal(err)
	}
	tmm := &models.TeamMemberModel{DB: testDB}
	isMember, err := tmm.IsMember(player.ID, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if !isMember {
		t.Fatal("expected the imported player to be on the roster")
	}
}

// The sample CSV template is downloadable by any active user, not just team
// managers — it's just a static format reference.
func TestRosterImportSample(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	code, headers, body := ts.get(t, "/rosterImport/sample.csv")
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	if ct := headers.Get("Content-Type"); ct != "text/csv" {
		t.Errorf("want Content-Type text/csv; got %q", ct)
	}
	if !strings.Contains(body, "Last Name,First Name,Email") {
		t.Fatalf("expected the sample CSV header row, got body: %s", body)
	}
}

func TestSeasonScheduleImportSubmit(t *testing.T) {
	app := newTestApplication(t)

	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: 1, Name: "Schedule Import Season"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Schedule Import Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Schedule Import Away FC"})
	if err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, testAdminEmail, testAdminPass)

	_, _, getBody := ts.get(t, fmt.Sprintf("/season/%d/scheduleImport", seasonID))
	csrfToken := extractCSRFToken(t, getBody)

	csvBody := fmt.Sprintf("Date,Time,Home,Away,Location\n9/13/2026,9:30 AM,%s,%s,\n", "Schedule Import Home FC", "Schedule Import Away FC")
	code, _, body := ts.postMultipart(t, fmt.Sprintf("/season/%d/scheduleImport", seasonID), csrfToken, "file", "schedule.csv", []byte(csvBody))
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	if !strings.Contains(body, "1") || !strings.Contains(body, "match(es) added") {
		t.Fatalf("expected the result page to report 1 match added, got body: %s", body)
	}

	mm := &models.MatchModel{DB: testDB}
	matches, err := mm.GetBySeason(seasonID)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].HomeTeamID != homeID || matches[0].AwayTeamID != awayID {
		t.Fatalf("expected 1 match between the imported teams, got %+v", matches)
	}
}

// The sample CSV template is downloadable by any active user, not just team
// managers — it's just a static format reference.
func TestScheduleImportSample(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	code, headers, body := ts.get(t, "/scheduleImport/sample.csv")
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	if ct := headers.Get("Content-Type"); ct != "text/csv" {
		t.Errorf("want Content-Type text/csv; got %q", ct)
	}
	if !strings.Contains(body, "Date,Time,Home,Away,Location") {
		t.Fatalf("expected the sample CSV header row, got body: %s", body)
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
		if !strings.Contains(formBody, `name='matchtime' value='09:30'`) {
			t.Error("expected the fresh create form's time field to default to 09:30")
		}
		matchCSRF := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, "/admin/match/create", url.Values{
			"seasonID":   {fmt.Sprintf("%d", seasonID)},
			"hometeamid": {fmt.Sprintf("%d", homeTeamID)},
			"awayteamid": {fmt.Sprintf("%d", awayTeamID)},
			"matchdate":  {"2024-05-05"},
			"matchtime":  {"09:30"},
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

// Deleting a season is a bulk delete — its matches (and everything
// attached to them) don't need to be removed by hand first.
func TestSeasonDeleteBulkDeletesMatches(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Season Bulk Delete League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Season Bulk Delete Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Season Bulk Delete Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Season Bulk Delete Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, testAdminEmail, testAdminPass)

	code, _, _ := ts.delete(t, fmt.Sprintf("/admin/season/delete/%d", seasonID))
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}

	if _, err := sm.Get(seasonID); err != models.ErrNoRecord {
		t.Fatalf("expected the season to be gone, got %v", err)
	}
	if _, err := mm.Get(matchID); err != models.ErrNoRecord {
		t.Fatalf("expected the match to be gone too (bulk deleted), not left dangling or blocking the season delete, got %v", err)
	}
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

// The league page's Matches tab groups a season's matches into per-day
// cards (see groupMatchesByDay in handlers_leagues.go) instead of
// season-view.html's flat table — this exercises that grouping end to end
// rather than just unit-testing the grouping function in isolation.
func TestLeagueMatchesTabGroupsByDay(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Matches Tab Test League"})
	if err != nil {
		t.Fatal(err)
	}

	tm := &models.TeamModel{DB: testDB}
	teamA, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Matches Tab Team A"})
	if err != nil {
		t.Fatal(err)
	}
	teamB, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Matches Tab Team B"})
	if err != nil {
		t.Fatal(err)
	}
	teamC, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Matches Tab Team C"})
	if err != nil {
		t.Fatal(err)
	}

	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Matches Tab Season"})
	if err != nil {
		t.Fatal(err)
	}

	mm := &models.MatchModel{DB: testDB}
	day1 := time.Date(2026, 9, 13, 13, 30, 0, 0, time.UTC) // 9:30 AM Eastern (EDT)
	day2 := time.Date(2026, 9, 20, 13, 30, 0, 0, time.UTC)
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamA, AwayTeamID: teamB, MatchDate: day1,
		HomeScore: sql.NullInt32{Int32: 2, Valid: true}, AwayScore: sql.NullInt32{Int32: 1, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamA, AwayTeamID: teamC, MatchDate: day2}); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	code, _, body := ts.get(t, fmt.Sprintf("/league/%d?tab=matches", leagueID))
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	if !strings.Contains(body, "Sunday, September 13, 2026") || !strings.Contains(body, "Sunday, September 20, 2026") {
		t.Error("expected a matchday heading for each of the two match dates")
	}
	if !strings.Contains(body, "2&ndash;1") {
		t.Error("expected the played match's score to render on its card")
	}
	if !strings.Contains(body, "9:30 AM") {
		t.Error("expected the upcoming match's Eastern kickoff time to render on its card")
	}
	if !strings.Contains(body, "Matches Tab Team A") || !strings.Contains(body, "Matches Tab Team B") || !strings.Contains(body, "Matches Tab Team C") {
		t.Error("expected all three team names to appear across the two matchday cards")
	}
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
			"matchtime":  {"09:30"},
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

	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}
	awayPlayerID, err := pm.Insert(&models.Player{FirstName: "Unlucky", LastName: "Defender"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(awayPlayerID, awayTeamID); err != nil {
		t.Fatal(err)
	}

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
			"matchtime":  {"09:30"},
			"homescore":  {"2"},
			"awayscore":  {"1"},
			"goalTeamID": {fmt.Sprintf("%d", homeTeamID), fmt.Sprintf("%d", awayTeamID)},
			// First goal: scored and assisted by the same test player (not
			// realistic, but exercises both tallies incrementing). Second
			// goal (away team) intentionally has no scorer/assister. Minutes
			// chosen so chronological order matches submission order.
			"goalScorerID":   {fmt.Sprintf("%d", scorerID), ""},
			"goalAssisterID": {fmt.Sprintf("%d", scorerID), ""},
			"goalMinute":     {"10", "50"},
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
		if goals[0].TeamID != homeTeamID || !goals[0].ScorerPlayerID.Valid || int(goals[0].ScorerPlayerID.Int32) != scorerID || goals[0].Minute.Int32 != 10 {
			t.Fatalf("expected the first goal attributed to the scorer at minute 10, got %+v", goals[0])
		}
		if goals[1].TeamID != awayTeamID || goals[1].ScorerPlayerID.Valid || goals[1].Minute.Int32 != 50 {
			t.Fatalf("expected the second goal unattributed for the away team at minute 50, got %+v", goals[1])
		}

		_, _, viewBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(viewBody, "10") || !strings.Contains(viewBody, "50") {
			t.Error("expected both goal minutes to appear on the match view box score")
		}
		if !strings.Contains(viewBody, "Unattributed goal") {
			t.Error("expected the unattributed away goal to show as such on the match view")
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

	t.Run("an own goal can be credited to a team with a scorer from the other roster", func(t *testing.T) {
		form := url.Values{
			"matchdate":    {"2024-05-05"},
			"matchtime":    {"09:30"},
			"homescore":    {"2"},
			"awayscore":    {"0"},
			"goalTeamID":   {fmt.Sprintf("%d", homeTeamID)},
			"goalScorerID": {fmt.Sprintf("%d", awayPlayerID)},
			"goalMinute":   {"75"},
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
		if len(goals) != 1 || goals[0].TeamID != homeTeamID || !goals[0].ScorerPlayerID.Valid || int(goals[0].ScorerPlayerID.Int32) != awayPlayerID {
			t.Fatalf("expected the own goal saved credited to home with the away player as scorer, got %+v", goals)
		}

		_, _, viewBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(viewBody, "(own goal)") {
			t.Error("expected the match view box score to flag the own goal")
		}

		pmsm := &models.PlayerMatchStatModel{DB: testDB}
		stats, err := pmsm.ListByMatch(matchID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stats) != 1 || stats[0].PlayerID != awayPlayerID || stats[0].TeamID != awayTeamID || stats[0].Goals != 0 || stats[0].OwnGoals != 1 {
			t.Fatalf("expected the scorer tallied under the away team with OwnGoals=1 and Goals=0, got %+v", stats)
		}
	})

	t.Run("resubmitting with fewer rows replaces rather than accumulates", func(t *testing.T) {
		form := url.Values{
			"matchdate":    {"2024-05-05"},
			"matchtime":    {"09:30"},
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

	t.Run("upcoming match shows both Confirmed and Not Attending to the team's own roster", func(t *testing.T) {
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

		// A member of the home roster — before the match has a result, a
		// non-manager only sees their own team's box (see
		// TestMatchScreenBoxVisibilityBeforeResult for the hidden case).
		setupRosterMember(t, homeTeamID, "attendance-viewer@test.com", "validpassword123")
		ts := newTestServer(t, app.routes())
		ts.login(t, "attendance-viewer@test.com", "validpassword123")

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

// The team page's roster table defaults to goals descending once there's a
// leaderboard to sort by, and its column headers toggle sort/order via
// query params, same pattern as the admin user list.
func TestTeamRosterDefaultSortAndToggle(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Roster Sort League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Roster Sort Team"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Roster Sort Opponent"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Roster Sort Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: teamID, AwayTeamID: opponentID, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 3, Valid: true}, AwayScore: sql.NullInt32{Int32: 0, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}
	pmsm := &models.PlayerMatchStatModel{DB: testDB}

	lowScorerID, err := pm.Insert(&models.Player{FirstName: "Aaron", LastName: "Low"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(lowScorerID, teamID); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&models.PlayerMatchStat{MatchID: matchID, PlayerID: lowScorerID, TeamID: teamID, Goals: 1}); err != nil {
		t.Fatal(err)
	}

	highScorerID, err := pm.Insert(&models.Player{FirstName: "Zack", LastName: "High"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(highScorerID, teamID); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&models.PlayerMatchStat{MatchID: matchID, PlayerID: highScorerID, TeamID: teamID, Goals: 2}); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	t.Run("defaults to goals descending", func(t *testing.T) {
		code, _, body := ts.get(t, fmt.Sprintf("/team/%d", teamID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		highIdx := strings.Index(body, "Zack High")
		lowIdx := strings.Index(body, "Aaron Low")
		if highIdx == -1 || lowIdx == -1 || highIdx > lowIdx {
			t.Errorf("expected the 2-goal scorer to appear before the 1-goal scorer by default, got indices %d, %d", highIdx, lowIdx)
		}
		if !strings.Contains(body, "sort=goals&order=ASC") {
			t.Error("expected the active Goals header to link to toggle into ASC")
		}
	})

	t.Run("sorting by name ascending shows name as the active column", func(t *testing.T) {
		code, _, body := ts.get(t, fmt.Sprintf("/team/%d?sort=name&order=ASC", teamID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		// Sorted by lastname then firstname: "High" sorts before "Low".
		highIdx := strings.Index(body, "Zack High")
		lowIdx := strings.Index(body, "Aaron Low")
		if highIdx == -1 || lowIdx == -1 || highIdx > lowIdx {
			t.Errorf("expected alphabetical-by-lastname order (High before Low) when sorted by name, got indices %d, %d", highIdx, lowIdx)
		}
		if !strings.Contains(body, "sort=name&order=DESC") {
			t.Error("expected the active Name header to link to toggle into DESC")
		}
	})
}

// The team page's leaders line includes an Own goal leader alongside the
// existing leading scorer/assister, once someone on the roster has one.
func TestTeamViewOwnGoalLeader(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Own Goal Leader League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Own Goal Leader Team"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Own Goal Leader Opponent"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Own Goal Leader Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: teamID, AwayTeamID: opponentID, MatchDate: time.Now(),
		HomeScore: sql.NullInt32{Int32: 0, Valid: true}, AwayScore: sql.NullInt32{Int32: 1, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}
	pmsm := &models.PlayerMatchStatModel{DB: testDB}

	unluckyID, err := pm.Insert(&models.Player{FirstName: "Unlucky", LastName: "Defender"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(unluckyID, teamID); err != nil {
		t.Fatal(err)
	}
	if err := pmsm.Upsert(&models.PlayerMatchStat{MatchID: matchID, PlayerID: unluckyID, TeamID: teamID, OwnGoals: 1}); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	code, _, body := ts.get(t, fmt.Sprintf("/team/%d", teamID))
	if code != http.StatusOK {
		t.Fatalf("want %d; got %d", http.StatusOK, code)
	}
	if !strings.Contains(body, "Own goal leader: Unlucky Defender (1)") {
		t.Error("expected the Own goal leader line to appear on the team page")
	}
}

// A captain can designate a roster member as a scorekeeper, which grants
// that player match-editing rights (score/goals/cards) for the team's
// matches without granting the wider team-management rights (roster,
// invites) a captain has. Removing the designation revokes match access
// again.
func TestScorekeeperTier(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Scorekeeper Test League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Scorekeeper Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Scorekeeper Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Scorekeeper Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, homeTeamID, "scorekeeper-captain@test.com", "validpassword123")
	candidateID := setupRosterMember(t, homeTeamID, "scorekeeper-candidate@test.com", "validpassword123")
	setupRosterMember(t, homeTeamID, "scorekeeper-bystander@test.com", "validpassword123")

	captainTS := newTestServer(t, app.routes())
	captainTS.login(t, "scorekeeper-captain@test.com", "validpassword123")

	t.Run("captain can designate a roster member as scorekeeper", func(t *testing.T) {
		_, _, formBody := captainTS.get(t, fmt.Sprintf("/team/%d", homeTeamID))
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := captainTS.postForm(t, "/admin/team/scorekeepers/add", url.Values{
			"teamID":     {fmt.Sprintf("%d", homeTeamID)},
			"playerID":   {fmt.Sprintf("%d", candidateID)},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/team/%d", homeTeamID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/team/%d", homeTeamID), loc)
		}

		tsm := &models.TeamScorekeeperModel{DB: testDB}
		isScorekeeper, err := tsm.IsScorekeeper(candidateID, homeTeamID)
		if err != nil {
			t.Fatal(err)
		}
		if !isScorekeeper {
			t.Fatal("expected candidate to be a scorekeeper of the home team")
		}

		_, _, body := captainTS.get(t, fmt.Sprintf("/team/%d", homeTeamID))
		if !strings.Contains(body, "Remove Scorekeeper") {
			t.Error("expected the roster row's action to flip to Remove Scorekeeper")
		}
	})

	t.Run("scorekeeper can edit the team's match but not manage the team", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "scorekeeper-candidate@test.com", "validpassword123")

		code, _, formBody := ts.get(t, fmt.Sprintf("/admin/match/update/%d", matchID))
		if code != http.StatusOK {
			t.Fatalf("want %d; got %d", http.StatusOK, code)
		}
		csrfToken := extractCSRFToken(t, formBody)

		postCode, _, _ := ts.postForm(t, "/admin/match/update", url.Values{
			"match-id":   {fmt.Sprintf("%d", matchID)},
			"matchdate":  {"2024-05-05"},
			"matchtime":  {"09:30"},
			"homescore":  {"1"},
			"awayscore":  {"0"},
			"csrf_token": {csrfToken},
		})
		if postCode != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, postCode)
		}

		inviteCode, headers, _ := ts.get(t, fmt.Sprintf("/team/%d/invite", homeTeamID))
		if inviteCode != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, inviteCode)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("a plain roster member who isn't a scorekeeper cannot edit the match", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "scorekeeper-bystander@test.com", "validpassword123")

		code, headers, _ := ts.get(t, fmt.Sprintf("/admin/match/update/%d", matchID))
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("captain can revoke scorekeeper status", func(t *testing.T) {
		_, _, formBody := captainTS.get(t, fmt.Sprintf("/team/%d", homeTeamID))
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := captainTS.postForm(t, "/admin/team/scorekeepers/remove", url.Values{
			"teamID":     {fmt.Sprintf("%d", homeTeamID)},
			"playerID":   {fmt.Sprintf("%d", candidateID)},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/team/%d", homeTeamID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/team/%d", homeTeamID), loc)
		}

		ts := newTestServer(t, app.routes())
		ts.login(t, "scorekeeper-candidate@test.com", "validpassword123")

		matchCode, matchHeaders, _ := ts.get(t, fmt.Sprintf("/admin/match/update/%d", matchID))
		if matchCode != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, matchCode)
		}
		if loc := matchHeaders.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})
}

// Each team's own captain (or a scorekeeper they've designated) can set
// that team's Player of the Match and captain's notes — but only for their
// own side. Unlike score/goals/cards (either team's manager can edit the
// shared record), a manager of one team cannot touch the other team's
// notes.
func TestMatchTeamNotes(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Match Notes Test League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Notes Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Notes Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Notes Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	homeCaptainID := setupTeamCaptain(t, homeTeamID, "notes-home-captain@test.com", "validpassword123")
	awayCaptainID := setupTeamCaptain(t, awayTeamID, "notes-away-captain@test.com", "validpassword123")
	scorekeeperID := setupRosterMember(t, homeTeamID, "notes-home-scorekeeper@test.com", "validpassword123")
	tsm := &models.TeamScorekeeperModel{DB: testDB}
	if err := tsm.AddScorekeeper(scorekeeperID, homeTeamID); err != nil {
		t.Fatal(err)
	}

	t.Run("home captain sets the home team's Player of the Match, notes, and captain's message", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "notes-home-captain@test.com", "validpassword123")

		_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/notes", matchID), url.Values{
			"teamID":          {fmt.Sprintf("%d", homeTeamID)},
			"playerOfMatchID": {fmt.Sprintf("%d", homeCaptainID)},
			"notes":           {"Great effort from everyone"},
			"captainMessage":  {"Meet at the field 30 minutes early"},
			"csrf_token":      {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
		}

		_, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(body, "Great effort from everyone") {
			t.Error("expected the saved notes to appear on the match view")
		}
		if !strings.Contains(body, "Cap Tain") {
			t.Error("expected the Player of the Match's name to appear on the match view")
		}
		if !strings.Contains(body, "Meet at the field 30 minutes early") {
			t.Error("expected the saved captain's message to round-trip into the edit form")
		}
		if !strings.Contains(body, "only shown in the reminder email") {
			t.Error("expected the captain's message to be flagged as set without its text appearing in the read-only summary")
		}
	})

	t.Run("home captain cannot set the away team's notes", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "notes-home-captain@test.com", "validpassword123")

		_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/notes", matchID), url.Values{
			"teamID":     {fmt.Sprintf("%d", awayTeamID)},
			"notes":      {"trying to edit the other side"},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}

		mtnm := &models.MatchTeamNoteModel{DB: testDB}
		if _, err := mtnm.GetByMatchAndTeam(matchID, awayTeamID); !errors.Is(err, models.ErrNoRecord) {
			t.Fatalf("expected the away team's note to remain unset, got %v", err)
		}
	})

	t.Run("a designated scorekeeper cannot edit the home team's Player of the Match/Notes/RSVP message", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "notes-home-scorekeeper@test.com", "validpassword123")

		// Scorekeepers get match-day editing (score/goals/cards) but not
		// this captain-only section — they can still see the read-only
		// Player of the Match/Notes (they're a roster member), just not
		// edit them, so a CSRF token is available regardless of which form
		// it comes from.
		_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if strings.Contains(formBody, "field-label\">Player of the Match<") {
			t.Error("expected the scorekeeper to see no editable form, only the read-only view")
		}
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/notes", matchID), url.Values{
			"teamID":     {fmt.Sprintf("%d", homeTeamID)},
			"notes":      {"updated by the scorekeeper"},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}

		_, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if strings.Contains(body, "updated by the scorekeeper") {
			t.Error("expected the scorekeeper's update to be rejected, not saved")
		}
		if !strings.Contains(body, "Great effort from everyone") {
			t.Error("expected the previously saved notes to remain unchanged")
		}
	})

	t.Run("picking a Player of the Match not on the roster fails validation without losing the saved note", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "notes-home-captain@test.com", "validpassword123")

		_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken := extractCSRFToken(t, formBody)

		// awayCaptainID belongs to the away team's roster, not the home team's.
		mtnm := &models.MatchTeamNoteModel{DB: testDB}
		beforeNote, err := mtnm.GetByMatchAndTeam(matchID, homeTeamID)
		if err != nil {
			t.Fatal(err)
		}

		code, _, body := ts.postForm(t, fmt.Sprintf("/match/%d/notes", matchID), url.Values{
			"teamID":          {fmt.Sprintf("%d", homeTeamID)},
			"playerOfMatchID": {fmt.Sprintf("%d", awayCaptainID)},
			"notes":           {"this should not save"},
			"csrf_token":      {csrfToken},
		})
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("want %d; got %d", http.StatusUnprocessableEntity, code)
		}
		if !strings.Contains(body, "roster") {
			t.Error("expected a field error mentioning the roster requirement")
		}

		afterNote, err := mtnm.GetByMatchAndTeam(matchID, homeTeamID)
		if err != nil {
			t.Fatal(err)
		}
		if afterNote.Notes.String != beforeNote.Notes.String {
			t.Errorf("expected the previously saved note to remain untouched, got %q", afterNote.Notes.String)
		}
	})
}

// A logged-out hit on a protected page (the active tier — requireActive)
// carries its own URL through login via a `next` param, so a deep link
// (an email's "RSVP Now!" link, for instance) survives having to log in
// first, rather than always landing on the homepage.
func TestLoginRedirectsToNextAfterAuth(t *testing.T) {
	app := newTestApplication(t)

	t.Run("a logged-out hit on a protected page redirects to login with next set", func(t *testing.T) {
		ts := newTestServer(t, app.routes())

		code, headers, _ := ts.get(t, "/league")
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		wantLocation := "/user/login?next=" + url.QueryEscape("/league")
		if loc := headers.Get("Location"); loc != wantLocation {
			t.Errorf("want Location %q; got %q", wantLocation, loc)
		}
	})

	t.Run("logging in through that redirect lands back on the original page", func(t *testing.T) {
		ts := newTestServer(t, app.routes())

		_, _, body := ts.get(t, "/user/login?next="+url.QueryEscape("/league"))
		if !strings.Contains(body, "value='/league'") {
			t.Error("expected the login form's hidden next field to carry /league through")
		}
		csrfToken := extractCSRFToken(t, body)

		code, headers, _ := ts.postForm(t, "/user/login", url.Values{
			"email":      {testActiveEmail},
			"password":   {testActivePass},
			"next":       {"/league"},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/league" {
			t.Errorf("want Location %q; got %q", "/league", loc)
		}
	})

	t.Run("an unsafe next falls back to the homepage instead of following it", func(t *testing.T) {
		ts := newTestServer(t, app.routes())

		_, _, formBody := ts.get(t, "/user/login")
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, "/user/login", url.Values{
			"email":      {testActiveEmail},
			"password":   {testActivePass},
			"next":       {"//evil.example.com"},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q (unsafe next rejected); got %q", "/", loc)
		}
	})
}

// The per-match "Send Test Reminder" tool — a captain (or admin) previewing
// the real RSVP reminder email's content/delivery on demand, either by
// typing addresses directly or picking from teammates who already have an
// activated account. Unlike the old admin-wide trigger this replaces, it's
// not date-gated and records nothing to matchRSVPReminders.
func TestMatchTestReminderSubmit(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Test Reminder League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Test Reminder Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Test Reminder Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Test Reminder Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	// Deliberately far from the real 3/2/1-day RSVP schedule — proves the
	// test-reminder tool isn't date-gated like the real scheduled sends.
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID,
		MatchDate: time.Now().AddDate(0, 0, 30),
	})
	if err != nil {
		t.Fatal(err)
	}

	homeCaptainID := setupTeamCaptain(t, homeTeamID, "test-reminder-captain@test.com", "validpassword123")

	mrrm := &models.MatchRSVPReminderModel{DB: testDB}

	t.Run("the captain can send a test reminder to a typed-in email", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "test-reminder-captain@test.com", "validpassword123")

		_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/testReminder", matchID), url.Values{
			"teamID":     {fmt.Sprintf("%d", homeTeamID)},
			"emails":     {"someone@example.com, not-an-email, another@example.com"},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
		}

		// A far-future match is nowhere near the real 3/2/1-day schedule —
		// the test send must not have touched the real tracking table.
		wasSent, err := mrrm.WasSent(matchID, homeCaptainID, 3)
		if err != nil {
			t.Fatal(err)
		}
		if wasSent {
			t.Fatal("expected the test reminder to record nothing to matchRSVPReminders")
		}
	})

	t.Run("a plain roster member (not the captain) can't send a test reminder", func(t *testing.T) {
		setupRosterMember(t, homeTeamID, "test-reminder-bystander@test.com", "validpassword123")
		ts := newTestServer(t, app.routes())
		ts.login(t, "test-reminder-bystander@test.com", "validpassword123")

		_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/testReminder", matchID), url.Values{
			"teamID":     {fmt.Sprintf("%d", homeTeamID)},
			"emails":     {"someone@example.com"},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("selecting an activated teammate resolves to their real login email", func(t *testing.T) {
		um := &models.UserModel{DB: testDB}
		pm := &models.PlayerModel{DB: testDB}
		tmm := &models.TeamMemberModel{DB: testDB}
		teammateID, err := pm.Insert(&models.Player{FirstName: "Active", LastName: "Teammate"})
		if err != nil {
			t.Fatal(err)
		}
		if err := tmm.AddMembership(teammateID, homeTeamID); err != nil {
			t.Fatal(err)
		}
		userID, err := um.Insert("active-teammate@test.com", "validpassword123")
		if err != nil {
			t.Fatal(err)
		}
		if err := um.Activate(userID); err != nil {
			t.Fatal(err)
		}
		if err := um.SetPlayerID(userID, teammateID); err != nil {
			t.Fatal(err)
		}

		ts := newTestServer(t, app.routes())
		ts.login(t, "test-reminder-captain@test.com", "validpassword123")

		_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(formBody, "active-teammate@test.com") {
			t.Error("expected the activated teammate to appear in the test-reminder picker")
		}
		csrfToken := extractCSRFToken(t, formBody)

		code, _, _ := ts.postForm(t, fmt.Sprintf("/match/%d/testReminder", matchID), url.Values{
			"teamID":     {fmt.Sprintf("%d", homeTeamID)},
			"playerIDs":  {fmt.Sprintf("%d", teammateID)},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
	})

	t.Run("no addresses and no selection redirects back with a flash, sends nothing", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "test-reminder-captain@test.com", "validpassword123")

		_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/testReminder", matchID), url.Values{
			"teamID":     {fmt.Sprintf("%d", homeTeamID)},
			"csrf_token": {csrfToken},
		})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
			t.Errorf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
		}
	})
}

// The SMS side of Test Reminder has no free-text option at all — only
// roster players with a verified phone number appear in the picker, and
// SendTestReminderSMS re-checks that server-side regardless of what's
// posted.
func TestMatchTestReminderSMSSubmit(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Test Reminder SMS League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Test Reminder SMS Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Test Reminder SMS Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Test Reminder SMS Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{
		SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID,
		MatchDate: time.Now().AddDate(0, 0, 30),
	})
	if err != nil {
		t.Fatal(err)
	}

	setupTeamCaptain(t, homeTeamID, "test-reminder-sms-captain@test.com", "validpassword123")

	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}

	verifiedID, err := pm.Insert(&models.Player{FirstName: "Verified", LastName: "Teammate", PhoneNumber: sql.NullString{String: "518-555-0100", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(verifiedID, homeTeamID); err != nil {
		t.Fatal(err)
	}
	if err := pm.SetPhoneVerificationCode(verifiedID, "123456", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := pm.ConfirmPhoneVerified(verifiedID); err != nil {
		t.Fatal(err)
	}

	unverifiedID, err := pm.Insert(&models.Player{FirstName: "Unverified", LastName: "Teammate", PhoneNumber: sql.NullString{String: "518-555-0101", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(unverifiedID, homeTeamID); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, "test-reminder-sms-captain@test.com", "validpassword123")

	_, _, formBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
	// Both players' names legitimately appear elsewhere on the page (e.g.
	// the Player of the Match dropdown lists the whole roster regardless
	// of verification), so check for the phone number the SMS picker
	// specifically renders alongside a verified teammate's name — that
	// only ever appears in the VerifiedPhoneTeammates picker.
	if !strings.Contains(formBody, "(518-555-0100)") {
		t.Error("expected the verified teammate's phone to appear in the SMS test-reminder picker")
	}
	if strings.Contains(formBody, "(518-555-0101)") {
		t.Error("expected the unverified teammate to NOT appear in the SMS test-reminder picker")
	}
	csrfToken := extractCSRFToken(t, formBody)

	// Post both IDs — even the unverified one, which a crafted request
	// could add even though the UI never offers it — and confirm only the
	// verified one is actually counted as sent.
	code, headers, _ := ts.postForm(t, fmt.Sprintf("/match/%d/testReminderSMS", matchID), url.Values{
		"teamID":       {fmt.Sprintf("%d", homeTeamID)},
		"smsPlayerIDs": {fmt.Sprintf("%d", verifiedID), fmt.Sprintf("%d", unverifiedID)},
		"csrf_token":   {csrfToken},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
	}
	if loc := headers.Get("Location"); loc != fmt.Sprintf("/match/%d", matchID) {
		t.Errorf("want Location %q; got %q", fmt.Sprintf("/match/%d", matchID), loc)
	}

	_, _, afterBody := ts.get(t, headers.Get("Location"))
	if !strings.Contains(afterBody, "Test text sent to 1 teammate(s).") {
		t.Error("expected the flash to report exactly 1 teammate texted (the unverified one must not count)")
	}
}

// While SMS_FEATURE_ENABLED isn't set to "true" — deliberately a
// different signal than whether SMS_ACCOUNT_SID/credentials exist, since
// real credentials can sit in the environment for testing before a
// number is actually carrier-approved — the whole notifications feature
// (the link on a player's profile, and the page itself) is hidden rather
// than offering a "verify your phone" flow that can't actually deliver
// anything to real users yet.
func TestPlayerNotificationsHiddenWithoutSMSConfigured(t *testing.T) {
	os.Setenv("SMS_FEATURE_ENABLED", "false")
	defer os.Setenv("SMS_FEATURE_ENABLED", "true")

	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "SMS Disabled Team"})
	if err != nil {
		t.Fatal(err)
	}
	playerID := setupRosterMember(t, teamID, "sms-disabled-self@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())
	ts.login(t, "sms-disabled-self@test.com", "validpassword123")

	_, _, profileBody := ts.get(t, fmt.Sprintf("/player/view/%d", playerID))
	if strings.Contains(profileBody, "Notification Preferences") {
		t.Error("expected the Notification Preferences link to be hidden while SMS isn't configured")
	}

	code, _, _ := ts.get(t, fmt.Sprintf("/player/notifications/%d", playerID))
	if code != http.StatusNotFound {
		t.Errorf("want %d; got %d", http.StatusNotFound, code)
	}
}

// Notification Preferences is a player's own account settings — narrower
// than canManagePlayer, which also lets an admin or a captain/league-admin
// of the player's team manage roster contact info. Nobody but the player
// themself can reach it, including their own team's captain and an admin.
func TestPlayerNotificationsGating(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Notifications Gating Team"})
	if err != nil {
		t.Fatal(err)
	}
	playerID := setupRosterMember(t, teamID, "notif-gating-self@test.com", "validpassword123")
	setupTeamCaptain(t, teamID, "notif-gating-captain@test.com", "validpassword123")

	t.Run("the player themself can reach it", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "notif-gating-self@test.com", "validpassword123")
		code, _, _ := ts.get(t, fmt.Sprintf("/player/notifications/%d", playerID))
		if code != http.StatusOK {
			t.Errorf("want %d; got %d", http.StatusOK, code)
		}
	})

	t.Run("that team's own captain cannot", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "notif-gating-captain@test.com", "validpassword123")
		code, headers, _ := ts.get(t, fmt.Sprintf("/player/notifications/%d", playerID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})

	t.Run("an admin cannot", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testAdminEmail, testAdminPass)
		code, headers, _ := ts.get(t, fmt.Sprintf("/player/notifications/%d", playerID))
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})
}

// End-to-end: a player requests a verification code, confirms it, and can
// then set an RSVP-reminder preference of "sms" — and that setting sms
// before ever verifying is rejected.
func TestPlayerNotificationsVerifyPhoneAndSetPreference(t *testing.T) {
	app := newTestApplication(t)

	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: 1, Name: "Notifications Flow Team"})
	if err != nil {
		t.Fatal(err)
	}
	playerID := setupRosterMember(t, teamID, "notif-flow-self@test.com", "validpassword123")

	ts := newTestServer(t, app.routes())
	ts.login(t, "notif-flow-self@test.com", "validpassword123")

	_, _, body := ts.get(t, fmt.Sprintf("/player/notifications/%d", playerID))
	csrfToken := extractCSRFToken(t, body)

	// Setting sms before ever verifying a phone is rejected.
	code, headers, _ := ts.postForm(t, fmt.Sprintf("/player/notifications/%d/preferences", playerID), url.Values{
		"csrf_token":  {csrfToken},
		"rsvpChannel": {"sms"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
	}
	pm := &models.PlayerModel{DB: testDB}
	npm := &models.NotificationPreferenceModel{DB: testDB}
	channel, err := npm.GetChannel(playerID, models.CategoryRSVPReminder)
	if err != nil {
		t.Fatal(err)
	}
	if channel != models.ChannelEmail {
		t.Fatalf("expected the sms preference to be rejected (still default %q), got %q", models.ChannelEmail, channel)
	}

	// Request a verification code.
	code, headers, _ = ts.postForm(t, fmt.Sprintf("/player/notifications/%d/phone", playerID), url.Values{
		"csrf_token":  {csrfToken},
		"phonenumber": {"518-555-0100"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
	}

	player, err := pm.Get(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if !player.PhoneVerificationCode.Valid {
		t.Fatal("expected a pending verification code after requesting one")
	}

	// Confirm with the real code.
	_, _, body = ts.get(t, headers.Get("Location"))
	csrfToken = extractCSRFToken(t, body)
	code, headers, _ = ts.postForm(t, fmt.Sprintf("/player/notifications/%d/phone/confirm", playerID), url.Values{
		"csrf_token": {csrfToken},
		"code":       {player.PhoneVerificationCode.String},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
	}

	verified, err := pm.Get(playerID)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.PhoneVerifiedAt.Valid {
		t.Fatal("expected the phone to be verified")
	}

	// Now sms is accepted.
	_, _, body = ts.get(t, headers.Get("Location"))
	csrfToken = extractCSRFToken(t, body)
	code, _, _ = ts.postForm(t, fmt.Sprintf("/player/notifications/%d/preferences", playerID), url.Values{
		"csrf_token":  {csrfToken},
		"rsvpChannel": {"sms"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
	}
	channel, err = npm.GetChannel(playerID, models.CategoryRSVPReminder)
	if err != nil {
		t.Fatal(err)
	}
	if channel != models.ChannelSMS {
		t.Fatalf("expected channel %q after verifying, got %q", models.ChannelSMS, channel)
	}
}

// A viewer who captains one of a match's two teams sees edit controls
// (Player of the Match/Notes/Captain's Message form, Send Test Reminder)
// for only their own side — even when they're also an admin, whose access
// would otherwise reach both sides. A viewer who isn't captain of either
// team (a plain admin, here) is unaffected and still sees both.
func TestMatchScreenEditBoxesScopedToOwnCaptain(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Scoped Edit Boxes League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Scoped Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Scoped Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Scoped Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	// setupTeamCaptain's player isn't an admin; promote this one to admin
	// too, mirroring the real case that prompted this (a captain whose
	// account also happens to be a system admin).
	um := &models.UserModel{DB: testDB}
	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}
	adminCaptainID, err := pm.Insert(&models.Player{FirstName: "Admin", LastName: "Captain"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(adminCaptainID, homeTeamID); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetCaptain(homeTeamID, sql.NullInt32{Int32: int32(adminCaptainID), Valid: true}); err != nil {
		t.Fatal(err)
	}
	adminCaptainUserID, err := um.Insert("admin-captain@test.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.Activate(adminCaptainUserID); err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(adminCaptainUserID, adminCaptainID); err != nil {
		t.Fatal(err)
	}
	if err := um.InsertUserRole(adminCaptainUserID, "ADMIN"); err != nil {
		t.Fatal(err)
	}

	homeTeamIDField := fmt.Sprintf("name='teamID' value='%d'", homeTeamID)
	awayTeamIDField := fmt.Sprintf("name='teamID' value='%d'", awayTeamID)

	t.Run("an admin who captains the home team sees only the home team's edit boxes", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "admin-captain@test.com", "validpassword123")

		_, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(body, homeTeamIDField) {
			t.Error("expected the home team's edit controls to be visible")
		}
		if strings.Contains(body, awayTeamIDField) {
			t.Error("expected the away team's edit controls to be hidden from the home captain, even though they're an admin")
		}
	})

	t.Run("a plain admin who captains neither team sees both teams' edit boxes", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testAdminEmail, testAdminPass)

		_, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(body, homeTeamIDField) {
			t.Error("expected the home team's edit controls to be visible to a plain admin")
		}
		if !strings.Contains(body, awayTeamIDField) {
			t.Error("expected the away team's edit controls to be visible to a plain admin")
		}
	})
}

// "View as player" lets an admin (who is very often also a captain/league
// admin, like this app's real primary user) temporarily see the site
// exactly as a plain roster member would — no admin/captain/league-admin
// controls anywhere, even though the underlying account still holds those
// roles. Only a real admin can toggle it, and it never survives a fresh
// login.
func TestViewAsPlayerToggle(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "View As Player League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	teamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "View As Player FC"})
	if err != nil {
		t.Fatal(err)
	}
	opponentID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "View As Player Opponent FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "View As Player Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: teamID, AwayTeamID: opponentID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	// An admin who is also this team's captain and this league's admin —
	// mirroring the real account this feature is for.
	um := &models.UserModel{DB: testDB}
	pm := &models.PlayerModel{DB: testDB}
	tmm := &models.TeamMemberModel{DB: testDB}
	lam := &models.LeagueAdminModel{DB: testDB}

	playerID, err := pm.Insert(&models.Player{FirstName: "Admin", LastName: "Captain"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tmm.AddMembership(playerID, teamID); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetCaptain(teamID, sql.NullInt32{Int32: int32(playerID), Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := lam.AddAdmin(playerID, leagueID); err != nil {
		t.Fatal(err)
	}
	userID, err := um.Insert("view-as-player@test.com", "validpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if err := um.Activate(userID); err != nil {
		t.Fatal(err)
	}
	if err := um.SetPlayerID(userID, playerID); err != nil {
		t.Fatal(err)
	}
	if err := um.InsertUserRole(userID, "ADMIN"); err != nil {
		t.Fatal(err)
	}

	ts := newTestServer(t, app.routes())
	ts.login(t, "view-as-player@test.com", "validpassword123")

	t.Run("before toggling, the account sees its real admin/captain controls", func(t *testing.T) {
		_, _, body := ts.get(t, "/")
		if !strings.Contains(body, ">Admin<") {
			t.Error("expected the Admin nav dropdown to be visible")
		}
		if !strings.Contains(body, "My Leagues (Admin)") {
			t.Error("expected the My Leagues (Admin) nav dropdown to be visible")
		}
		if !strings.Contains(body, "View as Player") {
			t.Error("expected the toggle control itself to be visible")
		}
		if strings.Contains(body, "Exit Player View") {
			t.Error("expected the toggle to still read 'View as Player', not 'Exit Player View'")
		}
	})

	t.Run("toggling on hides every admin/captain/league-admin control", func(t *testing.T) {
		_, _, formBody := ts.get(t, "/")
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := ts.postForm(t, "/user/toggleViewAsPlayer", url.Values{"csrf_token": {csrfToken}})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}

		_, _, body := ts.get(t, "/")
		if strings.Contains(body, ">Admin<") {
			t.Error("expected the Admin nav dropdown to be hidden while viewing as player")
		}
		if strings.Contains(body, "My Leagues (Admin)") {
			t.Error("expected the My Leagues (Admin) nav dropdown to be hidden while viewing as player")
		}
		if !strings.Contains(body, "Viewing as a plain player") {
			t.Error("expected the persistent banner to appear")
		}
		if !strings.Contains(body, "Exit Player View") {
			t.Error("expected the toggle to now read 'Exit Player View'")
		}

		// Confirms the suppression reaches real permission checks
		// (canManageMatch), not just what the nav happens to render.
		_, _, matchBody := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if strings.Contains(matchBody, "Edit Match") {
			t.Error("expected the Edit Match link to be hidden while viewing as player")
		}
	})

	t.Run("toggling off restores every control", func(t *testing.T) {
		_, _, formBody := ts.get(t, "/")
		csrfToken := extractCSRFToken(t, formBody)

		code, _, _ := ts.postForm(t, "/user/toggleViewAsPlayer", url.Values{"csrf_token": {csrfToken}})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}

		_, _, body := ts.get(t, "/")
		if !strings.Contains(body, ">Admin<") {
			t.Error("expected the Admin nav dropdown to be visible again")
		}
		if !strings.Contains(body, "View as Player") {
			t.Error("expected the toggle to read 'View as Player' again")
		}
		if strings.Contains(body, "Exit Player View") {
			t.Error("expected 'Exit Player View' to be gone")
		}
	})

	t.Run("a non-admin can't enable it", func(t *testing.T) {
		other := newTestServer(t, app.routes())
		other.login(t, testActiveEmail, testActivePass)

		_, _, formBody := other.get(t, "/")
		csrfToken := extractCSRFToken(t, formBody)

		code, headers, _ := other.postForm(t, "/user/toggleViewAsPlayer", url.Values{"csrf_token": {csrfToken}})
		if code != http.StatusSeeOther {
			t.Fatalf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}

		_, _, body := other.get(t, "/")
		if strings.Contains(body, "View as Player") || strings.Contains(body, "Exit Player View") {
			t.Error("expected a non-admin to never see the toggle control at all")
		}
	})

	t.Run("re-logging in resets a stale toggle", func(t *testing.T) {
		reLogin := newTestServer(t, app.routes())
		reLogin.login(t, "view-as-player@test.com", "validpassword123")

		_, _, formBody := reLogin.get(t, "/")
		csrfToken := extractCSRFToken(t, formBody)
		reLogin.postForm(t, "/user/toggleViewAsPlayer", url.Values{"csrf_token": {csrfToken}})

		_, _, toggledBody := reLogin.get(t, "/")
		if strings.Contains(toggledBody, ">Admin<") {
			t.Fatal("setup: expected the toggle to have taken effect before re-login")
		}

		// Same session/cookie jar, logging in again without logging out
		// first — RenewToken rotates the session ID but keeps existing
		// data unless explicitly cleared, which is exactly what
		// userLoginPost's Remove(viewAsPlayer) guards against.
		reLogin.login(t, "view-as-player@test.com", "validpassword123")

		_, _, body := reLogin.get(t, "/")
		if !strings.Contains(body, ">Admin<") {
			t.Error("expected re-logging in to reset the stale 'view as player' toggle")
		}
	})
}

// Before a match has a recorded result, a non-manager only sees their own
// team's box on the match screen — not the opponent's roster/RSVP roll
// call, notes, etc. — so a team can't scout who's confirmed to play for
// the other side ahead of time. A captain (or any other manager) is
// unaffected and always sees both; once the match has a score, the
// restriction lifts for everyone.
func TestMatchScreenBoxVisibilityBeforeResult(t *testing.T) {
	app := newTestApplication(t)

	lm := &models.LeagueModel{DB: testDB}
	leagueID, err := lm.Insert(&models.League{Name: "Box Visibility League"})
	if err != nil {
		t.Fatal(err)
	}
	tm := &models.TeamModel{DB: testDB}
	homeTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Box Visibility Home FC"})
	if err != nil {
		t.Fatal(err)
	}
	awayTeamID, err := tm.Insert(&models.Team{LeagueID: leagueID, Name: "Box Visibility Away FC"})
	if err != nil {
		t.Fatal(err)
	}
	sm := &models.SeasonModel{DB: testDB}
	seasonID, err := sm.Insert(&models.Season{LeagueID: leagueID, Name: "Box Visibility Season"})
	if err != nil {
		t.Fatal(err)
	}
	mm := &models.MatchModel{DB: testDB}
	matchID, err := mm.Insert(&models.Match{SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	setupRosterMember(t, homeTeamID, "box-visibility-home-member@test.com", "validpassword123")
	setupTeamCaptain(t, homeTeamID, "box-visibility-home-captain@test.com", "validpassword123")

	// Each box's card heading links to /team/<id> immediately followed by
	// </h4> — specific enough not to also match the nav's own "My Teams"
	// link to the same team (which a roster-member viewer would otherwise
	// also have), unlike a bare href="/team/<id>" check.
	homeBoxMarker := fmt.Sprintf(`<a href="/team/%d">Box Visibility Home FC</a></h4>`, homeTeamID)
	awayBoxMarker := fmt.Sprintf(`<a href="/team/%d">Box Visibility Away FC</a></h4>`, awayTeamID)

	t.Run("an unrelated active user sees neither box before a result", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		_, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if strings.Contains(body, homeBoxMarker) {
			t.Error("expected the home box to be hidden before a result")
		}
		if strings.Contains(body, awayBoxMarker) {
			t.Error("expected the away box to be hidden before a result")
		}
	})

	t.Run("a home roster member sees their own box but not the away box", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "box-visibility-home-member@test.com", "validpassword123")

		_, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(body, homeBoxMarker) {
			t.Error("expected their own (home) box to be visible")
		}
		if strings.Contains(body, awayBoxMarker) {
			t.Error("expected the away box to still be hidden")
		}
	})

	t.Run("the home captain sees both boxes regardless", func(t *testing.T) {
		ts := newTestServer(t, app.routes())
		ts.login(t, "box-visibility-home-captain@test.com", "validpassword123")

		_, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(body, homeBoxMarker) || !strings.Contains(body, awayBoxMarker) {
			t.Error("expected a captain to see both boxes even before a result")
		}
	})

	t.Run("once the match has a score, the unrelated active user sees both boxes", func(t *testing.T) {
		if err := mm.Update(&models.Match{
			ID: matchID, SeasonID: seasonID, HomeTeamID: homeTeamID, AwayTeamID: awayTeamID, MatchDate: time.Now(),
			HomeScore: sql.NullInt32{Int32: 2, Valid: true}, AwayScore: sql.NullInt32{Int32: 1, Valid: true},
		}); err != nil {
			t.Fatal(err)
		}

		ts := newTestServer(t, app.routes())
		ts.login(t, testActiveEmail, testActivePass)

		_, _, body := ts.get(t, fmt.Sprintf("/match/%d", matchID))
		if !strings.Contains(body, homeBoxMarker) || !strings.Contains(body, awayBoxMarker) {
			t.Error("expected both boxes visible once the match has a recorded score")
		}
	})
}

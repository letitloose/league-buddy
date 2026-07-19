package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"testing"

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

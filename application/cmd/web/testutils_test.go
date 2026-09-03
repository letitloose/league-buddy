package main

import (
	"bytes"
	"database/sql"
	"html"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
	_ "github.com/go-sql-driver/mysql"
	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/services"
	"golang.org/x/crypto/bcrypt"
)

const (
	testDSN         = "leaguebuddy:changeme_test_pw@tcp(:3308)/league_buddy_test?parseTime=true&multiStatements=true"
	testActiveEmail = "active@test.com"
	testActivePass  = "validpassword123"
	testAdminEmail  = "admin@test.com"
	testAdminPass   = "validpassword123"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	// Tests exercise the notifications feature by default; individual
	// tests that specifically cover the "not yet launched" state
	// (TestPlayerNotificationsHiddenWithoutSMSConfigured) temporarily
	// unset this for their own duration.
	os.Setenv("SMS_FEATURE_ENABLED", "true")

	// Real bcrypt cost is deliberately slow (~250-300ms/hash); this
	// package's route tests create and log in a lot of users, which
	// dominates the runtime. Fake test passwords need none of that.
	models.BcryptCost = bcrypt.MinCost

	var err error
	testDB, err = setupTestDB()
	if err != nil {
		log.Fatal("handler test DB setup failed:", err)
	}

	code := m.Run()

	if script, err := os.ReadFile("../../sql/teardown-test.sql"); err == nil {
		testDB.Exec(string(script))
	}
	testDB.Close()

	os.Exit(code)
}

func setupTestDB() (*sql.DB, error) {
	db, err := sql.Open("mysql", testDSN)
	if err != nil {
		return nil, err
	}

	for i := 0; i < 30; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return nil, err
	}

	if script, _ := os.ReadFile("../../sql/teardown-test.sql"); script != nil {
		db.Exec(string(script))
	}

	script, err := os.ReadFile("../../sql/setup.sql")
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(string(script)); err != nil {
		return nil, err
	}

	mm := models.MigrationModel{DB: db}
	os.Setenv("MIGRATION_PATH", "../../sql/migrations/")
	if err = mm.PerformMigrations(); err != nil {
		return nil, err
	}

	um := &models.UserModel{DB: db}

	activeID, err := um.Insert(testActiveEmail, testActivePass)
	if err != nil {
		return nil, err
	}
	if err = um.Activate(activeID); err != nil {
		return nil, err
	}

	adminID, err := um.Insert(testAdminEmail, testAdminPass)
	if err != nil {
		return nil, err
	}
	if err = um.Activate(adminID); err != nil {
		return nil, err
	}
	if err = um.InsertUserRole(adminID, "ADMIN"); err != nil {
		return nil, err
	}

	return db, nil
}

func newTestApplication(t *testing.T) *application {
	t.Helper()

	templateCache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}

	sessionManager := scs.New()
	sessionManager.Store = mysqlstore.New(testDB)
	sessionManager.Lifetime = 12 * time.Hour

	players := &models.PlayerModel{DB: testDB}
	playerService := &services.PlayerService{PlayerModel: players, DB: testDB}
	users := &models.UserModel{DB: testDB}
	userService := &services.UserService{UserModel: users}
	leagues := &models.LeagueModel{DB: testDB}
	leagueService := &services.LeagueService{LeagueModel: leagues, DB: testDB}
	teams := &models.TeamModel{DB: testDB}
	teamService := &services.TeamService{TeamModel: teams, DB: testDB}
	locations := &models.LocationModel{DB: testDB}
	locationService := &services.LocationService{LocationModel: locations, DB: testDB}
	seasons := &models.SeasonModel{DB: testDB}
	seasonService := &services.SeasonService{SeasonModel: seasons, DB: testDB}
	matches := &models.MatchModel{DB: testDB}
	matchService := &services.MatchService{MatchModel: matches, DB: testDB}
	rsvps := &models.RSVPModel{DB: testDB}
	rsvpService := &services.RSVPService{RSVPModel: rsvps}
	matchTeamNotes := &models.MatchTeamNoteModel{DB: testDB}
	matchTeamNoteService := &services.MatchTeamNoteService{MatchTeamNoteModel: matchTeamNotes, DB: testDB}
	matchReminderService := &services.MatchReminderService{DB: testDB}
	invites := &models.InviteModel{DB: testDB}
	inviteService := &services.InviteService{InviteModel: invites, DB: testDB}
	joinRequests := &models.JoinRequestModel{DB: testDB}
	joinRequestService := &services.JoinRequestService{JoinRequestModel: joinRequests, DB: testDB}
	rosterExportService := &services.RosterExportService{DB: testDB}
	rosterImportService := &services.RosterImportService{DB: testDB}
	scheduleImportService := &services.ScheduleImportService{DB: testDB}
	calendarService := &services.CalendarService{DB: testDB}
	notificationPreferences := &models.NotificationPreferenceModel{DB: testDB}
	notificationPreferenceService := &services.NotificationPreferenceService{NotificationPreferenceModel: notificationPreferences, DB: testDB}

	return &application{
		errorLog:                      log.New(io.Discard, "", 0),
		infoLog:                       log.New(io.Discard, "", 0),
		playerService:                 playerService,
		userService:                   userService,
		leagueService:                 leagueService,
		teamService:                   teamService,
		locationService:               locationService,
		seasonService:                 seasonService,
		matchService:                  matchService,
		rsvpService:                   rsvpService,
		matchTeamNoteService:          matchTeamNoteService,
		matchReminderService:          matchReminderService,
		inviteService:                 inviteService,
		joinRequestService:            joinRequestService,
		rosterExportService:           rosterExportService,
		rosterImportService:           rosterImportService,
		scheduleImportService:         scheduleImportService,
		calendarService:               calendarService,
		notificationPreferenceService: notificationPreferenceService,
		templateCache:                 templateCache,
		sessionManager:                sessionManager,
		useTemplateCache:              true,
	}
}

type testServer struct {
	*httptest.Server
}

func newTestServer(t *testing.T, h http.Handler) *testServer {
	t.Helper()
	ts := httptest.NewTLSServer(h)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ts.Client().Jar = jar
	ts.Client().CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	t.Cleanup(ts.Close)
	return &testServer{ts}
}

func (ts *testServer) get(t *testing.T, urlPath string) (int, http.Header, string) {
	t.Helper()
	rs, err := ts.Client().Get(ts.URL + urlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rs.StatusCode, rs.Header, string(body)
}

var csrfTokenRX = regexp.MustCompile(`<input type='hidden' name='csrf_token' value='([^']+)'`)

func extractCSRFToken(t *testing.T, body string) string {
	t.Helper()
	matches := csrfTokenRX.FindStringSubmatch(body)
	if len(matches) < 2 {
		t.Fatal("no csrf token found in body")
	}
	// Go templates HTML-escape '+' as '&#43;' in attribute values; unescape before
	// passing to url.Values so base64 decoding on the server side works correctly.
	return html.UnescapeString(matches[1])
}

// postForm submits a same-origin form POST. nosurf (as of v1.2.0, newer than
// the version the reference project pins) checks Origin/Referer for
// same-origin *before* checking the CSRF token itself — real browsers send
// these automatically on a same-page form submit, but Go's http.Client does
// not, so the test client has to set Referer itself to avoid a spurious 400.
func (ts *testServer) postForm(t *testing.T, urlPath string, form url.Values) (int, http.Header, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, ts.URL+urlPath, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", ts.URL+urlPath)

	rs, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rs.StatusCode, rs.Header, string(body)
}

// postMultipart submits a same-origin multipart/form-data POST with a
// single file field — the shape the CSV roster import form uses. Mirrors
// postForm's Referer handling for nosurf's same-origin check.
func (ts *testServer) postMultipart(t *testing.T, urlPath, csrfToken, fileFieldName, fileName string, fileContent []byte) (int, http.Header, string) {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("csrf_token", csrfToken); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile(fileFieldName, fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(fileContent); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+urlPath, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Referer", ts.URL+urlPath)

	rs, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rs.StatusCode, rs.Header, string(body)
}

// delete makes a DELETE request, automatically fetching a CSRF token from the
// signup page (a stable public route that always renders a CSRF field).
func (ts *testServer) delete(t *testing.T, urlPath string) (int, http.Header, string) {
	t.Helper()

	_, _, body := ts.get(t, "/user/signup")
	csrfToken := extractCSRFToken(t, body)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+urlPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("Referer", ts.URL+urlPath)

	rs, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()
	respBody, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rs.StatusCode, rs.Header, string(respBody)
}

func (ts *testServer) login(t *testing.T, email, password string) {
	t.Helper()

	_, _, body := ts.get(t, "/user/login")
	csrfToken := extractCSRFToken(t, body)

	form := url.Values{
		"email":      {email},
		"password":   {password},
		"csrf_token": {csrfToken},
	}
	code, headers, body := ts.postForm(t, "/user/login", form)
	if code != http.StatusSeeOther {
		t.Fatalf("login: expected status %d, got %d, body: %s", http.StatusSeeOther, code, body)
	}
	if headers.Get("Location") != "/" {
		t.Fatalf("login: expected redirect to /, got %q", headers.Get("Location"))
	}
}

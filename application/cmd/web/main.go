package main

import (
	"database/sql"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/services"

	_ "github.com/go-sql-driver/mysql"
)

type application struct {
	errorLog           *log.Logger
	infoLog            *log.Logger
	playerService      *services.PlayerService
	userService        *services.UserService
	leagueService      *services.LeagueService
	teamService        *services.TeamService
	locationService    *services.LocationService
	seasonService      *services.SeasonService
	matchService       *services.MatchService
	rsvpService        *services.RSVPService
	inviteService      *services.InviteService
	joinRequestService *services.JoinRequestService
	emailService       *services.Email
	templateCache      map[string]*template.Template
	sessionManager     *scs.SessionManager
	useTemplateCache   bool
}

func main() {

	dsn := flag.String("dsn", "leaguebuddy:changeme@/league_buddy?parseTime=true&multiStatements=true", "MySQL data source name")
	useTemplateCache := flag.Bool("useTemplateCache", true, "When false, templates will render on each request.")

	flag.Parse()

	infoLog, errorLog, err := setupLogs()
	if err != nil {
		errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)
		errorLog.Fatal(err)
	}

	dsn = checkForEnvDSN(dsn)
	db, err := openDB(*dsn)
	if err != nil {
		errorLog.Fatal(err)
	}
	defer db.Close()

	var templateCache = map[string]*template.Template{}
	infoLog.Println("Using Template Cache:", *useTemplateCache)
	if *useTemplateCache {
		templateCache, err = newTemplateCache()
		if err != nil {
			errorLog.Fatal(err)
		}
	}

	// Only construct the email service when creds are actually configured, so
	// that dev/CI environments without Mailjet creds skip sending instead of
	// hitting a nil-pointer panic deep in SendEmailV2.
	var email *services.Email
	if os.Getenv("EMAIL_USER") != "" {
		email = &services.Email{
			Username: os.Getenv("EMAIL_USER"),
			Password: os.Getenv("EMAIL_PASSWORD"),
			Sender:   os.Getenv("EMAIL_SENDER"),
		}
	}

	players := &models.PlayerModel{DB: db}
	playerService := &services.PlayerService{PlayerModel: players, DB: db}
	users := &models.UserModel{DB: db}
	userService := &services.UserService{UserModel: users, Email: email, InfoLog: infoLog}
	leagues := &models.LeagueModel{DB: db}
	leagueService := &services.LeagueService{LeagueModel: leagues, DB: db}
	teams := &models.TeamModel{DB: db}
	teamService := &services.TeamService{TeamModel: teams, DB: db}
	locations := &models.LocationModel{DB: db}
	locationService := &services.LocationService{LocationModel: locations, DB: db}
	seasons := &models.SeasonModel{DB: db}
	seasonService := &services.SeasonService{SeasonModel: seasons, DB: db}
	matches := &models.MatchModel{DB: db}
	matchService := &services.MatchService{MatchModel: matches, DB: db}
	rsvps := &models.RSVPModel{DB: db}
	rsvpService := &services.RSVPService{RSVPModel: rsvps}
	invites := &models.InviteModel{DB: db}
	inviteService := &services.InviteService{InviteModel: invites, DB: db, Email: email, InfoLog: infoLog}
	joinRequests := &models.JoinRequestModel{DB: db}
	joinRequestService := &services.JoinRequestService{JoinRequestModel: joinRequests, DB: db, Email: email, InfoLog: infoLog}

	sessionManager := scs.New()
	sessionManager.Store = mysqlstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour

	app := &application{
		errorLog:           errorLog,
		infoLog:            infoLog,
		playerService:      playerService,
		userService:        userService,
		leagueService:      leagueService,
		teamService:        teamService,
		locationService:    locationService,
		seasonService:      seasonService,
		matchService:       matchService,
		rsvpService:        rsvpService,
		inviteService:      inviteService,
		joinRequestService: joinRequestService,
		emailService:       email,
		templateCache:      templateCache,
		sessionManager:     sessionManager,
		useTemplateCache:   *useTemplateCache,
	}

	reset := os.Getenv("RESETDB")
	if reset == "true" {
		err = app.reset()
		if err != nil {
			errorLog.Fatal(err)
		}
	} else {
		mm := models.MigrationModel{DB: db}
		err = mm.PerformMigrations()
		if err != nil {
			errorLog.Println(err)
		}
	}

	siteHost := os.Getenv("SITE_HOST")
	sitePort := os.Getenv("SITE_PORT")
	srv := &http.Server{
		Addr:         ":" + sitePort,
		ErrorLog:     app.errorLog,
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	app.infoLog.Printf("Starting server on %s:%s (codename: prancy-bouncing-snail)", siteHost, sitePort)

	err = srv.ListenAndServe()
	app.errorLog.Fatal(err)
}

func setupLogs() (*log.Logger, *log.Logger, error) {
	infoWriter := os.Stdout
	errWriter := os.Stderr

	infoLogFile := os.Getenv("INFO_LOG")
	if len(infoLogFile) > 0 {
		infoFile, err := os.OpenFile(infoLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, err
		}
		infoWriter = infoFile
	}
	infoLog := log.New(infoWriter, "INFO\t", log.Ldate|log.Ltime)

	errorLogFile := os.Getenv("ERROR_LOG")
	if len(errorLogFile) > 0 {
		errorFile, err := os.OpenFile(errorLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, err
		}
		errWriter = errorFile
	}
	errorLog := log.New(errWriter, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	return infoLog, errorLog, nil
}

func checkForEnvDSN(dsn *string) *string {

	host := os.Getenv("DBHOST")

	if host == "" {
		return dsn
	}

	port := os.Getenv("DBPORT")
	dbName := os.Getenv("MYSQL_DATABASE")
	username := os.Getenv("MYSQL_USER")
	password := os.Getenv("MYSQL_PASSWORD")

	newDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true",
		username, password, host, port, dbName)
	return &newDSN
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 60; i++ {
		if err := db.Ping(); err == nil {
			fmt.Println("We are connected!")
			break
		}
		time.Sleep(time.Second)
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// reset tears down and rebuilds the schema from scratch, then creates a
// default admin account from LEAGUEBUDDYUSER/LEAGUEBUDDYPASSWORD. Gated
// behind RESETDB=true — never run this against a database with real data.
func (app *application) reset() error {

	app.infoLog.Println("reseting DB")
	script, err := os.ReadFile("./sql/teardown.sql")
	if err != nil {
		app.errorLog.Fatal(err)
	}
	_, err = app.playerService.DB.Exec(string(script))
	if err != nil {
		app.errorLog.Println(err)
	}

	script, err = os.ReadFile("./sql/setup.sql")
	if err != nil {
		app.errorLog.Fatal(err)
	}
	_, err = app.playerService.DB.Exec(string(script))
	if err != nil {
		app.errorLog.Println(err)
	}

	mm := models.MigrationModel{DB: app.playerService.DB}
	err = mm.PerformMigrations()
	if err != nil {
		app.errorLog.Fatal(err)
	}

	leagueBuddyUser := os.Getenv("LEAGUEBUDDYUSER")
	leagueBuddyPassword := os.Getenv("LEAGUEBUDDYPASSWORD")

	if err := app.seedAdminProfile(leagueBuddyUser); err != nil {
		app.errorLog.Println(err)
	}

	uf := &services.UserForm{Email: leagueBuddyUser, Password: leagueBuddyPassword, ConfirmPassword: leagueBuddyPassword}
	err = app.userService.InsertUser(uf)
	if err != nil {
		app.errorLog.Println(err)
	}

	vhash, err := app.userService.GetVerificationHashByEmail(leagueBuddyUser)
	if err != nil {
		app.errorLog.Println(err)
	}

	err = app.userService.ActivateUser(vhash)
	if err != nil {
		app.errorLog.Println(err)
	}

	userID, err := app.userService.Authenticate(leagueBuddyUser, leagueBuddyPassword)
	if err != nil {
		app.errorLog.Println(err)
	}

	err = app.userService.InsertUserRole(userID, "ADMIN")
	if err != nil {
		app.errorLog.Println(err)
	}

	if err := app.seedTestUsers(); err != nil {
		app.errorLog.Println(err)
	}

	// Runs after seedTestUsers so Lou's captaincy of team 1 wins over the
	// synthetic "Team Captain" test login seeded just above.
	if err := app.seedColonialRoster(leagueBuddyUser); err != nil {
		app.errorLog.Println(err)
	}

	if err := app.seedHistoricalSeasons(); err != nil {
		app.errorLog.Println(err)
	}

	return nil
}

// seedTestUsers creates one test login per non-system-admin role (league
// admin, team captain, plain roster player) against the seeded "CapReg over
// 30" league (id 1) / "Colonial FC" team (id 1), so all four tiers can be
// logged into directly after a reset. Names are role-reflective so they're
// recognizable in the UI.
func (app *application) seedTestUsers() error {
	if err := app.seedRoleUser("League", "Admin", os.Getenv("LEAGUEBUDDY_LEAGUEADMIN_EMAIL"), os.Getenv("LEAGUEBUDDY_LEAGUEADMIN_PASSWORD"),
		func(playerID int) error {
			return (&models.LeagueAdminModel{DB: app.playerService.DB}).AddAdmin(playerID, 1)
		}); err != nil {
		return err
	}

	if err := app.seedRoleUser("Team", "Captain", os.Getenv("LEAGUEBUDDY_CAPTAIN_EMAIL"), os.Getenv("LEAGUEBUDDY_CAPTAIN_PASSWORD"),
		func(playerID int) error {
			tmm := &models.TeamMemberModel{DB: app.playerService.DB}
			if err := tmm.AddMembership(playerID, 1); err != nil {
				return err
			}
			return (&models.TeamModel{DB: app.playerService.DB}).SetCaptain(1, sql.NullInt32{Int32: int32(playerID), Valid: true})
		}); err != nil {
		return err
	}

	return app.seedRoleUser("Roster", "Player", os.Getenv("LEAGUEBUDDY_PLAYER_EMAIL"), os.Getenv("LEAGUEBUDDY_PLAYER_PASSWORD"),
		func(playerID int) error {
			return (&models.TeamMemberModel{DB: app.playerService.DB}).AddMembership(playerID, 1)
		})
}

// seedRoleUser creates and activates a test login for one role, then calls
// attach with the resulting playerID for the role-specific bit (league
// admin, captain, roster membership). No-ops if email is unset, the same
// optional/backward-compatible guard seedAdminProfile uses for
// LEAGUEBUDDYFIRSTNAME. Follows the same pre-create-player-then-activate
// pattern seedAdminProfile already relies on: activation's linkOrCreatePlayer
// finds the pre-seeded player by email and links it automatically.
func (app *application) seedRoleUser(firstName, lastName, email, password string, attach func(playerID int) error) error {
	if email == "" {
		return nil
	}

	pm := &models.PlayerModel{DB: app.playerService.DB}
	playerID, err := pm.Insert(&models.Player{
		FirstName: firstName,
		LastName:  lastName,
		Email:     sql.NullString{String: email, Valid: true},
	})
	if err != nil {
		return err
	}

	uf := &services.UserForm{Email: email, Password: password, ConfirmPassword: password}
	if err := app.userService.InsertUser(uf); err != nil {
		return err
	}

	vhash, err := app.userService.GetVerificationHashByEmail(email)
	if err != nil {
		return err
	}

	if err := app.userService.ActivateUser(vhash); err != nil {
		return err
	}

	return attach(playerID)
}

// seedAdminProfile pre-creates a real player record (name, DOB, phone,
// address) for the bootstrap admin account from LEAGUEBUDDY* env vars, so
// that when the admin user is activated below, UserService.linkOrCreatePlayer
// finds this row via its "existing unlinked player with a matching email"
// branch and links to it — instead of falling back to a blank placeholder
// player. A no-op if LEAGUEBUDDYFIRSTNAME isn't set, so this stays optional
// and backward-compatible with environments that don't set it.
func (app *application) seedAdminProfile(email string) error {
	firstName := os.Getenv("LEAGUEBUDDYFIRSTNAME")
	if firstName == "" {
		return nil
	}

	am := &models.AddressModel{DB: app.playerService.DB}
	addressID, err := am.Insert(&models.Address{
		Address1:      sql.NullString{String: os.Getenv("LEAGUEBUDDYADDRESS1"), Valid: true},
		City:          sql.NullString{String: os.Getenv("LEAGUEBUDDYCITY"), Valid: true},
		StateProvince: sql.NullString{String: os.Getenv("LEAGUEBUDDYSTATE"), Valid: true},
		ZipCode:       sql.NullString{String: os.Getenv("LEAGUEBUDDYZIP"), Valid: true},
	})
	if err != nil {
		return err
	}

	player := &models.Player{
		FirstName:   firstName,
		LastName:    os.Getenv("LEAGUEBUDDYLASTNAME"),
		Email:       sql.NullString{String: email, Valid: true},
		PhoneNumber: sql.NullString{String: os.Getenv("LEAGUEBUDDYPHONE"), Valid: true},
		AddressID:   sql.NullInt32{Int32: int32(addressID), Valid: true},
	}
	if dob, err := time.Parse("2006-01-02", os.Getenv("LEAGUEBUDDYDOB")); err == nil {
		player.DateOfBirth = sql.NullTime{Time: dob, Valid: true}
	}

	pm := &models.PlayerModel{DB: app.playerService.DB}
	_, err = pm.Insert(player)
	return err
}

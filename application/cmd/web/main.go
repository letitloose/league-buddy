package main

import (
	"database/sql"
	"errors"
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
	errorLog         *log.Logger
	infoLog          *log.Logger
	playerService    *services.PlayerService
	userService      *services.UserService
	emailService     *services.Email
	templateCache    map[string]*template.Template
	sessionManager   *scs.SessionManager
	useTemplateCache bool
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
	userService := &services.UserService{UserModel: users, Email: email}

	sessionManager := scs.New()
	sessionManager.Store = mysqlstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour

	app := &application{
		errorLog:         errorLog,
		infoLog:          infoLog,
		playerService:    playerService,
		userService:      userService,
		emailService:     email,
		templateCache:    templateCache,
		sessionManager:   sessionManager,
		useTemplateCache: *useTemplateCache,
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

	if err := app.ensureDefaultTeam(); err != nil {
		errorLog.Println(err)
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

	return nil
}

// ensureDefaultTeam is an idempotent, defensive check that runs on every
// boot (not just RESETDB=true): if the teams table is empty, seed one row
// from TEAM_NAME so a fresh production database self-heals without needing
// the destructive reset path.
func (app *application) ensureDefaultTeam() error {
	tm := &models.TeamModel{DB: app.playerService.DB}

	_, err := tm.GetDefault()
	if err == nil {
		return nil
	}
	if !errors.Is(err, models.ErrNoRecord) {
		return err
	}

	teamName := os.Getenv("TEAM_NAME")
	if teamName == "" {
		teamName = "My Team"
	}

	_, err = tm.Insert(teamName)
	return err
}

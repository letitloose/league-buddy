package models

import (
	"database/sql"
	"os"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const testDSN = "leaguebuddy:changeme_test_pw@tcp(:3308)/league_buddy_test?parseTime=true&multiStatements=true"

var (
	sharedTestDB *sql.DB
	schemaOnce   sync.Once
	testUserHash []byte
	testVerHash  []byte
	testUserOnce sync.Once
)

// NewTestDB returns a shared DB with the schema set up once per process.
// Each call truncates user-data tables and inserts a fresh test user, so
// tests get a clean state without paying the DDL cost every time.
func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()

	schemaOnce.Do(func() {
		// Real bcrypt cost is deliberately slow (~250-300ms/hash); every
		// test that creates or logs in a user pays that, which dominates
		// this suite's runtime. Fake test passwords need none of that.
		BcryptCost = bcrypt.MinCost

		db, err := sql.Open("mysql", testDSN)
		if err != nil {
			panic("NewTestDB open: " + err.Error())
		}

		script, err := os.ReadFile("../../sql/teardown-test.sql")
		if err != nil {
			panic("NewTestDB teardown: " + err.Error())
		}
		if _, err = db.Exec(string(script)); err != nil {
			panic("NewTestDB teardown exec: " + err.Error())
		}

		script, err = os.ReadFile("../../sql/setup.sql")
		if err != nil {
			panic("NewTestDB setup: " + err.Error())
		}
		if _, err = db.Exec(string(script)); err != nil {
			panic("NewTestDB setup exec: " + err.Error())
		}

		os.Setenv("MIGRATION_PATH", "../../sql/migrations/")
		mm := MigrationModel{DB: db}
		if err = mm.PerformMigrations(); err != nil {
			panic("NewTestDB migrations: " + err.Error())
		}

		sharedTestDB = db
	})

	resetTestData(t, sharedTestDB)
	return sharedTestDB
}

// resetTestData truncates user-data tables and inserts a single active test user.
func resetTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	script, err := os.ReadFile("../../sql/reset-test.sql")
	if err != nil {
		t.Fatal("reset-test.sql:", err)
	}
	if _, err = db.Exec(string(script)); err != nil {
		t.Fatal("reset exec:", err)
	}

	// Compute bcrypt hashes once across all tests in this process.
	testUserOnce.Do(func() {
		testUserHash, _ = bcrypt.GenerateFromPassword([]byte("testpassword"), bcrypt.MinCost)
		testVerHash, _ = bcrypt.GenerateFromPassword([]byte("player@example.com"+"testpassword"), bcrypt.MinCost)
	})

	// Explicit id=1 — several tests hardcode userID 1 for this seed user,
	// an invariant TRUNCATE's auto-increment reset used to provide for
	// free (see reset-test.sql's DELETE-vs-TRUNCATE comment for why that
	// reset is gone) but an explicit id restores just as well without it.
	_, err = db.Exec(
		`INSERT INTO users (id, email, hashed_password, created, active, verification_hash)
		 VALUES (1, ?, ?, UTC_TIMESTAMP(), 1, ?)`,
		"player@example.com",
		string(testUserHash),
		string(testVerHash),
	)
	if err != nil {
		t.Fatal("insert test user:", err)
	}
}

package models

import (
	"os"
	"testing"
	"time"
)

func TestInsertMigration(t *testing.T) {
	db := NewTestDB(t)

	mm := MigrationModel{DB: db}

	err := mm.insert("filename.sql", time.Now())
	if err != nil {
		t.Fatal(err)
	}
}

func TestPerformMigrations(t *testing.T) {
	db := NewTestDB(t)

	mm := MigrationModel{DB: db}

	os.Setenv("MIGRATION_PATH", "../../sql/migrations/")
	if err := mm.PerformMigrations(); err != nil {
		t.Fatal(err)
	}

	// sql/migrations/ starts empty in a fresh scaffold; running twice
	// with nothing to apply should still be a no-op, not an error.
	if err := mm.PerformMigrations(); err != nil {
		t.Fatal(err)
	}
}

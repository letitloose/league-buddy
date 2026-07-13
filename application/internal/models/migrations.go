package models

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"
)

type Migration struct {
	filename string
	rundate  time.Time
}

type MigrationModel struct {
	DB *sql.DB
}

func (mm *MigrationModel) insert(filename string, rundate time.Time) error {
	statement := `INSERT INTO migration (filename, rundate)
    VALUES(?, ?)`

	_, err := mm.DB.Exec(statement, filename, rundate)
	if err != nil {
		return err
	}

	return nil
}

func (mm *MigrationModel) getByFilename(filename string) *Migration {
	statement := `SELECT filename, rundate from migration where filename = ?`

	row := mm.DB.QueryRow(statement, filename)

	migration := &Migration{}
	row.Scan(&migration.filename, &migration.rundate)

	return migration
}

func (mm *MigrationModel) list() ([]*Migration, error) {

	stmt := `select filename, rundate from migration;`

	rows, err := mm.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	migrations := []*Migration{}

	for rows.Next() {
		migration := &Migration{}
		err := rows.Scan(&migration.filename, &migration.rundate)
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, migration)
	}

	return migrations, nil
}

func (mm *MigrationModel) performMigration(filename string) error {
	migrationsPath := os.Getenv("MIGRATION_PATH")
	script, err := os.ReadFile(migrationsPath + filename)
	if err != nil {
		return err
	}
	_, err = mm.DB.Exec(string(script))
	if err != nil {
		return err
	}

	migration := Migration{
		filename: filename,
		rundate:  time.Now(),
	}

	err = mm.insert(migration.filename, migration.rundate)

	return err
}

func (mm *MigrationModel) PerformMigrations() error {
	migrationsPath := os.Getenv("MIGRATION_PATH")

	if len(migrationsPath) == 0 {
		return nil
	}

	migrationFiles, err := os.ReadDir(migrationsPath)
	if err != nil {
		return err
	}

	for _, file := range migrationFiles {
		if filepath.Ext(file.Name()) != ".sql" {
			// Skips .gitkeep and similar placeholder files that keep an
			// otherwise-empty migrations directory tracked in git.
			continue
		}
		existingMigration := mm.getByFilename(file.Name())
		if existingMigration.filename == file.Name() {
			continue
		}
		err := mm.performMigration(file.Name())
		if err != nil {
			return err
		}
	}

	return nil
}

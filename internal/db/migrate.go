package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations_v2/*.sql
var migrationFiles embed.FS

func ApplyMigrations(_ context.Context, db *sql.DB, driver string) error {
	driver = normalizeDriver(driver)

	sourceDriver, err := iofs.New(migrationFiles, "migrations_v2")
	if err != nil {
		return err
	}
	defer sourceDriver.Close()

	var dbDriver database.Driver
	switch driver {
	case DriverSQLite:
		dbDriver, err = sqlite3.WithInstance(db, &sqlite3.Config{MigrationsTable: "schema_migrations"})
	case DriverPostgres:
		dbDriver, err = postgres.WithInstance(db, &postgres.Config{MigrationsTable: "schema_migrations"})
	default:
		return fmt.Errorf("unsupported DB_DRIVER %q", driver)
	}
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, driver, dbDriver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

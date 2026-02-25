package db

import (
	"context"
	"database/sql"
	"fmt"

	postgresqlsqlc "github.com/ryotarai/hayai/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/hayai/internal/db/sqlc/sqlite"
)

type Store struct {
	db            *sql.DB
	driver        string
	sqliteQueries *sqlitesqlc.Queries
	pgQueries     *postgresqlsqlc.Queries
}

func NewStore(db *sql.DB, driver string) *Store {
	driver = normalizeDriver(driver)

	store := &Store{
		db:     db,
		driver: driver,
	}

	switch driver {
	case DriverSQLite:
		store.sqliteQueries = sqlitesqlc.New(db)
	case DriverPostgres:
		store.pgQueries = postgresqlsqlc.New(db)
	}

	return store
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}

	switch s.driver {
	case DriverSQLite:
		_, err := s.sqliteQueries.GetMeta(ctx, "schema_version")
		return err
	case DriverPostgres:
		_, err := s.pgQueries.GetMeta(ctx, "schema_version")
		return err
	default:
		return unsupportedDriverError(s.driver)
	}
}

func unsupportedDriverError(driver string) error {
	return fmt.Errorf("unsupported driver %q", driver)
}

package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
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

// LoadWorkflowState loads persisted workflow state by ID.
// Returns nil, nil if no state exists.
func (s *Store) LoadWorkflowState(ctx context.Context, id string) ([]byte, error) {
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM workflow_states WHERE id = ?", id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(data), nil
}

// SaveWorkflowState upserts workflow state.
func (s *Store) SaveWorkflowState(ctx context.Context, id string, data []byte) error {
	nowUnix := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO workflow_states (id, data, updated_at) VALUES (?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
			id, string(data), nowUnix)
		return err
	case DriverPostgres:
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO workflow_states (id, data, updated_at) VALUES ($1, $2, $3)
			 ON CONFLICT(id) DO UPDATE SET data = EXCLUDED.data, updated_at = EXCLUDED.updated_at`,
			id, string(data), nowUnix)
		return err
	default:
		return unsupportedDriverError(s.driver)
	}
}

// DeleteWorkflowState removes workflow state by ID.
func (s *Store) DeleteWorkflowState(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM workflow_states WHERE id = ?", id)
	return err
}

func unsupportedDriverError(driver string) error {
	return fmt.Errorf("unsupported driver %q", driver)
}

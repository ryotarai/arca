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
		store.sqliteQueries = sqlitesqlc.New(&noCancelDB{db: db})
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

// beginTx starts a transaction. For SQLite, the context's cancellation
// signal is stripped so that sqlite3_interrupt is never called on the
// shared connection (see nocanceldb.go).
func (s *Store) beginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if s.driver == DriverSQLite {
		ctx = context.WithoutCancel(ctx)
	}
	return s.db.BeginTx(ctx, opts)
}

// LoadWorkflowState loads persisted workflow state by ID.
// Returns nil, nil if no state exists.
func (s *Store) LoadWorkflowState(ctx context.Context, id string) ([]byte, error) {
	switch s.driver {
	case DriverSQLite:
		data, err := s.sqliteQueries.LoadWorkflowState(ctx, id)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return []byte(data), err
	case DriverPostgres:
		data, err := s.pgQueries.LoadWorkflowState(ctx, id)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return []byte(data), err
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

// SaveWorkflowState upserts workflow state.
func (s *Store) SaveWorkflowState(ctx context.Context, id string, data []byte) error {
	nowUnix := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertWorkflowState(ctx, sqlitesqlc.UpsertWorkflowStateParams{
			ID: id, Data: string(data), UpdatedAt: nowUnix,
		})
	case DriverPostgres:
		return s.pgQueries.UpsertWorkflowState(ctx, postgresqlsqlc.UpsertWorkflowStateParams{
			ID: id, Data: string(data), UpdatedAt: nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

// DeleteWorkflowState removes workflow state by ID.
func (s *Store) DeleteWorkflowState(ctx context.Context, id string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.DeleteWorkflowState(ctx, id)
	case DriverPostgres:
		return s.pgQueries.DeleteWorkflowState(ctx, id)
	default:
		return unsupportedDriverError(s.driver)
	}
}

func unsupportedDriverError(driver string) error {
	return fmt.Errorf("unsupported driver %q", driver)
}

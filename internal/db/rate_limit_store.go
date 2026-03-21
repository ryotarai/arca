package db

import (
	"context"
	"fmt"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

func (s *Store) InsertRateLimitEntry(ctx context.Context, key string, timestampUnix int64) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.InsertRateLimitEntry(ctx, sqlitesqlc.InsertRateLimitEntryParams{
			Key:           key,
			TimestampUnix: timestampUnix,
		})
	case DriverPostgres:
		return s.pgQueries.InsertRateLimitEntry(ctx, postgresqlsqlc.InsertRateLimitEntryParams{
			Key:           key,
			TimestampUnix: timestampUnix,
		})
	default:
		return fmt.Errorf("unsupported driver: %s", s.driver)
	}
}

func (s *Store) CountRateLimitEntries(ctx context.Context, key string, windowStart int64) (int64, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CountRateLimitEntries(ctx, sqlitesqlc.CountRateLimitEntriesParams{
			Key:         key,
			WindowStart: windowStart,
		})
	case DriverPostgres:
		return s.pgQueries.CountRateLimitEntries(ctx, postgresqlsqlc.CountRateLimitEntriesParams{
			Key:         key,
			WindowStart: windowStart,
		})
	default:
		return 0, fmt.Errorf("unsupported driver: %s", s.driver)
	}
}

func (s *Store) CleanupRateLimitEntries(ctx context.Context, cutoff int64) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CleanupRateLimitEntries(ctx, cutoff)
	case DriverPostgres:
		return s.pgQueries.CleanupRateLimitEntries(ctx, cutoff)
	default:
		return fmt.Errorf("unsupported driver: %s", s.driver)
	}
}

package db

import (
	"context"
	"database/sql"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

// JobHealthStats contains counts of recent job statuses.
type JobHealthStats struct {
	Succeeded int64
	Failed    int64
	Stuck     int64
}

// CountRecentJobsByStatus returns counts of succeeded, failed, and stuck jobs.
// since is the unix timestamp threshold for succeeded/failed jobs.
// now is the current unix timestamp used to detect stuck jobs (running past lease_until).
func (s *Store) CountRecentJobsByStatus(ctx context.Context, since, now int64) (JobHealthStats, error) {
	nowNull := sql.NullInt64{Int64: now, Valid: true}

	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.CountRecentJobsByStatus(ctx, sqlitesqlc.CountRecentJobsByStatusParams{
			Since: since,
			Now:   nowNull,
		})
		if err != nil {
			return JobHealthStats{}, err
		}
		return JobHealthStats{
			Succeeded: toInt64(row.Succeeded),
			Failed:    toInt64(row.Failed),
			Stuck:     toInt64(row.Stuck),
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.CountRecentJobsByStatus(ctx, postgresqlsqlc.CountRecentJobsByStatusParams{
			Since: since,
			Now:   nowNull,
		})
		if err != nil {
			return JobHealthStats{}, err
		}
		return JobHealthStats{
			Succeeded: toInt64(row.Succeeded),
			Failed:    toInt64(row.Failed),
			Stuck:     toInt64(row.Stuck),
		}, nil
	default:
		return JobHealthStats{}, unsupportedDriverError(s.driver)
	}
}

// toInt64 converts an interface{} value from sqlc to int64.
func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int32:
		return int64(n)
	default:
		return 0
	}
}

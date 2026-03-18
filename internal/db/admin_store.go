package db

import (
	"context"
	"database/sql"
	"time"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

type AuditLogEntry struct {
	ID             string
	ActorUserID    string
	ActingAsUserID string
	Action         string
	ResourceType   string
	ResourceID     string
	DetailsJSON    string
	CreatedAt      time.Time
	ActorEmail     string
	ActingAsEmail  string
}

type SessionImpersonation struct {
	ImpersonatedUserID   string
	ImpersonatedByUserID string
}

func (s *Store) SetSessionImpersonation(ctx context.Context, tokenHash, impersonatedUserID, impersonatedByUserID string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.SetSessionImpersonation(ctx, sqlitesqlc.SetSessionImpersonationParams{
			ImpersonatedUserID:   sql.NullString{String: impersonatedUserID, Valid: true},
			ImpersonatedByUserID: sql.NullString{String: impersonatedByUserID, Valid: true},
			TokenHash:            tokenHash,
		})
		if err != nil {
			return false, err
		}
		return n > 0, nil
	case DriverPostgres:
		n, err := s.pgQueries.SetSessionImpersonation(ctx, postgresqlsqlc.SetSessionImpersonationParams{
			ImpersonatedUserID:   sql.NullString{String: impersonatedUserID, Valid: true},
			ImpersonatedByUserID: sql.NullString{String: impersonatedByUserID, Valid: true},
			TokenHash:            tokenHash,
		})
		if err != nil {
			return false, err
		}
		return n > 0, nil
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ClearSessionImpersonation(ctx context.Context, tokenHash string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.ClearSessionImpersonation(ctx, tokenHash)
		if err != nil {
			return false, err
		}
		return n > 0, nil
	case DriverPostgres:
		n, err := s.pgQueries.ClearSessionImpersonation(ctx, tokenHash)
		if err != nil {
			return false, err
		}
		return n > 0, nil
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetSessionImpersonation(ctx context.Context, tokenHash string, nowUnix int64) (SessionImpersonation, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetSessionImpersonation(ctx, sqlitesqlc.GetSessionImpersonationParams{
			TokenHash: tokenHash,
			NowUnix:   nowUnix,
		})
		if err != nil {
			return SessionImpersonation{}, err
		}
		return SessionImpersonation{
			ImpersonatedUserID:   row.ImpersonatedUserID.String,
			ImpersonatedByUserID: row.ImpersonatedByUserID.String,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetSessionImpersonation(ctx, postgresqlsqlc.GetSessionImpersonationParams{
			TokenHash: tokenHash,
			NowUnix:   nowUnix,
		})
		if err != nil {
			return SessionImpersonation{}, err
		}
		return SessionImpersonation{
			ImpersonatedUserID:   row.ImpersonatedUserID.String,
			ImpersonatedByUserID: row.ImpersonatedByUserID.String,
		}, nil
	default:
		return SessionImpersonation{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateAuditLog(ctx context.Context, entry AuditLogEntry) error {
	actingAs := sql.NullString{String: entry.ActingAsUserID, Valid: entry.ActingAsUserID != ""}
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CreateAuditLog(ctx, sqlitesqlc.CreateAuditLogParams{
			ID:             entry.ID,
			ActorUserID:    entry.ActorUserID,
			ActingAsUserID: actingAs,
			Action:         entry.Action,
			ResourceType:   entry.ResourceType,
			ResourceID:     entry.ResourceID,
			DetailsJson:    entry.DetailsJSON,
			CreatedAt:      entry.CreatedAt,
		})
	case DriverPostgres:
		return s.pgQueries.CreateAuditLog(ctx, postgresqlsqlc.CreateAuditLogParams{
			ID:             entry.ID,
			ActorUserID:    entry.ActorUserID,
			ActingAsUserID: actingAs,
			Action:         entry.Action,
			ResourceType:   entry.ResourceType,
			ResourceID:     entry.ResourceID,
			DetailsJson:    entry.DetailsJSON,
			CreatedAt:      entry.CreatedAt,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListAuditLogs(ctx context.Context, limit int64) ([]AuditLogEntry, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListAuditLogs(ctx, limit)
		if err != nil {
			return nil, err
		}
		entries := make([]AuditLogEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, AuditLogEntry{
				ID:             row.ID,
				ActorUserID:    row.ActorUserID,
				ActingAsUserID: row.ActingAsUserID.String,
				Action:         row.Action,
				ResourceType:   row.ResourceType,
				ResourceID:     row.ResourceID,
				DetailsJSON:    row.DetailsJson,
				CreatedAt:      row.CreatedAt,
				ActorEmail:     row.ActorEmail,
				ActingAsEmail:  row.ActingAsEmail.String,
			})
		}
		return entries, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListAuditLogs(ctx, int32(limit))
		if err != nil {
			return nil, err
		}
		entries := make([]AuditLogEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, AuditLogEntry{
				ID:             row.ID,
				ActorUserID:    row.ActorUserID,
				ActingAsUserID: row.ActingAsUserID.String,
				Action:         row.Action,
				ResourceType:   row.ResourceType,
				ResourceID:     row.ResourceID,
				DetailsJSON:    row.DetailsJson,
				CreatedAt:      row.CreatedAt,
				ActorEmail:     row.ActorEmail,
				ActingAsEmail:  row.ActingAsEmail.String,
			})
		}
		return entries, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

type AuditLogFilter struct {
	ActionPrefix string
	ActorEmail   string
	Limit        int64
	Offset       int64
}

func (s *Store) ListAuditLogsFiltered(ctx context.Context, filter AuditLogFilter) ([]AuditLogEntry, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListAuditLogsFiltered(ctx, sqlitesqlc.ListAuditLogsFilteredParams{
			ActionPrefix: filter.ActionPrefix,
			ActorEmail:   filter.ActorEmail,
			LimitCount:   filter.Limit,
			OffsetCount:  filter.Offset,
		})
		if err != nil {
			return nil, err
		}
		entries := make([]AuditLogEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, AuditLogEntry{
				ID:             row.ID,
				ActorUserID:    row.ActorUserID,
				ActingAsUserID: row.ActingAsUserID.String,
				Action:         row.Action,
				ResourceType:   row.ResourceType,
				ResourceID:     row.ResourceID,
				DetailsJSON:    row.DetailsJson,
				CreatedAt:      row.CreatedAt,
				ActorEmail:     row.ActorEmail,
				ActingAsEmail:  row.ActingAsEmail.String,
			})
		}
		return entries, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListAuditLogsFiltered(ctx, postgresqlsqlc.ListAuditLogsFilteredParams{
			ActionPrefix: filter.ActionPrefix,
			ActorEmail:   filter.ActorEmail,
			LimitCount:   int32(filter.Limit),
			OffsetCount:  int32(filter.Offset),
		})
		if err != nil {
			return nil, err
		}
		entries := make([]AuditLogEntry, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, AuditLogEntry{
				ID:             row.ID,
				ActorUserID:    row.ActorUserID,
				ActingAsUserID: row.ActingAsUserID.String,
				Action:         row.Action,
				ResourceType:   row.ResourceType,
				ResourceID:     row.ResourceID,
				DetailsJSON:    row.DetailsJson,
				CreatedAt:      row.CreatedAt,
				ActorEmail:     row.ActorEmail,
				ActingAsEmail:  row.ActingAsEmail.String,
			})
		}
		return entries, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CountAuditLogsFiltered(ctx context.Context, actionPrefix, actorEmail string) (int64, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CountAuditLogsFiltered(ctx, sqlitesqlc.CountAuditLogsFilteredParams{
			ActionPrefix: actionPrefix,
			ActorEmail:   actorEmail,
		})
	case DriverPostgres:
		return s.pgQueries.CountAuditLogsFiltered(ctx, postgresqlsqlc.CountAuditLogsFilteredParams{
			ActionPrefix: actionPrefix,
			ActorEmail:   actorEmail,
		})
	default:
		return 0, unsupportedDriverError(s.driver)
	}
}

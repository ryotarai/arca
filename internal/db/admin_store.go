package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func (s *Store) SetAdminViewMode(ctx context.Context, userID, mode string, nowUnix int64) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertAdminViewMode(ctx, sqlitesqlc.UpsertAdminViewModeParams{
			UserID:    userID,
			Mode:      mode,
			UpdatedAt: nowUnix,
		})
	case DriverPostgres:
		return s.pgQueries.UpsertAdminViewMode(ctx, postgresqlsqlc.UpsertAdminViewModeParams{
			UserID:    userID,
			Mode:      mode,
			UpdatedAt: nowUnix,
		})
	default:
		return fmt.Errorf("unsupported driver: %s", s.driver)
	}
}

func (s *Store) GetAdminViewMode(ctx context.Context, userID string) (string, error) {
	switch s.driver {
	case DriverSQLite:
		mode, err := s.sqliteQueries.GetAdminViewMode(ctx, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "admin", nil
			}
			return "", err
		}
		return mode, nil
	case DriverPostgres:
		mode, err := s.pgQueries.GetAdminViewMode(ctx, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "admin", nil
			}
			return "", err
		}
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported driver: %s", s.driver)
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

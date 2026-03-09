package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

const (
	RuntimeTypeLibvirt = "libvirt"
	RuntimeTypeGCE     = "gce"
	RuntimeTypeLXD     = "lxd"
)

type RuntimeCatalog struct {
	ID         string
	Name       string
	Type       string
	ConfigJSON string
	CreatedAt  int64
	UpdatedAt  int64
}

var ErrRuntimeNameAlreadyExists = errors.New("runtime name already exists")
var ErrRuntimeInUse = errors.New("runtime is in use")

func (s *Store) ListRuntimes(ctx context.Context) ([]RuntimeCatalog, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListRuntimes(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]RuntimeCatalog, 0, len(rows))
		for _, row := range rows {
			items = append(items, RuntimeCatalog{
				ID:         row.ID,
				Name:       row.Name,
				Type:       strings.ToLower(strings.TrimSpace(row.Type)),
				ConfigJSON: strings.TrimSpace(row.ConfigJson),
				CreatedAt:  row.CreatedAt,
				UpdatedAt:  row.UpdatedAt,
			})
		}
		return items, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListRuntimes(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]RuntimeCatalog, 0, len(rows))
		for _, row := range rows {
			items = append(items, RuntimeCatalog{
				ID:         row.ID,
				Name:       row.Name,
				Type:       strings.ToLower(strings.TrimSpace(row.Type)),
				ConfigJSON: strings.TrimSpace(row.ConfigJson),
				CreatedAt:  row.CreatedAt,
				UpdatedAt:  row.UpdatedAt,
			})
		}
		return items, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateRuntime(ctx context.Context, name, runtimeType, configJSON string) (RuntimeCatalog, error) {
	runtimeID, err := randomID()
	if err != nil {
		return RuntimeCatalog{}, err
	}
	nowUnix := time.Now().Unix()
	item := RuntimeCatalog{
		ID:         runtimeID,
		Name:       strings.TrimSpace(name),
		Type:       strings.ToLower(strings.TrimSpace(runtimeType)),
		ConfigJSON: strings.TrimSpace(configJSON),
		CreatedAt:  nowUnix,
		UpdatedAt:  nowUnix,
	}

	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.CreateRuntime(ctx, sqlitesqlc.CreateRuntimeParams{
			ID:         item.ID,
			Name:       item.Name,
			Type:       item.Type,
			ConfigJson: item.ConfigJSON,
			CreatedAt:  item.CreatedAt,
			UpdatedAt:  item.UpdatedAt,
		})
	case DriverPostgres:
		err = s.pgQueries.CreateRuntime(ctx, postgresqlsqlc.CreateRuntimeParams{
			ID:         item.ID,
			Name:       item.Name,
			Type:       item.Type,
			ConfigJson: item.ConfigJSON,
			CreatedAt:  item.CreatedAt,
			UpdatedAt:  item.UpdatedAt,
		})
	default:
		return RuntimeCatalog{}, unsupportedDriverError(s.driver)
	}
	if err != nil {
		if isRuntimeNameUniqueConstraintError(err) {
			return RuntimeCatalog{}, ErrRuntimeNameAlreadyExists
		}
		return RuntimeCatalog{}, err
	}

	return item, nil
}

func (s *Store) UpdateRuntimeByID(ctx context.Context, runtimeID, name, runtimeType, configJSON string) (RuntimeCatalog, bool, error) {
	runtimeID = strings.TrimSpace(runtimeID)
	item := RuntimeCatalog{
		ID:         runtimeID,
		Name:       strings.TrimSpace(name),
		Type:       strings.ToLower(strings.TrimSpace(runtimeType)),
		ConfigJSON: strings.TrimSpace(configJSON),
	}
	if runtimeID == "" {
		return RuntimeCatalog{}, false, nil
	}
	nowUnix := time.Now().Unix()
	item.UpdatedAt = nowUnix

	var (
		updated int64
		err     error
	)
	switch s.driver {
	case DriverSQLite:
		updated, err = s.sqliteQueries.UpdateRuntimeByID(ctx, sqlitesqlc.UpdateRuntimeByIDParams{
			ID:         item.ID,
			Name:       item.Name,
			Type:       item.Type,
			ConfigJson: item.ConfigJSON,
			UpdatedAt:  nowUnix,
		})
	case DriverPostgres:
		updated, err = s.pgQueries.UpdateRuntimeByID(ctx, postgresqlsqlc.UpdateRuntimeByIDParams{
			ID:         item.ID,
			Name:       item.Name,
			Type:       item.Type,
			ConfigJson: item.ConfigJSON,
			UpdatedAt:  nowUnix,
		})
	default:
		return RuntimeCatalog{}, false, unsupportedDriverError(s.driver)
	}
	if err != nil {
		if isRuntimeNameUniqueConstraintError(err) {
			return RuntimeCatalog{}, false, ErrRuntimeNameAlreadyExists
		}
		return RuntimeCatalog{}, false, err
	}
	if updated == 0 {
		return RuntimeCatalog{}, false, nil
	}

	current, err := s.GetRuntimeByID(ctx, runtimeID)
	if err != nil {
		return RuntimeCatalog{}, false, err
	}
	return current, true, nil
}

func (s *Store) DeleteRuntimeByID(ctx context.Context, runtimeID string) (bool, error) {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return false, nil
	}

	inUseCount, err := s.countMachinesByRuntimeID(ctx, runtimeID)
	if err != nil {
		return false, err
	}
	if inUseCount > 0 {
		return false, ErrRuntimeInUse
	}

	switch s.driver {
	case DriverSQLite:
		updated, err := s.sqliteQueries.DeleteRuntimeByID(ctx, runtimeID)
		return updated > 0, err
	case DriverPostgres:
		updated, err := s.pgQueries.DeleteRuntimeByID(ctx, runtimeID)
		return updated > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) countMachinesByRuntimeID(ctx context.Context, runtimeID string) (int64, error) {
	var query string
	switch s.driver {
	case DriverSQLite:
		query = "SELECT COUNT(1) FROM machines WHERE runtime_id = ?"
	case DriverPostgres:
		query = "SELECT COUNT(1) FROM machines WHERE runtime_id = $1"
	default:
		return 0, unsupportedDriverError(s.driver)
	}

	var count int64
	if err := s.db.QueryRowContext(ctx, query, runtimeID).Scan(&count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

func (s *Store) GetRuntimeByID(ctx context.Context, runtimeID string) (RuntimeCatalog, error) {
	runtimeID = strings.TrimSpace(runtimeID)

	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetRuntimeByID(ctx, runtimeID)
		if err != nil {
			return RuntimeCatalog{}, err
		}
		return RuntimeCatalog{
			ID:         row.ID,
			Name:       row.Name,
			Type:       strings.ToLower(strings.TrimSpace(row.Type)),
			ConfigJSON: strings.TrimSpace(row.ConfigJson),
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetRuntimeByID(ctx, runtimeID)
		if err != nil {
			return RuntimeCatalog{}, err
		}
		return RuntimeCatalog{
			ID:         row.ID,
			Name:       row.Name,
			Type:       strings.ToLower(strings.TrimSpace(row.Type)),
			ConfigJSON: strings.TrimSpace(row.ConfigJson),
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		}, nil
	default:
		return RuntimeCatalog{}, unsupportedDriverError(s.driver)
	}
}

func isRuntimeNameUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(strings.ToLower(pgErr.Message), "name")
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "runtimes.name") {
		return true
	}
	return strings.Contains(msg, "duplicate key value") && strings.Contains(msg, "name")
}

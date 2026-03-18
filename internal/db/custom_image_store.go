package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

type CustomImage struct {
	ID          string
	Name        string
	RuntimeType string
	DataJSON    string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var ErrCustomImageNameAlreadyExists = errors.New("custom image name already exists")

func (s *Store) ListCustomImages(ctx context.Context) ([]CustomImage, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListCustomImages(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]CustomImage, 0, len(rows))
		for _, row := range rows {
			items = append(items, CustomImage{
				ID:          row.ID,
				Name:        row.Name,
				RuntimeType: row.RuntimeType,
				DataJSON:    row.DataJson,
				Description: row.Description,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
			})
		}
		return items, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListCustomImages(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]CustomImage, 0, len(rows))
		for _, row := range rows {
			items = append(items, CustomImage{
				ID:          row.ID,
				Name:        row.Name,
				RuntimeType: row.RuntimeType,
				DataJSON:    row.DataJson,
				Description: row.Description,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
			})
		}
		return items, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetCustomImage(ctx context.Context, id string) (CustomImage, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetCustomImage(ctx, id)
		if err != nil {
			return CustomImage{}, err
		}
		return CustomImage{
			ID:          row.ID,
			Name:        row.Name,
			RuntimeType: row.RuntimeType,
			DataJSON:    row.DataJson,
			Description: row.Description,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetCustomImage(ctx, id)
		if err != nil {
			return CustomImage{}, err
		}
		return CustomImage{
			ID:          row.ID,
			Name:        row.Name,
			RuntimeType: row.RuntimeType,
			DataJSON:    row.DataJson,
			Description: row.Description,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}, nil
	default:
		return CustomImage{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateCustomImage(ctx context.Context, name, runtimeType, dataJSON, description string) (CustomImage, error) {
	id, err := randomID()
	if err != nil {
		return CustomImage{}, err
	}
	now := time.Now().UTC()
	item := CustomImage{
		ID:          id,
		Name:        strings.TrimSpace(name),
		RuntimeType: strings.TrimSpace(runtimeType),
		DataJSON:    dataJSON,
		Description: strings.TrimSpace(description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.CreateCustomImage(ctx, sqlitesqlc.CreateCustomImageParams{
			ID:          item.ID,
			Name:        item.Name,
			RuntimeType: item.RuntimeType,
			DataJson:    item.DataJSON,
			Description: item.Description,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	case DriverPostgres:
		err = s.pgQueries.CreateCustomImage(ctx, postgresqlsqlc.CreateCustomImageParams{
			ID:          item.ID,
			Name:        item.Name,
			RuntimeType: item.RuntimeType,
			DataJson:    item.DataJSON,
			Description: item.Description,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	default:
		return CustomImage{}, unsupportedDriverError(s.driver)
	}
	if err != nil {
		if isCustomImageNameUniqueConstraintError(err) {
			return CustomImage{}, ErrCustomImageNameAlreadyExists
		}
		return CustomImage{}, err
	}
	return item, nil
}

func (s *Store) UpdateCustomImage(ctx context.Context, id, name, runtimeType, dataJSON, description string) (CustomImage, bool, error) {
	now := time.Now().UTC()
	var (
		updated int64
		err     error
	)
	switch s.driver {
	case DriverSQLite:
		updated, err = s.sqliteQueries.UpdateCustomImage(ctx, sqlitesqlc.UpdateCustomImageParams{
			ID:          id,
			Name:        strings.TrimSpace(name),
			RuntimeType: strings.TrimSpace(runtimeType),
			DataJson:    dataJSON,
			Description: strings.TrimSpace(description),
			UpdatedAt:   now,
		})
	case DriverPostgres:
		updated, err = s.pgQueries.UpdateCustomImage(ctx, postgresqlsqlc.UpdateCustomImageParams{
			ID:          id,
			Name:        strings.TrimSpace(name),
			RuntimeType: strings.TrimSpace(runtimeType),
			DataJson:    dataJSON,
			Description: strings.TrimSpace(description),
			UpdatedAt:   now,
		})
	default:
		return CustomImage{}, false, unsupportedDriverError(s.driver)
	}
	if err != nil {
		if isCustomImageNameUniqueConstraintError(err) {
			return CustomImage{}, false, ErrCustomImageNameAlreadyExists
		}
		return CustomImage{}, false, err
	}
	if updated == 0 {
		return CustomImage{}, false, nil
	}
	img, err := s.GetCustomImage(ctx, id)
	if err != nil {
		return CustomImage{}, false, err
	}
	return img, true, nil
}

func (s *Store) DeleteCustomImage(ctx context.Context, id string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.DeleteCustomImage(ctx, id)
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.DeleteCustomImage(ctx, id)
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListCustomImagesByRuntimeID(ctx context.Context, runtimeID string) ([]CustomImage, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListCustomImagesByRuntimeID(ctx, runtimeID)
		if err != nil {
			return nil, err
		}
		items := make([]CustomImage, 0, len(rows))
		for _, row := range rows {
			items = append(items, CustomImage{
				ID:          row.ID,
				Name:        row.Name,
				RuntimeType: row.RuntimeType,
				DataJSON:    row.DataJson,
				Description: row.Description,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
			})
		}
		return items, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListCustomImagesByRuntimeID(ctx, runtimeID)
		if err != nil {
			return nil, err
		}
		items := make([]CustomImage, 0, len(rows))
		for _, row := range rows {
			items = append(items, CustomImage{
				ID:          row.ID,
				Name:        row.Name,
				RuntimeType: row.RuntimeType,
				DataJSON:    row.DataJson,
				Description: row.Description,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
			})
		}
		return items, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) AssociateRuntimeCustomImage(ctx context.Context, runtimeID, customImageID string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.AssociateRuntimeCustomImage(ctx, sqlitesqlc.AssociateRuntimeCustomImageParams{
			RuntimeID:     runtimeID,
			CustomImageID: customImageID,
		})
	case DriverPostgres:
		return s.pgQueries.AssociateRuntimeCustomImage(ctx, postgresqlsqlc.AssociateRuntimeCustomImageParams{
			RuntimeID:     runtimeID,
			CustomImageID: customImageID,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) DisassociateRuntimeCustomImage(ctx context.Context, runtimeID, customImageID string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.DisassociateRuntimeCustomImage(ctx, sqlitesqlc.DisassociateRuntimeCustomImageParams{
			RuntimeID:     runtimeID,
			CustomImageID: customImageID,
		})
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.DisassociateRuntimeCustomImage(ctx, postgresqlsqlc.DisassociateRuntimeCustomImageParams{
			RuntimeID:     runtimeID,
			CustomImageID: customImageID,
		})
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) DisassociateAllRuntimesFromCustomImage(ctx context.Context, customImageID string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.DisassociateAllRuntimesFromCustomImage(ctx, customImageID)
	case DriverPostgres:
		return s.pgQueries.DisassociateAllRuntimesFromCustomImage(ctx, customImageID)
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListRuntimeIDsByCustomImageID(ctx context.Context, customImageID string) ([]string, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.ListRuntimeIDsByCustomImageID(ctx, customImageID)
	case DriverPostgres:
		return s.pgQueries.ListRuntimeIDsByCustomImageID(ctx, customImageID)
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func isCustomImageNameUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "custom_images") {
		return true
	}
	return strings.Contains(msg, "duplicate key value") && strings.Contains(msg, "custom_images")
}

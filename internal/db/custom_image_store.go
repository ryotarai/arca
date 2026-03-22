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

type CustomImage struct {
	ID              string
	Name            string
	ProviderType    string
	DataJSON        string
	Description     string
	SourceMachineID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
				ProviderType: row.ProviderType,
				DataJSON:    row.DataJson,
				Description: row.Description,
				SourceMachineID: row.SourceMachineID.String,
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
				ProviderType: row.ProviderType,
				DataJSON:    row.DataJson,
				Description: row.Description,
				SourceMachineID: row.SourceMachineID.String,
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
			ProviderType: row.ProviderType,
			DataJSON:    row.DataJson,
			Description: row.Description,
			SourceMachineID: row.SourceMachineID.String,
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
			ProviderType: row.ProviderType,
			DataJSON:    row.DataJson,
			Description: row.Description,
			SourceMachineID: row.SourceMachineID.String,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		}, nil
	default:
		return CustomImage{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetCustomImageByNameAndProviderType(ctx context.Context, name, providerType string) (CustomImage, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetCustomImageByNameAndProviderType(ctx, sqlitesqlc.GetCustomImageByNameAndProviderTypeParams{
			Name:         name,
			ProviderType: providerType,
		})
		if err != nil {
			return CustomImage{}, err
		}
		return CustomImage{
			ID:              row.ID,
			Name:            row.Name,
			ProviderType:    row.ProviderType,
			DataJSON:        row.DataJson,
			Description:     row.Description,
			SourceMachineID: row.SourceMachineID.String,
			CreatedAt:       row.CreatedAt,
			UpdatedAt:       row.UpdatedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetCustomImageByNameAndProviderType(ctx, postgresqlsqlc.GetCustomImageByNameAndProviderTypeParams{
			Name:         name,
			ProviderType: providerType,
		})
		if err != nil {
			return CustomImage{}, err
		}
		return CustomImage{
			ID:              row.ID,
			Name:            row.Name,
			ProviderType:    row.ProviderType,
			DataJSON:        row.DataJson,
			Description:     row.Description,
			SourceMachineID: row.SourceMachineID.String,
			CreatedAt:       row.CreatedAt,
			UpdatedAt:       row.UpdatedAt,
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
		ProviderType: strings.TrimSpace(runtimeType),
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
			ProviderType: item.ProviderType,
			DataJson:    item.DataJSON,
			Description: item.Description,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	case DriverPostgres:
		err = s.pgQueries.CreateCustomImage(ctx, postgresqlsqlc.CreateCustomImageParams{
			ID:          item.ID,
			Name:        item.Name,
			ProviderType: item.ProviderType,
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
			ProviderType: strings.TrimSpace(runtimeType),
			DataJson:    dataJSON,
			Description: strings.TrimSpace(description),
			UpdatedAt:   now,
		})
	case DriverPostgres:
		updated, err = s.pgQueries.UpdateCustomImage(ctx, postgresqlsqlc.UpdateCustomImageParams{
			ID:          id,
			Name:        strings.TrimSpace(name),
			ProviderType: strings.TrimSpace(runtimeType),
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

func (s *Store) ListCustomImagesByProfileID(ctx context.Context, profileID string) ([]CustomImage, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListCustomImagesByProfileID(ctx, profileID)
		if err != nil {
			return nil, err
		}
		items := make([]CustomImage, 0, len(rows))
		for _, row := range rows {
			items = append(items, CustomImage{
				ID:          row.ID,
				Name:        row.Name,
				ProviderType: row.ProviderType,
				DataJSON:    row.DataJson,
				Description: row.Description,
				SourceMachineID: row.SourceMachineID.String,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
			})
		}
		return items, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListCustomImagesByProfileID(ctx, profileID)
		if err != nil {
			return nil, err
		}
		items := make([]CustomImage, 0, len(rows))
		for _, row := range rows {
			items = append(items, CustomImage{
				ID:          row.ID,
				Name:        row.Name,
				ProviderType: row.ProviderType,
				DataJSON:    row.DataJson,
				Description: row.Description,
				SourceMachineID: row.SourceMachineID.String,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
			})
		}
		return items, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

// ListCustomImagesByTemplateID is an alias for backward compatibility.
// Deprecated: Use ListCustomImagesByProfileID instead.
func (s *Store) ListCustomImagesByTemplateID(ctx context.Context, profileID string) ([]CustomImage, error) {
	return s.ListCustomImagesByProfileID(ctx, profileID)
}

func (s *Store) AssociateProfileCustomImage(ctx context.Context, profileID, customImageID string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.AssociateProfileCustomImage(ctx, sqlitesqlc.AssociateProfileCustomImageParams{
			ProfileID:     profileID,
			CustomImageID: customImageID,
		})
	case DriverPostgres:
		return s.pgQueries.AssociateProfileCustomImage(ctx, postgresqlsqlc.AssociateProfileCustomImageParams{
			ProfileID:     profileID,
			CustomImageID: customImageID,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

// AssociateTemplateCustomImage is an alias for backward compatibility.
// Deprecated: Use AssociateProfileCustomImage instead.
func (s *Store) AssociateTemplateCustomImage(ctx context.Context, profileID, customImageID string) error {
	return s.AssociateProfileCustomImage(ctx, profileID, customImageID)
}

func (s *Store) DisassociateProfileCustomImage(ctx context.Context, profileID, customImageID string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.DisassociateProfileCustomImage(ctx, sqlitesqlc.DisassociateProfileCustomImageParams{
			ProfileID:     profileID,
			CustomImageID: customImageID,
		})
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.DisassociateProfileCustomImage(ctx, postgresqlsqlc.DisassociateProfileCustomImageParams{
			ProfileID:     profileID,
			CustomImageID: customImageID,
		})
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

// DisassociateTemplateCustomImage is an alias for backward compatibility.
// Deprecated: Use DisassociateProfileCustomImage instead.
func (s *Store) DisassociateTemplateCustomImage(ctx context.Context, profileID, customImageID string) (bool, error) {
	return s.DisassociateProfileCustomImage(ctx, profileID, customImageID)
}

func (s *Store) DisassociateAllProfilesFromCustomImage(ctx context.Context, customImageID string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.DisassociateAllProfilesFromCustomImage(ctx, customImageID)
	case DriverPostgres:
		return s.pgQueries.DisassociateAllProfilesFromCustomImage(ctx, customImageID)
	default:
		return unsupportedDriverError(s.driver)
	}
}

// DisassociateAllTemplatesFromCustomImage is an alias for backward compatibility.
// Deprecated: Use DisassociateAllProfilesFromCustomImage instead.
func (s *Store) DisassociateAllTemplatesFromCustomImage(ctx context.Context, customImageID string) error {
	return s.DisassociateAllProfilesFromCustomImage(ctx, customImageID)
}

func (s *Store) ListProfileIDsByCustomImageID(ctx context.Context, customImageID string) ([]string, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.ListProfileIDsByCustomImageID(ctx, customImageID)
	case DriverPostgres:
		return s.pgQueries.ListProfileIDsByCustomImageID(ctx, customImageID)
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

// ListTemplateIDsByCustomImageID is an alias for backward compatibility.
// Deprecated: Use ListProfileIDsByCustomImageID instead.
func (s *Store) ListTemplateIDsByCustomImageID(ctx context.Context, customImageID string) ([]string, error) {
	return s.ListProfileIDsByCustomImageID(ctx, customImageID)
}

func (s *Store) CreateCustomImageFromMachine(ctx context.Context, name, providerType, dataJSON, description, sourceMachineID, profileID string) (*CustomImage, error) {
	id, err := randomID()
	if err != nil {
		return nil, err
	}

	switch s.driver {
	case DriverSQLite:
		tx, err := s.beginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		q := s.sqliteQueries.WithTx(tx)
		if err := q.InsertCustomImageWithSource(ctx, sqlitesqlc.InsertCustomImageWithSourceParams{
			ID:              id,
			Name:            strings.TrimSpace(name),
			ProviderType:    strings.TrimSpace(providerType),
			DataJson:        dataJSON,
			Description:     strings.TrimSpace(description),
			SourceMachineID: sql.NullString{String: sourceMachineID, Valid: sourceMachineID != ""},
		}); err != nil {
			if isCustomImageNameUniqueConstraintError(err) {
				existing, fetchErr := s.GetCustomImageByNameAndProviderType(ctx, strings.TrimSpace(name), strings.TrimSpace(providerType))
				if fetchErr != nil {
					return nil, ErrCustomImageNameAlreadyExists
				}
				return &existing, nil
			}
			return nil, err
		}
		if err := q.AssociateProfileCustomImage(ctx, sqlitesqlc.AssociateProfileCustomImageParams{
			ProfileID:    profileID,
			CustomImageID: id,
		}); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	case DriverPostgres:
		tx, err := s.beginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		q := s.pgQueries.WithTx(tx)
		if err := q.InsertCustomImageWithSource(ctx, postgresqlsqlc.InsertCustomImageWithSourceParams{
			ID:              id,
			Name:            strings.TrimSpace(name),
			ProviderType:    strings.TrimSpace(providerType),
			DataJson:        dataJSON,
			Description:     strings.TrimSpace(description),
			SourceMachineID: sql.NullString{String: sourceMachineID, Valid: sourceMachineID != ""},
		}); err != nil {
			if isCustomImageNameUniqueConstraintError(err) {
				existing, fetchErr := s.GetCustomImageByNameAndProviderType(ctx, strings.TrimSpace(name), strings.TrimSpace(providerType))
				if fetchErr != nil {
					return nil, ErrCustomImageNameAlreadyExists
				}
				return &existing, nil
			}
			return nil, err
		}
		if err := q.AssociateProfileCustomImage(ctx, postgresqlsqlc.AssociateProfileCustomImageParams{
			ProfileID:    profileID,
			CustomImageID: id,
		}); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	default:
		return nil, unsupportedDriverError(s.driver)
	}

	img, err := s.GetCustomImage(ctx, id)
	if err != nil {
		return nil, err
	}
	return &img, nil
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

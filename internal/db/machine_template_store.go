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
	TemplateTypeLibvirt = "libvirt"
	TemplateTypeGCE     = "gce"
	TemplateTypeLXD     = "lxd"
)

type MachineTemplate struct {
	ID         string
	Name       string
	Type       string
	ConfigJSON string
	CreatedAt  int64
	UpdatedAt  int64
}

var ErrTemplateNameAlreadyExists = errors.New("template name already exists")
var ErrTemplateInUse = errors.New("template is in use")

func (s *Store) ListMachineTemplates(ctx context.Context) ([]MachineTemplate, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachineTemplates(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]MachineTemplate, 0, len(rows))
		for _, row := range rows {
			items = append(items, MachineTemplate{
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
		rows, err := s.pgQueries.ListMachineTemplates(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]MachineTemplate, 0, len(rows))
		for _, row := range rows {
			items = append(items, MachineTemplate{
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

func (s *Store) CreateMachineTemplate(ctx context.Context, name, templateType, configJSON string) (MachineTemplate, error) {
	templateID, err := randomID()
	if err != nil {
		return MachineTemplate{}, err
	}
	nowUnix := time.Now().Unix()
	item := MachineTemplate{
		ID:         templateID,
		Name:       strings.TrimSpace(name),
		Type:       strings.ToLower(strings.TrimSpace(templateType)),
		ConfigJSON: strings.TrimSpace(configJSON),
		CreatedAt:  nowUnix,
		UpdatedAt:  nowUnix,
	}

	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.CreateMachineTemplate(ctx, sqlitesqlc.CreateMachineTemplateParams{
			ID:         item.ID,
			Name:       item.Name,
			Type:       item.Type,
			ConfigJson: item.ConfigJSON,
			CreatedAt:  item.CreatedAt,
			UpdatedAt:  item.UpdatedAt,
		})
	case DriverPostgres:
		err = s.pgQueries.CreateMachineTemplate(ctx, postgresqlsqlc.CreateMachineTemplateParams{
			ID:         item.ID,
			Name:       item.Name,
			Type:       item.Type,
			ConfigJson: item.ConfigJSON,
			CreatedAt:  item.CreatedAt,
			UpdatedAt:  item.UpdatedAt,
		})
	default:
		return MachineTemplate{}, unsupportedDriverError(s.driver)
	}
	if err != nil {
		if isTemplateNameUniqueConstraintError(err) {
			return MachineTemplate{}, ErrTemplateNameAlreadyExists
		}
		return MachineTemplate{}, err
	}

	return item, nil
}

func (s *Store) UpdateMachineTemplateByID(ctx context.Context, templateID, name, templateType, configJSON string) (MachineTemplate, bool, error) {
	templateID = strings.TrimSpace(templateID)
	item := MachineTemplate{
		ID:         templateID,
		Name:       strings.TrimSpace(name),
		Type:       strings.ToLower(strings.TrimSpace(templateType)),
		ConfigJSON: strings.TrimSpace(configJSON),
	}
	if templateID == "" {
		return MachineTemplate{}, false, nil
	}
	nowUnix := time.Now().Unix()
	item.UpdatedAt = nowUnix

	var (
		updated int64
		err     error
	)
	switch s.driver {
	case DriverSQLite:
		updated, err = s.sqliteQueries.UpdateMachineTemplateByID(ctx, sqlitesqlc.UpdateMachineTemplateByIDParams{
			ID:         item.ID,
			Name:       item.Name,
			Type:       item.Type,
			ConfigJson: item.ConfigJSON,
			UpdatedAt:  nowUnix,
		})
	case DriverPostgres:
		updated, err = s.pgQueries.UpdateMachineTemplateByID(ctx, postgresqlsqlc.UpdateMachineTemplateByIDParams{
			ID:         item.ID,
			Name:       item.Name,
			Type:       item.Type,
			ConfigJson: item.ConfigJSON,
			UpdatedAt:  nowUnix,
		})
	default:
		return MachineTemplate{}, false, unsupportedDriverError(s.driver)
	}
	if err != nil {
		if isTemplateNameUniqueConstraintError(err) {
			return MachineTemplate{}, false, ErrTemplateNameAlreadyExists
		}
		return MachineTemplate{}, false, err
	}
	if updated == 0 {
		return MachineTemplate{}, false, nil
	}

	current, err := s.GetMachineTemplateByID(ctx, templateID)
	if err != nil {
		return MachineTemplate{}, false, err
	}
	return current, true, nil
}

func (s *Store) DeleteMachineTemplateByID(ctx context.Context, templateID string) (bool, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return false, nil
	}

	inUseCount, err := s.countMachinesByTemplateID(ctx, templateID)
	if err != nil {
		return false, err
	}
	if inUseCount > 0 {
		return false, ErrTemplateInUse
	}

	switch s.driver {
	case DriverSQLite:
		updated, err := s.sqliteQueries.DeleteMachineTemplateByID(ctx, templateID)
		return updated > 0, err
	case DriverPostgres:
		updated, err := s.pgQueries.DeleteMachineTemplateByID(ctx, templateID)
		return updated > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) countMachinesByTemplateID(ctx context.Context, templateID string) (int64, error) {
	var query string
	switch s.driver {
	case DriverSQLite:
		query = "SELECT COUNT(1) FROM machines WHERE template_id = ?"
	case DriverPostgres:
		query = "SELECT COUNT(1) FROM machines WHERE template_id = $1"
	default:
		return 0, unsupportedDriverError(s.driver)
	}

	var count int64
	if err := s.db.QueryRowContext(ctx, query, templateID).Scan(&count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

func (s *Store) GetMachineTemplateByID(ctx context.Context, templateID string) (MachineTemplate, error) {
	templateID = strings.TrimSpace(templateID)

	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineTemplateByID(ctx, templateID)
		if err != nil {
			return MachineTemplate{}, err
		}
		return MachineTemplate{
			ID:         row.ID,
			Name:       row.Name,
			Type:       strings.ToLower(strings.TrimSpace(row.Type)),
			ConfigJSON: strings.TrimSpace(row.ConfigJson),
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineTemplateByID(ctx, templateID)
		if err != nil {
			return MachineTemplate{}, err
		}
		return MachineTemplate{
			ID:         row.ID,
			Name:       row.Name,
			Type:       strings.ToLower(strings.TrimSpace(row.Type)),
			ConfigJSON: strings.TrimSpace(row.ConfigJson),
			CreatedAt:  row.CreatedAt,
			UpdatedAt:  row.UpdatedAt,
		}, nil
	default:
		return MachineTemplate{}, unsupportedDriverError(s.driver)
	}
}

func isTemplateNameUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(strings.ToLower(pgErr.Message), "name")
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "machine_templates.name") {
		return true
	}
	return strings.Contains(msg, "duplicate key value") && strings.Contains(msg, "name")
}

package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

const (
	ProviderTypeLibvirt = "libvirt"
	ProviderTypeGCE     = "gce"
	ProviderTypeLXD     = "lxd"
	ProviderTypeMock    = "mock"
)

type MachineProfile struct {
	ID             string
	Name           string
	Type           string
	ConfigJSON     string
	BootConfigHash string
	CreatedAt      int64
	UpdatedAt      int64
}

var ErrProfileNameAlreadyExists = errors.New("profile name already exists")
var ErrProfileInUse = errors.New("profile is in use")

func (s *Store) ListMachineProfiles(ctx context.Context) ([]MachineProfile, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachineProfiles(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]MachineProfile, 0, len(rows))
		for _, row := range rows {
			items = append(items, MachineProfile{
				ID:             row.ID,
				Name:           row.Name,
				Type:           strings.ToLower(strings.TrimSpace(row.Type)),
				ConfigJSON:     strings.TrimSpace(row.ConfigJson),
				BootConfigHash: row.BootConfigHash,
				CreatedAt:      row.CreatedAt,
				UpdatedAt:      row.UpdatedAt,
			})
		}
		return items, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachineProfiles(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]MachineProfile, 0, len(rows))
		for _, row := range rows {
			items = append(items, MachineProfile{
				ID:             row.ID,
				Name:           row.Name,
				Type:           strings.ToLower(strings.TrimSpace(row.Type)),
				ConfigJSON:     strings.TrimSpace(row.ConfigJson),
				BootConfigHash: row.BootConfigHash,
				CreatedAt:      row.CreatedAt,
				UpdatedAt:      row.UpdatedAt,
			})
		}
		return items, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateMachineProfile(ctx context.Context, name, profileType, configJSON string) (MachineProfile, error) {
	profileID, err := randomID()
	if err != nil {
		return MachineProfile{}, err
	}
	nowUnix := time.Now().Unix()
	item := MachineProfile{
		ID:             profileID,
		Name:           strings.TrimSpace(name),
		Type:           strings.ToLower(strings.TrimSpace(profileType)),
		ConfigJSON:     strings.TrimSpace(configJSON),
		BootConfigHash: computeBootConfigHash(strings.TrimSpace(configJSON)),
		CreatedAt:      nowUnix,
		UpdatedAt:      nowUnix,
	}

	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.CreateMachineProfile(ctx, sqlitesqlc.CreateMachineProfileParams{
			ID:             item.ID,
			Name:           item.Name,
			Type:           item.Type,
			ConfigJson:     item.ConfigJSON,
			BootConfigHash: item.BootConfigHash,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
		})
	case DriverPostgres:
		err = s.pgQueries.CreateMachineProfile(ctx, postgresqlsqlc.CreateMachineProfileParams{
			ID:             item.ID,
			Name:           item.Name,
			Type:           item.Type,
			ConfigJson:     item.ConfigJSON,
			BootConfigHash: item.BootConfigHash,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
		})
	default:
		return MachineProfile{}, unsupportedDriverError(s.driver)
	}
	if err != nil {
		if isProfileNameUniqueConstraintError(err) {
			return MachineProfile{}, ErrProfileNameAlreadyExists
		}
		return MachineProfile{}, err
	}

	return item, nil
}

func (s *Store) UpdateMachineProfileByID(ctx context.Context, profileID, name, profileType, configJSON string) (MachineProfile, bool, error) {
	profileID = strings.TrimSpace(profileID)
	item := MachineProfile{
		ID:             profileID,
		Name:           strings.TrimSpace(name),
		Type:           strings.ToLower(strings.TrimSpace(profileType)),
		ConfigJSON:     strings.TrimSpace(configJSON),
		BootConfigHash: computeBootConfigHash(strings.TrimSpace(configJSON)),
	}
	if profileID == "" {
		return MachineProfile{}, false, nil
	}
	nowUnix := time.Now().Unix()
	item.UpdatedAt = nowUnix

	var (
		updated int64
		err     error
	)
	switch s.driver {
	case DriverSQLite:
		updated, err = s.sqliteQueries.UpdateMachineProfileByID(ctx, sqlitesqlc.UpdateMachineProfileByIDParams{
			ID:             item.ID,
			Name:           item.Name,
			Type:           item.Type,
			ConfigJson:     item.ConfigJSON,
			BootConfigHash: item.BootConfigHash,
			UpdatedAt:      nowUnix,
		})
	case DriverPostgres:
		updated, err = s.pgQueries.UpdateMachineProfileByID(ctx, postgresqlsqlc.UpdateMachineProfileByIDParams{
			ID:             item.ID,
			Name:           item.Name,
			Type:           item.Type,
			ConfigJson:     item.ConfigJSON,
			BootConfigHash: item.BootConfigHash,
			UpdatedAt:      nowUnix,
		})
	default:
		return MachineProfile{}, false, unsupportedDriverError(s.driver)
	}
	if err != nil {
		if isProfileNameUniqueConstraintError(err) {
			return MachineProfile{}, false, ErrProfileNameAlreadyExists
		}
		return MachineProfile{}, false, err
	}
	if updated == 0 {
		return MachineProfile{}, false, nil
	}

	current, err := s.GetMachineProfileByID(ctx, profileID)
	if err != nil {
		return MachineProfile{}, false, err
	}
	return current, true, nil
}

func (s *Store) DeleteMachineProfileByID(ctx context.Context, profileID string) (bool, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return false, nil
	}

	inUseCount, err := s.countMachinesByProfileID(ctx, profileID)
	if err != nil {
		return false, err
	}
	if inUseCount > 0 {
		return false, ErrProfileInUse
	}

	switch s.driver {
	case DriverSQLite:
		updated, err := s.sqliteQueries.DeleteMachineProfileByID(ctx, profileID)
		return updated > 0, err
	case DriverPostgres:
		updated, err := s.pgQueries.DeleteMachineProfileByID(ctx, profileID)
		return updated > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) countMachinesByProfileID(ctx context.Context, profileID string) (int64, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CountMachinesByProfileID(ctx, profileID)
	case DriverPostgres:
		return s.pgQueries.CountMachinesByProfileID(ctx, profileID)
	default:
		return 0, unsupportedDriverError(s.driver)
	}
}

// CountMachinesByProfileID returns total machine count for a profile (exported).
func (s *Store) CountMachinesByProfileID(ctx context.Context, profileID string) (int64, error) {
	return s.countMachinesByProfileID(ctx, profileID)
}

// CountRunningMachinesByProfileID returns running machine count for a profile.
func (s *Store) CountRunningMachinesByProfileID(ctx context.Context, profileID string) (int64, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CountRunningMachinesByProfileID(ctx, profileID)
	case DriverPostgres:
		return s.pgQueries.CountRunningMachinesByProfileID(ctx, profileID)
	default:
		return 0, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineProfileByID(ctx context.Context, profileID string) (MachineProfile, error) {
	profileID = strings.TrimSpace(profileID)

	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineProfileByID(ctx, profileID)
		if err != nil {
			return MachineProfile{}, err
		}
		return MachineProfile{
			ID:             row.ID,
			Name:           row.Name,
			Type:           strings.ToLower(strings.TrimSpace(row.Type)),
			ConfigJSON:     strings.TrimSpace(row.ConfigJson),
			BootConfigHash: row.BootConfigHash,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineProfileByID(ctx, profileID)
		if err != nil {
			return MachineProfile{}, err
		}
		return MachineProfile{
			ID:             row.ID,
			Name:           row.Name,
			Type:           strings.ToLower(strings.TrimSpace(row.Type)),
			ConfigJSON:     strings.TrimSpace(row.ConfigJson),
			BootConfigHash: row.BootConfigHash,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		}, nil
	default:
		return MachineProfile{}, unsupportedDriverError(s.driver)
	}
}

func isProfileNameUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(strings.ToLower(pgErr.Message), "name")
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "machine_profiles.name") {
		return true
	}
	return strings.Contains(msg, "duplicate key value") && strings.Contains(msg, "name")
}

// computeBootConfigHash extracts boot-time fields from the config JSON
// (startup_script from provider configs) and returns a SHA-256 hex hash
// of a canonical serialization of those fields.
func computeBootConfigHash(configJSON string) string {
	if configJSON == "" || configJSON == "{}" {
		return ""
	}

	// Parse the config to extract boot-time fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		return ""
	}

	bootFields := make(map[string]interface{})

	// Extract startup_script from each provider config
	for _, provider := range []string{"libvirt", "gce", "lxd"} {
		providerRaw, ok := raw[provider]
		if !ok {
			continue
		}
		var providerConfig map[string]json.RawMessage
		if err := json.Unmarshal(providerRaw, &providerConfig); err != nil {
			continue
		}
		if startupScript, ok := providerConfig["startupScript"]; ok {
			var script string
			if err := json.Unmarshal(startupScript, &script); err == nil {
				bootFields[provider+".startupScript"] = script
			}
		}
		if startupScript, ok := providerConfig["startup_script"]; ok {
			var script string
			if err := json.Unmarshal(startupScript, &script); err == nil {
				bootFields[provider+".startup_script"] = script
			}
		}
	}

	if len(bootFields) == 0 {
		return ""
	}

	// Serialize canonically (sorted keys)
	keys := make([]string, 0, len(bootFields))
	for k := range bootFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	canonical, err := json.Marshal(bootFields)
	if err != nil {
		return ""
	}

	// Use sorted keys for determinism - json.Marshal sorts map keys in Go
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

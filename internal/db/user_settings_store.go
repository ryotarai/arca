package db

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

type UserSettings struct {
	SSHPublicKeys []string
}

func (s *Store) GetUserSettingsByUserID(ctx context.Context, userID string) (UserSettings, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetUserSettingsByUserID(ctx, userID)
		if err != nil {
			return UserSettings{}, err
		}
		keys, err := decodeSSHPublicKeysJSON(row.SshPublicKeysJson)
		if err != nil {
			return UserSettings{}, err
		}
		return UserSettings{SSHPublicKeys: keys}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetUserSettingsByUserID(ctx, userID)
		if err != nil {
			return UserSettings{}, err
		}
		keys, err := decodeSSHPublicKeysJSON(row.SshPublicKeysJson)
		if err != nil {
			return UserSettings{}, err
		}
		return UserSettings{SSHPublicKeys: keys}, nil
	default:
		return UserSettings{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertUserSettingsByUserID(ctx context.Context, userID string, settings UserSettings) error {
	sshPublicKeysJSON, err := encodeSSHPublicKeysJSON(settings.SSHPublicKeys)
	if err != nil {
		return err
	}
	nowUnix := time.Now().Unix()

	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertUserSettingsByUserID(ctx, sqlitesqlc.UpsertUserSettingsByUserIDParams{
			UserID:            userID,
			SshPublicKeysJson: sshPublicKeysJSON,
			CreatedAt:         nowUnix,
			UpdatedAt:         nowUnix,
		})
	case DriverPostgres:
		return s.pgQueries.UpsertUserSettingsByUserID(ctx, postgresqlsqlc.UpsertUserSettingsByUserIDParams{
			UserID:            userID,
			SshPublicKeysJson: sshPublicKeysJSON,
			CreatedAt:         nowUnix,
			UpdatedAt:         nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func encodeSSHPublicKeysJSON(keys []string) (string, error) {
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	data, err := json.Marshal(filtered)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSSHPublicKeysJSON(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []string{}, nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(trimmed), &keys); err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		filtered = append(filtered, key)
	}
	return filtered, nil
}

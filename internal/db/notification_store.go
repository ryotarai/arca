package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

const (
	slackMetaEnabled          = "slack.enabled"
	slackMetaBotToken         = "slack.bot_token"
	slackMetaDefaultChannelID = "slack.default_channel_id"
)

// SlackConfig holds Slack integration settings stored in app_meta.
type SlackConfig struct {
	Enabled          bool
	BotToken         string
	DefaultChannelID string
}

// UserNotificationSettings holds per-user notification preferences.
type UserNotificationSettings struct {
	SlackEnabled bool
	SlackUserID  string
}

func (s *Store) GetSlackConfig(ctx context.Context) (SlackConfig, error) {
	enabledRaw, err := s.getMetaValue(ctx, slackMetaEnabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SlackConfig{}, err
	}
	botToken, err := s.getMetaValue(ctx, slackMetaBotToken)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SlackConfig{}, err
	}
	defaultChannelID, err := s.getMetaValue(ctx, slackMetaDefaultChannelID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SlackConfig{}, err
	}

	return SlackConfig{
		Enabled:          parseBoolMetaValue(enabledRaw),
		BotToken:         strings.TrimSpace(botToken),
		DefaultChannelID: strings.TrimSpace(defaultChannelID),
	}, nil
}

func (s *Store) UpdateSlackConfig(ctx context.Context, config SlackConfig) error {
	if err := s.upsertMetaValue(ctx, slackMetaEnabled, boolMetaValue(config.Enabled)); err != nil {
		return err
	}
	if config.BotToken != "" {
		if err := s.upsertMetaValue(ctx, slackMetaBotToken, strings.TrimSpace(config.BotToken)); err != nil {
			return err
		}
	}
	return s.upsertMetaValue(ctx, slackMetaDefaultChannelID, strings.TrimSpace(config.DefaultChannelID))
}

func (s *Store) GetUserNotificationSettings(ctx context.Context, userID string) (UserNotificationSettings, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetUserNotificationSettings(ctx, userID)
		if err != nil {
			return UserNotificationSettings{}, err
		}
		return UserNotificationSettings{
			SlackEnabled: row.SlackEnabled,
			SlackUserID:  row.SlackUserID,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetUserNotificationSettings(ctx, userID)
		if err != nil {
			return UserNotificationSettings{}, err
		}
		return UserNotificationSettings{
			SlackEnabled: row.SlackEnabled,
			SlackUserID:  row.SlackUserID,
		}, nil
	default:
		return UserNotificationSettings{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertUserNotificationSettings(ctx context.Context, userID string, settings UserNotificationSettings) error {
	nowUnix := time.Now().Unix()

	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertUserNotificationSettings(ctx, sqlitesqlc.UpsertUserNotificationSettingsParams{
			UserID:       userID,
			SlackEnabled: settings.SlackEnabled,
			SlackUserID:  strings.TrimSpace(settings.SlackUserID),
			CreatedAt:    nowUnix,
			UpdatedAt:    nowUnix,
		})
	case DriverPostgres:
		return s.pgQueries.UpsertUserNotificationSettings(ctx, postgresqlsqlc.UpsertUserNotificationSettingsParams{
			UserID:       userID,
			SlackEnabled: settings.SlackEnabled,
			SlackUserID:  strings.TrimSpace(settings.SlackUserID),
			CreatedAt:    nowUnix,
			UpdatedAt:    nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

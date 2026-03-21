package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
	"github.com/ryotarai/arca/internal/notification"
)

type notificationConnectService struct {
	store         *db.Store
	authenticator Authenticator
	slack         *notification.SlackService
}

func newNotificationConnectService(store *db.Store, authenticator Authenticator, slack *notification.SlackService) *notificationConnectService {
	return &notificationConnectService{store: store, authenticator: authenticator, slack: slack}
}

func (s *notificationConnectService) GetSlackConfig(ctx context.Context, req *connect.Request[arcav1.GetSlackConfigRequest]) (*connect.Response[arcav1.GetSlackConfigResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	config, err := s.store.GetSlackConfig(ctx)
	if err != nil {
		log.Printf("get slack config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load slack config"))
	}

	maskedToken := ""
	if config.BotToken != "" {
		maskedToken = "****" + config.BotToken[max(0, len(config.BotToken)-4):]
	}

	return connect.NewResponse(&arcav1.GetSlackConfigResponse{
		Config: &arcav1.SlackConfig{
			Enabled:            config.Enabled,
			BotToken:           maskedToken,
			DefaultChannelId:   config.DefaultChannelID,
			BotTokenConfigured: config.BotToken != "",
		},
	}), nil
}

func (s *notificationConnectService) UpdateSlackConfig(ctx context.Context, req *connect.Request[arcav1.UpdateSlackConfigRequest]) (*connect.Response[arcav1.UpdateSlackConfigResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	cfg := req.Msg.GetConfig()
	update := db.SlackConfig{
		Enabled:          cfg.GetEnabled(),
		BotToken:         strings.TrimSpace(cfg.GetBotToken()),
		DefaultChannelID: strings.TrimSpace(cfg.GetDefaultChannelId()),
	}

	if err := s.store.UpdateSlackConfig(ctx, update); err != nil {
		log.Printf("update slack config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update slack config"))
	}

	// Reload to return the saved state (with masked token).
	saved, err := s.store.GetSlackConfig(ctx)
	if err != nil {
		log.Printf("reload slack config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to reload slack config"))
	}

	maskedToken := ""
	if saved.BotToken != "" {
		maskedToken = "****" + saved.BotToken[max(0, len(saved.BotToken)-4):]
	}

	return connect.NewResponse(&arcav1.UpdateSlackConfigResponse{
		Config: &arcav1.SlackConfig{
			Enabled:            saved.Enabled,
			BotToken:           maskedToken,
			DefaultChannelId:   saved.DefaultChannelID,
			BotTokenConfigured: saved.BotToken != "",
		},
	}), nil
}

func (s *notificationConnectService) TestSlackNotification(ctx context.Context, req *connect.Request[arcav1.TestSlackNotificationRequest]) (*connect.Response[arcav1.TestSlackNotificationResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	channelID := strings.TrimSpace(req.Msg.GetChannelId())
	if channelID == "" {
		// Fall back to the configured default channel.
		config, err := s.store.GetSlackConfig(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load slack config"))
		}
		channelID = config.DefaultChannelID
	}
	if channelID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("channel id is required"))
	}

	if err := s.slack.SendTestMessage(ctx, channelID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&arcav1.TestSlackNotificationResponse{}), nil
}

func (s *notificationConnectService) GetUserNotificationSettings(ctx context.Context, req *connect.Request[arcav1.GetUserNotificationSettingsRequest]) (*connect.Response[arcav1.GetUserNotificationSettingsResponse], error) {
	userID, err := s.authenticateUser(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	slackConfig, err := s.store.GetSlackConfig(ctx)
	if err != nil {
		log.Printf("get slack config failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load slack config"))
	}

	settings, err := s.store.GetUserNotificationSettings(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No settings yet; return defaults.
			return connect.NewResponse(&arcav1.GetUserNotificationSettingsResponse{
				Settings: &arcav1.UserNotificationSettings{
					SlackEnabled: true,
					SlackUserId:  "",
				},
				SlackAdminEnabled: slackConfig.Enabled,
			}), nil
		}
		log.Printf("get user notification settings failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load notification settings"))
	}

	return connect.NewResponse(&arcav1.GetUserNotificationSettingsResponse{
		Settings: &arcav1.UserNotificationSettings{
			SlackEnabled: settings.SlackEnabled,
			SlackUserId:  settings.SlackUserID,
		},
		SlackAdminEnabled: slackConfig.Enabled,
	}), nil
}

func (s *notificationConnectService) UpdateUserNotificationSettings(ctx context.Context, req *connect.Request[arcav1.UpdateUserNotificationSettingsRequest]) (*connect.Response[arcav1.UpdateUserNotificationSettingsResponse], error) {
	userID, err := s.authenticateUser(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	input := req.Msg.GetSettings()
	update := db.UserNotificationSettings{
		SlackEnabled: input.GetSlackEnabled(),
		SlackUserID:  strings.TrimSpace(input.GetSlackUserId()),
	}

	if err := s.store.UpsertUserNotificationSettings(ctx, userID, update); err != nil {
		log.Printf("update user notification settings failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update notification settings"))
	}

	return connect.NewResponse(&arcav1.UpdateUserNotificationSettingsResponse{
		Settings: &arcav1.UserNotificationSettings{
			SlackEnabled: update.SlackEnabled,
			SlackUserId:  update.SlackUserID,
		},
	}), nil
}

func (s *notificationConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("notification service unavailable"))
	}
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, header)
	if err != nil {
		return "", err
	}
	if result.Role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage notification settings"))
	}
	return result.UserID, nil
}

func (s *notificationConnectService) authenticateUser(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("notification service unavailable"))
	}
	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		userID, _, _, err := s.authenticator.Authenticate(ctx, sessionToken)
		if err == nil {
			return userID, nil
		}
	}

	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, _, _, err := s.authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			return userID, nil
		}
	}

	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

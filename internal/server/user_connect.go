package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
	"golang.org/x/crypto/ssh"
)

type userConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newUserConnectService(store *db.Store, authenticator Authenticator) *userConnectService {
	return &userConnectService{store: store, authenticator: authenticator}
}

func (s *userConnectService) ListUsers(ctx context.Context, req *connect.Request[arcav1.ListUsersRequest]) (*connect.Response[arcav1.ListUsersResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	users, err := s.authenticator.ListUsers(ctx)
	if err != nil {
		log.Printf("list users failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list users"))
	}

	items := make([]*arcav1.ManagedUser, 0, len(users))
	for _, user := range users {
		items = append(items, toManagedUserMessage(user))
	}
	return connect.NewResponse(&arcav1.ListUsersResponse{Users: items}), nil
}

func (s *userConnectService) CreateUser(ctx context.Context, req *connect.Request[arcav1.CreateUserRequest]) (*connect.Response[arcav1.CreateUserResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	email := strings.TrimSpace(req.Msg.GetEmail())
	if email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("email is required"))
	}

	userID, normalizedEmail, setupToken, expiresAt, err := s.authenticator.ProvisionUser(ctx, email, adminUserID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("email is invalid"))
		case errors.Is(err, auth.ErrEmailAlreadyUsed):
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("email already used"))
		default:
			log.Printf("create user failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create user"))
		}
	}

	return connect.NewResponse(&arcav1.CreateUserResponse{
		User: &arcav1.ManagedUser{
			Id:                  userID,
			Email:               normalizedEmail,
			SetupRequired:       true,
			SetupTokenExpiresAt: expiresAt.Unix(),
		},
		SetupToken:          setupToken,
		SetupTokenExpiresAt: expiresAt.Unix(),
	}), nil
}

func (s *userConnectService) IssueUserSetupToken(ctx context.Context, req *connect.Request[arcav1.IssueUserSetupTokenRequest]) (*connect.Response[arcav1.IssueUserSetupTokenResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user id is required"))
	}

	setupToken, expiresAt, err := s.authenticator.IssueUserSetupToken(ctx, userID, adminUserID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user id is invalid"))
		case errors.Is(err, auth.ErrUserNotFound):
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		default:
			log.Printf("issue user setup token failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to issue setup token"))
		}
	}

	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		log.Printf("get user failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load user"))
	}

	return connect.NewResponse(&arcav1.IssueUserSetupTokenResponse{
		User: &arcav1.ManagedUser{
			Id:                  user.ID,
			Email:               user.Email,
			SetupRequired:       true,
			SetupTokenExpiresAt: expiresAt.Unix(),
		},
		SetupToken:          setupToken,
		SetupTokenExpiresAt: expiresAt.Unix(),
	}), nil
}

func (s *userConnectService) CompleteUserSetup(ctx context.Context, req *connect.Request[arcav1.CompleteUserSetupRequest]) (*connect.Response[arcav1.CompleteUserSetupResponse], error) {
	userID, email, err := s.authenticator.CompleteUserSetup(ctx, req.Msg.GetSetupToken(), req.Msg.GetPassword())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("setup token and password are required"))
		case errors.Is(err, auth.ErrInvalidSetupToken):
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("setup token is invalid or expired"))
		default:
			log.Printf("complete user setup failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to complete user setup"))
		}
	}

	return connect.NewResponse(&arcav1.CompleteUserSetupResponse{
		User: &arcav1.User{Id: userID, Email: email},
	}), nil
}

func (s *userConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("user management unavailable"))
	}
	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	userID, _, role, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage users"))
	}
	return userID, nil
}

func (s *userConnectService) UpdateUserRole(ctx context.Context, req *connect.Request[arcav1.UpdateUserRoleRequest]) (*connect.Response[arcav1.UpdateUserRoleResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	userID := strings.TrimSpace(req.Msg.GetUserId())
	role := strings.TrimSpace(req.Msg.GetRole())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user id is required"))
	}
	if role != db.UserRoleAdmin && role != db.UserRoleUser {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("role must be 'admin' or 'user'"))
	}
	if userID == adminUserID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot change your own role"))
	}

	updated, err := s.store.UpdateUserRoleByID(ctx, userID, role)
	if err != nil {
		log.Printf("update user role failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update user role"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}

	users, err := s.authenticator.ListUsers(ctx)
	if err != nil {
		log.Printf("list users failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load user"))
	}
	for _, u := range users {
		if u.ID == userID {
			return connect.NewResponse(&arcav1.UpdateUserRoleResponse{User: toManagedUserMessage(u)}), nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
}

func (s *userConnectService) GetUserSettings(ctx context.Context, req *connect.Request[arcav1.GetUserSettingsRequest]) (*connect.Response[arcav1.GetUserSettingsResponse], error) {
	userID, err := s.authenticateUser(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	settings, err := s.store.GetUserSettingsByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		log.Printf("get user settings failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load user settings"))
	}
	return connect.NewResponse(&arcav1.GetUserSettingsResponse{
		Settings: toUserSettingsMessage(settings),
	}), nil
}

func (s *userConnectService) UpdateUserSettings(ctx context.Context, req *connect.Request[arcav1.UpdateUserSettingsRequest]) (*connect.Response[arcav1.UpdateUserSettingsResponse], error) {
	userID, err := s.authenticateUser(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	settings := req.Msg.GetSettings()
	normalizedKeys, err := normalizeSSHPublicKeys(settings.GetSshPublicKeys())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	updated := db.UserSettings{SSHPublicKeys: normalizedKeys}
	if err := s.store.UpsertUserSettingsByUserID(ctx, userID, updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		log.Printf("update user settings failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update user settings"))
	}
	return connect.NewResponse(&arcav1.UpdateUserSettingsResponse{
		Settings: toUserSettingsMessage(updated),
	}), nil
}

func toManagedUserMessage(user db.ManagedUser) *arcav1.ManagedUser {
	return &arcav1.ManagedUser{
		Id:                  user.ID,
		Email:               user.Email,
		SetupRequired:       user.PasswordSetupRequired,
		SetupTokenExpiresAt: user.SetupTokenExpiresAt,
		CreatedAt:           user.CreatedAt,
		Role:                user.Role,
	}
}

func toUserSettingsMessage(settings db.UserSettings) *arcav1.UserSettings {
	return &arcav1.UserSettings{
		SshPublicKeys: append([]string(nil), settings.SSHPublicKeys...),
	}
}

func (s *userConnectService) authenticateUser(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("user management unavailable"))
	}
	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	userID, _, _, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return userID, nil
}

func normalizeSSHPublicKeys(input []string) ([]string, error) {
	const maxSSHPublicKeys = 50
	const maxSSHPublicKeyLen = 16 * 1024

	normalized := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, key := range input {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > maxSSHPublicKeyLen {
			return nil, errors.New("ssh public key is too long")
		}
		parsed, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(trimmed))
		if err != nil || len(bytes.TrimSpace(rest)) > 0 {
			return nil, errors.New("ssh public key is invalid")
		}
		fingerprint := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(parsed)))
		if _, ok := seen[fingerprint]; ok {
			continue
		}
		seen[fingerprint] = struct{}{}
		normalized = append(normalized, trimmed)
		if len(normalized) > maxSSHPublicKeys {
			return nil, errors.New("too many ssh public keys")
		}
	}
	return normalized, nil
}

package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/crypto"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
	"golang.org/x/crypto/ssh"
)

type userConnectService struct {
	store         *db.Store
	authenticator Authenticator
	encryptor     *crypto.Encryptor
}

func newUserConnectService(store *db.Store, authenticator Authenticator, encryptor *crypto.Encryptor) *userConnectService {
	return &userConnectService{store: store, authenticator: authenticator, encryptor: encryptor}
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

	writeAuditLog(ctx, s.store, adminUserID, "", "user.create", "user", userID, fmt.Sprintf(`{"email":%q}`, normalizedEmail))

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

	writeAuditLog(ctx, s.store, adminUserID, "", "user.issue_setup_token", "user", userID, fmt.Sprintf(`{"email":%q}`, user.Email))

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
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, header)
	if err != nil {
		return "", err
	}
	if result.Role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage users"))
	}
	return result.UserID, nil
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

	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		log.Printf("get user failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load user"))
	}

	writeAuditLog(ctx, s.store, adminUserID, "", "user.update_role", "user", userID, fmt.Sprintf(`{"email":%q,"role":%q}`, user.Email, role))

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

func (s *userConnectService) SearchUsers(ctx context.Context, req *connect.Request[arcav1.SearchUsersRequest]) (*connect.Response[arcav1.SearchUsersResponse], error) {
	if _, err := s.authenticateUser(ctx, req.Header()); err != nil {
		return nil, err
	}

	query := strings.TrimSpace(req.Msg.GetQuery())
	if query == "" {
		return connect.NewResponse(&arcav1.SearchUsersResponse{}), nil
	}

	limit := int64(req.Msg.GetLimit())
	if limit <= 0 || limit > 20 {
		limit = 20
	}

	results, err := s.store.SearchUsersByEmail(ctx, query, limit)
	if err != nil {
		log.Printf("search users failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to search users"))
	}

	items := make([]*arcav1.UserSearchResult, 0, len(results))
	for _, r := range results {
		items = append(items, &arcav1.UserSearchResult{Id: r.ID, Email: r.Email})
	}
	return connect.NewResponse(&arcav1.SearchUsersResponse{Users: items}), nil
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

var validEndpointTypes = map[string]bool{
	"openai_chat":     true,
	"openai_response": true,
	"anthropic":       true,
	"google_gemini":   true,
}

func (s *userConnectService) ListUserLLMModels(ctx context.Context, req *connect.Request[arcav1.ListUserLLMModelsRequest]) (*connect.Response[arcav1.ListUserLLMModelsResponse], error) {
	userID, err := s.authenticateUser(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	models, err := s.store.ListUserLLMModels(ctx, userID)
	if err != nil {
		log.Printf("list user llm models failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list LLM models"))
	}

	items := make([]*arcav1.LLMModel, 0, len(models))
	for _, m := range models {
		items = append(items, toLLMModelMessage(m))
	}
	return connect.NewResponse(&arcav1.ListUserLLMModelsResponse{Models: items}), nil
}

func (s *userConnectService) CreateUserLLMModel(ctx context.Context, req *connect.Request[arcav1.CreateUserLLMModelRequest]) (*connect.Response[arcav1.CreateUserLLMModelResponse], error) {
	userID, err := s.authenticateUser(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	if s.encryptor == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("encryption not configured; set ARCA_ENCRYPTION_KEY"))
	}

	configName := strings.TrimSpace(req.Msg.GetConfigName())
	if configName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config_name is required"))
	}
	endpointType := strings.TrimSpace(req.Msg.GetEndpointType())
	if !validEndpointTypes[endpointType] {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("endpoint_type must be one of: openai_chat, openai_response, anthropic, google_gemini"))
	}
	modelName := strings.TrimSpace(req.Msg.GetModelName())
	if modelName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("model_name is required"))
	}
	apiKey := req.Msg.GetApiKey()

	encryptedKey := ""
	if apiKey != "" {
		encryptedKey, err = s.encryptor.Encrypt(apiKey)
		if err != nil {
			log.Printf("encrypt api key failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encrypt API key"))
		}
	}

	id := uuid.New().String()
	model := db.UserLLMModel{
		ID:               id,
		UserID:           userID,
		ConfigName:       configName,
		EndpointType:     endpointType,
		CustomEndpoint:   strings.TrimSpace(req.Msg.GetCustomEndpoint()),
		ModelName:        modelName,
		APIKeyEncrypted:  encryptedKey,
		MaxContextTokens: int64(req.Msg.GetMaxContextTokens()),
	}

	if err := s.store.CreateUserLLMModel(ctx, model); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("config_name already exists"))
		}
		log.Printf("create user llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create LLM model"))
	}

	created, err := s.store.GetUserLLMModel(ctx, id, userID)
	if err != nil {
		log.Printf("get created llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load created LLM model"))
	}

	writeAuditLog(ctx, s.store, userID, "", "user.create_llm_model", "user_llm_model", id, fmt.Sprintf(`{"config_name":%q}`, configName))

	return connect.NewResponse(&arcav1.CreateUserLLMModelResponse{
		Model: toLLMModelMessageFromFull(created),
	}), nil
}

func (s *userConnectService) UpdateUserLLMModel(ctx context.Context, req *connect.Request[arcav1.UpdateUserLLMModelRequest]) (*connect.Response[arcav1.UpdateUserLLMModelResponse], error) {
	userID, err := s.authenticateUser(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	if s.encryptor == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("encryption not configured; set ARCA_ENCRYPTION_KEY"))
	}

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	existing, err := s.store.GetUserLLMModel(ctx, id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("LLM model not found"))
		}
		log.Printf("get user llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load LLM model"))
	}

	configName := strings.TrimSpace(req.Msg.GetConfigName())
	if configName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config_name is required"))
	}
	endpointType := strings.TrimSpace(req.Msg.GetEndpointType())
	if !validEndpointTypes[endpointType] {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("endpoint_type must be one of: openai_chat, openai_response, anthropic, google_gemini"))
	}
	modelName := strings.TrimSpace(req.Msg.GetModelName())
	if modelName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("model_name is required"))
	}

	apiKey := req.Msg.GetApiKey()
	encryptedKey := existing.APIKeyEncrypted
	if apiKey != "" {
		encryptedKey, err = s.encryptor.Encrypt(apiKey)
		if err != nil {
			log.Printf("encrypt api key failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encrypt API key"))
		}
	}

	model := db.UserLLMModel{
		ID:               id,
		UserID:           userID,
		ConfigName:       configName,
		EndpointType:     endpointType,
		CustomEndpoint:   strings.TrimSpace(req.Msg.GetCustomEndpoint()),
		ModelName:        modelName,
		APIKeyEncrypted:  encryptedKey,
		MaxContextTokens: int64(req.Msg.GetMaxContextTokens()),
	}

	updated, err := s.store.UpdateUserLLMModel(ctx, model)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("config_name already exists"))
		}
		log.Printf("update user llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update LLM model"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("LLM model not found"))
	}

	result, err := s.store.GetUserLLMModel(ctx, id, userID)
	if err != nil {
		log.Printf("get updated llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load updated LLM model"))
	}

	writeAuditLog(ctx, s.store, userID, "", "user.update_llm_model", "user_llm_model", id, fmt.Sprintf(`{"config_name":%q}`, configName))

	return connect.NewResponse(&arcav1.UpdateUserLLMModelResponse{
		Model: toLLMModelMessageFromFull(result),
	}), nil
}

func (s *userConnectService) DeleteUserLLMModel(ctx context.Context, req *connect.Request[arcav1.DeleteUserLLMModelRequest]) (*connect.Response[arcav1.DeleteUserLLMModelResponse], error) {
	userID, err := s.authenticateUser(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	existing, getErr := s.store.GetUserLLMModel(ctx, id, userID)
	var configName string
	if getErr == nil {
		configName = existing.ConfigName
	}

	deleted, err := s.store.DeleteUserLLMModel(ctx, id, userID)
	if err != nil {
		log.Printf("delete user llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete LLM model"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("LLM model not found"))
	}

	writeAuditLog(ctx, s.store, userID, "", "user.delete_llm_model", "user_llm_model", id, fmt.Sprintf(`{"config_name":%q}`, configName))

	return connect.NewResponse(&arcav1.DeleteUserLLMModelResponse{}), nil
}

func toLLMModelMessage(m db.UserLLMModelSummary) *arcav1.LLMModel {
	return &arcav1.LLMModel{
		Id:               m.ID,
		ConfigName:       m.ConfigName,
		EndpointType:     m.EndpointType,
		CustomEndpoint:   m.CustomEndpoint,
		ModelName:        m.ModelName,
		HasApiKey:        true, // API key is always present in summary (we don't know if it's empty from summary)
		MaxContextTokens: int32(m.MaxContextTokens),
		CreatedAt:        fmt.Sprintf("%d", m.CreatedAt),
		UpdatedAt:        fmt.Sprintf("%d", m.UpdatedAt),
	}
}

func toLLMModelMessageFromFull(m db.UserLLMModel) *arcav1.LLMModel {
	return &arcav1.LLMModel{
		Id:               m.ID,
		ConfigName:       m.ConfigName,
		EndpointType:     m.EndpointType,
		CustomEndpoint:   m.CustomEndpoint,
		ModelName:        m.ModelName,
		HasApiKey:        m.APIKeyEncrypted != "",
		MaxContextTokens: int32(m.MaxContextTokens),
		CreatedAt:        fmt.Sprintf("%d", m.CreatedAt),
		UpdatedAt:        fmt.Sprintf("%d", m.UpdatedAt),
	}
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

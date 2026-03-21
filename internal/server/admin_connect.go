package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type adminConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newAdminConnectService(store *db.Store, authenticator Authenticator) *adminConnectService {
	return &adminConnectService{store: store, authenticator: authenticator}
}

func (s *adminConnectService) SetAdminViewMode(ctx context.Context, req *connect.Request[arcav1.SetAdminViewModeRequest]) (*connect.Response[arcav1.SetAdminViewModeResponse], error) {
	result, err := s.authenticateActualAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	mode := req.Msg.GetMode()
	if mode != "admin" && mode != "user" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("mode must be 'admin' or 'user'"))
	}
	if err := s.store.SetAdminViewMode(ctx, result.UserID, mode, time.Now().Unix()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&arcav1.SetAdminViewModeResponse{}), nil
}

func (s *adminConnectService) GetAdminViewMode(ctx context.Context, req *connect.Request[arcav1.GetAdminViewModeRequest]) (*connect.Response[arcav1.GetAdminViewModeResponse], error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, req.Header())
	if err != nil {
		return nil, err
	}
	user, err := s.store.GetUserByID(ctx, result.UserID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch user"))
	}
	isAdmin := user.Role == db.UserRoleAdmin
	mode := "admin"
	if isAdmin {
		m, mErr := s.store.GetAdminViewMode(ctx, result.UserID)
		if mErr == nil {
			mode = m
		}
	}
	return connect.NewResponse(&arcav1.GetAdminViewModeResponse{
		Mode:    mode,
		IsAdmin: isAdmin,
	}), nil
}

func (s *adminConnectService) ListAuditLogs(ctx context.Context, req *connect.Request[arcav1.ListAuditLogsRequest]) (*connect.Response[arcav1.ListAuditLogsResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	limit := int64(req.Msg.GetLimit())
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	actionPrefix := strings.TrimSpace(req.Msg.GetActionPrefix())
	actorEmail := strings.TrimSpace(req.Msg.GetActorEmail())
	offset := int64(req.Msg.GetOffset())
	if offset < 0 {
		offset = 0
	}

	filter := db.AuditLogFilter{
		ActionPrefix: actionPrefix,
		ActorEmail:   actorEmail,
		Limit:        limit,
		Offset:       offset,
	}

	entries, err := s.store.ListAuditLogsFiltered(ctx, filter)
	if err != nil {
		log.Printf("list audit logs failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list audit logs"))
	}

	totalCount, err := s.store.CountAuditLogsFiltered(ctx, actionPrefix, actorEmail)
	if err != nil {
		log.Printf("count audit logs failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to count audit logs"))
	}

	items := make([]*arcav1.AuditLog, 0, len(entries))
	for _, e := range entries {
		items = append(items, &arcav1.AuditLog{
			Id:            e.ID,
			ActorEmail:    e.ActorEmail,
			ActingAsEmail: e.ActingAsEmail,
			Action:        e.Action,
			ResourceType:  e.ResourceType,
			ResourceId:    e.ResourceID,
			Details:       e.DetailsJSON,
			CreatedAt:     fmt.Sprintf("%d", e.CreatedAt.Unix()),
		})
	}

	return connect.NewResponse(&arcav1.ListAuditLogsResponse{AuditLogs: items, TotalCount: int32(totalCount)}), nil
}

// authenticateAdmin checks the effective role (blocked by non-admin mode).
func (s *adminConnectService) authenticateAdmin(ctx context.Context, header http.Header) (auth.AuthResult, error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, header)
	if err != nil {
		return auth.AuthResult{}, err
	}
	if result.Role != db.UserRoleAdmin {
		return auth.AuthResult{}, connect.NewError(connect.CodePermissionDenied, errors.New("admin required"))
	}
	return result, nil
}

// authenticateActualAdmin checks the actual DB role, not effective role.
// This allows admins in non-admin mode to switch back.
func (s *adminConnectService) authenticateActualAdmin(ctx context.Context, header http.Header) (auth.AuthResult, error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, header)
	if err != nil {
		return auth.AuthResult{}, err
	}
	user, err := s.store.GetUserByID(ctx, result.UserID)
	if err != nil {
		return auth.AuthResult{}, connect.NewError(connect.CodeInternal, errors.New("failed to fetch user"))
	}
	if user.Role != db.UserRoleAdmin {
		return auth.AuthResult{}, connect.NewError(connect.CodePermissionDenied, errors.New("admin required"))
	}
	return result, nil
}

func (s *adminConnectService) logAudit(ctx context.Context, actorUserID, actingAsUserID, action, resourceType, resourceID, detailsJSON string) {
	id, err := randomAuditID()
	if err != nil {
		log.Printf("generate audit log id failed: %v", err)
		return
	}
	entry := db.AuditLogEntry{
		ID:             id,
		ActorUserID:    actorUserID,
		ActingAsUserID: actingAsUserID,
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		DetailsJSON:    detailsJSON,
		CreatedAt:      time.Now(),
	}
	if err := s.store.CreateAuditLog(ctx, entry); err != nil {
		log.Printf("create audit log failed: %v", err)
	}
}

func randomAuditID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *adminConnectService) ListServerLLMModels(ctx context.Context, req *connect.Request[arcav1.ListServerLLMModelsRequest]) (*connect.Response[arcav1.ListServerLLMModelsResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	models, err := s.store.ListServerLLMModels(ctx)
	if err != nil {
		log.Printf("list server llm models failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list server LLM models"))
	}

	items := make([]*arcav1.ServerLLMModel, 0, len(models))
	for _, m := range models {
		items = append(items, serverLLMModelToProto(m))
	}

	return connect.NewResponse(&arcav1.ListServerLLMModelsResponse{Models: items}), nil
}

func (s *adminConnectService) CreateServerLLMModel(ctx context.Context, req *connect.Request[arcav1.CreateServerLLMModelRequest]) (*connect.Response[arcav1.CreateServerLLMModelResponse], error) {
	adminResult, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	configName := strings.TrimSpace(req.Msg.GetConfigName())
	endpointType := strings.TrimSpace(req.Msg.GetEndpointType())
	modelName := strings.TrimSpace(req.Msg.GetModelName())
	tokenCommand := strings.TrimSpace(req.Msg.GetTokenCommand())

	if configName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config_name is required"))
	}
	if endpointType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("endpoint_type is required"))
	}
	if modelName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("model_name is required"))
	}
	if tokenCommand == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("token_command is required"))
	}

	id, err := randomAuditID()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to generate ID"))
	}

	model := db.ServerLLMModel{
		ID:               id,
		ConfigName:       configName,
		EndpointType:     endpointType,
		CustomEndpoint:   strings.TrimSpace(req.Msg.GetCustomEndpoint()),
		ModelName:        modelName,
		TokenCommand:     tokenCommand,
		MaxContextTokens: int64(req.Msg.GetMaxContextTokens()),
	}

	if err := s.store.CreateServerLLMModel(ctx, model); err != nil {
		log.Printf("create server llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create server LLM model"))
	}

	created, err := s.store.GetServerLLMModel(ctx, id)
	if err != nil {
		log.Printf("get server llm model after create failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to retrieve created model"))
	}

	s.logAudit(ctx, adminResult.UserID, "", "server_llm_model.create", "server_llm_model", id, fmt.Sprintf(`{"config_name":%q}`, configName))

	return connect.NewResponse(&arcav1.CreateServerLLMModelResponse{Model: serverLLMModelToProto(created)}), nil
}

func (s *adminConnectService) UpdateServerLLMModel(ctx context.Context, req *connect.Request[arcav1.UpdateServerLLMModelRequest]) (*connect.Response[arcav1.UpdateServerLLMModelResponse], error) {
	adminResult, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	configName := strings.TrimSpace(req.Msg.GetConfigName())
	endpointType := strings.TrimSpace(req.Msg.GetEndpointType())
	modelName := strings.TrimSpace(req.Msg.GetModelName())
	tokenCommand := strings.TrimSpace(req.Msg.GetTokenCommand())

	if configName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("config_name is required"))
	}
	if endpointType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("endpoint_type is required"))
	}
	if modelName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("model_name is required"))
	}
	if tokenCommand == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("token_command is required"))
	}

	// Check existence
	if _, err := s.store.GetServerLLMModel(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server LLM model not found"))
		}
		log.Printf("get server llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get server LLM model"))
	}

	model := db.ServerLLMModel{
		ID:               id,
		ConfigName:       configName,
		EndpointType:     endpointType,
		CustomEndpoint:   strings.TrimSpace(req.Msg.GetCustomEndpoint()),
		ModelName:        modelName,
		TokenCommand:     tokenCommand,
		MaxContextTokens: int64(req.Msg.GetMaxContextTokens()),
	}

	updated, err := s.store.UpdateServerLLMModel(ctx, model)
	if err != nil {
		log.Printf("update server llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update server LLM model"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server LLM model not found"))
	}

	result, err := s.store.GetServerLLMModel(ctx, id)
	if err != nil {
		log.Printf("get server llm model after update failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to retrieve updated model"))
	}

	s.logAudit(ctx, adminResult.UserID, "", "server_llm_model.update", "server_llm_model", id, fmt.Sprintf(`{"config_name":%q}`, configName))

	return connect.NewResponse(&arcav1.UpdateServerLLMModelResponse{Model: serverLLMModelToProto(result)}), nil
}

func (s *adminConnectService) DeleteServerLLMModel(ctx context.Context, req *connect.Request[arcav1.DeleteServerLLMModelRequest]) (*connect.Response[arcav1.DeleteServerLLMModelResponse], error) {
	adminResult, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	existing, err := s.store.GetServerLLMModel(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server LLM model not found"))
		}
		log.Printf("get server llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get server LLM model"))
	}

	deleted, err := s.store.DeleteServerLLMModel(ctx, id)
	if err != nil {
		log.Printf("delete server llm model failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete server LLM model"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server LLM model not found"))
	}

	s.logAudit(ctx, adminResult.UserID, "", "server_llm_model.delete", "server_llm_model", id, fmt.Sprintf(`{"config_name":%q}`, existing.ConfigName))

	return connect.NewResponse(&arcav1.DeleteServerLLMModelResponse{}), nil
}

func serverLLMModelToProto(m db.ServerLLMModel) *arcav1.ServerLLMModel {
	return &arcav1.ServerLLMModel{
		Id:               m.ID,
		ConfigName:       m.ConfigName,
		EndpointType:     m.EndpointType,
		CustomEndpoint:   m.CustomEndpoint,
		ModelName:        m.ModelName,
		TokenCommand:     m.TokenCommand,
		MaxContextTokens: int32(m.MaxContextTokens),
		CreatedAt:        fmt.Sprintf("%d", m.CreatedAt),
		UpdatedAt:        fmt.Sprintf("%d", m.UpdatedAt),
	}
}

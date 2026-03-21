package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/crypto"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type exposureConnectService struct {
	store            *db.Store
	authenticator    Authenticator
	encryptor        *crypto.Encryptor
	llmTokenExecutor *LLMTokenExecutor
}

func newExposureConnectService(store *db.Store, authenticator Authenticator, encryptor *crypto.Encryptor, llmTokenExecutor *LLMTokenExecutor) *exposureConnectService {
	return &exposureConnectService{store: store, authenticator: authenticator, encryptor: encryptor, llmTokenExecutor: llmTokenExecutor}
}

func (s *exposureConnectService) UpsertMachineExposure(ctx context.Context, req *connect.Request[arcav1.UpsertMachineExposureRequest]) (*connect.Response[arcav1.UpsertMachineExposureResponse], error) {
	if s.store == nil || s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("exposure service unavailable"))
	}
	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	name := strings.TrimSpace(req.Msg.GetName())
	if machineID == "" || name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id and name are required"))
	}

	role := s.store.ResolveMachineRole(ctx, userID, machineID)
	if role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	// Validate machine exists
	m, err := s.store.GetMachineByID(ctx, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("load machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load machine"))
	}

	// Dynamically construct the exposure from setup_state
	setup, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("get setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	hostname := db.MachineHostname(setup.DomainPrefix, m.Name, setup.BaseDomain)

	return connect.NewResponse(&arcav1.UpsertMachineExposureResponse{
		Exposure: &arcav1.MachineExposure{
			Id:        m.ID + "/default",
			MachineId: m.ID,
			Name:      "default",
			Hostname:  hostname,
			Service:   "http://localhost:21030",
		},
	}), nil
}

func (s *exposureConnectService) ListMachineExposures(ctx context.Context, req *connect.Request[arcav1.ListMachineExposuresRequest]) (*connect.Response[arcav1.ListMachineExposuresResponse], error) {
	if s.store == nil || s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("exposure service unavailable"))
	}
	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	role := s.store.ResolveMachineRole(ctx, userID, machineID)
	if role == db.MachineRoleNone {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	m, err := s.store.GetMachineByID(ctx, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("load machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load machine"))
	}

	setup, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("get setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	hostname := db.MachineHostname(setup.DomainPrefix, m.Name, setup.BaseDomain)

	items := []*arcav1.MachineExposure{
		{
			Id:        m.ID + "/default",
			MachineId: m.ID,
			Name:      "default",
			Hostname:  hostname,
			Service:   "http://localhost:21030",
		},
	}
	return connect.NewResponse(&arcav1.ListMachineExposuresResponse{Exposures: items}), nil
}

func (s *exposureConnectService) GetMachineExposureByHostname(ctx context.Context, req *connect.Request[arcav1.GetMachineExposureByHostnameRequest]) (*connect.Response[arcav1.GetMachineExposureByHostnameResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("exposure service unavailable"))
	}

	machineToken := strings.TrimSpace(machineTokenFromHeader(req.Header()))
	machineID := strings.TrimSpace(req.Header().Get("X-Arca-Machine-ID"))
	if machineToken == "" && machineID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine token or machine id is required"))
	}

	if machineToken != "" {
		resolvedMachineID, err := s.store.GetMachineIDByMachineToken(ctx, machineToken)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid machine token"))
			}
			log.Printf("get machine id by token failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
		}
		if machineID != "" && machineID != resolvedMachineID {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine id does not match token"))
		}
		machineID = resolvedMachineID
	}

	hostname := strings.TrimSpace(req.Msg.GetHostname())
	if hostname == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hostname is required"))
	}

	// Resolve machine name from hostname via setup_state
	setup, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("get setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	name, ok := db.ExtractMachineNameFromHostname(hostname, setup.DomainPrefix, setup.BaseDomain)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("exposure not found"))
	}
	m, err := s.store.GetMachineByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("exposure not found"))
		}
		log.Printf("get machine by name failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve exposure"))
	}
	if m.ID != machineID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("exposure not found"))
	}

	return connect.NewResponse(&arcav1.GetMachineExposureByHostnameResponse{
		Exposure: &arcav1.MachineExposure{
			Id:        m.ID + "/default",
			MachineId: m.ID,
			Name:      "default",
			Hostname:  hostname,
			Service:   "http://localhost:21030",
		},
	}), nil
}

func (s *exposureConnectService) ReportMachineReadiness(ctx context.Context, req *connect.Request[arcav1.ReportMachineReadinessRequest]) (*connect.Response[arcav1.ReportMachineReadinessResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("exposure service unavailable"))
	}

	machineToken := strings.TrimSpace(machineTokenFromHeader(req.Header()))
	if machineToken == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine token is required"))
	}
	machineID, err := s.store.GetMachineIDByMachineToken(ctx, machineToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid machine token"))
		}
		log.Printf("get machine id by token failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
	}

	requestMachineID := strings.TrimSpace(req.Msg.GetMachineId())
	if requestMachineID != "" && requestMachineID != machineID {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine id does not match token"))
	}

	updated, err := s.store.ReportMachineReadinessByMachineID(
		ctx,
		machineID,
		req.Msg.GetReady(),
		strings.TrimSpace(req.Msg.GetReason()),
		strings.TrimSpace(req.Msg.GetContainerId()),
		strings.TrimSpace(req.Msg.GetArcadVersion()),
	)
	if err != nil {
		log.Printf("report machine readiness failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to report machine readiness"))
	}

	return connect.NewResponse(&arcav1.ReportMachineReadinessResponse{Accepted: updated}), nil
}

func (s *exposureConnectService) GetMachineLLMModels(ctx context.Context, req *connect.Request[arcav1.GetMachineLLMModelsRequest]) (*connect.Response[arcav1.GetMachineLLMModelsResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("exposure service unavailable"))
	}

	machineToken := strings.TrimSpace(machineTokenFromHeader(req.Header()))
	if machineToken == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine token is required"))
	}
	machineID, err := s.store.GetMachineIDByMachineToken(ctx, machineToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid machine token"))
		}
		log.Printf("get machine id by token failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
	}

	ownerUserID, err := s.store.GetMachineOwnerUserID(ctx, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connect.NewResponse(&arcav1.GetMachineLLMModelsResponse{}), nil
		}
		log.Printf("get machine owner failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve machine owner"))
	}

	// Build a map of config_name -> model from user models (user models take priority)
	configNameSet := make(map[string]bool)

	userModels, err := s.store.ListUserLLMModelsWithAPIKey(ctx, ownerUserID)
	if err != nil {
		log.Printf("list user llm models failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list LLM models"))
	}

	items := make([]*arcav1.MachineLLMModel, 0, len(userModels))
	for _, m := range userModels {
		apiKey := ""
		if s.encryptor != nil && m.APIKeyEncrypted != "" {
			decrypted, err := s.encryptor.Decrypt(m.APIKeyEncrypted)
			if err != nil {
				log.Printf("decrypt api key for model %s failed: %v", m.ID, err)
				continue
			}
			apiKey = decrypted
		}
		items = append(items, &arcav1.MachineLLMModel{
			ConfigName:       m.ConfigName + " (Arca)",
			EndpointType:     m.EndpointType,
			CustomEndpoint:   m.CustomEndpoint,
			ModelName:        m.ModelName,
			ApiKey:           apiKey,
			MaxContextTokens: int32(m.MaxContextTokens),
		})
		configNameSet[m.ConfigName] = true
	}

	// Merge server-wide LLM models (skip if config_name already provided by user)
	if s.llmTokenExecutor != nil {
		serverModels, err := s.store.ListServerLLMModels(ctx)
		if err != nil {
			log.Printf("list server llm models failed: %v", err)
			// Continue without server models rather than failing
		} else if len(serverModels) > 0 {
			// Get owner's email for token command stdin
			ownerEmail := ""
			ownerUser, err := s.store.GetUserByID(ctx, ownerUserID)
			if err != nil {
				log.Printf("get owner user for token command failed: %v", err)
			} else {
				ownerEmail = ownerUser.Email
			}

			for _, sm := range serverModels {
				if configNameSet[sm.ConfigName] {
					continue // User model takes priority
				}
				token, err := s.llmTokenExecutor.GetToken(ctx, sm.ID, sm.TokenCommand, ownerEmail, ownerUserID)
				if err != nil {
					log.Printf("execute token command for server model %s failed: %v", sm.ID, err)
					continue
				}
				items = append(items, &arcav1.MachineLLMModel{
					ConfigName:       sm.ConfigName + " (Arca Server)",
					EndpointType:     sm.EndpointType,
					CustomEndpoint:   sm.CustomEndpoint,
					ModelName:        sm.ModelName,
					ApiKey:           token,
					MaxContextTokens: int32(sm.MaxContextTokens),
				})
			}
		}
	}

	return connect.NewResponse(&arcav1.GetMachineLLMModelsResponse{Models: items}), nil
}

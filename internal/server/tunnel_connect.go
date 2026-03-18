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

type tunnelConnectService struct {
	store         *db.Store
	authenticator Authenticator
	encryptor     *crypto.Encryptor
}

func newTunnelConnectService(store *db.Store, authenticator Authenticator, encryptor *crypto.Encryptor) *tunnelConnectService {
	return &tunnelConnectService{store: store, authenticator: authenticator, encryptor: encryptor}
}

func (s *tunnelConnectService) CreateMachineTunnel(context.Context, *connect.Request[arcav1.CreateMachineTunnelRequest]) (*connect.Response[arcav1.CreateMachineTunnelResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (s *tunnelConnectService) UpsertMachineExposure(ctx context.Context, req *connect.Request[arcav1.UpsertMachineExposureRequest]) (*connect.Response[arcav1.UpsertMachineExposureResponse], error) {
	if s.store == nil || s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
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

	existing, err := s.store.GetMachineExposureByMachineIDAndName(ctx, machineID, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("exposure is not provisioned yet"))
		}
		log.Printf("load machine exposure failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load exposure"))
	}

	exposure, err := s.store.UpsertMachineExposure(
		ctx,
		existing.MachineID,
		existing.Name,
		existing.Hostname,
		existing.Service,
	)
	if err != nil {
		log.Printf("upsert machine exposure failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist exposure settings"))
	}

	return connect.NewResponse(&arcav1.UpsertMachineExposureResponse{Exposure: toMachineExposureMessage(exposure)}), nil
}

func (s *tunnelConnectService) ListMachineExposures(ctx context.Context, req *connect.Request[arcav1.ListMachineExposuresRequest]) (*connect.Response[arcav1.ListMachineExposuresResponse], error) {
	if s.store == nil || s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
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

	exposures, err := s.store.ListMachineExposuresByMachineID(ctx, machineID)
	if err != nil {
		log.Printf("list machine exposures failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list exposures"))
	}

	items := make([]*arcav1.MachineExposure, 0, len(exposures))
	for _, exposure := range exposures {
		items = append(items, toMachineExposureMessage(exposure))
	}
	return connect.NewResponse(&arcav1.ListMachineExposuresResponse{Exposures: items}), nil
}

func (s *tunnelConnectService) GetMachineExposureByHostname(ctx context.Context, req *connect.Request[arcav1.GetMachineExposureByHostnameRequest]) (*connect.Response[arcav1.GetMachineExposureByHostnameResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
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
	exposure, err := s.store.GetMachineExposureByHostname(ctx, hostname)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("exposure not found"))
		}
		log.Printf("get exposure by hostname failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve exposure"))
	}
	if exposure.MachineID != machineID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("exposure not found"))
	}

	return connect.NewResponse(&arcav1.GetMachineExposureByHostnameResponse{Exposure: toMachineExposureMessage(exposure)}), nil
}

func (s *tunnelConnectService) ReportMachineReadiness(ctx context.Context, req *connect.Request[arcav1.ReportMachineReadinessRequest]) (*connect.Response[arcav1.ReportMachineReadinessResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
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

func (s *tunnelConnectService) GetMachineLLMModels(ctx context.Context, req *connect.Request[arcav1.GetMachineLLMModelsRequest]) (*connect.Response[arcav1.GetMachineLLMModelsResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("tunnel service unavailable"))
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

	models, err := s.store.ListUserLLMModelsWithAPIKey(ctx, ownerUserID)
	if err != nil {
		log.Printf("list user llm models failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list LLM models"))
	}

	items := make([]*arcav1.MachineLLMModel, 0, len(models))
	for _, m := range models {
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
			ConfigName:       m.ConfigName,
			EndpointType:     m.EndpointType,
			CustomEndpoint:   m.CustomEndpoint,
			ModelName:        m.ModelName,
			ApiKey:           apiKey,
			MaxContextTokens: int32(m.MaxContextTokens),
		})
	}

	return connect.NewResponse(&arcav1.GetMachineLLMModelsResponse{Models: items}), nil
}

func toMachineExposureMessage(exposure db.MachineExposure) *arcav1.MachineExposure {
	return &arcav1.MachineExposure{
		Id:        exposure.ID,
		MachineId: exposure.MachineID,
		Name:      exposure.Name,
		Hostname:  exposure.Hostname,
		Service:   exposure.Service,
	}
}

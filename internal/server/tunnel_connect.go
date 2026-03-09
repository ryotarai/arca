package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type tunnelConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newTunnelConnectService(store *db.Store, authenticator Authenticator) *tunnelConnectService {
	return &tunnelConnectService{store: store, authenticator: authenticator}
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
	if _, err := s.store.GetMachineByIDForUser(ctx, userID, machineID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("authorize machine for exposure update failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
	}

	existing, err := s.store.GetMachineExposureByMachineIDAndName(ctx, machineID, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("exposure is not provisioned yet"))
		}
		log.Printf("load machine exposure failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load exposure"))
	}

	visibility := visibilityFromRequest(req.Msg)
	if visibility == db.EndpointVisibilityInternetPublic {
		setup, err := s.store.GetSetupState(ctx)
		if err != nil {
			log.Printf("load setup state failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to evaluate exposure policy"))
		}
		if setup.InternetPublicExposureDisabled {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("internet public visibility is disabled by admin policy"))
		}
	}

	selectedUserIDs := normalizeSelectedUserIDs(req.Msg.GetSelectedUserIds(), userID)
	if visibility != db.EndpointVisibilitySelectedUsers {
		selectedUserIDs = nil
	}

	exposure, err := s.store.UpsertMachineExposure(
		ctx,
		existing.MachineID,
		existing.Name,
		existing.Hostname,
		existing.Service,
		visibility,
		selectedUserIDs,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("selected user ids include unknown users"))
		}
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
	if _, err := s.store.GetMachineByIDForUser(ctx, userID, machineID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("authorize machine for exposure listing failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
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
	)
	if err != nil {
		log.Printf("report machine readiness failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to report machine readiness"))
	}

	return connect.NewResponse(&arcav1.ReportMachineReadinessResponse{Accepted: updated}), nil
}

func visibilityFromRequest(req *arcav1.UpsertMachineExposureRequest) string {
	switch req.GetVisibility() {
	case arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_SELECTED_USERS:
		return db.EndpointVisibilitySelectedUsers
	case arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_ALL_ARCA_USERS:
		return db.EndpointVisibilityAllArcaUsers
	case arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_INTERNET_PUBLIC:
		return db.EndpointVisibilityInternetPublic
	case arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_OWNER_ONLY:
		return db.EndpointVisibilityOwnerOnly
	default:
		if req.GetPublic() {
			return db.EndpointVisibilityInternetPublic
		}
		return db.EndpointVisibilityOwnerOnly
	}
}

func normalizeSelectedUserIDs(ids []string, ownerUserID string) []string {
	result := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || slices.Contains(result, trimmed) {
			continue
		}
		result = append(result, trimmed)
	}
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID != "" && !slices.Contains(result, ownerUserID) {
		result = append(result, ownerUserID)
	}
	return result
}

func visibilityToProto(visibility string) arcav1.EndpointVisibility {
	switch db.NormalizeEndpointVisibility(visibility) {
	case db.EndpointVisibilitySelectedUsers:
		return arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_SELECTED_USERS
	case db.EndpointVisibilityAllArcaUsers:
		return arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_ALL_ARCA_USERS
	case db.EndpointVisibilityInternetPublic:
		return arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_INTERNET_PUBLIC
	default:
		return arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_OWNER_ONLY
	}
}

func toMachineExposureMessage(exposure db.MachineExposure) *arcav1.MachineExposure {
	return &arcav1.MachineExposure{
		Id:              exposure.ID,
		MachineId:       exposure.MachineID,
		Name:            exposure.Name,
		Hostname:        exposure.Hostname,
		Service:         exposure.Service,
		Public:          db.IsInternetPublicVisibility(exposure.Visibility),
		Visibility:      visibilityToProto(exposure.Visibility),
		SelectedUserIds: exposure.SelectedUserIDs,
	}
}

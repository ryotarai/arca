package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type tunnelConnectService struct {
	store *db.Store
}

func newTunnelConnectService(store *db.Store) *tunnelConnectService {
	return &tunnelConnectService{store: store}
}

func (s *tunnelConnectService) CreateMachineTunnel(context.Context, *connect.Request[arcav1.CreateMachineTunnelRequest]) (*connect.Response[arcav1.CreateMachineTunnelResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (s *tunnelConnectService) UpsertMachineExposure(context.Context, *connect.Request[arcav1.UpsertMachineExposureRequest]) (*connect.Response[arcav1.UpsertMachineExposureResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (s *tunnelConnectService) ListMachineExposures(context.Context, *connect.Request[arcav1.ListMachineExposuresRequest]) (*connect.Response[arcav1.ListMachineExposuresResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))
}

func (s *tunnelConnectService) GetMachineExposureByHostname(ctx context.Context, req *connect.Request[arcav1.GetMachineExposureByHostnameRequest]) (*connect.Response[arcav1.GetMachineExposureByHostnameResponse], error) {
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

	return connect.NewResponse(&arcav1.GetMachineExposureByHostnameResponse{
		Exposure: &arcav1.MachineExposure{
			Id:        exposure.ID,
			MachineId: exposure.MachineID,
			Name:      exposure.Name,
			Hostname:  exposure.Hostname,
			Service:   exposure.Service,
			Public:    exposure.IsPublic,
		},
	}), nil
}

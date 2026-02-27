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
)

type machineConnectService struct {
	authenticator Authenticator
	store         MachineStore
}

func newMachineConnectService(authenticator Authenticator, store MachineStore) *machineConnectService {
	return &machineConnectService{authenticator: authenticator, store: store}
}

func (s *machineConnectService) ListMachines(ctx context.Context, req *connect.Request[arcav1.ListMachinesRequest]) (*connect.Response[arcav1.ListMachinesResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	machines, err := s.store.ListMachinesByUser(ctx, userID)
	if err != nil {
		log.Printf("list machines failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list machines"))
	}

	items := make([]*arcav1.Machine, 0, len(machines))
	for _, machine := range machines {
		items = append(items, toMachineMessage(machine))
	}

	return connect.NewResponse(&arcav1.ListMachinesResponse{Machines: items}), nil
}

func (s *machineConnectService) GetMachine(ctx context.Context, req *connect.Request[arcav1.GetMachineRequest]) (*connect.Response[arcav1.GetMachineResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("get machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}

	return connect.NewResponse(&arcav1.GetMachineResponse{Machine: toMachineMessage(machine)}), nil
}

func (s *machineConnectService) CreateMachine(ctx context.Context, req *connect.Request[arcav1.CreateMachineRequest]) (*connect.Response[arcav1.CreateMachineResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	machine, err := s.store.CreateMachineWithOwner(ctx, userID, name)
	if err != nil {
		log.Printf("create machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create machine"))
	}

	return connect.NewResponse(&arcav1.CreateMachineResponse{Machine: toMachineMessage(machine)}), nil
}

func (s *machineConnectService) UpdateMachine(ctx context.Context, req *connect.Request[arcav1.UpdateMachineRequest]) (*connect.Response[arcav1.UpdateMachineResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	updated, err := s.store.UpdateMachineNameByIDForOwner(ctx, userID, machineID, name)
	if err != nil {
		log.Printf("update machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update machine"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("fetch updated machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch machine"))
	}

	return connect.NewResponse(&arcav1.UpdateMachineResponse{Machine: toMachineMessage(machine)}), nil
}

func (s *machineConnectService) StartMachine(ctx context.Context, req *connect.Request[arcav1.StartMachineRequest]) (*connect.Response[arcav1.StartMachineResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	updated, err := s.store.RequestStartMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		log.Printf("start machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to start machine"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("fetch started machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch machine"))
	}

	return connect.NewResponse(&arcav1.StartMachineResponse{Machine: toMachineMessage(machine)}), nil
}

func (s *machineConnectService) StopMachine(ctx context.Context, req *connect.Request[arcav1.StopMachineRequest]) (*connect.Response[arcav1.StopMachineResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	updated, err := s.store.RequestStopMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		log.Printf("stop machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to stop machine"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("fetch stopped machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch machine"))
	}

	return connect.NewResponse(&arcav1.StopMachineResponse{Machine: toMachineMessage(machine)}), nil
}

func (s *machineConnectService) DeleteMachine(ctx context.Context, req *connect.Request[arcav1.DeleteMachineRequest]) (*connect.Response[arcav1.DeleteMachineResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	deleted, err := s.store.DeleteMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		log.Printf("delete machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete machine"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	return connect.NewResponse(&arcav1.DeleteMachineResponse{}), nil
}

func (s *machineConnectService) authenticate(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("auth unavailable"))
	}
	if s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("machine store unavailable"))
	}

	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	userID, _, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return userID, nil
}

func toMachineMessage(machine db.Machine) *arcav1.Machine {
	return &arcav1.Machine{
		Id:            machine.ID,
		Name:          machine.Name,
		Status:        machine.Status,
		DesiredStatus: machine.DesiredStatus,
		LastError:     machine.LastError,
	}
}

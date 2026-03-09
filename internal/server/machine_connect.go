package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/cloudflare"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type machineConnectService struct {
	authenticator Authenticator
	store         MachineStore
}

var machineNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

var reservedMachineNames = []string{"admin", "console", "dash", "api", "system"}

func newMachineConnectService(authenticator Authenticator, store MachineStore, _ *cloudflare.Client) *machineConnectService {
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
	if err := validateMachineName(name); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	runtimeID, err := s.resolveCreateRuntimeID(ctx, req.Msg.GetRuntimeId())
	if err != nil {
		return nil, err
	}
	machine, err := s.store.CreateMachineWithOwner(ctx, userID, name, runtimeID, currentSetupVersion())
	if err != nil {
		if errors.Is(err, db.ErrMachineNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("machine name already exists"))
		}
		log.Printf("create machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create machine"))
	}

	return connect.NewResponse(&arcav1.CreateMachineResponse{Machine: toMachineMessage(machine), MachineToken: machine.MachineToken}), nil
}

func (s *machineConnectService) UpdateMachine(ctx context.Context, req *connect.Request[arcav1.UpdateMachineRequest]) (*connect.Response[arcav1.UpdateMachineResponse], error) {
	return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("machine name cannot be changed"))
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

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("get machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}

	runtimeID, err := s.resolveStartRuntimeID(ctx, machine, req.Msg.GetRuntimeId())
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(runtimeID) != strings.TrimSpace(machine.RuntimeID) {
		runtimeUpdated, updateErr := s.store.UpdateMachineRuntimeByIDForOwner(ctx, userID, machineID, runtimeID, currentSetupVersion())
		if updateErr != nil {
			log.Printf("set machine runtime id failed: %v", updateErr)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to select runtime"))
		}
		if !runtimeUpdated {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
	}

	updated, err := s.store.RequestStartMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		log.Printf("start machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to start machine"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	machine, err = s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("fetch started machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch machine"))
	}

	return connect.NewResponse(&arcav1.StartMachineResponse{Machine: toMachineMessage(machine)}), nil
}

func (s *machineConnectService) resolveCreateRuntimeID(ctx context.Context, requestedRuntimeID string) (string, error) {
	runtimeID := strings.TrimSpace(requestedRuntimeID)
	if runtimeID == "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("runtime id is required"))
	}

	if err := s.validateRuntimeExists(ctx, runtimeID); err != nil {
		return "", err
	}

	return runtimeID, nil
}

func (s *machineConnectService) resolveStartRuntimeID(ctx context.Context, machine db.Machine, requestedRuntimeID string) (string, error) {
	runtimeID := strings.TrimSpace(requestedRuntimeID)
	if runtimeID == "" {
		runtimeID = strings.TrimSpace(machine.RuntimeID)
	}
	if runtimeID == "" {
		return "", connect.NewError(connect.CodeFailedPrecondition, errors.New("machine runtime is not configured"))
	}

	if err := s.validateRuntimeExists(ctx, runtimeID); err != nil {
		return "", err
	}

	return runtimeID, nil
}

func (s *machineConnectService) validateRuntimeExists(ctx context.Context, runtimeID string) error {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("runtime id is required"))
	}

	_, err := s.store.GetRuntimeByID(ctx, runtimeID)
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("runtime not found"))
	}

	log.Printf("get runtime failed: %v", err)
	return connect.NewError(connect.CodeInternal, errors.New("failed to resolve runtime"))
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

	requested, err := s.store.RequestDeleteMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		log.Printf("request machine delete failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to request machine deletion"))
	}
	if !requested {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	return connect.NewResponse(&arcav1.DeleteMachineResponse{}), nil
}

func (s *machineConnectService) ListMachineEvents(ctx context.Context, req *connect.Request[arcav1.ListMachineEventsRequest]) (*connect.Response[arcav1.ListMachineEventsResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	limit := int64(req.Msg.GetLimit())
	events, err := s.store.ListMachineEventsByMachineIDForUser(ctx, userID, machineID, limit)
	if err != nil {
		log.Printf("list machine events failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list machine events"))
	}

	items := make([]*arcav1.MachineEvent, 0, len(events))
	for _, event := range events {
		items = append(items, toMachineEventMessage(event))
	}

	return connect.NewResponse(&arcav1.ListMachineEventsResponse{Events: items}), nil
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

	userID, _, _, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return userID, nil
}

func toMachineMessage(machine db.Machine) *arcav1.Machine {
	return &arcav1.Machine{
		Id:              machine.ID,
		Name:            machine.Name,
		Status:          machine.Status,
		DesiredStatus:   machine.DesiredStatus,
		LastError:       machine.LastError,
		Endpoint:        machine.Endpoint,
		RuntimeId:       machine.RuntimeID,
		UpdateRequired:  machineUpdateRequired(machine),
		Ready:           machine.Ready,
		ReadyReportedAt: machine.ReadyReportedAt,
	}
}

func toMachineEventMessage(event db.MachineEvent) *arcav1.MachineEvent {
	return &arcav1.MachineEvent{
		Id:        event.ID,
		MachineId: event.MachineID,
		JobId:     event.JobID,
		Level:     event.Level,
		EventType: event.EventType,
		Message:   event.Message,
		CreatedAt: event.CreatedAt,
	}
}

func validateMachineName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) < 3 {
		return errors.New("name must be at least 3 characters")
	}
	if len(name) > 63 {
		return errors.New("name must be at most 63 characters")
	}
	if slices.Contains(reservedMachineNames, name) {
		return errors.New("name is reserved")
	}
	if strings.HasPrefix(name, "arca-") {
		return errors.New("name cannot start with arca-")
	}
	if !machineNamePattern.MatchString(name) {
		return errors.New("name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen")
	}
	return nil
}

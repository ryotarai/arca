package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type machineConnectService struct {
	authenticator Authenticator
	store         MachineStore
	dbStore       *db.Store
}

var machineNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

var reservedMachineNames = []string{
	// Sorted alphabetically.
	"admin", "alerts", "api", "app", "artifacts", "assets", "auth",
	"blog", "cas", "cd", "cdn", "ci", "cluster", "console",
	"dash", "demo", "dev", "dns", "docs",
	"faq", "files", "ftp",
	"gateway", "git", "gitlab", "grafana",
	"health", "help",
	"imap", "internal",
	"lb", "localhost", "login",
	"mail", "media", "mobile", "monitoring",
	"node", "ns1", "ns2",
	"oauth",
	"preview", "private", "prod", "prometheus", "proxy", "public",
	"registry", "relay",
	"sandbox", "server", "smtp", "sso", "staging", "static", "status", "support", "system",
	"test",
	"upload",
	"vpn",
	"web", "wiki", "ws", "www",
}

func newMachineConnectService(authenticator Authenticator, store MachineStore, dbStore *db.Store) *machineConnectService {
	return &machineConnectService{authenticator: authenticator, store: store, dbStore: dbStore}
}

func (s *machineConnectService) ListMachines(ctx context.Context, req *connect.Request[arcav1.ListMachinesRequest]) (*connect.Response[arcav1.ListMachinesResponse], error) {
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	isAdmin := authResult.Role == "admin"

	machines, err := s.store.ListMachinesByUser(ctx, authResult.UserID)
	if err != nil {
		log.Printf("list machines failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list machines"))
	}

	items := make([]*arcav1.Machine, 0, len(machines))
	for _, machine := range machines {
		items = append(items, toMachineMessageWithAdmin(machine, isAdmin))
	}

	return connect.NewResponse(&arcav1.ListMachinesResponse{Machines: items}), nil
}

func (s *machineConnectService) GetMachine(ctx context.Context, req *connect.Request[arcav1.GetMachineRequest]) (*connect.Response[arcav1.GetMachineResponse], error) {
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	userID := authResult.UserID
	isAdmin := authResult.Role == "admin"

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	role := s.resolveMachineRole(ctx, userID, machineID)
	if role == db.MachineRoleNone {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// User has general access but no user_machines row — fetch directly
			if s.dbStore != nil {
				m, mErr := s.dbStore.GetMachineByID(ctx, machineID)
				if mErr == nil {
					m.UserRole = role
					return connect.NewResponse(&arcav1.GetMachineResponse{Machine: toMachineMessageWithAdmin(m, isAdmin)}), nil
				}
			}
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("get machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}
	machine.UserRole = role

	return connect.NewResponse(&arcav1.GetMachineResponse{Machine: toMachineMessageWithAdmin(machine, isAdmin)}), nil
}

func (s *machineConnectService) CreateMachine(ctx context.Context, req *connect.Request[arcav1.CreateMachineRequest]) (*connect.Response[arcav1.CreateMachineResponse], error) {
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	userID := authResult.UserID

	name := strings.TrimSpace(req.Msg.GetName())
	if err := validateMachineName(name); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	runtimeID, err := s.resolveCreateRuntimeID(ctx, req.Msg.GetRuntimeId())
	if err != nil {
		return nil, err
	}

	// Snapshot runtime config at creation time
	runtime, err := s.store.GetRuntimeByID(ctx, runtimeID)
	if err != nil {
		log.Printf("get runtime for snapshot failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve runtime"))
	}

	options := req.Msg.GetOptions()
	optionsJSON := "{}"
	if len(options) > 0 {
		if machineType := strings.TrimSpace(options["machine_type"]); machineType != "" {
			if err := s.validateMachineType(ctx, runtimeID, machineType); err != nil {
				return nil, err
			}
		}
		data, jsonErr := json.Marshal(options)
		if jsonErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid options"))
		}
		optionsJSON = string(data)
	}

	customImageID := strings.TrimSpace(req.Msg.GetCustomImageId())
	if customImageID != "" {
		if err := s.validateCustomImage(ctx, runtimeID, customImageID); err != nil {
			return nil, err
		}
		// Inject custom image data into options so runtimes can resolve images
		optionsJSON, err = s.injectCustomImageOptions(ctx, customImageID, optionsJSON)
		if err != nil {
			return nil, err
		}
	}

	machine, err := s.store.CreateMachineWithOwner(ctx, userID, name, runtimeID, currentSetupVersion(), optionsJSON, customImageID, runtime.Type, runtime.ConfigJSON)
	if err != nil {
		if errors.Is(err, db.ErrMachineNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("machine name already exists"))
		}
		log.Printf("create machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create machine"))
	}

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.create", "machine", machine.ID, fmt.Sprintf(`{"name":%q}`, name))

	isAdmin := authResult.Role == "admin"
	return connect.NewResponse(&arcav1.CreateMachineResponse{Machine: toMachineMessageWithAdmin(machine, isAdmin), MachineToken: machine.MachineToken}), nil
}

func (s *machineConnectService) UpdateMachine(ctx context.Context, req *connect.Request[arcav1.UpdateMachineRequest]) (*connect.Response[arcav1.UpdateMachineResponse], error) {
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	userID := authResult.UserID

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	if role := s.resolveMachineRole(ctx, userID, machineID); role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	options := req.Msg.GetOptions()
	if len(options) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("options are required"))
	}

	// Verify machine is stopped
	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("get machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}

	if machine.Status != db.MachineStatusStopped {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("machine must be stopped to update options"))
	}

	// Validate machine_type if provided
	if machineType := strings.TrimSpace(options["machine_type"]); machineType != "" {
		if err := s.validateMachineType(ctx, machine.RuntimeID, machineType); err != nil {
			return nil, err
		}
	}

	// Merge options with existing
	existing := parseMachineOptions(machine.OptionsJSON)
	for k, v := range options {
		existing[k] = v
	}

	data, jsonErr := json.Marshal(existing)
	if jsonErr != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encode options"))
	}

	if s.dbStore == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("store unavailable"))
	}
	if _, err := s.dbStore.UpdateMachineOptionsByID(ctx, machineID, string(data)); err != nil {
		log.Printf("update machine options failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update machine options"))
	}

	updated, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		log.Printf("fetch updated machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch machine"))
	}
	updated.UserRole = db.MachineRoleAdmin

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.update", "machine", machineID, fmt.Sprintf(`{"name":%q}`, updated.Name))

	return connect.NewResponse(&arcav1.UpdateMachineResponse{Machine: toMachineMessage(updated)}), nil
}

func (s *machineConnectService) StartMachine(ctx context.Context, req *connect.Request[arcav1.StartMachineRequest]) (*connect.Response[arcav1.StartMachineResponse], error) {
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	userID := authResult.UserID

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	if role := s.resolveMachineRole(ctx, userID, machineID); role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
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
		// Re-snapshot runtime config when switching runtimes
		rt, rtErr := s.store.GetRuntimeByID(ctx, runtimeID)
		if rtErr != nil {
			log.Printf("get runtime for snapshot failed: %v", rtErr)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve runtime"))
		}
		runtimeUpdated, updateErr := s.store.UpdateMachineRuntimeByIDForOwner(ctx, userID, machineID, runtimeID, currentSetupVersion(), rt.Type, rt.ConfigJSON)
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

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.start", "machine", machineID, "{}")

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
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	userID := authResult.UserID

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	if role := s.resolveMachineRole(ctx, userID, machineID); role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
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

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.stop", "machine", machineID, "{}")

	return connect.NewResponse(&arcav1.StopMachineResponse{Machine: toMachineMessage(machine)}), nil
}

func (s *machineConnectService) DeleteMachine(ctx context.Context, req *connect.Request[arcav1.DeleteMachineRequest]) (*connect.Response[arcav1.DeleteMachineResponse], error) {
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	userID := authResult.UserID

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	if role := s.resolveMachineRole(ctx, userID, machineID); role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	requested, err := s.store.RequestDeleteMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		log.Printf("request machine delete failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to request machine deletion"))
	}
	if !requested {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.delete", "machine", machineID, "{}")

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

	role := s.resolveMachineRole(ctx, userID, machineID)
	if role == db.MachineRoleNone {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	// Admin-only: viewers cannot see events per design
	if role != db.MachineRoleAdmin {
		return connect.NewResponse(&arcav1.ListMachineEventsResponse{Events: nil}), nil
	}

	limit := int64(req.Msg.GetLimit())
	events, err := s.dbStore.ListMachineEventsByMachineID(ctx, machineID, limit)
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
	result, err := s.authenticateWithResult(ctx, header)
	if err != nil {
		return "", err
	}
	return result.UserID, nil
}

func (s *machineConnectService) authenticateWithResult(ctx context.Context, header http.Header) (auth.AuthResult, error) {
	if s.authenticator == nil {
		return auth.AuthResult{}, connect.NewError(connect.CodeUnavailable, errors.New("auth unavailable"))
	}
	if s.store == nil {
		return auth.AuthResult{}, connect.NewError(connect.CodeUnavailable, errors.New("machine store unavailable"))
	}

	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		result, err := s.authenticator.AuthenticateFull(ctx, sessionToken)
		if err == nil {
			return result, nil
		}
	}

	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, email, role, err := s.authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			return auth.AuthResult{UserID: userID, Email: email, Role: role}, nil
		}
	}

	return auth.AuthResult{}, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

func (s *machineConnectService) resolveMachineRole(ctx context.Context, userID, machineID string) string {
	if s.dbStore != nil {
		return s.dbStore.ResolveMachineRole(ctx, userID, machineID)
	}
	// Fallback for tests without dbStore: check via GetMachineByIDForUser
	if _, err := s.store.GetMachineByIDForUser(ctx, userID, machineID); err == nil {
		return db.MachineRoleAdmin
	}
	return db.MachineRoleNone
}

func toMachineMessage(machine db.Machine) *arcav1.Machine {
	return toMachineMessageWithAdmin(machine, false)
}

func toMachineMessageWithAdmin(machine db.Machine, includeConfig bool) *arcav1.Machine {
	msg := &arcav1.Machine{
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
		UserRole:        machine.UserRole,
		ArcadVersion:    machine.ArcadVersion,
		Options:         parseMachineOptions(machine.OptionsJSON),
		RuntimeType:     machine.RuntimeType,
	}
	if includeConfig {
		msg.RuntimeConfigJson = machine.RuntimeConfigJSON
	}
	return msg
}

func parseMachineOptions(optionsJSON string) map[string]string {
	optionsJSON = strings.TrimSpace(optionsJSON)
	if optionsJSON == "" || optionsJSON == "{}" {
		return nil
	}
	var opts map[string]string
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		return nil
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

func (s *machineConnectService) injectCustomImageOptions(ctx context.Context, customImageID, optionsJSON string) (string, error) {
	img, err := s.dbStore.GetCustomImage(ctx, customImageID)
	if err != nil {
		log.Printf("get custom image for options injection failed: %v", err)
		return "", connect.NewError(connect.CodeInternal, errors.New("failed to resolve custom image"))
	}

	var imgData map[string]string
	if err := json.Unmarshal([]byte(img.DataJSON), &imgData); err != nil {
		return "", connect.NewError(connect.CodeInternal, errors.New("failed to parse custom image data"))
	}

	opts := make(map[string]string)
	if optionsJSON != "" && optionsJSON != "{}" {
		_ = json.Unmarshal([]byte(optionsJSON), &opts)
	}

	// Inject custom image data with prefix to avoid conflicts
	for k, v := range imgData {
		opts["custom_image_"+k] = v
	}

	data, err := json.Marshal(opts)
	if err != nil {
		return "", connect.NewError(connect.CodeInternal, errors.New("failed to encode options"))
	}
	return string(data), nil
}

func (s *machineConnectService) validateCustomImage(ctx context.Context, runtimeID, customImageID string) error {
	if s.dbStore == nil {
		return connect.NewError(connect.CodeInternal, errors.New("store unavailable"))
	}

	img, err := s.dbStore.GetCustomImage(ctx, customImageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("custom image not found"))
		}
		log.Printf("get custom image failed: %v", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to resolve custom image"))
	}

	// Verify image is associated with the specified runtime
	runtimeIDs, err := s.dbStore.ListRuntimeIDsByCustomImageID(ctx, customImageID)
	if err != nil {
		log.Printf("list runtime IDs for image failed: %v", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to verify image association"))
	}
	found := false
	for _, rid := range runtimeIDs {
		if rid == runtimeID {
			found = true
			break
		}
	}
	if !found {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("custom image is not associated with the specified runtime"))
	}

	// Verify runtime type matches
	runtime, err := s.store.GetRuntimeByID(ctx, runtimeID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, errors.New("failed to resolve runtime"))
	}
	if strings.ToLower(runtime.Type) != strings.ToLower(img.RuntimeType) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("custom image runtime type does not match runtime"))
	}

	return nil
}

func (s *machineConnectService) validateMachineType(ctx context.Context, runtimeID, machineType string) error {
	runtime, err := s.store.GetRuntimeByID(ctx, runtimeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("runtime not found"))
		}
		log.Printf("get runtime failed: %v", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to resolve runtime"))
	}

	config := &arcav1.RuntimeConfig{}
	if err := protojson.Unmarshal([]byte(runtime.ConfigJSON), config); err != nil {
		log.Printf("decode runtime config failed: %v", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to decode runtime config"))
	}

	gce := config.GetGce()
	if gce == nil {
		return nil // non-GCE runtime, no validation needed
	}

	allowed := gce.GetAllowedMachineTypes()
	if len(allowed) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("runtime has no allowed machine types configured"))
	}
	if !slices.Contains(allowed, machineType) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("machine type is not allowed for this runtime"))
	}
	return nil
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

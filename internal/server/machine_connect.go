package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"connectrpc.com/connect"

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
		slog.ErrorContext(ctx, "list machines failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list machines"))
	}

	// Batch-load tags for all machines
	var allTags map[string][]string
	if s.dbStore != nil {
		allTags, err = s.dbStore.ListAllMachineTags(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "list machine tags failed", "error", err)
			// Non-fatal: continue without tags
			allTags = nil
		}
	}

	items := make([]*arcav1.Machine, 0, len(machines))
	for _, machine := range machines {
		if allTags != nil {
			machine.Tags = allTags[machine.ID]
		}
		msg := toMachineMessageWithAdmin(machine, isAdmin)
		msg.RestartNeeded = s.computeRestartNeeded(ctx, machine)
		items = append(items, msg)
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
					s.populateMachineTags(ctx, &m)
					msg := toMachineMessageWithAdmin(m, isAdmin)
					msg.RestartNeeded = s.computeRestartNeeded(ctx, m)
					return connect.NewResponse(&arcav1.GetMachineResponse{Machine: msg}), nil
				}
			}
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		slog.ErrorContext(ctx, "get machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}
	machine.UserRole = role
	s.populateMachineTags(ctx, &machine)

	msg := toMachineMessageWithAdmin(machine, isAdmin)
	msg.RestartNeeded = s.computeRestartNeeded(ctx, machine)
	return connect.NewResponse(&arcav1.GetMachineResponse{Machine: msg}), nil
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

	profileID, err := s.resolveCreateProfileID(ctx, req.Msg.GetProfileId())
	if err != nil {
		return nil, err
	}

	// Snapshot profile config at creation time
	profile, err := s.store.GetMachineProfileByID(ctx, profileID)
	if err != nil {
		slog.ErrorContext(ctx, "get profile for snapshot failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve profile"))
	}

	// Extract infrastructure-only config (remove dynamic settings)
	infraConfigJSON, err := extractInfrastructureConfig(profile.ConfigJSON)
	if err != nil {
		slog.ErrorContext(ctx, "extract infrastructure config failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to process profile config"))
	}

	options := req.Msg.GetOptions()
	optionsJSON := "{}"
	if len(options) > 0 {
		if machineType := strings.TrimSpace(options["machine_type"]); machineType != "" {
			if err := s.validateMachineType(ctx, profileID, machineType); err != nil {
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
		if err := s.validateCustomImage(ctx, profileID, customImageID); err != nil {
			return nil, err
		}
		// Inject custom image data into options so profiles can resolve images
		optionsJSON, err = s.injectCustomImageOptions(ctx, customImageID, optionsJSON)
		if err != nil {
			return nil, err
		}
	}

	// Validate and normalize tags
	tags := req.Msg.GetTags()
	var normalizedTags []string
	if len(tags) > 0 {
		normalizedTags = make([]string, 0, len(tags))
		for _, t := range tags {
			normalizedTags = append(normalizedTags, strings.TrimSpace(t))
		}
		slices.Sort(normalizedTags)
		normalizedTags = slices.Compact(normalizedTags)
		if err := validateMachineTags(normalizedTags); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	machine, err := s.store.CreateMachineWithOwner(ctx, userID, name, profileID, currentSetupVersion(), optionsJSON, customImageID, profile.Type, infraConfigJSON, profile.BootConfigHash)
	if err != nil {
		if errors.Is(err, db.ErrMachineNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("machine name already exists"))
		}
		slog.ErrorContext(ctx, "create machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create machine"))
	}

	// Store tags after machine creation
	if len(normalizedTags) > 0 && s.dbStore != nil {
		if err := s.dbStore.SetMachineTags(ctx, machine.ID, normalizedTags); err != nil {
			slog.ErrorContext(ctx, "set machine tags on create failed", "error", err)
			// Non-fatal: machine was created successfully
		} else {
			machine.Tags = normalizedTags
		}
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
		slog.ErrorContext(ctx, "get machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}

	if machine.Status != db.MachineStatusStopped {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("machine must be stopped to update options"))
	}

	// Validate machine_type if provided
	if machineType := strings.TrimSpace(options["machine_type"]); machineType != "" {
		if err := s.validateMachineType(ctx, machine.ProfileID, machineType); err != nil {
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
		slog.ErrorContext(ctx, "update machine options failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update machine options"))
	}

	updated, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		slog.ErrorContext(ctx, "fetch updated machine failed", "error", err)
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
		slog.ErrorContext(ctx, "get machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}

	if machine.LockedOperation != "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("machine is locked by operation: %s", machine.LockedOperation))
	}

	updated, err := s.store.RequestStartMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		slog.ErrorContext(ctx, "start machine failed", "error", err)
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
		slog.ErrorContext(ctx, "fetch started machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch machine"))
	}

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.start", "machine", machineID, "{}")

	return connect.NewResponse(&arcav1.StartMachineResponse{Machine: toMachineMessage(machine)}), nil
}

func (s *machineConnectService) resolveCreateProfileID(ctx context.Context, requestedProfileID string) (string, error) {
	profileID := strings.TrimSpace(requestedProfileID)
	if profileID == "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("profile id is required"))
	}

	if err := s.validateProfileExists(ctx, profileID); err != nil {
		return "", err
	}

	return profileID, nil
}

func (s *machineConnectService) validateProfileExists(ctx context.Context, profileID string) error {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("profile id is required"))
	}

	_, err := s.store.GetMachineProfileByID(ctx, profileID)
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("profile not found"))
	}

	slog.ErrorContext(ctx, "get profile failed", "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("failed to resolve profile"))
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

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		slog.ErrorContext(ctx, "get machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}

	if machine.LockedOperation != "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("machine is locked by operation: %s", machine.LockedOperation))
	}

	updated, err := s.store.RequestStopMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		slog.ErrorContext(ctx, "stop machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to stop machine"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	machine, err = s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		slog.ErrorContext(ctx, "fetch stopped machine failed", "error", err)
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

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		slog.ErrorContext(ctx, "get machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}

	if machine.LockedOperation != "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("machine is locked by operation: %s", machine.LockedOperation))
	}

	requested, err := s.store.RequestDeleteMachineByIDForOwner(ctx, userID, machineID)
	if err != nil {
		slog.ErrorContext(ctx, "request machine delete failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to request machine deletion"))
	}
	if !requested {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
	}

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.delete", "machine", machineID, "{}")

	return connect.NewResponse(&arcav1.DeleteMachineResponse{}), nil
}

var imageNamePattern = regexp.MustCompile(`^[a-z](?:[-a-z0-9]*[a-z0-9])?$`)

func (s *machineConnectService) CreateImageFromMachine(ctx context.Context, req *connect.Request[arcav1.CreateImageFromMachineRequest]) (*connect.Response[arcav1.CreateImageFromMachineResponse], error) {
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	userID := authResult.UserID

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	// Must be admin or owner
	role := s.resolveMachineRole(ctx, userID, machineID)
	if role != db.MachineRoleAdmin && role != db.MachineRoleOwner {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin or owner access required"))
	}

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if s.dbStore != nil {
				m, mErr := s.dbStore.GetMachineByID(ctx, machineID)
				if mErr == nil {
					machine = m
				} else {
					return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
				}
			} else {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
			}
		} else {
			slog.ErrorContext(ctx, "get machine failed", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
		}
	}

	// Validate machine state: must be running with desired running and not locked
	if machine.Status != db.MachineStatusRunning {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("machine must be running"))
	}
	if machine.DesiredStatus != db.MachineDesiredRunning {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("machine desired status must be running"))
	}
	if machine.LockedOperation != "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("machine is locked by operation: %s", machine.LockedOperation))
	}

	// Validate image name
	imageName := strings.TrimSpace(req.Msg.GetName())
	if imageName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image name is required"))
	}
	if !imageNamePattern.MatchString(imageName) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image name must start with a lowercase letter, and contain only lowercase letters, digits, and hyphens"))
	}

	if s.dbStore == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("store unavailable"))
	}

	// Check image name uniqueness for the template type
	existingImages, err := s.dbStore.ListCustomImages(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list custom images failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check image name uniqueness"))
	}
	for _, img := range existingImages {
		if img.Name == imageName && strings.EqualFold(img.TemplateType, machine.TemplateType) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("image with this name already exists for this template type"))
		}
	}

	// Check no existing active create_image job for this machine
	hasActive, err := s.dbStore.HasActiveCreateImageJob(ctx, machineID)
	if err != nil {
		slog.ErrorContext(ctx, "check active create image job failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check active jobs"))
	}
	if hasActive {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("an image creation job is already in progress for this machine"))
	}

	// Set locked_operation = 'create_image'
	if err := s.dbStore.SetMachineLockedOperation(ctx, machineID, "create_image"); err != nil {
		slog.ErrorContext(ctx, "set machine locked operation failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to lock machine for image creation"))
	}

	// Build metadata JSON
	description := strings.TrimSpace(req.Msg.GetDescription())
	metadata := map[string]string{"image_name": imageName}
	if description != "" {
		metadata["image_description"] = description
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to encode metadata"))
	}

	// Enqueue create_image job
	jobID, err := s.dbStore.EnqueueCreateImageJob(ctx, machineID, description, string(metadataJSON))
	if err != nil {
		slog.ErrorContext(ctx, "enqueue create image job failed", "error", err)
		// Attempt to unlock the machine since job creation failed
		if unlockErr := s.dbStore.SetMachineLockedOperation(ctx, machineID, ""); unlockErr != nil {
			slog.ErrorContext(ctx, "failed to unlock machine after job enqueue failure", "error", unlockErr)
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to enqueue image creation job"))
	}

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.create_image", "machine", machineID,
		fmt.Sprintf(`{"image_name":%q,"job_id":%q}`, imageName, jobID))

	return connect.NewResponse(&arcav1.CreateImageFromMachineResponse{
		JobId: jobID,
	}), nil
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
		slog.ErrorContext(ctx, "list machine events failed", "error", err)
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

	return authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.dbStore, header)
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

func (s *machineConnectService) populateMachineTags(ctx context.Context, machine *db.Machine) {
	if s.dbStore == nil {
		return
	}
	tags, err := s.dbStore.ListMachineTagsByMachineID(ctx, machine.ID)
	if err != nil {
		slog.ErrorContext(ctx, "list machine tags failed", "error", err)
		return
	}
	machine.Tags = tags
}

var machineTagPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func validateMachineTags(tags []string) error {
	if len(tags) > 10 {
		return fmt.Errorf("too many tags (max 10)")
	}
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if len(tag) == 0 {
			return fmt.Errorf("tag must not be empty")
		}
		if len(tag) > 50 {
			return fmt.Errorf("tag %q is too long (max 50 characters)", tag)
		}
		if !machineTagPattern.MatchString(tag) {
			return fmt.Errorf("tag %q is invalid (must be lowercase alphanumeric with hyphens)", tag)
		}
		if seen[tag] {
			return fmt.Errorf("duplicate tag %q", tag)
		}
		seen[tag] = true
	}
	return nil
}

func (s *machineConnectService) UpdateMachineTags(ctx context.Context, req *connect.Request[arcav1.UpdateMachineTagsRequest]) (*connect.Response[arcav1.UpdateMachineTagsResponse], error) {
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

	if role := s.resolveMachineRole(ctx, userID, machineID); role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	tags := req.Msg.GetTags()
	// Normalize: deduplicate and sort
	normalized := make([]string, 0, len(tags))
	for _, t := range tags {
		normalized = append(normalized, strings.TrimSpace(t))
	}
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)

	if err := validateMachineTags(normalized); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if s.dbStore == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("store unavailable"))
	}

	if err := s.dbStore.SetMachineTags(ctx, machineID, normalized); err != nil {
		slog.ErrorContext(ctx, "set machine tags failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update tags"))
	}

	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if s.dbStore != nil {
				m, mErr := s.dbStore.GetMachineByID(ctx, machineID)
				if mErr == nil {
					m.Tags = normalized
					return connect.NewResponse(&arcav1.UpdateMachineTagsResponse{Machine: toMachineMessageWithAdmin(m, isAdmin)}), nil
				}
			}
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		slog.ErrorContext(ctx, "fetch machine after tag update failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch machine"))
	}
	machine.Tags = normalized

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.update_tags", "machine", machineID, "{}")

	return connect.NewResponse(&arcav1.UpdateMachineTagsResponse{Machine: toMachineMessageWithAdmin(machine, isAdmin)}), nil
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
		ProfileId:       machine.ProfileID,
		UpdateRequired:  machineUpdateRequired(machine),
		Ready:           machine.Ready,
		ReadyReportedAt: machine.ReadyReportedAt,
		UserRole:        machine.UserRole,
		ArcadVersion:    machine.ArcadVersion,
		Options:         parseMachineOptions(machine.OptionsJSON),
		ProviderType:    machine.ProviderType,
		Tags:            machine.Tags,
		LockedOperation: machine.LockedOperation,
	}
	if includeConfig {
		msg.InfrastructureConfigJson = machine.InfrastructureConfigJSON
	}
	return msg
}

// computeRestartNeeded checks whether a machine's applied boot config hash
// differs from its profile's current boot config hash. Returns true only
// when the machine is running and a restart is needed to pick up profile changes.
func (s *machineConnectService) computeRestartNeeded(ctx context.Context, machine db.Machine) bool {
	if machine.Status != db.MachineStatusRunning {
		return false
	}
	if s.dbStore == nil {
		return false
	}
	profileID := strings.TrimSpace(machine.ProfileID)
	if profileID == "" {
		return false
	}
	profile, err := s.dbStore.GetMachineProfileByID(ctx, profileID)
	if err != nil {
		return false
	}
	if profile.BootConfigHash == "" || machine.AppliedBootConfigHash == "" {
		return false
	}
	return machine.AppliedBootConfigHash != profile.BootConfigHash
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
		slog.ErrorContext(ctx, "get custom image for options injection failed", "error", err)
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

func (s *machineConnectService) validateCustomImage(ctx context.Context, profileID, customImageID string) error {
	if s.dbStore == nil {
		return connect.NewError(connect.CodeInternal, errors.New("store unavailable"))
	}

	img, err := s.dbStore.GetCustomImage(ctx, customImageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("custom image not found"))
		}
		slog.ErrorContext(ctx, "get custom image failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to resolve custom image"))
	}

	// Verify image is associated with the specified profile
	profileIDs, err := s.dbStore.ListTemplateIDsByCustomImageID(ctx, customImageID)
	if err != nil {
		slog.ErrorContext(ctx, "list profile IDs for image failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to verify image association"))
	}
	found := false
	for _, pid := range profileIDs {
		if pid == profileID {
			found = true
			break
		}
	}
	if !found {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("custom image is not associated with the specified profile"))
	}

	// Verify profile type matches
	profile, err := s.store.GetMachineProfileByID(ctx, profileID)
	if err != nil {
		return connect.NewError(connect.CodeInternal, errors.New("failed to resolve profile"))
	}
	if strings.ToLower(profile.Type) != strings.ToLower(img.ProviderType) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("custom image type does not match profile"))
	}

	return nil
}

func (s *machineConnectService) validateMachineType(ctx context.Context, profileID, machineType string) error {
	profile, err := s.store.GetMachineProfileByID(ctx, profileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("profile not found"))
		}
		slog.ErrorContext(ctx, "get profile failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to resolve profile"))
	}

	config, err := unmarshalProfileConfigJSON(profile.ConfigJSON)
	if err != nil {
		slog.ErrorContext(ctx, "decode profile config failed", "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("failed to decode profile config"))
	}

	gce := config.GetGce()
	if gce == nil {
		return nil // non-GCE profile, no validation needed
	}

	allowed := gce.GetAllowedMachineTypes()
	if len(allowed) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("profile has no allowed machine types configured"))
	}
	if !slices.Contains(allowed, machineType) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("machine type is not allowed for this profile"))
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

func (s *machineConnectService) ChangeMachineProfile(ctx context.Context, req *connect.Request[arcav1.ChangeMachineProfileRequest]) (*connect.Response[arcav1.ChangeMachineProfileResponse], error) {
	authResult, err := s.authenticateWithResult(ctx, req.Header())
	if err != nil {
		return nil, err
	}
	userID := authResult.UserID

	// Only admins can change a machine's profile
	if authResult.Role != db.UserRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admin can change machine profile"))
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	newProfileID := strings.TrimSpace(req.Msg.GetProfileId())
	if newProfileID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("profile id is required"))
	}

	if role := s.resolveMachineRole(ctx, userID, machineID); role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	// Verify machine is stopped
	machine, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		slog.ErrorContext(ctx, "get machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
	}
	if machine.Status != db.MachineStatusStopped {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("machine must be stopped to change profile"))
	}

	// Fetch new profile
	newProfile, err := s.store.GetMachineProfileByID(ctx, newProfileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("profile not found"))
		}
		slog.ErrorContext(ctx, "get new profile failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve profile"))
	}

	// Verify same provider type
	if machine.ProviderType != "" && machine.ProviderType != newProfile.Type {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot change provider type from %q to %q", machine.ProviderType, newProfile.Type))
	}

	// Verify machine_type is in the new profile's allowed list (for GCE)
	opts := parseMachineOptions(machine.OptionsJSON)
	if machineType := strings.TrimSpace(opts["machine_type"]); machineType != "" {
		if newProfile.Type == db.ProviderTypeGCE {
			config, cfgErr := unmarshalProfileConfigJSON(newProfile.ConfigJSON)
			if cfgErr == nil {
				if gce := config.GetGce(); gce != nil {
					allowed := gce.GetAllowedMachineTypes()
					if len(allowed) > 0 && !slices.Contains(allowed, machineType) {
						return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("machine type %q is not allowed in the new profile", machineType))
					}
				}
			}
		}
	}

	// Update only the machine's profile_id. The infrastructure_config_json
	// remains frozen from original creation — it is NOT updated when
	// changing profiles.
	if s.dbStore == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("store unavailable"))
	}

	if err := s.dbStore.UpdateMachineProfileID(ctx, machineID, newProfileID); err != nil {
		slog.ErrorContext(ctx, "update machine profile failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update machine profile"))
	}

	updated, err := s.store.GetMachineByIDForUser(ctx, userID, machineID)
	if err != nil {
		slog.ErrorContext(ctx, "fetch updated machine failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch machine"))
	}
	updated.UserRole = db.MachineRoleAdmin

	writeAuditLogFromAuth(ctx, s.dbStore, authResult, "machine.change_profile", "machine", machineID, fmt.Sprintf(`{"profile_id":%q}`, newProfileID))

	isAdmin := authResult.Role == "admin"
	return connect.NewResponse(&arcav1.ChangeMachineProfileResponse{Machine: toMachineMessageWithAdmin(updated, isAdmin)}), nil
}

// extractInfrastructureConfig removes dynamic/boot settings from a profile
// config JSON so only infrastructure fields remain. Stripped settings:
//   - Top-level: serverApiUrl, server_api_url, autoStopTimeoutSeconds, auto_stop_timeout_seconds, agentPrompt, agent_prompt
//   - Provider sub-objects: startup_script, startupScript (for libvirt, gce, lxd)
//
// This matches the migration (000048) that strips startup scripts from
// existing machines' infrastructure_config_json.
func extractInfrastructureConfig(configJSON string) (string, error) {
	if configJSON == "" || configJSON == "{}" {
		return configJSON, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		return configJSON, err
	}

	// Remove top-level dynamic settings
	delete(raw, "serverApiUrl")
	delete(raw, "server_api_url")
	delete(raw, "autoStopTimeoutSeconds")
	delete(raw, "auto_stop_timeout_seconds")
	delete(raw, "agentPrompt")
	delete(raw, "agent_prompt")

	// Remove startup_script / startupScript from each provider sub-object
	for _, provider := range []string{"libvirt", "gce", "lxd"} {
		providerRaw, ok := raw[provider]
		if !ok {
			continue
		}
		var providerConfig map[string]json.RawMessage
		if err := json.Unmarshal(providerRaw, &providerConfig); err != nil {
			continue
		}
		delete(providerConfig, "startup_script")
		delete(providerConfig, "startupScript")
		updated, err := json.Marshal(providerConfig)
		if err != nil {
			continue
		}
		raw[provider] = updated
	}

	result, err := json.Marshal(raw)
	if err != nil {
		return configJSON, err
	}
	return string(result), nil
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

package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type sharingConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newSharingConnectService(store *db.Store, authenticator Authenticator) *sharingConnectService {
	return &sharingConnectService{store: store, authenticator: authenticator}
}

func (s *sharingConnectService) GetMachineSharing(ctx context.Context, req *connect.Request[arcav1.GetMachineSharingRequest]) (*connect.Response[arcav1.GetMachineSharingResponse], error) {
	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	role := s.store.ResolveMachineRole(ctx, userID, machineID)
	if role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	members, err := s.store.ListUserMachinesByMachineID(ctx, machineID)
	if err != nil {
		log.Printf("list machine members failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list members"))
	}

	sharing, err := s.store.GetMachineSharing(ctx, machineID)
	if err != nil {
		// Default to none/none if no sharing row exists
		sharing = db.MachineSharing{
			GeneralAccessScope: db.GeneralAccessScopeNone,
			GeneralAccessRole:  db.GeneralAccessRoleNone,
		}
	}

	protoMembers := make([]*arcav1.MachineSharingMember, 0, len(members))
	for _, m := range members {
		protoMembers = append(protoMembers, &arcav1.MachineSharingMember{
			UserId: m.UserID,
			Email:  m.Email,
			Role:   m.Role,
		})
	}

	return connect.NewResponse(&arcav1.GetMachineSharingResponse{
		Members: protoMembers,
		GeneralAccess: &arcav1.GeneralAccess{
			Scope: sharing.GeneralAccessScope,
			Role:  sharing.GeneralAccessRole,
		},
	}), nil
}

func (s *sharingConnectService) UpdateMachineSharing(ctx context.Context, req *connect.Request[arcav1.UpdateMachineSharingRequest]) (*connect.Response[arcav1.UpdateMachineSharingResponse], error) {
	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	role := s.store.ResolveMachineRole(ctx, userID, machineID)
	if role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	// Update general access
	ga := req.Msg.GetGeneralAccess()
	if ga != nil {
		scope := strings.TrimSpace(ga.Scope)
		gaRole := strings.TrimSpace(ga.Role)
		if scope == "" {
			scope = db.GeneralAccessScopeNone
		}
		if gaRole == "" {
			gaRole = db.GeneralAccessRoleNone
		}
		if err := s.store.UpsertMachineSharing(ctx, machineID, db.MachineSharing{
			GeneralAccessScope: scope,
			GeneralAccessRole:  gaRole,
		}); err != nil {
			log.Printf("update machine sharing failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update sharing"))
		}
	}

	// Sync members: build desired state from request
	requestedMembers := req.Msg.GetMembers()
	// Get current members to detect removals
	currentMembers, err := s.store.ListUserMachinesByMachineID(ctx, machineID)
	if err != nil {
		log.Printf("list machine members for sync failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to sync members"))
	}

	// Build a set of requested user IDs
	requestedSet := make(map[string]string) // userID -> role
	for _, m := range requestedMembers {
		uid := strings.TrimSpace(m.UserId)
		r := strings.TrimSpace(m.Role)
		if r == "" {
			continue
		}
		// Resolve email to user ID if not provided
		if uid == "" {
			email := strings.TrimSpace(m.Email)
			if email == "" {
				continue
			}
			user, err := s.store.GetUserByEmail(ctx, email)
			if err != nil {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found: %s", email))
			}
			uid = user.ID
		}
		requestedSet[uid] = r
	}

	// Ensure the caller cannot remove themselves as admin
	if _, ok := requestedSet[userID]; !ok {
		requestedSet[userID] = db.MachineRoleAdmin
	}

	// Remove members not in request
	for _, current := range currentMembers {
		if _, ok := requestedSet[current.UserID]; !ok {
			if err := s.store.DeleteUserMachine(ctx, current.UserID, machineID); err != nil {
				log.Printf("remove machine member failed: %v", err)
			}
		}
	}

	// Upsert requested members
	for uid, r := range requestedSet {
		if err := s.store.UpsertUserMachine(ctx, uid, machineID, r); err != nil {
			log.Printf("upsert machine member failed: %v", err)
		}
	}

	// Read back
	updatedMembers, err := s.store.ListUserMachinesByMachineID(ctx, machineID)
	if err != nil {
		log.Printf("list updated machine members failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list updated members"))
	}

	updatedSharing, err := s.store.GetMachineSharing(ctx, machineID)
	if err != nil {
		updatedSharing = db.MachineSharing{
			GeneralAccessScope: db.GeneralAccessScopeNone,
			GeneralAccessRole:  db.GeneralAccessRoleNone,
		}
	}

	protoMembers := make([]*arcav1.MachineSharingMember, 0, len(updatedMembers))
	for _, m := range updatedMembers {
		protoMembers = append(protoMembers, &arcav1.MachineSharingMember{
			UserId: m.UserID,
			Email:  m.Email,
			Role:   m.Role,
		})
	}

	return connect.NewResponse(&arcav1.UpdateMachineSharingResponse{
		Members: protoMembers,
		GeneralAccess: &arcav1.GeneralAccess{
			Scope: updatedSharing.GeneralAccessScope,
			Role:  updatedSharing.GeneralAccessRole,
		},
	}), nil
}

func (s *sharingConnectService) RequestMachineAccess(ctx context.Context, req *connect.Request[arcav1.RequestMachineAccessRequest]) (*connect.Response[arcav1.RequestMachineAccessResponse], error) {
	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	// Verify machine exists
	if _, err := s.store.GetMachineByID(ctx, machineID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("request machine access: get machine failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check machine"))
	}

	// Verify user does not already have access
	role := s.store.ResolveMachineRole(ctx, userID, machineID)
	if role != db.MachineRoleNone {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("you already have access to this machine"))
	}

	// Check for existing pending request
	if _, err := s.store.GetPendingMachineAccessRequest(ctx, machineID, userID); err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("access request already pending"))
	}

	message := strings.TrimSpace(req.Msg.GetMessage())
	if err := s.store.CreateMachineAccessRequest(ctx, machineID, userID, db.MachineRoleViewer, message); err != nil {
		log.Printf("create access request failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create access request"))
	}

	return connect.NewResponse(&arcav1.RequestMachineAccessResponse{}), nil
}

func (s *sharingConnectService) ListMachineAccessRequests(ctx context.Context, req *connect.Request[arcav1.ListMachineAccessRequestsRequest]) (*connect.Response[arcav1.ListMachineAccessRequestsResponse], error) {
	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}

	role := s.store.ResolveMachineRole(ctx, userID, machineID)
	if role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	requests, err := s.store.ListPendingMachineAccessRequests(ctx, machineID)
	if err != nil {
		log.Printf("list access requests failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list access requests"))
	}

	protoRequests := make([]*arcav1.MachineAccessRequest, 0, len(requests))
	for _, r := range requests {
		protoRequests = append(protoRequests, &arcav1.MachineAccessRequest{
			Id:            r.ID,
			MachineId:     r.MachineID,
			UserId:        r.UserID,
			Email:         r.Email,
			Status:        r.Status,
			RequestedRole: r.RequestedRole,
			Message:       r.Message,
			CreatedAt:     r.CreatedAt,
		})
	}

	return connect.NewResponse(&arcav1.ListMachineAccessRequestsResponse{
		Requests: protoRequests,
	}), nil
}

func (s *sharingConnectService) ResolveMachineAccessRequest(ctx context.Context, req *connect.Request[arcav1.ResolveMachineAccessRequestRequest]) (*connect.Response[arcav1.ResolveMachineAccessRequestResponse], error) {
	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	requestID := strings.TrimSpace(req.Msg.GetRequestId())
	if requestID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("request id is required"))
	}

	action := strings.TrimSpace(req.Msg.GetAction())
	if action != "approve" && action != "deny" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("action must be 'approve' or 'deny', got %q", action))
	}

	// Fetch the access request
	accessReq, err := s.store.GetMachineAccessRequestByID(ctx, requestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("access request not found"))
		}
		log.Printf("get access request failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get access request"))
	}

	// Verify caller is admin of the machine
	role := s.store.ResolveMachineRole(ctx, userID, accessReq.MachineID)
	if role != db.MachineRoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("admin access required"))
	}

	resolvedRole := strings.TrimSpace(req.Msg.GetRole())
	if action == "approve" {
		if resolvedRole == "" {
			resolvedRole = db.MachineRoleViewer
		}
		if resolvedRole != db.MachineRoleViewer && resolvedRole != db.MachineRoleEditor {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("role must be 'viewer' or 'editor', got %q", resolvedRole))
		}

		// Grant access
		if err := s.store.UpsertUserMachine(ctx, accessReq.UserID, accessReq.MachineID, resolvedRole); err != nil {
			log.Printf("grant machine access failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to grant access"))
		}
	}

	status := "approved"
	if action == "deny" {
		status = "denied"
		resolvedRole = ""
	}

	updated, err := s.store.ResolveMachineAccessRequest(ctx, requestID, status, userID, resolvedRole)
	if err != nil {
		log.Printf("resolve access request failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve access request"))
	}
	if updated == 0 {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("access request not found or already resolved"))
	}

	return connect.NewResponse(&arcav1.ResolveMachineAccessRequestResponse{}), nil
}

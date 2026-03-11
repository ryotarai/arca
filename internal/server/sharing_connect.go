package server

import (
	"context"
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

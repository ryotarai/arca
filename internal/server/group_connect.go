package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type groupConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newGroupConnectService(store *db.Store, authenticator Authenticator) *groupConnectService {
	return &groupConnectService{store: store, authenticator: authenticator}
}

func (s *groupConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("group management unavailable"))
	}
	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		userID, _, role, err := s.authenticator.Authenticate(ctx, sessionToken)
		if err == nil {
			if role != db.UserRoleAdmin {
				return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage groups"))
			}
			return userID, nil
		}
	}

	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, _, role, err := s.authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			if role != db.UserRoleAdmin {
				return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage groups"))
			}
			return userID, nil
		}
	}

	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

func (s *groupConnectService) authenticateUser(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("group management unavailable"))
	}
	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		userID, _, _, err := s.authenticator.Authenticate(ctx, sessionToken)
		if err == nil {
			return userID, nil
		}
	}

	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, _, _, err := s.authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			return userID, nil
		}
	}

	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

func (s *groupConnectService) ListGroups(ctx context.Context, req *connect.Request[arcav1.ListGroupsRequest]) (*connect.Response[arcav1.ListGroupsResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	groups, err := s.store.ListUserGroups(ctx)
	if err != nil {
		log.Printf("list groups failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list groups"))
	}

	items := make([]*arcav1.UserGroup, 0, len(groups))
	for _, g := range groups {
		items = append(items, &arcav1.UserGroup{
			Id:          g.ID,
			Name:        g.Name,
			MemberCount: int32(g.MemberCount),
		})
	}
	return connect.NewResponse(&arcav1.ListGroupsResponse{Groups: items}), nil
}

func (s *groupConnectService) CreateGroup(ctx context.Context, req *connect.Request[arcav1.CreateGroupRequest]) (*connect.Response[arcav1.CreateGroupResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	id := uuid.New().String()
	if err := s.store.CreateUserGroup(ctx, id, name); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("group name already exists"))
		}
		log.Printf("create group failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create group"))
	}

	return connect.NewResponse(&arcav1.CreateGroupResponse{
		Group: &arcav1.UserGroup{
			Id:          id,
			Name:        name,
			MemberCount: 0,
		},
	}), nil
}

func (s *groupConnectService) DeleteGroup(ctx context.Context, req *connect.Request[arcav1.DeleteGroupRequest]) (*connect.Response[arcav1.DeleteGroupResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	groupID := strings.TrimSpace(req.Msg.GetGroupId())
	if groupID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("group id is required"))
	}

	deleted, err := s.store.DeleteUserGroup(ctx, groupID)
	if err != nil {
		log.Printf("delete group failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete group"))
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("group not found"))
	}

	return connect.NewResponse(&arcav1.DeleteGroupResponse{}), nil
}

func (s *groupConnectService) GetGroup(ctx context.Context, req *connect.Request[arcav1.GetGroupRequest]) (*connect.Response[arcav1.GetGroupResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	groupID := strings.TrimSpace(req.Msg.GetGroupId())
	if groupID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("group id is required"))
	}

	group, err := s.store.GetUserGroup(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("group not found"))
		}
		log.Printf("get group failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get group"))
	}

	members, err := s.store.ListUserGroupMembers(ctx, groupID)
	if err != nil {
		log.Printf("list group members failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list group members"))
	}

	protoMembers := make([]*arcav1.UserGroupMember, 0, len(members))
	for _, m := range members {
		protoMembers = append(protoMembers, &arcav1.UserGroupMember{
			UserId: m.UserID,
			Email:  m.Email,
		})
	}

	return connect.NewResponse(&arcav1.GetGroupResponse{
		Group: &arcav1.UserGroup{
			Id:          group.ID,
			Name:        group.Name,
			MemberCount: int32(len(members)),
		},
		Members: protoMembers,
	}), nil
}

func (s *groupConnectService) AddGroupMember(ctx context.Context, req *connect.Request[arcav1.AddGroupMemberRequest]) (*connect.Response[arcav1.AddGroupMemberResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	groupID := strings.TrimSpace(req.Msg.GetGroupId())
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if groupID == "" || userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("group id and user id are required"))
	}

	// Verify group exists
	if _, err := s.store.GetUserGroup(ctx, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("group not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check group"))
	}

	// Verify user exists
	if _, err := s.store.GetUserByID(ctx, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check user"))
	}

	if err := s.store.AddUserGroupMember(ctx, groupID, userID); err != nil {
		log.Printf("add group member failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to add group member"))
	}

	return connect.NewResponse(&arcav1.AddGroupMemberResponse{}), nil
}

func (s *groupConnectService) RemoveGroupMember(ctx context.Context, req *connect.Request[arcav1.RemoveGroupMemberRequest]) (*connect.Response[arcav1.RemoveGroupMemberResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	groupID := strings.TrimSpace(req.Msg.GetGroupId())
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if groupID == "" || userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("group id and user id are required"))
	}

	removed, err := s.store.RemoveUserGroupMember(ctx, groupID, userID)
	if err != nil {
		log.Printf("remove group member failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to remove group member"))
	}
	if !removed {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("member not found in group"))
	}

	return connect.NewResponse(&arcav1.RemoveGroupMemberResponse{}), nil
}

func (s *groupConnectService) SearchGroups(ctx context.Context, req *connect.Request[arcav1.SearchGroupsRequest]) (*connect.Response[arcav1.SearchGroupsResponse], error) {
	if _, err := s.authenticateUser(ctx, req.Header()); err != nil {
		return nil, err
	}

	query := strings.TrimSpace(req.Msg.GetQuery())
	if query == "" {
		return connect.NewResponse(&arcav1.SearchGroupsResponse{}), nil
	}

	limit := int64(req.Msg.GetLimit())
	if limit <= 0 || limit > 20 {
		limit = 20
	}

	groups, err := s.store.SearchUserGroups(ctx, query, limit)
	if err != nil {
		log.Printf("search groups failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to search groups"))
	}

	items := make([]*arcav1.UserGroup, 0, len(groups))
	for _, g := range groups {
		items = append(items, &arcav1.UserGroup{
			Id:   g.ID,
			Name: g.Name,
		})
	}
	return connect.NewResponse(&arcav1.SearchGroupsResponse{Groups: items}), nil
}

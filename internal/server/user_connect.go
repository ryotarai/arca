package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type userConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newUserConnectService(store *db.Store, authenticator Authenticator) *userConnectService {
	return &userConnectService{store: store, authenticator: authenticator}
}

func (s *userConnectService) ListUsers(ctx context.Context, req *connect.Request[arcav1.ListUsersRequest]) (*connect.Response[arcav1.ListUsersResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	users, err := s.authenticator.ListUsers(ctx)
	if err != nil {
		log.Printf("list users failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list users"))
	}

	items := make([]*arcav1.ManagedUser, 0, len(users))
	for _, user := range users {
		items = append(items, toManagedUserMessage(user))
	}
	return connect.NewResponse(&arcav1.ListUsersResponse{Users: items}), nil
}

func (s *userConnectService) CreateUser(ctx context.Context, req *connect.Request[arcav1.CreateUserRequest]) (*connect.Response[arcav1.CreateUserResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	email := strings.TrimSpace(req.Msg.GetEmail())
	if email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("email is required"))
	}

	userID, normalizedEmail, setupToken, expiresAt, err := s.authenticator.ProvisionUser(ctx, email, adminUserID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("email is invalid"))
		case errors.Is(err, auth.ErrEmailAlreadyUsed):
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("email already used"))
		default:
			log.Printf("create user failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create user"))
		}
	}

	return connect.NewResponse(&arcav1.CreateUserResponse{
		User: &arcav1.ManagedUser{
			Id:                  userID,
			Email:               normalizedEmail,
			SetupRequired:       true,
			SetupTokenExpiresAt: expiresAt.Unix(),
		},
		SetupToken:          setupToken,
		SetupTokenExpiresAt: expiresAt.Unix(),
	}), nil
}

func (s *userConnectService) IssueUserSetupToken(ctx context.Context, req *connect.Request[arcav1.IssueUserSetupTokenRequest]) (*connect.Response[arcav1.IssueUserSetupTokenResponse], error) {
	adminUserID, err := s.authenticateAdmin(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user id is required"))
	}

	setupToken, expiresAt, err := s.authenticator.IssueUserSetupToken(ctx, userID, adminUserID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user id is invalid"))
		case errors.Is(err, auth.ErrUserNotFound):
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		default:
			log.Printf("issue user setup token failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to issue setup token"))
		}
	}

	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		log.Printf("get user failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load user"))
	}

	return connect.NewResponse(&arcav1.IssueUserSetupTokenResponse{
		User: &arcav1.ManagedUser{
			Id:                  user.ID,
			Email:               user.Email,
			SetupRequired:       true,
			SetupTokenExpiresAt: expiresAt.Unix(),
		},
		SetupToken:          setupToken,
		SetupTokenExpiresAt: expiresAt.Unix(),
	}), nil
}

func (s *userConnectService) CompleteUserSetup(ctx context.Context, req *connect.Request[arcav1.CompleteUserSetupRequest]) (*connect.Response[arcav1.CompleteUserSetupResponse], error) {
	userID, email, err := s.authenticator.CompleteUserSetup(ctx, req.Msg.GetSetupToken(), req.Msg.GetPassword())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("setup token and password are required"))
		case errors.Is(err, auth.ErrInvalidSetupToken):
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("setup token is invalid or expired"))
		default:
			log.Printf("complete user setup failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to complete user setup"))
		}
	}

	return connect.NewResponse(&arcav1.CompleteUserSetupResponse{
		User: &arcav1.User{Id: userID, Email: email},
	}), nil
}

func (s *userConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	if s.authenticator == nil || s.store == nil {
		return "", connect.NewError(connect.CodeUnavailable, errors.New("user management unavailable"))
	}
	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	userID, _, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	setupState, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("load setup state failed: %v", err)
		return "", connect.NewError(connect.CodeInternal, errors.New("failed to authorize user"))
	}
	if strings.TrimSpace(setupState.AdminUserID) == "" || setupState.AdminUserID != userID {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can manage users"))
	}
	return userID, nil
}

func toManagedUserMessage(user db.ManagedUser) *arcav1.ManagedUser {
	return &arcav1.ManagedUser{
		Id:                  user.ID,
		Email:               user.Email,
		SetupRequired:       user.PasswordSetupRequired,
		SetupTokenExpiresAt: user.SetupTokenExpiresAt,
		CreatedAt:           user.CreatedAt,
	}
}

package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type adminConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newAdminConnectService(store *db.Store, authenticator Authenticator) *adminConnectService {
	return &adminConnectService{store: store, authenticator: authenticator}
}

func (s *adminConnectService) StartImpersonation(ctx context.Context, req *connect.Request[arcav1.StartImpersonationRequest]) (*connect.Response[arcav1.StartImpersonationResponse], error) {
	adminUserID, sessionToken, err := s.authenticateAdminWithToken(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	targetUserID := strings.TrimSpace(req.Msg.GetUserId())
	if targetUserID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}

	if targetUserID == adminUserID {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot impersonate yourself"))
	}

	// Verify target user exists
	targetUser, err := s.store.GetUserByID(ctx, targetUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
		}
		log.Printf("get user for impersonation failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get user"))
	}

	tokenHash := hashToken(sessionToken)
	updated, err := s.store.SetSessionImpersonation(ctx, tokenHash, targetUserID, adminUserID)
	if err != nil {
		log.Printf("set session impersonation failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to start impersonation"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeInternal, errors.New("session not found"))
	}

	// Audit log
	s.logAudit(ctx, adminUserID, targetUserID, "admin.impersonation_start", "user", targetUser.ID, fmt.Sprintf(`{"target_email":%q}`, targetUser.Email))

	return connect.NewResponse(&arcav1.StartImpersonationResponse{}), nil
}

func (s *adminConnectService) StopImpersonation(ctx context.Context, req *connect.Request[arcav1.StopImpersonationRequest]) (*connect.Response[arcav1.StopImpersonationResponse], error) {
	sessionToken, _ := sessionTokenFromHeader(req.Header())
	if sessionToken == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	result, err := s.authenticator.AuthenticateFull(ctx, sessionToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	if result.OriginalUserID == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("not currently impersonating"))
	}

	tokenHash := hashToken(sessionToken)
	if _, err := s.store.ClearSessionImpersonation(ctx, tokenHash); err != nil {
		log.Printf("clear session impersonation failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to stop impersonation"))
	}

	// Audit log
	s.logAudit(ctx, result.OriginalUserID, result.UserID, "admin.impersonation_stop", "user", result.UserID, fmt.Sprintf(`{"target_email":%q}`, result.Email))

	return connect.NewResponse(&arcav1.StopImpersonationResponse{}), nil
}

func (s *adminConnectService) GetImpersonationStatus(ctx context.Context, req *connect.Request[arcav1.GetImpersonationStatusRequest]) (*connect.Response[arcav1.GetImpersonationStatusResponse], error) {
	sessionToken, _ := sessionTokenFromHeader(req.Header())
	if sessionToken == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	result, err := s.authenticator.AuthenticateFull(ctx, sessionToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	resp := &arcav1.GetImpersonationStatusResponse{}
	if result.OriginalUserID != "" {
		resp.IsImpersonating = true
		resp.ImpersonatedUserEmail = result.Email
		resp.OriginalUserEmail = result.OriginalEmail
	}

	return connect.NewResponse(resp), nil
}

func (s *adminConnectService) ListAuditLogs(ctx context.Context, req *connect.Request[arcav1.ListAuditLogsRequest]) (*connect.Response[arcav1.ListAuditLogsResponse], error) {
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}

	limit := int64(req.Msg.GetLimit())
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	entries, err := s.store.ListAuditLogs(ctx, limit)
	if err != nil {
		log.Printf("list audit logs failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list audit logs"))
	}

	items := make([]*arcav1.AuditLog, 0, len(entries))
	for _, e := range entries {
		items = append(items, &arcav1.AuditLog{
			Id:            e.ID,
			ActorEmail:    e.ActorEmail,
			ActingAsEmail: e.ActingAsEmail,
			Action:        e.Action,
			ResourceType:  e.ResourceType,
			ResourceId:    e.ResourceID,
			Details:       e.DetailsJSON,
			CreatedAt:     fmt.Sprintf("%d", e.CreatedAt.Unix()),
		})
	}

	return connect.NewResponse(&arcav1.ListAuditLogsResponse{AuditLogs: items}), nil
}

func (s *adminConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		result, err := s.authenticator.AuthenticateFull(ctx, sessionToken)
		if err == nil {
			checkUserID := result.UserID
			checkRole := result.Role
			if result.OriginalUserID != "" {
				checkUserID = result.OriginalUserID
				origUser, getErr := s.store.GetUserByID(ctx, result.OriginalUserID)
				if getErr != nil {
					return "", connect.NewError(connect.CodeInternal, errors.New("failed to verify admin"))
				}
				checkRole = origUser.Role
			}
			if checkRole != db.UserRoleAdmin {
				return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can access admin functions"))
			}
			return checkUserID, nil
		}
	}

	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, _, role, err := s.authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			if role != db.UserRoleAdmin {
				return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can access admin functions"))
			}
			return userID, nil
		}
	}

	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

// authenticateAdminWithToken returns (adminUserID, sessionToken, error).
func (s *adminConnectService) authenticateAdminWithToken(ctx context.Context, header http.Header) (string, string, error) {
	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken == "" {
		return "", "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	result, err := s.authenticator.AuthenticateFull(ctx, sessionToken)
	if err != nil {
		return "", "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	checkUserID := result.UserID
	checkRole := result.Role
	if result.OriginalUserID != "" {
		checkUserID = result.OriginalUserID
		origUser, getErr := s.store.GetUserByID(ctx, result.OriginalUserID)
		if getErr != nil {
			return "", "", connect.NewError(connect.CodeInternal, errors.New("failed to verify admin"))
		}
		checkRole = origUser.Role
	}

	if checkRole != db.UserRoleAdmin {
		return "", "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can impersonate users"))
	}

	return checkUserID, sessionToken, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *adminConnectService) logAudit(ctx context.Context, actorUserID, actingAsUserID, action, resourceType, resourceID, detailsJSON string) {
	id, err := randomAuditID()
	if err != nil {
		log.Printf("generate audit log id failed: %v", err)
		return
	}
	entry := db.AuditLogEntry{
		ID:             id,
		ActorUserID:    actorUserID,
		ActingAsUserID: actingAsUserID,
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		DetailsJSON:    detailsJSON,
		CreatedAt:      time.Now(),
	}
	if err := s.store.CreateAuditLog(ctx, entry); err != nil {
		log.Printf("create audit log failed: %v", err)
	}
}

func randomAuditID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
)

// writeAuditLog records an audit log entry. actingAsUserID may be empty.
func writeAuditLog(ctx context.Context, store *db.Store, actorUserID, actingAsUserID, action, resourceType, resourceID, detailsJSON string) {
	if store == nil {
		return
	}
	id, err := randomAuditID()
	if err != nil {
		slog.ErrorContext(ctx, "generate audit log id failed", "error", err)
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
	if err := store.CreateAuditLog(ctx, entry); err != nil {
		slog.ErrorContext(ctx, "create audit log failed", "error", err)
	}
}

// writeAuditLogFromAuth writes an audit log, deriving actor from AuthResult.
func writeAuditLogFromAuth(ctx context.Context, store *db.Store, result auth.AuthResult, action, resourceType, resourceID, detailsJSON string) {
	if result.IsNonAdminMode {
		if detailsJSON == "{}" {
			detailsJSON = `{"non_admin_mode":true}`
		} else {
			detailsJSON = `{"non_admin_mode":true,` + detailsJSON[1:]
		}
	}
	writeAuditLog(ctx, store, result.UserID, "", action, resourceType, resourceID, detailsJSON)
}

// authenticateUserFromHeaderWithResult returns the full AuthResult.
func authenticateUserFromHeaderWithResult(ctx context.Context, authenticator Authenticator, store *db.Store, header http.Header) (auth.AuthResult, error) {
	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		result, err := authenticator.AuthenticateFull(ctx, sessionToken)
		if err == nil {
			// Apply admin view mode override (skip for static API tokens)
			if result.Role == db.UserRoleAdmin && !result.IsStaticToken && store != nil {
				mode, modeErr := store.GetAdminViewMode(ctx, result.UserID)
				if modeErr == nil && mode == "user" {
					result.Role = db.UserRoleUser
					result.IsNonAdminMode = true
				}
			}
			return result, nil
		}
	}
	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, email, role, err := authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			result := auth.AuthResult{UserID: userID, Email: email, Role: role}
			// Apply admin view mode override
			if result.Role == db.UserRoleAdmin && store != nil {
				mode, modeErr := store.GetAdminViewMode(ctx, result.UserID)
				if modeErr == nil && mode == "user" {
					result.Role = db.UserRoleUser
					result.IsNonAdminMode = true
				}
			}
			return result, nil
		}
	}
	return auth.AuthResult{}, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

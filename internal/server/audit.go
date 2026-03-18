package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
)

// writeAuditLog records an audit log entry. actingAsUserID may be empty for
// non-impersonated operations.
func writeAuditLog(ctx context.Context, store *db.Store, actorUserID, actingAsUserID, action, resourceType, resourceID, detailsJSON string) {
	if store == nil {
		return
	}
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
	if err := store.CreateAuditLog(ctx, entry); err != nil {
		log.Printf("create audit log failed: %v", err)
	}
}

// writeAuditLogFromAuth writes an audit log, deriving actor/actingAs from AuthResult.
func writeAuditLogFromAuth(ctx context.Context, store *db.Store, result auth.AuthResult, action, resourceType, resourceID, detailsJSON string) {
	actorUserID := result.UserID
	actingAsUserID := ""
	if result.OriginalUserID != "" {
		actorUserID = result.OriginalUserID
		actingAsUserID = result.UserID
	}
	writeAuditLog(ctx, store, actorUserID, actingAsUserID, action, resourceType, resourceID, detailsJSON)
}

// authenticateUserFromHeaderWithResult returns the full AuthResult.
func authenticateUserFromHeaderWithResult(ctx context.Context, authenticator Authenticator, header http.Header) (auth.AuthResult, error) {
	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		result, err := authenticator.AuthenticateFull(ctx, sessionToken)
		if err == nil {
			return result, nil
		}
	}
	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, email, role, err := authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			return auth.AuthResult{UserID: userID, Email: email, Role: role}, nil
		}
	}
	return auth.AuthResult{}, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

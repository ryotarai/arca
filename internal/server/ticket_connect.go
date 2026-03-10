package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

const authTicketTTL = 2 * time.Minute
const arcadSessionTTL = 8 * time.Hour

type ticketConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

func newTicketConnectService(store *db.Store, authenticator Authenticator) *ticketConnectService {
	return &ticketConnectService{store: store, authenticator: authenticator}
}

func (s *ticketConnectService) IssueTicket(ctx context.Context, req *connect.Request[arcav1.IssueTicketRequest]) (*connect.Response[arcav1.IssueTicketResponse], error) {
	if s.store == nil || s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("ticket service unavailable"))
	}

	userID, err := authenticateUserFromHeader(ctx, s.authenticator, req.Header())
	if err != nil {
		return nil, err
	}

	machineID := strings.TrimSpace(req.Msg.GetMachineId())
	if machineID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("machine id is required"))
	}
	if _, err := s.store.GetMachineByIDForUser(ctx, userID, machineID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("machine not found"))
		}
		log.Printf("authorize machine for ticket failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
	}

	expiresAt := time.Now().Add(authTicketTTL)
	ticket, err := s.store.CreateAuthTicket(ctx, userID, machineID, strings.TrimSpace(req.Msg.GetExposureId()), expiresAt.Unix())
	if err != nil {
		log.Printf("issue ticket failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to issue ticket"))
	}

	return connect.NewResponse(&arcav1.IssueTicketResponse{Ticket: ticket, ExpiresAtUnix: expiresAt.Unix()}), nil
}

func (s *ticketConnectService) VerifyTicket(ctx context.Context, req *connect.Request[arcav1.VerifyTicketRequest]) (*connect.Response[arcav1.VerifyTicketResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("ticket service unavailable"))
	}

	ticket := strings.TrimSpace(req.Msg.GetTicket())
	if ticket == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ticket is required"))
	}
	machineID, err := resolveMachineIDFromHeader(ctx, s.store, req.Header())
	if err != nil {
		return nil, err
	}

	nowUnix := time.Now().Unix()
	verified, err := s.store.VerifyAndConsumeAuthTicketByMachineID(ctx, machineID, ticket, nowUnix)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid ticket"))
		}
		log.Printf("verify ticket failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to verify ticket"))
	}

	return connect.NewResponse(&arcav1.VerifyTicketResponse{
		User:       &arcav1.TicketUser{Id: verified.UserID, Email: verified.UserEmail},
		MachineId:  verified.MachineID,
		ExposureId: verified.ExposureID,
	}), nil
}

func (s *ticketConnectService) ExchangeArcadSession(ctx context.Context, req *connect.Request[arcav1.ExchangeArcadSessionRequest]) (*connect.Response[arcav1.ExchangeArcadSessionResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("ticket service unavailable"))
	}

	token := strings.TrimSpace(req.Msg.GetToken())
	if token == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("token is required"))
	}
	machineID, err := resolveMachineIDFromHeader(ctx, s.store, req.Header())
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session, err := s.store.ExchangeArcadTokenByMachineID(ctx, machineID, token, now.Unix(), now.Add(arcadSessionTTL).Unix())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid ticket"))
		}
		log.Printf("exchange arcad session failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to exchange arcad session"))
	}

	return connect.NewResponse(&arcav1.ExchangeArcadSessionResponse{
		SessionId:     session.SessionID,
		ExpiresAtUnix: session.ExpiresAt,
		User:          &arcav1.TicketUser{Id: session.UserID, Email: session.UserEmail},
		MachineId:     session.MachineID,
		ExposureId:    session.ExposureID,
	}), nil
}

func (s *ticketConnectService) ValidateArcadSession(ctx context.Context, req *connect.Request[arcav1.ValidateArcadSessionRequest]) (*connect.Response[arcav1.ValidateArcadSessionResponse], error) {
	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("ticket service unavailable"))
	}

	sessionID := strings.TrimSpace(req.Msg.GetSessionId())
	if sessionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session id is required"))
	}
	hostname := strings.TrimSpace(req.Msg.GetHostname())
	if hostname == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hostname is required"))
	}
	targetPath := strings.TrimSpace(req.Msg.GetPath())
	if targetPath == "" {
		targetPath = "/"
	}

	machineID, err := resolveMachineIDFromHeader(ctx, s.store, req.Header())
	if err != nil {
		return nil, err
	}

	session, err := s.store.GetActiveArcadSessionByMachineID(ctx, machineID, sessionID, time.Now().Unix())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid session"))
		}
		log.Printf("validate arcad session lookup failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to validate session"))
	}

	exposure, err := s.store.GetMachineExposureByHostname(ctx, hostname)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid session"))
		}
		log.Printf("validate arcad session exposure lookup failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to resolve exposure"))
	}
	if exposure.MachineID != machineID {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid session"))
	}
	if session.ExposureID != "" && exposure.ID != session.ExposureID {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid session"))
	}
	if !canUserAccessExposure(ctx, s.store, exposure, session.UserID, targetPath) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("access denied"))
	}

	return connect.NewResponse(&arcav1.ValidateArcadSessionResponse{
		User:       &arcav1.TicketUser{Id: session.UserID, Email: session.UserEmail},
		MachineId:  session.MachineID,
		ExposureId: session.ExposureID,
	}), nil
}

func resolveMachineIDFromHeader(ctx context.Context, store *db.Store, header http.Header) (string, error) {
	machineToken := strings.TrimSpace(machineTokenFromHeader(header))
	machineID := strings.TrimSpace(header.Get("X-Arca-Machine-ID"))
	switch {
	case machineToken != "":
		resolvedID, err := store.GetMachineIDByMachineToken(ctx, machineToken)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", connect.NewError(connect.CodeUnauthenticated, errors.New("invalid machine token"))
			}
			return "", connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
		}
		if machineID != "" && machineID != resolvedID {
			return "", connect.NewError(connect.CodeUnauthenticated, errors.New("machine id does not match token"))
		}
		return resolvedID, nil
	case machineID != "":
		return machineID, nil
	default:
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("machine token or machine id is required"))
	}
}

func authenticateUserFromHeader(ctx context.Context, authenticator Authenticator, header http.Header) (string, error) {
	sessionToken, _ := sessionTokenFromHeader(header)
	if sessionToken != "" {
		userID, _, _, err := authenticator.Authenticate(ctx, sessionToken)
		if err == nil {
			return userID, nil
		}
	}

	// Fallback: try IAP JWT
	if iapJWT := iapJWTFromHeader(header); iapJWT != "" {
		userID, _, _, err := authenticator.AuthenticateIAPJWT(ctx, iapJWT)
		if err == nil {
			return userID, nil
		}
	}

	return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
}

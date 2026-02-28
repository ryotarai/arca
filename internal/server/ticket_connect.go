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
	machineToken := strings.TrimSpace(machineTokenFromHeader(req.Header()))
	machineID := strings.TrimSpace(req.Header().Get("X-Arca-Machine-ID"))

	nowUnix := time.Now().Unix()
	var (
		verified db.VerifiedTicket
		err      error
	)
	switch {
	case machineToken != "":
		verified, err = s.store.VerifyAndConsumeAuthTicket(ctx, machineToken, ticket, nowUnix)
	case machineID != "":
		verified, err = s.store.VerifyAndConsumeAuthTicketByMachineID(ctx, machineID, ticket, nowUnix)
	default:
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine token or machine id is required"))
	}
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

func authenticateUserFromHeader(ctx context.Context, authenticator Authenticator, header http.Header) (string, error) {
	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	userID, _, err := authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return userID, nil
}

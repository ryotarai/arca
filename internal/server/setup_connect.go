package server

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/cloudflare"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type setupConnectService struct {
	store         *db.Store
	authenticator Authenticator
	cf            *cloudflare.Client
}

func newSetupConnectService(store *db.Store, authenticator Authenticator, cf *cloudflare.Client) *setupConnectService {
	return &setupConnectService{store: store, authenticator: authenticator, cf: cf}
}

func (s *setupConnectService) GetSetupStatus(ctx context.Context, _ *connect.Request[arcav1.GetSetupStatusRequest]) (*connect.Response[arcav1.GetSetupStatusResponse], error) {
	if shouldSkipSetup() {
		return connect.NewResponse(&arcav1.GetSetupStatusResponse{
			Status: &arcav1.SetupStatus{
				Completed:             true,
				AdminConfigured:       true,
				CloudflareConfigured:  true,
				DockerProviderEnabled: true,
			},
		}), nil
	}

	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("setup store unavailable"))
	}

	state, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("get setup status failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup status"))
	}

	return connect.NewResponse(&arcav1.GetSetupStatusResponse{Status: setupStatusMessage(state)}), nil
}

func (s *setupConnectService) ValidateCloudflareToken(ctx context.Context, req *connect.Request[arcav1.ValidateCloudflareTokenRequest]) (*connect.Response[arcav1.ValidateCloudflareTokenResponse], error) {
	if s.cf == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("cloudflare client unavailable"))
	}

	token := strings.TrimSpace(req.Msg.GetApiToken())
	if token == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("api token is required"))
	}

	if shouldSkipCloudflareValidation() {
		return connect.NewResponse(&arcav1.ValidateCloudflareTokenResponse{Valid: true, Message: "token verification skipped"}), nil
	}

	verification, err := s.cf.VerifyToken(ctx, token)
	if err != nil {
		return connect.NewResponse(&arcav1.ValidateCloudflareTokenResponse{Valid: false, Message: err.Error()}), nil
	}

	valid := strings.EqualFold(verification.Status, "active")
	message := "token verified"
	if !valid {
		message = "token is not active"
	}
	return connect.NewResponse(&arcav1.ValidateCloudflareTokenResponse{Valid: valid, Message: message}), nil
}

func (s *setupConnectService) CompleteSetup(ctx context.Context, req *connect.Request[arcav1.CompleteSetupRequest]) (*connect.Response[arcav1.CompleteSetupResponse], error) {
	if s.store == nil || s.authenticator == nil || s.cf == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("setup dependencies unavailable"))
	}

	current, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("load setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	if current.Completed {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("setup already completed"))
	}

	email := strings.TrimSpace(req.Msg.GetAdminEmail())
	password := req.Msg.GetAdminPassword()
	baseDomain := normalizeBaseDomain(req.Msg.GetBaseDomain())
	cfToken := strings.TrimSpace(req.Msg.GetCloudflareApiToken())
	zoneID := strings.TrimSpace(req.Msg.GetCloudflareZoneId())
	if email == "" || password == "" || baseDomain == "" || cfToken == "" || zoneID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("admin email, password, base domain, cloudflare token, and cloudflare zone id are required"))
	}

	if !shouldSkipCloudflareValidation() {
		verification, err := s.cf.VerifyToken(ctx, cfToken)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cloudflare token verification failed"))
		}
		if !strings.EqualFold(verification.Status, "active") {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cloudflare token is not active"))
		}
	}

	adminUserID, _, err := s.authenticator.Register(ctx, email, password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("admin email or password is invalid"))
		case errors.Is(err, auth.ErrEmailAlreadyUsed):
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("admin email already used"))
		default:
			log.Printf("create admin failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create admin"))
		}
	}

	state := db.SetupState{
		Completed:             true,
		AdminUserID:           adminUserID,
		BaseDomain:            baseDomain,
		CloudflareAPIToken:    cfToken,
		CloudflareZoneID:      zoneID,
		DockerProviderEnabled: req.Msg.GetDockerProviderEnabled(),
	}
	if err := s.store.UpsertSetupState(ctx, state); err != nil {
		log.Printf("persist setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist setup state"))
	}

	return connect.NewResponse(&arcav1.CompleteSetupResponse{Status: setupStatusMessage(state)}), nil
}

func setupStatusMessage(state db.SetupState) *arcav1.SetupStatus {
	return &arcav1.SetupStatus{
		Completed:             state.Completed,
		AdminConfigured:       strings.TrimSpace(state.AdminUserID) != "",
		CloudflareConfigured:  strings.TrimSpace(state.CloudflareAPIToken) != "",
		BaseDomain:            state.BaseDomain,
		DockerProviderEnabled: state.DockerProviderEnabled,
		CloudflareZoneId:      state.CloudflareZoneID,
	}
}

func normalizeBaseDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimSuffix(domain, "/")
	return domain
}

func shouldSkipCloudflareValidation() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ARCA_SKIP_CLOUDFLARE_VALIDATION")))
	return value == "1" || value == "true" || value == "yes"
}

func shouldSkipSetup() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ARCA_SKIP_SETUP")))
	return value == "1" || value == "true" || value == "yes"
}

package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"regexp"
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
	consoleTunnel *ConsoleTunnelManager
}

var (
	baseDomainPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	domainPrefixPattern = regexp.MustCompile(`^[a-z0-9-]*$`)
)

func newSetupConnectService(store *db.Store, authenticator Authenticator, cf *cloudflare.Client, consoleTunnel *ConsoleTunnelManager) *setupConnectService {
	return &setupConnectService{store: store, authenticator: authenticator, cf: cf, consoleTunnel: consoleTunnel}
}

func (s *setupConnectService) GetSetupStatus(ctx context.Context, _ *connect.Request[arcav1.GetSetupStatusRequest]) (*connect.Response[arcav1.GetSetupStatusResponse], error) {
	if shouldSkipSetup() {
		return connect.NewResponse(&arcav1.GetSetupStatusResponse{
			Status: &arcav1.SetupStatus{
				Completed:             true,
				AdminConfigured:       true,
				CloudflareConfigured:  true,
				DockerProviderEnabled: false,
				MachineRuntime:        db.MachineRuntimeLibvirt,
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
	accountID := strings.TrimSpace(req.Msg.GetAccountId())
	if token == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("api token is required"))
	}
	if accountID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account id is required"))
	}

	if shouldSkipCloudflareValidation() {
		return connect.NewResponse(&arcav1.ValidateCloudflareTokenResponse{Valid: true, Message: "token verification skipped"}), nil
	}

	verification, verifyErr := s.cf.VerifyToken(ctx, token)
	valid := false
	message := ""
	if verifyErr == nil {
		if !strings.EqualFold(verification.Status, "active") {
			return connect.NewResponse(&arcav1.ValidateCloudflareTokenResponse{
				Valid:   false,
				Message: "token is not active",
			}), nil
		}
		if err := s.cf.VerifyAccountToken(ctx, token, accountID); err == nil {
			valid = true
			message = "token verified"
		} else {
			message = "token is not a valid account token for the provided account id"
		}
	} else if err := s.cf.VerifyAccountToken(ctx, token, accountID); err == nil {
		valid = true
		message = "account token verified"
	} else {
		message = verifyErr.Error()
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
	baseDomain, err := validateBaseDomain(req.Msg.GetBaseDomain())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	domainPrefix, err := validateDomainPrefix(req.Msg.GetDomainPrefix())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	cfToken := strings.TrimSpace(req.Msg.GetCloudflareApiToken())
	zoneID := strings.TrimSpace(req.Msg.GetCloudflareZoneId())
	if email == "" || password == "" || cfToken == "" || zoneID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("admin email, password, base domain, cloudflare token, and cloudflare zone id are required"))
	}

	if !shouldSkipCloudflareValidation() {
		verification, verifyErr := s.cf.VerifyToken(ctx, cfToken)
		if verifyErr == nil {
			if !strings.EqualFold(verification.Status, "active") {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cloudflare token is not active"))
			}
		} else if err := s.cf.VerifyZoneAccess(ctx, cfToken, zoneID); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cloudflare token verification failed"))
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
		DomainPrefix:          domainPrefix,
		CloudflareAPIToken:    cfToken,
		MachineRuntime:        normalizeMachineRuntime(req.Msg.GetMachineRuntime()),
		CloudflareZoneID:      zoneID,
		DockerProviderEnabled: false,
	}

	if err := s.store.UpsertSetupState(ctx, state); err != nil {
		log.Printf("persist setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist setup state"))
	}
	if s.consoleTunnel != nil {
		if _, err := s.consoleTunnel.EnsureExposed(ctx, state); err != nil {
			log.Printf("ensure console tunnel failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to expose console endpoint"))
		}
	}

	return connect.NewResponse(&arcav1.CompleteSetupResponse{Status: setupStatusMessage(state)}), nil
}

func (s *setupConnectService) UpdateDomainSettings(ctx context.Context, req *connect.Request[arcav1.UpdateDomainSettingsRequest]) (*connect.Response[arcav1.UpdateDomainSettingsResponse], error) {
	if s.store == nil || s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("setup dependencies unavailable"))
	}
	if _, err := s.authenticate(ctx, req.Header()); err != nil {
		return nil, err
	}

	current, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("load setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	if !current.Completed {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("setup is not completed yet"))
	}

	baseDomain, err := validateBaseDomain(req.Msg.GetBaseDomain())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	domainPrefix, err := validateDomainPrefix(req.Msg.GetDomainPrefix())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	current.BaseDomain = baseDomain
	current.DomainPrefix = domainPrefix
	if strings.TrimSpace(req.Msg.GetMachineRuntime()) != "" {
		current.MachineRuntime = normalizeMachineRuntime(req.Msg.GetMachineRuntime())
	}
	current.DockerProviderEnabled = false
	if err := s.store.UpsertSetupState(ctx, current); err != nil {
		log.Printf("persist setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist setup state"))
	}
	if s.consoleTunnel != nil {
		if _, err := s.consoleTunnel.EnsureExposed(ctx, current); err != nil {
			log.Printf("ensure console tunnel failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to expose console endpoint"))
		}
	}

	return connect.NewResponse(&arcav1.UpdateDomainSettingsResponse{Status: setupStatusMessage(current)}), nil
}

func setupStatusMessage(state db.SetupState) *arcav1.SetupStatus {
	return &arcav1.SetupStatus{
		Completed:             state.Completed,
		AdminConfigured:       strings.TrimSpace(state.AdminUserID) != "",
		CloudflareConfigured:  strings.TrimSpace(state.CloudflareAPIToken) != "",
		BaseDomain:            state.BaseDomain,
		DomainPrefix:          state.DomainPrefix,
		DockerProviderEnabled: state.DockerProviderEnabled,
		CloudflareZoneId:      state.CloudflareZoneID,
		MachineRuntime:        normalizeMachineRuntime(state.MachineRuntime),
	}
}

func validateBaseDomain(domain string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(domain))
	if value == "" {
		return "", errors.New("base domain is required")
	}
	if len(value) > 253 {
		return "", errors.New("base domain is too long")
	}
	if !baseDomainPattern.MatchString(value) {
		return "", errors.New("base domain must be a valid domain name")
	}
	return value, nil
}

func validateDomainPrefix(prefix string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(prefix))
	if !domainPrefixPattern.MatchString(value) {
		return "", errors.New("domain prefix may contain only lowercase letters, numbers, and hyphens")
	}
	label := strings.Trim(value+"app", "-")
	if label == "" {
		label = "app"
	}
	if len(label) > 63 {
		return "", errors.New("domain prefix is too long")
	}
	return value, nil
}

func normalizeMachineRuntime(runtime string) string {
	return db.NormalizeMachineRuntime(runtime)
}

func (s *setupConnectService) authenticate(ctx context.Context, header http.Header) (string, error) {
	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	userID, _, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return userID, nil
}

func shouldSkipCloudflareValidation() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ARCA_SKIP_CLOUDFLARE_VALIDATION")))
	return value == "1" || value == "true" || value == "yes"
}

func shouldSkipSetup() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ARCA_SKIP_SETUP")))
	return value == "1" || value == "true" || value == "yes"
}

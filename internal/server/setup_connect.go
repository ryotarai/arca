package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
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
		internetPublicExposureDisabled := false
		if s.store != nil {
			state, err := s.store.GetSetupState(ctx)
			if err == nil {
				internetPublicExposureDisabled = state.InternetPublicExposureDisabled
			}
		}
		return connect.NewResponse(&arcav1.GetSetupStatusResponse{
			Status: &arcav1.SetupStatus{
				Completed:                      true,
				AdminConfigured:                true,
				CloudflareConfigured:           true,
				MachineRuntime:                 db.MachineRuntimeLibvirt,
				InternetPublicExposureDisabled: internetPublicExposureDisabled,
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

func (s *setupConnectService) VerifySetupPassword(ctx context.Context, req *connect.Request[arcav1.VerifySetupPasswordRequest]) (*connect.Response[arcav1.VerifySetupPasswordResponse], error) {
	if shouldSkipSetup() {
		return connect.NewResponse(&arcav1.VerifySetupPasswordResponse{Valid: true}), nil
	}

	if s.store == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("setup store unavailable"))
	}

	state, err := s.store.GetSetupState(ctx)
	if err != nil {
		log.Printf("get setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	if state.Completed {
		return connect.NewResponse(&arcav1.VerifySetupPasswordResponse{Valid: true}), nil
	}

	storedPassword, err := s.store.GetSetupPassword(ctx)
	if err != nil {
		log.Printf("get setup password failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to verify setup password"))
	}

	valid := storedPassword == "" || req.Msg.GetSetupPassword() == storedPassword
	return connect.NewResponse(&arcav1.VerifySetupPasswordResponse{Valid: valid}), nil
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

func serverExposureMethodFromProto(method arcav1.ServerExposureMethod) string {
	switch method {
	case arcav1.ServerExposureMethod_SERVER_EXPOSURE_METHOD_MANUAL:
		return db.ServerExposureMethodManual
	default:
		return db.ServerExposureMethodCloudflareTunnel
	}
}

func serverExposureMethodToProto(method string) arcav1.ServerExposureMethod {
	switch db.NormalizeServerExposureMethod(method) {
	case db.ServerExposureMethodManual:
		return arcav1.ServerExposureMethod_SERVER_EXPOSURE_METHOD_MANUAL
	default:
		return arcav1.ServerExposureMethod_SERVER_EXPOSURE_METHOD_CLOUDFLARE_TUNNEL
	}
}

func validateServerDomain(domain string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(domain))
	if value == "" {
		return "", errors.New("server domain is required")
	}
	if len(value) > 253 {
		return "", errors.New("server domain is too long")
	}
	if !baseDomainPattern.MatchString(value) {
		return "", errors.New("server domain must be a valid domain name")
	}
	return value, nil
}

func (s *setupConnectService) CompleteSetup(ctx context.Context, req *connect.Request[arcav1.CompleteSetupRequest]) (*connect.Response[arcav1.CompleteSetupResponse], error) {
	if s.store == nil || s.authenticator == nil {
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

	if !shouldSkipSetup() {
		storedPassword, pwErr := s.store.GetSetupPassword(ctx)
		if pwErr != nil {
			log.Printf("get setup password failed: %v", pwErr)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to verify setup password"))
		}
		if storedPassword != "" && req.Msg.GetSetupPassword() != storedPassword {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("invalid setup password"))
		}
	}

	email := strings.TrimSpace(req.Msg.GetAdminEmail())
	password := req.Msg.GetAdminPassword()
	if email == "" || password == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("admin email and password are required"))
	}

	serverExposureMethod := serverExposureMethodFromProto(req.Msg.GetServerExposureMethod())

	cfToken := strings.TrimSpace(req.Msg.GetCloudflareApiToken())
	zoneID := strings.TrimSpace(req.Msg.GetCloudflareZoneId())

	// Server domain: required for manual, optional for cloudflare_tunnel (derived from tunnel)
	var serverDomain string
	if serverExposureMethod == db.ServerExposureMethodManual {
		var domainErr error
		serverDomain, domainErr = validateServerDomain(req.Msg.GetServerDomain())
		if domainErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, domainErr)
		}
	} else {
		serverDomain = strings.ToLower(strings.TrimSpace(req.Msg.GetServerDomain()))
	}

	if serverExposureMethod == db.ServerExposureMethodCloudflareTunnel {
		if s.cf == nil {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("cloudflare client unavailable"))
		}
		if cfToken == "" || zoneID == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cloudflare token and zone id are required for cloudflare tunnel exposure"))
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

	if _, err := s.store.UpdateUserRoleByID(ctx, adminUserID, db.UserRoleAdmin); err != nil {
		log.Printf("set admin role failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to set admin role"))
	}

	state := db.SetupState{
		Completed:            true,
		HasAdmin:             true,
		CloudflareAPIToken:   cfToken,
		MachineRuntime:       normalizeMachineRuntime(req.Msg.GetMachineRuntime()),
		CloudflareZoneID:     zoneID,
		ServerExposureMethod: serverExposureMethod,
		ServerDomain:         serverDomain,
		OIDCEnabled:          current.OIDCEnabled,
		OIDCIssuerURL:        current.OIDCIssuerURL,
		OIDCClientID:         current.OIDCClientID,
		OIDCClientSecret:     current.OIDCClientSecret,
		OIDCAllowedEmailDomains: append([]string(nil),
			current.OIDCAllowedEmailDomains...,
		),
	}

	if err := s.store.UpsertSetupState(ctx, state); err != nil {
		log.Printf("persist setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist setup state"))
	}
	if serverExposureMethod == db.ServerExposureMethodCloudflareTunnel && s.consoleTunnel != nil && !shouldSkipCloudflareValidation() {
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
	if _, err := s.authenticateAdmin(ctx, req.Header()); err != nil {
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

	if strings.TrimSpace(req.Msg.GetMachineRuntime()) != "" {
		current.MachineRuntime = normalizeMachineRuntime(req.Msg.GetMachineRuntime())
	}
	current.InternetPublicExposureDisabled = req.Msg.GetDisableInternetPublicExposure()

	// Server exposure settings
	if req.Msg.GetServerExposureMethod() != arcav1.ServerExposureMethod_SERVER_EXPOSURE_METHOD_UNSPECIFIED {
		current.ServerExposureMethod = serverExposureMethodFromProto(req.Msg.GetServerExposureMethod())
	}
	if serverDomain := strings.TrimSpace(req.Msg.GetServerDomain()); serverDomain != "" {
		validated, domErr := validateServerDomain(serverDomain)
		if domErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, domErr)
		}
		current.ServerDomain = validated
	}
	if cfToken := strings.TrimSpace(req.Msg.GetCloudflareApiToken()); cfToken != "" {
		current.CloudflareAPIToken = cfToken
	}
	if cfZoneID := strings.TrimSpace(req.Msg.GetCloudflareZoneId()); cfZoneID != "" {
		current.CloudflareZoneID = cfZoneID
	}

	oidcIssuerURL, err := validateOIDCIssuerURL(req.Msg.GetOidcIssuerUrl())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	oidcAllowedEmailDomains, err := normalizeOIDCAllowedEmailDomains(req.Msg.GetOidcAllowedEmailDomains())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	current.OIDCEnabled = req.Msg.GetOidcEnabled()
	current.OIDCIssuerURL = oidcIssuerURL
	current.OIDCClientID = strings.TrimSpace(req.Msg.GetOidcClientId())
	if req.Msg.GetClearOidcClientSecret() {
		current.OIDCClientSecret = ""
	} else if secret := strings.TrimSpace(req.Msg.GetOidcClientSecret()); secret != "" {
		current.OIDCClientSecret = secret
	}
	current.OIDCAllowedEmailDomains = oidcAllowedEmailDomains
	if current.OIDCEnabled {
		if current.OIDCIssuerURL == "" || current.OIDCClientID == "" || strings.TrimSpace(current.OIDCClientSecret) == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("oidc issuer url, client id, and client secret are required when oidc is enabled"))
		}
	}
	// Password login disabled: only allowed when OIDC is fully configured
	current.PasswordLoginDisabled = req.Msg.GetPasswordLoginDisabled()
	if current.PasswordLoginDisabled && !current.OIDCEnabled {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot disable password login without enabling oidc"))
	}
	if current.PasswordLoginDisabled && current.OIDCEnabled {
		if current.OIDCIssuerURL == "" || current.OIDCClientID == "" || strings.TrimSpace(current.OIDCClientSecret) == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cannot disable password login without a fully configured oidc provider"))
		}
	}
	if err := s.store.UpsertSetupState(ctx, current); err != nil {
		log.Printf("persist setup state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist setup state"))
	}
	if current.ServerExposureMethod == db.ServerExposureMethodCloudflareTunnel && s.consoleTunnel != nil && !shouldSkipCloudflareValidation() {
		if _, err := s.consoleTunnel.EnsureExposed(ctx, current); err != nil {
			log.Printf("ensure console tunnel failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to expose console endpoint"))
		}
	}

	return connect.NewResponse(&arcav1.UpdateDomainSettingsResponse{Status: setupStatusMessage(current)}), nil
}

func setupStatusMessage(state db.SetupState) *arcav1.SetupStatus {
	return &arcav1.SetupStatus{
		Completed:                      state.Completed,
		AdminConfigured:                state.HasAdmin,
		CloudflareConfigured:           strings.TrimSpace(state.CloudflareAPIToken) != "",
		CloudflareZoneId:               state.CloudflareZoneID,
		MachineRuntime:                 normalizeMachineRuntime(state.MachineRuntime),
		InternetPublicExposureDisabled: state.InternetPublicExposureDisabled,
		OidcEnabled:                    state.OIDCEnabled,
		OidcIssuerUrl:                  state.OIDCIssuerURL,
		OidcClientId:                   state.OIDCClientID,
		OidcClientSecretConfigured:     strings.TrimSpace(state.OIDCClientSecret) != "",
		OidcAllowedEmailDomains:        append([]string(nil), state.OIDCAllowedEmailDomains...),
		ServerExposureMethod:           serverExposureMethodToProto(state.ServerExposureMethod),
		ServerDomain:                   state.ServerDomain,
		PasswordLoginDisabled:          state.PasswordLoginDisabled,
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

func validateOIDCIssuerURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", errors.New("oidc issuer url is invalid")
	}
	if !strings.EqualFold(parsed.Scheme, "https") || strings.TrimSpace(parsed.Hostname()) == "" {
		return "", errors.New("oidc issuer url must be an https url")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeOIDCAllowedEmailDomains(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if strings.Contains(normalized, "@") || strings.Contains(normalized, "/") || !strings.Contains(normalized, ".") {
			return nil, errors.New("oidc allowed email domains must contain only domain names like example.com")
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func (s *setupConnectService) authenticate(ctx context.Context, header http.Header) (string, error) {
	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	userID, _, _, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	return userID, nil
}

func (s *setupConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	sessionToken, err := sessionTokenFromHeader(header)
	if err != nil || sessionToken == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	userID, _, role, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}
	if role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can update setup settings"))
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

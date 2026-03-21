package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type setupConnectService struct {
	store         *db.Store
	authenticator Authenticator
}

var (
	baseDomainPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	domainPrefixPattern = regexp.MustCompile(`^[a-z0-9-]*$`)
)

func newSetupConnectService(store *db.Store, authenticator Authenticator) *setupConnectService {
	return &setupConnectService{store: store, authenticator: authenticator}
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
		slog.ErrorContext(ctx, "get setup status failed", "error", err)
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
		slog.ErrorContext(ctx, "get setup state failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	if state.Completed {
		return connect.NewResponse(&arcav1.VerifySetupPasswordResponse{Valid: true}), nil
	}

	storedPassword, err := s.store.GetSetupPassword(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "get setup password failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to verify setup password"))
	}

	valid := storedPassword == "" || req.Msg.GetSetupPassword() == storedPassword
	return connect.NewResponse(&arcav1.VerifySetupPasswordResponse{Valid: valid}), nil
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
		slog.ErrorContext(ctx, "load setup state failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	if current.Completed {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("setup already completed"))
	}

	if !shouldSkipSetup() {
		storedPassword, pwErr := s.store.GetSetupPassword(ctx)
		if pwErr != nil {
			slog.ErrorContext(ctx, "get setup password failed", "error", pwErr)
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

	serverDomain, domainErr := validateServerDomain(req.Msg.GetServerDomain())
	if domainErr != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, domainErr)
	}

	adminUserID, _, err := s.authenticator.Register(ctx, email, password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("admin email or password is invalid"))
		case errors.Is(err, auth.ErrEmailAlreadyUsed):
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("admin email already used"))
		default:
			slog.ErrorContext(ctx, "create admin failed", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create admin"))
		}
	}

	if _, err := s.store.UpdateUserRoleByID(ctx, adminUserID, db.UserRoleAdmin); err != nil {
		slog.ErrorContext(ctx, "set admin role failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to set admin role"))
	}

	state := db.SetupState{
		Completed:      true,
		HasAdmin:       true,
		MachineRuntime: normalizeMachineRuntime(req.Msg.GetMachineRuntime()),
		ServerDomain:   serverDomain,
		OIDCEnabled:    current.OIDCEnabled,
		OIDCIssuerURL:  current.OIDCIssuerURL,
		OIDCClientID:   current.OIDCClientID,
		OIDCClientSecret: current.OIDCClientSecret,
		OIDCAllowedEmailDomains: append([]string(nil),
			current.OIDCAllowedEmailDomains...,
		),
		IAPEnabled:  current.IAPEnabled,
		IAPAudience: current.IAPAudience,
	}

	if err := s.store.UpsertSetupState(ctx, state); err != nil {
		slog.ErrorContext(ctx, "persist setup state failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist setup state"))
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
		slog.ErrorContext(ctx, "load setup state failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to load setup state"))
	}
	if !current.Completed {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("setup is not completed yet"))
	}

	if strings.TrimSpace(req.Msg.GetMachineRuntime()) != "" {
		current.MachineRuntime = normalizeMachineRuntime(req.Msg.GetMachineRuntime())
	}
	current.InternetPublicExposureDisabled = req.Msg.GetDisableInternetPublicExposure()

	if serverDomain := strings.TrimSpace(req.Msg.GetServerDomain()); serverDomain != "" {
		validated, domErr := validateServerDomain(serverDomain)
		if domErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, domErr)
		}
		current.ServerDomain = validated
	}

	if bd := strings.TrimSpace(req.Msg.GetBaseDomain()); bd != "" {
		validated, domErr := validateBaseDomain(bd)
		if domErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, domErr)
		}
		current.BaseDomain = validated
	}

	if dp := req.Msg.GetDomainPrefix(); dp != "" {
		validated, dpErr := validateDomainPrefix(dp)
		if dpErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, dpErr)
		}
		current.DomainPrefix = validated
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
	// IAP settings
	current.IAPEnabled = req.Msg.GetIapEnabled()
	if current.IAPEnabled {
		iapAudience := strings.TrimSpace(req.Msg.GetIapAudience())
		if iapAudience == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("iap audience is required when iap is enabled"))
		}
		current.IAPAudience = iapAudience
	} else {
		current.IAPAudience = strings.TrimSpace(req.Msg.GetIapAudience())
	}

	current.IAPAutoProvisioning = req.Msg.GetIapAutoProvisioning()
	current.OIDCAutoProvisioning = req.Msg.GetOidcAutoProvisioning()

	// Password login validation: at least one auth method must remain enabled
	current.PasswordLoginDisabled = req.Msg.GetPasswordLoginDisabled()
	passwordEnabled := !current.PasswordLoginDisabled
	if !passwordEnabled && !current.OIDCEnabled && !current.IAPEnabled {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least one authentication method must be enabled"))
	}
	if err := s.store.UpsertSetupState(ctx, current); err != nil {
		slog.ErrorContext(ctx, "persist setup state failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to persist setup state"))
	}
	return connect.NewResponse(&arcav1.UpdateDomainSettingsResponse{Status: setupStatusMessage(current)}), nil
}

func setupStatusMessage(state db.SetupState) *arcav1.SetupStatus {
	return &arcav1.SetupStatus{
		Completed:                      state.Completed,
		AdminConfigured:                state.HasAdmin,
		MachineRuntime:                 normalizeMachineRuntime(state.MachineRuntime),
		InternetPublicExposureDisabled: state.InternetPublicExposureDisabled,
		OidcEnabled:                    state.OIDCEnabled,
		OidcIssuerUrl:                  state.OIDCIssuerURL,
		OidcClientId:                   state.OIDCClientID,
		OidcClientSecretConfigured:     strings.TrimSpace(state.OIDCClientSecret) != "",
		OidcAllowedEmailDomains:        append([]string(nil), state.OIDCAllowedEmailDomains...),
		ServerDomain:                   state.ServerDomain,
		BaseDomain:                     state.BaseDomain,
		DomainPrefix:                   state.DomainPrefix,
		PasswordLoginDisabled:          state.PasswordLoginDisabled,
		IapEnabled:                     state.IAPEnabled,
		IapAudience:                    state.IAPAudience,
		IapAutoProvisioning:            state.IAPAutoProvisioning,
		OidcAutoProvisioning:           state.OIDCAutoProvisioning,
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
	return db.NormalizeMachineTemplate(runtime)
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

func (s *setupConnectService) authenticateAdmin(ctx context.Context, header http.Header) (string, error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, header)
	if err != nil {
		return "", err
	}
	if result.Role != db.UserRoleAdmin {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only admin can update setup settings"))
	}
	return result.UserID, nil
}

func shouldSkipSetup() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ARCA_SKIP_SETUP")))
	return value == "1" || value == "true" || value == "yes"
}

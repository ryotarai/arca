package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/ryotarai/arca/internal/db"
	"golang.org/x/crypto/argon2"
	"golang.org/x/oauth2"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailAlreadyUsed   = errors.New("email already used")
	ErrInvalidInput       = errors.New("invalid input")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidSetupToken  = errors.New("invalid setup token")
	ErrOIDCNotConfigured  = errors.New("oidc not configured")
	ErrOIDCRejected       = errors.New("oidc login rejected")
)

type Service struct {
	store             *db.Store
	sessionTTL        time.Duration
	userSetupTokenTTL time.Duration
	now               func() time.Time
	staticAPIToken    string
}

func NewService(store *db.Store) *Service {
	return &Service{
		store:             store,
		sessionTTL:        7 * 24 * time.Hour,
		userSetupTokenTTL: 24 * time.Hour,
		now:               time.Now,
	}
}

// SetStaticAPIToken configures a static API token for non-browser clients.
// When set, requests with this token in the Authorization header are
// authenticated as the first admin user.
func (s *Service) SetStaticAPIToken(token string) {
	s.staticAPIToken = token
}

func (s *Service) Register(ctx context.Context, email, password string) (string, string, error) {
	email, password, err := validateAndNormalize(email, password)
	if err != nil {
		return "", "", err
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return "", "", err
	}

	userID, err := randomID()
	if err != nil {
		return "", "", err
	}

	if err := s.store.CreateUser(ctx, userID, email, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return "", "", ErrEmailAlreadyUsed
		}
		return "", "", err
	}

	return userID, email, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (string, string, string, string, time.Time, error) {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return "", "", "", "", time.Time{}, ErrInvalidCredentials
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", "", "", time.Time{}, ErrInvalidCredentials
		}
		return "", "", "", "", time.Time{}, err
	}
	if user.PasswordSetupRequired {
		return "", "", "", "", time.Time{}, ErrInvalidCredentials
	}

	ok, err := verifyPassword(user.PasswordHash, password)
	if err != nil {
		return "", "", "", "", time.Time{}, err
	}
	if !ok {
		return "", "", "", "", time.Time{}, ErrInvalidCredentials
	}

	sessionToken, expiresAt, err := s.createSession(ctx, user.ID)
	if err != nil {
		return "", "", "", "", time.Time{}, err
	}

	return user.ID, user.Email, user.Role, sessionToken, expiresAt, nil
}

func (s *Service) ListUsers(ctx context.Context) ([]db.ManagedUser, error) {
	return s.store.ListUsers(ctx, s.now().Unix())
}

func (s *Service) ProvisionUser(ctx context.Context, email, createdByUserID string) (string, string, string, time.Time, error) {
	email = normalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return "", "", "", time.Time{}, ErrInvalidInput
	}

	initialPassword, err := randomToken()
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	passwordHash, err := hashPassword(initialPassword)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	userID, err := randomID()
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	if err := s.store.CreateUser(ctx, userID, email, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return "", "", "", time.Time{}, ErrEmailAlreadyUsed
		}
		return "", "", "", time.Time{}, err
	}
	setupToken, expiresAt, err := s.issueUserSetupToken(ctx, userID, createdByUserID)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	return userID, email, setupToken, expiresAt, nil
}

func (s *Service) IssueUserSetupToken(ctx context.Context, userID, createdByUserID string) (string, time.Time, error) {
	if strings.TrimSpace(userID) == "" {
		return "", time.Time{}, ErrInvalidInput
	}
	if _, err := s.store.GetUserByID(ctx, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", time.Time{}, ErrUserNotFound
		}
		return "", time.Time{}, err
	}
	return s.issueUserSetupToken(ctx, userID, createdByUserID)
}

func (s *Service) CompleteUserSetup(ctx context.Context, setupToken, password string) (string, string, error) {
	setupToken = strings.TrimSpace(setupToken)
	password = strings.TrimSpace(password)
	if setupToken == "" || len(password) < 8 {
		return "", "", ErrInvalidInput
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return "", "", err
	}

	user, err := s.store.CompleteUserSetup(ctx, hashSessionToken(setupToken), passwordHash, s.now().Unix())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrInvalidSetupToken
		}
		return "", "", err
	}
	return user.ID, user.Email, nil
}

func (s *Service) StartOIDCLogin(ctx context.Context, redirectURI, state string) (string, error) {
	config, err := s.loadOIDCConfig(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(state) == "" {
		return "", ErrInvalidInput
	}
	oauthConfig := oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  strings.TrimSpace(redirectURI),
		Endpoint: oauth2.Endpoint{
			AuthURL:  strings.TrimSpace(config.AuthURL),
			TokenURL: strings.TrimSpace(config.TokenURL),
		},
		Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return oauthConfig.AuthCodeURL(state), nil
}

func (s *Service) LoginWithOIDCCode(ctx context.Context, code, redirectURI string) (string, string, string, string, time.Time, error) {
	config, err := s.loadOIDCConfig(ctx)
	if err != nil {
		return "", "", "", "", time.Time{}, err
	}
	if strings.TrimSpace(code) == "" {
		return "", "", "", "", time.Time{}, ErrOIDCRejected
	}
	oauthConfig := oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  strings.TrimSpace(redirectURI),
		Endpoint: oauth2.Endpoint{
			AuthURL:  strings.TrimSpace(config.AuthURL),
			TokenURL: strings.TrimSpace(config.TokenURL),
		},
		Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}
	oauthToken, err := oauthConfig.Exchange(ctx, strings.TrimSpace(code))
	if err != nil {
		return "", "", "", "", time.Time{}, ErrOIDCRejected
	}
	rawIDToken, _ := oauthToken.Extra("id_token").(string)
	if strings.TrimSpace(rawIDToken) == "" {
		return "", "", "", "", time.Time{}, ErrOIDCRejected
	}
	verifier := oidc.NewVerifier(strings.TrimSpace(config.IssuerURL), oidc.NewRemoteKeySet(ctx, strings.TrimSpace(config.JWKSURL)), &oidc.Config{
		ClientID: config.ClientID,
	})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", "", "", "", time.Time{}, ErrOIDCRejected
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", "", "", "", time.Time{}, ErrOIDCRejected
	}
	email := normalizeEmail(claims.Email)
	if email == "" || !claims.EmailVerified || !isEmailDomainAllowed(email, config.AllowedEmailDomains) {
		return "", "", "", "", time.Time{}, ErrOIDCRejected
	}
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if !config.AutoProvisioning {
				return "", "", "", "", time.Time{}, ErrOIDCRejected
			}
			userID, createErr := s.autoProvisionUser(ctx, email)
			if createErr != nil {
				return "", "", "", "", time.Time{}, createErr
			}
			sessionToken, expiresAt, err := s.createSession(ctx, userID)
			if err != nil {
				return "", "", "", "", time.Time{}, err
			}
			return userID, email, "user", sessionToken, expiresAt, nil
		}
		return "", "", "", "", time.Time{}, err
	}
	sessionToken, expiresAt, err := s.createSession(ctx, user.ID)
	if err != nil {
		return "", "", "", "", time.Time{}, err
	}
	return user.ID, user.Email, user.Role, sessionToken, expiresAt, nil
}

// AuthResult holds the effective user identity after authentication.
type AuthResult struct {
	UserID         string
	Email          string
	Role           string
	IsNonAdminMode bool
	IsStaticToken  bool
}

func (s *Service) Authenticate(ctx context.Context, sessionToken string) (string, string, string, error) {
	r, err := s.AuthenticateFull(ctx, sessionToken)
	if err != nil {
		return "", "", "", err
	}
	return r.UserID, r.Email, r.Role, nil
}

func (s *Service) AuthenticateFull(ctx context.Context, sessionToken string) (AuthResult, error) {
	if sessionToken == "" {
		return AuthResult{}, ErrUnauthenticated
	}
	// Check static API token first (dev/scripting use).
	if s.staticAPIToken != "" && subtle.ConstantTimeCompare([]byte(sessionToken), []byte(s.staticAPIToken)) == 1 {
		id, email, role, err := s.authenticateAsFirstAdmin(ctx)
		if err != nil {
			return AuthResult{}, err
		}
		return AuthResult{UserID: id, Email: email, Role: role, IsStaticToken: true}, nil
	}
	tokenHash := hashSessionToken(sessionToken)
	user, err := s.store.GetUserByActiveSessionTokenHash(ctx, tokenHash, s.now().Unix())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AuthResult{}, ErrUnauthenticated
		}
		return AuthResult{}, err
	}

	return AuthResult{UserID: user.ID, Email: user.Email, Role: user.Role}, nil
}

func (s *Service) authenticateAsFirstAdmin(ctx context.Context) (string, string, string, error) {
	admin, err := s.store.GetFirstAdmin(ctx)
	if err != nil {
		return "", "", "", fmt.Errorf("static API token: %w", ErrUnauthenticated)
	}
	return admin.ID, admin.Email, admin.Role, nil
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if sessionToken == "" {
		return nil
	}
	tokenHash := hashSessionToken(sessionToken)
	if user, err := s.store.GetUserByActiveSessionTokenHash(ctx, tokenHash, s.now().Unix()); err == nil {
		_ = s.store.DeleteArcadSessionsByUserID(ctx, user.ID)
	}
	return s.store.RevokeSessionByTokenHash(ctx, tokenHash)
}

type oidcConfig struct {
	IssuerURL           string
	AuthURL             string
	TokenURL            string
	JWKSURL             string
	ClientID            string
	ClientSecret        string
	AllowedEmailDomains []string
	AutoProvisioning    bool
}

func (s *Service) loadOIDCConfig(ctx context.Context) (oidcConfig, error) {
	setup, err := s.store.GetSetupState(ctx)
	if err != nil {
		return oidcConfig{}, err
	}
	if !setup.OIDCEnabled {
		return oidcConfig{}, ErrOIDCNotConfigured
	}
	issuerURL, err := validateOIDCIssuerURL(setup.OIDCIssuerURL)
	if err != nil {
		return oidcConfig{}, ErrOIDCNotConfigured
	}
	clientID := strings.TrimSpace(setup.OIDCClientID)
	clientSecret := strings.TrimSpace(setup.OIDCClientSecret)
	if clientID == "" || clientSecret == "" {
		return oidcConfig{}, ErrOIDCNotConfigured
	}
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return oidcConfig{}, ErrOIDCRejected
	}
	var discovery struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		JWKSURI               string `json:"jwks_uri"`
	}
	if err := provider.Claims(&discovery); err != nil {
		return oidcConfig{}, ErrOIDCRejected
	}
	if strings.TrimSpace(discovery.AuthorizationEndpoint) == "" || strings.TrimSpace(discovery.TokenEndpoint) == "" || strings.TrimSpace(discovery.JWKSURI) == "" {
		return oidcConfig{}, ErrOIDCRejected
	}
	return oidcConfig{
		IssuerURL:           issuerURL,
		AuthURL:             discovery.AuthorizationEndpoint,
		TokenURL:            discovery.TokenEndpoint,
		JWKSURL:             discovery.JWKSURI,
		ClientID:            clientID,
		ClientSecret:        clientSecret,
		AllowedEmailDomains: setup.OIDCAllowedEmailDomains,
		AutoProvisioning:    setup.OIDCAutoProvisioning,
	}, nil
}

func validateOIDCIssuerURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("issuer is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", errors.New("issuer must use https")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return "", errors.New("issuer host is required")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isEmailDomainAllowed(email string, allowedDomains []string) bool {
	idx := strings.LastIndex(email, "@")
	if idx <= 0 || idx == len(email)-1 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(email[idx+1:]))
	if domain == "" {
		return false
	}
	if len(allowedDomains) == 0 {
		return true
	}
	for _, allowed := range allowedDomains {
		if domain == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

func (s *Service) createSession(ctx context.Context, userID string) (string, time.Time, error) {
	sessionToken, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	tokenHash := hashSessionToken(sessionToken)
	sessionID, err := randomID()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().Add(s.sessionTTL)
	if err := s.store.CreateSession(ctx, sessionID, userID, tokenHash, expiresAt.Unix()); err != nil {
		return "", time.Time{}, err
	}
	return sessionToken, expiresAt, nil
}

func validateAndNormalize(email, password string) (string, string, error) {
	email = normalizeEmail(email)
	password = strings.TrimSpace(password)

	if email == "" || !strings.Contains(email, "@") {
		return "", "", ErrInvalidInput
	}
	if len(password) < 8 {
		return "", "", ErrInvalidInput
	}

	return email, password, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) issueUserSetupToken(ctx context.Context, userID, createdByUserID string) (string, time.Time, error) {
	setupToken, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	tokenID, err := randomID()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().Add(s.userSetupTokenTTL)
	if err := s.store.IssueUserSetupToken(
		ctx,
		tokenID,
		hashSessionToken(setupToken),
		userID,
		strings.TrimSpace(createdByUserID),
		expiresAt.Unix(),
		s.now().Unix(),
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", time.Time{}, ErrUserNotFound
		}
		return "", time.Time{}, err
	}
	return setupToken, expiresAt, nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	const (
		memory      = 64 * 1024
		iterations  = 3
		parallelism = 1
		keyLength   = 32
	)

	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	return fmt.Sprintf(
		"argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(encodedHash, password string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 5 || parts[0] != "argon2id" {
		return false, errors.New("invalid password hash format")
	}

	if parts[1] != fmt.Sprintf("v=%d", argon2.Version) {
		return false, errors.New("unsupported argon2 version")
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8

	for _, param := range strings.Split(parts[2], ",") {
		keyValue := strings.SplitN(param, "=", 2)
		if len(keyValue) != 2 {
			return false, errors.New("invalid argon2 params")
		}

		switch keyValue[0] {
		case "m":
			value, err := strconv.ParseUint(keyValue[1], 10, 32)
			if err != nil {
				return false, err
			}
			memory = uint32(value)
		case "t":
			value, err := strconv.ParseUint(keyValue[1], 10, 32)
			if err != nil {
				return false, err
			}
			iterations = uint32(value)
		case "p":
			value, err := strconv.ParseUint(keyValue[1], 10, 8)
			if err != nil {
				return false, err
			}
			parallelism = uint8(value)
		default:
			return false, errors.New("unknown argon2 parameter")
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, err
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	computed := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(expectedHash, computed) == 1, nil
}

func (s *Service) autoProvisionUser(ctx context.Context, email string) (string, error) {
	userID, err := randomID()
	if err != nil {
		return "", err
	}
	password, err := randomToken()
	if err != nil {
		return "", err
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return "", err
	}
	if err := s.store.CreateUser(ctx, userID, email, passwordHash); err != nil {
		if isUniqueViolation(err) {
			// Race: user was created between check and insert; fetch existing.
			user, getErr := s.store.GetUserByEmail(ctx, email)
			if getErr != nil {
				return "", getErr
			}
			return user.ID, nil
		}
		return "", err
	}
	return userID, nil
}

func isUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

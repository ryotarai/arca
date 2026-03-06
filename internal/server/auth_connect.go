package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type authConnectService struct {
	authenticator Authenticator
}

func newAuthConnectService(authenticator Authenticator) *authConnectService {
	return &authConnectService{authenticator: authenticator}
}

func (s *authConnectService) Login(ctx context.Context, req *connect.Request[arcav1.LoginRequest]) (*connect.Response[arcav1.LoginResponse], error) {
	if s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("auth unavailable"))
	}

	userID, email, role, token, expiresAt, err := s.authenticator.Login(ctx, req.Msg.GetEmail(), req.Msg.GetPassword())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
		default:
			log.Printf("login failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to login"))
		}
	}

	resp := connect.NewResponse(&arcav1.LoginResponse{User: &arcav1.User{Id: userID, Email: email, Role: role}})
	setSessionCookie(resp.Header(), token, expiresAt, isSecureRequest(req.Header()))
	return resp, nil
}

func (s *authConnectService) StartOidcLogin(ctx context.Context, req *connect.Request[arcav1.StartOidcLoginRequest]) (*connect.Response[arcav1.StartOidcLoginResponse], error) {
	if s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("auth unavailable"))
	}
	state, err := randomCookieToken(24)
	if err != nil {
		log.Printf("generate oidc state failed: %v", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to prepare oidc login"))
	}
	authURL, err := s.authenticator.StartOIDCLogin(ctx, req.Msg.GetRedirectUri(), state)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrOIDCNotConfigured):
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("oidc is not configured"))
		case errors.Is(err, auth.ErrInvalidInput):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid oidc login request"))
		default:
			log.Printf("start oidc login failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to start oidc login"))
		}
	}
	resp := connect.NewResponse(&arcav1.StartOidcLoginResponse{AuthorizationUrl: authURL})
	setOIDCStateCookie(resp.Header(), state, time.Now().Add(10*time.Minute), isSecureRequest(req.Header()))
	return resp, nil
}

func (s *authConnectService) CompleteOidcLogin(ctx context.Context, req *connect.Request[arcav1.CompleteOidcLoginRequest]) (*connect.Response[arcav1.CompleteOidcLoginResponse], error) {
	if s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("auth unavailable"))
	}
	stateCookie, err := oidcStateFromHeader(req.Header())
	if err != nil || stateCookie == "" || stateCookie != strings.TrimSpace(req.Msg.GetState()) {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid oidc state"))
	}
	userID, email, role, token, expiresAt, err := s.authenticator.LoginWithOIDCCode(ctx, req.Msg.GetCode(), req.Msg.GetRedirectUri())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrOIDCNotConfigured):
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("oidc is not configured"))
		case errors.Is(err, auth.ErrOIDCRejected):
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("oidc login rejected"))
		default:
			log.Printf("complete oidc login failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to complete oidc login"))
		}
	}
	resp := connect.NewResponse(&arcav1.CompleteOidcLoginResponse{User: &arcav1.User{Id: userID, Email: email, Role: role}})
	secure := isSecureRequest(req.Header())
	setSessionCookie(resp.Header(), token, expiresAt, secure)
	clearOIDCStateCookie(resp.Header(), secure)
	return resp, nil
}

func (s *authConnectService) Logout(ctx context.Context, req *connect.Request[arcav1.LogoutRequest]) (*connect.Response[arcav1.LogoutResponse], error) {
	if s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("auth unavailable"))
	}

	sessionToken, _ := sessionTokenFromHeader(req.Header())
	if sessionToken != "" {
		_ = s.authenticator.Logout(ctx, sessionToken)
	}

	resp := connect.NewResponse(&arcav1.LogoutResponse{Status: "ok"})
	clearSessionCookie(resp.Header(), isSecureRequest(req.Header()))
	return resp, nil
}

func (s *authConnectService) Me(ctx context.Context, req *connect.Request[arcav1.MeRequest]) (*connect.Response[arcav1.MeResponse], error) {
	if s.authenticator == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("auth unavailable"))
	}

	sessionToken, err := sessionTokenFromHeader(req.Header())
	if err != nil || sessionToken == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	userID, email, role, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthenticated):
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
		default:
			log.Printf("authenticate failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authenticate"))
		}
	}

	return connect.NewResponse(&arcav1.MeResponse{User: &arcav1.User{Id: userID, Email: email, Role: role}}), nil
}

func sessionTokenFromHeader(header http.Header) (string, error) {
	req := &http.Request{Header: header}
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func oidcStateFromHeader(header http.Header) (string, error) {
	req := &http.Request{Header: header}
	cookie, err := req.Cookie(oidcStateCookieName)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(cookie.Value), nil
}

func setSessionCookie(header http.Header, token string, expiresAt time.Time, secure bool) {
	header.Add("Set-Cookie", (&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  expiresAt,
	}).String())
}

func clearSessionCookie(header http.Header, secure bool) {
	header.Add("Set-Cookie", (&http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}).String())
}

func setOIDCStateCookie(header http.Header, state string, expiresAt time.Time, secure bool) {
	header.Add("Set-Cookie", (&http.Cookie{
		Name:     oidcStateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  expiresAt,
	}).String())
}

func clearOIDCStateCookie(header http.Header, secure bool) {
	header.Add("Set-Cookie", (&http.Cookie{
		Name:     oidcStateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}).String())
}

func randomCookieToken(n int) (string, error) {
	if n <= 0 {
		n = 24
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func isSecureRequest(header http.Header) bool {
	if strings.EqualFold(header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	for _, forwarded := range header.Values("Forwarded") {
		if strings.Contains(strings.ToLower(forwarded), "proto=https") {
			return true
		}
	}
	return false
}

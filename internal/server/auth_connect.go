package server

import (
	"context"
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

	userID, email, token, expiresAt, err := s.authenticator.Login(ctx, req.Msg.GetEmail(), req.Msg.GetPassword())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
		default:
			log.Printf("login failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to login"))
		}
	}

	resp := connect.NewResponse(&arcav1.LoginResponse{User: &arcav1.User{Id: userID, Email: email}})
	setSessionCookie(resp.Header(), token, expiresAt, isSecureRequest(req.Header()))
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

	userID, email, err := s.authenticator.Authenticate(ctx, sessionToken)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthenticated):
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
		default:
			log.Printf("authenticate failed: %v", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authenticate"))
		}
	}

	return connect.NewResponse(&arcav1.MeResponse{User: &arcav1.User{Id: userID, Email: email}}), nil
}

func sessionTokenFromHeader(header http.Header) (string, error) {
	req := &http.Request{Header: header}
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
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

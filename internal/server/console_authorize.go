package server

import (
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

func newConsoleAuthorizeHandler(store *db.Store, authenticator Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if store == nil || authenticator == nil {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		targetRaw := strings.TrimSpace(r.URL.Query().Get("target"))
		if targetRaw == "" {
			http.Error(w, "target is required", http.StatusBadRequest)
			return
		}
		targetURL, err := url.Parse(targetRaw)
		if err != nil || !targetURL.IsAbs() || strings.TrimSpace(targetURL.Host) == "" {
			http.Error(w, "invalid target", http.StatusBadRequest)
			return
		}

		userID, err := userIDFromSessionCookie(r, authenticator)
		if err != nil {
			loginURL := url.URL{Path: "/login"}
			q := loginURL.Query()
			q.Set("next", r.URL.RequestURI())
			loginURL.RawQuery = q.Encode()
			http.Redirect(w, r, loginURL.String(), http.StatusFound)
			return
		}

		exposureHost := stripPort(targetURL.Host)
		exposure, err := store.GetMachineExposureByHostname(r.Context(), exposureHost)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			log.Printf("console authorize exposure lookup failed: %v", err)
			http.Error(w, "failed to resolve exposure", http.StatusInternalServerError)
			return
		}

		if !canUserAccessExposure(r.Context(), store, exposure, userID, targetURL.Path) {
			http.NotFound(w, r)
			return
		}

		expiresAt := time.Now().Add(authTicketTTL)
		token, err := store.CreateArcadExchangeToken(r.Context(), userID, exposure.MachineID, exposure.ID, expiresAt.Unix())
		if err != nil {
			log.Printf("console authorize arcad token issue failed: %v", err)
			http.Error(w, "failed to issue token", http.StatusInternalServerError)
			return
		}

		query := targetURL.Query()
		query.Set("token", token)
		targetURL.RawQuery = query.Encode()
		http.Redirect(w, r, targetURL.String(), http.StatusFound)
	}
}

func userIDFromSessionCookie(r *http.Request, authenticator Authenticator) (string, error) {
	sessionToken, _ := sessionTokenFromHeader(r.Header)
	if sessionToken != "" {
		userID, _, _, err := authenticator.Authenticate(r.Context(), sessionToken)
		if err == nil {
			return userID, nil
		}
	}

	if iapJWT := iapJWTFromHeader(r.Header); iapJWT != "" {
		userID, _, _, err := authenticator.AuthenticateIAPJWT(r.Context(), iapJWT)
		if err == nil {
			return userID, nil
		}
	}

	return "", errors.New("unauthenticated")
}

func stripPort(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		return h
	}
	return host
}

package server

import (
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
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

		if !canUserAccessExposure(r, store, exposure, userID, targetURL.Path) {
			http.NotFound(w, r)
			return
		}

		expiresAt := time.Now().Add(authTicketTTL)
		ticket, err := store.CreateAuthTicket(r.Context(), userID, exposure.MachineID, exposure.ID, expiresAt.Unix())
		if err != nil {
			log.Printf("console authorize ticket issue failed: %v", err)
			http.Error(w, "failed to issue token", http.StatusInternalServerError)
			return
		}

		query := targetURL.Query()
		query.Set("token", ticket)
		targetURL.RawQuery = query.Encode()
		http.Redirect(w, r, targetURL.String(), http.StatusFound)
	}
}

func canUserAccessExposure(r *http.Request, store *db.Store, exposure db.MachineExposure, userID, targetPath string) bool {
	if isOwnerOnlyArcaPath(targetPath) {
		_, err := store.GetMachineByIDForUser(r.Context(), userID, exposure.MachineID)
		return err == nil
	}

	if db.NormalizeEndpointVisibility(exposure.Visibility) == db.EndpointVisibilityAllArcaUsers || db.NormalizeEndpointVisibility(exposure.Visibility) == db.EndpointVisibilityInternetPublic {
		return true
	}
	if _, err := store.GetMachineByIDForUser(r.Context(), userID, exposure.MachineID); err == nil {
		return true
	}
	if db.NormalizeEndpointVisibility(exposure.Visibility) == db.EndpointVisibilitySelectedUsers {
		return slices.Contains(exposure.SelectedUserIDs, userID)
	}
	return false
}

func userIDFromSessionCookie(r *http.Request, authenticator Authenticator) (string, error) {
	sessionToken, err := sessionTokenFromHeader(r.Header)
	if err != nil || sessionToken == "" {
		return "", errors.New("unauthenticated")
	}
	userID, _, _, err := authenticator.Authenticate(r.Context(), sessionToken)
	if err != nil {
		return "", err
	}
	return userID, nil
}

func stripPort(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		return h
	}
	return host
}

func isOwnerOnlyArcaPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == "/__arca/readyz" {
		return false
	}
	return path == "/__arca" || strings.HasPrefix(path, "/__arca/")
}

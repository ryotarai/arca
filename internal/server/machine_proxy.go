package server

import (
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/ryotarai/arca/internal/db"
)

// MachineProxyHandler handles HTTP requests for machines exposed via
// "proxy via server" mode. It looks up the machine exposure by hostname
// (from the Host header), resolves the machine's upstream address, and
// reverse-proxies the request.
type MachineProxyHandler struct {
	store         *db.Store
	authenticator Authenticator
}

func NewMachineProxyHandler(store *db.Store, authenticator Authenticator) *MachineProxyHandler {
	return &MachineProxyHandler{store: store, authenticator: authenticator}
}

// TryServeHTTP attempts to handle the request as a machine proxy request.
// Returns true if the request was handled (the Host matched a machine exposure).
func (h *MachineProxyHandler) TryServeHTTP(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.store == nil {
		return false
	}

	hostname := extractHostname(r.Host)
	if hostname == "" {
		return false
	}

	exposure, err := h.store.GetMachineExposureByHostname(r.Context(), hostname)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		log.Printf("machine proxy: lookup exposure by hostname %q failed: %v", hostname, err)
		return false
	}

	// Check if this machine uses proxy-via-server exposure
	machine, err := h.store.GetMachineByID(r.Context(), exposure.MachineID)
	if err != nil {
		log.Printf("machine proxy: lookup machine %q failed: %v", exposure.MachineID, err)
		http.Error(w, "machine not found", http.StatusBadGateway)
		return true
	}

	runtimeCatalog, err := h.store.GetRuntimeByID(r.Context(), machine.RuntimeID)
	if err != nil {
		log.Printf("machine proxy: lookup runtime %q failed: %v", machine.RuntimeID, err)
		http.Error(w, "runtime not found", http.StatusBadGateway)
		return true
	}

	exposureMethod := db.GetRuntimeExposureMethod(runtimeCatalog.ConfigJSON)
	if exposureMethod != db.MachineExposureMethodProxyViaServer {
		return false
	}

	// Access control
	userID := h.authenticateRequest(r)
	if !canUserAccessExposure(r.Context(), h.store, exposure, userID, r.URL.Path) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return true
	}

	// Resolve the upstream URL for the machine
	upstreamURL := resolveUpstreamURL(machine, exposure)
	if upstreamURL == "" {
		http.Error(w, "machine upstream unavailable", http.StatusBadGateway)
		return true
	}

	target, err := url.Parse(upstreamURL)
	if err != nil {
		log.Printf("machine proxy: parse upstream url %q failed: %v", upstreamURL, err)
		http.Error(w, "invalid upstream", http.StatusBadGateway)
		return true
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
		},
	}
	proxy.ServeHTTP(w, r)
	return true
}

func (h *MachineProxyHandler) authenticateRequest(r *http.Request) string {
	if h.authenticator == nil {
		return ""
	}
	sessionToken, err := sessionTokenFromHeader(r.Header)
	if err != nil || sessionToken == "" {
		cookie, cookieErr := r.Cookie(sessionCookieName)
		if cookieErr != nil || cookie.Value == "" {
			return ""
		}
		sessionToken = cookie.Value
	}
	userID, _, _, err := h.authenticator.Authenticate(r.Context(), sessionToken)
	if err != nil {
		return ""
	}
	return userID
}

func resolveUpstreamURL(machine db.Machine, exposure db.MachineExposure) string {
	service := strings.TrimSpace(exposure.Service)
	if service == "" {
		service = "http://localhost:11030"
	}

	serviceURL, err := url.Parse(service)
	if err != nil {
		return ""
	}
	port := serviceURL.Port()
	if port == "" {
		port = "11030"
	}

	// For proxy-via-server, we need the machine's actual IP.
	// The machine endpoint stores the hostname for DNS-based access.
	// The machine's IP is resolved at runtime - for now check if endpoint is an IP.
	if machine.Endpoint != "" {
		if ip := net.ParseIP(machine.Endpoint); ip != nil {
			return "http://" + net.JoinHostPort(machine.Endpoint, port)
		}
	}

	return ""
}

func extractHostname(host string) string {
	hostname := strings.TrimSpace(host)
	if hostname == "" {
		return ""
	}
	h, _, err := net.SplitHostPort(hostname)
	if err != nil {
		return strings.ToLower(hostname)
	}
	return strings.ToLower(h)
}

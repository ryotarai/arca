package server

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/machine"
)

const proxySessionCookieName = "arca_proxy_session"
const proxyCallbackPath = "/__arca_proxy_callback"
const proxySessionTTL = 8 * time.Hour

// MachineProxyHandler handles HTTP requests for machines exposed via
// "proxy via server" mode. It looks up the machine exposure by hostname
// (from the Host header), resolves the machine's upstream address, and
// reverse-proxies the request.
type MachineProxyHandler struct {
	store         *db.Store
	authenticator Authenticator
	ipCache       *machine.MachineIPCache
}

func NewMachineProxyHandler(store *db.Store, authenticator Authenticator, ipCache *machine.MachineIPCache) *MachineProxyHandler {
	return &MachineProxyHandler{store: store, authenticator: authenticator, ipCache: ipCache}
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
	m, err := h.store.GetMachineByID(r.Context(), exposure.MachineID)
	if err != nil {
		log.Printf("machine proxy: lookup machine %q failed: %v", exposure.MachineID, err)
		http.Error(w, "machine not found", http.StatusBadGateway)
		return true
	}

	runtimeCatalog, err := h.store.GetRuntimeByID(r.Context(), m.RuntimeID)
	if err != nil {
		log.Printf("machine proxy: lookup runtime %q failed: %v", m.RuntimeID, err)
		http.Error(w, "runtime not found", http.StatusBadGateway)
		return true
	}

	exposureMethod := db.GetRuntimeExposureMethod(runtimeCatalog.ConfigJSON)
	if exposureMethod != db.MachineExposureMethodProxyViaServer {
		return false
	}

	// Handle proxy auth callback
	if r.URL.Path == proxyCallbackPath {
		h.handleProxyCallback(w, r, exposure)
		return true
	}

	// Access control: try regular session first, then proxy session
	userID := h.authenticateRequest(r)
	if userID == "" {
		userID = h.authenticateProxySession(r, exposure.MachineID)
	}
	if !canUserAccessExposure(r.Context(), h.store, exposure, userID, r.URL.Path) {
		h.redirectToAuthorize(w, r)
		return true
	}

	// Resolve the upstream URL for the machine via IP cache
	var machineInfo *machine.RuntimeMachineInfo
	if h.ipCache != nil {
		var infoErr error
		machineInfo, infoErr = h.ipCache.Get(r.Context(), m)
		if infoErr != nil {
			log.Printf("machine proxy: ip cache lookup for %q failed: %v", m.ID, infoErr)
		}
	}
	connectivity := db.GetRuntimeExposureConfig(runtimeCatalog.ConfigJSON).Connectivity
	upstreamURL := resolveUpstreamURL(machineInfo, m, exposure, connectivity)
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

// authenticateProxySession checks for a proxy session cookie and validates
// it as an arcad session tied to the given machine.
func (h *MachineProxyHandler) authenticateProxySession(r *http.Request, machineID string) string {
	cookie, err := r.Cookie(proxySessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return ""
	}
	session, err := h.store.GetActiveArcadSessionByMachineID(r.Context(), machineID, cookie.Value, time.Now().Unix())
	if err != nil {
		return ""
	}
	return session.UserID
}

// handleProxyCallback exchanges a short-lived token for a proxy session cookie.
func (h *MachineProxyHandler) handleProxyCallback(w http.ResponseWriter, r *http.Request, exposure db.MachineExposure) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	now := time.Now()
	session, err := h.store.ExchangeArcadTokenByMachineID(r.Context(), exposure.MachineID, token, now.Unix(), now.Add(proxySessionTTL).Unix())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		log.Printf("machine proxy: token exchange failed for exposure %q: %v", exposure.ID, err)
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}

	nextPath := sanitizeProxyNext(r.URL.Query().Get("next"))
	http.SetCookie(w, &http.Cookie{
		Name:     proxySessionCookieName,
		Value:    session.SessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureProxyRequest(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(session.ExpiresAt, 0),
	})
	http.Redirect(w, r, nextPath, http.StatusFound)
}

// redirectToAuthorize redirects the user to /console/authorize on the main
// server domain, which will authenticate via the arca_session cookie and
// redirect back with a short-lived token.
func (h *MachineProxyHandler) redirectToAuthorize(w http.ResponseWriter, r *http.Request) {
	serverDomain, err := h.resolveServerDomain(r)
	if err != nil {
		log.Printf("machine proxy: cannot resolve server domain for authorize redirect: %v", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Build the callback URL on the machine hostname
	ext := proxyExternalURL(r)
	callbackURL := url.URL{
		Scheme: ext.Scheme,
		Host:   ext.Host,
		Path:   proxyCallbackPath,
	}
	q := callbackURL.Query()
	q.Set("next", ext.RequestURI())
	callbackURL.RawQuery = q.Encode()

	// Build the authorize URL on the main server domain
	authorizeURL := fmt.Sprintf("%s://%s/console/authorize?target=%s", ext.Scheme, serverDomain, url.QueryEscape(callbackURL.String()))
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

func (h *MachineProxyHandler) resolveServerDomain(r *http.Request) (string, error) {
	state, err := h.store.GetSetupState(r.Context())
	if err != nil {
		return "", err
	}
	domain := strings.TrimSpace(state.ServerDomain)
	if domain == "" {
		return "", fmt.Errorf("server domain not configured")
	}
	return domain, nil
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

func resolveUpstreamURL(info *machine.RuntimeMachineInfo, m db.Machine, exposure db.MachineExposure, connectivity string) string {
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

	// Determine the target IP based on the runtime's connectivity setting.
	var ip string
	if info != nil {
		conn := strings.ToLower(strings.TrimSpace(connectivity))
		switch {
		case conn == "public_ip" || strings.HasSuffix(conn, "_public_ip"):
			ip = info.PublicIP
			if ip == "" {
				ip = info.PrivateIP
			}
		default:
			ip = info.PrivateIP
		}
	}

	if ip != "" {
		return "http://" + net.JoinHostPort(ip, port)
	}

	// Legacy fallback: check if endpoint is an IP address.
	if m.Endpoint != "" {
		if parsed := net.ParseIP(m.Endpoint); parsed != nil {
			return "http://" + net.JoinHostPort(m.Endpoint, port)
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

func proxyExternalURL(r *http.Request) url.URL {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	if host == "" {
		host = "localhost"
	}
	return url.URL{Scheme: scheme, Host: host, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
}

func sanitizeProxyNext(next string) string {
	if next == "" {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil {
		return "/"
	}
	if u.IsAbs() {
		return "/"
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	if u.RawQuery == "" {
		return u.Path
	}
	return fmt.Sprintf("%s?%s", u.Path, u.RawQuery)
}

func isSecureProxyRequest(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return r.TLS != nil
}

package arcad

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

const callbackPath = "/callback"
const claudecodeuiBasePath = "/__arca/claudecodeui"
const ttydBasePath = "/__arca/ttyd"
const shelleyBasePath = "/__arca/shelley"
const readyPath = "/__arca/readyz"

type Proxy struct {
	cache         *ExposureCache
	controlPlane  ControlPlaneClient
	sessions      *SessionManager
	upstream      *url.URL
	claudecodeui  *url.URL
	ttyd          *url.URL
	shelley       *url.URL
	ttydSocket    string
	sessionCookie string
	sessionMaxAge time.Duration
	readiness     *ReadinessChecker
}

func NewProxy(cache *ExposureCache, controlPlane ControlPlaneClient, sessions *SessionManager, sessionCookie string, upstream *url.URL, ttydSocket string) *Proxy {
	if upstream == nil {
		upstream = &url.URL{Scheme: "http", Host: "127.0.0.1:8080"}
	}
	claudecodeui := &url.URL{Scheme: "http", Host: "127.0.0.1:21031"}
	ttyd := &url.URL{Scheme: "http", Host: "unix"}
	shelley := &url.URL{Scheme: "http", Host: "127.0.0.1:21032"}
	return &Proxy{
		cache:         cache,
		controlPlane:  controlPlane,
		sessions:      sessions,
		upstream:      upstream,
		claudecodeui:  claudecodeui,
		ttyd:          ttyd,
		shelley:       shelley,
		ttydSocket:    ttydSocket,
		sessionCookie: sessionCookie,
		sessionMaxAge: 8 * time.Hour,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == readyPath {
		p.handleReady(w, r)
		return
	}

	host := stripPort(r.Host)
	exposure, err := p.cache.GetByHost(r.Context(), host)
	if err != nil {
		if errors.Is(err, ErrExposureNotFound) {
			http.NotFound(w, r)
			return
		}
		log.Printf("exposure lookup failed for host %q: %v", host, err)
		http.Error(w, "upstream lookup failed", http.StatusBadGateway)
		return
	}

	if !exposure.Public {
		if r.URL.Path == callbackPath {
			p.handleTicketCallback(w, r, host)
			return
		}
		if !p.isAuthenticated(r) {
			redirectTarget := callbackTargetURL(r)
			http.Redirect(w, r, p.controlPlane.AuthorizeURL(redirectTarget), http.StatusFound)
			return
		}
	}

	target := p.targetUpstream(r.URL.Path)
	proxy := httputil.NewSingleHostReverseProxy(target)
	if target == p.ttyd && p.ttydSocket != "" {
		proxy.Transport = ttydUnixSocketTransport(p.ttydSocket)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("reverse proxy error for host %q target %q: %v", host, target, err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

func ttydUnixSocketTransport(socketPath string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
}

func (p *Proxy) SetReadinessChecker(checker *ReadinessChecker) {
	p.readiness = checker
}

func (p *Proxy) handleReady(w http.ResponseWriter, r *http.Request) {
	if p.readiness == nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}
	if err := p.readiness.Ready(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (p *Proxy) targetUpstream(path string) *url.URL {
	if path == claudecodeuiBasePath || strings.HasPrefix(path, claudecodeuiBasePath+"/") {
		return p.claudecodeui
	}
	if path == ttydBasePath || strings.HasPrefix(path, ttydBasePath+"/") {
		return p.ttyd
	}
	if path == shelleyBasePath || strings.HasPrefix(path, shelleyBasePath+"/") {
		return p.shelley
	}
	return p.upstream
}

func (p *Proxy) handleTicketCallback(w http.ResponseWriter, r *http.Request, host string) {
	ticket := strings.TrimSpace(r.URL.Query().Get("token"))
	if ticket == "" {
		ticket = strings.TrimSpace(r.URL.Query().Get("ticket"))
	}
	if ticket == "" {
		http.Error(w, "missing ticket", http.StatusBadRequest)
		return
	}
	claims, err := p.controlPlane.VerifyTicket(r.Context(), host, ticket)
	if err != nil {
		if errors.Is(err, ErrInvalidTicket) {
			http.Error(w, "invalid ticket", http.StatusUnauthorized)
			return
		}
		log.Printf("ticket verification failed for host %q: %v", host, err)
		http.Error(w, "ticket verification failed", http.StatusBadGateway)
		return
	}
	expiresAt := claims.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(p.sessionMaxAge)
	}
	token, err := p.sessions.Encode(Session{UserID: claims.UserID, ExpiresAt: expiresAt})
	if err != nil {
		log.Printf("session encode failed for host %q: %v", host, err)
		http.Error(w, "failed to set session", http.StatusInternalServerError)
		return
	}

	nextPath := sanitizeNext(r.URL.Query().Get("next"))
	http.SetCookie(w, &http.Cookie{
		Name:     p.sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
	http.Redirect(w, r, nextPath, http.StatusFound)
}

func (p *Proxy) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(p.sessionCookie)
	if err != nil {
		return false
	}
	_, err = p.sessions.Decode(cookie.Value)
	return err == nil
}

func callbackTargetURL(r *http.Request) string {
	current := externalRequestURL(r)
	callback := url.URL{
		Scheme: current.Scheme,
		Host:   current.Host,
		Path:   callbackPath,
	}
	q := callback.Query()
	q.Set("next", current.RequestURI())
	callback.RawQuery = q.Encode()
	return callback.String()
}

func externalRequestURL(r *http.Request) url.URL {
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

func sanitizeNext(next string) string {
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

func stripPort(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		return h
	}
	return host
}

func isSecureRequest(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return r.TLS != nil
}

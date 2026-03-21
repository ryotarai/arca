package arcad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

const callbackPath = "/callback"
const ttydBasePath = "/__arca/ttyd"
const shelleyBasePath = "/__arca/shelley"
const readyPath = "/__arca/readyz"

type Proxy struct {
	cache         *ExposureCache
	controlPlane  ControlPlaneClient
	upstream *url.URL
	ttyd     *url.URL
	shelley       *url.URL
	ttydSocket    string
	sessionCookie string
	sessionMaxAge time.Duration
	sessionCache  *SessionValidationCache
	readiness     *ReadinessChecker
}

func NewProxy(cache *ExposureCache, controlPlane ControlPlaneClient, sessionCookie string, upstream *url.URL, ttydSocket string) *Proxy {
	if upstream == nil {
		upstream = &url.URL{Scheme: "http", Host: "127.0.0.1:11030"}
	}
	ttyd := &url.URL{Scheme: "http", Host: "unix"}
	shelley := &url.URL{Scheme: "http", Host: "127.0.0.1:21032"}
	return &Proxy{
		cache:        cache,
		controlPlane: controlPlane,
		upstream:     upstream,
		ttyd:         ttyd,
		shelley:       shelley,
		ttydSocket:    ttydSocket,
		sessionCookie: sessionCookie,
		sessionMaxAge: 8 * time.Hour,
		sessionCache:  NewSessionValidationCache(time.Minute),
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

	var claims *ArcadSessionClaims
	if !exposure.Public || isOwnerOnlyArcaPath(r.URL.Path) {
		if r.URL.Path == callbackPath {
			p.handleTicketCallback(w, r, host)
			return
		}
		c, ok := p.authenticatedClaims(r.Context(), r, host)
		if !ok {
			redirectTarget := callbackTargetURL(r)
			http.Redirect(w, r, p.controlPlane.AuthorizeURL(redirectTarget), http.StatusFound)
			return
		}
		claims = &c
	} else {
		// Public exposure: try to resolve user claims if a session cookie is present.
		if c, ok := p.authenticatedClaims(r.Context(), r, host); ok {
			claims = &c
		}
	}

	target := p.targetUpstream(r.URL.Path)
	proxy := httputil.NewSingleHostReverseProxy(target)
	if target == p.ttyd && p.ttydSocket != "" {
		proxy.Transport = ttydUnixSocketTransport(p.ttydSocket)
	}
	if target == p.shelley {
		proxy.ModifyResponse = rewriteShelleyHTMLResponse
	}
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		setUserHeaders(req, claims)
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
	claims, err := p.controlPlane.ExchangeArcadSession(r.Context(), host, ticket)
	if err != nil {
		if errors.Is(err, ErrInvalidTicket) {
			http.Error(w, "invalid ticket", http.StatusUnauthorized)
			return
		}
		log.Printf("session exchange failed for host %q: %v", host, err)
		http.Error(w, "session exchange failed", http.StatusBadGateway)
		return
	}
	expiresAt := claims.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(p.sessionMaxAge)
	}

	nextPath := sanitizeNext(r.URL.Query().Get("next"))
	http.SetCookie(w, &http.Cookie{
		Name:     p.sessionCookie,
		Value:    claims.SessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
	http.Redirect(w, r, nextPath, http.StatusFound)
}

func (p *Proxy) authenticatedClaims(ctx context.Context, r *http.Request, host string) (ArcadSessionClaims, bool) {
	cookie, err := r.Cookie(p.sessionCookie)
	if err != nil {
		return ArcadSessionClaims{}, false
	}
	sessionID := strings.TrimSpace(cookie.Value)
	if sessionID == "" {
		return ArcadSessionClaims{}, false
	}
	ownerOnlyArca := isOwnerOnlyArcaPath(r.URL.Path)
	if cached, ok := p.sessionCache.GetClaims(sessionID, host, ownerOnlyArca); ok {
		return cached, true
	}
	claims, err := p.controlPlane.ValidateArcadSession(ctx, host, r.URL.Path, sessionID)
	if err != nil {
		if errors.Is(err, ErrInvalidSession) {
			p.sessionCache.Invalidate(sessionID, host)
			return ArcadSessionClaims{}, false
		}
		log.Printf("session validation failed for host %q: %v", host, err)
		return ArcadSessionClaims{}, false
	}
	p.sessionCache.MarkValid(sessionID, host, ownerOnlyArca, claims)
	return claims, true
}

func setUserHeaders(req *http.Request, claims *ArcadSessionClaims) {
	if claims != nil {
		req.Header.Set("X-Arca-User-Id", claims.UserID)
		req.Header.Set("X-Arca-User-Email", claims.UserEmail)
	} else {
		req.Header.Del("X-Arca-User-Id")
		req.Header.Del("X-Arca-User-Email")
	}
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

func rewriteShelleyHTMLResponse(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	if !isShelleyPath(resp.Request.URL.Path) {
		return nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil
	}
	if strings.TrimSpace(resp.Header.Get("Content-Encoding")) != "" {
		return nil
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "text/html") {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	rewritten := rewriteShelleyAssetPaths(string(body))
	resp.Body = io.NopCloser(strings.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(rewritten)))
	return nil
}

func rewriteShelleyAssetPaths(indexHTML string) string {
	replacer := strings.NewReplacer(
		`href="/manifest.json"`, `href="`+shelleyBasePath+`/manifest.json"`,
		`href='/manifest.json'`, `href='`+shelleyBasePath+`/manifest.json'`,
		`href="/apple-touch-icon.png"`, `href="`+shelleyBasePath+`/apple-touch-icon.png"`,
		`href='/apple-touch-icon.png'`, `href='`+shelleyBasePath+`/apple-touch-icon.png'`,
		`href="/styles.css"`, `href="`+shelleyBasePath+`/styles.css"`,
		`href='/styles.css'`, `href='`+shelleyBasePath+`/styles.css'`,
		`href="/main.css"`, `href="`+shelleyBasePath+`/main.css"`,
		`href='/main.css'`, `href='`+shelleyBasePath+`/main.css'`,
		`src="/main.js"`, `src="`+shelleyBasePath+`/main.js"`,
		`src='/main.js'`, `src='`+shelleyBasePath+`/main.js'`,
	)
	return replacer.Replace(indexHTML)
}

func isShelleyPath(path string) bool {
	return path == shelleyBasePath || strings.HasPrefix(path, shelleyBasePath+"/")
}

func isOwnerOnlyArcaPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == readyPath {
		return false
	}
	return path == "/__arca" || strings.HasPrefix(path, "/__arca/")
}

package arcad

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type proxyStubControlPlane struct {
	exposure       Exposure
	exchangeClaims ArcadSessionClaims
	exchangeErr    error
	validateErr    error
}

func (s *proxyStubControlPlane) GetExposureByHost(_ context.Context, _ string) (Exposure, error) {
	return s.exposure, nil
}

func (s *proxyStubControlPlane) ExchangeArcadSession(_ context.Context, _, _ string) (ArcadSessionClaims, error) {
	if s.exchangeErr != nil {
		return ArcadSessionClaims{}, s.exchangeErr
	}
	return s.exchangeClaims, nil
}

func (s *proxyStubControlPlane) ValidateArcadSession(_ context.Context, _, _, sessionID string) (ArcadSessionClaims, error) {
	if s.validateErr != nil {
		return ArcadSessionClaims{}, s.validateErr
	}
	if strings.TrimSpace(sessionID) == "" || sessionID != s.exchangeClaims.SessionID {
		return ArcadSessionClaims{}, ErrInvalidSession
	}
	return ArcadSessionClaims{UserID: s.exchangeClaims.UserID, UserEmail: s.exchangeClaims.UserEmail}, nil
}

func (s *proxyStubControlPlane) ReportMachineReadiness(_ context.Context, _ bool, _, _, _ string) (bool, error) {
	return true, nil
}

func (s *proxyStubControlPlane) GetMachineLLMModels(_ context.Context) ([]MachineLLMModel, error) {
	return nil, nil
}

func (s *proxyStubControlPlane) AuthorizeURL(target string) string {
	return "https://control.example/auth/authorize?target=" + target
}

func TestProxyRedirectsUnauthenticatedPrivateExposure(t *testing.T) {
	cp := &proxyStubControlPlane{exposure: Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: false}}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, "http://127.0.0.1:11030"), "")

	req := httptest.NewRequest(http.MethodGet, "http://app.example/path?x=1", nil)
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://control.example/auth/authorize?target=") {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
}

func TestProxyCallbackSetsSessionCookie(t *testing.T) {
	cp := &proxyStubControlPlane{
		exposure: Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: false},
		exchangeClaims: ArcadSessionClaims{
			SessionID: "as_1",
			UserID:    "u1",
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, "http://127.0.0.1:11030"), "")

	req := httptest.NewRequest(http.MethodGet, "http://app.example/callback?token=tk_1&next=%2Fworkspace", nil)
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rr.Code)
	}
	if rr.Header().Get("Location") != "/workspace" {
		t.Fatalf("unexpected next redirect: %q", rr.Header().Get("Location"))
	}
	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie")
	}
	if cookies[0].Name != "arcad_session" || cookies[0].Value != "as_1" {
		t.Fatalf("unexpected cookie: %+v", cookies[0])
	}
}

func TestProxyRoutesTTydPathToDedicatedUnixSocket(t *testing.T) {
	defaultUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("default"))
	}))
	t.Cleanup(defaultUpstream.Close)

	socketPath := filepath.Join(t.TempDir(), "ttyd.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	t.Cleanup(func() {
		_ = ln.Close()
	})
	ttydUpstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ttyd"))
	})}
	go func() {
		_ = ttydUpstream.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = ttydUpstream.Close()
	})

	cp := &proxyStubControlPlane{
		exposure:       Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: true},
		exchangeClaims: ArcadSessionClaims{SessionID: "as_valid", UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)},
	}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, defaultUpstream.URL), socketPath)

	req := httptest.NewRequest(http.MethodGet, "http://app.example/__arca/ttyd/", nil)
	rr := httptest.NewRecorder()
	addValidSessionCookie(req)
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	body, err := io.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ttyd" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

func TestProxyRoutesShelleyPathToDedicatedUpstream(t *testing.T) {
	defaultUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("default"))
	}))
	t.Cleanup(defaultUpstream.Close)

	shelleyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("shelley"))
	}))
	t.Cleanup(shelleyUpstream.Close)

	cp := &proxyStubControlPlane{
		exposure:       Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: true},
		exchangeClaims: ArcadSessionClaims{SessionID: "as_valid", UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)},
	}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, defaultUpstream.URL), "")
	proxy.shelley = mustURL(t, shelleyUpstream.URL)

	req := httptest.NewRequest(http.MethodGet, "http://app.example/__arca/shelley/", nil)
	rr := httptest.NewRecorder()
	addValidSessionCookie(req)
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	body, err := io.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "shelley" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

func TestProxyRewritesShelleyIndexAssetPaths(t *testing.T) {
	defaultUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("default"))
	}))
	t.Cleanup(defaultUpstream.Close)

	shelleyIndex := `<!doctype html><html><head><link rel="manifest" href="/manifest.json"><link rel="apple-touch-icon" href="/apple-touch-icon.png"><link rel="stylesheet" href="/styles.css"><link rel="stylesheet" href="/main.css"></head><body><script type="module" src="/main.js"></script></body></html>`
	shelleyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(shelleyIndex))
	}))
	t.Cleanup(shelleyUpstream.Close)

	cp := &proxyStubControlPlane{
		exposure:       Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: true},
		exchangeClaims: ArcadSessionClaims{SessionID: "as_valid", UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)},
	}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, defaultUpstream.URL), "")
	proxy.shelley = mustURL(t, shelleyUpstream.URL)

	req := httptest.NewRequest(http.MethodGet, "http://app.example/__arca/shelley/", nil)
	rr := httptest.NewRecorder()
	addValidSessionCookie(req)
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	body, err := io.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	got := string(body)
	for _, want := range []string{
		`href="/__arca/shelley/manifest.json"`,
		`href="/__arca/shelley/apple-touch-icon.png"`,
		`href="/__arca/shelley/styles.css"`,
		`href="/__arca/shelley/main.css"`,
		`src="/__arca/shelley/main.js"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected rewritten asset path %q in %q", want, got)
		}
	}
}

func TestRewriteShelleyAssetPaths(t *testing.T) {
	input := `<link rel="manifest" href="/manifest.json"><link rel="apple-touch-icon" href='/apple-touch-icon.png'><link rel="stylesheet" href="/styles.css"><link rel="stylesheet" href='/main.css'><script src='/main.js'></script>`
	got := rewriteShelleyAssetPaths(input)
	for _, want := range []string{
		`href="/__arca/shelley/manifest.json"`,
		`href='/__arca/shelley/apple-touch-icon.png'`,
		`href="/__arca/shelley/styles.css"`,
		`href='/__arca/shelley/main.css'`,
		`src='/__arca/shelley/main.js'`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
}

func TestProxyReadyz(t *testing.T) {
	cp := &proxyStubControlPlane{exposure: Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: true}}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, "http://127.0.0.1:11030"), "")

	sentinel := filepath.Join(t.TempDir(), "startup.done")
	proxy.SetReadinessChecker(NewReadinessChecker(sentinel, nil))

	req := httptest.NewRequest(http.MethodGet, "http://app.example/__arca/readyz", nil)
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}

	if err := os.WriteFile(sentinel, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	rr = httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestProxyRedirectsUnauthenticatedArcaPathEvenWhenPublicExposure(t *testing.T) {
	cp := &proxyStubControlPlane{exposure: Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: true}}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, "http://127.0.0.1:11030"), "")

	req := httptest.NewRequest(http.MethodGet, "http://app.example/__arca/ttyd/", nil)
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://control.example/auth/authorize?target=") {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
}

func TestProxySetsUserHeadersForAuthenticatedUser(t *testing.T) {
	var receivedHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	cp := &proxyStubControlPlane{
		exposure:       Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: false},
		exchangeClaims: ArcadSessionClaims{SessionID: "as_valid", UserID: "u1", UserEmail: "user@example.com", ExpiresAt: time.Now().Add(time.Hour)},
	}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, upstream.URL), "")

	req := httptest.NewRequest(http.MethodGet, "http://app.example/", nil)
	addValidSessionCookie(req)
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if got := receivedHeaders.Get("X-Arca-User-Id"); got != "u1" {
		t.Fatalf("expected X-Arca-User-Id %q, got %q", "u1", got)
	}
	if got := receivedHeaders.Get("X-Arca-User-Email"); got != "user@example.com" {
		t.Fatalf("expected X-Arca-User-Email %q, got %q", "user@example.com", got)
	}
}

func TestProxyRemovesUserHeadersForAnonymousPublicExposure(t *testing.T) {
	var receivedHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	cp := &proxyStubControlPlane{
		exposure: Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: true},
	}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, upstream.URL), "")

	req := httptest.NewRequest(http.MethodGet, "http://app.example/", nil)
	// Set spoofed headers that should be stripped
	req.Header.Set("X-Arca-User-Id", "spoofed")
	req.Header.Set("X-Arca-User-Email", "spoofed@evil.com")
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if got := receivedHeaders.Get("X-Arca-User-Id"); got != "" {
		t.Fatalf("expected empty X-Arca-User-Id, got %q", got)
	}
	if got := receivedHeaders.Get("X-Arca-User-Email"); got != "" {
		t.Fatalf("expected empty X-Arca-User-Email, got %q", got)
	}
}

func TestProxySetsUserHeadersForAuthenticatedPublicExposure(t *testing.T) {
	var receivedHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	cp := &proxyStubControlPlane{
		exposure:       Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: true},
		exchangeClaims: ArcadSessionClaims{SessionID: "as_valid", UserID: "u1", UserEmail: "user@example.com", ExpiresAt: time.Now().Add(time.Hour)},
	}
	proxy := NewProxy(NewExposureCache(cp), cp, "arcad_session", mustURL(t, upstream.URL), "")

	req := httptest.NewRequest(http.MethodGet, "http://app.example/", nil)
	addValidSessionCookie(req)
	rr := httptest.NewRecorder()
	proxy.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if got := receivedHeaders.Get("X-Arca-User-Id"); got != "u1" {
		t.Fatalf("expected X-Arca-User-Id %q, got %q", "u1", got)
	}
	if got := receivedHeaders.Get("X-Arca-User-Email"); got != "user@example.com" {
		t.Fatalf("expected X-Arca-User-Email %q, got %q", "user@example.com", got)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}

func addValidSessionCookie(req *http.Request) {
	req.AddCookie(&http.Cookie{Name: "arcad_session", Value: "as_valid"})
}

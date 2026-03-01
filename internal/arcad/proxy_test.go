package arcad

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type proxyStubControlPlane struct {
	exposure  Exposure
	claims    TicketClaims
	verifyErr error
}

func (s *proxyStubControlPlane) GetExposureByHost(_ context.Context, _ string) (Exposure, error) {
	return s.exposure, nil
}

func (s *proxyStubControlPlane) VerifyTicket(_ context.Context, _, _ string) (TicketClaims, error) {
	if s.verifyErr != nil {
		return TicketClaims{}, s.verifyErr
	}
	return s.claims, nil
}

func (s *proxyStubControlPlane) AuthorizeURL(target string) string {
	return "https://control.example/auth/authorize?target=" + target
}

func TestProxyRedirectsUnauthenticatedPrivateExposure(t *testing.T) {
	cp := &proxyStubControlPlane{exposure: Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: false}}
	proxy := NewProxy(NewExposureCache(cp), cp, NewSessionManager("secret"), "arcad_session", mustURL(t, "http://127.0.0.1:8080"))

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
		claims:   TicketClaims{UserID: "u1", ExpiresAt: time.Now().Add(time.Hour)},
	}
	proxy := NewProxy(NewExposureCache(cp), cp, NewSessionManager("secret"), "arcad_session", mustURL(t, "http://127.0.0.1:8080"))

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
	if cookies[0].Name != "arcad_session" || cookies[0].Value == "" {
		t.Fatalf("unexpected cookie: %+v", cookies[0])
	}
}

func TestProxyRoutesClaudeCodeUIPathToDedicatedUpstream(t *testing.T) {
	defaultUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("default"))
	}))
	t.Cleanup(defaultUpstream.Close)

	claudecodeuiUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("claudecodeui"))
	}))
	t.Cleanup(claudecodeuiUpstream.Close)

	cp := &proxyStubControlPlane{exposure: Exposure{Host: "app.example", Target: "127.0.0.1:3000", Public: true}}
	proxy := NewProxy(NewExposureCache(cp), cp, NewSessionManager("secret"), "arcad_session", mustURL(t, defaultUpstream.URL))
	proxy.claudecodeui = mustURL(t, claudecodeuiUpstream.URL)

	t.Run("claudecodeui path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://app.example/__arca/claudecodeui/", nil)
		rr := httptest.NewRecorder()
		proxy.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		body, err := io.ReadAll(rr.Result().Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "claudecodeui" {
			t.Fatalf("unexpected body: %q", string(body))
		}
	})

	t.Run("default path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://app.example/", nil)
		rr := httptest.NewRecorder()
		proxy.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		body, err := io.ReadAll(rr.Result().Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "default" {
			t.Fatalf("unexpected body: %q", string(body))
		}
	})
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}

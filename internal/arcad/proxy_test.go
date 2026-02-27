package arcad

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	proxy := NewProxy(NewExposureCache(cp), cp, NewSessionManager("secret"), "arcad_session")

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
	proxy := NewProxy(NewExposureCache(cp), cp, NewSessionManager("secret"), "arcad_session")

	req := httptest.NewRequest(http.MethodGet, "http://app.example/callback?ticket=tk_1&next=%2Fworkspace", nil)
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

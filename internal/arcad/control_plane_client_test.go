package arcad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExchangeArcadSessionAcceptsCamelCasePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != exchangeArcadSessionEndpoint {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessionId":"as_1","expiresAtUnix":"1710000000","user":{"id":"u1"}}`))
	}))
	t.Cleanup(server.Close)

	client := NewHTTPControlPlaneClient(server.URL, "", "m1", "mt_1", server.Client())
	claims, err := client.ExchangeArcadSession(context.Background(), "arca-test3.ryotarai.info", "atk_1")
	if err != nil {
		t.Fatalf("ExchangeArcadSession failed: %v", err)
	}
	if claims.SessionID != "as_1" {
		t.Fatalf("unexpected session id: %q", claims.SessionID)
	}
	if claims.UserID != "u1" {
		t.Fatalf("unexpected user id: %q", claims.UserID)
	}
	if claims.ExpiresAt.Unix() != 1710000000 {
		t.Fatalf("unexpected expiry: %d", claims.ExpiresAt.Unix())
	}
}

func TestExchangeArcadSessionAcceptsSnakeCasePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != exchangeArcadSessionEndpoint {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"as_2","expires_at_unix":1710000100,"user":{"id":"u2"}}`))
	}))
	t.Cleanup(server.Close)

	client := NewHTTPControlPlaneClient(server.URL, "", "m1", "mt_1", server.Client())
	claims, err := client.ExchangeArcadSession(context.Background(), "arca-test3.ryotarai.info", "atk_2")
	if err != nil {
		t.Fatalf("ExchangeArcadSession failed: %v", err)
	}
	if claims.SessionID != "as_2" {
		t.Fatalf("unexpected session id: %q", claims.SessionID)
	}
	if claims.UserID != "u2" {
		t.Fatalf("unexpected user id: %q", claims.UserID)
	}
	if claims.ExpiresAt.Unix() != 1710000100 {
		t.Fatalf("unexpected expiry: %d", claims.ExpiresAt.Unix())
	}
}

func TestReportMachineReadinessAcceptsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != reportMachineReadinessEndpoint {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	t.Cleanup(server.Close)

	client := NewHTTPControlPlaneClient(server.URL, "", "m1", "mt_1", server.Client())
	accepted, err := client.ReportMachineReadiness(context.Background(), true, "ready", "container-1", "v0.1.0")
	if err != nil {
		t.Fatalf("ReportMachineReadiness failed: %v", err)
	}
	if !accepted {
		t.Fatalf("accepted = false, want true")
	}
}

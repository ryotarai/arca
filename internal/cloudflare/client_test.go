package cloudflare

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyToken(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/client/v4/user/tokens/verify"; got != want {
			t.Fatalf("path = %s, want %s", got, want)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"id":"tok","status":"active"}}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL+"/client/v4")
	result, err := client.VerifyToken(context.Background(), "token")
	if err != nil {
		t.Fatalf("VerifyToken() error = %v", err)
	}
	if result.ID != "tok" || result.Status != "active" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUpsertDNSCNAMEUpdatesExistingRecord(t *testing.T) {
	t.Parallel()

	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step++
		switch step {
		case 1:
			if got, want := r.Method, http.MethodGet; got != want {
				t.Fatalf("step1 method = %s, want %s", got, want)
			}
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"record-1"}]}`))
		case 2:
			if got, want := r.Method, http.MethodPatch; got != want {
				t.Fatalf("step2 method = %s, want %s", got, want)
			}
			if !strings.HasSuffix(r.URL.Path, "/zones/zone-1/dns_records/record-1") {
				t.Fatalf("unexpected update path: %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			t.Fatalf("unexpected extra request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	if err := client.UpsertDNSCNAME(context.Background(), "token", "zone-1", "app.example.com", "abc.cfargotunnel.com", true); err != nil {
		t.Fatalf("UpsertDNSCNAME() error = %v", err)
	}
}

func TestUpdateTunnelIngressReturnsAPIError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1003,"message":"bad ingress"}]}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	err := client.UpdateTunnelIngress(context.Background(), "token", "acc", "tun", []IngressRule{{Hostname: "app.example.com", Service: "http://localhost:80"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "1003") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyAccountToken(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/accounts/acc-1/cfd_tunnel"; got != want {
			t.Fatalf("path = %s, want %s", got, want)
		}
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("query page = %s, want 1", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "1" {
			t.Fatalf("query per_page = %s, want 1", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	if err := client.VerifyAccountToken(context.Background(), "token", "acc-1"); err != nil {
		t.Fatalf("VerifyAccountToken() error = %v", err)
	}
}

func TestVerifyZoneAccess(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/zones/zone-1"; got != want {
			t.Fatalf("path = %s, want %s", got, want)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"id":"zone-1"}}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	if err := client.VerifyZoneAccess(context.Background(), "token", "zone-1"); err != nil {
		t.Fatalf("VerifyZoneAccess() error = %v", err)
	}
}

func TestGetZone(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/zones/zone-1"; got != want {
			t.Fatalf("path = %s, want %s", got, want)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"id":"zone-1","name":"example.com","account":{"id":"acc-1"}}}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	zone, err := client.GetZone(context.Background(), "token", "zone-1")
	if err != nil {
		t.Fatalf("GetZone() error = %v", err)
	}
	if got, want := zone.ID, "zone-1"; got != want {
		t.Fatalf("zone.id = %s, want %s", got, want)
	}
	if got, want := zone.Account.ID, "acc-1"; got != want {
		t.Fatalf("zone.account.id = %s, want %s", got, want)
	}
}

func TestGetTunnelByName(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/accounts/acc-1/cfd_tunnel"; got != want {
			t.Fatalf("path = %s, want %s", got, want)
		}
		if got, want := r.URL.Query().Get("name"), "arca-machine-123"; got != want {
			t.Fatalf("name query = %s, want %s", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"tun-1","name":"arca-machine-123"}]}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	tunnel, err := client.GetTunnelByName(context.Background(), "token", "acc-1", "arca-machine-123")
	if err != nil {
		t.Fatalf("GetTunnelByName() error = %v", err)
	}
	if got, want := tunnel.ID, "tun-1"; got != want {
		t.Fatalf("tunnel.id = %s, want %s", got, want)
	}
}

func TestGetTunnelByNameNotFound(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	_, err := client.GetTunnelByName(context.Background(), "token", "acc-1", "arca-machine-123")
	if !errors.Is(err, ErrTunnelNotFound) {
		t.Fatalf("expected ErrTunnelNotFound, got %v", err)
	}
}

func TestCreateTunnelReturnsAPIErrorOnNon2xx(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1013,"message":"already exists"}]}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	_, err := client.CreateTunnel(context.Background(), "token", "acc-1", "name")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if got, want := apiErr.Code, 1013; got != want {
		t.Fatalf("api error code = %d, want %d", got, want)
	}
}

func TestCreateTunnelTokenUsesGET(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodGet; got != want {
			t.Fatalf("method = %s, want %s", got, want)
		}
		if got, want := r.URL.Path, "/accounts/acc-1/cfd_tunnel/tun-1/token"; got != want {
			t.Fatalf("path = %s, want %s", got, want)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":"tok-1"}`))
	}))
	defer ts.Close()

	client := NewClientWithBaseURL(ts.Client(), ts.URL)
	token, err := client.CreateTunnelToken(context.Background(), "token", "acc-1", "tun-1")
	if err != nil {
		t.Fatalf("CreateTunnelToken() error = %v", err)
	}
	if got, want := token, "tok-1"; got != want {
		t.Fatalf("token = %s, want %s", got, want)
	}
}

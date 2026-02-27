package cloudflare

import (
	"context"
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
			if got, want := r.Method, http.MethodPut; got != want {
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
		w.WriteHeader(http.StatusOK)
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

func TestVerifyZoneToken(t *testing.T) {
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
	if err := client.VerifyZoneToken(context.Background(), "token", "zone-1"); err != nil {
		t.Fatalf("VerifyZoneToken() error = %v", err)
	}
}

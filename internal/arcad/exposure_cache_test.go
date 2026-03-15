package arcad

import (
	"context"
	"testing"
	"time"
)

type stubControlPlaneClient struct {
	exposureCalls int
	exposure      Exposure
	err           error
}

func (s *stubControlPlaneClient) GetExposureByHost(_ context.Context, _ string) (Exposure, error) {
	s.exposureCalls++
	return s.exposure, s.err
}

func (s *stubControlPlaneClient) ExchangeArcadSession(_ context.Context, _, _ string) (ArcadSessionClaims, error) {
	return ArcadSessionClaims{}, nil
}

func (s *stubControlPlaneClient) ValidateArcadSession(_ context.Context, _, _, _ string) (ArcadSessionClaims, error) {
	return ArcadSessionClaims{}, nil
}

func (s *stubControlPlaneClient) ReportMachineReadiness(_ context.Context, _ bool, _, _, _ string) (bool, error) {
	return true, nil
}

func (s *stubControlPlaneClient) AuthorizeURL(_ string) string {
	return ""
}

func TestExposureCacheUsesTTL(t *testing.T) {
	client := &stubControlPlaneClient{exposure: Exposure{Host: "app.example", Target: "127.0.0.1:3000"}}
	cache := NewExposureCache(client)

	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }

	_, err := cache.GetByHost(context.Background(), "app.example")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	_, err = cache.GetByHost(context.Background(), "app.example")
	if err != nil {
		t.Fatalf("cache fetch failed: %v", err)
	}
	if client.exposureCalls != 1 {
		t.Fatalf("expected 1 call before TTL, got %d", client.exposureCalls)
	}

	now = now.Add(exposureCacheTTL + time.Second)
	_, err = cache.GetByHost(context.Background(), "app.example")
	if err != nil {
		t.Fatalf("fetch after ttl failed: %v", err)
	}
	if client.exposureCalls != 2 {
		t.Fatalf("expected 2 calls after TTL, got %d", client.exposureCalls)
	}
}

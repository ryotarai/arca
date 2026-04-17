package arcad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
	"github.com/ryotarai/arca/internal/gen/arca/v1/arcav1connect"
)

type fakeTicketService struct {
	arcav1connect.UnimplementedTicketServiceHandler
	exchangeResp *arcav1.ExchangeArcadSessionResponse
	lastHeader   http.Header
	lastReq      *arcav1.ExchangeArcadSessionRequest
}

func (s *fakeTicketService) ExchangeArcadSession(ctx context.Context, req *connect.Request[arcav1.ExchangeArcadSessionRequest]) (*connect.Response[arcav1.ExchangeArcadSessionResponse], error) {
	s.lastHeader = req.Header().Clone()
	s.lastReq = req.Msg
	return connect.NewResponse(s.exchangeResp), nil
}

type fakeExposureService struct {
	arcav1connect.UnimplementedExposureServiceHandler
	readinessResp *arcav1.ReportMachineReadinessResponse
	lastHeader    http.Header
	lastReadiness *arcav1.ReportMachineReadinessRequest
}

func (s *fakeExposureService) ReportMachineReadiness(ctx context.Context, req *connect.Request[arcav1.ReportMachineReadinessRequest]) (*connect.Response[arcav1.ReportMachineReadinessResponse], error) {
	s.lastHeader = req.Header().Clone()
	s.lastReadiness = req.Msg
	return connect.NewResponse(s.readinessResp), nil
}

func newTestServer(t *testing.T, ticket arcav1connect.TicketServiceHandler, exposure arcav1connect.ExposureServiceHandler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	if ticket != nil {
		path, handler := arcav1connect.NewTicketServiceHandler(ticket)
		mux.Handle(path, handler)
	}
	if exposure != nil {
		path, handler := arcav1connect.NewExposureServiceHandler(exposure)
		mux.Handle(path, handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestExchangeArcadSession(t *testing.T) {
	fake := &fakeTicketService{
		exchangeResp: &arcav1.ExchangeArcadSessionResponse{
			SessionId:     "as_1",
			ExpiresAtUnix: 1710000000,
			User:          &arcav1.TicketUser{Id: "u1", Email: "u1@example.com"},
		},
	}
	srv := newTestServer(t, fake, nil)

	client := NewHTTPControlPlaneClient(srv.URL, "", "m1", "mt_1", srv.Client())
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
	if claims.UserEmail != "u1@example.com" {
		t.Fatalf("unexpected user email: %q", claims.UserEmail)
	}
	if claims.ExpiresAt.Unix() != 1710000000 {
		t.Fatalf("unexpected expiry: %d", claims.ExpiresAt.Unix())
	}

	if got := fake.lastReq.GetToken(); got != "atk_1" {
		t.Fatalf("unexpected token sent: %q", got)
	}
	if got := fake.lastReq.GetHostname(); got != "arca-test3.ryotarai.info" {
		t.Fatalf("unexpected hostname sent: %q", got)
	}
	if got := fake.lastHeader.Get("X-Arca-Machine-ID"); got != "m1" {
		t.Fatalf("missing or wrong X-Arca-Machine-ID header: %q", got)
	}
	if got := fake.lastHeader.Get("Authorization"); got != "Bearer mt_1" {
		t.Fatalf("missing or wrong Authorization header: %q", got)
	}
}

func TestReportMachineReadinessAcceptsResponse(t *testing.T) {
	fake := &fakeExposureService{
		readinessResp: &arcav1.ReportMachineReadinessResponse{Accepted: true},
	}
	srv := newTestServer(t, nil, fake)

	client := NewHTTPControlPlaneClient(srv.URL, "", "m1", "mt_1", srv.Client())
	accepted, err := client.ReportMachineReadiness(context.Background(), true, "ready", "container-1", "v0.1.0")
	if err != nil {
		t.Fatalf("ReportMachineReadiness failed: %v", err)
	}
	if !accepted {
		t.Fatalf("accepted = false, want true")
	}

	got := fake.lastReadiness
	if !got.GetReady() || got.GetReason() != "ready" || got.GetMachineId() != "m1" || got.GetContainerId() != "container-1" || got.GetArcadVersion() != "v0.1.0" {
		t.Fatalf("unexpected readiness payload: %+v", got)
	}
	if h := fake.lastHeader.Get("Authorization"); h != "Bearer mt_1" {
		t.Fatalf("missing Authorization header: %q", h)
	}
}

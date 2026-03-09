package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func TestVisibilityFromRequest(t *testing.T) {
	tests := []struct {
		name string
		req  *arcav1.UpsertMachineExposureRequest
		want string
	}{
		{name: "owner only", req: &arcav1.UpsertMachineExposureRequest{Visibility: arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_OWNER_ONLY}, want: db.EndpointVisibilityOwnerOnly},
		{name: "selected users", req: &arcav1.UpsertMachineExposureRequest{Visibility: arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_SELECTED_USERS}, want: db.EndpointVisibilitySelectedUsers},
		{name: "all users", req: &arcav1.UpsertMachineExposureRequest{Visibility: arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_ALL_ARCA_USERS}, want: db.EndpointVisibilityAllArcaUsers},
		{name: "internet public", req: &arcav1.UpsertMachineExposureRequest{Visibility: arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_INTERNET_PUBLIC}, want: db.EndpointVisibilityInternetPublic},
		{name: "fallback from legacy public", req: &arcav1.UpsertMachineExposureRequest{Public: true}, want: db.EndpointVisibilityInternetPublic},
		{name: "fallback default", req: &arcav1.UpsertMachineExposureRequest{}, want: db.EndpointVisibilityOwnerOnly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := visibilityFromRequest(tt.req); got != tt.want {
				t.Fatalf("visibilityFromRequest() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeSelectedUserIDs(t *testing.T) {
	got := normalizeSelectedUserIDs([]string{" user-b ", "user-a", "", "user-b"}, "owner-1")
	want := []string{"user-b", "user-a", "owner-1"}
	if len(got) != len(want) {
		t.Fatalf("len(normalizeSelectedUserIDs()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeSelectedUserIDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestToMachineExposureMessage(t *testing.T) {
	msg := toMachineExposureMessage(db.MachineExposure{
		ID:              "exp-1",
		MachineID:       "mach-1",
		Name:            "default",
		Hostname:        "app.example.com",
		Service:         "http://localhost:8080",
		Visibility:      db.EndpointVisibilitySelectedUsers,
		SelectedUserIDs: []string{"user-1", "user-2"},
	})

	if msg.GetVisibility() != arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_SELECTED_USERS {
		t.Fatalf("visibility = %v, want %v", msg.GetVisibility(), arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_SELECTED_USERS)
	}
	if msg.GetPublic() {
		t.Fatalf("public = true, want false")
	}
	if len(msg.GetSelectedUserIds()) != 2 {
		t.Fatalf("selected user ids len = %d, want 2", len(msg.GetSelectedUserIds()))
	}
}

func TestReportMachineReadiness_AcceptsWhenDesiredRunning(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newTunnelConnectService(store, authenticator)

	userID, _, err := authenticator.Register(ctx, "ready-owner@example.com", "owner-password")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	machine, err := store.CreateMachineWithOwner(ctx, userID, "ready-machine", "libvirt", "v1")
	if err != nil {
		t.Fatalf("create machine: %v", err)
	}

	req := connect.NewRequest(&arcav1.ReportMachineReadinessRequest{
		Ready:       true,
		Reason:      "startup sentinel and tcp endpoints are ready",
		MachineId:   machine.ID,
		ContainerId: "container-1",
	})
	req.Header().Set("Authorization", "Bearer "+machine.MachineToken)

	resp, err := service.ReportMachineReadiness(ctx, req)
	if err != nil {
		t.Fatalf("ReportMachineReadiness failed: %v", err)
	}
	if !resp.Msg.GetAccepted() {
		t.Fatalf("accepted = false, want true")
	}

	updated, err := store.GetMachineByID(ctx, machine.ID)
	if err != nil {
		t.Fatalf("get machine: %v", err)
	}
	if !updated.Ready {
		t.Fatalf("ready = false, want true")
	}
	if updated.Status != db.MachineStatusRunning {
		t.Fatalf("status = %q, want %q", updated.Status, db.MachineStatusRunning)
	}
}

func TestReportMachineReadiness_RejectsReadyWhenDesiredStopped(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newTunnelConnectService(store, authenticator)

	userID, _, err := authenticator.Register(ctx, "ready-stop-owner@example.com", "owner-password")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	machine, err := store.CreateMachineWithOwner(ctx, userID, "ready-stop-machine", "libvirt", "v1")
	if err != nil {
		t.Fatalf("create machine: %v", err)
	}
	updated, err := store.RequestStopMachineByIDForOwner(ctx, userID, machine.ID)
	if err != nil {
		t.Fatalf("request stop machine: %v", err)
	}
	if !updated {
		t.Fatalf("request stop machine updated = false, want true")
	}

	req := connect.NewRequest(&arcav1.ReportMachineReadinessRequest{
		Ready:     true,
		Reason:    "should be rejected",
		MachineId: machine.ID,
	})
	req.Header().Set("Authorization", "Bearer "+machine.MachineToken)
	resp, err := service.ReportMachineReadiness(ctx, req)
	if err != nil {
		t.Fatalf("ReportMachineReadiness failed: %v", err)
	}
	if resp.Msg.GetAccepted() {
		t.Fatalf("accepted = true, want false")
	}
}

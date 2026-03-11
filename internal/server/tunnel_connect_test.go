package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func TestToMachineExposureMessage(t *testing.T) {
	msg := toMachineExposureMessage(db.MachineExposure{
		ID:        "exp-1",
		MachineID: "mach-1",
		Name:      "default",
		Hostname:  "app.example.com",
		Service:   "http://localhost:8080",
	})

	if msg.GetName() != "default" {
		t.Fatalf("name = %q, want %q", msg.GetName(), "default")
	}
	if msg.GetHostname() != "app.example.com" {
		t.Fatalf("hostname = %q, want %q", msg.GetHostname(), "app.example.com")
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

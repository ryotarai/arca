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
	service := newTunnelConnectService(store, authenticator, nil)

	userID, _, err := authenticator.Register(ctx, "ready-owner@example.com", "owner-password")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	machine, err := store.CreateMachineWithOwner(ctx, userID, "ready-machine", "libvirt", "v1")
	if err != nil {
		t.Fatalf("create machine: %v", err)
	}

	req := connect.NewRequest(&arcav1.ReportMachineReadinessRequest{
		Ready:        true,
		Reason:       "startup sentinel and tcp endpoints are ready",
		MachineId:    machine.ID,
		ContainerId:  "container-1",
		ArcadVersion: "v0.2.0",
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
	if updated.ArcadVersion != "v0.2.0" {
		t.Fatalf("arcad_version = %q, want %q", updated.ArcadVersion, "v0.2.0")
	}
}

func TestReportMachineReadiness_StoresArcadVersion(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newTunnelConnectService(store, authenticator, nil)

	userID, _, err := authenticator.Register(ctx, "version-owner@example.com", "owner-password")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	machine, err := store.CreateMachineWithOwner(ctx, userID, "version-machine", "libvirt", "v1")
	if err != nil {
		t.Fatalf("create machine: %v", err)
	}

	// Report with version.
	req := connect.NewRequest(&arcav1.ReportMachineReadinessRequest{
		Ready:        true,
		Reason:       "ready",
		MachineId:    machine.ID,
		ArcadVersion: "v0.3.1",
	})
	req.Header().Set("Authorization", "Bearer "+machine.MachineToken)
	_, err = service.ReportMachineReadiness(ctx, req)
	if err != nil {
		t.Fatalf("ReportMachineReadiness failed: %v", err)
	}

	updated, err := store.GetMachineByID(ctx, machine.ID)
	if err != nil {
		t.Fatalf("get machine: %v", err)
	}
	if updated.ArcadVersion != "v0.3.1" {
		t.Fatalf("arcad_version = %q, want %q", updated.ArcadVersion, "v0.3.1")
	}

	// Report without version (backward compat: should preserve existing).
	req2 := connect.NewRequest(&arcav1.ReportMachineReadinessRequest{
		Ready:     true,
		Reason:    "still ready",
		MachineId: machine.ID,
	})
	req2.Header().Set("Authorization", "Bearer "+machine.MachineToken)
	_, err = service.ReportMachineReadiness(ctx, req2)
	if err != nil {
		t.Fatalf("ReportMachineReadiness (no version) failed: %v", err)
	}

	updated2, err := store.GetMachineByID(ctx, machine.ID)
	if err != nil {
		t.Fatalf("get machine: %v", err)
	}
	if updated2.ArcadVersion != "v0.3.1" {
		t.Fatalf("arcad_version after no-version report = %q, want %q (should be preserved)", updated2.ArcadVersion, "v0.3.1")
	}
}

func TestReportMachineReadiness_RejectsReadyWhenDesiredStopped(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newTunnelConnectService(store, authenticator, nil)

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

package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

const (
	libvirtCatalogConfigJSON = `{"libvirt":{"uri":"qemu:///system","network":"default","storagePool":"default"}}`
	gceCatalogConfigJSON     = `{"gce":{"project":"arca-project","zone":"us-central1-a","network":"main","subnetwork":"main","serviceAccountEmail":"svc@example.iam.gserviceaccount.com"}}`
)

func TestMachineRuntimeSelection_CreateRequiresExplicitRuntime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineConnectService(authenticator, store, store)

	if _, _, err := authenticator.Register(ctx, "runtime-owner@example.com", "owner-password"); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	ownerToken := loginToken(t, authenticator, "runtime-owner@example.com", "owner-password")

	overrideRuntime, err := store.CreateRuntime(ctx, "edge-gce", db.RuntimeTypeGCE, gceCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create override runtime: %v", err)
	}

	_, err = service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{Name: "machine-missing-runtime"}, ownerToken))
	if err == nil {
		t.Fatalf("expected create to fail when runtime id is omitted")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("create missing-runtime error code = %v, want %v", got, connect.CodeInvalidArgument)
	}

	createdWithExplicit, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:      "machine-explicit-runtime",
		RuntimeId: overrideRuntime.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("create machine with explicit runtime: %v", err)
	}
	if got := createdWithExplicit.Msg.GetMachine().GetRuntimeId(); got != overrideRuntime.ID {
		t.Fatalf("explicit runtime id = %q, want %q", got, overrideRuntime.ID)
	}
}

func TestMachineRuntimeSelection_InvalidOrDeletedRuntimeIsRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineConnectService(authenticator, store, store)

	if _, _, err := authenticator.Register(ctx, "runtime-owner-deleted@example.com", "owner-password"); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	ownerToken := loginToken(t, authenticator, "runtime-owner-deleted@example.com", "owner-password")

	runtimeEntry, err := store.CreateRuntime(ctx, "runtime-to-delete", db.RuntimeTypeLibvirt, libvirtCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	machineResp, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:      "machine-runtime-deletion",
		RuntimeId: runtimeEntry.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("create machine before runtime deletion: %v", err)
	}

	if deleted, err := store.DeleteRuntimeByID(ctx, runtimeEntry.ID); err == nil {
		t.Fatalf("expected delete runtime to fail while in use, got deleted=%v", deleted)
	} else if err != db.ErrRuntimeInUse {
		t.Fatalf("delete runtime error = %v, want %v", err, db.ErrRuntimeInUse)
	}

	startResp, err := service.StartMachine(ctx, authRequest(arcav1.StartMachineRequest{MachineId: machineResp.Msg.GetMachine().GetId()}, ownerToken))
	if err != nil {
		t.Fatalf("start machine with still-configured runtime: %v", err)
	}
	if got := startResp.Msg.GetMachine().GetRuntimeId(); got != runtimeEntry.ID {
		t.Fatalf("runtime id after start = %q, want %q", got, runtimeEntry.ID)
	}

	createdResp, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:      "machine-runtime-still-valid",
		RuntimeId: runtimeEntry.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("expected create to succeed with runtime still configured: %v", err)
	}
	if got := createdResp.Msg.GetMachine().GetRuntimeId(); got != runtimeEntry.ID {
		t.Fatalf("create runtime id = %q, want %q", got, runtimeEntry.ID)
	}

	_, err = service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:      "machine-runtime-unknown-id",
		RuntimeId: "runtime-does-not-exist",
	}, ownerToken))
	if err == nil {
		t.Fatalf("expected create to fail with unknown runtime id")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("create unknown-runtime error code = %v, want %v", got, connect.CodeInvalidArgument)
	}
}

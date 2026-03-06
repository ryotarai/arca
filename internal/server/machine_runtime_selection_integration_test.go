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

func TestMachineRuntimeSelection_DefaultAssignmentAndExplicitSelection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineConnectService(authenticator, store, nil)

	ownerID, _, err := authenticator.Register(ctx, "runtime-owner@example.com", "owner-password")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	ownerToken := loginToken(t, authenticator, "runtime-owner@example.com", "owner-password")

	defaultRuntime, err := store.CreateRuntime(ctx, "default-libvirt", db.RuntimeTypeLibvirt, libvirtCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create default runtime: %v", err)
	}
	overrideRuntime, err := store.CreateRuntime(ctx, "edge-gce", db.RuntimeTypeGCE, gceCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create override runtime: %v", err)
	}

	if err := store.UpsertSetupState(ctx, db.SetupState{Completed: true, AdminUserID: ownerID, MachineRuntime: defaultRuntime.ID}); err != nil {
		t.Fatalf("upsert setup state: %v", err)
	}

	createdWithDefault, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{Name: "machine-default-runtime"}, ownerToken))
	if err != nil {
		t.Fatalf("create machine with default runtime: %v", err)
	}
	if got := createdWithDefault.Msg.GetMachine().GetRuntimeId(); got != defaultRuntime.ID {
		t.Fatalf("default runtime id = %q, want %q", got, defaultRuntime.ID)
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

	startResp, err := service.StartMachine(ctx, authRequest(arcav1.StartMachineRequest{
		MachineId: createdWithExplicit.Msg.GetMachine().GetId(),
		RuntimeId: defaultRuntime.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("start machine with explicit runtime override: %v", err)
	}
	if got := startResp.Msg.GetMachine().GetRuntimeId(); got != defaultRuntime.ID {
		t.Fatalf("runtime id after start override = %q, want %q", got, defaultRuntime.ID)
	}
}

func TestMachineRuntimeSelection_InvalidOrDeletedRuntimeIsRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineConnectService(authenticator, store, nil)

	ownerID, _, err := authenticator.Register(ctx, "runtime-owner-deleted@example.com", "owner-password")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	ownerToken := loginToken(t, authenticator, "runtime-owner-deleted@example.com", "owner-password")

	runtimeEntry, err := store.CreateRuntime(ctx, "runtime-to-delete", db.RuntimeTypeLibvirt, libvirtCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}

	if err := store.UpsertSetupState(ctx, db.SetupState{Completed: true, AdminUserID: ownerID, MachineRuntime: runtimeEntry.ID}); err != nil {
		t.Fatalf("upsert setup state: %v", err)
	}

	machineResp, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:      "machine-runtime-deletion",
		RuntimeId: runtimeEntry.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("create machine before runtime deletion: %v", err)
	}

	if deleted, err := store.DeleteRuntimeByID(ctx, runtimeEntry.ID); err != nil {
		t.Fatalf("delete runtime: %v", err)
	} else if !deleted {
		t.Fatalf("delete runtime returned false")
	}

	_, err = service.StartMachine(ctx, authRequest(arcav1.StartMachineRequest{MachineId: machineResp.Msg.GetMachine().GetId()}, ownerToken))
	if err == nil {
		t.Fatalf("expected start to fail for deleted runtime")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("start error code = %v, want %v", got, connect.CodeInvalidArgument)
	}

	_, err = service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:      "machine-runtime-deleted-id",
		RuntimeId: runtimeEntry.ID,
	}, ownerToken))
	if err == nil {
		t.Fatalf("expected create to fail with deleted runtime")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("create error code = %v, want %v", got, connect.CodeInvalidArgument)
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

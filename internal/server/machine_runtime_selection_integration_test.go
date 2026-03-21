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

func TestMachineProfileSelection_CreateRequiresExplicitProfile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineConnectService(authenticator, store, store)

	if _, _, err := authenticator.Register(ctx, "template-owner@example.com", "owner-password"); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	ownerToken := loginToken(t, authenticator, "template-owner@example.com", "owner-password")

	overrideProfile, err := store.CreateMachineProfile(ctx, "edge-gce", db.ProviderTypeGCE, gceCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create override profile: %v", err)
	}

	_, err = service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{Name: "machine-missing-template"}, ownerToken))
	if err == nil {
		t.Fatalf("expected create to fail when template id is omitted")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("create missing-template error code = %v, want %v", got, connect.CodeInvalidArgument)
	}

	createdWithExplicit, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:       "machine-explicit-template",
		TemplateId: overrideProfile.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("create machine with explicit profile: %v", err)
	}
	if got := createdWithExplicit.Msg.GetMachine().GetTemplateId(); got != overrideProfile.ID {
		t.Fatalf("explicit profile id = %q, want %q", got, overrideProfile.ID)
	}
}

func TestMachineProfileSelection_InvalidOrDeletedProfileIsRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineConnectService(authenticator, store, store)

	if _, _, err := authenticator.Register(ctx, "template-owner-deleted@example.com", "owner-password"); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	ownerToken := loginToken(t, authenticator, "template-owner-deleted@example.com", "owner-password")

	profileEntry, err := store.CreateMachineProfile(ctx, "template-to-delete", db.ProviderTypeLibvirt, libvirtCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	machineResp, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:       "machine-template-deletion",
		TemplateId: profileEntry.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("create machine before profile deletion: %v", err)
	}

	if deleted, err := store.DeleteMachineProfileByID(ctx, profileEntry.ID); err == nil {
		t.Fatalf("expected delete profile to fail while in use, got deleted=%v", deleted)
	} else if err != db.ErrProfileInUse {
		t.Fatalf("delete profile error = %v, want %v", err, db.ErrProfileInUse)
	}

	startResp, err := service.StartMachine(ctx, authRequest(arcav1.StartMachineRequest{MachineId: machineResp.Msg.GetMachine().GetId()}, ownerToken))
	if err != nil {
		t.Fatalf("start machine with still-configured profile: %v", err)
	}
	if got := startResp.Msg.GetMachine().GetTemplateId(); got != profileEntry.ID {
		t.Fatalf("profile id after start = %q, want %q", got, profileEntry.ID)
	}

	createdResp, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:       "machine-template-still-valid",
		TemplateId: profileEntry.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("expected create to succeed with profile still configured: %v", err)
	}
	if got := createdResp.Msg.GetMachine().GetTemplateId(); got != profileEntry.ID {
		t.Fatalf("create profile id = %q, want %q", got, profileEntry.ID)
	}

	_, err = service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:       "machine-template-unknown-id",
		TemplateId: "template-does-not-exist",
	}, ownerToken))
	if err == nil {
		t.Fatalf("expected create to fail with unknown profile id")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("create unknown-profile error code = %v, want %v", got, connect.CodeInvalidArgument)
	}
}

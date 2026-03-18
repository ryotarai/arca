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

func TestMachineTemplateSelection_CreateRequiresExplicitTemplateAndAllowsStartOverride(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineConnectService(authenticator, store, store)

	if _, _, err := authenticator.Register(ctx, "template-owner@example.com", "owner-password"); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	ownerToken := loginToken(t, authenticator, "template-owner@example.com", "owner-password")

	defaultTemplate, err := store.CreateMachineTemplate(ctx, "default-libvirt", db.TemplateTypeLibvirt, libvirtCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create default template: %v", err)
	}
	overrideTemplate, err := store.CreateMachineTemplate(ctx, "edge-gce", db.TemplateTypeGCE, gceCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create override template: %v", err)
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
		TemplateId: overrideTemplate.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("create machine with explicit template: %v", err)
	}
	if got := createdWithExplicit.Msg.GetMachine().GetTemplateId(); got != overrideTemplate.ID {
		t.Fatalf("explicit template id = %q, want %q", got, overrideTemplate.ID)
	}

	startResp, err := service.StartMachine(ctx, authRequest(arcav1.StartMachineRequest{
		MachineId:  createdWithExplicit.Msg.GetMachine().GetId(),
		TemplateId: defaultTemplate.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("start machine with explicit template override: %v", err)
	}
	if got := startResp.Msg.GetMachine().GetTemplateId(); got != defaultTemplate.ID {
		t.Fatalf("template id after start override = %q, want %q", got, defaultTemplate.ID)
	}
}

func TestMachineTemplateSelection_InvalidOrDeletedTemplateIsRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineConnectService(authenticator, store, store)

	if _, _, err := authenticator.Register(ctx, "template-owner-deleted@example.com", "owner-password"); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	ownerToken := loginToken(t, authenticator, "template-owner-deleted@example.com", "owner-password")

	templateEntry, err := store.CreateMachineTemplate(ctx, "template-to-delete", db.TemplateTypeLibvirt, libvirtCatalogConfigJSON)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	machineResp, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:       "machine-template-deletion",
		TemplateId: templateEntry.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("create machine before template deletion: %v", err)
	}

	if deleted, err := store.DeleteMachineTemplateByID(ctx, templateEntry.ID); err == nil {
		t.Fatalf("expected delete template to fail while in use, got deleted=%v", deleted)
	} else if err != db.ErrTemplateInUse {
		t.Fatalf("delete template error = %v, want %v", err, db.ErrTemplateInUse)
	}

	startResp, err := service.StartMachine(ctx, authRequest(arcav1.StartMachineRequest{MachineId: machineResp.Msg.GetMachine().GetId()}, ownerToken))
	if err != nil {
		t.Fatalf("start machine with still-configured template: %v", err)
	}
	if got := startResp.Msg.GetMachine().GetTemplateId(); got != templateEntry.ID {
		t.Fatalf("template id after start = %q, want %q", got, templateEntry.ID)
	}

	createdResp, err := service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:       "machine-template-still-valid",
		TemplateId: templateEntry.ID,
	}, ownerToken))
	if err != nil {
		t.Fatalf("expected create to succeed with template still configured: %v", err)
	}
	if got := createdResp.Msg.GetMachine().GetTemplateId(); got != templateEntry.ID {
		t.Fatalf("create template id = %q, want %q", got, templateEntry.ID)
	}

	_, err = service.CreateMachine(ctx, authRequest(arcav1.CreateMachineRequest{
		Name:       "machine-template-unknown-id",
		TemplateId: "template-does-not-exist",
	}, ownerToken))
	if err == nil {
		t.Fatalf("expected create to fail with unknown template id")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("create unknown-template error code = %v, want %v", got, connect.CodeInvalidArgument)
	}
}

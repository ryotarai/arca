package machine

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/ryotarai/arca/internal/db"
)

type routingTemplateStoreStub struct {
	entries map[string]db.MachineTemplate
}

func (s *routingTemplateStoreStub) GetMachineTemplateByID(_ context.Context, templateID string) (db.MachineTemplate, error) {
	entry, ok := s.entries[templateID]
	if !ok {
		return db.MachineTemplate{}, sql.ErrNoRows
	}
	return entry, nil
}

type fakeRuntime struct {
	name string
}

func (r *fakeRuntime) EnsureRunning(context.Context, db.Machine, RuntimeStartOptions) (string, error) {
	return r.name, nil
}

func (r *fakeRuntime) EnsureStopped(context.Context, db.Machine) error {
	return nil
}

func (r *fakeRuntime) EnsureDeleted(context.Context, db.Machine) error {
	return nil
}

func (r *fakeRuntime) IsRunning(context.Context, db.Machine) (bool, string, error) {
	return true, r.name, nil
}

func (r *fakeRuntime) GetMachineInfo(context.Context, db.Machine) (*RuntimeMachineInfo, error) {
	return &RuntimeMachineInfo{}, nil
}

func TestRoutingTemplate_ResolvesCatalogTemplateIDsAcrossTypes(t *testing.T) {
	t.Parallel()

	store := &routingTemplateStoreStub{
		entries: map[string]db.MachineTemplate{
			"rt-libvirt": {ID: "rt-libvirt", Type: db.TemplateTypeLibvirt, ConfigJSON: `{"libvirt":{"uri":"qemu:///custom","network":"custom-net","storagePool":"custom-pool","startupScript":"echo libvirt"}}`},
			"rt-gce":     {ID: "rt-gce", Type: db.TemplateTypeGCE, ConfigJSON: `{"gce":{"project":"p","zone":"z","network":"n","subnetwork":"s","serviceAccountEmail":"svc@example.iam.gserviceaccount.com","startupScript":"echo gce"}}`},
		},
	}

	routing := NewRoutingTemplateWithCatalog(store, map[string]Runtime{})
	routing.factory = map[string]TemplateFactory{
		db.TemplateTypeLibvirt: func(catalog db.MachineTemplate) (Runtime, error) {
			return &fakeRuntime{name: "catalog-libvirt:" + catalog.ID}, nil
		},
		db.TemplateTypeGCE: func(catalog db.MachineTemplate) (Runtime, error) {
			return &fakeRuntime{name: "catalog-gce:" + catalog.ID}, nil
		},
	}

	ctx := context.Background()

	libvirtContainer, err := routing.EnsureRunning(ctx, db.Machine{ID: "machine-libvirt", TemplateID: "rt-libvirt"}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running libvirt catalog template: %v", err)
	}
	if libvirtContainer != "catalog-libvirt:rt-libvirt" {
		t.Fatalf("catalog libvirt container id = %q, want %q", libvirtContainer, "catalog-libvirt:rt-libvirt")
	}

	gceContainer, err := routing.EnsureRunning(ctx, db.Machine{ID: "machine-gce", TemplateID: "rt-gce"}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running gce catalog template: %v", err)
	}
	if gceContainer != "catalog-gce:rt-gce" {
		t.Fatalf("catalog gce container id = %q, want %q", gceContainer, "catalog-gce:rt-gce")
	}
}

func TestRoutingTemplate_MissingCatalogTemplateFails(t *testing.T) {
	t.Parallel()

	routing := NewRoutingTemplateWithCatalog(&routingTemplateStoreStub{entries: map[string]db.MachineTemplate{}}, map[string]Runtime{})
	_, _, err := routing.IsRunning(context.Background(), db.Machine{ID: "machine-missing", TemplateID: "rt-missing"})
	if err == nil {
		t.Fatalf("expected missing template lookup to fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want template not found", err.Error())
	}
}

func TestRoutingTemplate_UsesSnapshotConfigWhenAvailable(t *testing.T) {
	t.Parallel()

	store := &routingTemplateStoreStub{
		entries: map[string]db.MachineTemplate{
			"rt-lxd": {ID: "rt-lxd", Type: db.TemplateTypeLXD, ConfigJSON: `{"lxd":{"endpoint":"http://old-host:8443","startupScript":"echo old"}}`},
		},
	}

	factoryCalls := make(map[string]string) // templateID -> configJSON used
	routing := NewRoutingTemplateWithCatalog(store, map[string]Runtime{})
	routing.factory = map[string]TemplateFactory{
		db.TemplateTypeLXD: func(catalog db.MachineTemplate) (Runtime, error) {
			factoryCalls[catalog.ID] = catalog.ConfigJSON
			return &fakeRuntime{name: "lxd:" + catalog.ID}, nil
		},
	}

	ctx := context.Background()

	// Machine with snapshotted config should use it instead of catalog
	snapshotConfig := `{"lxd":{"endpoint":"http://snapshot-host:8443","startupScript":"echo snapshot"}}`
	_, err := routing.EnsureRunning(ctx, db.Machine{
		ID:                "machine-snap",
		TemplateID:         "rt-lxd",
		TemplateType:       db.TemplateTypeLXD,
		TemplateConfigJSON: snapshotConfig,
	}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running with snapshot: %v", err)
	}
	if factoryCalls["rt-lxd"] != snapshotConfig {
		t.Fatalf("expected snapshot config to be used, got %q", factoryCalls["rt-lxd"])
	}

	// Machine without snapshot should fall back to catalog
	delete(factoryCalls, "rt-lxd")
	_, err = routing.EnsureRunning(ctx, db.Machine{
		ID:        "machine-no-snap",
		TemplateID: "rt-lxd",
	}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running without snapshot: %v", err)
	}
	catalogConfig := `{"lxd":{"endpoint":"http://old-host:8443","startupScript":"echo old"}}`
	if factoryCalls["rt-lxd"] != catalogConfig {
		t.Fatalf("expected catalog config to be used, got %q", factoryCalls["rt-lxd"])
	}
}

func TestTemplateFromConfig_StartupScriptIsPropagated(t *testing.T) {
	t.Parallel()

	libvirtRuntime, err := templateFromLibvirtConfig(db.MachineTemplate{
		ID:         "rt-libvirt",
		Type:       db.TemplateTypeLibvirt,
		ConfigJSON: `{"libvirt":{"uri":"qemu:///custom","network":"custom-net","storagePool":"custom-pool","startupScript":"echo libvirt startup"}}`,
	})
	if err != nil {
		t.Fatalf("templateFromLibvirtConfig: %v", err)
	}
	libvirtImpl, ok := libvirtRuntime.(*LibvirtRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *LibvirtRuntime", libvirtRuntime)
	}
	if libvirtImpl.startupScript != "echo libvirt startup" {
		t.Fatalf("libvirt startup script = %q", libvirtImpl.startupScript)
	}

	gceRuntime, err := templateFromGceConfig(db.MachineTemplate{
		ID:         "rt-gce",
		Type:       db.TemplateTypeGCE,
		ConfigJSON: `{"gce":{"project":"p","zone":"z","network":"n","subnetwork":"s","serviceAccountEmail":"svc@example.iam.gserviceaccount.com","startupScript":"echo gce startup"}}`,
	})
	if err != nil {
		t.Fatalf("templateFromGceConfig: %v", err)
	}
	gceImpl, ok := gceRuntime.(*GceRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *GceRuntime", gceRuntime)
	}
	if gceImpl.startupScript != "echo gce startup" {
		t.Fatalf("gce startup script = %q", gceImpl.startupScript)
	}
}

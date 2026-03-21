package machine

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/ryotarai/arca/internal/db"
)

type routingTemplateStoreStub struct {
	entries map[string]db.MachineProfile
}

func (s *routingTemplateStoreStub) GetMachineProfileByID(_ context.Context, profileID string) (db.MachineProfile, error) {
	entry, ok := s.entries[profileID]
	if !ok {
		return db.MachineProfile{}, sql.ErrNoRows
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

func TestRoutingTemplate_ResolvesCatalogProfileIDsAcrossTypes(t *testing.T) {
	t.Parallel()

	store := &routingTemplateStoreStub{
		entries: map[string]db.MachineProfile{
			"rt-libvirt": {ID: "rt-libvirt", Type: db.ProviderTypeLibvirt, ConfigJSON: `{"libvirt":{"uri":"qemu:///custom","network":"custom-net","storagePool":"custom-pool","startupScript":"echo libvirt"}}`},
			"rt-gce":     {ID: "rt-gce", Type: db.ProviderTypeGCE, ConfigJSON: `{"gce":{"project":"p","zone":"z","network":"n","subnetwork":"s","serviceAccountEmail":"svc@example.iam.gserviceaccount.com","startupScript":"echo gce"}}`},
		},
	}

	routing := NewRoutingTemplateWithCatalog(store, map[string]Runtime{})
	routing.factory = map[string]ProfileFactory{
		db.ProviderTypeLibvirt: func(profile db.MachineProfile) (Runtime, error) {
			return &fakeRuntime{name: "catalog-libvirt:" + profile.ID}, nil
		},
		db.ProviderTypeGCE: func(profile db.MachineProfile) (Runtime, error) {
			return &fakeRuntime{name: "catalog-gce:" + profile.ID}, nil
		},
	}

	ctx := context.Background()

	libvirtContainer, err := routing.EnsureRunning(ctx, db.Machine{ID: "machine-libvirt", ProfileID: "rt-libvirt"}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running libvirt catalog profile: %v", err)
	}
	if libvirtContainer != "catalog-libvirt:rt-libvirt" {
		t.Fatalf("catalog libvirt container id = %q, want %q", libvirtContainer, "catalog-libvirt:rt-libvirt")
	}

	gceContainer, err := routing.EnsureRunning(ctx, db.Machine{ID: "machine-gce", ProfileID: "rt-gce"}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running gce catalog profile: %v", err)
	}
	if gceContainer != "catalog-gce:rt-gce" {
		t.Fatalf("catalog gce container id = %q, want %q", gceContainer, "catalog-gce:rt-gce")
	}
}

func TestRoutingTemplate_MissingCatalogProfileFails(t *testing.T) {
	t.Parallel()

	routing := NewRoutingTemplateWithCatalog(&routingTemplateStoreStub{entries: map[string]db.MachineProfile{}}, map[string]Runtime{})
	_, _, err := routing.IsRunning(context.Background(), db.Machine{ID: "machine-missing", ProfileID: "rt-missing"})
	if err == nil {
		t.Fatalf("expected missing profile lookup to fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want profile not found", err.Error())
	}
}

func TestRoutingTemplate_UsesSnapshotConfigWhenAvailable(t *testing.T) {
	t.Parallel()

	store := &routingTemplateStoreStub{
		entries: map[string]db.MachineProfile{
			"rt-lxd": {ID: "rt-lxd", Type: db.ProviderTypeLXD, ConfigJSON: `{"lxd":{"endpoint":"http://old-host:8443","startupScript":"echo old"}}`},
		},
	}

	factoryCalls := make(map[string]string) // profileID -> configJSON used
	routing := NewRoutingTemplateWithCatalog(store, map[string]Runtime{})
	routing.factory = map[string]ProfileFactory{
		db.ProviderTypeLXD: func(profile db.MachineProfile) (Runtime, error) {
			factoryCalls[profile.ID] = profile.ConfigJSON
			return &fakeRuntime{name: "lxd:" + profile.ID}, nil
		},
	}

	ctx := context.Background()

	// Machine with snapshotted config should use it instead of catalog
	snapshotConfig := `{"lxd":{"endpoint":"http://snapshot-host:8443","startupScript":"echo snapshot"}}`
	_, err := routing.EnsureRunning(ctx, db.Machine{
		ID:                       "machine-snap",
		ProfileID:                "rt-lxd",
		ProviderType:             db.ProviderTypeLXD,
		InfrastructureConfigJSON: snapshotConfig,
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
		ProfileID: "rt-lxd",
	}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running without snapshot: %v", err)
	}
	catalogConfig := `{"lxd":{"endpoint":"http://old-host:8443","startupScript":"echo old"}}`
	if factoryCalls["rt-lxd"] != catalogConfig {
		t.Fatalf("expected catalog config to be used, got %q", factoryCalls["rt-lxd"])
	}
}

func TestProfileFromConfig_StartupScriptIsPropagated(t *testing.T) {
	t.Parallel()

	libvirtRuntime, err := profileFromLibvirtConfig(db.MachineProfile{
		ID:         "rt-libvirt",
		Type:       db.ProviderTypeLibvirt,
		ConfigJSON: `{"libvirt":{"uri":"qemu:///custom","network":"custom-net","storagePool":"custom-pool","startupScript":"echo libvirt startup"}}`,
	})
	if err != nil {
		t.Fatalf("profileFromLibvirtConfig: %v", err)
	}
	libvirtImpl, ok := libvirtRuntime.(*LibvirtRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *LibvirtRuntime", libvirtRuntime)
	}
	if libvirtImpl.startupScript != "echo libvirt startup" {
		t.Fatalf("libvirt startup script = %q", libvirtImpl.startupScript)
	}

	gceRuntime, err := profileFromGceConfig(db.MachineProfile{
		ID:         "rt-gce",
		Type:       db.ProviderTypeGCE,
		ConfigJSON: `{"gce":{"project":"p","zone":"z","network":"n","subnetwork":"s","serviceAccountEmail":"svc@example.iam.gserviceaccount.com","startupScript":"echo gce startup"}}`,
	})
	if err != nil {
		t.Fatalf("profileFromGceConfig: %v", err)
	}
	gceImpl, ok := gceRuntime.(*GceRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *GceRuntime", gceRuntime)
	}
	if gceImpl.startupScript != "echo gce startup" {
		t.Fatalf("gce startup script = %q", gceImpl.startupScript)
	}
}

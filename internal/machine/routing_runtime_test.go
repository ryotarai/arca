package machine

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/ryotarai/arca/internal/db"
)

type routingRuntimeStoreStub struct {
	entries map[string]db.RuntimeCatalog
}

func (s *routingRuntimeStoreStub) GetRuntimeByID(_ context.Context, runtimeID string) (db.RuntimeCatalog, error) {
	entry, ok := s.entries[runtimeID]
	if !ok {
		return db.RuntimeCatalog{}, sql.ErrNoRows
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

func TestRoutingRuntime_ResolvesCatalogRuntimeIDsAcrossTypes(t *testing.T) {
	t.Parallel()

	store := &routingRuntimeStoreStub{
		entries: map[string]db.RuntimeCatalog{
			"rt-libvirt": {ID: "rt-libvirt", Type: db.RuntimeTypeLibvirt, ConfigJSON: `{"libvirt":{"uri":"qemu:///custom","network":"custom-net","storagePool":"custom-pool","startupScript":"echo libvirt"}}`},
			"rt-gce":     {ID: "rt-gce", Type: db.RuntimeTypeGCE, ConfigJSON: `{"gce":{"project":"p","zone":"z","network":"n","subnetwork":"s","serviceAccountEmail":"svc@example.iam.gserviceaccount.com","startupScript":"echo gce"}}`},
		},
	}

	runtime := NewRoutingRuntimeWithCatalog(store, map[string]Runtime{})
	runtime.factory = map[string]RuntimeFactory{
		db.RuntimeTypeLibvirt: func(catalog db.RuntimeCatalog) (Runtime, error) {
			return &fakeRuntime{name: "catalog-libvirt:" + catalog.ID}, nil
		},
		db.RuntimeTypeGCE: func(catalog db.RuntimeCatalog) (Runtime, error) {
			return &fakeRuntime{name: "catalog-gce:" + catalog.ID}, nil
		},
	}

	ctx := context.Background()

	libvirtContainer, err := runtime.EnsureRunning(ctx, db.Machine{ID: "machine-libvirt", RuntimeID: "rt-libvirt"}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running libvirt catalog runtime: %v", err)
	}
	if libvirtContainer != "catalog-libvirt:rt-libvirt" {
		t.Fatalf("catalog libvirt container id = %q, want %q", libvirtContainer, "catalog-libvirt:rt-libvirt")
	}

	gceContainer, err := runtime.EnsureRunning(ctx, db.Machine{ID: "machine-gce", RuntimeID: "rt-gce"}, RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("ensure running gce catalog runtime: %v", err)
	}
	if gceContainer != "catalog-gce:rt-gce" {
		t.Fatalf("catalog gce container id = %q, want %q", gceContainer, "catalog-gce:rt-gce")
	}
}

func TestRoutingRuntime_MissingCatalogRuntimeFails(t *testing.T) {
	t.Parallel()

	runtime := NewRoutingRuntimeWithCatalog(&routingRuntimeStoreStub{entries: map[string]db.RuntimeCatalog{}}, map[string]Runtime{})
	_, _, err := runtime.IsRunning(context.Background(), db.Machine{ID: "machine-missing", RuntimeID: "rt-missing"})
	if err == nil {
		t.Fatalf("expected missing runtime lookup to fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want runtime not found", err.Error())
	}
}

func TestRuntimeFromCatalog_StartupScriptIsPropagated(t *testing.T) {
	t.Parallel()

	libvirtRuntime, err := runtimeFromLibvirtCatalog(db.RuntimeCatalog{
		ID:         "rt-libvirt",
		Type:       db.RuntimeTypeLibvirt,
		ConfigJSON: `{"libvirt":{"uri":"qemu:///custom","network":"custom-net","storagePool":"custom-pool","startupScript":"echo libvirt startup"}}`,
	})
	if err != nil {
		t.Fatalf("runtimeFromLibvirtCatalog: %v", err)
	}
	libvirtImpl, ok := libvirtRuntime.(*LibvirtRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *LibvirtRuntime", libvirtRuntime)
	}
	if libvirtImpl.startupScript != "echo libvirt startup" {
		t.Fatalf("libvirt startup script = %q", libvirtImpl.startupScript)
	}

	gceRuntime, err := runtimeFromGceCatalog(db.RuntimeCatalog{
		ID:         "rt-gce",
		Type:       db.RuntimeTypeGCE,
		ConfigJSON: `{"gce":{"project":"p","zone":"z","network":"n","subnetwork":"s","serviceAccountEmail":"svc@example.iam.gserviceaccount.com","startupScript":"echo gce startup"}}`,
	})
	if err != nil {
		t.Fatalf("runtimeFromGceCatalog: %v", err)
	}
	gceImpl, ok := gceRuntime.(*GceRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *GceRuntime", gceRuntime)
	}
	if gceImpl.startupScript != "echo gce startup" {
		t.Fatalf("gce startup script = %q", gceImpl.startupScript)
	}
}

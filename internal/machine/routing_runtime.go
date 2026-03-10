package machine

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type RuntimeCatalogStore interface {
	GetRuntimeByID(context.Context, string) (db.RuntimeCatalog, error)
}

type RuntimeFactory func(db.RuntimeCatalog) (Runtime, error)

type RoutingRuntime struct {
	runtimes map[string]Runtime
	store    RuntimeCatalogStore
	factory  map[string]RuntimeFactory
}

func NewRoutingRuntime(runtimes map[string]Runtime) *RoutingRuntime {
	return NewRoutingRuntimeWithCatalog(nil, runtimes)
}

func NewRoutingRuntimeWithCatalog(store RuntimeCatalogStore, runtimes map[string]Runtime) *RoutingRuntime {
	if runtimes == nil {
		runtimes = map[string]Runtime{}
	}
	return &RoutingRuntime{
		runtimes: runtimes,
		store:    store,
		factory: map[string]RuntimeFactory{
			db.RuntimeTypeLibvirt: runtimeFromLibvirtCatalog,
			db.RuntimeTypeGCE:     runtimeFromGceCatalog,
			db.RuntimeTypeLXD:     runtimeFromLxdCatalog,
		},
	}
}

func (r *RoutingRuntime) EnsureRunning(ctx context.Context, machine db.Machine, opts RuntimeStartOptions) (string, error) {
	runtime, err := r.runtimeFor(ctx, machine.RuntimeID)
	if err != nil {
		return "", err
	}
	return runtime.EnsureRunning(ctx, machine, opts)
}

func (r *RoutingRuntime) EnsureStopped(ctx context.Context, machine db.Machine) error {
	runtime, err := r.runtimeFor(ctx, machine.RuntimeID)
	if err != nil {
		return err
	}
	return runtime.EnsureStopped(ctx, machine)
}

func (r *RoutingRuntime) EnsureDeleted(ctx context.Context, machine db.Machine) error {
	runtime, err := r.runtimeFor(ctx, machine.RuntimeID)
	if err != nil {
		return err
	}
	return runtime.EnsureDeleted(ctx, machine)
}

func (r *RoutingRuntime) IsRunning(ctx context.Context, machine db.Machine) (bool, string, error) {
	runtime, err := r.runtimeFor(ctx, machine.RuntimeID)
	if err != nil {
		return false, "", err
	}
	return runtime.IsRunning(ctx, machine)
}

func (r *RoutingRuntime) runtimeFor(ctx context.Context, runtimeID string) (Runtime, error) {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return nil, fmt.Errorf("runtime is not specified")
	}

	runtime, ok := r.runtimes[runtimeID]
	if ok && runtime != nil {
		return runtime, nil
	}

	if r.store == nil {
		return nil, fmt.Errorf("runtime %q is not configured", runtimeID)
	}

	catalogRuntime, err := r.store.GetRuntimeByID(ctx, runtimeID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("runtime %q not found", runtimeID)
		}
		return nil, fmt.Errorf("load runtime %q: %w", runtimeID, err)
	}

	factory := r.factory[catalogRuntime.Type]
	if factory == nil {
		return nil, fmt.Errorf("runtime type %q is not supported", catalogRuntime.Type)
	}

	runtime, err = factory(catalogRuntime)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime %q: %w", runtimeID, err)
	}
	if runtime == nil {
		return nil, fmt.Errorf("runtime %q is not configured", runtimeID)
	}
	return runtime, nil
}

func runtimeFromLibvirtCatalog(catalogRuntime db.RuntimeCatalog) (Runtime, error) {
	config := &arcav1.RuntimeConfig{}
	if err := protojson.Unmarshal([]byte(catalogRuntime.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode runtime config: %w", err)
	}

	libvirt := config.GetLibvirt()
	if libvirt == nil {
		return nil, fmt.Errorf("libvirt runtime config is missing")
	}

	return NewLibvirtRuntimeWithOptions(LibvirtRuntimeOptions{
		URI:           strings.TrimSpace(libvirt.GetUri()),
		Network:       strings.TrimSpace(libvirt.GetNetwork()),
		StoragePool:   strings.TrimSpace(libvirt.GetStoragePool()),
		StartupScript: libvirt.GetStartupScript(),
	}), nil
}

func runtimeFromGceCatalog(catalogRuntime db.RuntimeCatalog) (Runtime, error) {
	config := &arcav1.RuntimeConfig{}
	if err := protojson.Unmarshal([]byte(catalogRuntime.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode runtime config: %w", err)
	}

	gce := config.GetGce()
	if gce == nil {
		return nil, fmt.Errorf("gce runtime config is missing")
	}

	runtime, err := NewGceRuntimeWithOptions(GceRuntimeOptions{
		Project:             strings.TrimSpace(gce.GetProject()),
		Zone:                strings.TrimSpace(gce.GetZone()),
		Network:             strings.TrimSpace(gce.GetNetwork()),
		Subnetwork:          strings.TrimSpace(gce.GetSubnetwork()),
		ServiceAccountEmail: strings.TrimSpace(gce.GetServiceAccountEmail()),
		StartupScript:       gce.GetStartupScript(),
		MachineType:         strings.TrimSpace(gce.GetMachineType()),
		DiskSizeGB:          gce.GetDiskSizeGb(),
		ImageProject:        strings.TrimSpace(gce.GetImageProject()),
		ImageFamily:         strings.TrimSpace(gce.GetImageFamily()),
	})
	if err != nil {
		return nil, err
	}
	return runtime, nil
}

func runtimeFromLxdCatalog(catalogRuntime db.RuntimeCatalog) (Runtime, error) {
	config := &arcav1.RuntimeConfig{}
	if err := protojson.Unmarshal([]byte(catalogRuntime.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode runtime config: %w", err)
	}

	lxd := config.GetLxd()
	if lxd == nil {
		return nil, fmt.Errorf("lxd runtime config is missing")
	}

	return NewLxdRuntimeWithOptions(LxdRuntimeOptions{
		Endpoint:      strings.TrimSpace(lxd.GetEndpoint()),
		StartupScript: lxd.GetStartupScript(),
	}), nil
}

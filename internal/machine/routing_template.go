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

type ProfileCatalogStore interface {
	GetMachineProfileByID(context.Context, string) (db.MachineProfile, error)
}

// TemplateCatalogStore is an alias for backward compatibility.
// Deprecated: Use ProfileCatalogStore instead.
type TemplateCatalogStore = ProfileCatalogStore

type ProfileFactory func(db.MachineProfile) (Runtime, error)

// TemplateFactory is an alias for backward compatibility.
// Deprecated: Use ProfileFactory instead.
type TemplateFactory = ProfileFactory

type RoutingTemplate struct {
	runtimes map[string]Runtime
	store    ProfileCatalogStore
	factory  map[string]ProfileFactory
}

func NewRoutingTemplate(runtimes map[string]Runtime) *RoutingTemplate {
	return NewRoutingTemplateWithCatalog(nil, runtimes)
}

func NewRoutingTemplateWithCatalog(store ProfileCatalogStore, runtimes map[string]Runtime) *RoutingTemplate {
	if runtimes == nil {
		runtimes = map[string]Runtime{}
	}
	return &RoutingTemplate{
		runtimes: runtimes,
		store:    store,
		factory: map[string]ProfileFactory{
			db.ProviderTypeLibvirt: profileFromLibvirtConfig,
			db.ProviderTypeGCE:    profileFromGceConfig,
			db.ProviderTypeLXD:    profileFromLxdConfig,
		},
	}
}

// RegisterMockFactory registers the mock provider factory.
func (r *RoutingTemplate) RegisterMockFactory(mockRT *MockRuntime) {
	r.factory[db.ProviderTypeMock] = func(_ db.MachineProfile) (Runtime, error) {
		return mockRT, nil
	}
}

func (r *RoutingTemplate) EnsureRunning(ctx context.Context, machine db.Machine, opts RuntimeStartOptions) (string, error) {
	runtime, err := r.runtimeForMachine(ctx, machine)
	if err != nil {
		return "", err
	}
	return runtime.EnsureRunning(ctx, machine, opts)
}

func (r *RoutingTemplate) EnsureStopped(ctx context.Context, machine db.Machine) error {
	runtime, err := r.runtimeForMachine(ctx, machine)
	if err != nil {
		return err
	}
	return runtime.EnsureStopped(ctx, machine)
}

func (r *RoutingTemplate) EnsureDeleted(ctx context.Context, machine db.Machine) error {
	runtime, err := r.runtimeForMachine(ctx, machine)
	if err != nil {
		return err
	}
	return runtime.EnsureDeleted(ctx, machine)
}

func (r *RoutingTemplate) IsRunning(ctx context.Context, machine db.Machine) (bool, string, error) {
	runtime, err := r.runtimeForMachine(ctx, machine)
	if err != nil {
		return false, "", err
	}
	return runtime.IsRunning(ctx, machine)
}

func (r *RoutingTemplate) GetMachineInfo(ctx context.Context, machine db.Machine) (*RuntimeMachineInfo, error) {
	runtime, err := r.runtimeForMachine(ctx, machine)
	if err != nil {
		return nil, err
	}
	return runtime.GetMachineInfo(ctx, machine)
}

func (r *RoutingTemplate) CreateImage(ctx context.Context, machine db.Machine, imageName string) (map[string]string, error) {
	runtime, err := r.runtimeForMachine(ctx, machine)
	if err != nil {
		return nil, err
	}
	return runtime.CreateImage(ctx, machine, imageName)
}

// runtimeForMachine resolves a Runtime using the machine's snapshotted
// provider type and infrastructure config. Falls back to profile lookup
// when the snapshot is empty (pre-migration machines).
func (r *RoutingTemplate) runtimeForMachine(ctx context.Context, machine db.Machine) (Runtime, error) {
	profileID := strings.TrimSpace(machine.ProfileID)
	if profileID == "" {
		return nil, fmt.Errorf("profile is not specified")
	}

	// Check static runtimes first (used in tests)
	runtime, ok := r.runtimes[profileID]
	if ok && runtime != nil {
		return runtime, nil
	}

	// Prefer machine's snapshotted provider type and infrastructure config
	providerType := strings.TrimSpace(machine.ProviderType)
	configJSON := strings.TrimSpace(machine.InfrastructureConfigJSON)
	if providerType != "" && configJSON != "" && configJSON != "{}" {
		factory := r.factory[providerType]
		if factory == nil {
			return nil, fmt.Errorf("provider type %q is not supported", providerType)
		}
		rt, err := factory(db.MachineProfile{
			ID:         profileID,
			Type:       providerType,
			ConfigJSON: configJSON,
		})
		if err != nil {
			return nil, fmt.Errorf("resolve profile %q from snapshot: %w", profileID, err)
		}
		if rt == nil {
			return nil, fmt.Errorf("profile %q is not configured", profileID)
		}
		return rt, nil
	}

	// Fallback: look up from profile catalog (pre-migration machines)
	if r.store == nil {
		return nil, fmt.Errorf("profile %q is not configured", profileID)
	}

	catalogProfile, err := r.store.GetMachineProfileByID(ctx, profileID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("profile %q not found", profileID)
		}
		return nil, fmt.Errorf("load profile %q: %w", profileID, err)
	}

	factory := r.factory[catalogProfile.Type]
	if factory == nil {
		return nil, fmt.Errorf("provider type %q is not supported", catalogProfile.Type)
	}

	rt, err := factory(catalogProfile)
	if err != nil {
		return nil, fmt.Errorf("resolve profile %q: %w", profileID, err)
	}
	if rt == nil {
		return nil, fmt.Errorf("profile %q is not configured", profileID)
	}
	return rt, nil
}

var profileConfigUnmarshaler = protojson.UnmarshalOptions{DiscardUnknown: true}

func profileFromLibvirtConfig(profile db.MachineProfile) (Runtime, error) {
	config := &arcav1.MachineProfileConfig{}
	if err := profileConfigUnmarshaler.Unmarshal([]byte(profile.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode profile config: %w", err)
	}

	libvirt := config.GetLibvirt()
	if libvirt == nil {
		return nil, fmt.Errorf("libvirt profile config is missing")
	}

	return NewLibvirtRuntimeWithOptions(LibvirtRuntimeOptions{
		URI:           strings.TrimSpace(libvirt.GetUri()),
		Network:       strings.TrimSpace(libvirt.GetNetwork()),
		StoragePool:   strings.TrimSpace(libvirt.GetStoragePool()),
		StartupScript: libvirt.GetStartupScript(),
	}), nil
}

func profileFromGceConfig(profile db.MachineProfile) (Runtime, error) {
	config := &arcav1.MachineProfileConfig{}
	if err := profileConfigUnmarshaler.Unmarshal([]byte(profile.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode profile config: %w", err)
	}

	gce := config.GetGce()
	if gce == nil {
		return nil, fmt.Errorf("gce profile config is missing")
	}

	runtime, err := NewGceRuntimeWithOptions(GceRuntimeOptions{
		Project:             strings.TrimSpace(gce.GetProject()),
		Zone:                strings.TrimSpace(gce.GetZone()),
		Network:             strings.TrimSpace(gce.GetNetwork()),
		Subnetwork:          strings.TrimSpace(gce.GetSubnetwork()),
		ServiceAccountEmail: strings.TrimSpace(gce.GetServiceAccountEmail()),
		StartupScript:       gce.GetStartupScript(),
		DiskSizeGB:          gce.GetDiskSizeGb(),
	})
	if err != nil {
		return nil, err
	}
	return runtime, nil
}

func profileFromLxdConfig(profile db.MachineProfile) (Runtime, error) {
	config := &arcav1.MachineProfileConfig{}
	if err := profileConfigUnmarshaler.Unmarshal([]byte(profile.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode profile config: %w", err)
	}

	lxd := config.GetLxd()
	if lxd == nil {
		return nil, fmt.Errorf("lxd profile config is missing")
	}

	return NewLxdRuntimeWithOptions(LxdRuntimeOptions{
		Endpoint:      strings.TrimSpace(lxd.GetEndpoint()),
		StartupScript: lxd.GetStartupScript(),
	}), nil
}

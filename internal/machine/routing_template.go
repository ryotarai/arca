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

type TemplateCatalogStore interface {
	GetMachineTemplateByID(context.Context, string) (db.MachineTemplate, error)
}

type TemplateFactory func(db.MachineTemplate) (Runtime, error)

type RoutingTemplate struct {
	runtimes map[string]Runtime
	store    TemplateCatalogStore
	factory  map[string]TemplateFactory
}

func NewRoutingTemplate(runtimes map[string]Runtime) *RoutingTemplate {
	return NewRoutingTemplateWithCatalog(nil, runtimes)
}

func NewRoutingTemplateWithCatalog(store TemplateCatalogStore, runtimes map[string]Runtime) *RoutingTemplate {
	if runtimes == nil {
		runtimes = map[string]Runtime{}
	}
	return &RoutingTemplate{
		runtimes: runtimes,
		store:    store,
		factory: map[string]TemplateFactory{
			db.TemplateTypeLibvirt: templateFromLibvirtConfig,
			db.TemplateTypeGCE:    templateFromGceConfig,
			db.TemplateTypeLXD:    templateFromLxdConfig,
		},
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

// runtimeForMachine resolves a Runtime using the machine's snapshotted
// template type and config. Falls back to template lookup when
// the snapshot is empty (pre-migration machines).
func (r *RoutingTemplate) runtimeForMachine(ctx context.Context, machine db.Machine) (Runtime, error) {
	templateID := strings.TrimSpace(machine.TemplateID)
	if templateID == "" {
		return nil, fmt.Errorf("template is not specified")
	}

	// Check static runtimes first (used in tests)
	runtime, ok := r.runtimes[templateID]
	if ok && runtime != nil {
		return runtime, nil
	}

	// Prefer machine's snapshotted template type and config
	templateType := strings.TrimSpace(machine.TemplateType)
	configJSON := strings.TrimSpace(machine.TemplateConfigJSON)
	if templateType != "" && configJSON != "" && configJSON != "{}" {
		factory := r.factory[templateType]
		if factory == nil {
			return nil, fmt.Errorf("template type %q is not supported", templateType)
		}
		rt, err := factory(db.MachineTemplate{
			ID:         templateID,
			Type:       templateType,
			ConfigJSON: configJSON,
		})
		if err != nil {
			return nil, fmt.Errorf("resolve template %q from snapshot: %w", templateID, err)
		}
		if rt == nil {
			return nil, fmt.Errorf("template %q is not configured", templateID)
		}
		return rt, nil
	}

	// Fallback: look up from template catalog (pre-migration machines)
	if r.store == nil {
		return nil, fmt.Errorf("template %q is not configured", templateID)
	}

	catalogTemplate, err := r.store.GetMachineTemplateByID(ctx, templateID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("template %q not found", templateID)
		}
		return nil, fmt.Errorf("load template %q: %w", templateID, err)
	}

	factory := r.factory[catalogTemplate.Type]
	if factory == nil {
		return nil, fmt.Errorf("template type %q is not supported", catalogTemplate.Type)
	}

	rt, err := factory(catalogTemplate)
	if err != nil {
		return nil, fmt.Errorf("resolve template %q: %w", templateID, err)
	}
	if rt == nil {
		return nil, fmt.Errorf("template %q is not configured", templateID)
	}
	return rt, nil
}

var templateConfigUnmarshaler = protojson.UnmarshalOptions{DiscardUnknown: true}

func templateFromLibvirtConfig(catalogTemplate db.MachineTemplate) (Runtime, error) {
	config := &arcav1.MachineTemplateConfig{}
	if err := templateConfigUnmarshaler.Unmarshal([]byte(catalogTemplate.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode template config: %w", err)
	}

	libvirt := config.GetLibvirt()
	if libvirt == nil {
		return nil, fmt.Errorf("libvirt template config is missing")
	}

	return NewLibvirtRuntimeWithOptions(LibvirtRuntimeOptions{
		URI:           strings.TrimSpace(libvirt.GetUri()),
		Network:       strings.TrimSpace(libvirt.GetNetwork()),
		StoragePool:   strings.TrimSpace(libvirt.GetStoragePool()),
		StartupScript: libvirt.GetStartupScript(),
	}), nil
}

func templateFromGceConfig(catalogTemplate db.MachineTemplate) (Runtime, error) {
	config := &arcav1.MachineTemplateConfig{}
	if err := templateConfigUnmarshaler.Unmarshal([]byte(catalogTemplate.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode template config: %w", err)
	}

	gce := config.GetGce()
	if gce == nil {
		return nil, fmt.Errorf("gce template config is missing")
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

func templateFromLxdConfig(catalogTemplate db.MachineTemplate) (Runtime, error) {
	config := &arcav1.MachineTemplateConfig{}
	if err := templateConfigUnmarshaler.Unmarshal([]byte(catalogTemplate.ConfigJSON), config); err != nil {
		return nil, fmt.Errorf("decode template config: %w", err)
	}

	lxd := config.GetLxd()
	if lxd == nil {
		return nil, fmt.Errorf("lxd template config is missing")
	}

	return NewLxdRuntimeWithOptions(LxdRuntimeOptions{
		Endpoint:      strings.TrimSpace(lxd.GetEndpoint()),
		StartupScript: lxd.GetStartupScript(),
	}), nil
}

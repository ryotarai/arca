package machine

import (
	"context"
	"fmt"

	"github.com/ryotarai/arca/internal/db"
)

type RoutingRuntime struct {
	runtimes map[string]Runtime
}

func NewRoutingRuntime(runtimes map[string]Runtime) *RoutingRuntime {
	return &RoutingRuntime{runtimes: runtimes}
}

func (r *RoutingRuntime) EnsureRunning(ctx context.Context, machine db.Machine, opts RuntimeStartOptions) (string, error) {
	runtime, err := r.runtimeFor(machine.Runtime)
	if err != nil {
		return "", err
	}
	return runtime.EnsureRunning(ctx, machine, opts)
}

func (r *RoutingRuntime) EnsureStopped(ctx context.Context, machine db.Machine) error {
	runtime, err := r.runtimeFor(machine.Runtime)
	if err != nil {
		return err
	}
	return runtime.EnsureStopped(ctx, machine)
}

func (r *RoutingRuntime) IsRunning(ctx context.Context, machine db.Machine) (bool, string, error) {
	runtime, err := r.runtimeFor(machine.Runtime)
	if err != nil {
		return false, "", err
	}
	return runtime.IsRunning(ctx, machine)
}

func (r *RoutingRuntime) WaitReady(ctx context.Context, machine db.Machine, instanceID string) error {
	runtime, err := r.runtimeFor(machine.Runtime)
	if err != nil {
		return err
	}
	return runtime.WaitReady(ctx, machine, instanceID)
}

func (r *RoutingRuntime) runtimeFor(runtimeName string) (Runtime, error) {
	normalized := db.NormalizeMachineRuntime(runtimeName)
	runtime, ok := r.runtimes[normalized]
	if !ok || runtime == nil {
		return nil, fmt.Errorf("runtime %q is not configured", normalized)
	}
	return runtime, nil
}

# Mock Runtime for E2E Testing and Development

## Goals

1. **E2E test speedup** — Replace slow LXD/libvirt provisioning with a mock runtime so `fast` tests cover all machine lifecycle use cases in seconds.
2. **Dev environment simplification** — Allow local development of machine-related features without LXD/libvirt setup.
3. **Maintain LXD integration tests** — Keep existing `slow` LXD tests as real-environment validation.

## Non-Goals

- Full arcad simulation inside mock machines.
- Replacing LXD tests entirely.
- Multi-node or distributed mock scenarios.

## Design

### New Provider Type

Add `ProviderTypeMock = "mock"` to `internal/db/machine_profile_store.go` alongside existing `lxd`, `libvirt`, and `gce` types.

Add `MACHINE_PROFILE_TYPE_MOCK = 4` to the `MachineProfileType` enum in `proto/arca/v1/machine_profile.proto`.

### MockRuntime Implementation

**File:** `internal/machine/mock_runtime.go`

A single `MockRuntime` instance implements the `Runtime` interface and manages all mock machines in-memory. It also holds a reference to the store for reporting machine readiness.

```go
type MockRuntime struct {
    mu              sync.Mutex
    store           MockRuntimeStore  // for readiness reporting
    machines        map[string]*mockMachine
    defaultBehavior MockBehavior
    machineBehaviors map[string]MockBehavior
}

type MockRuntimeStore interface {
    ReportMachineReadinessByMachineID(ctx context.Context, machineID string, ready bool) error
}

type mockMachine struct {
    status      string        // "running", "stopped"
    containerID string        // "mock-<machineID>"
    stubServer  *http.Server
    stubAddr    string        // "127.0.0.1:<port>"
}

type MockBehavior struct {
    Delay   time.Duration
    ErrorOn map[string]string // operation name → error message
}
```

#### Runtime Interface Methods

The `Runtime` interface has 6 methods. MockRuntime implements all of them:

| Method | Behavior |
|--------|----------|
| `EnsureRunning` | Check error injection → context-aware delay → start stub HTTP server on random port → report readiness to store → set status to `running` → return `containerID` (string `"mock-<machineID>"`) |
| `EnsureStopped` | Check error injection → context-aware delay → stop stub server → set status to `stopped` |
| `EnsureDeleted` | Check error injection → context-aware delay → stop stub server → remove from map |
| `IsRunning` | Return in-memory status and `containerID` immediately (no delay, no error injection) |
| `GetMachineInfo` | Return `RuntimeMachineInfo{PrivateIP: "<stubAddr IP>"}` from the stub server's bound address |
| `CreateImage` | Return `ErrImageCreationNotSupported` (mock does not support image creation) |

**Key detail: `EnsureRunning` return value.** Real runtimes return a container identifier (e.g., LXD returns `"arca-machine-xxx"`), not an IP address. The worker stores this as `machine.ContainerID`. IP resolution happens separately through `GetMachineInfo` → `RuntimeMachineInfo.PrivateIP`, which the proxy layer reads via `MachineIPCache`. MockRuntime follows this pattern: `EnsureRunning` returns `"mock-<machineID>"` as the container ID, and `GetMachineInfo` returns the stub server's address as `PrivateIP`.

**Key detail: `IsRunning` return value.** The signature is `(bool, string, error)` where the string is the `containerID`. The reconcile loop compares this with the stored `ContainerID`. MockRuntime returns `(true, "mock-<machineID>", nil)` when running, `(false, "", nil)` when stopped, to avoid reconcile thrashing.

#### Readiness Gate

After `EnsureRunning`, the worker calls `waitMachineReady`, which polls `GetMachineReadinessByMachineID` from the database. In real runtimes, arcad running inside the VM reports readiness back to the server. Mock machines have no arcad.

**Solution:** `MockRuntime.EnsureRunning` directly calls `store.ReportMachineReadinessByMachineID(ctx, machineID, true)` after starting the stub server. This writes the readiness flag to the database before `EnsureRunning` returns, so `waitMachineReady` finds the machine ready on its first poll.

This requires `MockRuntime` to hold a `MockRuntimeStore` interface (a subset of `*db.Store`) for the readiness report call. The constructor is:

```go
func NewMockRuntime(store MockRuntimeStore) *MockRuntime
```

#### Behavior Resolution

Per-machine behavior takes priority over default behavior. If neither is set, operations complete instantly with no errors.

```
resolve(machineID, operation):
  if machineBehaviors[machineID] exists:
    use machineBehaviors[machineID]
  else:
    use defaultBehavior

  if behavior.ErrorOn[operation] is set:
    return error
  if behavior.Delay > 0:
    sleepContext(ctx, delay)  // respect context cancellation
  proceed with operation
```

Context-aware delay uses the same `sleepContext` pattern as the existing worker (`worker.go:710-720`), returning early if the context is cancelled during shutdown.

#### Graceful Shutdown

`MockRuntime` exposes a `Shutdown(ctx context.Context)` method that stops all running stub HTTP servers. Called during server shutdown in `cmd/server/main.go` to prevent port leaks in long-running dev sessions.

### Stub HTTP Server

Each mock machine runs a lightweight HTTP server on a random port when in `running` state:

- `GET /` → `200 OK` with `Content-Type: application/json` and body `{"machine_id": "<id>", "status": "running"}`
- All other paths → `200 OK` with same response (proxy may forward to arbitrary paths)
- Server binds to `127.0.0.1:0` (OS-assigned port)

The stub server's address is stored in `mockMachine.stubAddr` and exposed via `GetMachineInfo` → `RuntimeMachineInfo{PrivateIP: "<ip>"}`. The existing proxy mechanism reads this through `MachineIPCache` → `GetMachineInfo` and routes requests to the stub server without modification.

The stub server is stopped when the machine is stopped or deleted.

### Control API

**File:** `proto/arca/v1/mock.proto`

```protobuf
syntax = "proto3";

package arca.v1;

service MockService {
  // Set default behavior for all mock machines
  rpc SetDefaultBehavior(SetDefaultBehaviorRequest) returns (SetDefaultBehaviorResponse);
  // Set behavior for a specific machine (overrides default)
  rpc SetMachineBehavior(SetMachineBehaviorRequest) returns (SetMachineBehaviorResponse);
  // Reset all behavior and clear in-memory machine state
  rpc ResetBehavior(ResetBehaviorRequest) returns (ResetBehaviorResponse);
}

message MockBehavior {
  // Delay in milliseconds before each operation completes
  int64 delay_ms = 1;
  // Map of operation name to error message. Operations: "EnsureRunning", "EnsureStopped", "EnsureDeleted"
  map<string, string> error_on = 2;
}

message SetDefaultBehaviorRequest {
  MockBehavior behavior = 1;
}
message SetDefaultBehaviorResponse {}

message SetMachineBehaviorRequest {
  string machine_id = 1;
  MockBehavior behavior = 2;
}
message SetMachineBehaviorResponse {}

message ResetBehaviorRequest {}
message ResetBehaviorResponse {}
```

**Note:** `delay_ms` is `int64` for consistency with existing proto conventions (e.g., `auto_stop_timeout_seconds` in `machine_profile.proto`).

**Note:** `ResetBehavior` resets both behavior configuration AND clears the in-memory machine map (stopping any running stub servers). This ensures clean state between tests even if a previous test leaked machines.

The `MockService` handler holds a reference to the singleton `MockRuntime` and delegates directly to it. It requires the same authentication as other services (Bearer token via `authenticateAdmin`).

### Enablement Control

Mock runtime is gated by the environment variable `ARCA_ENABLE_MOCK=true`.

When **disabled** (default):
- `mock` provider type is not registered in the RoutingTemplate factory
- `MockService` ConnectRPC handler is not registered
- UI does not show `Mock` in the provider type dropdown

When **enabled**:
- `mock` factory is registered in RoutingTemplate
- `MockService` handler is registered
- UI shows `Mock` as a provider type option

### RoutingTemplate Integration

**File:** `internal/machine/routing_template.go`

Register mock factory conditionally:

```go
func (rt *RoutingTemplate) RegisterMockFactory(mockRT *MockRuntime) {
    rt.factory[db.ProviderTypeMock] = func(profile db.MachineProfile) (Runtime, error) {
        return mockRT, nil
    }
}
```

**Config JSON handling:** The `runtimeForMachine` method has a guard (`providerType != "" && configJSON != "" && configJSON != "{}"`). To ensure mock machines pass this guard, mock profile config should include a non-empty body. Use `{"mock": true}` as the minimum config for mock profiles, and document this in the UI (auto-populate when Mock provider is selected).

### Server Initialization

**File:** `cmd/server/main.go`

```go
if os.Getenv("ARCA_ENABLE_MOCK") == "true" {
    mockRT := machine.NewMockRuntime(store)
    runtime.RegisterMockFactory(mockRT)
    // Register MockService ConnectRPC handler
    mockHandler := server.NewMockServiceHandler(mockRT)
    mux.Handle(mockHandler...)
    // Register shutdown hook
    defer mockRT.Shutdown(context.Background())
}
```

### Mock Profile Configuration

Mock profiles use a minimal but non-empty config JSON to pass the RoutingTemplate's config guard:

```json
{
  "mock": true
}
```

No infrastructure connection details are needed.

### UI Changes

In the profile creation form (`web/src/`):
- Query the server for whether mock provider is enabled (via existing config/settings endpoint or a new field)
- Conditionally show `Mock` in the provider type dropdown
- When `Mock` is selected, auto-populate config with `{"mock": true}` and show minimal or empty configuration form

The `typeMap` in `web/e2e/helpers/machine-profile.ts` needs to be updated with `mock: 4` to match the new proto enum value.

## E2E Test Strategy

### Test Configuration

| Project | Runtime | Timeout | Coverage |
|---------|---------|---------|----------|
| `fast` | Mock | 60s | All use cases |
| `slow` | LXD | 600s | Real-environment integration |

### Server Startup

Playwright config adds `ARCA_ENABLE_MOCK` to the existing `env` map (not as a command prefix):

```typescript
// playwright.config.ts
webServer: {
  command: '...',
  env: {
    ...process.env,
    ARCA_ENABLE_MOCK: 'true',
  },
}
```

### New E2E Test Files

| File | Purpose |
|------|---------|
| `web/e2e/helpers/mock.ts` | MockService API helpers (`setDefaultBehavior`, `setMachineBehavior`, `resetBehavior`) |
| `web/e2e/machines.spec.ts` | Expand existing tests: full lifecycle (create → running → stop → start → delete) with mock runtime |
| `web/e2e/machine-errors.spec.ts` | Error injection: startup failure → Failed state display, stop failure handling |
| `web/e2e/machine-proxy.spec.ts` | Proxy connection to stub HTTP server |

New mock-based tests create mock-type profiles (not LXD profiles). Existing tests in `machines.spec.ts` that use `ensureLxdProfile` continue to work as-is; new lifecycle tests use a separate `ensureMockProfile` helper.

None of the new file names match the `testIgnore` regex in playwright config (`/(?:lxd-provisioning|critical-user-journey)\.spec\.ts$/`), so they correctly run in the `fast` project.

### Test Cleanup

Each test calls `resetBehavior()` in `afterEach` to ensure clean state between tests. `ResetBehavior` clears both behavior configuration and in-memory machine state (including stopping stub servers).

### Use Cases Covered by Fast Tests

| Use Case | How |
|----------|-----|
| Machine create → Running | Default behavior (0 delay) |
| Machine stop / restart | Default behavior |
| Machine delete | Default behavior |
| Starting/Stopping intermediate states | `setDefaultBehavior({ delayMs: 2000 })` |
| Startup failure → Failed state | `setMachineBehavior(id, { errorOn: { EnsureRunning: "simulated error" } })` |
| Proxy connection to machine | Connect through proxy to stub HTTP server |
| Idle timeout auto-stop | Short timeout config + mock runtime |

## File Changes Summary

| File | Change |
|------|--------|
| `proto/arca/v1/mock.proto` | New: MockService protobuf definition |
| `proto/arca/v1/machine_profile.proto` | Add `MACHINE_PROFILE_TYPE_MOCK = 4` to enum |
| `internal/machine/mock_runtime.go` | New: MockRuntime implementation |
| `internal/machine/mock_runtime_test.go` | New: Unit tests |
| `internal/db/machine_profile_store.go` | Add `ProviderTypeMock` constant |
| `internal/machine/routing_template.go` | Add `RegisterMockFactory` method |
| `cmd/server/main.go` | Conditional MockRuntime init, handler registration, shutdown hook |
| `internal/server/server.go` | MockService handler routing |
| `web/src/` (profile form) | Mock provider option (conditionally shown, auto-populate config) |
| `web/e2e/helpers/mock.ts` | New: MockService test helpers |
| `web/e2e/helpers/machine-profile.ts` | Add `mock: 4` to typeMap |
| `web/e2e/machines.spec.ts` | Expand with full lifecycle tests using mock profiles |
| `web/e2e/machine-errors.spec.ts` | New: Error injection tests |
| `web/e2e/machine-proxy.spec.ts` | New: Proxy connection tests |
| `web/playwright.config.ts` | Add `ARCA_ENABLE_MOCK: 'true'` to webServer env |

## Data Flow

```
E2E Test
  │
  ├── MockService (Control API) ──→ MockRuntime.SetBehavior()
  │
  ├── MachineService (normal API) ──→ Worker ──→ RoutingTemplate
  │                                                   │
  │                                        ┌──────────┴──────────┐
  │                                        │ provider_type=mock   │
  │                                        ▼                      │
  │                                   MockRuntime                 │
  │                                     │                         │
  │                        ┌────────────┼────────────┐            │
  │                        ▼            ▼            ▼             │
  │                  In-memory     Stub HTTP    Store.Report       │
  │                  state         Server       Readiness          │
  │                                  ▲                             │
  │                                  │                             │
  └── Proxy test ───────────────────┘          (LXD/GCE/Libvirt)
```

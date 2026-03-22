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

### MockRuntime Implementation

**File:** `internal/machine/mock_runtime.go`

A single `MockRuntime` instance implements the `Runtime` interface and manages all mock machines in-memory.

```go
type MockRuntime struct {
    mu             sync.Mutex
    machines       map[string]*mockMachine
    defaultBehavior MockBehavior
    machineBehaviors map[string]MockBehavior
}

type mockMachine struct {
    status     string        // "running", "stopped"
    stubServer *http.Server
    stubAddr   string        // "127.0.0.1:<port>"
}

type MockBehavior struct {
    Delay   time.Duration
    ErrorOn map[string]string // operation name → error message
}
```

#### Runtime Interface Methods

| Method | Behavior |
|--------|----------|
| `EnsureRunning` | Check error injection → wait configured delay → start stub HTTP server on random port → set status to `running` → return stub address |
| `EnsureStopped` | Check error injection → wait delay → stop stub server → set status to `stopped` |
| `EnsureDeleted` | Check error injection → wait delay → stop stub server → remove from map |
| `IsRunning` | Return in-memory status immediately (no delay, no error injection) |
| `GetMachineInfo` | Return stub address and basic info |

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
    sleep(delay)
  proceed with operation
```

### Stub HTTP Server

Each mock machine runs a lightweight HTTP server on a random port when in `running` state:

- `GET /` → `200 OK` with JSON `{"machine_id": "<id>", "status": "running"}`
- Server binds to `127.0.0.1:0` (OS-assigned port)
- The address is returned from `EnsureRunning` and stored in machine runtime state, so the existing proxy mechanism routes to it without modification

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
  // Reset all behavior to defaults (no delay, no errors)
  rpc ResetBehavior(ResetBehaviorRequest) returns (ResetBehaviorResponse);
}

message MockBehavior {
  // Delay in milliseconds before each operation completes
  int32 delay_ms = 1;
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

The `MockService` handler holds a reference to the singleton `MockRuntime` and delegates directly to it.

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
    rt.factory[db.ProviderTypeMock] = func(config json.RawMessage) (*TemplateProfile, error) {
        return &TemplateProfile{Runtime: mockRT}, nil
    }
}
```

### Server Initialization

**File:** `cmd/server/main.go`

```go
if os.Getenv("ARCA_ENABLE_MOCK") == "true" {
    mockRT := machine.NewMockRuntime()
    runtime.RegisterMockFactory(mockRT)
    // Register MockService ConnectRPC handler
    mockHandler := server.NewMockServiceHandler(mockRT)
    mux.Handle(mockHandler...)
}
```

### Mock Profile Configuration

Mock profiles require minimal configuration — no infrastructure connection details needed:

```json
{
  "provider_type": "mock"
}
```

### UI Changes

In the profile creation form (`web/src/`):
- Query the server for whether mock provider is enabled (via existing config/settings endpoint or a new field)
- Conditionally show `Mock` in the provider type dropdown
- When `Mock` is selected, show minimal or empty configuration form

## E2E Test Strategy

### Test Configuration

| Project | Runtime | Timeout | Coverage |
|---------|---------|---------|----------|
| `fast` | Mock | 60s | All use cases |
| `slow` | LXD | 600s | Real-environment integration |

### Server Startup

Playwright `webServer` starts the server with mock enabled:

```typescript
// playwright.config.ts
webServer: {
  command: 'ARCA_ENABLE_MOCK=true ./bin/server',
  // ...
}
```

### New E2E Test Files

| File | Purpose |
|------|---------|
| `web/e2e/helpers/mock.ts` | MockService API helpers (`setDefaultBehavior`, `setMachineBehavior`, `resetBehavior`) |
| `web/e2e/machines.spec.ts` | Expand existing tests: full lifecycle (create → running → stop → start → delete) with mock runtime |
| `web/e2e/machine-errors.spec.ts` | Error injection: startup failure → Failed state display, stop failure handling |
| `web/e2e/machine-proxy.spec.ts` | Proxy connection to stub HTTP server |

### Test Cleanup

Each test calls `resetBehavior()` in `afterEach` to ensure clean state between tests.

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
| `internal/machine/mock_runtime.go` | New: MockRuntime implementation |
| `internal/machine/mock_runtime_test.go` | New: Unit tests |
| `internal/db/machine_profile_store.go` | Add `ProviderTypeMock` constant |
| `internal/machine/routing_template.go` | Add `RegisterMockFactory` method |
| `cmd/server/main.go` | Conditional MockRuntime init and handler registration |
| `internal/server/server.go` | MockService handler routing |
| `web/src/` (profile form) | Mock provider option (conditionally shown) |
| `web/e2e/helpers/mock.ts` | New: MockService test helpers |
| `web/e2e/machines.spec.ts` | Expand with full lifecycle tests |
| `web/e2e/machine-errors.spec.ts` | New: Error injection tests |
| `web/e2e/machine-proxy.spec.ts` | New: Proxy connection tests |
| `web/playwright.config.ts` | Add `ARCA_ENABLE_MOCK=true` to server command |

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
  │                        ┌────────────┤                         │
  │                        ▼            ▼                          │
  │                  In-memory state  Stub HTTP Server             │
  │                                     ▲                         │
  │                                     │                         │
  └── Proxy test ──────────────────────┘        (LXD/GCE/Libvirt)
```

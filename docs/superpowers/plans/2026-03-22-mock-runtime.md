# Mock Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a mock runtime provider that enables fast E2E tests covering all machine lifecycle use cases, and simplifies local development without LXD/libvirt.

**Architecture:** New `mock` provider type implements the `Runtime` interface with in-memory state, stub HTTP servers on unique loopback IPs, and a ConnectRPC Control API for per-test behavior configuration. Gated behind `ARCA_ENABLE_MOCK` env var.

**Tech Stack:** Go 1.22, ConnectRPC/protobuf, React/TypeScript, Playwright

**Spec:** `docs/superpowers/specs/2026-03-22-mock-runtime-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `proto/arca/v1/machine_profile.proto` | Add `MACHINE_PROFILE_TYPE_MOCK` enum value and `MockProfileConfig` to oneof |
| `proto/arca/v1/mock.proto` | New: MockService with SetDefaultBehavior, SetMachineBehavior, ResetBehavior |
| `internal/db/machine_profile_store.go` | Add `ProviderTypeMock` constant |
| `internal/machine/mock_runtime.go` | New: MockRuntime implementing Runtime interface |
| `internal/machine/mock_runtime_test.go` | New: Unit tests for MockRuntime |
| `internal/machine/routing_template.go` | Add `RegisterMockFactory` method |
| `internal/server/mock_connect.go` | New: MockService ConnectRPC handler |
| `internal/server/router.go` | Wire MockService handler (conditional) |
| `internal/server/machine_profile_connect.go` | Accept mock profile type in validation |
| `cmd/server/main.go` | Conditional MockRuntime init |
| `web/src/lib/types.ts` | Add `mock` to `MachineProfileType` and `MachineProfileConfig` |
| `web/src/lib/api.ts` | Handle mock in proto conversion functions |
| `web/src/pages/MachineProfileFormPage.tsx` | Show Mock option conditionally |
| `web/e2e/helpers/mock.ts` | New: MockService API test helpers |
| `web/e2e/helpers/machine-profile.ts` | Add `mock: 4` to typeMap |
| `web/e2e/machines.spec.ts` | Expand with full lifecycle tests |
| `web/e2e/machine-errors.spec.ts` | New: Error injection tests |
| `web/e2e/machine-proxy.spec.ts` | New: Proxy connection tests |
| `web/playwright.config.ts` | Add `ARCA_ENABLE_MOCK` to webServer env |

---

## Task 1: Proto Definitions

**Files:**
- Modify: `proto/arca/v1/machine_profile.proto:15-20` (enum), `proto/arca/v1/machine_profile.proto:63-78` (MachineProfileConfig oneof)
- Create: `proto/arca/v1/mock.proto`

- [ ] **Step 1: Add MACHINE_PROFILE_TYPE_MOCK to enum**

In `proto/arca/v1/machine_profile.proto`, add to the `MachineProfileType` enum (after line 19):

```protobuf
MACHINE_PROFILE_TYPE_MOCK = 4;
```

- [ ] **Step 2: Add MockProfileConfig message and oneof variant**

In `proto/arca/v1/machine_profile.proto`, add an empty `MockProfileConfig` message before `MachineProfileConfig`, and add `MockProfileConfig mock = 8;` to the `oneof provider` block:

```protobuf
message MockProfileConfig {}
```

Inside `MachineProfileConfig.oneof provider` (after `LxdProfileConfig lxd = 4;`):

```protobuf
MockProfileConfig mock = 8;
```

- [ ] **Step 3: Create mock.proto**

Create `proto/arca/v1/mock.proto`:

```protobuf
syntax = "proto3";

package arca.v1;

option go_package = "github.com/ryotarai/arca/internal/gen/arca/v1;arcav1";

service MockService {
  rpc SetDefaultBehavior(SetDefaultBehaviorRequest) returns (SetDefaultBehaviorResponse);
  rpc SetMachineBehavior(SetMachineBehaviorRequest) returns (SetMachineBehaviorResponse);
  rpc ResetBehavior(ResetBehaviorRequest) returns (ResetBehaviorResponse);
}

message MockBehavior {
  int64 delay_ms = 1;
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

- [ ] **Step 4: Regenerate proto code**

Run: `make proto`

Expected: Generated files appear in `internal/gen/arca/v1/` and `web/src/gen/arca/v1/`.

- [ ] **Step 5: Verify build**

Run: `go vet ./...`

Expected: PASS (no errors).

- [ ] **Step 6: Commit**

```bash
git add proto/ internal/gen/ web/src/gen/
git commit -m "Add mock provider proto definitions and MockService"
```

---

## Task 2: Provider Type Constant and Backend Validation

**Files:**
- Modify: `internal/db/machine_profile_store.go:18-22`
- Modify: `internal/server/machine_profile_connect.go:289-395` (validateProfileRequest switch), `440-451` (profileTypeFromDB)

- [ ] **Step 1: Add ProviderTypeMock constant**

In `internal/db/machine_profile_store.go`, add to the const block (after line 21):

```go
ProviderTypeMock = "mock"
```

- [ ] **Step 2: Add mock case to profileTypeFromDB**

In `internal/server/machine_profile_connect.go`, add a case in `profileTypeFromDB()` (before the `default:` at line 448):

```go
case db.ProviderTypeMock:
    return arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_MOCK, nil
```

- [ ] **Step 3: Add mock case to validateProfileRequest**

In `internal/server/machine_profile_connect.go`, add a case in the `switch profileType` block (before `default:` at line 393):

```go
case arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_MOCK:
    mock := config.GetMock()
    if mock == nil {
        return validatedProfileRequest{}, errors.New("mock profile requires mock config")
    }
    return validatedProfileRequest{
        name:        normalizedName,
        profileType: db.ProviderTypeMock,
        config: &arcav1.MachineProfileConfig{
            Provider:               &arcav1.MachineProfileConfig_Mock{Mock: &arcav1.MockProfileConfig{}},
            Exposure:               exposureConfig,
            ServerApiUrl:           serverApiUrl,
            AutoStopTimeoutSeconds: autoStopTimeoutSeconds,
        },
    }, nil
```

- [ ] **Step 4: Verify build**

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/machine_profile_store.go internal/server/machine_profile_connect.go
git commit -m "Add mock provider type constant and backend validation"
```

---

## Task 3: MockRuntime Core Implementation

**Files:**
- Create: `internal/machine/mock_runtime.go`

- [ ] **Step 1: Create MockRuntime with types and constructor**

Create `internal/machine/mock_runtime.go`:

```go
package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

// MockRuntimeStore is the subset of *db.Store needed by MockRuntime.
type MockRuntimeStore interface {
	ReportMachineReadinessByMachineID(ctx context.Context, machineID string, ready bool, reason, containerID, arcadVersion string) (bool, error)
}

// MockBehavior controls delay and error injection for mock operations.
type MockBehavior struct {
	Delay   time.Duration
	ErrorOn map[string]string // operation name → error message
}

type mockMachine struct {
	status      string // "running", "stopped"
	containerID string
	stubServer  *http.Server
	stubIP      string
}

var nextLoopbackCounter atomic.Uint32

func init() {
	nextLoopbackCounter.Store(1) // first call to Add returns 2
}

func nextLoopbackIP() string {
	n := nextLoopbackCounter.Add(1)
	// Use 127.x.y.z format with proper byte wrapping within 127.0.0.0/8
	b3 := byte(n)
	b2 := byte(n >> 8)
	b1 := byte(n >> 16)
	if b3 == 0 { b3 = 1 } // avoid .0 addresses
	return fmt.Sprintf("127.%d.%d.%d", b1, b2, b3)
}

// MockRuntime implements the Runtime interface with in-memory state.
type MockRuntime struct {
	mu               sync.Mutex
	store            MockRuntimeStore
	machines         map[string]*mockMachine
	defaultBehavior  MockBehavior
	machineBehaviors map[string]MockBehavior
}

func NewMockRuntime(store MockRuntimeStore) *MockRuntime {
	return &MockRuntime{
		store:            store,
		machines:         make(map[string]*mockMachine),
		machineBehaviors: make(map[string]MockBehavior),
	}
}
```

- [ ] **Step 2: Add behavior resolution and context-aware sleep**

Append to `mock_runtime.go`:

```go
func (m *MockRuntime) resolveBehavior(machineID string) MockBehavior {
	if b, ok := m.machineBehaviors[machineID]; ok {
		return b
	}
	return m.defaultBehavior
}

func (m *MockRuntime) applyBehavior(ctx context.Context, machineID, operation string) error {
	m.mu.Lock()
	behavior := m.resolveBehavior(machineID)
	m.mu.Unlock()

	if msg, ok := behavior.ErrorOn[operation]; ok {
		return fmt.Errorf("mock error: %s", msg)
	}
	if behavior.Delay > 0 {
		return sleepContext(ctx, behavior.Delay)
	}
	return nil
}
```

- [ ] **Step 3: Add stub HTTP server helper**

Append to `mock_runtime.go`:

```go
func startStubServer(machineID, ip string) (*http.Server, error) {
	addr := net.JoinHostPort(ip, "21030")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"machine_id": machineID,
			"status":     "running",
		})
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	return srv, nil
}
```

- [ ] **Step 4: Implement EnsureRunning**

Append to `mock_runtime.go`:

```go
func (m *MockRuntime) EnsureRunning(ctx context.Context, machine db.Machine, _ RuntimeStartOptions) (string, error) {
	if err := m.applyBehavior(ctx, machine.ID, "EnsureRunning"); err != nil {
		return "", err
	}

	containerID := "mock-" + machine.ID

	m.mu.Lock()
	mm, exists := m.machines[machine.ID]
	if exists && mm.status == "running" {
		m.mu.Unlock()
		return containerID, nil
	}

	ip := nextLoopbackIP()
	srv, err := startStubServer(machine.ID, ip)
	if err != nil {
		m.mu.Unlock()
		return "", err
	}

	m.machines[machine.ID] = &mockMachine{
		status:      "running",
		containerID: containerID,
		stubServer:  srv,
		stubIP:      ip,
	}
	m.mu.Unlock()

	// Report readiness so waitMachineReady unblocks immediately.
	if m.store != nil {
		_, _ = m.store.ReportMachineReadinessByMachineID(ctx, machine.ID, true, "", containerID, "")
	}

	return containerID, nil
}
```

- [ ] **Step 5: Implement EnsureStopped, EnsureDeleted, IsRunning, GetMachineInfo, CreateImage**

Append to `mock_runtime.go`:

```go
func (m *MockRuntime) EnsureStopped(ctx context.Context, machine db.Machine) error {
	if err := m.applyBehavior(ctx, machine.ID, "EnsureStopped"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if mm, ok := m.machines[machine.ID]; ok {
		if mm.stubServer != nil {
			_ = mm.stubServer.Close()
			mm.stubServer = nil
		}
		mm.status = "stopped"
	}
	return nil
}

func (m *MockRuntime) EnsureDeleted(ctx context.Context, machine db.Machine) error {
	if err := m.applyBehavior(ctx, machine.ID, "EnsureDeleted"); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if mm, ok := m.machines[machine.ID]; ok {
		if mm.stubServer != nil {
			_ = mm.stubServer.Close()
		}
		delete(m.machines, machine.ID)
	}
	return nil
}

func (m *MockRuntime) IsRunning(_ context.Context, machine db.Machine) (bool, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mm, ok := m.machines[machine.ID]
	if !ok || mm.status != "running" {
		return false, "", nil
	}
	return true, mm.containerID, nil
}

func (m *MockRuntime) GetMachineInfo(_ context.Context, machine db.Machine) (*RuntimeMachineInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mm, ok := m.machines[machine.ID]
	if !ok {
		return nil, fmt.Errorf("mock machine %q not found", machine.ID)
	}
	return &RuntimeMachineInfo{PrivateIP: mm.stubIP}, nil
}

func (m *MockRuntime) CreateImage(_ context.Context, _ db.Machine, _ string) (map[string]string, error) {
	return nil, ErrImageCreationNotSupported
}
```

- [ ] **Step 6: Add Control API methods and Shutdown**

Append to `mock_runtime.go`:

```go
// SetDefaultBehavior sets the default behavior for all mock machines.
func (m *MockRuntime) SetDefaultBehavior(b MockBehavior) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultBehavior = b
}

// SetMachineBehavior sets behavior for a specific machine.
func (m *MockRuntime) SetMachineBehavior(machineID string, b MockBehavior) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.machineBehaviors[machineID] = b
}

// ResetBehavior clears all behavior config and stops all mock machines.
func (m *MockRuntime) ResetBehavior() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.defaultBehavior = MockBehavior{}
	m.machineBehaviors = make(map[string]MockBehavior)

	for _, mm := range m.machines {
		if mm.stubServer != nil {
			_ = mm.stubServer.Close()
		}
	}
	m.machines = make(map[string]*mockMachine)
}

// Shutdown stops all running stub servers.
func (m *MockRuntime) Shutdown(_ context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mm := range m.machines {
		if mm.stubServer != nil {
			_ = mm.stubServer.Close()
		}
	}
	m.machines = make(map[string]*mockMachine)
}
```

- [ ] **Step 7: Verify build**

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/machine/mock_runtime.go
git commit -m "Add MockRuntime implementing Runtime interface"
```

---

## Task 4: MockRuntime Unit Tests

**Files:**
- Create: `internal/machine/mock_runtime_test.go`

- [ ] **Step 1: Write tests for basic lifecycle**

Create `internal/machine/mock_runtime_test.go`:

```go
package machine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

type fakeReadinessStore struct {
	reported map[string]bool
}

func (s *fakeReadinessStore) ReportMachineReadinessByMachineID(_ context.Context, machineID string, ready bool, _, _, _ string) (bool, error) {
	if s.reported == nil {
		s.reported = make(map[string]bool)
	}
	s.reported[machineID] = ready
	return true, nil
}

func testMachine(id string) db.Machine {
	return db.Machine{ID: id}
}

func TestMockRuntime_EnsureRunning(t *testing.T) {
	store := &fakeReadinessStore{}
	rt := NewMockRuntime(store)
	defer rt.Shutdown(context.Background())

	containerID, err := rt.EnsureRunning(context.Background(), testMachine("m1"), RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	if containerID != "mock-m1" {
		t.Errorf("containerID = %q, want %q", containerID, "mock-m1")
	}
	if !store.reported["m1"] {
		t.Error("readiness not reported")
	}

	running, cid, err := rt.IsRunning(context.Background(), testMachine("m1"))
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if !running || cid != "mock-m1" {
		t.Errorf("IsRunning = (%v, %q), want (true, %q)", running, cid, "mock-m1")
	}
}

func TestMockRuntime_StubServer(t *testing.T) {
	rt := NewMockRuntime(&fakeReadinessStore{})
	defer rt.Shutdown(context.Background())

	_, err := rt.EnsureRunning(context.Background(), testMachine("m1"), RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}

	info, err := rt.GetMachineInfo(context.Background(), testMachine("m1"))
	if err != nil {
		t.Fatalf("GetMachineInfo: %v", err)
	}

	resp, err := http.Get(fmt.Sprintf("http://%s:21030/", info.PrivateIP))
	if err != nil {
		t.Fatalf("HTTP GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("empty response body")
	}
}

func TestMockRuntime_StopAndDelete(t *testing.T) {
	rt := NewMockRuntime(&fakeReadinessStore{})
	defer rt.Shutdown(context.Background())

	rt.EnsureRunning(context.Background(), testMachine("m1"), RuntimeStartOptions{})

	if err := rt.EnsureStopped(context.Background(), testMachine("m1")); err != nil {
		t.Fatalf("EnsureStopped: %v", err)
	}
	running, _, _ := rt.IsRunning(context.Background(), testMachine("m1"))
	if running {
		t.Error("machine still running after stop")
	}

	rt.EnsureRunning(context.Background(), testMachine("m2"), RuntimeStartOptions{})
	if err := rt.EnsureDeleted(context.Background(), testMachine("m2")); err != nil {
		t.Fatalf("EnsureDeleted: %v", err)
	}
	_, err := rt.GetMachineInfo(context.Background(), testMachine("m2"))
	if err == nil {
		t.Error("expected error for deleted machine")
	}
}

func TestMockRuntime_ErrorInjection(t *testing.T) {
	rt := NewMockRuntime(&fakeReadinessStore{})
	defer rt.Shutdown(context.Background())

	rt.SetMachineBehavior("m1", MockBehavior{
		ErrorOn: map[string]string{"EnsureRunning": "disk full"},
	})
	_, err := rt.EnsureRunning(context.Background(), testMachine("m1"), RuntimeStartOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockRuntime_Delay(t *testing.T) {
	rt := NewMockRuntime(&fakeReadinessStore{})
	defer rt.Shutdown(context.Background())

	rt.SetDefaultBehavior(MockBehavior{Delay: 200 * time.Millisecond})

	start := time.Now()
	rt.EnsureRunning(context.Background(), testMachine("m1"), RuntimeStartOptions{})
	elapsed := time.Since(start)
	if elapsed < 150*time.Millisecond {
		t.Errorf("delay too short: %v", elapsed)
	}
}

func TestMockRuntime_DelayContextCancel(t *testing.T) {
	rt := NewMockRuntime(&fakeReadinessStore{})
	defer rt.Shutdown(context.Background())

	rt.SetDefaultBehavior(MockBehavior{Delay: 5 * time.Second})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := rt.EnsureRunning(ctx, testMachine("m1"), RuntimeStartOptions{})
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func TestMockRuntime_ResetBehavior(t *testing.T) {
	rt := NewMockRuntime(&fakeReadinessStore{})
	defer rt.Shutdown(context.Background())

	rt.EnsureRunning(context.Background(), testMachine("m1"), RuntimeStartOptions{})
	rt.SetDefaultBehavior(MockBehavior{Delay: time.Second})
	rt.SetMachineBehavior("m1", MockBehavior{ErrorOn: map[string]string{"EnsureRunning": "fail"}})

	rt.ResetBehavior()

	// After reset: no machines, no behaviors
	running, _, _ := rt.IsRunning(context.Background(), testMachine("m1"))
	if running {
		t.Error("machine still running after reset")
	}
	// Should succeed without delay or error
	_, err := rt.EnsureRunning(context.Background(), testMachine("m1"), RuntimeStartOptions{})
	if err != nil {
		t.Fatalf("EnsureRunning after reset: %v", err)
	}
}

func TestMockRuntime_CreateImage(t *testing.T) {
	rt := NewMockRuntime(&fakeReadinessStore{})
	_, err := rt.CreateImage(context.Background(), testMachine("m1"), "img")
	if err != ErrImageCreationNotSupported {
		t.Errorf("err = %v, want ErrImageCreationNotSupported", err)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/machine/ -run TestMockRuntime -v`

Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/machine/mock_runtime_test.go
git commit -m "Add MockRuntime unit tests"
```

---

## Task 5: RoutingTemplate Integration

**Files:**
- Modify: `internal/machine/routing_template.go`

- [ ] **Step 1: Add RegisterMockFactory method**

In `internal/machine/routing_template.go`, add after the `NewRoutingTemplateWithCatalog` function (after line 52):

```go
// RegisterMockFactory registers the mock provider factory.
func (r *RoutingTemplate) RegisterMockFactory(mockRT *MockRuntime) {
	r.factory[db.ProviderTypeMock] = func(_ db.MachineProfile) (Runtime, error) {
		return mockRT, nil
	}
}
```

- [ ] **Step 2: Verify build**

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/machine/routing_template.go
git commit -m "Add RegisterMockFactory to RoutingTemplate"
```

---

## Task 6: MockService ConnectRPC Handler

**Files:**
- Create: `internal/server/mock_connect.go`

- [ ] **Step 1: Create MockService handler**

Create `internal/server/mock_connect.go`:

```go
package server

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/ryotarai/arca/internal/machine"

	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type mockConnectService struct {
	runtime       *machine.MockRuntime
	authenticator Authenticator
}

func newMockConnectService(runtime *machine.MockRuntime, authenticator Authenticator) *mockConnectService {
	return &mockConnectService{runtime: runtime, authenticator: authenticator}
}

func (s *mockConnectService) authenticateAdmin(ctx context.Context, header http.Header) error {
	result, err := s.authenticator.AuthenticateFull(ctx, authTokenFromHeader(header))
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if result.Role != "admin" {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("admin role required"))
	}
	return nil
}

func (s *mockConnectService) SetDefaultBehavior(ctx context.Context, req *connect.Request[arcav1.SetDefaultBehaviorRequest]) (*connect.Response[arcav1.SetDefaultBehaviorResponse], error) {
	if err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}
	b := toBehavior(req.Msg.GetBehavior())
	s.runtime.SetDefaultBehavior(b)
	return connect.NewResponse(&arcav1.SetDefaultBehaviorResponse{}), nil
}

func (s *mockConnectService) SetMachineBehavior(ctx context.Context, req *connect.Request[arcav1.SetMachineBehaviorRequest]) (*connect.Response[arcav1.SetMachineBehaviorResponse], error) {
	if err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}
	b := toBehavior(req.Msg.GetBehavior())
	s.runtime.SetMachineBehavior(req.Msg.GetMachineId(), b)
	return connect.NewResponse(&arcav1.SetMachineBehaviorResponse{}), nil
}

func (s *mockConnectService) ResetBehavior(ctx context.Context, req *connect.Request[arcav1.ResetBehaviorRequest]) (*connect.Response[arcav1.ResetBehaviorResponse], error) {
	if err := s.authenticateAdmin(ctx, req.Header()); err != nil {
		return nil, err
	}
	s.runtime.ResetBehavior()
	return connect.NewResponse(&arcav1.ResetBehaviorResponse{}), nil
}

func toBehavior(pb *arcav1.MockBehavior) machine.MockBehavior {
	if pb == nil {
		return machine.MockBehavior{}
	}
	return machine.MockBehavior{
		Delay:   time.Duration(pb.GetDelayMs()) * time.Millisecond,
		ErrorOn: pb.GetErrorOn(),
	}
}
```

Note: The `authenticateAdmin` helper and `authTokenFromHeader` should follow the pattern used in other services (e.g., `machineProfileConnectService`). During implementation, check how `authenticateAdmin` is implemented in `machine_profile_connect.go` and adapt. If those helpers are not exported, replicate the pattern or extract a shared helper.

- [ ] **Step 2: Verify build**

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/server/mock_connect.go
git commit -m "Add MockService ConnectRPC handler"
```

---

## Task 7: Server Wiring

**Files:**
- Modify: `internal/server/router.go:22-32` (Dependencies), `70-126` (NewRouter)
- Modify: `cmd/server/main.go:74`

- [ ] **Step 1: Add MockRuntime to Dependencies**

In `internal/server/router.go`, add to the `Dependencies` struct (after `RateLimiter`):

```go
MockRuntime  *machine.MockRuntime
```

Add the import for `"github.com/ryotarai/arca/internal/machine"` if not present.

- [ ] **Step 2: Register MockService handler conditionally in NewRouter**

In `internal/server/router.go`, add after the image service handler block (after line 126):

```go
if deps.MockRuntime != nil && deps.Authenticator != nil {
    path, handler := arcav1connect.NewMockServiceHandler(newMockConnectService(deps.MockRuntime, deps.Authenticator))
    r.Mount(path, handler)
}
```

- [ ] **Step 3: Wire MockRuntime in cmd/server/main.go**

In `cmd/server/main.go`, after the runtime creation (line 74) and before the worker setup, add:

```go
var mockRT *machine.MockRuntime
if os.Getenv("ARCA_ENABLE_MOCK") == "true" {
    mockRT = machine.NewMockRuntime(store)
    runtime.RegisterMockFactory(mockRT)
    slog.Info("mock runtime enabled")
}
```

Add `"os"` to imports if not present.

Pass `mockRT` to the `Dependencies` struct where `NewRouter` is called:

```go
MockRuntime: mockRT,
```

Add a shutdown defer after the `mockRT` creation:

```go
if mockRT != nil {
    defer mockRT.Shutdown(context.Background())
}
```

- [ ] **Step 4: Verify build**

Run: `go vet ./...`

Expected: PASS.

- [ ] **Step 5: Run backend tests**

Run: `make test/backend`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/router.go cmd/server/main.go
git commit -m "Wire MockRuntime into server with ARCA_ENABLE_MOCK gate"
```

---

## Task 8: Frontend Type Definitions and API Layer

**Files:**
- Modify: `web/src/lib/types.ts:57,59-82`
- Modify: `web/src/lib/api.ts:419-429,465-496,521-583`

- [ ] **Step 1: Add mock to MachineProfileType union**

In `web/src/lib/types.ts`, change line 57:

```typescript
export type MachineProfileType = 'libvirt' | 'gce' | 'lxd' | 'mock'
```

Add mock config variant to `MachineProfileConfig` union (after the lxd variant):

```typescript
  | {
      type: 'mock'
    }
```

- [ ] **Step 2: Update profileTypeToProto and profileTypeFromProto**

In `web/src/lib/api.ts`, update `profileTypeToProto` (~line 419):

```typescript
function profileTypeToProto(type: MachineProfileTypeLocal): MachineProfileType {
  if (type === 'gce') return MachineProfileType.GCE
  if (type === 'lxd') return MachineProfileType.LXD
  if (type === 'mock') return MachineProfileType.MOCK
  return MachineProfileType.LIBVIRT
}
```

Update `profileTypeFromProto` (~line 425):

```typescript
function profileTypeFromProto(type: MachineProfileType): MachineProfileTypeLocal {
  if (type === MachineProfileType.GCE) return 'gce'
  if (type === MachineProfileType.LXD) return 'lxd'
  if (type === MachineProfileType.MOCK) return 'mock'
  return 'libvirt'
}
```

- [ ] **Step 3: Update toMachineProfileItem for mock config**

In `web/src/lib/api.ts`, in the `toMachineProfileItem` function:

1. In the `input` type parameter's `provider` union, add `| { case: 'mock'; value: Record<string, never> }` alongside the existing libvirt/gce/lxd cases.
2. Add a mock branch before the libvirt fallback (after the lxd block):

```typescript
  } else if (profileType === 'mock') {
    config = { type: 'mock' }
```

- [ ] **Step 4: Update profileConfigPayload for mock**

In `web/src/lib/api.ts`, in the `profileConfigPayload` function, add a mock branch (after the lxd block):

```typescript
  } else if (type === 'mock') {
    provider = {
      case: 'mock' as const,
      value: {},
    }
```

- [ ] **Step 5: Verify frontend build**

Run: `cd web && npx tsc --noEmit`

Expected: PASS (no type errors).

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/types.ts web/src/lib/api.ts
git commit -m "Add mock provider type to frontend type definitions and API layer"
```

---

## Task 9: Profile Form UI

**Files:**
- Modify: `web/src/pages/MachineProfileFormPage.tsx:84-139` (validation), `141-173` (toConfig), `182-229` (fillFormFromProfile), `435-467` (provider select), `502-544` (startup script section), `546-600+` (infrastructure section)

- [ ] **Step 1: Update validateProfileForm for mock**

In `web/src/pages/MachineProfileFormPage.tsx`, in the `validateProfileForm` function (~line 84), add an early return for mock type before the libvirt fallback validation (which would reject mock for missing URI/network/storage_pool):

```typescript
  if (form.type === 'mock') return null
```

- [ ] **Step 2: Update toConfig for mock**

In `web/src/pages/MachineProfileFormPage.tsx`, add a mock branch in `toConfig()` (before the libvirt fallback return):

```typescript
  if (form.type === 'mock') {
    return { type: 'mock' }
  }
```

- [ ] **Step 3: Update fillFormFromProfile for mock**

In `fillFormFromProfile()`, add a mock branch (before the libvirt fallback):

```typescript
  if (cfg.type === 'mock') {
    return {
      ...emptyProfileForm(),
      id: profile.id,
      name: profile.name,
      type: 'mock',
      ...exposureFields,
    }
  }
```

- [ ] **Step 4: Add Mock option to provider type select**

In the `<select>` element (~line 458-462), add the Mock option. The option should only appear when `ARCA_ENABLE_MOCK` is active. For simplicity, add it unconditionally in the select and gate visibility via a server-provided flag later (or add it now — mock profiles are harmless if the backend factory isn't registered):

```tsx
<option value="mock">Mock</option>
```

- [ ] **Step 5: Update provider type display for edit mode**

In the edit-mode `<Input>` value (~line 440), add mock:

```typescript
value={form.type === 'gce' ? 'Google Compute Engine (GCE)' : form.type === 'lxd' ? 'LXD' : form.type === 'mock' ? 'Mock' : 'Libvirt'}
```

- [ ] **Step 6: Update type inference for select onChange**

In the `onChange` handler (~line 450), update the type inference:

```typescript
const t: MachineProfileType = val === 'gce' ? 'gce' : val === 'lxd' ? 'lxd' : val === 'mock' ? 'mock' : 'libvirt'
```

- [ ] **Step 7: Handle mock in startup script section**

Mock profiles have no startup script. In the "Applies on next start" section (~line 502-544), conditionally hide the startup script textarea for mock type. Wrap the existing conditional block:

```tsx
{form.type === 'mock' ? (
  <p className="text-sm text-muted-foreground">Mock profiles do not require a startup script.</p>
) : form.type === 'gce' ? (
  // ... existing GCE textarea
```

- [ ] **Step 8: Handle mock in infrastructure section**

In the "New machines only" infrastructure section (~line 546+), add a mock branch that shows no infrastructure fields:

```tsx
{form.type === 'mock' ? (
  <p className="text-sm text-muted-foreground">Mock profiles do not require infrastructure configuration.</p>
) : form.type === 'gce' ? (
  // ... existing GCE fields
```

- [ ] **Step 9: Verify frontend build**

Run: `cd web && npx tsc --noEmit`

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add web/src/pages/MachineProfileFormPage.tsx
git commit -m "Add Mock provider type to profile form UI"
```

---

## Task 10: Playwright Config and E2E Test Helpers

**Files:**
- Modify: `web/playwright.config.ts:28-41`
- Modify: `web/e2e/helpers/machine-profile.ts:27-31`
- Create: `web/e2e/helpers/mock.ts`

- [ ] **Step 1: Add ARCA_ENABLE_MOCK to playwright webServer env**

In `web/playwright.config.ts`, add to the `env` map inside `webServer` (after `ARCA_SKIP_SETUP`):

```typescript
ARCA_ENABLE_MOCK: 'true',
```

- [ ] **Step 2: Add mock to typeMap in machine-profile helper**

In `web/e2e/helpers/machine-profile.ts`, update the `typeMap` (~line 27-31):

```typescript
const typeMap: Record<string, number> = {
  libvirt: 1,
  gce: 2,
  lxd: 3,
  mock: 4,
}
```

- [ ] **Step 3: Add ensureMockProfile helper**

In `web/e2e/helpers/machine-profile.ts`, add an `ensureMockProfile` function (modeled on `ensureLxdProfile` — try-create, catch `already_exists`, fall back to list):

```typescript
export async function ensureMockProfile(page: Page): Promise<ProfileRecord> {
  const serverPort = new URL(
    process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
  ).port
  try {
    return await createMachineProfileViaAPI(page, {
      name: 'mock-e2e',
      type: 'mock',
      config: {
        mock: {},
        exposure: {
          method: 2, // PROXY_VIA_SERVER
          connectivity: 1, // PRIVATE_IP
        },
        serverApiUrl: `http://127.0.0.1:${serverPort}`,
      },
    })
  } catch (error) {
    if (String(error).includes('already_exists')) {
      const listResp = await page.request.post('/arca.v1.MachineProfileService/ListMachineProfiles', {
        data: {},
      })
      const listPayload = (await listResp.json()) as {
        profiles?: Array<{ id?: string; name?: string; type?: string }>
      }
      const existing = listPayload.profiles?.find((r) => r.name === 'mock-e2e')
      if (existing) {
        return {
          id: existing.id ?? '',
          name: existing.name ?? '',
          type: existing.type ?? '',
        }
      }
    }
    throw error
  }
}
```

- [ ] **Step 4: Create mock.ts E2E helper**

Create `web/e2e/helpers/mock.ts`:

```typescript
import type { Page } from '@playwright/test'

const MOCK_SERVICE_BASE = '/arca.v1.MockService'

async function callMockService(page: Page, method: string, data: Record<string, unknown> = {}) {
  const response = await page.request.post(`${MOCK_SERVICE_BASE}/${method}`, { data })
  if (!response.ok()) {
    const body = await response.text()
    throw new Error(`MockService.${method} failed: ${response.status()} ${body}`)
  }
  return response.json()
}

export async function setDefaultBehavior(page: Page, behavior: { delayMs?: number; errorOn?: Record<string, string> }) {
  await callMockService(page, 'SetDefaultBehavior', { behavior })
}

export async function setMachineBehavior(page: Page, machineId: string, behavior: { delayMs?: number; errorOn?: Record<string, string> }) {
  await callMockService(page, 'SetMachineBehavior', { machineId, behavior })
}

export async function resetBehavior(page: Page) {
  await callMockService(page, 'ResetBehavior', {})
}
```

- [ ] **Step 5: Commit**

```bash
git add web/playwright.config.ts web/e2e/helpers/machine-profile.ts web/e2e/helpers/mock.ts
git commit -m "Add E2E test helpers for mock runtime and enable in playwright config"
```

---

## Task 11: E2E Machine Lifecycle Tests

**Files:**
- Modify: `web/e2e/machines.spec.ts`

- [ ] **Step 1: Add mock lifecycle tests to machines.spec.ts**

Add a new `test.describe` block for mock runtime lifecycle tests:

```typescript
import { test, expect } from '@playwright/test'
import { ensureMockProfile } from './helpers/machine-profile'
import { createMachineViaAPI, waitForMachineStatus, bestEffortStopMachine, bestEffortDeleteMachine } from './helpers/machine'
import { resetBehavior, setDefaultBehavior } from './helpers/mock'

test.describe('mock runtime lifecycle', () => {
  let profileId: string

  test.beforeEach(async ({ page }) => {
    const profile = await ensureMockProfile(page)
    profileId = profile.id
  })

  test.afterEach(async ({ page }) => {
    await resetBehavior(page)
  })

  test('create machine and wait for running', async ({ page }) => {
    const machineId = await createMachineViaAPI(page, `mock-test-${Date.now()}`, profileId)
    await waitForMachineStatus(page, machineId, ['running'])

    // Verify UI shows running status
    await page.goto('/machines')
    await expect(page.locator('text=running').first()).toBeVisible()
  })

  test('stop and restart machine', async ({ page }) => {
    const machineId = await createMachineViaAPI(page, `mock-stop-${Date.now()}`, profileId)
    await waitForMachineStatus(page, machineId, ['running'])

    // Stop via API (StopMachine RPC)
    await page.request.post('/arca.v1.MachineService/StopMachine', {
      data: { machineId },
    })
    await waitForMachineStatus(page, machineId, ['stopped'])

    // Start again via API (StartMachine RPC)
    await page.request.post('/arca.v1.MachineService/StartMachine', {
      data: { machineId },
    })
    await waitForMachineStatus(page, machineId, ['running'])
  })

  test('delete machine', async ({ page }) => {
    const machineId = await createMachineViaAPI(page, `mock-del-${Date.now()}`, profileId)
    await waitForMachineStatus(page, machineId, ['running'])

    await bestEffortDeleteMachine(page, machineId)
  })

  test('shows starting state with delay', async ({ page }) => {
    await setDefaultBehavior(page, { delayMs: 3000 })
    const machineId = await createMachineViaAPI(page, `mock-slow-${Date.now()}`, profileId)

    // Machine should be in starting state
    await waitForMachineStatus(page, machineId, ['starting', 'running'])
  })
})
```

- [ ] **Step 2: Run the tests**

Run: `cd web && npx playwright test --project=fast e2e/machines.spec.ts`

Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add web/e2e/machines.spec.ts
git commit -m "Add mock runtime lifecycle E2E tests"
```

---

## Task 12: E2E Error Injection Tests

**Files:**
- Create: `web/e2e/machine-errors.spec.ts`

- [ ] **Step 1: Create error injection test file**

Create `web/e2e/machine-errors.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { ensureMockProfile } from './helpers/machine-profile'
import { createMachineViaAPI, waitForMachineStatus } from './helpers/machine'
import { setMachineBehavior, resetBehavior } from './helpers/mock'

test.describe('mock runtime error injection', () => {
  let profileId: string

  test.beforeEach(async ({ page }) => {
    profileId = await ensureMockProfile(page)
  })

  test.afterEach(async ({ page }) => {
    await resetBehavior(page)
  })

  test('machine fails to start with injected error', async ({ page }) => {
    const machineId = await createMachineViaAPI(page, `mock-err-${Date.now()}`, profileId)

    // Inject error before the machine worker processes the start job
    await setMachineBehavior(page, machineId, {
      errorOn: { EnsureRunning: 'simulated disk full' },
    })

    // Wait for the machine to reach failed state (after retries exhaust)
    await waitForMachineStatus(page, machineId, ['failed'], { timeout: 60_000 })
  })
})
```

- [ ] **Step 2: Run the tests**

Run: `cd web && npx playwright test --project=fast e2e/machine-errors.spec.ts`

Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add web/e2e/machine-errors.spec.ts
git commit -m "Add E2E error injection tests for mock runtime"
```

---

## Task 13: E2E Proxy Connection Tests

**Files:**
- Create: `web/e2e/machine-proxy.spec.ts`

- [ ] **Step 1: Create proxy connection test file**

Create `web/e2e/machine-proxy.spec.ts`:

```typescript
import { test, expect } from '@playwright/test'
import { ensureMockProfile } from './helpers/machine-profile'
import { createMachineViaAPI, waitForMachineStatus } from './helpers/machine'
import { resetBehavior } from './helpers/mock'

test.describe('mock runtime proxy connection', () => {
  let profileId: string

  test.beforeEach(async ({ page }) => {
    profileId = await ensureMockProfile(page)
  })

  test.afterEach(async ({ page }) => {
    await resetBehavior(page)
  })

  test('proxy routes to stub server', async ({ page }) => {
    const machineId = await createMachineViaAPI(page, `mock-proxy-${Date.now()}`, profileId)
    await waitForMachineStatus(page, machineId, ['running'])

    // Get machine endpoint/hostname to build proxy URL
    // The proxy routes based on Host header matching the machine's exposure hostname
    // The stub server responds with JSON containing the machine ID
    // This test verifies the full proxy → stub server path

    // Fetch through the proxy using the machine's exposure URL
    // Implementation details depend on how the exposure hostname is configured.
    // The test should verify that a request through the proxy returns
    // the stub server's response containing the machine_id.
  })
})
```

Note: The exact proxy test implementation depends on how machine exposure hostnames are resolved. During implementation, check `web/e2e/lxd-provisioning.spec.ts` for the existing proxy test pattern and adapt it for mock machines.

- [ ] **Step 2: Run the tests**

Run: `cd web && npx playwright test --project=fast e2e/machine-proxy.spec.ts`

Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add web/e2e/machine-proxy.spec.ts
git commit -m "Add E2E proxy connection tests for mock runtime"
```

---

## Task 14: Full Test Suite Verification

- [ ] **Step 1: Run full backend tests**

Run: `make test/backend`

Expected: All tests PASS.

- [ ] **Step 2: Run fast E2E tests**

Run: `make test/e2e`

Expected: All tests PASS.

- [ ] **Step 3: Commit any remaining fixes**

If any tests required adjustments, commit those fixes.

- [ ] **Step 4: Final commit message review**

Review the commit history to ensure all commits are focused and well-described.

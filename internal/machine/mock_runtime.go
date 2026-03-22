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
	ErrorOn map[string]string // operation name -> error message
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
	b3 := byte(n)
	b2 := byte(n >> 8)
	b1 := byte(n >> 16)
	if b3 == 0 {
		b3 = 1
	}
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

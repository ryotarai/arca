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

	running, _, _ := rt.IsRunning(context.Background(), testMachine("m1"))
	if running {
		t.Error("machine still running after reset")
	}
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

package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ryotarai/arca/internal/workflow"
)

// memStore is an in-memory Store for tests.
type memStore struct {
	data   map[string][]byte
	saves  []map[string]string // captures each save for assertions
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string][]byte)}
}

func (s *memStore) LoadWorkflowState(_ context.Context, id string) ([]byte, error) {
	return s.data[id], nil
}

func (s *memStore) SaveWorkflowState(_ context.Context, id string, data []byte) error {
	s.data[id] = append([]byte(nil), data...)
	var m map[string]string
	if err := json.Unmarshal(data, &m); err == nil {
		s.saves = append(s.saves, m)
	}
	return nil
}

func (s *memStore) DeleteWorkflowState(_ context.Context, id string) error {
	delete(s.data, id)
	return nil
}

func savedSteps(saves []map[string]string) []string {
	out := make([]string, len(saves))
	for i, s := range saves {
		out[i] = s["step"]
	}
	return out
}

func TestRunAllSteps(t *testing.T) {
	store := newMemStore()
	var executed []string

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("a", func(ctx context.Context) error {
			executed = append(executed, "a")
			return nil
		}).
		Step("b", func(ctx context.Context) error {
			executed = append(executed, "b")
			return nil
		}).
		Step("c", func(ctx context.Context) error {
			executed = append(executed, "c")
			return nil
		}).
		Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "a,b,c" {
		t.Errorf("executed = %q, want a,b,c", got)
	}
	if got := strings.Join(savedSteps(store.saves), ","); got != "b,c" {
		t.Errorf("checkpointed steps = %q, want b,c", got)
	}
	// State should be deleted after successful completion.
	if store.data["test-1"] != nil {
		t.Error("expected workflow state to be deleted after success")
	}
}

func TestResumeFromStep(t *testing.T) {
	store := newMemStore()
	// Simulate a previous checkpoint.
	store.data["test-1"] = []byte(`{"step":"b"}`)
	var executed []string

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("a", func(ctx context.Context) error {
			executed = append(executed, "a")
			return nil
		}).
		Step("b", func(ctx context.Context) error {
			executed = append(executed, "b")
			return nil
		}).
		Step("c", func(ctx context.Context) error {
			executed = append(executed, "c")
			return nil
		}).
		Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "b,c" {
		t.Errorf("executed = %q, want b,c", got)
	}
}

func TestResumeFromLastStep(t *testing.T) {
	store := newMemStore()
	store.data["test-1"] = []byte(`{"step":"b"}`)
	var executed []string

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("a", func(ctx context.Context) error {
			executed = append(executed, "a")
			return nil
		}).
		Step("b", func(ctx context.Context) error {
			executed = append(executed, "b")
			return nil
		}).
		Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "b" {
		t.Errorf("executed = %q, want b", got)
	}
}

func TestErrorStopsExecution(t *testing.T) {
	store := newMemStore()
	var executed []string
	testErr := errors.New("step b failed")

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("a", func(ctx context.Context) error {
			executed = append(executed, "a")
			return nil
		}).
		Step("b", func(ctx context.Context) error {
			executed = append(executed, "b")
			return testErr
		}).
		Step("c", func(ctx context.Context) error {
			executed = append(executed, "c")
			return nil
		}).
		Run(context.Background())

	if !errors.Is(err, testErr) {
		t.Errorf("expected testErr, got %v", err)
	}
	if got := strings.Join(executed, ","); got != "a,b" {
		t.Errorf("executed = %q, want a,b (c should not run)", got)
	}
	// State should be persisted (not deleted) on failure for retry.
	if store.data["test-1"] == nil {
		t.Error("expected workflow state to be persisted on failure")
	}
}

func TestTerminalError(t *testing.T) {
	store := newMemStore()

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("a", func(ctx context.Context) error {
			return workflow.Terminal(errors.New("permanent failure"))
		}).
		Step("b", func(ctx context.Context) error {
			t.Fatal("should not reach step b")
			return nil
		}).
		Run(context.Background())

	if err == nil {
		t.Fatal("expected error")
	}
	if !workflow.IsTerminal(err) {
		t.Errorf("expected terminal error, got %v", err)
	}
}

func TestTerminalErrorPreservesMessage(t *testing.T) {
	inner := errors.New("arcad 404")
	err := workflow.Terminal(inner)

	if err.Error() != "arcad 404" {
		t.Errorf("error message = %q, want %q", err.Error(), "arcad 404")
	}
	if !errors.Is(err, inner) {
		t.Error("expected errors.Is to find inner error")
	}
}

func TestUnknownStepIsTerminal(t *testing.T) {
	store := newMemStore()
	store.data["test-1"] = []byte(`{"step":"nonexistent"}`)

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("a", func(ctx context.Context) error { return nil }).
		Run(context.Background())

	if err == nil {
		t.Fatal("expected error for unknown step")
	}
	if !workflow.IsTerminal(err) {
		t.Errorf("expected terminal error for unknown step, got %v", err)
	}
}

func TestStepWithTimeout(t *testing.T) {
	store := newMemStore()

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		StepWithTimeout("slow", 10*time.Millisecond, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}).
		Run(context.Background())

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestNoCheckpointForSingleStep(t *testing.T) {
	store := newMemStore()

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("only", func(ctx context.Context) error { return nil }).
		Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.saves) != 0 {
		t.Errorf("expected no checkpoints for single step, got %v", store.saves)
	}
}

func TestEmptyWorkflow(t *testing.T) {
	store := newMemStore()
	runner := workflow.NewRunner(store, "test-1")
	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckpointNotCalledOnError(t *testing.T) {
	store := newMemStore()

	runner := workflow.NewRunner(store, "test-1")
	runner.
		Step("a", func(ctx context.Context) error { return nil }).
		Step("b", func(ctx context.Context) error { return errors.New("fail") }).
		Step("c", func(ctx context.Context) error { return nil }).
		Run(context.Background())

	if got := strings.Join(savedSteps(store.saves), ","); got != "b" {
		t.Errorf("checkpointed steps = %q, want b", got)
	}
}

func TestParentContextCancellation(t *testing.T) {
	store := newMemStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("a", func(ctx context.Context) error {
			return ctx.Err()
		}).
		Run(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestStatePersistedAcrossCheckpoints(t *testing.T) {
	store := newMemStore()

	runner := workflow.NewRunner(store, "test-1")
	runner.Set("initial", "value")
	err := runner.
		Step("a", func(ctx context.Context) error {
			runner.Set("from_a", "hello")
			return nil
		}).
		Step("b", func(ctx context.Context) error {
			if got := runner.Get("from_a"); got != "hello" {
				t.Errorf("Get(from_a) = %q, want hello", got)
			}
			if got := runner.Get("initial"); got != "value" {
				t.Errorf("Get(initial) = %q, want value", got)
			}
			return nil
		}).
		Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.saves) < 1 {
		t.Fatal("expected at least one save")
	}
	saved := store.saves[0]
	if saved["step"] != "b" {
		t.Errorf("saved step = %q, want b", saved["step"])
	}
	if saved["from_a"] != "hello" {
		t.Errorf("saved from_a = %q, want hello", saved["from_a"])
	}
}

func TestStateLoadedFromStore(t *testing.T) {
	store := newMemStore()
	store.data["test-1"] = []byte(`{"step":"b","image_name":"my-image","image_data":"snapshot-123"}`)

	runner := workflow.NewRunner(store, "test-1")
	err := runner.
		Step("a", func(ctx context.Context) error { return nil }).
		Step("b", func(ctx context.Context) error {
			if got := runner.Get("image_name"); got != "my-image" {
				t.Errorf("Get(image_name) = %q, want my-image", got)
			}
			if got := runner.Get("image_data"); got != "snapshot-123" {
				t.Errorf("Get(image_data) = %q, want snapshot-123", got)
			}
			return nil
		}).
		Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFirstRunWithSetDefaults(t *testing.T) {
	store := newMemStore()
	var executed []string

	runner := workflow.NewRunner(store, "test-1")
	runner.Set("image_name", "foo")
	err := runner.
		Step("a", func(ctx context.Context) error {
			if got := runner.Get("image_name"); got != "foo" {
				t.Errorf("Get(image_name) = %q, want foo", got)
			}
			executed = append(executed, "a")
			return nil
		}).
		Step("b", func(ctx context.Context) error {
			executed = append(executed, "b")
			return nil
		}).
		Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "a,b" {
		t.Errorf("executed = %q, want a,b", got)
	}
}

func TestStoreOverridesDefaults(t *testing.T) {
	store := newMemStore()
	// Previous checkpoint has different value.
	store.data["test-1"] = []byte(`{"step":"b","image_name":"from-db"}`)

	runner := workflow.NewRunner(store, "test-1")
	runner.Set("image_name", "from-caller") // should be overwritten by DB

	err := runner.
		Step("a", func(ctx context.Context) error { return nil }).
		Step("b", func(ctx context.Context) error {
			if got := runner.Get("image_name"); got != "from-db" {
				t.Errorf("Get(image_name) = %q, want from-db (DB should override)", got)
			}
			return nil
		}).
		Run(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

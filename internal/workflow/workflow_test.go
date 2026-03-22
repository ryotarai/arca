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

// captureStore records all saved states for test assertions.
type captureStore struct {
	saves []map[string]string
}

func (s *captureStore) Save(_ context.Context, data []byte) error {
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	s.saves = append(s.saves, m)
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
	store := &captureStore{}
	var executed []string

	runner := workflow.New(store)
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
		Run(context.Background(), nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "a,b,c" {
		t.Errorf("executed = %q, want a,b,c", got)
	}
	if got := strings.Join(savedSteps(store.saves), ","); got != "b,c" {
		t.Errorf("checkpointed steps = %q, want b,c", got)
	}
}

func TestResumeFromStep(t *testing.T) {
	store := &captureStore{}
	var executed []string

	runner := workflow.New(store)
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
		Run(context.Background(), []byte(`{"step":"b"}`))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "b,c" {
		t.Errorf("executed = %q, want b,c", got)
	}
}

func TestResumeFromLastStep(t *testing.T) {
	store := &captureStore{}
	var executed []string

	runner := workflow.New(store)
	err := runner.
		Step("a", func(ctx context.Context) error {
			executed = append(executed, "a")
			return nil
		}).
		Step("b", func(ctx context.Context) error {
			executed = append(executed, "b")
			return nil
		}).
		Run(context.Background(), []byte(`{"step":"b"}`))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "b" {
		t.Errorf("executed = %q, want b", got)
	}
	if len(store.saves) != 0 {
		t.Errorf("expected no saves after last step, got %d", len(store.saves))
	}
}

func TestErrorStopsExecution(t *testing.T) {
	store := &captureStore{}
	var executed []string
	testErr := errors.New("step b failed")

	runner := workflow.New(store)
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
		Run(context.Background(), nil)

	if !errors.Is(err, testErr) {
		t.Errorf("expected testErr, got %v", err)
	}
	if got := strings.Join(executed, ","); got != "a,b" {
		t.Errorf("executed = %q, want a,b (c should not run)", got)
	}
}

func TestTerminalError(t *testing.T) {
	store := &captureStore{}

	runner := workflow.New(store)
	err := runner.
		Step("a", func(ctx context.Context) error {
			return workflow.Terminal(errors.New("permanent failure"))
		}).
		Step("b", func(ctx context.Context) error {
			t.Fatal("should not reach step b")
			return nil
		}).
		Run(context.Background(), nil)

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
	store := &captureStore{}

	runner := workflow.New(store)
	err := runner.
		Step("a", func(ctx context.Context) error { return nil }).
		Run(context.Background(), []byte(`{"step":"nonexistent"}`))

	if err == nil {
		t.Fatal("expected error for unknown step")
	}
	if !workflow.IsTerminal(err) {
		t.Errorf("expected terminal error for unknown step, got %v", err)
	}
}

func TestStepWithTimeout(t *testing.T) {
	store := &captureStore{}

	runner := workflow.New(store)
	err := runner.
		StepWithTimeout("slow", 10*time.Millisecond, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}).
		Run(context.Background(), nil)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestNoCheckpointForSingleStep(t *testing.T) {
	store := &captureStore{}

	runner := workflow.New(store)
	err := runner.
		Step("only", func(ctx context.Context) error { return nil }).
		Run(context.Background(), nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.saves) != 0 {
		t.Errorf("expected no checkpoints for single step, got %v", store.saves)
	}
}

func TestEmptyWorkflow(t *testing.T) {
	store := &captureStore{}
	runner := workflow.New(store)
	err := runner.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckpointNotCalledOnError(t *testing.T) {
	store := &captureStore{}

	runner := workflow.New(store)
	runner.
		Step("a", func(ctx context.Context) error { return nil }).
		Step("b", func(ctx context.Context) error { return errors.New("fail") }).
		Step("c", func(ctx context.Context) error { return nil }).
		Run(context.Background(), nil)

	if got := strings.Join(savedSteps(store.saves), ","); got != "b" {
		t.Errorf("checkpointed steps = %q, want b", got)
	}
}

func TestParentContextCancellation(t *testing.T) {
	store := &captureStore{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := workflow.New(store)
	err := runner.
		Step("a", func(ctx context.Context) error {
			return ctx.Err()
		}).
		Run(ctx, nil)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestStatePersistedAcrossCheckpoints(t *testing.T) {
	store := &captureStore{}

	runner := workflow.New(store)
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
		Run(context.Background(), nil)

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
	if saved["initial"] != "value" {
		t.Errorf("saved initial = %q, want value", saved["initial"])
	}
}

func TestStateLoadedFromSavedState(t *testing.T) {
	store := &captureStore{}

	runner := workflow.New(store)
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
		Run(context.Background(), []byte(`{"step":"b","image_name":"my-image","image_data":"snapshot-123"}`))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitialStateWithoutStep(t *testing.T) {
	store := &captureStore{}
	var executed []string

	runner := workflow.New(store)
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
		Run(context.Background(), []byte(`{"image_name":"foo"}`))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "a,b" {
		t.Errorf("executed = %q, want a,b", got)
	}
}

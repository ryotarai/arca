package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ryotarai/arca/internal/workflow"
)

func TestRunAllSteps(t *testing.T) {
	var executed []string
	var checkpoints []string

	err := workflow.New(func(next string) {
		checkpoints = append(checkpoints, next)
	}).
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
		Run(context.Background(), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "a,b,c" {
		t.Errorf("executed = %q, want a,b,c", got)
	}
	// Checkpoints are emitted for the next step after each completes,
	// except after the last step.
	if got := strings.Join(checkpoints, ","); got != "b,c" {
		t.Errorf("checkpoints = %q, want b,c", got)
	}
}

func TestResumeFromStep(t *testing.T) {
	var executed []string

	err := workflow.New(nil).
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
		Run(context.Background(), "b")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "b,c" {
		t.Errorf("executed = %q, want b,c", got)
	}
}

func TestResumeFromLastStep(t *testing.T) {
	var executed []string

	err := workflow.New(nil).
		Step("a", func(ctx context.Context) error {
			executed = append(executed, "a")
			return nil
		}).
		Step("b", func(ctx context.Context) error {
			executed = append(executed, "b")
			return nil
		}).
		Run(context.Background(), "b")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(executed, ","); got != "b" {
		t.Errorf("executed = %q, want b", got)
	}
}

func TestErrorStopsExecution(t *testing.T) {
	var executed []string
	testErr := errors.New("step b failed")

	err := workflow.New(nil).
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
		Run(context.Background(), "")

	if !errors.Is(err, testErr) {
		t.Errorf("expected testErr, got %v", err)
	}
	if got := strings.Join(executed, ","); got != "a,b" {
		t.Errorf("executed = %q, want a,b (c should not run)", got)
	}
}

func TestTerminalError(t *testing.T) {
	err := workflow.New(nil).
		Step("a", func(ctx context.Context) error {
			return workflow.Terminal(errors.New("permanent failure"))
		}).
		Step("b", func(ctx context.Context) error {
			t.Fatal("should not reach step b")
			return nil
		}).
		Run(context.Background(), "")

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
	err := workflow.New(nil).
		Step("a", func(ctx context.Context) error { return nil }).
		Run(context.Background(), "nonexistent")

	if err == nil {
		t.Fatal("expected error for unknown step")
	}
	if !workflow.IsTerminal(err) {
		t.Errorf("expected terminal error for unknown step, got %v", err)
	}
}

func TestStepWithTimeout(t *testing.T) {
	err := workflow.New(nil).
		StepWithTimeout("slow", 10*time.Millisecond, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}).
		Run(context.Background(), "")

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestNoCheckpointForSingleStep(t *testing.T) {
	var checkpoints []string

	err := workflow.New(func(next string) {
		checkpoints = append(checkpoints, next)
	}).
		Step("only", func(ctx context.Context) error { return nil }).
		Run(context.Background(), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checkpoints) != 0 {
		t.Errorf("expected no checkpoints for single step, got %v", checkpoints)
	}
}

func TestEmptyWorkflow(t *testing.T) {
	err := workflow.New(nil).Run(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckpointNotCalledOnError(t *testing.T) {
	var checkpoints []string

	workflow.New(func(next string) {
		checkpoints = append(checkpoints, next)
	}).
		Step("a", func(ctx context.Context) error { return nil }).
		Step("b", func(ctx context.Context) error { return errors.New("fail") }).
		Step("c", func(ctx context.Context) error { return nil }).
		Run(context.Background(), "")

	// Only checkpoint after "a" (pointing to "b"). No checkpoint after
	// "b" because it failed.
	if got := strings.Join(checkpoints, ","); got != "b" {
		t.Errorf("checkpoints = %q, want b", got)
	}
}

func TestParentContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := workflow.New(nil).
		Step("a", func(ctx context.Context) error {
			return ctx.Err()
		}).
		Run(ctx, "")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

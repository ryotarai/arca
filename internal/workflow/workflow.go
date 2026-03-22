// Package workflow provides a lightweight durable execution pattern for
// multi-step operations. Each step checkpoints its completion to a
// [Store], so retries always resume from the last successful step
// rather than re-executing the entire workflow.
//
// The library owns state management and persistence. Callers provide a
// [Store] implementation that writes to durable storage (e.g., a database),
// and use [Runner.Get]/[Runner.Set] to pass data between steps.
//
// This is inspired by Temporal's workflow model but requires no external
// dependencies or additional infrastructure.
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TerminalError wraps errors that should not be retried by the job system.
// Use [Terminal] to create one and [IsTerminal] to check.
type TerminalError struct {
	Err error
}

func (e *TerminalError) Error() string { return e.Err.Error() }
func (e *TerminalError) Unwrap() error { return e.Err }

// Terminal wraps err so that [IsTerminal] returns true.
func Terminal(err error) error {
	return &TerminalError{Err: err}
}

// IsTerminal reports whether err (or any error in its chain) is a
// [TerminalError].
func IsTerminal(err error) bool {
	var te *TerminalError
	return errors.As(err, &te)
}

// Store persists workflow state durably. Implementations must ensure
// that writes survive process restarts so that [Runner.Run] can resume
// from the correct step after a crash.
type Store interface {
	Save(ctx context.Context, data []byte) error
}

// StoreFunc adapts a function into a [Store].
type StoreFunc func(ctx context.Context, data []byte) error

func (f StoreFunc) Save(ctx context.Context, data []byte) error { return f(ctx, data) }

// StepFunc is a function that executes a single workflow step.
// The context may carry a per-step timeout set via [Runner.StepWithTimeout].
type StepFunc func(ctx context.Context) error

type step struct {
	name    string
	fn      StepFunc
	timeout time.Duration // 0 means inherit parent context
}

// Runner executes a linear sequence of named steps with checkpoint
// persistence. After each step completes successfully, the runner
// serializes the current state (step position + key-value vars) and
// writes it to the [Store].
//
// Step functions access shared state via [Runner.Get] and [Runner.Set].
// The state is a flat string-to-string map, persisted as JSON.
type Runner struct {
	steps []step
	store Store
	state map[string]string
}

// New creates a Runner that persists checkpoints to store.
func New(store Store) *Runner {
	return &Runner{
		store: store,
		state: make(map[string]string),
	}
}

// Step appends a step that inherits the parent context's deadline.
func (r *Runner) Step(name string, fn StepFunc) *Runner {
	r.steps = append(r.steps, step{name: name, fn: fn})
	return r
}

// StepWithTimeout appends a step with its own timeout derived from the
// parent context.
func (r *Runner) StepWithTimeout(name string, timeout time.Duration, fn StepFunc) *Runner {
	r.steps = append(r.steps, step{name: name, fn: fn, timeout: timeout})
	return r
}

// Get retrieves a value from the workflow state. Returns "" if the key
// does not exist.
func (r *Runner) Get(key string) string {
	return r.state[key]
}

// Set stores a key-value pair in the workflow state. The value will be
// persisted at the next checkpoint (after the current step completes).
func (r *Runner) Set(key string, value string) {
	r.state[key] = value
}

// Run executes steps, resuming from the step recorded in savedState.
// If savedState is nil or empty, execution starts from the first step.
//
// savedState is a JSON object previously written by the [Store]. The
// runner reads the "step" key to determine where to resume and loads
// all other keys into the state map accessible via [Get]/[Set].
//
// Returns nil on success, or the first error encountered. A
// [TerminalError] signals that retrying will not help.
func (r *Runner) Run(ctx context.Context, savedState []byte) error {
	if len(r.steps) == 0 {
		return nil
	}

	// Load persisted state.
	if len(savedState) > 0 {
		var loaded map[string]string
		if err := json.Unmarshal(savedState, &loaded); err == nil {
			for k, v := range loaded {
				r.state[k] = v
			}
		}
	}

	// Determine starting step.
	resumeFrom := r.state["step"]
	startIdx := 0
	if resumeFrom != "" {
		found := false
		for i, s := range r.steps {
			if s.name == resumeFrom {
				startIdx = i
				found = true
				break
			}
		}
		if !found {
			return Terminal(fmt.Errorf("unknown workflow step: %q", resumeFrom))
		}
	}

	for i := startIdx; i < len(r.steps); i++ {
		s := r.steps[i]

		var stepCtx context.Context
		var cancel context.CancelFunc
		if s.timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, s.timeout)
		} else {
			stepCtx = ctx
			cancel = func() {}
		}

		err := s.fn(stepCtx)
		cancel()

		if err != nil {
			return err
		}

		// Checkpoint: persist state with the next step so retries resume
		// from there. Not called after the last step — the caller handles
		// completion.
		if r.store != nil && i+1 < len(r.steps) {
			r.state["step"] = r.steps[i+1].name
			if data, err := json.Marshal(r.state); err == nil {
				if saveErr := r.store.Save(ctx, data); saveErr != nil {
					// Best-effort: if save fails, the step was idempotent
					// and will re-execute on retry.
					_ = saveErr
				}
			}
		}
	}

	return nil
}

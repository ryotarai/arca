// Package workflow provides a lightweight durable execution pattern for
// multi-step operations. Each step checkpoints its completion to a caller-
// provided persistence callback, so retries always resume from the last
// successful step rather than re-executing the entire workflow.
//
// This is inspired by Temporal's workflow model but requires no external
// dependencies or additional infrastructure.
package workflow

import (
	"context"
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

// StepFunc is a function that executes a single workflow step.
// The context may carry a per-step timeout set via [Runner.StepWithTimeout].
type StepFunc func(ctx context.Context) error

type step struct {
	name    string
	fn      StepFunc
	timeout time.Duration // 0 means inherit parent context
}

// Runner executes a linear sequence of named steps with checkpoint
// persistence. After each step completes successfully, the onCheckpoint
// callback is invoked with the name of the next step. The caller should
// persist this value so that [Runner.Run] can resume from the right point.
type Runner struct {
	steps        []step
	onCheckpoint func(nextStep string)
}

// New creates a Runner. onCheckpoint is called after each step (except the
// last) with the name of the next step to execute. Pass nil if persistence
// is not needed (e.g., in tests).
func New(onCheckpoint func(nextStep string)) *Runner {
	return &Runner{onCheckpoint: onCheckpoint}
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

// Run executes steps starting from resumeFrom. If resumeFrom is empty,
// execution starts from the first step. Returns nil on success, or the
// first error encountered. A [TerminalError] signals that retrying will
// not help.
func (r *Runner) Run(ctx context.Context, resumeFrom string) error {
	if len(r.steps) == 0 {
		return nil
	}

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

		// Checkpoint: persist the next step so retries resume from there.
		// Not called after the last step — the caller handles completion.
		if r.onCheckpoint != nil && i+1 < len(r.steps) {
			r.onCheckpoint(r.steps[i+1].name)
		}
	}

	return nil
}

// Package testutil provides helpers for testing workflow activities in isolation,
// without requiring a running Engine, PocketBase, or any external dependencies.
package testutil

import (
	"context"
	"fmt"
	"time"

	"cosmicbizwitch/pkg/workflow"
)

// Harness runs a single ActivityFunc in isolation for unit testing.
// Build one with New, optionally configure it, then call Run or RunWithTimeout.
//
// Example:
//
//	fn := NewMyActivity(mockApp)
//	out, err := testutil.New(fn).
//	    WithInput("some_key", "some_value").
//	    Run(t)
//	require.NoError(t, err)
//	assert.NotEmpty(t, out["result"])
type Harness struct {
	fn    workflow.ActivityFunc
	input map[string]any
	ctx   context.Context
}

// New creates a Harness wrapping fn.
func New(fn workflow.ActivityFunc) *Harness {
	return &Harness{
		fn:    fn,
		input: make(map[string]any),
		ctx:   context.Background(),
	}
}

// WithInput sets one input key on the harness. Returns the harness for chaining.
func (h *Harness) WithInput(key string, value any) *Harness {
	h.input[key] = value
	return h
}

// WithInputs merges a map of inputs into the harness. Returns the harness for chaining.
func (h *Harness) WithInputs(inputs map[string]any) *Harness {
	for k, v := range inputs {
		h.input[k] = v
	}
	return h
}

// WithContext sets a custom context (e.g. with a workflow ID injected for worker-mode activities).
func (h *Harness) WithContext(ctx context.Context) *Harness {
	h.ctx = ctx
	return h
}

// WithWorkflowID injects a fake workflow ID into the context, needed for activities
// that call workflow.WorkflowIDFromContext.
func (h *Harness) WithWorkflowID(id string) *Harness {
	h.ctx = workflow.WithWorkflowID(h.ctx, id)
	return h
}

// Run executes the activity and returns its output and error.
func (h *Harness) Run() (map[string]any, error) {
	return h.fn(h.ctx, h.input)
}

// RunWithTimeout executes the activity with a deadline. Returns an error if the
// activity does not complete within d.
func (h *Harness) RunWithTimeout(d time.Duration) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(h.ctx, d)
	defer cancel()
	ch := make(chan struct {
		out map[string]any
		err error
	}, 1)
	go func() {
		out, err := h.fn(ctx, h.input)
		ch <- struct {
			out map[string]any
			err error
		}{out, err}
	}()
	select {
	case res := <-ch:
		return res.out, res.err
	case <-ctx.Done():
		return nil, fmt.Errorf("activity timed out after %s", d)
	}
}

// TB is a subset of *testing.T — declared as interface to avoid importing testing in production code.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
}

// MustRun runs the activity and calls t.Fatalf if it returns an error.
func (h *Harness) MustRun(t TB) map[string]any {
	t.Helper()
	out, err := h.fn(h.ctx, h.input)
	if err != nil {
		t.Fatalf("activity failed: %v", err)
	}
	return out
}

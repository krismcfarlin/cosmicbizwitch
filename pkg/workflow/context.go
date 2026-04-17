package workflow

import "context"

type contextKey int

const workflowIDKey contextKey = iota

// WithWorkflowID returns a new context carrying the workflow ID.
func WithWorkflowID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, workflowIDKey, id)
}

// WorkflowIDFromContext retrieves the workflow ID injected by the runner.
// Returns ("", false) if not present.
func WorkflowIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(workflowIDKey).(string)
	return v, ok
}

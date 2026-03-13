package workflow

import "errors"

var (
	ErrWorkflowNotFound    = errors.New("workflow not found")
	ErrActivityNotFound    = errors.New("activity not found")
	ErrGraphNotFound       = errors.New("graph not found")
	ErrActivityNotReg      = errors.New("activity not registered")
	ErrAlreadyRunning      = errors.New("workflow already running")
	ErrNotPaused           = errors.New("workflow is not paused")
	ErrNotCancellable      = errors.New("workflow is not in a cancellable state")
	ErrNotRestartable      = errors.New("workflow cannot be restarted in its current state")
	ErrGraphValidation     = errors.New("graph validation failed")
)

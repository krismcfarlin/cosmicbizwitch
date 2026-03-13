package workflow

import "context"

// Store is the persistence interface. The core package has zero external dependencies.
// Provide a concrete implementation (e.g., pbstore.PBStore) and pass it to NewEngine.
type Store interface {
	// Workflow CRUD
	CreateWorkflow(ctx context.Context, wf *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	UpdateWorkflow(ctx context.Context, wf *Workflow) error
	ListWorkflows(ctx context.Context, filter ListFilter) ([]*Workflow, error)

	// ActivityInstance CRUD
	CreateActivityInstance(ctx context.Context, inst *ActivityInstance) error
	GetActivityInstance(ctx context.Context, id string) (*ActivityInstance, error)
	UpdateActivityInstance(ctx context.Context, inst *ActivityInstance) error
	ListActivityInstances(ctx context.Context, workflowID string) ([]*ActivityInstance, error)

	// DeleteActivityInstancesForWorkflow removes all instances (used on restart).
	DeleteActivityInstancesForWorkflow(ctx context.Context, workflowID string) error

	// GetRunnableWorkflows returns workflows that are ready to execute:
	// status=pending AND (not_before IS NULL OR not_before <= now).
	GetRunnableWorkflows(ctx context.Context) ([]*Workflow, error)

	// GetPausedWorkflow returns the workflow only if status=paused.
	GetPausedWorkflow(ctx context.Context, id string) (*Workflow, error)

	// PersistGraph upserts a graph definition by name so it survives server restarts.
	PersistGraph(ctx context.Context, g *ActivityGraph) error

	// LoadGraphs returns all persisted graph definitions.
	LoadGraphs(ctx context.Context) ([]*ActivityGraph, error)

	// ResetStuckWorkflows resets workflows in running or paused status back to
	// pending so the poller picks them up after a restart.
	ResetStuckWorkflows(ctx context.Context) error
}

// ListFilter controls pagination and filtering for ListWorkflows.
type ListFilter struct {
	Status    string // "" = all statuses
	GraphName string // "" = all graphs
	Limit     int    // 0 = default 50
	Offset    int
}

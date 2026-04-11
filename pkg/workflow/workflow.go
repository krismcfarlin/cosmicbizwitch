package workflow

import "time"

// Status represents the lifecycle state of a Workflow.
type Status string

const (
	StatusPending   Status = "pending"   // created, waiting to be picked up
	StatusScheduled Status = "scheduled" // has a not_before time in the future
	StatusRunning   Status = "running"   // currently executing an activity
	StatusPaused    Status = "paused"    // waiting for human-in-the-middle trigger
	StatusCompleted Status = "completed" // all activities finished successfully
	StatusCancelled Status = "cancelled" // cancelled; stopped after current activity
	StatusFailed    Status = "failed"    // unrecoverable failure (max retries exhausted)
)

// ActivityStatus represents the lifecycle state of one activity execution.
type ActivityStatus string

const (
	ActivityPending      ActivityStatus = "pending"
	ActivityRunning      ActivityStatus = "running"
	ActivityCompleted    ActivityStatus = "completed"
	ActivityFailed       ActivityStatus = "failed"
	ActivitySkipped      ActivityStatus = "skipped"       // bypassed by conditional routing
	ActivityWaitingHuman ActivityStatus = "waiting_human" // human-in-the-middle
)

// Workflow is the top-level record that tracks one execution of a graph.
type Workflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	GraphName   string         `json:"graph_name"`
	Status      Status         `json:"status"`
	NotBefore   *time.Time     `json:"not_before,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	CurrentNode    string         `json:"current_node"`
	Context        map[string]any `json:"context"`                   // accumulated data across all activities
	InitialContext map[string]any `json:"initial_context,omitempty"` // snapshot of context at workflow creation
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// ActivityInstance is one execution record for a node within a workflow run.
type ActivityInstance struct {
	ID           string         `json:"id"`
	WorkflowID   string         `json:"workflow_id"`
	NodeID       string         `json:"node_id"`
	ActivityName string         `json:"activity_name"`
	Status       ActivityStatus `json:"status"`
	Input        map[string]any `json:"input"`
	Output       map[string]any `json:"output,omitempty"`
	ErrorMsg     string         `json:"error_msg,omitempty"`
	ErrorCount   int            `json:"error_count"`
	MaxRetries   int            `json:"max_retries"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// Event is broadcast to SSE subscribers on any workflow or activity state change.
type Event struct {
	Type     string            `json:"type"` // "workflow_update" | "activity_update"
	Workflow *Workflow         `json:"workflow,omitempty"`
	Activity *ActivityInstance `json:"activity,omitempty"`
}

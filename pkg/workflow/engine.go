package workflow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrSkip is returned by an activity to signal it should be marked as skipped
// rather than completed or failed. The workflow continues normally; no output
// is merged into the context.
var ErrSkip = errors.New("activity skipped")

// ActivityMeta holds documentation for a registered activity (for the visual builder UI).
type ActivityMeta struct {
	Category     string      `json:"category,omitempty"`
	Description  string      `json:"description,omitempty"`
	InputFields  []FieldMeta `json:"input_fields,omitempty"`
	OutputFields []FieldMeta `json:"output_fields,omitempty"`
}

// FieldMeta describes one input or output field of an activity.
type FieldMeta struct {
	Name        string `json:"name"`
	Type        string `json:"type"`        // "string", "number", "boolean", "any"
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ActivityInfo combines name and metadata for API responses.
type ActivityInfo struct {
	Name string       `json:"name"`
	Meta ActivityMeta `json:"meta"`
}

// Engine manages registered graphs and activities, dispatches runner goroutines,
// and polls storage for runnable workflows.
type Engine struct {
	store        Store
	registry     map[string]ActivityFunc
	activityMeta map[string]ActivityMeta
	graphs       map[string]*ActivityGraph

	mu      sync.RWMutex
	running map[string]context.CancelFunc // workflowID → cancel

	// humanChannels maps workflowID → channel the runner is blocking on.
	// TriggerHuman writes to this channel to resume a paused workflow.
	humanChannels sync.Map // map[string]chan HumanSignal

	// SSE subscribers
	subMu       sync.Mutex
	subscribers map[chan Event]struct{}

	logger       *log.Logger
	pollInterval time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// HumanSignal carries the trigger payload for human-in-the-middle nodes.
type HumanSignal struct {
	Input map[string]any
}

// EngineConfig holds optional configuration for the Engine.
type EngineConfig struct {
	PollInterval time.Duration // default 5s
	Logger       *log.Logger
}

// NewEngine creates a new Engine. Call RegisterActivity/RegisterGraph before Start.
func NewEngine(store Store, cfg EngineConfig) *Engine {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Engine{
		store:        store,
		registry:     make(map[string]ActivityFunc),
		activityMeta: make(map[string]ActivityMeta),
		graphs:       make(map[string]*ActivityGraph),
		running:      make(map[string]context.CancelFunc),
		subscribers:  make(map[chan Event]struct{}),
		logger:       cfg.Logger,
		pollInterval: cfg.PollInterval,
		stopCh:       make(chan struct{}),
	}
}

// RegisterActivity registers an ActivityFunc under a name.
// Must be called before Start.
func (e *Engine) RegisterActivity(name string, fn ActivityFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry[name] = fn
}

// RegisterActivityWithMeta registers an ActivityFunc along with its ActivityMeta
// documentation (used by the visual builder UI).
func (e *Engine) RegisterActivityWithMeta(name string, fn ActivityFunc, meta ActivityMeta) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry[name] = fn
	e.activityMeta[name] = meta
}

// ListActivities returns all registered activities with their metadata.
// Activities registered without metadata appear with an empty ActivityMeta.
func (e *Engine) ListActivities() []ActivityInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]ActivityInfo, 0, len(e.registry))
	for name := range e.registry {
		result = append(result, ActivityInfo{
			Name: name,
			Meta: e.activityMeta[name],
		})
	}
	return result
}

// RegisterGraph validates and registers an ActivityGraph.
// Must be called before Start.
func (e *Engine) RegisterGraph(g *ActivityGraph) error {
	e.mu.RLock()
	reg := e.registry
	e.mu.RUnlock()

	if err := g.Validate(reg); err != nil {
		return err
	}
	e.mu.Lock()
	e.graphs[g.Name] = g
	e.mu.Unlock()
	return nil
}

// CreateWorkflow creates and persists a new workflow.
// If notBefore is nil or in the past, status is "pending". Otherwise "scheduled".
func (e *Engine) CreateWorkflow(ctx context.Context, name, graphName string, initialContext map[string]any, notBefore *time.Time) (*Workflow, error) {
	e.mu.RLock()
	_, ok := e.graphs[graphName]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrGraphNotFound, graphName)
	}

	if initialContext == nil {
		initialContext = make(map[string]any)
	}

	status := StatusPending
	if notBefore != nil && notBefore.After(time.Now()) {
		status = StatusScheduled
	}

	wf := &Workflow{
		ID:        uuid.New().String(),
		Name:      name,
		GraphName: graphName,
		Status:    status,
		NotBefore: notBefore,
		Context:   initialContext,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := e.store.CreateWorkflow(ctx, wf); err != nil {
		return nil, err
	}

	e.broadcast(Event{Type: "workflow_update", Workflow: wf})
	return wf, nil
}

// Start begins the polling loop. Call this once after registering all activities/graphs.
// On startup it reloads any user-persisted graphs and resets stuck workflows so the
// poller can pick them up cleanly.
func (e *Engine) Start(ctx context.Context) {
	// Load user-persisted graphs (does not overwrite graphs already registered in code).
	if graphs, err := e.store.LoadGraphs(ctx); err == nil {
		for _, g := range graphs {
			if regErr := e.RegisterGraph(g); regErr != nil {
				e.logger.Printf("[workflow] failed to reload graph %q: %v", g.Name, regErr)
			} else {
				e.logger.Printf("[workflow] reloaded graph %q from store", g.Name)
			}
		}
	} else {
		e.logger.Printf("[workflow] load graphs error: %v", err)
	}

	// Reset any workflows that were interrupted mid-execution so the poller picks them up.
	if err := e.store.ResetStuckWorkflows(ctx); err != nil {
		e.logger.Printf("[workflow] reset stuck workflows error: %v", err)
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.pollLoop(ctx)
	}()
	e.logger.Println("[workflow] engine started")
}

// Stop signals the polling loop to exit and waits for all runners to finish.
func (e *Engine) Stop() {
	close(e.stopCh)
	e.wg.Wait()
	e.logger.Println("[workflow] engine stopped")
}

// CancelWorkflow marks a workflow as cancelled.
// If it's currently running, the goroutine will stop after the current activity.
func (e *Engine) CancelWorkflow(ctx context.Context, id string) error {
	wf, err := e.store.GetWorkflow(ctx, id)
	if err != nil {
		return ErrWorkflowNotFound
	}
	if wf.Status == StatusCompleted || wf.Status == StatusCancelled || wf.Status == StatusFailed {
		return ErrNotCancellable
	}
	wf.Status = StatusCancelled
	wf.UpdatedAt = time.Now().UTC()
	if err := e.store.UpdateWorkflow(ctx, wf); err != nil {
		return err
	}
	// Cancel the running goroutine if any.
	e.mu.RLock()
	cancel, ok := e.running[id]
	e.mu.RUnlock()
	if ok {
		cancel()
	}
	e.broadcast(Event{Type: "workflow_update", Workflow: wf})
	return nil
}

// RestartWorkflow resets a workflow and all its activity instances, then re-queues it.
func (e *Engine) RestartWorkflow(ctx context.Context, id string, overrideContext map[string]any) error {
	wf, err := e.store.GetWorkflow(ctx, id)
	if err != nil {
		return ErrWorkflowNotFound
	}

	// Cancel running goroutine first.
	e.mu.RLock()
	cancel, ok := e.running[id]
	e.mu.RUnlock()
	if ok {
		cancel()
		// Give the goroutine a moment to exit.
		time.Sleep(100 * time.Millisecond)
	}

	// Delete all activity instances.
	if err := e.store.DeleteActivityInstancesForWorkflow(ctx, id); err != nil {
		return err
	}

	if overrideContext != nil {
		wf.Context = overrideContext
	}
	wf.Status = StatusPending
	wf.CurrentNode = ""
	wf.StartedAt = nil
	wf.FinishedAt = nil
	wf.UpdatedAt = time.Now().UTC()

	if err := e.store.UpdateWorkflow(ctx, wf); err != nil {
		return err
	}
	e.broadcast(Event{Type: "workflow_update", Workflow: wf})
	e.logger.Printf("[workflow] restarted workflow %s", id)
	return nil
}

// TriggerHuman resumes a paused (human-in-the-middle) workflow.
func (e *Engine) TriggerHuman(ctx context.Context, id string, input map[string]any) error {
	wf, err := e.store.GetPausedWorkflow(ctx, id)
	if err != nil || wf == nil {
		return ErrNotPaused
	}
	ch, ok := e.humanChannels.Load(id)
	if !ok {
		return ErrNotPaused
	}
	ch.(chan HumanSignal) <- HumanSignal{Input: input}
	return nil
}

// Subscribe registers an SSE subscriber for workflow/activity events.
func (e *Engine) Subscribe() chan Event {
	ch := make(chan Event, 64)
	e.subMu.Lock()
	e.subscribers[ch] = struct{}{}
	e.subMu.Unlock()
	return ch
}

// Unsubscribe removes and closes an SSE subscriber channel.
func (e *Engine) Unsubscribe(ch chan Event) {
	e.subMu.Lock()
	delete(e.subscribers, ch)
	e.subMu.Unlock()
	close(ch)
}

// broadcast sends an event to all SSE subscribers.
func (e *Engine) broadcast(ev Event) {
	e.subMu.Lock()
	for ch := range e.subscribers {
		select {
		case ch <- ev:
		default: // drop if subscriber is slow
		}
	}
	e.subMu.Unlock()
}

// pollLoop ticks and dispatches runners for runnable workflows.
func (e *Engine) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	// Immediate first tick.
	e.poll(ctx)

	for {
		select {
		case <-ticker.C:
			e.poll(ctx)
		case <-e.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) poll(ctx context.Context) {
	workflows, err := e.store.GetRunnableWorkflows(ctx)
	if err != nil {
		e.logger.Printf("[workflow] poll error: %v", err)
		return
	}
	for _, wf := range workflows {
		e.dispatch(ctx, wf)
	}
}

// dispatch starts a runner goroutine for one workflow if not already running.
func (e *Engine) dispatch(ctx context.Context, wf *Workflow) {
	e.mu.Lock()
	if _, ok := e.running[wf.ID]; ok {
		e.mu.Unlock()
		return // already running
	}
	runCtx, cancel := context.WithCancel(ctx)
	e.running[wf.ID] = cancel
	e.mu.Unlock()

	e.wg.Add(1)
	go func() {
		defer func() {
			e.wg.Done()
			cancel()
			e.mu.Lock()
			delete(e.running, wf.ID)
			e.mu.Unlock()
		}()
		e.runWorkflow(runCtx, wf)
	}()
}

// GetGraph returns a registered graph by name (for API inspection/visualization).
func (e *Engine) GetGraph(name string) (*ActivityGraph, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	g, ok := e.graphs[name]
	return g, ok
}

// Store returns the underlying Store (for direct queries from HTTP handlers).
func (e *Engine) Store() Store {
	return e.store
}

// ExecuteActivity looks up a registered activity by name and invokes it directly,
// bypassing the workflow scheduler. Useful for one-off node testing from the UI.
func (e *Engine) ExecuteActivity(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	e.mu.RLock()
	fn, ok := e.registry[name]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("activity %q not registered", name)
	}
	return fn(ctx, input)
}

// ListGraphs returns all registered graph names.
func (e *Engine) ListGraphs() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.graphs))
	for n := range e.graphs {
		names = append(names, n)
	}
	return names
}

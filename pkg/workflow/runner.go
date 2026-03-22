package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// runWorkflow is the main goroutine that executes one workflow end-to-end.
func (e *Engine) runWorkflow(ctx context.Context, wf *Workflow) {
	e.logger.Printf("[workflow] starting workflow %s (%s/%s)", wf.ID, wf.GraphName, wf.Name)

	e.mu.RLock()
	graph, ok := e.graphs[wf.GraphName]
	e.mu.RUnlock()
	if !ok {
		e.failWorkflow(ctx, wf, fmt.Sprintf("graph %q not registered", wf.GraphName))
		return
	}

	// Initialise the current node if this is a fresh start.
	if wf.CurrentNode == "" {
		wf.CurrentNode = graph.StartNode
	}
	now := time.Now().UTC()
	wf.Status = StatusRunning
	wf.StartedAt = &now
	wf.UpdatedAt = now
	if err := e.store.UpdateWorkflow(ctx, wf); err != nil {
		e.logger.Printf("[workflow] failed to update workflow %s: %v", wf.ID, err)
		return
	}
	e.broadcast(Event{Type: "workflow_update", Workflow: wf})

	for {
		// Reload to catch external cancellation.
		fresh, err := e.store.GetWorkflow(ctx, wf.ID)
		if err != nil {
			e.logger.Printf("[workflow] reload failed for %s: %v", wf.ID, err)
			return
		}
		wf = fresh

		if wf.Status == StatusCancelled {
			e.logger.Printf("[workflow] workflow %s cancelled", wf.ID)
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		nodeID := wf.CurrentNode
		node, ok := graph.Nodes[nodeID]
		if !ok {
			e.failWorkflow(ctx, wf, fmt.Sprintf("node %q not found in graph", nodeID))
			return
		}

		// Find or create the ActivityInstance for this node.
		inst, err := e.findOrCreateInstance(ctx, wf, node)
		if err != nil {
			e.failWorkflow(ctx, wf, fmt.Sprintf("instance setup failed: %v", err))
			return
		}

		// Special handling: loop iterates body nodes for each item in a list.
		if node.ActivityName == "loop" {
			doneNodeID, err := e.runLoop(ctx, wf, graph, node)
			if err != nil {
				e.failWorkflow(ctx, wf, fmt.Sprintf("loop at %s failed: %v", nodeID, err))
				return
			}
			// Mark the loop instance as completed.
			now := time.Now().UTC()
			inst.Status = ActivityCompleted
			inst.FinishedAt = &now
			inst.UpdatedAt = now
			_ = e.store.UpdateActivityInstance(ctx, inst)
			e.broadcast(Event{Type: "activity_update", Activity: inst})

			if doneNodeID == "" {
				e.completeWorkflow(ctx, wf)
				return
			}
			wf.CurrentNode = doneNodeID
			wf.UpdatedAt = time.Now().UTC()
			_ = e.store.UpdateWorkflow(ctx, wf)
			e.broadcast(Event{Type: "workflow_update", Workflow: wf})
			continue
		}

		// Special handling: muxer fans out all transitions in parallel.
		if node.ActivityName == "muxer" {
			condenserID, err := e.runMuxerBranches(ctx, wf, graph, node)
			if err != nil {
				e.failWorkflow(ctx, wf, fmt.Sprintf("muxer at %s failed: %v", nodeID, err))
				return
			}
			// Mark the muxer instance as completed.
			now := time.Now().UTC()
			inst.Status = ActivityCompleted
			inst.FinishedAt = &now
			inst.UpdatedAt = now
			_ = e.store.UpdateActivityInstance(ctx, inst)
			e.broadcast(Event{Type: "activity_update", Activity: inst})

			// Advance past the condenser to its next node.
			nextAfterCondenser, found := graph.NextNode(condenserID, wf.Context)
			if !found || nextAfterCondenser == "" {
				e.completeWorkflow(ctx, wf)
				return
			}
			wf.CurrentNode = nextAfterCondenser
			wf.UpdatedAt = time.Now().UTC()
			_ = e.store.UpdateWorkflow(ctx, wf)
			e.broadcast(Event{Type: "workflow_update", Workflow: wf})
			continue
		}

		// Human-in-the-middle: pause workflow, wait for external trigger.
		if node.IsHuman {
			inst, err = e.handleHumanNode(ctx, wf, inst)
			if err != nil {
				// Context cancelled.
				return
			}
		} else {
			// Execute the activity with retries.
			inst, err = e.executeActivity(ctx, wf, node, inst)
			if err != nil {
				// Permanent failure already persisted by executeActivity.
				return
			}
		}

		// Merge activity output into workflow context.
		if inst.Output != nil {
			for k, v := range inst.Output {
				wf.Context[k] = v
			}
		}
		wf.UpdatedAt = time.Now().UTC()
		if err := e.store.UpdateWorkflow(ctx, wf); err != nil {
			e.logger.Printf("[workflow] context merge failed for %s: %v", wf.ID, err)
			return
		}

		// Check cancellation again between activities.
		wf, err = e.store.GetWorkflow(ctx, wf.ID)
		if err != nil {
			return
		}
		if wf.Status == StatusCancelled {
			e.logger.Printf("[workflow] workflow %s cancelled after activity", wf.ID)
			return
		}

		// Evaluate transitions to find the next node.
		nextNodeID, found := graph.NextNode(nodeID, inst.Output)
		if !found {
			node := graph.Nodes[nodeID]
			if node != nil && len(node.Transitions) == 0 {
				e.logger.Printf("[workflow] node %s (%s) has no transitions — workflow ends here. Connect it to the next node in the builder.", nodeID, node.ActivityName)
			} else {
				e.logger.Printf("[workflow] node %s: no transition matched output keys=%v — workflow ends here.", nodeID, outputKeys(inst.Output))
			}
			e.completeWorkflow(ctx, wf)
			return
		}
		if nextNodeID == "" {
			// Explicit end-of-workflow (transition to END node).
			e.completeWorkflow(ctx, wf)
			return
		}

		// Advance to next node.
		wf.CurrentNode = nextNodeID
		wf.UpdatedAt = time.Now().UTC()
		if err := e.store.UpdateWorkflow(ctx, wf); err != nil {
			e.logger.Printf("[workflow] advance failed for %s: %v", wf.ID, err)
			return
		}
		e.broadcast(Event{Type: "workflow_update", Workflow: wf})
	}
}

// findOrCreateInstance returns an existing pending/running instance for this node,
// or creates a fresh one.
func (e *Engine) findOrCreateInstance(ctx context.Context, wf *Workflow, node *Node) (*ActivityInstance, error) {
	instances, err := e.store.ListActivityInstances(ctx, wf.ID)
	if err != nil {
		return nil, err
	}
	for _, inst := range instances {
		if inst.NodeID == node.ID && inst.Status != ActivityCompleted && inst.Status != ActivitySkipped {
			return inst, nil
		}
	}

	// Build input from workflow context, then overlay node's static input.
	input := buildInput(wf.Context, node.InputMapping, node.Input)

	now := time.Now().UTC()
	inst := &ActivityInstance{
		ID:           uuid.New().String(),
		WorkflowID:   wf.ID,
		NodeID:       node.ID,
		ActivityName: node.ActivityName,
		Status:       ActivityPending,
		Input:        input,
		MaxRetries:   node.MaxRetries,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := e.store.CreateActivityInstance(ctx, inst); err != nil {
		return nil, err
	}
	e.broadcast(Event{Type: "activity_update", Activity: inst})
	return inst, nil
}

// executeActivity runs an activity with retry logic.
// Returns the completed instance, or an error if permanently failed.
func (e *Engine) executeActivity(ctx context.Context, wf *Workflow, node *Node, inst *ActivityInstance) (*ActivityInstance, error) {
	e.mu.RLock()
	fn, ok := e.registry[node.ActivityName]
	e.mu.RUnlock()
	if !ok {
		inst.Status = ActivityFailed
		inst.ErrorMsg = fmt.Sprintf("activity %q not registered", node.ActivityName)
		now := time.Now().UTC()
		inst.FinishedAt = &now
		inst.UpdatedAt = now
		_ = e.store.UpdateActivityInstance(ctx, inst)
		e.broadcast(Event{Type: "activity_update", Activity: inst})
		e.failWorkflow(ctx, wf, inst.ErrorMsg)
		return inst, fmt.Errorf("%s", inst.ErrorMsg)
	}

	now := time.Now().UTC()
	inst.Status = ActivityRunning
	inst.StartedAt = &now
	inst.UpdatedAt = now
	_ = e.store.UpdateActivityInstance(ctx, inst)
	e.broadcast(Event{Type: "activity_update", Activity: inst})

	for {
		select {
		case <-ctx.Done():
			return inst, ctx.Err()
		default:
		}

		output, err := fn(ctx, inst.Input)
		if err == nil {
			finished := time.Now().UTC()
			inst.Output = output
			inst.Status = ActivityCompleted
			inst.FinishedAt = &finished
			inst.UpdatedAt = finished
			_ = e.store.UpdateActivityInstance(ctx, inst)
			e.broadcast(Event{Type: "activity_update", Activity: inst})
			e.logger.Printf("[workflow] node %s completed in workflow %s", node.ID, wf.ID)
			return inst, nil
		}

		inst.ErrorCount++
		inst.ErrorMsg = err.Error()
		inst.UpdatedAt = time.Now().UTC()
		_ = e.store.UpdateActivityInstance(ctx, inst)
		e.broadcast(Event{Type: "activity_update", Activity: inst})
		e.logger.Printf("[workflow] node %s error (attempt %d/%d): %v", node.ID, inst.ErrorCount, inst.MaxRetries+1, err)

		if inst.ErrorCount > inst.MaxRetries {
			finished := time.Now().UTC()
			inst.Status = ActivityFailed
			inst.FinishedAt = &finished
			inst.UpdatedAt = finished
			_ = e.store.UpdateActivityInstance(ctx, inst)
			e.broadcast(Event{Type: "activity_update", Activity: inst})
			e.failWorkflow(ctx, wf, fmt.Sprintf("node %s failed after %d attempts: %s", node.ID, inst.ErrorCount, err))
			return inst, err
		}

		// Exponential backoff: 500ms, 1s, 2s, 4s … capped at 30s.
		backoff := time.Duration(math.Min(float64(500*time.Millisecond)*math.Pow(2, float64(inst.ErrorCount-1)), float64(30*time.Second)))
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return inst, ctx.Err()
		}
	}
}

// handleHumanNode pauses the workflow and waits for an external trigger.
func (e *Engine) handleHumanNode(ctx context.Context, wf *Workflow, inst *ActivityInstance) (*ActivityInstance, error) {
	e.logger.Printf("[workflow] paused at human node %s in workflow %s", inst.NodeID, wf.ID)

	now := time.Now().UTC()
	inst.Status = ActivityWaitingHuman
	inst.StartedAt = &now
	inst.UpdatedAt = now
	_ = e.store.UpdateActivityInstance(ctx, inst)
	e.broadcast(Event{Type: "activity_update", Activity: inst})

	wf.Status = StatusPaused
	wf.UpdatedAt = now
	_ = e.store.UpdateWorkflow(ctx, wf)
	e.broadcast(Event{Type: "workflow_update", Workflow: wf})

	// Register the human trigger channel.
	humanCh := make(chan HumanSignal, 1)
	e.humanChannels.Store(wf.ID, humanCh)
	defer e.humanChannels.Delete(wf.ID)

	select {
	case <-ctx.Done():
		return inst, ctx.Err()
	case sig := <-humanCh:
		// Merge human-provided input into output (and context will merge from output).
		if sig.Input != nil {
			inst.Output = sig.Input
		} else {
			inst.Output = inst.Input // pass input through if no human data
		}
		finished := time.Now().UTC()
		inst.Status = ActivityCompleted
		inst.FinishedAt = &finished
		inst.UpdatedAt = finished
		_ = e.store.UpdateActivityInstance(ctx, inst)
		e.broadcast(Event{Type: "activity_update", Activity: inst})

		wf.Status = StatusRunning
		wf.UpdatedAt = finished
		_ = e.store.UpdateWorkflow(ctx, wf)
		e.broadcast(Event{Type: "workflow_update", Workflow: wf})

		e.logger.Printf("[workflow] human node %s triggered in workflow %s", inst.NodeID, wf.ID)
		return inst, nil
	}
}

// failWorkflow marks a workflow as permanently failed.
func (e *Engine) failWorkflow(ctx context.Context, wf *Workflow, reason string) {
	e.logger.Printf("[workflow] FAILED workflow %s: %s", wf.ID, reason)
	now := time.Now().UTC()
	wf.Status = StatusFailed
	wf.FinishedAt = &now
	wf.UpdatedAt = now
	_ = e.store.UpdateWorkflow(ctx, wf)
	e.broadcast(Event{Type: "workflow_update", Workflow: wf})
}

// completeWorkflow marks a workflow as successfully completed.
func (e *Engine) completeWorkflow(ctx context.Context, wf *Workflow) {
	e.logger.Printf("[workflow] completed workflow %s", wf.ID)
	now := time.Now().UTC()
	wf.Status = StatusCompleted
	wf.FinishedAt = &now
	wf.UpdatedAt = now
	_ = e.store.UpdateWorkflow(ctx, wf)
	e.broadcast(Event{Type: "workflow_update", Workflow: wf})
}

// runMuxerBranches executes all outgoing transitions from a muxer node in parallel.
// Each branch runs until it hits a condenser node. Returns the condenser node ID.
func (e *Engine) runMuxerBranches(ctx context.Context, wf *Workflow, graph *ActivityGraph, muxerNode *Node) (string, error) {
	type result struct {
		output      map[string]any
		condenserID string
		err         error
	}

	ch := make(chan result, len(muxerNode.Transitions))
	var wg sync.WaitGroup

	for _, t := range muxerNode.Transitions {
		if t.NextNode == "" {
			continue
		}
		wg.Add(1)
		go func(startID string) {
			defer wg.Done()
			output, cID, err := e.runBranch(ctx, wf, graph, startID, wf.Context)
			ch <- result{output: output, condenserID: cID, err: err}
		}(t.NextNode)
	}

	wg.Wait()
	close(ch)

	var condenserID string
	var firstErr error
	for r := range ch {
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		if r.condenserID != "" {
			condenserID = r.condenserID
		}
		// Merge branch output into workflow context (last-write-wins).
		for k, v := range r.output {
			wf.Context[k] = v
		}
	}

	if firstErr != nil {
		return "", firstErr
	}
	if condenserID == "" {
		return "", fmt.Errorf("muxer: no branch reached a condenser node")
	}

	// Persist merged context.
	wf.UpdatedAt = time.Now().UTC()
	_ = e.store.UpdateWorkflow(ctx, wf)
	return condenserID, nil
}

// runBranch sequentially executes nodes starting at startID until it hits a condenser
// node (or a dead end). Returns the final branch output and the condenser node ID (if any).
func (e *Engine) runBranch(ctx context.Context, wf *Workflow, graph *ActivityGraph, startID string, initialCtx map[string]any) (map[string]any, string, error) {
	// Give this branch its own copy of the context.
	branchCtx := make(map[string]any, len(initialCtx))
	for k, v := range initialCtx {
		branchCtx[k] = v
	}

	currentID := startID
	for {
		select {
		case <-ctx.Done():
			return branchCtx, "", ctx.Err()
		default:
		}

		node, ok := graph.Nodes[currentID]
		if !ok {
			return branchCtx, "", fmt.Errorf("branch: node %q not found", currentID)
		}

		// Stop when we reach the condenser.
		if node.ActivityName == "condenser" {
			return branchCtx, currentID, nil
		}

		// Create and execute the activity instance for this branch node.
		inst, err := e.findOrCreateInstance(ctx, wf, node)
		if err != nil {
			return branchCtx, "", err
		}

		if node.IsHuman {
			inst, err = e.handleHumanNode(ctx, wf, inst)
		} else {
			inst, err = e.executeActivity(ctx, wf, node, inst)
		}
		if err != nil {
			return branchCtx, "", err
		}

		// Merge output into branch context.
		for k, v := range inst.Output {
			branchCtx[k] = v
		}

		nextID, found := graph.NextNode(currentID, inst.Output)
		if !found || nextID == "" {
			return branchCtx, "", nil // branch ended before reaching condenser
		}
		currentID = nextID
	}
}

// runLoop executes a loop node: iterates over a list, running body nodes for each item.
// Returns the ID of the node to run after the loop (from the "done" transition), or "" for end.
func (e *Engine) runLoop(ctx context.Context, wf *Workflow, graph *ActivityGraph, loopNode *Node) (string, error) {
	listKey, _ := loopNode.Input["list_key"].(string)
	if listKey == "" {
		listKey = "records"
	}
	itemKey, _ := loopNode.Input["item_key"].(string)
	if itemKey == "" {
		itemKey = "item"
	}

	// Support dotted paths like "records.records"
	var rawList any = map[string]any(wf.Context)
	for _, part := range strings.Split(listKey, ".") {
		m, ok := rawList.(map[string]any)
		if !ok {
			rawList = nil
			break
		}
		rawList = m[part]
	}
	items, ok := toAnySlice(rawList)
	if !ok {
		return "", fmt.Errorf("loop: context key %q is not a list (got %T)", listKey, rawList)
	}

	var bodyNodeID, doneNodeID string
	for _, t := range loopNode.Transitions {
		switch t.Label {
		case "body":
			bodyNodeID = t.NextNode
		case "done":
			doneNodeID = t.NextNode
		}
	}
	// Fallback: if no explicit "body" label, use the first transition as body
	if bodyNodeID == "" && len(loopNode.Transitions) > 0 {
		bodyNodeID = loopNode.Transitions[0].NextNode
	}
	if bodyNodeID == "" {
		return "", fmt.Errorf("loop node %q has no 'body' transition", loopNode.ID)
	}

	e.logger.Printf("[workflow] loop %s: %d items, item_key=%q", loopNode.ID, len(items), itemKey)

	for i, item := range items {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		wf.Context[itemKey] = item
		wf.Context["loop_index"] = i
		wf.UpdatedAt = time.Now().UTC()
		_ = e.store.UpdateWorkflow(ctx, wf)
		e.broadcast(Event{Type: "workflow_update", Workflow: wf})

		if err := e.runLoopIteration(ctx, wf, graph, bodyNodeID); err != nil {
			return "", fmt.Errorf("iteration %d: %w", i, err)
		}
	}

	// Remove loop-specific keys from context after completion.
	delete(wf.Context, itemKey)
	delete(wf.Context, "loop_index")
	wf.UpdatedAt = time.Now().UTC()
	_ = e.store.UpdateWorkflow(ctx, wf)

	return doneNodeID, nil
}

// runLoopIteration runs nodes starting at startID until it reaches a loop_next node.
func (e *Engine) runLoopIteration(ctx context.Context, wf *Workflow, graph *ActivityGraph, startID string) error {
	currentID := startID
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node, ok := graph.Nodes[currentID]
		if !ok {
			return fmt.Errorf("loop body: node %q not found", currentID)
		}
		if node.ActivityName == "loop_next" {
			return nil // end of iteration
		}

		inst, err := e.findOrCreateInstance(ctx, wf, node)
		if err != nil {
			return err
		}

		if node.IsHuman {
			inst, err = e.handleHumanNode(ctx, wf, inst)
		} else {
			inst, err = e.executeActivity(ctx, wf, node, inst)
		}
		if err != nil {
			return err
		}

		for k, v := range inst.Output {
			wf.Context[k] = v
		}
		wf.UpdatedAt = time.Now().UTC()
		_ = e.store.UpdateWorkflow(ctx, wf)

		nextID, found := graph.NextNode(currentID, inst.Output)
		if !found || nextID == "" {
			return nil
		}
		currentID = nextID
	}
}

// toAnySlice coerces v into []any if possible.
func toAnySlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	if s, ok := v.([]any); ok {
		return s, true
	}
	return nil, false
}

// buildInput returns the input map to pass to an activity.
// If mapping is nil, a shallow copy of the full context is returned.
// Otherwise only the specified keys are included.
// staticInput values are merged on top (with {{key}} template interpolation),
// allowing node-level configuration to override or extend the workflow context.
func buildInput(ctx map[string]any, mapping []string, staticInput map[string]any) map[string]any {
	var out map[string]any
	if mapping == nil {
		out = make(map[string]any, len(ctx))
		for k, v := range ctx {
			out[k] = v
		}
	} else {
		out = make(map[string]any, len(mapping))
		for _, k := range mapping {
			if v, ok := ctx[k]; ok {
				out[k] = v
			}
		}
	}
	// Overlay static node input, applying {{key}} template interpolation deeply.
	for k, v := range staticInput {
		out[k] = deepInterpolate(v, ctx)
	}
	return out
}

// deepInterpolate recursively walks a value and interpolates {{key}} placeholders
// in any strings found, including inside maps and slices.
func deepInterpolate(v any, ctx map[string]any) any {
	switch val := v.(type) {
	case string:
		interpolated := interpolateCtx(val, ctx)
		trimmed := strings.TrimSpace(interpolated)
		if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
			(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
			var parsed any
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				return parsed
			}
		}
		return interpolated
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = deepInterpolate(item, ctx)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = deepInterpolate(item, ctx)
		}
		return out
	default:
		return v
	}
}

func outputKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// interpolateCtx replaces {{key}} and {{a.b.c}} placeholders with values from ctx.
func interpolateCtx(s string, ctx map[string]any) string {
	// Flat keys first.
	for k, v := range ctx {
		s = strings.ReplaceAll(s, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}
	// Dot-notation: resolve any remaining {{a.b.c}} placeholders.
	s = dotPlaceholderRe.ReplaceAllStringFunc(s, func(match string) string {
		key := match[2 : len(match)-2] // strip {{ }}
		parts := strings.Split(key, ".")
		var cur any = map[string]any(ctx)
		for _, p := range parts {
			m, ok := cur.(map[string]any)
			if !ok {
				return match // unresolved — leave as-is
			}
			cur, ok = m[p]
			if !ok {
				return match
			}
		}
		return fmt.Sprintf("%v", cur)
	})
	return s
}

var dotPlaceholderRe = regexp.MustCompile(`\{\{[a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)+\}\}`)

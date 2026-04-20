package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"cosmicbizwitch/pkg/workflow"
)

// handleWorkflowList returns paginated workflows.
// GET /api/workflows?status=running&graph=my_graph&limit=50&offset=0
// Multiple status values are supported: ?status=completed&status=failed
func (s *Server) handleWorkflowList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	statuses := q["status"] // []string, may be empty, single, or multiple

	filter := workflow.ListFilter{
		GraphName: q.Get("graph"),
		Limit:     limit,
		Offset:    offset,
	}
	if len(statuses) == 1 {
		filter.Status = statuses[0]
	} else if len(statuses) > 1 {
		filter.Statuses = statuses
	}

	store := s.engine.Store()
	wfs, err := store.ListWorkflows(r.Context(), filter)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "list workflows failed", err)
		return
	}
	total, _ := store.CountWorkflows(r.Context(), filter)

	s.respondJSON(w, http.StatusOK, map[string]any{"workflows": wfs, "count": total})
}

// handleWorkflowCreate creates a new workflow.
// POST /api/workflows
// Body: { "name": "...", "graph_name": "...", "context": {...}, "not_before": "RFC3339" }
func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string         `json:"name"`
		GraphName string         `json:"graph_name"`
		Context   map[string]any `json:"context"`
		NotBefore *string        `json:"not_before"`
		StartNode string         `json:"start_node"` // optional: override which node to start at
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.GraphName == "" {
		http.Error(w, "graph_name is required", http.StatusBadRequest)
		return
	}

	var notBefore *interface{ IsZero() bool }
	_ = notBefore

	wf, err := s.engine.CreateWorkflow(r.Context(), body.Name, body.GraphName, body.Context, nil)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "create workflow failed", err)
		return
	}
	if body.StartNode != "" {
		fresh, ferr := s.engine.Store().GetWorkflow(r.Context(), wf.ID)
		if ferr == nil {
			fresh.CurrentNode = body.StartNode
			_ = s.engine.Store().UpdateWorkflow(r.Context(), fresh)
			wf = fresh
		}
	}
	s.respondJSON(w, http.StatusCreated, map[string]any{"workflow": wf})
}

// handleWorkflowGet returns a single workflow with its activity instances.
// GET /api/workflows/{id}
func (s *Server) handleWorkflowGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf, err := s.engine.Store().GetWorkflow(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	activities, _ := s.engine.Store().ListActivityInstances(r.Context(), id)
	graph, _ := s.engine.GetGraph(wf.GraphName)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"workflow":   wf,
		"activities": activities,
		"graph":      graph,
	})
}

// handleWorkflowDelete permanently deletes a workflow and its activity instances.
// DELETE /api/workflows/{id}
func (s *Server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.engine.Store().DeleteWorkflow(r.Context(), id); err != nil {
		s.respondError(w, http.StatusInternalServerError, "delete workflow failed", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleWorkflowCancel cancels a workflow.
// POST /api/workflows/{id}/cancel
func (s *Server) handleWorkflowCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.engine.CancelWorkflow(r.Context(), id); err != nil {
		s.respondError(w, http.StatusConflict, "cancel failed", err)
		return
	}
	wf, _ := s.engine.Store().GetWorkflow(r.Context(), id)
	s.respondJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

// handleWorkflowRestart resets and re-queues a workflow.
// POST /api/workflows/{id}/restart
// Body (optional): { "context": {...} }
func (s *Server) handleWorkflowRestart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Context map[string]any `json:"context"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // ignore decode error; body is optional

	if err := s.engine.RestartWorkflow(r.Context(), id, body.Context); err != nil {
		s.respondError(w, http.StatusConflict, "restart failed", err)
		return
	}
	wf, _ := s.engine.Store().GetWorkflow(r.Context(), id)
	s.respondJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

// handleWorkflowTrigger resumes a paused (human-in-the-middle) workflow.
// POST /api/workflows/{id}/trigger
// Body: { "input": { "approved": true, ... } }
func (s *Server) handleWorkflowTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Input map[string]any `json:"input"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := s.engine.TriggerHuman(r.Context(), id, body.Input); err != nil {
		s.respondError(w, http.StatusConflict, "trigger failed", err)
		return
	}
	wf, _ := s.engine.Store().GetWorkflow(r.Context(), id)
	s.respondJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

// handleWorkflowActivitiesList returns all registered activities with metadata.
// GET /api/workflows/activities
func (s *Server) handleWorkflowActivitiesList(w http.ResponseWriter, r *http.Request) {
	activities := s.engine.ListActivities()
	s.respondJSON(w, http.StatusOK, map[string]any{"activities": activities})
}

// handleGraphSave accepts an inline graph definition (ActivityGraph JSON), registers it
// in the engine under the given name, and returns it. If a graph with that name already
// exists it is overwritten.
// POST /api/workflows/graphs
// Body: { "name": "my_graph", "graph": { ...ActivityGraph JSON... } }
func (s *Server) handleGraphSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string                  `json:"name"`
		Graph *workflow.ActivityGraph `json:"graph"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Graph == nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	body.Graph.Name = body.Name
	if err := s.engine.RegisterGraph(body.Graph); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid graph", err)
		return
	}
	// Persist so the graph survives server restarts. Non-fatal on failure.
	if err := s.engine.Store().PersistGraph(r.Context(), body.Graph); err != nil {
		s.logger.Printf("[workflow] failed to persist graph %q: %v", body.Name, err)
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"graph": body.Graph})
}

// handleWorkflowActivities returns all activity instances for a workflow.
// GET /api/workflows/{id}/activities
func (s *Server) handleWorkflowActivities(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	activities, err := s.engine.Store().ListActivityInstances(r.Context(), id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "list activities failed", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"activities": activities})
}

// handleExecuteNode executes a single registered activity by name and returns its output.
// POST /api/workflows/execute-node
// Body: { "activity": "my_activity", "input": { ... } }
func (s *Server) handleExecuteNode(w http.ResponseWriter, r *http.Request) {
	// Catch any panic so the browser always gets a JSON response instead of a dropped connection.
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Printf("handleExecuteNode: panic: %v", rec)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("internal panic: %v", rec)})
		}
	}()

	var req struct {
		Activity string         `json:"activity"`
		Input    map[string]any `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Activity == "" {
		http.Error(w, "activity required", http.StatusBadRequest)
		return
	}
	if req.Input == nil {
		req.Input = map[string]any{}
	}
	// Coerce any string values that are JSON objects/arrays back to native types.
	// The debug panel sends static inputs (e.g. "data") as interpolated JSON strings.
	for k, v := range req.Input {
		if s2, ok := v.(string); ok {
			t := strings.TrimSpace(s2)
			if (strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}")) ||
				(strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]")) {
				var parsed any
				if err := json.Unmarshal([]byte(t), &parsed); err == nil {
					req.Input[k] = parsed
				}
			}
		}
	}
	s.logger.Printf("execute-node: activity=%q input_keys=%v", req.Activity, inputKeys(req.Input))
	output, err := s.engine.ExecuteActivity(r.Context(), req.Activity, req.Input)
	if errors.Is(err, workflow.ErrSkip) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{}, "skipped": true})
		return
	}
	if err != nil {
		s.logger.Printf("execute-node: activity=%q error: %v", req.Activity, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"output": output})
}

func inputKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// handleGraphDuplicate duplicates an existing graph under a new name ("Copy of <original>").
// POST /api/workflows/graphs/{name}/duplicate
func (s *Server) handleGraphDuplicate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	src, ok := s.engine.GetGraph(name)
	if !ok {
		http.Error(w, "graph not found", http.StatusNotFound)
		return
	}

	// Build a deep copy via JSON round-trip.
	data, err := json.Marshal(src)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "marshal graph failed", err)
		return
	}
	var copy workflow.ActivityGraph
	if err := json.Unmarshal(data, &copy); err != nil {
		s.respondError(w, http.StatusInternalServerError, "unmarshal graph failed", err)
		return
	}
	copy.Name = "Copy of " + name

	if err := s.engine.RegisterGraph(&copy); err != nil {
		s.respondError(w, http.StatusBadRequest, "register duplicate graph failed", err)
		return
	}
	if err := s.engine.Store().PersistGraph(r.Context(), &copy); err != nil {
		s.logger.Printf("[workflow] failed to persist duplicate graph %q: %v", copy.Name, err)
	}
	s.respondJSON(w, http.StatusCreated, map[string]any{"graph": copy})
}

// handleGraphDelete removes a graph from the engine and store.
// DELETE /api/workflows/graphs/{name}
func (s *Server) handleGraphDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.engine.RemoveGraph(r.Context(), name); err != nil {
		s.respondError(w, http.StatusInternalServerError, "delete graph failed", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

// handleWorkflowGraphs returns all registered graph definitions.
// GET /api/workflows/graphs
func (s *Server) handleWorkflowGraphs(w http.ResponseWriter, r *http.Request) {
	names := s.engine.ListGraphs()
	graphs := make(map[string]any, len(names))
	for _, name := range names {
		if g, ok := s.engine.GetGraph(name); ok {
			graphs[name] = g
		}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"graphs": graphs, "names": names})
}

// handleWorkflowStream is an SSE endpoint for real-time workflow/activity updates.
// GET /api/workflows/stream
func (s *Server) handleWorkflowStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send a heartbeat immediately so the client knows it's connected.
	fmt.Fprint(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	ch := s.engine.Subscribe()
	defer s.engine.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// handlePbCollectionFields returns the field names and types for a PocketBase collection.
// GET /api/pb/collections/{name}/fields
func (s *Server) handlePbCollectionFields(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]any{"error": "collection name required"})
		return
	}

	col, err := s.store.App().FindCollectionByNameOrId(name)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]any{"error": "collection not found: " + name})
		return
	}

	type fieldInfo struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	fields := make([]fieldInfo, 0, len(col.Fields))
	for _, f := range col.Fields {
		// Skip internal PocketBase system fields
		if f.GetName() == "id" || f.GetName() == "created" || f.GetName() == "updated" {
			continue
		}
		fields = append(fields, fieldInfo{
			Name: f.GetName(),
			Type: f.Type(),
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]any{"fields": fields})
}

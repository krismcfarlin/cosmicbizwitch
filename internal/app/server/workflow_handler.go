package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"cosmicbizwitch/pkg/workflow"
)

// handleWorkflowList returns paginated workflows.
// GET /api/workflows?status=running&graph=my_graph&limit=50&offset=0
func (s *Server) handleWorkflowList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	wfs, err := s.engine.Store().ListWorkflows(r.Context(), workflow.ListFilter{
		Status:    q.Get("status"),
		GraphName: q.Get("graph"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "list workflows failed", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"workflows": wfs, "count": len(wfs)})
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

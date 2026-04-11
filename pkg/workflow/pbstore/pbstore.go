package pbstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	colWorkflows  = "wf_workflows"
	colActivities = "wf_activity_instances"
	colGraphs     = "wf_graphs"
	timeFmt       = "2006-01-02 15:04:05.000Z"
)

// PBStore implements workflow.Store using PocketBase.
type PBStore struct {
	app core.App
}

// New creates a new PBStore.
func New(app core.App) *PBStore {
	return &PBStore{app: app}
}

// ── Workflow ──────────────────────────────────────────────────────────────────

func (s *PBStore) CreateWorkflow(_ context.Context, wf *workflow.Workflow) error {
	col, err := s.app.FindCollectionByNameOrId(colWorkflows)
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	setWorkflowFields(rec, wf)
	if err := s.app.Save(rec); err != nil {
		return err
	}
	wf.ID = rec.Id
	return nil
}

func (s *PBStore) GetWorkflow(_ context.Context, id string) (*workflow.Workflow, error) {
	rec, err := s.app.FindRecordById(colWorkflows, id)
	if err != nil {
		return nil, workflow.ErrWorkflowNotFound
	}
	return recordToWorkflow(rec), nil
}

func (s *PBStore) UpdateWorkflow(_ context.Context, wf *workflow.Workflow) error {
	rec, err := s.app.FindRecordById(colWorkflows, wf.ID)
	if err != nil {
		return workflow.ErrWorkflowNotFound
	}
	setWorkflowFields(rec, wf)
	return s.app.Save(rec)
}

func (s *PBStore) ListWorkflows(_ context.Context, filter workflow.ListFilter) ([]*workflow.Workflow, error) {
	if filter.Limit == 0 {
		filter.Limit = 50
	}

	filterStr := "1=1"
	params := dbx.Params{}

	if filter.Status != "" {
		filterStr += " && status = {:status}"
		params["status"] = filter.Status
	}
	if filter.GraphName != "" {
		filterStr += " && graph_name = {:graph_name}"
		params["graph_name"] = filter.GraphName
	}

	records, err := s.app.FindRecordsByFilter(colWorkflows, filterStr, "-created", filter.Limit, filter.Offset, params)
	if err != nil {
		return nil, err
	}

	out := make([]*workflow.Workflow, len(records))
	for i, r := range records {
		out[i] = recordToWorkflow(r)
	}
	return out, nil
}

func (s *PBStore) GetRunnableWorkflows(_ context.Context) ([]*workflow.Workflow, error) {
	now := time.Now().UTC().Format(timeFmt)
	records, err := s.app.FindRecordsByFilter(
		colWorkflows,
		"status = {:pending} && (not_before = '' || not_before <= {:now})",
		"+created", 100, 0,
		dbx.Params{"pending": string(workflow.StatusPending), "now": now},
	)
	if err != nil {
		return nil, err
	}
	out := make([]*workflow.Workflow, len(records))
	for i, r := range records {
		out[i] = recordToWorkflow(r)
	}
	return out, nil
}

func (s *PBStore) GetPausedWorkflow(_ context.Context, id string) (*workflow.Workflow, error) {
	rec, err := s.app.FindRecordById(colWorkflows, id)
	if err != nil {
		return nil, workflow.ErrWorkflowNotFound
	}
	wf := recordToWorkflow(rec)
	if wf.Status != workflow.StatusPaused {
		return nil, workflow.ErrNotPaused
	}
	return wf, nil
}

// ── ActivityInstance ──────────────────────────────────────────────────────────

func (s *PBStore) CreateActivityInstance(_ context.Context, inst *workflow.ActivityInstance) error {
	col, err := s.app.FindCollectionByNameOrId(colActivities)
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	setActivityFields(rec, inst)
	if err := s.app.Save(rec); err != nil {
		return err
	}
	inst.ID = rec.Id
	return nil
}

func (s *PBStore) GetActivityInstance(_ context.Context, id string) (*workflow.ActivityInstance, error) {
	rec, err := s.app.FindRecordById(colActivities, id)
	if err != nil {
		return nil, workflow.ErrActivityNotFound
	}
	return recordToActivity(rec), nil
}

func (s *PBStore) UpdateActivityInstance(_ context.Context, inst *workflow.ActivityInstance) error {
	rec, err := s.app.FindRecordById(colActivities, inst.ID)
	if err != nil {
		return workflow.ErrActivityNotFound
	}
	setActivityFields(rec, inst)
	return s.app.Save(rec)
}

func (s *PBStore) ListActivityInstances(_ context.Context, workflowID string) ([]*workflow.ActivityInstance, error) {
	records, err := s.app.FindRecordsByFilter(
		colActivities,
		"workflow_id = {:wf_id}",
		"+created", 0, 0,
		dbx.Params{"wf_id": workflowID},
	)
	if err != nil {
		return nil, err
	}
	out := make([]*workflow.ActivityInstance, len(records))
	for i, r := range records {
		out[i] = recordToActivity(r)
	}
	return out, nil
}

func (s *PBStore) DeleteWorkflow(ctx context.Context, id string) error {
	if err := s.DeleteActivityInstancesForWorkflow(ctx, id); err != nil {
		return err
	}
	rec, err := s.app.FindRecordById(colWorkflows, id)
	if err != nil {
		return err
	}
	return s.app.Delete(rec)
}

func (s *PBStore) DeleteActivityInstancesForWorkflow(_ context.Context, workflowID string) error {
	records, err := s.app.FindRecordsByFilter(
		colActivities,
		"workflow_id = {:wf_id}",
		"+created", 0, 0,
		dbx.Params{"wf_id": workflowID},
	)
	if err != nil {
		return err
	}
	for _, r := range records {
		if err := s.app.Delete(r); err != nil {
			return err
		}
	}
	return nil
}

// ── Graph persistence ─────────────────────────────────────────────────────────

// PersistGraph upserts an ActivityGraph by name. If a record with that name
// already exists it is updated; otherwise a new record is created.
func (s *PBStore) PersistGraph(_ context.Context, g *workflow.ActivityGraph) error {
	data, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("marshal graph %q: %w", g.Name, err)
	}

	col, err := s.app.FindCollectionByNameOrId(colGraphs)
	if err != nil {
		return fmt.Errorf("find collection %s: %w", colGraphs, err)
	}

	records, err := s.app.FindRecordsByFilter(
		colGraphs,
		"name = {:name}",
		"+created", 1, 0,
		dbx.Params{"name": g.Name},
	)
	if err != nil {
		return fmt.Errorf("query graph %q: %w", g.Name, err)
	}

	var rec *core.Record
	if len(records) > 0 {
		rec = records[0]
	} else {
		rec = core.NewRecord(col)
	}
	rec.Set("name", g.Name)
	rec.Set("definition", string(data))
	return s.app.Save(rec)
}

// LoadGraphs returns all persisted ActivityGraphs.
func (s *PBStore) LoadGraphs(_ context.Context) ([]*workflow.ActivityGraph, error) {
	records, err := s.app.FindRecordsByFilter(
		colGraphs,
		"1=1",
		"+created", 0, 0,
		dbx.Params{},
	)
	if err != nil {
		return nil, fmt.Errorf("load graphs: %w", err)
	}

	out := make([]*workflow.ActivityGraph, 0, len(records))
	for _, r := range records {
		raw := r.GetString("definition")
		var g workflow.ActivityGraph
		if err := json.Unmarshal([]byte(raw), &g); err != nil {
			// Skip malformed records rather than aborting the whole load.
			continue
		}
		out = append(out, &g)
	}
	return out, nil
}

// ResetStuckWorkflows finds all workflows in running or paused status and
// resets them to pending so the poller picks them up after a restart.
func (s *PBStore) ResetStuckWorkflows(_ context.Context) error {
	records, err := s.app.FindRecordsByFilter(
		colWorkflows,
		"status = {:running} || status = {:paused}",
		"+created", 0, 0,
		dbx.Params{
			"running": string(workflow.StatusRunning),
			"paused":  string(workflow.StatusPaused),
		},
	)
	if err != nil {
		return fmt.Errorf("find stuck workflows: %w", err)
	}
	for _, rec := range records {
		rec.Set("status", string(workflow.StatusPending))
		rec.Set("started_at", "")
		if err := s.app.Save(rec); err != nil {
			return fmt.Errorf("reset workflow %s: %w", rec.Id, err)
		}
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func setWorkflowFields(rec *core.Record, wf *workflow.Workflow) {
	rec.Set("name", wf.Name)
	rec.Set("graph_name", wf.GraphName)
	rec.Set("status", string(wf.Status))
	rec.Set("current_node", wf.CurrentNode)
	rec.Set("context", wf.Context)
	rec.Set("initial_context", wf.InitialContext)

	if wf.NotBefore != nil {
		rec.Set("not_before", wf.NotBefore.UTC().Format(timeFmt))
	} else {
		rec.Set("not_before", "")
	}
	if wf.StartedAt != nil {
		rec.Set("started_at", wf.StartedAt.UTC().Format(timeFmt))
	} else {
		rec.Set("started_at", "")
	}
	if wf.FinishedAt != nil {
		rec.Set("finished_at", wf.FinishedAt.UTC().Format(timeFmt))
	} else {
		rec.Set("finished_at", "")
	}
}

func recordToWorkflow(rec *core.Record) *workflow.Workflow {
	wf := &workflow.Workflow{
		ID:          rec.Id,
		Name:        rec.GetString("name"),
		GraphName:   rec.GetString("graph_name"),
		Status:         workflow.Status(rec.GetString("status")),
		CurrentNode:    rec.GetString("current_node"),
		Context:        getJSONMap(rec, "context"),
		InitialContext: getJSONMap(rec, "initial_context"),
		CreatedAt:      parseTime(rec.GetString("created")),
		UpdatedAt:   parseTime(rec.GetString("updated")),
	}
	if t := rec.GetString("not_before"); t != "" {
		pt := parseTime(t)
		wf.NotBefore = &pt
	}
	if t := rec.GetString("started_at"); t != "" {
		pt := parseTime(t)
		wf.StartedAt = &pt
	}
	if t := rec.GetString("finished_at"); t != "" {
		pt := parseTime(t)
		wf.FinishedAt = &pt
	}
	return wf
}

func setActivityFields(rec *core.Record, inst *workflow.ActivityInstance) {
	rec.Set("workflow_id", inst.WorkflowID)
	rec.Set("node_id", inst.NodeID)
	rec.Set("activity_name", inst.ActivityName)
	rec.Set("status", string(inst.Status))
	rec.Set("input", inst.Input)
	rec.Set("output", inst.Output)
	rec.Set("error_msg", inst.ErrorMsg)
	rec.Set("error_count", inst.ErrorCount)
	rec.Set("max_retries", inst.MaxRetries)

	if inst.StartedAt != nil {
		rec.Set("started_at", inst.StartedAt.UTC().Format(timeFmt))
	} else {
		rec.Set("started_at", "")
	}
	if inst.FinishedAt != nil {
		rec.Set("finished_at", inst.FinishedAt.UTC().Format(timeFmt))
	} else {
		rec.Set("finished_at", "")
	}
}

func recordToActivity(rec *core.Record) *workflow.ActivityInstance {
	inst := &workflow.ActivityInstance{
		ID:           rec.Id,
		WorkflowID:   rec.GetString("workflow_id"),
		NodeID:       rec.GetString("node_id"),
		ActivityName: rec.GetString("activity_name"),
		Status:       workflow.ActivityStatus(rec.GetString("status")),
		Input:        getJSONMap(rec, "input"),
		Output:       getJSONMap(rec, "output"),
		ErrorMsg:     rec.GetString("error_msg"),
		ErrorCount:   rec.GetInt("error_count"),
		MaxRetries:   rec.GetInt("max_retries"),
		CreatedAt:    parseTime(rec.GetString("created")),
		UpdatedAt:    parseTime(rec.GetString("updated")),
	}
	if t := rec.GetString("started_at"); t != "" {
		pt := parseTime(t)
		inst.StartedAt = &pt
	}
	if t := rec.GetString("finished_at"); t != "" {
		pt := parseTime(t)
		inst.FinishedAt = &pt
	}
	return inst
}

func getJSONMap(rec *core.Record, field string) map[string]any {
	v := rec.Get(field)
	if v == nil {
		return make(map[string]any)
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	// PocketBase JSON fields may come back as []byte or string — unmarshal them.
	var raw []byte
	switch t := v.(type) {
	case []byte:
		raw = t
	case string:
		raw = []byte(t)
	default:
		// Try marshaling back to JSON then unmarshaling into map.
		if b, err := json.Marshal(v); err == nil {
			raw = b
		}
	}
	if len(raw) > 0 {
		m := make(map[string]any)
		if err := json.Unmarshal(raw, &m); err == nil {
			return m
		}
	}
	return make(map[string]any)
}

func parseTime(s string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05.000Z",
		"2006-01-02 15:04:05Z",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// Ensure PBStore implements workflow.Store at compile time.
var _ workflow.Store = (*PBStore)(nil)

// CountWorkflows returns the total count matching a filter (for API pagination).
func (s *PBStore) CountWorkflows(_ context.Context, filter workflow.ListFilter) (int, error) {
	filterStr := "1=1"
	params := dbx.Params{}
	if filter.Status != "" {
		filterStr += " && status = {:status}"
		params["status"] = filter.Status
	}
	if filter.GraphName != "" {
		filterStr += " && graph_name = {:graph_name}"
		params["graph_name"] = filter.GraphName
	}
	n, err := s.app.CountRecords(colWorkflows, dbx.HashExp(params))
	if err != nil {
		return 0, fmt.Errorf("count workflows: %w", err)
	}
	return int(n), nil
}

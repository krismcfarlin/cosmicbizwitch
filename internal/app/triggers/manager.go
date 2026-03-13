package triggers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Trigger represents one configured trigger in the DB.
type Trigger struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`       // webhook | record_hook | cron
	GraphName string         `json:"graph_name"`
	Config    map[string]any `json:"config"`
	Token     string         `json:"token"`
	Enabled   bool           `json:"enabled"`
	LastFired *time.Time     `json:"last_fired,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

const colTriggers = "wf_triggers"
const timeFmt     = "2006-01-02 15:04:05.000Z"

// Manager handles all trigger types.
type Manager struct {
	app    core.App
	engine *workflow.Engine
	logger *log.Logger
}

// New creates a new Manager. If logger is nil, log.Default() is used.
func New(app core.App, engine *workflow.Engine, logger *log.Logger) *Manager {
	if logger == nil {
		logger = log.Default()
	}
	return &Manager{app: app, engine: engine, logger: logger}
}

// Start registers PocketBase record hooks and launches the cron goroutine.
func (m *Manager) Start(ctx context.Context) {
	// Record hooks — catch-all; each hook queries DB for matching enabled triggers.
	m.app.OnRecordAfterCreateSuccess().BindFunc(func(e *core.RecordEvent) error {
		m.fireRecordHooks(ctx, e.Record, "create")
		return e.Next()
	})
	m.app.OnRecordAfterUpdateSuccess().BindFunc(func(e *core.RecordEvent) error {
		m.fireRecordHooks(ctx, e.Record, "update")
		return e.Next()
	})
	m.app.OnRecordAfterDeleteSuccess().BindFunc(func(e *core.RecordEvent) error {
		m.fireRecordHooks(ctx, e.Record, "delete")
		return e.Next()
	})

	// Cron goroutine — ticks every minute, checks which cron triggers are due.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.fireCronTriggers(ctx)
			}
		}
	}()

	m.logger.Println("[triggers] manager started")
}

// FireWebhook finds a trigger by token and starts a workflow with the given payload.
func (m *Manager) FireWebhook(ctx context.Context, token string, payload map[string]any) (*workflow.Workflow, error) {
	triggers, err := m.loadByToken(token)
	if err != nil || len(triggers) == 0 {
		return nil, fmt.Errorf("unknown or disabled trigger token")
	}
	t := triggers[0]
	wf, err := m.fire(ctx, t, payload)
	if err != nil {
		return nil, err
	}
	_ = m.updateLastFired(t.ID)
	return wf, nil
}

func (m *Manager) fireRecordHooks(ctx context.Context, rec *core.Record, event string) {
	triggers, err := m.loadEnabled("record_hook")
	if err != nil {
		return
	}
	colName := rec.Collection().Name
	for _, t := range triggers {
		cfgCol, _ := t.Config["collection"].(string)
		cfgEvt, _ := t.Config["event"].(string)
		if cfgCol != colName {
			continue
		}
		if cfgEvt != event && cfgEvt != "create_or_update" {
			continue
		}
		// Build context from record fields.
		payload := make(map[string]any)
		for _, f := range rec.Collection().Fields {
			payload[f.GetName()] = rec.Get(f.GetName())
		}
		payload["_trigger_event"] = event
		if _, err := m.fire(ctx, t, payload); err != nil {
			m.logger.Printf("[triggers] record_hook %s failed: %v", t.Name, err)
		} else {
			_ = m.updateLastFired(t.ID)
		}
	}
}

func (m *Manager) fireCronTriggers(ctx context.Context) {
	triggers, err := m.loadEnabled("cron")
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, t := range triggers {
		intervalStr, _ := t.Config["interval"].(string)
		if intervalStr == "" {
			continue
		}
		d, err := time.ParseDuration(intervalStr)
		if err != nil || d <= 0 {
			continue
		}
		if t.LastFired != nil && now.Sub(*t.LastFired) < d {
			continue // not due yet
		}
		if _, err := m.fire(ctx, t, map[string]any{"_trigger_time": now.Format(time.RFC3339)}); err != nil {
			m.logger.Printf("[triggers] cron %s failed: %v", t.Name, err)
		} else {
			_ = m.updateLastFired(t.ID)
		}
	}
}

func (m *Manager) fire(ctx context.Context, t *Trigger, payload map[string]any) (*workflow.Workflow, error) {
	// Merge initial_context (static defaults) under the dynamic payload so
	// event data always wins.
	merged := make(map[string]any)
	if ic, ok := t.Config["initial_context"]; ok {
		if icMap, ok := ic.(map[string]any); ok {
			for k, v := range icMap {
				merged[k] = v
			}
		}
	}
	for k, v := range payload {
		merged[k] = v
	}
	wf, err := m.engine.CreateWorkflow(ctx, t.Name, t.GraphName, merged, nil)
	if err != nil {
		return nil, fmt.Errorf("trigger %q: %w", t.Name, err)
	}
	m.logger.Printf("[triggers] fired %s (%s) → workflow %s", t.Name, t.Type, wf.ID)
	return wf, nil
}

// ── DB helpers ────────────────────────────────────────────────────────────────

func (m *Manager) loadEnabled(triggerType string) ([]*Trigger, error) {
	recs, err := m.app.FindRecordsByFilter(
		colTriggers,
		"enabled = true && type = {:type}",
		"+created", 0, 0,
		dbx.Params{"type": triggerType},
	)
	if err != nil {
		return nil, err
	}
	return recordsToTriggers(recs), nil
}

func (m *Manager) loadByToken(token string) ([]*Trigger, error) {
	recs, err := m.app.FindRecordsByFilter(
		colTriggers,
		"enabled = true && type = 'webhook' && token = {:token}",
		"+created", 1, 0,
		dbx.Params{"token": token},
	)
	if err != nil {
		return nil, err
	}
	return recordsToTriggers(recs), nil
}

func (m *Manager) updateLastFired(id string) error {
	rec, err := m.app.FindRecordById(colTriggers, id)
	if err != nil {
		return err
	}
	rec.Set("last_fired", time.Now().UTC().Format(timeFmt))
	return m.app.Save(rec)
}

// ── CRUD (used by HTTP handlers) ──────────────────────────────────────────────

// List returns all triggers ordered by creation time descending.
func (m *Manager) List() ([]*Trigger, error) {
	recs, err := m.app.FindRecordsByFilter(colTriggers, "1=1", "-created", 0, 0, dbx.Params{})
	if err != nil {
		return nil, err
	}
	return recordsToTriggers(recs), nil
}

// Get retrieves a single trigger by ID.
func (m *Manager) Get(id string) (*Trigger, error) {
	rec, err := m.app.FindRecordById(colTriggers, id)
	if err != nil {
		return nil, fmt.Errorf("trigger not found")
	}
	return recordToTrigger(rec), nil
}

// Create persists a new trigger. Webhook triggers without a token get one auto-generated.
func (m *Manager) Create(t *Trigger) (*Trigger, error) {
	col, err := m.app.FindCollectionByNameOrId(colTriggers)
	if err != nil {
		return nil, err
	}
	if t.Type == "webhook" && t.Token == "" {
		t.Token = generateToken()
	}
	rec := core.NewRecord(col)
	setTriggerFields(rec, t)
	if err := m.app.Save(rec); err != nil {
		return nil, err
	}
	t.ID = rec.Id
	return t, nil
}

// Update updates an existing trigger by ID.
func (m *Manager) Update(id string, t *Trigger) (*Trigger, error) {
	rec, err := m.app.FindRecordById(colTriggers, id)
	if err != nil {
		return nil, fmt.Errorf("trigger not found")
	}
	setTriggerFields(rec, t)
	if err := m.app.Save(rec); err != nil {
		return nil, err
	}
	return recordToTrigger(rec), nil
}

// Delete removes a trigger by ID.
func (m *Manager) Delete(id string) error {
	rec, err := m.app.FindRecordById(colTriggers, id)
	if err != nil {
		return fmt.Errorf("trigger not found")
	}
	return m.app.Delete(rec)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func setTriggerFields(rec *core.Record, t *Trigger) {
	rec.Set("name", t.Name)
	rec.Set("type", t.Type)
	rec.Set("graph_name", t.GraphName)
	rec.Set("config", t.Config)
	rec.Set("token", t.Token)
	rec.Set("enabled", t.Enabled)
}

func recordToTrigger(rec *core.Record) *Trigger {
	t := &Trigger{
		ID:        rec.Id,
		Name:      rec.GetString("name"),
		Type:      rec.GetString("type"),
		GraphName: rec.GetString("graph_name"),
		Token:     rec.GetString("token"),
		Enabled:   rec.GetBool("enabled"),
		Config:    getJSONMap(rec, "config"),
		CreatedAt: parseTime(rec.GetString("created")),
		UpdatedAt: parseTime(rec.GetString("updated")),
	}
	if lf := rec.GetString("last_fired"); lf != "" {
		pt := parseTime(lf)
		t.LastFired = &pt
	}
	return t
}

func recordsToTriggers(recs []*core.Record) []*Trigger {
	out := make([]*Trigger, len(recs))
	for i, r := range recs {
		out[i] = recordToTrigger(r)
	}
	return out
}

func getJSONMap(rec *core.Record, field string) map[string]any {
	v := rec.Get(field)
	if v == nil {
		return map[string]any{}
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	var raw []byte
	switch t := v.(type) {
	case []byte:
		raw = t
	case string:
		raw = []byte(t)
	default:
		if b, err := json.Marshal(v); err == nil {
			raw = b
		}
	}
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	return m
}

func parseTime(s string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05.000Z",
		"2006-01-02 15:04:05Z",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

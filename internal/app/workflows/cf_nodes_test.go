package workflows

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"

	"cosmicbizwitch/internal/app/clickfunnels"
	"cosmicbizwitch/pkg/workflow"
)

// newCFNodesEngine builds an Engine with all activities registered using a nil CF client.
// CF nodes that check getCF() at the top will receive a non-nil func returning nil,
// which exercises the "client not configured" branch.
func newCFNodesEngine() *workflow.Engine {
	nilCF := func() *clickfunnels.Client { return nil }
	eng := workflow.NewEngine(nil, workflow.EngineConfig{Logger: log.New(io.Discard, "", 0)})
	registerActivities(eng, nil, nilCF, log.New(io.Discard, "", 0))
	return eng
}

func execCF(t *testing.T, name string, input map[string]any) (map[string]any, error) {
	t.Helper()
	eng := newCFNodesEngine()
	return eng.ExecuteActivity(context.Background(), name, input)
}

// ── cf_upsert_contact (removed stub) ────────────────────────────────────────

func TestCFUpsertContactRemoved_AlwaysErrors(t *testing.T) {
	_, err := execCF(t, "cf_upsert_contact", map[string]any{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "removed") {
		t.Errorf("expected 'removed' in error, got: %v", err)
	}
}

// ── cf_upsert ───────────────────────────────────────────────────────────────

func TestCFUpsert_NilClient(t *testing.T) {
	_, err := execCF(t, "cf_upsert", map[string]any{"data": map[string]any{"email_address": "x@x.com"}})
	if err == nil {
		t.Fatal("expected error for nil CF client")
	}
	if !strings.Contains(err.Error(), "CF_API_KEY") {
		t.Errorf("expected CF_API_KEY in error, got: %v", err)
	}
}

// ── cf_get_contact ──────────────────────────────────────────────────────────

func TestCFGetContact_NilClient(t *testing.T) {
	out, err := execCF(t, "cf_get_contact", map[string]any{"contact_id": float64(123)})
	if err != nil {
		t.Fatalf("expected soft error, got hard error: %v", err)
	}
	// Soft error: result stored under default result_var "contact"
	contact, ok := out["contact"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'contact' key in output, got %v", out)
	}
	if contact["status"] != "error" {
		t.Errorf("expected status=error, got %v", contact["status"])
	}
}

func TestCFGetContact_CustomResultVar(t *testing.T) {
	out, err := execCF(t, "cf_get_contact", map[string]any{
		"contact_id": float64(1),
		"result_var": "my_contact",
	})
	if err != nil {
		t.Fatalf("expected soft error, got: %v", err)
	}
	if _, ok := out["my_contact"]; !ok {
		t.Errorf("expected 'my_contact' key in output, got keys: %v", out)
	}
}

// ── cf_search_contact ───────────────────────────────────────────────────────

func TestCFSearchContact_NilClient(t *testing.T) {
	out, err := execCF(t, "cf_search_contact", map[string]any{"email": "test@example.com"})
	if err != nil {
		t.Fatalf("expected soft error, got hard error: %v", err)
	}
	contacts, ok := out["contacts"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'contacts' key in output, got %v", out)
	}
	if contacts["status"] != "error" {
		t.Errorf("expected status=error, got %v", contacts["status"])
	}
	if contacts["count"] != 0 {
		t.Errorf("expected count=0, got %v", contacts["count"])
	}
}

func TestCFSearchContact_CustomResultVar(t *testing.T) {
	out, err := execCF(t, "cf_search_contact", map[string]any{
		"email":      "x@y.com",
		"result_var": "results",
	})
	if err != nil {
		t.Fatalf("expected soft error, got: %v", err)
	}
	if _, ok := out["results"]; !ok {
		t.Errorf("expected 'results' key in output, got %v", out)
	}
}

// ── cf_get_tags ─────────────────────────────────────────────────────────────

func TestCFGetTags_NilClient(t *testing.T) {
	out, err := execCF(t, "cf_get_tags", map[string]any{})
	if err != nil {
		t.Fatalf("expected soft error, got hard error: %v", err)
	}
	tags, ok := out["tags"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'tags' key in output, got %v", out)
	}
	if tags["status"] != "error" {
		t.Errorf("expected status=error, got %v", tags["status"])
	}
}

func TestCFGetTags_CustomResultVar(t *testing.T) {
	out, err := execCF(t, "cf_get_tags", map[string]any{"result_var": "my_tags"})
	if err != nil {
		t.Fatalf("expected soft error, got: %v", err)
	}
	if _, ok := out["my_tags"]; !ok {
		t.Errorf("expected 'my_tags' key in output, got %v", out)
	}
}

// ── cf_add_tag ──────────────────────────────────────────────────────────────

func TestCFAddTag_NilClient(t *testing.T) {
	_, err := execCF(t, "cf_add_tag", map[string]any{
		"contact_id": float64(1),
		"tag_ids":    `[123]`,
	})
	if err == nil {
		t.Fatal("expected hard error for nil CF client")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' in error, got: %v", err)
	}
}

// ── cf_remove_tag ───────────────────────────────────────────────────────────

func TestCFRemoveTag_NilClient(t *testing.T) {
	_, err := execCF(t, "cf_remove_tag", map[string]any{
		"contact_id": float64(1),
		"tag_ids":    `[123]`,
	})
	if err == nil {
		t.Fatal("expected hard error for nil CF client")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' in error, got: %v", err)
	}
}

// ── cf_get_all ──────────────────────────────────────────────────────────────

func TestCFGetAll_NilClient(t *testing.T) {
	_, err := execCF(t, "cf_get_all", map[string]any{"table": "contacts"})
	if err == nil {
		t.Fatal("expected hard error for nil CF client")
	}
	if !strings.Contains(err.Error(), "CF_API_KEY") {
		t.Errorf("expected CF_API_KEY in error, got: %v", err)
	}
}

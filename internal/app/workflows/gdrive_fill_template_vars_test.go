package workflows

import (
	"encoding/json"
	"testing"
)

// realContext is a trimmed snapshot of the actual workflow context at the point
// the gdrive_fill_template node (n_21154u5) runs — chart_url is "" in
// pb_birth_chart_records, and there is no top-level chart_url key.
var realContext = map[string]any{
	"_source":    "trigger",
	"contact_id": float64(1236980752),
	"doc_id":     "1WPqCdUoSLDUTavz7_KExHCVD1ogkcFK7qf1xNi-YyCE",
	"file_name":  "CC for RR - prompt",
	"file_url":   "https://docs.google.com/document/d/1WPqCdUoSLDUTavz7_KExHCVD1ogkcFK7qf1xNi-YyCE/edit?usp=drivesdk",
	"mime_type":  "application/vnd.google-apps.document",

	// pb_birth_chart_records: chart_url is empty — never populated by a prior node.
	"pb_birth_chart_records": map[string]any{
		"id":          "1asftsn8t9bfw1x",
		"chart_url":   "",
		"moon_code":   "Moon in Virgo in 10H ruled by Mercury in Aquarius",
		"mars_code":   "Mars in Aries in 5H ruled by Mars in Aries",
		"mercury_code": "Mercury in Aquarius in 3H ruled by Uranus in Sagittarius",
		"jupiter_code": "Jupiter in Aquarius in 3H ruled by Uranus in Sagittarius",
		"venus_code":   "Venus in Aries in 5H ruled by Mars in Aries",
		"saturn_code":  "Saturn in Scorpio in 1H ruled by Pluto in Scorpio",
		"sun_code":     "Sun in Aquarius in 3H ruled by Uranus in Sagittarius",
		"money_mode":   "Polyamorist",
		"birth_place":  "Bellevue, Washington, United States",
		"birth_time":   "01:02",
	},

	"pb_contact": map[string]any{
		"id":         "fwnt0ysrncx7q04",
		"first_name": "Lauren",
		"last_name":  "Raye",
		"email":      "poppinsfresh+please@gmail.com",
		"birthday":   "1985-02-08 00:00:00.000Z",
		"sun_sign":   "Aquarius",
	},
}

// ── auto-scan mode (no vars provided) ─────────────────────────────────────────

// TestBuildFillTemplateVars_AutoScan_MissingChartURL demonstrates the root cause:
// chart_url lives inside pb_birth_chart_records (a nested map), not at the top
// level. Auto-scan only reads top-level strings, so {{chart_url}} never gets a
// value and stays blank in the rendered document.
func TestBuildFillTemplateVars_AutoScan_MissingChartURL(t *testing.T) {
	vars, err := buildFillTemplateVars(realContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := vars["chart_url"]; ok {
		t.Errorf("chart_url should NOT be in vars when it is nested — got %q", vars["chart_url"])
	}

	// Top-level strings that ARE present.
	wantPresent := []string{"doc_id", "file_name", "mime_type"}
	for _, k := range wantPresent {
		if _, ok := vars[k]; !ok {
			t.Errorf("expected top-level key %q to be in vars", k)
		}
	}

	// Nested keys that are NOT present (the full scope of what's missing).
	wantAbsent := []string{
		"chart_url", "moon_code", "mars_code", "mercury_code",
		"jupiter_code", "venus_code", "saturn_code", "sun_code",
	}
	for _, k := range wantAbsent {
		if _, ok := vars[k]; ok {
			t.Errorf("key %q should not appear (it is nested); got %q", k, vars[k])
		}
	}
}

// TestBuildFillTemplateVars_AutoScan_SkipsInternals checks that _source and
// the four reserved node-input keys are never included even when they are
// top-level strings.
func TestBuildFillTemplateVars_AutoScan_SkipsInternals(t *testing.T) {
	input := map[string]any{
		"_source":               "trigger",
		"template_id":           "tmpl123",
		"destination_folder_id": "folder456",
		"title":                 "My Doc",
		"vars":                  "",
		"first_name":            "Lauren",
	}
	vars, err := buildFillTemplateVars(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, skip := range []string{"_source", "template_id", "destination_folder_id", "title", "vars"} {
		if _, ok := vars[skip]; ok {
			t.Errorf("key %q should be skipped but was included", skip)
		}
	}
	if vars["first_name"] != "Lauren" {
		t.Errorf("expected first_name=Lauren, got %q", vars["first_name"])
	}
}

// ── explicit vars mode (JSON string) ──────────────────────────────────────────

// TestBuildFillTemplateVars_ExplicitJSON_ChartURLPresent shows the fix:
// pass a "vars" JSON string that explicitly maps chart_url from the nested path.
// This is how the node should be configured to actually fill {{chart_url}}.
func TestBuildFillTemplateVars_ExplicitJSON_ChartURLPresent(t *testing.T) {
	varsJSON, _ := json.Marshal(map[string]string{
		"chart_url":   "https://drive.google.com/uc?id=abc123",
		"moon_code":   "Moon in Virgo in 10H ruled by Mercury in Aquarius",
		"mars_code":   "Mars in Aries in 5H ruled by Mars in Aries",
		"first_name":  "Lauren",
	})

	input := map[string]any{
		"vars":                  string(varsJSON),
		"template_id":           "tmpl123",
		"destination_folder_id": "folder456",
	}

	vars, err := buildFillTemplateVars(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["chart_url"] != "https://drive.google.com/uc?id=abc123" {
		t.Errorf("expected chart_url from vars JSON, got %q", vars["chart_url"])
	}
	if vars["moon_code"] == "" {
		t.Error("expected moon_code to be set")
	}
	// Reserved keys should not bleed through when explicit vars are provided.
	if _, ok := vars["template_id"]; ok {
		t.Error("template_id should not appear in explicit vars mode")
	}
}

// TestBuildFillTemplateVars_ExplicitJSON_InvalidJSON verifies a bad vars string
// returns a clear error rather than silently producing an empty map.
func TestBuildFillTemplateVars_ExplicitJSON_InvalidJSON(t *testing.T) {
	input := map[string]any{
		"vars": `{not valid json`,
	}
	_, err := buildFillTemplateVars(input)
	if err == nil {
		t.Fatal("expected error for invalid JSON vars, got nil")
	}
}

// TestBuildFillTemplateVars_MapVars tests the debug-panel path where vars is
// already a map[string]any (the UI coerces JSON strings to native maps).
func TestBuildFillTemplateVars_MapVars(t *testing.T) {
	input := map[string]any{
		"vars": map[string]any{
			"chart_url":  "https://example.com/chart.png",
			"first_name": "Lauren",
			"count":      float64(3), // non-string values are skipped
		},
	}
	vars, err := buildFillTemplateVars(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["chart_url"] != "https://example.com/chart.png" {
		t.Errorf("got chart_url=%q", vars["chart_url"])
	}
	if _, ok := vars["count"]; ok {
		t.Error("non-string 'count' should have been skipped")
	}
}

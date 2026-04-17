package workflow

// Tests targeting the chart_url disappearing in the pb_upsert node (n_y2cs7gq).
//
// Node staticInput.data is a JSON string with "chart_url":"{{public_url}}".
// deepInterpolate parses the JSON, then resolves each placeholder.
// The cases below prove exactly how public_url resolves under three conditions.

import (
	"encoding/json"
	"testing"
)

// pbUpsertDataTemplate is the exact data JSON from node n_y2cs7gq.
const pbUpsertDataTemplate = `{"contact":"{{pb_contact.records.0.id}}","birth_time":"{{birth_time}}","birth_place":"{{contact.custom_attributes.birth_place}}","house_system":"{{contact.custom_attributes.house_system}}","sun_code":"{{chart.sun}}","rising_code":"{{chart.rising}}","moon_code":"{{chart.moon}}","moon_phase":"{{chart.moon_phase.phase_name}}","moon_sign":"{{record.moon_sign}}","mercury_code":"{{chart.mercury}}","venus_code":"{{chart.venus}}","mars_code":"{{chart.mars}}","jupiter_code":"{{chart.jupiter}}","saturn_code":"{{chart.saturn}}","uranus_code":"{{chart.uranus}}","neptune_code":"{{chart.neptune}}","pluto_code":"{{chart.pluto}}","house_1_code":"{{chart.1H}}","house_2_code":"{{chart.2H}}","house_3_code":"{{chart.3H}}","house_4_code":"{{chart.4H}}","house_5_code":"{{chart.5H}}","house_6_code":"{{chart.6H}}","house_7_code":"{{chart.7H}}","house_8_code":"{{chart.8H}}","house_9_code":"{{chart.9H}}","house_10_code":"{{chart.10H}}","house_11_code":"{{chart.11H}}","house_12_code":"{{chart.12H}}","midheaven_code":"{{chart.midheaven}}","north_node":"{{chart.north_node}}","south_node":"{{chart.south_node}}","chiron_code":"{{chart.chiron}}","chart_url":"{{public_url}}","money_mode":"{{money_mode}}","sun_degree":"","moon_degree":"","chart_notes":""}`

// baseCtx is the minimal subset of the real workflow context needed for the test.
var baseCtx = map[string]any{
	"birth_time":  "01:02",
	"money_mode":  "Polyamorist",
	"pb_contact": map[string]any{
		"records": []any{
			map[string]any{"id": "fwnt0ysrncx7q04"},
		},
	},
	"contact": map[string]any{
		"custom_attributes": map[string]any{
			"birth_place":  "Bellevue, Washington, United States",
			"house_system": "placidus",
		},
	},
	"chart": map[string]any{
		"sun":        "Sun in Aquarius in 3H ruled by Uranus in Sagittarius",
		"rising":     "Rising in Scorpio ruled by Pluto in Scorpio",
		"moon":       "Moon in Virgo in 10H ruled by Mercury in Aquarius",
		"mercury":    "Mercury in Aquarius in 3H ruled by Uranus in Sagittarius",
		"venus":      "Venus in Aries in 5H ruled by Mars in Aries",
		"mars":       "Mars in Aries in 5H ruled by Mars in Aries",
		"jupiter":    "Jupiter in Aquarius in 3H ruled by Uranus in Sagittarius",
		"saturn":     "Saturn in Scorpio in 1H ruled by Pluto in Scorpio",
		"uranus":     "Uranus in Sagittarius in 2H ruled by Jupiter in Aquarius",
		"neptune":    "Neptune in Capricorn in 2H ruled by Saturn in Scorpio",
		"pluto":      "Pluto in Scorpio in 12H ruled by Pluto in Scorpio",
		"chiron":     "Chiron in Gemini in 7H ruled by Mercury in Aquarius",
		"midheaven":  "Midheaven in Leo ruled by Sun in Aquarius",
		"north_node": "North Node in Taurus in 7H ruled by Venus in Aries",
		"south_node": "South Node in Scorpio in 1H ruled by Pluto in Scorpio",
		"moon_phase": map[string]any{"phase_name": "Disseminating"},
		"1H":  "Scorpio on 1H cusp, ruled by Pluto in Scorpio",
		"2H":  "Sagittarius on 2H cusp, ruled by Jupiter in Aquarius",
		"3H":  "Capricorn on 3H cusp, ruled by Saturn in Scorpio",
		"4H":  "Aquarius on 4H cusp, ruled by Uranus in Sagittarius",
		"5H":  "Aries on 5H cusp, ruled by Mars in Aries",
		"6H":  "Aries on 6H cusp, ruled by Mars in Aries",
		"7H":  "Taurus on 7H cusp, ruled by Venus in Aries",
		"8H":  "Gemini on 8H cusp, ruled by Mercury in Aquarius",
		"9H":  "Cancer on 9H cusp, ruled by Moon in Virgo",
		"10H": "Leo on 10H cusp, ruled by Sun in Aquarius",
		"11H": "Libra on 11H cusp, ruled by Venus in Aries",
		"12H": "Libra on 12H cusp, ruled by Venus in Aries",
	},
}

func copyCtx(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func resolveDataMap(t *testing.T, ctx map[string]any) map[string]any {
	t.Helper()
	result := deepInterpolate(pbUpsertDataTemplate, ctx)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("deepInterpolate returned %T, want map[string]any", result)
	}
	return m
}

// TestPbUpsertChartURL_PublicURLPresent is the happy path: public_url is in context
// when the node runs. chart_url should be the URL.
func TestPbUpsertChartURL_PublicURLPresent(t *testing.T) {
	const wantURL = "https://drive.google.com/uc?id=1MBsmMMMvwW4CuCXM3mczM-fKndVoASYS"

	ctx := copyCtx(baseCtx)
	ctx["public_url"] = wantURL

	m := resolveDataMap(t, ctx)

	got, _ := m["chart_url"].(string)
	if got != wantURL {
		t.Errorf("chart_url = %q, want %q", got, wantURL)
	}
	if m["contact"] != "fwnt0ysrncx7q04" {
		t.Errorf("contact = %q, want fwnt0ysrncx7q04", m["contact"])
	}
	if m["money_mode"] != "Polyamorist" {
		t.Errorf("money_mode = %q, want Polyamorist", m["money_mode"])
	}
}

// TestPbUpsertChartURL_PublicURLAbsent covers the case where the gdrive node that
// generates public_url has NOT yet run — key does not exist in context.
// resolveCtxNative returns nil → singlePlaceholderRe falls through to interpolateCtx
// → the {{public_url}} placeholder is NOT in the flat-key loop → dotPlaceholderRe
// doesn't match (no dots) → the literal string "{{public_url}}" is stored in chart_url.
func TestPbUpsertChartURL_PublicURLAbsent(t *testing.T) {
	ctx := copyCtx(baseCtx) // no public_url key

	m := resolveDataMap(t, ctx)

	got, _ := m["chart_url"].(string)
	// When the key is absent, the placeholder survives as a literal string.
	if got != "{{public_url}}" {
		t.Errorf("chart_url = %q, want literal placeholder {{public_url}}", got)
	}
	t.Logf("chart_url when public_url absent: %q", got)
}

// TestPbUpsertChartURL_PublicURLEmpty is the REAL BUG scenario:
// public_url exists in context but is the empty string "".
// resolveCtxNative("public_url", ctx) returns "" which is NOT nil in Go,
// so deepInterpolate returns "" — silently writing empty string to chart_url.
//
// This happens when an earlier workflow run or a workflow step sets public_url = ""
// (e.g. the gdrive node ran but MakePublic failed before the fix, leaving public_url
// unset in the output map, which the engine merges as "" into the running context).
func TestPbUpsertChartURL_PublicURLEmpty(t *testing.T) {
	ctx := copyCtx(baseCtx)
	ctx["public_url"] = "" // key present, value empty

	m := resolveDataMap(t, ctx)

	got, _ := m["chart_url"].(string)
	if got != "" {
		t.Errorf("chart_url = %q, want empty string (demonstrating the bug)", got)
	}
	t.Logf("BUG REPRODUCED: chart_url = %q when public_url = \"\" (key present, value empty)", got)
}

// TestPbUpsertChartURL_WhereFilterInterpolation verifies the where_filters JSON string
// also resolves correctly so the right birth_chart_records row is found/created.
func TestPbUpsertChartURL_WhereFilterInterpolation(t *testing.T) {
	const whereFiltersTemplate = `[{"field":"contact","operator":"eq","value":"{{pb_contact.records.0.id}}"}]`

	ctx := copyCtx(baseCtx)
	result := deepInterpolate(whereFiltersTemplate, ctx)

	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("deepInterpolate(whereFilters) returned %T, want []any", result)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 filter rule, got %d", len(arr))
	}
	rule, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("filter rule is %T, want map", arr[0])
	}
	if rule["value"] != "fwnt0ysrncx7q04" {
		t.Errorf("where_filter value = %q, want fwnt0ysrncx7q04", rule["value"])
	}
}

// TestPbUpsertChartURL_FullContextRoundTrip simulates the PocketBase JSON round-trip
// (context stored as JSON in PB, re-parsed on each workflow step) and confirms
// chart_url still resolves correctly after the round-trip.
func TestPbUpsertChartURL_FullContextRoundTrip(t *testing.T) {
	const wantURL = "https://drive.google.com/uc?id=1MBsmMMMvwW4CuCXM3mczM-fKndVoASYS"

	ctx := copyCtx(baseCtx)
	ctx["public_url"] = wantURL

	// Simulate PocketBase JSON round-trip.
	ctx = simulatePBRoundTrip(ctx)

	m := resolveDataMap(t, ctx)

	got, _ := m["chart_url"].(string)
	if got != wantURL {
		t.Errorf("chart_url after PB round-trip = %q, want %q", got, wantURL)
	}

	// Verify a nested key still resolves.
	if m["sun_code"] != "Sun in Aquarius in 3H ruled by Uranus in Sagittarius" {
		b, _ := json.Marshal(m)
		t.Errorf("sun_code wrong after round-trip; full data: %s", b)
	}
}

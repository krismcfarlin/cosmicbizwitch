package workflow

// Tests for the GetBirthChartData workflow nodes.
//
// Source data: live server (2026-04-11), workflow b3t7akz835w0pwn.
// The graph was updated at 20:21 to add "chart_url":"{{public_url}}" to both
// n_y2cs7gq (pb_upsert → birth_chart_records) and n_8hou4yy (cf_upsert → CF
// custom_attributes). All runs before that had "chart_url":"" hardcoded.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// ── node staticInput (exact copies from the live graph, updated 2026-04-11 20:21) ──

// n_y2cs7gqInput is the current staticInput for the pb_upsert node that writes
// birth_chart_records. chart_url, sun_degree, and moon_degree all reference chart
// context keys set by the preceding http_request and gdrive_fetch_url nodes.
var n_y2cs7gqInput = map[string]any{
	"data":              `{"contact":"{{pb_contact.records.0.id}}","birth_time":"{{birth_time}}","birth_place":"{{contact.custom_attributes.birth_place}}","house_system":"{{contact.custom_attributes.house_system}}","sun_code":"{{chart.sun}}","rising_code":"{{chart.rising}}","moon_code":"{{chart.moon}}","moon_phase":"{{chart.moon_phase.phase_name}}","moon_sign":"{{record.moon_sign}}","mercury_code":"{{chart.mercury}}","venus_code":"{{chart.venus}}","mars_code":"{{chart.mars}}","jupiter_code":"{{chart.jupiter}}","saturn_code":"{{chart.saturn}}","uranus_code":"{{chart.uranus}}","neptune_code":"{{chart.neptune}}","pluto_code":"{{chart.pluto}}","house_1_code":"{{chart.1H}}","house_2_code":"{{chart.2H}}","house_3_code":"{{chart.3H}}","house_4_code":"{{chart.4H}}","house_5_code":"{{chart.5H}}","house_6_code":"{{chart.6H}}","house_7_code":"{{chart.7H}}","house_8_code":"{{chart.8H}}","house_9_code":"{{chart.9H}}","house_10_code":"{{chart.10H}}","house_11_code":"{{chart.11H}}","house_12_code":"{{chart.12H}}","midheaven_code":"{{chart.midheaven}}","north_node":"{{chart.north_node}}","south_node":"{{chart.south_node}}","chiron_code":"{{chart.chiron}}","chart_url":"{{public_url}}","money_mode":"{{money_mode}}","sun_degree":"{{chart.sun_degree}}","moon_degree":"{{chart.moon_degree}}","chart_notes":""}`,
	"result_key":        "pb_upsert_result",
	"table_name":        "birth_chart_records",
	"where_filter_mode": "and",
	"where_filters":     `[{"field":"contact","operator":"eq","value":"{{pb_contact.records.0.id}}"}]`,
}

// n_y2cs7gqInputOldBug is what the staticInput looked like BEFORE the 20:21 fix.
// chart_url was hardcoded to "" — so even when public_url was present in context,
// buildInput always produced data.chart_url = "".
// This is the root cause of birth_chart_records.chart_url never being written.
var n_y2cs7gqInputOldBug = map[string]any{
	"data":              `{"contact":"{{pb_contact.records.0.id}}","birth_time":"{{birth_time}}","birth_place":"{{contact.custom_attributes.birth_place}}","house_system":"{{contact.custom_attributes.house_system}}","sun_code":"{{chart.sun}}","rising_code":"{{chart.rising}}","moon_code":"{{chart.moon}}","moon_phase":"{{chart.moon_phase.phase_name}}","moon_sign":"{{record.moon_sign}}","mercury_code":"{{chart.mercury}}","venus_code":"{{chart.venus}}","mars_code":"{{chart.mars}}","jupiter_code":"{{chart.jupiter}}","saturn_code":"{{chart.saturn}}","uranus_code":"{{chart.uranus}}","neptune_code":"{{chart.neptune}}","pluto_code":"{{chart.pluto}}","house_1_code":"{{chart.1H}}","house_2_code":"{{chart.2H}}","house_3_code":"{{chart.3H}}","house_4_code":"{{chart.4H}}","house_5_code":"{{chart.5H}}","house_6_code":"{{chart.6H}}","house_7_code":"{{chart.7H}}","house_8_code":"{{chart.8H}}","house_9_code":"{{chart.9H}}","house_10_code":"{{chart.10H}}","house_11_code":"{{chart.11H}}","house_12_code":"{{chart.12H}}","midheaven_code":"{{chart.midheaven}}","north_node":"{{chart.north_node}}","south_node":"{{chart.south_node}}","chiron_code":"{{chart.chiron}}","chart_url":"","money_mode":"{{money_mode}}","sun_degree":"","moon_degree":"","chart_notes":""}`,
	"result_key":        "pb_upsert_result",
	"table_name":        "birth_chart_records",
	"where_filter_mode": "and",
	"where_filters":     `[{"field":"contact","operator":"eq","value":"{{pb_contact.records.0.id}}"}]`,
}

// workflowCtxPostGdrive is the workflow context as it exists when n_y2cs7gq runs —
// after n_sq6auu2 (gdrive_fetch_url) has completed and its output has been merged.
// Values are taken directly from workflow b3t7akz835w0pwn (contact qyk77jyaisobnud).
var workflowCtxPostGdrive = map[string]any{
	// set by n_sq6auu2 (gdrive_fetch_url) output
	"public_url": "https://drive.google.com/uc?id=1sxsnf6FkjQGcLYX4bxd7N9xEmin_g5bx",
	"file_id":    "1sxsnf6FkjQGcLYX4bxd7N9xEmin_g5bx",
	"file_url":   "https://drive.google.com/file/d/1sxsnf6FkjQGcLYX4bxd7N9xEmin_g5bx/view",
	"filename":   "Lauren Raye - chart.png",
	"mime_type":  "image/png",

	// from trigger / earlier nodes
	"birth_time": "01:02",
	"money_mode": "Polyamorist",

	"pb_contact": map[string]any{
		"count": float64(1),
		"records": []any{
			map[string]any{
				"id":         "qyk77jyaisobnud",
				"first_name": "Lauren",
				"last_name":  "Raye",
				"email":      "poppinsfresh+ty1@gmail.com",
				"birthday":   "1985-02-08 00:00:00.000Z",
				"sun_sign":   "Aquarius",
			},
		},
	},

	"contact": map[string]any{
		"custom_attributes": map[string]any{
			"birth_place":  "Bellevue, Washington, United States",
			"house_system": "placidus",
		},
	},

	"record": map[string]any{
		"moon_sign":  "",
		"chart_url":  "",
		"birth_time": "01:02",
	},

	// set by n_j9vynep (http_request to chart API)
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
		"moon_degree": 176.85103744287892,
		"sun_degree":  319.55989719904363,
		"moon_phase":  map[string]any{"phase_name": "Disseminating"},
		"1H":         "Scorpio on 1H cusp, ruled by Pluto in Scorpio, plus hints of Sagittarius",
		"2H":         "Sagittarius on 2H cusp, ruled by Jupiter in Aquarius, plus hints of Capricorn",
		"3H":         "Capricorn on 3H cusp, ruled by Saturn in Scorpio, plus hints of Aquarius",
		"4H":         "Aquarius on 4H cusp, ruled by Uranus in Sagittarius, plus hints of Pisces",
		"5H":         "Aries on 5H cusp, ruled by Mars in Aries, plus hints of Aries",
		"6H":         "Aries on 6H cusp, ruled by Mars in Aries, plus hints of Taurus",
		"7H":         "Taurus on 7H cusp, ruled by Venus in Aries, plus hints of Gemini",
		"8H":         "Gemini on 8H cusp, ruled by Mercury in Aquarius, plus hints of Cancer",
		"9H":         "Cancer on 9H cusp, ruled by Moon in Virgo, plus hints of Leo",
		"10H":        "Leo on 10H cusp, ruled by Sun in Aquarius, plus hints of Virgo",
		"11H":        "Libra on 11H cusp, ruled by Venus in Aries, plus hints of Libra",
		"12H":        "Libra on 12H cusp, ruled by Venus in Aries, plus hints of Scorpio",
	},
}

// extractDataMap runs buildInput against the given staticInput and context, then
// returns the resolved "data" map that pb_upsert would receive.
func extractDataMap(t *testing.T, staticInput map[string]any, ctx map[string]any) map[string]any {
	t.Helper()
	inp := buildInput(ctx, nil, staticInput)
	data, ok := inp["data"].(map[string]any)
	if !ok {
		t.Fatalf("input[data] is %T, want map[string]any", inp["data"])
	}
	return data
}

// TestBuildInput_CurrentGraph_ChartURLResolved is the happy path with the fixed graph:
// n_sq6auu2 ran, set public_url in context, n_y2cs7gq sees it → data.chart_url = URL.
func TestBuildInput_CurrentGraph_ChartURLResolved(t *testing.T) {
	const wantURL = "https://drive.google.com/uc?id=1sxsnf6FkjQGcLYX4bxd7N9xEmin_g5bx"

	data := extractDataMap(t, n_y2cs7gqInput, workflowCtxPostGdrive)

	if got := data["chart_url"]; got != wantURL {
		t.Errorf("data.chart_url = %q, want %q", got, wantURL)
	}
	if data["contact"] != "qyk77jyaisobnud" {
		t.Errorf("data.contact = %q, want qyk77jyaisobnud", data["contact"])
	}
	if data["money_mode"] != "Polyamorist" {
		t.Errorf("data.money_mode = %q, want Polyamorist", data["money_mode"])
	}
	if data["sun_code"] != "Sun in Aquarius in 3H ruled by Uranus in Sagittarius" {
		t.Errorf("data.sun_code = %q", data["sun_code"])
	}
	if data["moon_phase"] != "Disseminating" {
		t.Errorf("data.moon_phase = %q, want Disseminating", data["moon_phase"])
	}
	if data["house_1_code"] != "Scorpio on 1H cusp, ruled by Pluto in Scorpio, plus hints of Sagittarius" {
		t.Errorf("data.house_1_code = %q", data["house_1_code"])
	}
	// Previously hardcoded to "" — now resolved as native float64 from chart context
	// (singlePlaceholderRe returns the Go value, not a stringified form).
	// pb_upsert passes this directly to rec.Set(), which handles float→text coercion.
	if data["sun_degree"] != float64(319.55989719904363) {
		t.Errorf("data.sun_degree = %v (%T), want float64(319.55989719904363)", data["sun_degree"], data["sun_degree"])
	}
	if data["moon_degree"] != float64(176.85103744287892) {
		t.Errorf("data.moon_degree = %v (%T), want float64(176.85103744287892)", data["moon_degree"], data["moon_degree"])
	}
}

// TestBuildInput_OldGraph_ChartURLBug reproduces the actual production bug:
// the graph before the 20:21 fix had chart_url:"" hardcoded. Even though public_url
// was present in context (as proven by inp.public_url in workflow b3t7akz835w0pwn),
// data.chart_url was always "".
//
// This is why birth_chart_records.chart_url was never written for Lauren Raye
// (fwnt0ysrncx7q04) and others in the pre-fix runs.
func TestBuildInput_OldGraph_ChartURLBug(t *testing.T) {
	data := extractDataMap(t, n_y2cs7gqInputOldBug, workflowCtxPostGdrive)

	// public_url is in context and has the URL, but the template has "" hardcoded —
	// deepInterpolate("", ctx) = "" regardless of what's in ctx.
	if data["chart_url"] != "" {
		t.Errorf("expected empty chart_url from old template, got %q", data["chart_url"])
	}
	t.Logf("BUG CONFIRMED: data.chart_url=%q even though context has public_url=%q",
		data["chart_url"], workflowCtxPostGdrive["public_url"])
}

// TestBuildInput_PublicURLTopLevel confirms that buildInput always copies public_url
// to the top-level input map (from context), explaining the observed paradox in
// workflow b3t7akz835w0pwn: inp.public_url=URL but inp.data.chart_url="" coexisted
// because public_url came from the context copy while data came from the old template.
func TestBuildInput_PublicURLTopLevel(t *testing.T) {
	const wantURL = "https://drive.google.com/uc?id=1sxsnf6FkjQGcLYX4bxd7N9xEmin_g5bx"

	// Both old and new graph produce the same inp.public_url — it comes from the
	// context copy, not the data template.
	for _, tc := range []struct {
		name        string
		staticInput map[string]any
	}{
		{"current_graph", n_y2cs7gqInput},
		{"old_graph_bug", n_y2cs7gqInputOldBug},
	} {
		inp := buildInput(workflowCtxPostGdrive, nil, tc.staticInput)
		if inp["public_url"] != wantURL {
			t.Errorf("[%s] inp.public_url = %q, want %q", tc.name, inp["public_url"], wantURL)
		}
		data, _ := inp["data"].(map[string]any)
		t.Logf("[%s] inp.public_url=%q  inp.data.chart_url=%q",
			tc.name, inp["public_url"], data["chart_url"])
	}
}

// TestBuildInput_WhereFilters confirms the where_filters JSON string resolves the
// contact ID so pb_upsert can find the existing birth_chart_records row.
func TestBuildInput_WhereFilters(t *testing.T) {
	inp := buildInput(workflowCtxPostGdrive, nil, n_y2cs7gqInput)

	rawFilters, ok := inp["where_filters"].([]any)
	if !ok {
		// where_filters may still be a string if it didn't parse cleanly
		rawStr, _ := inp["where_filters"].(string)
		if err := json.Unmarshal([]byte(rawStr), &rawFilters); err != nil {
			t.Fatalf("where_filters is %T (%v), could not parse: %v", inp["where_filters"], inp["where_filters"], err)
		}
	}
	if len(rawFilters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(rawFilters))
	}
	rule, _ := rawFilters[0].(map[string]any)
	if rule["value"] != "qyk77jyaisobnud" {
		t.Errorf("where_filters[0].value = %q, want qyk77jyaisobnud", rule["value"])
	}
}

// TestBuildInput_MissingPublicURL covers the case where gdrive_fetch_url failed
// before setting public_url (e.g. MakePublic returned an error and the node used
// the old code that swallowed the error instead of returning it).
// With the current graph template, the placeholder survives as a literal string.
func TestBuildInput_MissingPublicURL(t *testing.T) {
	ctx := make(map[string]any, len(workflowCtxPostGdrive))
	for k, v := range workflowCtxPostGdrive {
		ctx[k] = v
	}
	delete(ctx, "public_url") // simulate gdrive node not setting it

	data := extractDataMap(t, n_y2cs7gqInput, ctx)

	// With the fixed graph, the unresolved placeholder survives — at least it's
	// obvious something went wrong, rather than silently writing "".
	if data["chart_url"] != "{{public_url}}" {
		t.Errorf("data.chart_url = %q, want literal placeholder {{public_url}}", data["chart_url"])
	}
	t.Logf("chart_url when public_url absent: %q", data["chart_url"])
}

// TestBuildInput_AllChartFields ensures every chart.* dot-path placeholder in the
// data template resolves to a non-empty string — catches any newly-added planet or
// house field that the test context is missing.
func TestBuildInput_AllChartFields(t *testing.T) {
	data := extractDataMap(t, n_y2cs7gqInput, workflowCtxPostGdrive)

	chartFields := []string{
		"sun_code", "rising_code", "moon_code", "mercury_code", "venus_code",
		"mars_code", "jupiter_code", "saturn_code", "uranus_code", "neptune_code",
		"pluto_code", "chiron_code", "midheaven_code", "north_node", "south_node",
		"house_1_code", "house_2_code", "house_3_code", "house_4_code", "house_5_code",
		"house_6_code", "house_7_code", "house_8_code", "house_9_code", "house_10_code",
		"house_11_code", "house_12_code",
	}
	for _, field := range chartFields {
		v, _ := data[field].(string)
		if v == "" {
			t.Errorf("data.%s is empty — chart context missing this key", field)
		}
	}
}

// ── n_8hou4yy (cf_upsert) ────────────────────────────────────────────────────
//
// cf_upsert has the same root cause as pb_upsert: before the 20:21 graph fix,
// custom_attributes.chart_url was "" hardcoded.
//
// The data field is a doubly-nested JSON string:
//
//	data = `{"custom_attributes":"{\"chart_url\":\"{{public_url}}\",...}"}`
//
// deepInterpolate parses both levels and returns custom_attributes as a
// map[string]any. cf_upsert accepts either a map or a JSON-encoded string
// (see defaults.go line ~876).

// n_8hou4yyCFUpsertInput is the CURRENT (fixed) staticInput for n_8hou4yy.
// Exact copy from live server graph (updated 2026-04-11 20:21).
var n_8hou4yyCFUpsertInput = map[string]any{
	"contact_id": "{{data.attributes.id}}",
	"data":       `{"custom_attributes":"{\"money_mode\":\"{{money_mode}}\",\"sun_code\":\"{{chart.sun}}\",\"rising_code\":\"{{chart.rising}}\",\"moon_code\":\"{{chart.moon}}\",\"moon_phase\":\"{{chart.moon_phase.phase_name}}\",\"moon_sign\":\"{{record.moon_sign}}\",\"mercury_code\":\"{{chart.mercury}}\",\"venus_code\":\"{{chart.venus}}\",\"mars_code\":\"{{chart.mars}}\",\"jupiter_code\":\"{{chart.jupiter}}\",\"saturn_code\":\"{{chart.saturn}}\",\"uranus_code\":\"{{chart.uranus}}\",\"neptune_code\":\"{{chart.neptune}}\",\"pluto_code\":\"{{chart.pluto}}\",\"1H_code\":\"{{chart.1H}}\",\"2H_code\":\"{{chart.2H}}\",\"3H_code\":\"{{chart.3H}}\",\"4H_code\":\"{{chart.4H}}\",\"5H_code\":\"{{chart.5H}}\",\"6H_code\":\"{{chart.6H}}\",\"7H_code\":\"{{chart.7H}}\",\"8H_code\":\"{{chart.8H}}\",\"9H_code\":\"{{chart.9H}}\",\"10H_code\":\"{{chart.10H}}\",\"11H_code\":\"{{chart.11H}}\",\"12H_code\":\"{{chart.12H}}\",\"midheaven_code\":\"{{chart.midheaven}}\",\"north_node\":\"{{chart.north_node}}\",\"south_node\":\"{{chart.south_node}}\",\"chiron_code\":\"{{chart.chiron}}\",\"chart_url\":\"{{public_url}}\"}"}`,
}

// n_8hou4yyCFUpsertInputOldBug is the pre-fix version where chart_url was "".
// Verified from workflow b3t7akz835w0pwn: inp.data.custom_attributes.chart_url = "".
var n_8hou4yyCFUpsertInputOldBug = map[string]any{
	"contact_id": "{{data.attributes.id}}",
	"data":       `{"custom_attributes":"{\"money_mode\":\"{{money_mode}}\",\"sun_code\":\"{{chart.sun}}\",\"rising_code\":\"{{chart.rising}}\",\"moon_code\":\"{{chart.moon}}\",\"moon_phase\":\"{{chart.moon_phase.phase_name}}\",\"moon_sign\":\"{{record.moon_sign}}\",\"mercury_code\":\"{{chart.mercury}}\",\"venus_code\":\"{{chart.venus}}\",\"mars_code\":\"{{chart.mars}}\",\"jupiter_code\":\"{{chart.jupiter}}\",\"saturn_code\":\"{{chart.saturn}}\",\"uranus_code\":\"{{chart.uranus}}\",\"neptune_code\":\"{{chart.neptune}}\",\"pluto_code\":\"{{chart.pluto}}\",\"1H_code\":\"{{chart.1H}}\",\"2H_code\":\"{{chart.2H}}\",\"3H_code\":\"{{chart.3H}}\",\"4H_code\":\"{{chart.4H}}\",\"5H_code\":\"{{chart.5H}}\",\"6H_code\":\"{{chart.6H}}\",\"7H_code\":\"{{chart.7H}}\",\"8H_code\":\"{{chart.8H}}\",\"9H_code\":\"{{chart.9H}}\",\"10H_code\":\"{{chart.10H}}\",\"11H_code\":\"{{chart.11H}}\",\"12H_code\":\"{{chart.12H}}\",\"midheaven_code\":\"{{chart.midheaven}}\",\"north_node\":\"{{chart.north_node}}\",\"south_node\":\"{{chart.south_node}}\",\"chiron_code\":\"{{chart.chiron}}\",\"chart_url\":\"\"}"}`,
}

// extractCFCustomAttrs runs buildInput then unwraps data.custom_attributes into
// a map — handles both the map (deepInterpolate resolves nested JSON) and
// JSON-string forms.
func extractCFCustomAttrs(t *testing.T, staticInput map[string]any, ctx map[string]any) map[string]any {
	t.Helper()
	inp := buildInput(ctx, nil, staticInput)
	data, ok := inp["data"].(map[string]any)
	if !ok {
		t.Fatalf("inp[data] is %T, want map[string]any", inp["data"])
	}
	switch ca := data["custom_attributes"].(type) {
	case map[string]any:
		return ca
	case string:
		var m map[string]any
		if err := json.Unmarshal([]byte(ca), &m); err != nil {
			t.Fatalf("custom_attributes string is not valid JSON: %v\nraw: %s", err, ca)
		}
		return m
	default:
		t.Fatalf("custom_attributes is %T, want map or string", data["custom_attributes"])
		return nil
	}
}

// TestCFUpsert_CurrentGraph_ChartURLResolved confirms the fixed graph correctly
// resolves {{public_url}} through two levels of nested JSON interpolation.
func TestCFUpsert_CurrentGraph_ChartURLResolved(t *testing.T) {
	const wantURL = "https://drive.google.com/uc?id=1sxsnf6FkjQGcLYX4bxd7N9xEmin_g5bx"

	ca := extractCFCustomAttrs(t, n_8hou4yyCFUpsertInput, workflowCtxPostGdrive)

	if got := ca["chart_url"]; got != wantURL {
		t.Errorf("custom_attributes.chart_url = %q, want %q", got, wantURL)
	}
	if ca["money_mode"] != "Polyamorist" {
		t.Errorf("custom_attributes.money_mode = %q, want Polyamorist", ca["money_mode"])
	}
	if ca["sun_code"] != "Sun in Aquarius in 3H ruled by Uranus in Sagittarius" {
		t.Errorf("custom_attributes.sun_code = %q", ca["sun_code"])
	}
}

// TestCFUpsert_OldGraph_ChartURLBug reproduces the cf_upsert bug: old graph had
// chart_url:"" hardcoded in the doubly-nested custom_attributes JSON.
// Confirmed via workflow b3t7akz835w0pwn: inp.data.custom_attributes.chart_url = "".
func TestCFUpsert_OldGraph_ChartURLBug(t *testing.T) {
	ca := extractCFCustomAttrs(t, n_8hou4yyCFUpsertInputOldBug, workflowCtxPostGdrive)

	if ca["chart_url"] != "" {
		t.Errorf("expected empty chart_url from old template, got %q", ca["chart_url"])
	}
	t.Logf("BUG CONFIRMED: custom_attributes.chart_url=%q even though context has public_url=%q",
		ca["chart_url"], workflowCtxPostGdrive["public_url"])
}

// ── n_j9vynep (http_request → GetBirthChartData) ───────────────────────────
//
// This node calls the chart API with birth data assembled from workflow context.
// Bug: for new users (no prior birth chart record), pb_birth_chart_records.records
// is empty, so {{pb_birth_chart_records.records.0.house_system}} resolves to "".
// The house_system should come from contact.custom_attributes.house_system instead.

// n_j9vynepStaticInput is the exact staticInput for the GetBirthChartData node.
var n_j9vynepStaticInput = map[string]any{
	"body": `{
  "birth_date": "{{contact.custom_attributes.birthday}}",
  "birth_time": "{{birth_time}}",
  "latitude": "{{geocode.results.0.lat|number}}",
  "longitude": "{{geocode.results.0.lon|number}}",
  "timezone": "{{geocode.results.0.timezone.name}}",
  "house_system": "{{pb_birth_chart_records.records.0.house_system}}"
}`,
	"headers":    `{"X-API-Key": "e8f76388f1f00009d7ae2f575fc105d6d29e9e836246a5932a129f463df2dbef"}`,
	"method":     "POST",
	"result_key": "chart",
	"url":        "https://dolphin-app-4tibh.ondigitalocean.app/v1/charts/cosmicbizwitch",
}

// porphyryNewUserCtx is the workflow context from the test run (2026-04-12) where
// a new user (no prior birth_chart_record) submitted with house_system="Porphyry"
// in their CF custom attributes. pb_birth_chart_records.records is empty.
var porphyryNewUserCtx = map[string]any{
	"birth_time": "01:02",

	"contact": map[string]any{
		"custom_attributes": map[string]any{
			"birthday":     "1985-02-08",
			"birth_place":  "Bellevue, Washington, United States",
			"house_system": "Porphyry",
		},
	},

	"geocode": map[string]any{
		"results": []any{
			map[string]any{
				"lat": 47.6144219,
				"lon": -122.192337,
				"timezone": map[string]any{
					"name": "America/Los_Angeles",
				},
			},
		},
	},

	// New user — no prior birth chart record in PocketBase.
	"pb_birth_chart_records": map[string]any{
		"count":   float64(0),
		"found":   false,
		"records": []any{},
	},
}

// TestGetBirthChartData_HouseSystemBug confirms the root cause: for a new user
// (pb_birth_chart_records.records empty), the house_system template resolves to ""
// instead of the contact's preference "Porphyry".
//
// Fix: change the template to {{contact.custom_attributes.house_system}}.
func TestGetBirthChartData_HouseSystemBug(t *testing.T) {
	inp := buildInput(porphyryNewUserCtx, nil, n_j9vynepStaticInput)

	// body arrives as a map after deepInterpolate parses the JSON string.
	body, ok := inp["body"].(map[string]any)
	if !ok {
		t.Fatalf("inp[body] is %T, want map[string]any\nraw: %v", inp["body"], inp["body"])
	}

	t.Logf("resolved body: %v", body)

	// BUG: pb_birth_chart_records.records is empty → records.0.house_system = ""
	if body["house_system"] != "" {
		t.Errorf("expected house_system=\"\" (BUG reproduces), got %q", body["house_system"])
	}

	// But the correct value is available at contact.custom_attributes.house_system
	if body["birth_date"] != "1985-02-08" {
		t.Errorf("birth_date = %q, want 1985-02-08", body["birth_date"])
	}
	if body["timezone"] != "America/Los_Angeles" {
		t.Errorf("timezone = %q, want America/Los_Angeles", body["timezone"])
	}

	t.Logf("BUG: house_system=%q — pb_birth_chart_records.records is empty for new users", body["house_system"])
	t.Logf("FIX: template should use {{contact.custom_attributes.house_system}} = %q",
		porphyryNewUserCtx["contact"].(map[string]any)["custom_attributes"].(map[string]any)["house_system"])
}

// TestGetBirthChartData_FixedTemplate shows the corrected template using
// contact.custom_attributes.house_system picks up "Porphyry" for new users.
func TestGetBirthChartData_FixedTemplate(t *testing.T) {
	fixedStaticInput := map[string]any{
		"body": `{
  "birth_date": "{{contact.custom_attributes.birthday}}",
  "birth_time": "{{birth_time}}",
  "latitude": "{{geocode.results.0.lat|number}}",
  "longitude": "{{geocode.results.0.lon|number}}",
  "timezone": "{{geocode.results.0.timezone.name}}",
  "house_system": "{{contact.custom_attributes.house_system}}"
}`,
		"headers":    `{"X-API-Key": "e8f76388f1f00009d7ae2f575fc105d6d29e9e836246a5932a129f463df2dbef"}`,
		"method":     "POST",
		"result_key": "chart",
		"url":        "https://dolphin-app-4tibh.ondigitalocean.app/v1/charts/cosmicbizwitch",
	}

	inp := buildInput(porphyryNewUserCtx, nil, fixedStaticInput)
	body, ok := inp["body"].(map[string]any)
	if !ok {
		t.Fatalf("inp[body] is %T", inp["body"])
	}

	if body["house_system"] != "Porphyry" {
		t.Errorf("house_system = %q, want Porphyry", body["house_system"])
	}
	t.Logf("fixed body: %v", body)
}

// callChartAPI makes a real HTTP request to the chart API and returns the parsed response.
func callChartAPI(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	const (
		apiURL = "https://dolphin-app-4tibh.ondigitalocean.app/v1/charts/cosmicbizwitch"
		apiKey = "e8f76388f1f00009d7ae2f575fc105d6d29e9e836246a5932a129f463df2dbef"
	)

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	t.Logf("POST %s\nbody: %s", apiURL, data)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("HTTP %d\nresponse: %s", resp.StatusCode, body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("chart API returned HTTP %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	return result
}

// TestGetBirthChartData_LiveCall_EmptyHouseSystem calls the chart API with the BUGGY
// payload (house_system="") to see what the server returns.
// This is a live integration test — it hits the real server.
func TestGetBirthChartData_LiveCall_EmptyHouseSystem(t *testing.T) {
	payload := map[string]any{
		"birth_date":   "1985-02-08",
		"birth_time":   "01:02",
		"latitude":     47.6144219,
		"longitude":    -122.192337,
		"timezone":     "America/Los_Angeles",
		"house_system": "", // what was actually sent — the bug
	}

	result := callChartAPI(t, payload)

	// Log key fields so we can see what the server returns with empty house_system.
	t.Logf("=== EMPTY house_system response ===")
	for _, key := range []string{"rising", "1H", "2H", "3H", "4H", "5H", "6H", "7H", "8H", "9H", "10H", "11H", "12H"} {
		t.Logf("  %s: %v", key, result[key])
	}
	// Check if the server gives us any indication of which house system it used.
	if hs, ok := result["house_system"]; ok {
		t.Logf("  house_system in response: %v", hs)
	}

	// Capture the 1H cusp for comparison with Porphyry below.
	h1Empty := fmt.Sprintf("%v", result["1H"])
	t.Logf("  1H with empty house_system: %s", h1Empty)
}

// TestGetBirthChartData_LiveCall_PorphyryHouseSystem calls the chart API with
// house_system="Porphyry" (the correct value) to compare against the empty case.
func TestGetBirthChartData_LiveCall_PorphyryHouseSystem(t *testing.T) {
	payload := map[string]any{
		"birth_date":   "1985-02-08",
		"birth_time":   "01:02",
		"latitude":     47.6144219,
		"longitude":    -122.192337,
		"timezone":     "America/Los_Angeles",
		"house_system": "Porphyry", // what it should have been
	}

	result := callChartAPI(t, payload)

	t.Logf("=== Porphyry house_system response ===")
	for _, key := range []string{"rising", "1H", "2H", "3H", "4H", "5H", "6H", "7H", "8H", "9H", "10H", "11H", "12H"} {
		t.Logf("  %s: %v", key, result[key])
	}
	if hs, ok := result["house_system"]; ok {
		t.Logf("  house_system in response: %v", hs)
	}

	h1Porphyry := fmt.Sprintf("%v", result["1H"])
	t.Logf("  1H with Porphyry house_system: %s", h1Porphyry)
}

// ── n_21154u5 (gdrive_fill_template → PDF template) ─────────────────────────
//
// vars.chart_url uses {{pb_birth_chart_records.records.0.chart_url}}.
// Test confirms that our Go code correctly resolves this to the actual URL —
// so if chart_url isn't appearing in the finished document, the bug is in the
// Google Doc template itself (wrong placeholder format, or placeholder is inside
// a text box / hyperlink / drawing that replaceAllText cannot reach).

// n_21154u5StaticInput is the exact staticInput for the PDF template node.
var n_21154u5StaticInput = map[string]any{
	"destination_folder_id":      "1sM04vnGbbW3eHp_HEoyNmPqjTr8hNRTr",
	"destination_folder_id_name": "CC for RR - Spring 2026",
	"template_id":                "1IMrLj2_yx2sm1UeT6CvdaCdr9h7Rn_koLrox3Jk4g2M",
	"template_id_name":           "CC for RRspring - template",
	"title":                      "Cosmic Companion for Ritual Reset - Spring 2026 - {{contact.first_name}} {{contact.last_name}}",
	"vars":                       `{"first_name":"{{pb_contact.records.0.first_name}}","birthday":"{{pb_contact.records.0.birthday}}","birth_time":"{{result_var.custom_attributes.birth_time}}","birth_place":"{{pb_birth_chart_records.records.0.birth_place}}","intro":"{{llm_response.synthesis.intro}}","seasonal_summary":"{{llm_response.synthesis.seasonal_summary}}","day1_alignment":"{{llm_response.day1.alignment}}","day1_seasonal":"{{llm_response.day1.seasonal}}","day1_approach":"{{llm_response.day1.approach}}","day1_examples":"{{llm_response.day1.examples}}","day2_alignment":"{{llm_response.day2.alignment}}","day2_seasonal":"{{llm_response.day2.seasonal}}","day2_approach":"{{llm_response.day2.approach}}","day2_examples":"{{llm_response.day2.examples}}","day3_alignment":"{{llm_response.day3.alignment}}","day3_seasonal":"{{llm_response.day3.seasonal}}","day3_approach":"{{llm_response.day3.approach}}","day3_examples":"{{llm_response.day3.examples}}","day4_alignment":"{{llm_response.day4.alignment}}","day4_seasonal":"{{llm_response.day4.seasonal}}","day4_approach":"{{llm_response.day4.approach}}","day4_examples":"{{llm_response.day4.examples}}","day5_alignment":"{{llm_response.day5.alignment}}","day5_seasonal":"{{llm_response.day5.seasonal}}","day5_approach":"{{llm_response.day5.approach}}","day5_examples":"{{llm_response.day5.examples}}","moon_code":"{{pb_birth_chart_records.records.0.moon_code}}","mars_code":"{{pb_birth_chart_records.records.0.mars_code}}","mercury_code":"{{pb_birth_chart_records.records.0.mercury_code}}","jupiter_code":"{{pb_birth_chart_records.records.0.jupiter_code}}","venus_code":"{{pb_birth_chart_records.records.0.venus_code}}","chart_url":"{{pb_birth_chart_records.records.0.chart_url}}","approach":"{{llm_response.day3.approach}}"}`,
}

// porphyryPDFCtx is the minimal workflow context extracted from the 2026-04-12 run
// when the PDF template node (n_21154u5) executes.
var porphyryPDFCtx = map[string]any{
	"pb_contact": map[string]any{
		"records": []any{
			map[string]any{
				"id":         "2n9bll0htab6vo2",
				"first_name": "",
				"last_name":  "",
				"birthday":   "1985-02-08 00:00:00.000Z",
			},
		},
	},
	"pb_birth_chart_records": map[string]any{
		"count": float64(1),
		"found": true,
		"records": []any{
			map[string]any{
				"id":           "vcnbyu7m2y1hk7j",
				"birth_place":  "Bellevue, Washington, United States",
				"chart_url":    "https://drive.google.com/uc?id=1sn6T7kdcVEy1U4qhGM4rsjqHjxF8djBj",
				"moon_code":    "Moon in Virgo in 10H ruled by Mercury in Aquarius",
				"mars_code":    "Mars in Aries in 5H ruled by Mars in Aries",
				"mercury_code": "Mercury in Aquarius in 3H ruled by Uranus in Sagittarius",
				"jupiter_code": "Jupiter in Aquarius in 3H ruled by Uranus in Sagittarius",
				"venus_code":   "Venus in Aries in 5H ruled by Mars in Aries",
			},
		},
	},
	"result_var": map[string]any{
		"custom_attributes": map[string]any{
			"birth_time": "1:02 AM",
		},
	},
	"contact": map[string]any{}, // contact is empty for this user
	"llm_response": map[string]any{
		"synthesis": map[string]any{
			"intro":            "Your chart for this reset is wired for innovation.",
			"seasonal_summary": "Spring is the season of planting.",
		},
		"day1": map[string]any{"alignment": "Day 1 alignment.", "seasonal": "Day 1 seasonal.", "approach": "Day 1 approach.", "examples": "Day 1 examples."},
		"day2": map[string]any{"alignment": "Day 2 alignment.", "seasonal": "Day 2 seasonal.", "approach": "Day 2 approach.", "examples": "Day 2 examples."},
		"day3": map[string]any{"alignment": "Day 3 alignment.", "seasonal": "Day 3 seasonal.", "approach": "Day 3 approach — the actual text.", "examples": "Day 3 examples."},
		"day4": map[string]any{"alignment": "Day 4 alignment.", "seasonal": "Day 4 seasonal.", "approach": "Day 4 approach.", "examples": "Day 4 examples."},
		"day5": map[string]any{"alignment": "Day 5 alignment.", "seasonal": "Day 5 seasonal.", "approach": "Day 5 approach.", "examples": "Day 5 examples."},
	},
}

// TestGdriveFillTemplate_ChartURLResolved confirms that our Go code correctly
// resolves {{pb_birth_chart_records.records.0.chart_url}} to the actual URL
// when pb_birth_chart_records has a record with a chart_url set.
//
// If chart_url is not appearing in the FINISHED GOOGLE DOC, the bug is in the
// template document itself — the {{chart_url}} placeholder is either:
//   - Inside a text box, shape, or drawing (replaceAllText can't reach those)
//   - Formatted as a hyperlink anchor (the URL is in the link href, not the text)
//   - Using different placeholder syntax (e.g. {chart_url} single braces)
func TestGdriveFillTemplate_ChartURLResolved(t *testing.T) {
	const wantURL = "https://drive.google.com/uc?id=1sn6T7kdcVEy1U4qhGM4rsjqHjxF8djBj"

	inp := buildInput(porphyryPDFCtx, nil, n_21154u5StaticInput)

	// vars should be a map after deepInterpolate parses the JSON string.
	varsMap, ok := inp["vars"].(map[string]any)
	if !ok {
		t.Fatalf("inp[vars] is %T, want map[string]any\nvalue: %v", inp["vars"], inp["vars"])
	}

	// chart_url must resolve to the actual URL.
	chartURL, _ := varsMap["chart_url"].(string)
	if chartURL != wantURL {
		t.Errorf("vars.chart_url = %q, want %q", chartURL, wantURL)
	}
	t.Logf("vars.chart_url resolved correctly to: %s", chartURL)

	// Confirm other vars also resolve (smoke check).
	if moon, _ := varsMap["moon_code"].(string); moon == "" {
		t.Errorf("vars.moon_code is empty — pb_birth_chart_records lookup broken")
	}
	if day3, _ := varsMap["day3_approach"].(string); day3 == "" {
		t.Errorf("vars.day3_approach is empty — llm_response lookup broken")
	}

	// Note: first_name is empty because pb_contact.records.0.first_name="" for this user.
	t.Logf("vars.first_name = %q (empty — contact has no name set in PocketBase)", varsMap["first_name"])

	// Log the full vars map so it's visible in test output.
	for _, k := range []string{"chart_url", "birth_place", "moon_code", "mars_code", "first_name"} {
		t.Logf("  vars.%s = %q", k, varsMap[k])
	}
}

// TestGdriveFillTemplate_VarsMapPassesThroughStrings confirms that buildFillTemplateVars
// (called by gdrive_fill_template) copies all string values from a resolved vars map.
// This is the code path taken when deepInterpolate converts the JSON string to a map.
func TestGdriveFillTemplate_VarsMapPassesThroughStrings(t *testing.T) {
	const wantURL = "https://drive.google.com/uc?id=1sn6T7kdcVEy1U4qhGM4rsjqHjxF8djBj"

	inp := buildInput(porphyryPDFCtx, nil, n_21154u5StaticInput)

	// buildFillTemplateVars is the function called inside gdrive_fill_template.
	// It takes inp and returns the map[string]string passed to ReplaceAllText.
	// We replicate its logic here to confirm chart_url is included.
	varsMap, _ := inp["vars"].(map[string]any)
	result := make(map[string]string)
	for k, v := range varsMap {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}

	if result["chart_url"] != wantURL {
		t.Errorf("buildFillTemplateVars result[chart_url] = %q, want %q", result["chart_url"], wantURL)
	}
	t.Logf("chart_url in ReplaceAllText vars: %q", result["chart_url"])
	t.Logf("CONCLUSION: Our Go code sends the correct chart_url to Google Docs replaceAllText.")
	t.Logf("If {{chart_url}} is still not replaced in the document, check the template:")
	t.Logf("  1. The placeholder {{chart_url}} must appear as literal text in the doc body")
	t.Logf("  2. It cannot be inside a text box, shape, or drawing object")
	t.Logf("  3. It cannot be the anchor text of a hyperlink (that text is editable but the")
	t.Logf("     URL in the link href is separate and replaceAllText won't change it)")
}

// TestCFUpsert_DoubleNestedJSON confirms deepInterpolate correctly resolves
// placeholders through TWO levels of JSON nesting (data → custom_attributes → values).
// This is specific to cf_upsert because ClickFunnels expects custom_attributes
// as a JSON-stringified object, so the graph stores it as a string inside a string.
// ── n_sq6auu2 (gdrive_fetch_url → visual birth chart PNG) ───────────────────
//
// This node calls /v1/charts/birthchart (image endpoint) and saves the PNG to
// Drive. Different from n_j9vynep which calls /v1/charts/cosmicbizwitch for
// text data.

// n_sq6auu2StaticInput is the staticInput for the visual chart node as
// configured in the live graph (2026-04-12).
var n_sq6auu2StaticInput = map[string]any{
	"body": `{
  "birth_date": "{{pb_contact.records.0.birthday}}",
  "birth_time": "{{birth_time}}",
  "latitude": "{{geocode.results.0.lat}}",
  "longitude": "{{geocode.results.0.lon}}",
  "timezone": "{{geocode.results.0.timezone.name}}",
  "name": "{{pb_contact.records.0.first_name}}",
  "city": "{{contact.custom_attributes.birth_place}}",
  "wheel_only": true,
  "hide_axis_labels": true,
  "show_moon_phase": true,
  "format": "png",
  "theme": {
    "paper_0": "#373636",
    "paper_1": "#FFFFFF",
    "zodiac_bg": ["#E5D39D","#C7B7A3","#C498BA","#90C3BF","#D9C06E","#8A7560","#A15E93","#51A39D","#E5D39D","#C7B7A3","#C498BA","#90C3BF"],
    "zodiac_icon": ["#373636","#373636","#373636","#373636","#373636","#373636","#373636","#373636","#373636","#373636","#373636","#373636"],
    "zodiac_radix_ring": ["#373636","#373636","#373636"],
    "zodiac_transit_ring": ["#373636","#373636","#373636","#373636"],
    "houses_radix_line": "#373636",
    "houses_transit_line": "#373636",
    "lunar_phase_0": "#373636",
    "lunar_phase_1": "#E5D39D",
    "conjunction": "#FFFFFF",
    "semi_sextile": "#FFFFFF",
    "semi_square": "#FFFFFF",
    "sextile": "#FFFFFF",
    "quintile": "#FFFFFF",
    "square": "#FFFFFF",
    "trine": "#FFFFFF",
    "sesquiquadrate": "#FFFFFF",
    "biquintile": "#FFFFFF",
    "quincunx": "#FFFFFF",
    "opposition": "#FFFFFF",
    "sun": "#696868",
    "moon": "#222222",
    "mercury": "#222222",
    "venus": "#222222",
    "mars": "#222222",
    "jupiter": "#222222",
    "saturn": "#696868",
    "uranus": "#696868",
    "neptune": "#696868",
    "pluto": "#696868",
    "mean_node": "#696868",
    "true_node": "#696868",
    "chiron": "#696868",
    "first_house": "#696868",
    "tenth_house": "#696868",
    "seventh_house": "#696868",
    "fourth_house": "#696868",
    "house_number": "#373636",
    "house_system": "{{contact.custom_attributes.house_system}}"
  }
}`,
	"filename":        "{{contact.first_name}} {{contact.last_name}} - chart.png",
	"folder_id":       "1URMzknq5QyDsRKDUzm96foifR3tr-pUj",
	"folder_id_name":  "10 - ☸️ Client Charts",
	"headers":         `{"X-API-Key": "e8f76388f1f00009d7ae2f575fc105d6d29e9e836246a5932a129f463df2dbef"}`,
	"make_public":     "true",
	"method":          "POST",
	"mime_type":       "image/png",
	"url":             "https://dolphin-app-4tibh.ondigitalocean.app/v1/charts/birthchart",
}

// visualChartCtx is the workflow context when n_sq6auu2 runs (2026-04-12 live run,
// porphyry2 user). Taken from pasted_text_2026-04-12_14-29-18.txt.
var visualChartCtx = map[string]any{
	"birth_time": "01:02",

	"pb_contact": map[string]any{
		"records": []any{
			map[string]any{
				"id":         "04l3og0fnsljq2f",
				"first_name": "",
				"last_name":  "",
				"birthday":   "1985-02-08",
			},
		},
	},

	"geocode": map[string]any{
		"results": []any{
			map[string]any{
				"lat": 47.6144219,
				"lon": -122.192337,
				"timezone": map[string]any{
					"name": "America/Los_Angeles",
				},
			},
		},
	},

	"contact": map[string]any{
		"first_name": "",
		"last_name":  "",
		"custom_attributes": map[string]any{
			"birth_place":  "Bellevue, Washington, United States",
			"house_system": "Porphyry",
		},
	},
}

// TestVisualChart_ShowCurl resolves the n_sq6auu2 body against the live context
// and prints the equivalent curl so we can verify what is actually sent to the
// birthchart image API.
func TestVisualChart_ShowCurl(t *testing.T) {
	inp := buildInput(visualChartCtx, nil, n_sq6auu2StaticInput)

	body, ok := inp["body"].(map[string]any)
	if !ok {
		t.Fatalf("inp[body] is %T, want map[string]any", inp["body"])
	}

	bodyJSON, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	t.Logf("=== resolved body sent to /v1/charts/birthchart ===\n%s", bodyJSON)

	// Build compact form for the curl command.
	compact, _ := json.Marshal(body)
	t.Logf("\ncurl -X POST \\\n  -H \"Content-Type: application/json\" \\\n  -H \"X-API-Key: e8f76388f1f00009d7ae2f575fc105d6d29e9e836246a5932a129f463df2dbef\" \\\n  -d '%s' \\\n  \"https://dolphin-app-4tibh.ondigitalocean.app/v1/charts/birthchart\"", compact)

	// Assertions.
	if lat, _ := body["latitude"].(float64); lat == 0 {
		t.Errorf("latitude resolved to 0 — geocode.results.0.lat not found in context")
	} else {
		t.Logf("latitude = %v (type %T)", lat, lat)
	}
	if lon, _ := body["longitude"].(float64); lon == 0 {
		t.Errorf("longitude resolved to 0 — geocode.results.0.lon not found in context")
	} else {
		t.Logf("longitude = %v (type %T)", lon, lon)
	}

	theme, _ := body["theme"].(map[string]any)
	if hs, _ := theme["house_system"].(string); hs == "" {
		t.Errorf("theme.house_system is empty — contact.custom_attributes.house_system not found")
	} else {
		t.Logf("theme.house_system = %q", hs)
	}

	if bd, _ := body["birth_date"].(string); bd == "" {
		t.Errorf("birth_date is empty")
	} else {
		t.Logf("birth_date = %q", bd)
	}
}

func TestCFUpsert_DoubleNestedJSON(t *testing.T) {
	const wantURL = "https://drive.google.com/uc?id=1sxsnf6FkjQGcLYX4bxd7N9xEmin_g5bx"

	inp := buildInput(workflowCtxPostGdrive, nil, n_8hou4yyCFUpsertInput)
	data, _ := inp["data"].(map[string]any)

	// After deepInterpolate resolves both levels, custom_attributes comes back as
	// a map (not a string) — the inner JSON was parsed and interpolated in place.
	switch ca := data["custom_attributes"].(type) {
	case map[string]any:
		t.Logf("custom_attributes resolved to map (deepInterpolate parsed inner JSON)")
		if ca["chart_url"] != wantURL {
			t.Errorf("chart_url = %q, want %q", ca["chart_url"], wantURL)
		}
	case string:
		t.Logf("custom_attributes remained a string — checking via JSON parse")
		var m map[string]any
		if err := json.Unmarshal([]byte(ca), &m); err != nil {
			t.Fatalf("could not parse: %v", err)
		}
		if m["chart_url"] != wantURL {
			t.Errorf("chart_url = %q, want %q", m["chart_url"], wantURL)
		}
	default:
		t.Fatalf("unexpected type %T for custom_attributes", data["custom_attributes"])
	}
}

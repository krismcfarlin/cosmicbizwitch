package workflows

import (
	"context"
	"io"
	"log"
	"testing"

	"cosmicbizwitch/pkg/workflow"
)

// newPureLogicEngine builds a minimal Engine with all pure-logic activities registered.
// It uses a nil Store and nil external clients — safe because ExecuteActivity never
// touches the store or external services for these nodes.
func newPureLogicEngine() *workflow.Engine {
	eng := workflow.NewEngine(nil, workflow.EngineConfig{Logger: log.New(io.Discard, "", 0)})
	registerTemplateFill(eng)
	registerTextSubstitute(eng)
	registerAstrologyActivities(eng)
	init_birth_converters(eng)
	// register inline activities that live inside registerActivities
	registerActivities(eng, nil, nil, log.New(io.Discard, "", 0))
	return eng
}

func execActivity(t *testing.T, eng *workflow.Engine, name string, input map[string]any) (map[string]any, error) {
	t.Helper()
	return eng.ExecuteActivity(context.Background(), name, input)
}

// ── echo ──────────────────────────────────────────────────────────────────────

func TestEcho_PassThrough(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "echo", map[string]any{"foo": "bar", "num": float64(42)})
	if err != nil {
		t.Fatalf("echo error: %v", err)
	}
	if out["foo"] != "bar" {
		t.Errorf("echo foo = %v, want bar", out["foo"])
	}
	if out["num"] != float64(42) {
		t.Errorf("echo num = %v, want 42", out["num"])
	}
}

func TestEcho_Empty(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "echo", map[string]any{})
	if err != nil {
		t.Fatalf("echo error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("echo empty input should return empty map, got %v", out)
	}
}

// ── notes ─────────────────────────────────────────────────────────────────────

func TestNotes_AlwaysSkips(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "notes", map[string]any{"note": "this is a note"})
	if err != workflow.ErrSkip {
		t.Errorf("notes should return ErrSkip, got: %v", err)
	}
}

// ── test_data ─────────────────────────────────────────────────────────────────

func TestTestData_ParsesJSON(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "test_data", map[string]any{
		"data": `{"name":"Alice","age":30}`,
	})
	if err != nil {
		t.Fatalf("test_data error: %v", err)
	}
	if out["name"] != "Alice" {
		t.Errorf("test_data name = %v, want Alice", out["name"])
	}
}

func TestTestData_SkipsWhenTriggerSource(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "test_data", map[string]any{
		"_source": "trigger",
		"data":    `{"name":"Alice"}`,
	})
	if err != workflow.ErrSkip {
		t.Errorf("test_data with trigger source should return ErrSkip, got: %v", err)
	}
}

func TestTestData_EmptyData(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "test_data", map[string]any{"data": ""})
	if err != nil {
		t.Fatalf("test_data error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("test_data empty data should return empty map, got %v", out)
	}
}

func TestTestData_InvalidJSON(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "test_data", map[string]any{"data": "not json"})
	if err == nil {
		t.Error("test_data invalid JSON should return error, got nil")
	}
}

// ── cf_next_chunk ─────────────────────────────────────────────────────────────

func TestCFNextChunk_PassThrough(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "cf_next_chunk", map[string]any{
		"cf_has_more": true,
		"cf_cursor":   float64(50),
	})
	if err != nil {
		t.Fatalf("cf_next_chunk error: %v", err)
	}
	if out["cf_has_more"] != true {
		t.Errorf("cf_next_chunk cf_has_more = %v, want true", out["cf_has_more"])
	}
	if out["cf_cursor"] != 50 {
		t.Errorf("cf_next_chunk cf_cursor = %v, want 50", out["cf_cursor"])
	}
}

func TestCFNextChunk_NoMore(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "cf_next_chunk", map[string]any{
		"cf_has_more": false,
		"cf_cursor":   float64(0),
	})
	if err != nil {
		t.Fatalf("cf_next_chunk error: %v", err)
	}
	if out["cf_has_more"] != false {
		t.Errorf("cf_next_chunk cf_has_more = %v, want false", out["cf_has_more"])
	}
}

// ── mapper ────────────────────────────────────────────────────────────────────

func TestMapper_BasicMapping(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "mapper", map[string]any{
		"json": map[string]any{
			"contact": map[string]any{
				"email": "alice@example.com",
				"name":  "Alice",
			},
		},
		"source_key": "json",
		"mappings": []any{
			map[string]any{"from": "contact.email", "to": "email_address"},
			map[string]any{"from": "contact.name", "to": "full_name"},
		},
	})
	if err != nil {
		t.Fatalf("mapper error: %v", err)
	}
	if out["email_address"] != "alice@example.com" {
		t.Errorf("mapper email_address = %v, want alice@example.com", out["email_address"])
	}
	if out["full_name"] != "Alice" {
		t.Errorf("mapper full_name = %v, want Alice", out["full_name"])
	}
}

func TestMapper_MissingMappings(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "mapper", map[string]any{"source_key": "json"})
	if err == nil || err.Error() != "mapper: mappings is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMapper_DefaultSourceKey(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "mapper", map[string]any{
		"json": map[string]any{"x": "val"},
		"mappings": []any{
			map[string]any{"from": "x", "to": "y"},
		},
	})
	if err != nil {
		t.Fatalf("mapper error: %v", err)
	}
	if out["y"] != "val" {
		t.Errorf("mapper default source_key: y = %v, want val", out["y"])
	}
}

func TestMapper_MissingFromOrTo(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "mapper", map[string]any{
		"mappings": []any{
			map[string]any{"from": "x"}, // missing "to"
		},
	})
	if err == nil {
		t.Error("expected error for missing 'to', got nil")
	}
}

// ── template_fill ─────────────────────────────────────────────────────────────

func TestTemplateFill_Basic(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "template_fill", map[string]any{
		"template":   "Hello {{name}}, you are {{age}} years old.",
		"name":       "Bob",
		"age":        "25",
		"result_var": "greeting",
	})
	if err != nil {
		t.Fatalf("template_fill error: %v", err)
	}
	if out["greeting"] != "Hello Bob, you are 25 years old." {
		t.Errorf("template_fill greeting = %q", out["greeting"])
	}
}

func TestTemplateFill_DefaultResultVar(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "template_fill", map[string]any{
		"template": "Hi {{x}}",
		"x":        "world",
	})
	if err != nil {
		t.Fatalf("template_fill error: %v", err)
	}
	if _, ok := out["filled_content"]; !ok {
		t.Errorf("template_fill should use 'filled_content' as default result_var, got keys: %v", out)
	}
}

func TestTemplateFill_MissingTemplate(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "template_fill", map[string]any{})
	if err == nil || err.Error() != "template_fill: template is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── text_substitute ───────────────────────────────────────────────────────────

func TestTextSubstitute_Basic(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "text_substitute", map[string]any{
		"source": "Hello {{name}}!",
		"vars":   map[string]any{"name": "Carol"},
	})
	if err != nil {
		t.Fatalf("text_substitute error: %v", err)
	}
	if out["substituted"] != "Hello Carol!" {
		t.Errorf("text_substitute substituted = %q", out["substituted"])
	}
}

func TestTextSubstitute_MissingSource(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "text_substitute", map[string]any{})
	if err == nil || err.Error() != "text_substitute: source is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTextSubstitute_NormalizesEscapedUnderscores(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "text_substitute", map[string]any{
		"source": `Hello {{about\_your\_business}}!`,
		"vars":   map[string]any{"about_your_business": "my biz"},
	})
	if err != nil {
		t.Fatalf("text_substitute error: %v", err)
	}
	if out["substituted"] != "Hello my biz!" {
		t.Errorf("text_substitute with escaped underscores: got %q", out["substituted"])
	}
}

// ── format_birth_date ─────────────────────────────────────────────────────────

func TestFormatBirthDate_YYYYMMDD(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "format_birth_date", map[string]any{"date": "1985-02-08"})
	if err != nil {
		t.Fatalf("format_birth_date error: %v", err)
	}
	if out["birth_date"] != "1985-02-08" {
		t.Errorf("format_birth_date = %v, want 1985-02-08", out["birth_date"])
	}
}

func TestFormatBirthDate_MMDDYYYY(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "format_birth_date", map[string]any{"date": "02/08/1985"})
	if err != nil {
		t.Fatalf("format_birth_date error: %v", err)
	}
	if out["birth_date"] != "1985-02-08" {
		t.Errorf("format_birth_date = %v, want 1985-02-08", out["birth_date"])
	}
}

func TestFormatBirthDate_LongMonth(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "format_birth_date", map[string]any{"date": "February 8, 1985"})
	if err != nil {
		t.Fatalf("format_birth_date error: %v", err)
	}
	if out["birth_date"] != "1985-02-08" {
		t.Errorf("format_birth_date = %v, want 1985-02-08", out["birth_date"])
	}
}

func TestFormatBirthDate_AcceptsBirthdayKey(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "format_birth_date", map[string]any{"birthday": "1990-05-15"})
	if err != nil {
		t.Fatalf("format_birth_date error: %v", err)
	}
	if out["birth_date"] != "1990-05-15" {
		t.Errorf("format_birth_date = %v, want 1990-05-15", out["birth_date"])
	}
}

func TestFormatBirthDate_MissingInput(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "format_birth_date", map[string]any{})
	if err == nil {
		t.Error("expected error for missing date, got nil")
	}
}

func TestFormatBirthDate_UnrecognizedFormat(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "format_birth_date", map[string]any{"date": "not-a-date"})
	if err == nil {
		t.Error("expected error for unrecognized format, got nil")
	}
}

// ── format_birth_time ─────────────────────────────────────────────────────────

func TestFormatBirthTime_24Hour(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "format_birth_time", map[string]any{"time": "13:02"})
	if err != nil {
		t.Fatalf("format_birth_time error: %v", err)
	}
	if out["birth_time"] != "13:02" {
		t.Errorf("format_birth_time = %v, want 13:02", out["birth_time"])
	}
}

func TestFormatBirthTime_12HourAM(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "format_birth_time", map[string]any{"time": "1:02 AM"})
	if err != nil {
		t.Fatalf("format_birth_time error: %v", err)
	}
	if out["birth_time"] != "01:02" {
		t.Errorf("format_birth_time = %v, want 01:02", out["birth_time"])
	}
}

func TestFormatBirthTime_12HourPM(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "format_birth_time", map[string]any{"time": "1:30 PM"})
	if err != nil {
		t.Fatalf("format_birth_time error: %v", err)
	}
	if out["birth_time"] != "13:30" {
		t.Errorf("format_birth_time = %v, want 13:30", out["birth_time"])
	}
}

func TestFormatBirthTime_MissingInput(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "format_birth_time", map[string]any{})
	if err == nil {
		t.Error("expected error for missing time, got nil")
	}
}

// ── money_mode ────────────────────────────────────────────────────────────────

func TestMoneyMode_Cardinal(t *testing.T) {
	for _, sign := range []string{"Aries", "Cancer", "Libra", "Capricorn"} {
		t.Run(sign, func(t *testing.T) {
			eng := newPureLogicEngine()
			out, err := execActivity(t, eng, "money_mode", map[string]any{"sign": sign + " rising"})
			if err != nil {
				t.Fatalf("money_mode error: %v", err)
			}
			if out["mode"] != "cardinal" {
				t.Errorf("money_mode mode = %v, want cardinal", out["mode"])
			}
			if out["money_mode"] != "Serial Monogamist" {
				t.Errorf("money_mode money_mode = %v, want Serial Monogamist", out["money_mode"])
			}
		})
	}
}

func TestMoneyMode_Fixed(t *testing.T) {
	for _, sign := range []string{"Taurus", "Leo", "Scorpio", "Aquarius"} {
		t.Run(sign, func(t *testing.T) {
			eng := newPureLogicEngine()
			out, err := execActivity(t, eng, "money_mode", map[string]any{"sign": sign})
			if err != nil {
				t.Fatalf("money_mode error: %v", err)
			}
			if out["mode"] != "fixed" {
				t.Errorf("money_mode mode = %v, want fixed", out["mode"])
			}
			if out["money_mode"] != "Lifer" {
				t.Errorf("money_mode money_mode = %v, want Lifer", out["money_mode"])
			}
		})
	}
}

func TestMoneyMode_Mutable(t *testing.T) {
	for _, sign := range []string{"Gemini", "Virgo", "Sagittarius", "Pisces"} {
		t.Run(sign, func(t *testing.T) {
			eng := newPureLogicEngine()
			out, err := execActivity(t, eng, "money_mode", map[string]any{"sign": sign})
			if err != nil {
				t.Fatalf("money_mode error: %v", err)
			}
			if out["mode"] != "mutable" {
				t.Errorf("money_mode mode = %v, want mutable", out["mode"])
			}
			if out["money_mode"] != "Polyamorist" {
				t.Errorf("money_mode money_mode = %v, want Polyamorist", out["money_mode"])
			}
		})
	}
}

func TestMoneyMode_NoSign(t *testing.T) {
	eng := newPureLogicEngine()
	_, err := execActivity(t, eng, "money_mode", map[string]any{"sign": "no zodiac here"})
	if err == nil {
		t.Error("expected error for invalid sign, got nil")
	}
}

func TestMoneyMode_OutputKey(t *testing.T) {
	eng := newPureLogicEngine()
	out, err := execActivity(t, eng, "money_mode", map[string]any{
		"sign":       "Taurus",
		"output_key": "chart_money_mode",
	})
	if err != nil {
		t.Fatalf("money_mode error: %v", err)
	}
	if out["chart_money_mode"] != "Lifer" {
		t.Errorf("money_mode chart_money_mode = %v, want Lifer", out["chart_money_mode"])
	}
}

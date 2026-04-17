package workflows

import (
	"testing"

	"cosmicbizwitch/pkg/workflow"
	"cosmicbizwitch/pkg/workflow/testutil"
)

// ── pbQueryOp ─────────────────────────────────────────────────────────────────

func TestPBQueryOp(t *testing.T) {
	tests := []struct {
		op   string
		want string
	}{
		{"eq", "="},
		{"neq", "!="},
		{"gt", ">"},
		{"gte", ">="},
		{"lt", "<"},
		{"lte", "<="},
		{"contains", "~"},
		{"not_contains", "!~"},
		{"unknown", "="},  // default
		{"", "="},         // empty string → default
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			got := pbQueryOp(tt.op)
			if got != tt.want {
				t.Errorf("pbQueryOp(%q) = %q, want %q", tt.op, got, tt.want)
			}
		})
	}
}

// ── pb_query validation ───────────────────────────────────────────────────────

func TestPBQuery_MissingTableName(t *testing.T) {
	fn := newPBQueryFn(nil)
	_, err := testutil.New(fn).Run()
	if err == nil || err.Error() != "pb_query: table_name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── pb_create validation ──────────────────────────────────────────────────────

func TestPBCreate_MissingTableName(t *testing.T) {
	fn := newPBCreateFn(nil)
	_, err := testutil.New(fn).
		WithInput("data", map[string]any{"name": "Alice"}).
		Run()
	if err == nil || err.Error() != "pb_create: table_name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPBCreate_MissingData(t *testing.T) {
	fn := newPBCreateFn(nil)
	_, err := testutil.New(fn).WithInput("table_name", "contacts").Run()
	if err == nil || err.Error() != "pb_create: data is required and must be an object" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPBCreate_NilData(t *testing.T) {
	fn := newPBCreateFn(nil)
	_, err := testutil.New(fn).
		WithInput("table_name", "contacts").
		WithInput("data", nil).
		Run()
	if err == nil || err.Error() != "pb_create: data is required and must be an object" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── pb_update validation ──────────────────────────────────────────────────────

func TestPBUpdate_MissingTableName(t *testing.T) {
	fn := newPBUpdateFn(nil)
	_, err := testutil.New(fn).
		WithInput("id", "rec123").
		WithInput("data", map[string]any{"name": "Bob"}).
		Run()
	if err == nil || err.Error() != "pb_update: table_name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPBUpdate_MissingID(t *testing.T) {
	fn := newPBUpdateFn(nil)
	_, err := testutil.New(fn).
		WithInput("table_name", "contacts").
		WithInput("data", map[string]any{"name": "Bob"}).
		Run()
	if err == nil || err.Error() != "pb_update: id is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPBUpdate_MissingData(t *testing.T) {
	fn := newPBUpdateFn(nil)
	_, err := testutil.New(fn).
		WithInput("table_name", "contacts").
		WithInput("id", "rec123").
		Run()
	if err == nil || err.Error() != "pb_update: data is required and must be an object" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── pb_delete validation ──────────────────────────────────────────────────────

func TestPBDelete_MissingTableName(t *testing.T) {
	fn := newPBDeleteFn(nil)
	_, err := testutil.New(fn).WithInput("id", "rec123").Run()
	if err == nil || err.Error() != "pb_delete: table_name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPBDelete_MissingID(t *testing.T) {
	fn := newPBDeleteFn(nil)
	_, err := testutil.New(fn).WithInput("table_name", "contacts").Run()
	if err == nil || err.Error() != "pb_delete: id is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── pb_upsert validation ──────────────────────────────────────────────────────

func TestPBUpsert_MissingTableName(t *testing.T) {
	fn := newPBUpsertFn(nil)
	_, err := testutil.New(fn).
		WithInput("data", map[string]any{"name": "Charlie"}).
		Run()
	if err == nil || err.Error() != "pb_upsert: table_name is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPBUpsert_MissingData(t *testing.T) {
	fn := newPBUpsertFn(nil)
	_, err := testutil.New(fn).WithInput("table_name", "contacts").Run()
	if err == nil {
		t.Error("expected error for missing data, got nil")
	}
	if err != nil && err.Error() == "pb_upsert: table_name is required" {
		t.Error("got table_name error, but table_name was provided")
	}
}

func TestPBUpsert_WhereFiltersNoValidRules(t *testing.T) {
	// where_filters present but all rules are missing field/operator → error
	fn := newPBUpsertFn(nil)
	_, err := testutil.New(fn).
		WithInput("table_name", "contacts").
		WithInput("data", map[string]any{"name": "Dave"}).
		WithInput("where_filters", []any{
			map[string]any{"value": "something"}, // missing field and operator
		}).
		Run()
	if err == nil || err.Error() != "pb_upsert: where_filters provided but no valid rules" {
		t.Errorf("unexpected error: %v", err)
	}
}


// ── workflow.FmtVal (used for all DB filter params) ───────────────────────────
//
// deepInterpolate returns native float64 for JSON numbers. TEXT columns in
// PocketBase/SQLite require string values for equality matches. workflow.FmtVal
// is the canonical coercion function — activity nodes must use it when binding
// any deepInterpolate result to a SQL parameter.

func TestFmtVal_WholeFloat_ConvertsToIntegerString(t *testing.T) {
	// Contact IDs like 1241716883 arrive as float64 but are TEXT in PocketBase.
	got := workflow.FmtVal(float64(1241716883))
	if got != "1241716883" {
		t.Errorf("FmtVal(1241716883.0) = %q, want \"1241716883\"", got)
	}
}

func TestFmtVal_FractionalFloat_FormatsCorrectly(t *testing.T) {
	// Fractional floats are formatted without scientific notation.
	got := workflow.FmtVal(float64(47.6144219))
	if got != "47.6144219" {
		t.Errorf("FmtVal(47.6144219) = %q, want \"47.6144219\"", got)
	}
}

func TestFmtVal_String_PassesThrough(t *testing.T) {
	got := workflow.FmtVal("abc123")
	if got != "abc123" {
		t.Errorf("FmtVal(\"abc123\") = %q, want \"abc123\"", got)
	}
}

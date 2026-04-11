// Package workflows registers built-in demo activities and graphs.
// Add your real application activities and graphs here.
package workflows

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cosmicbizwitch/internal/app/clickfunnels"
	googleapp "cosmicbizwitch/internal/app/google"
	slackapp "cosmicbizwitch/internal/app/slack"
	telegramapp "cosmicbizwitch/internal/app/telegram"
	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterDefaults wires up built-in demo activities and graphs into the engine.
func RegisterDefaults(eng *workflow.Engine, app core.App, getCF func() *clickfunnels.Client, getGoogle func() *googleapp.Client, getSlack func() *slackapp.Client, getTelegram func() *telegramapp.Client) error {
	registerActivities(eng, app, getCF, log.Default())
	registerGoogleActivities(eng, getGoogle)
	registerImageActivities(eng, app, getGoogle)
	registerTranscribeActivities(eng, app, getGoogle)
	registerSlackActivities(eng, getSlack)
	registerTelegramActivities(eng, getTelegram, app)
	registerTemplateFill(eng)
	registerTextSubstitute(eng)
	registerFormatDate(eng)
	registerRunWorkflow(eng)
	registerAstrologyActivities(eng)
	return registerGraphs(eng, getCF)
}

// ── Activities ────────────────────────────────────────────────────────────────

func registerActivities(eng *workflow.Engine, app core.App, getCF func() *clickfunnels.Client, logger *log.Logger) {
	// test_data: injects a static JSON payload into the workflow context for debugging.
	// When the workflow was started by a real trigger (_source == "trigger"), this
	// node is skipped so live data is used instead.
	eng.RegisterActivityWithMeta("test_data",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			if src, _ := input["_source"].(string); src == "trigger" {
				return nil, workflow.ErrSkip
			}
			// input["data"] may arrive as a string or as an already-parsed map
			// (deepInterpolate auto-parses JSON-shaped strings).
			switch d := input["data"].(type) {
			case map[string]any:
				return d, nil
			case string:
				if d == "" {
					return map[string]any{}, nil
				}
				var parsed map[string]any
				if err := json.Unmarshal([]byte(d), &parsed); err != nil {
					return nil, fmt.Errorf("test_data: invalid JSON in 'data' field: %w", err)
				}
				return parsed, nil
			default:
				return map[string]any{}, nil
			}
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Injects test JSON data into the workflow context. Automatically skipped when triggered by a real webhook/cron/record-hook so live data is used instead.",
			InputFields: []workflow.FieldMeta{
				{Name: "data", Type: "string", Description: "JSON object string to merge into context (e.g. {\"contact_id\": \"123\"})"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "*", Type: "any", Description: "All keys from the parsed JSON object"},
			},
		},
	)

	// notes: documentation-only node, always skipped during execution.
	eng.RegisterActivityWithMeta("notes",
		func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return nil, workflow.ErrSkip
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Documentation node — skipped during execution",
			InputFields: []workflow.FieldMeta{
				{Name: "note", Type: "textarea", Description: "Free-form note for documentation purposes", Required: false},
			},
		},
	)

	// echo: returns its input unchanged. Useful as a pass-through step.
	eng.RegisterActivityWithMeta("echo",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			out := make(map[string]any, len(input))
			for k, v := range input {
				out[k] = v
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Passes input through unchanged",
			InputFields: []workflow.FieldMeta{
				{Name: "*", Type: "any", Description: "Any fields"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "*", Type: "any", Description: "Any fields"},
			},
		},
	)

	// echo_plus_1: increments "value" by 1, then sleeps a random 1–10 seconds.
	// Demonstrates both computation and the running state being visible in the UI.
	eng.RegisterActivityWithMeta("echo_plus_1",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			out := make(map[string]any, len(input))
			for k, v := range input {
				out[k] = v
			}

			// Increment "value" if present and numeric.
			current := toFloat64OrZero(input["value"])
			out["value"] = current + 1

			// Sleep 1–10 seconds so we can watch it run.
			sleep := time.Duration(1+rand.Intn(10)) * time.Second
			out["slept_ms"] = sleep.Milliseconds()
			time.Sleep(sleep)

			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Increments 'value' by 1 and sleeps 1-10s (demo)",
			InputFields: []workflow.FieldMeta{
				{Name: "value", Type: "number", Description: "Current value"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "value", Type: "number", Description: "Value + 1"},
				{Name: "slept_ms", Type: "number", Description: "Milliseconds slept"},
			},
		},
	)

	// echo_fail: always returns an error. Use to test retry + failure paths.
	eng.RegisterActivityWithMeta("echo_fail",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			time.Sleep(time.Duration(1+rand.Intn(3)) * time.Second)
			return nil, fmt.Errorf("intentional failure for testing (value=%v)", input["value"])
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Always fails — use to test retry and failure paths",
			InputFields: []workflow.FieldMeta{
				{Name: "value", Type: "any"},
			},
		},
	)

	// pb_query: queries a PocketBase collection by filter, returns matching records.
	eng.RegisterActivityWithMeta("pb_query",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			tableName, _ := input["table_name"].(string)
			if tableName == "" {
				return nil, fmt.Errorf("pb_query: table_name is required")
			}

			limitF, _ := input["limit"].(float64)
			limit := int(limitF)
			if limit <= 0 {
				limit = 50
			}

			// Build filter from structured rules or fall back to raw filter string.
			filterStr := ""
			params := dbx.Params{}
			rawFilters, _ := input["filters"].([]any)
			if len(rawFilters) > 0 {
				filterMode, _ := input["filter_mode"].(string)
				sep := " && "
				if filterMode == "or" {
					sep = " || "
				}
				parts := []string{}
				for i, rf := range rawFilters {
					rule, ok := rf.(map[string]any)
					if !ok {
						continue
					}
					field, _ := rule["field"].(string)
					operator, _ := rule["operator"].(string)
					value := rule["value"]
					if field == "" || operator == "" {
						continue
					}
					key := fmt.Sprintf("v%d", i)
					parts = append(parts, fmt.Sprintf("%s %s {:%s}", field, pbQueryOp(operator), key))
					params[key] = value
				}
				if len(parts) > 0 {
					filterStr = strings.Join(parts, sep)
				}
			}
			// Backward compat: raw filter string.
			if filterStr == "" {
				rawFilter, _ := input["filter"].(string)
				filterStr = rawFilter
			}
			if filterStr == "" {
				filterStr = "1=1"
			}

			// Build sort string: "-field" = desc, "field" = asc.
			sort := ""
			sortField, _ := input["sort_field"].(string)
			sortDir, _ := input["sort_dir"].(string)
			if sortField != "" {
				if sortDir == "desc" {
					sort = "-" + sortField
				} else {
					sort = sortField
				}
			}

			records, err := app.FindRecordsByFilter(tableName, filterStr, sort, limit, 0, params)
			if err != nil {
				return nil, fmt.Errorf("pb_query: %w", err)
			}

			out := map[string]any{
				"found": len(records) > 0,
				"count": len(records),
			}

			recList := make([]any, 0, len(records))
			for _, rec := range records {
				m := map[string]any{"id": rec.Id}
				for _, col := range rec.Collection().Fields {
					m[col.GetName()] = rec.Get(col.GetName())
				}
				recList = append(recList, m)
			}
			out["records"] = recList

			// Flat fields from first record for easy transition access.
			if len(records) > 0 {
				rec := records[0]
				out["id"] = rec.Id
				for _, col := range rec.Collection().Fields {
					out[col.GetName()] = rec.Get(col.GetName())
				}
			}

			// If result_key is set, nest everything under that key so multiple
			// pb_query nodes don't overwrite each other in the context.
			resultKey, _ := input["result_key"].(string)
			if resultKey != "" {
				return map[string]any{resultKey: out}, nil
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "PocketBase",
			Description: "Queries a PocketBase collection with a visual filter builder",
			InputFields: []workflow.FieldMeta{
				{Name: "table_name", Type: "string", Description: "PocketBase collection name", Required: true},
				{Name: "result_key", Type: "string", Description: "Context key to store results under (e.g. \"contacts\" → {{contacts.records}})"},
				{Name: "filters", Type: "any", Description: "Structured filter rules (use Query Builder)"},
				{Name: "filter_mode", Type: "string", Description: "\"and\" or \"or\" (default: and)"},
				{Name: "sort_field", Type: "string", Description: "Field to sort by"},
				{Name: "sort_dir", Type: "string", Description: "\"asc\" or \"desc\""},
				{Name: "limit", Type: "number", Description: "Max records to return (default 50)"},
				{Name: "filter", Type: "string", Description: "Raw PocketBase filter string (backward compat)"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "<result_key>.found", Type: "bool", Description: "True if any records matched"},
				{Name: "<result_key>.count", Type: "number", Description: "Number of records returned"},
				{Name: "<result_key>.records", Type: "any", Description: "Array of all matching records"},
				{Name: "<result_key>.*", Type: "any", Description: "Fields from the first record (flat, under result_key)"},
			},
		},
	)

	// pb_create: creates a new record in a PocketBase collection.
	eng.RegisterActivityWithMeta("pb_create",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			tableName, _ := input["table_name"].(string)
			if tableName == "" {
				return nil, fmt.Errorf("pb_create: table_name is required")
			}
			data, ok := input["data"].(map[string]any)
			if !ok || data == nil {
				return nil, fmt.Errorf("pb_create: data is required and must be an object")
			}

			col, err := app.FindCollectionByNameOrId(tableName)
			if err != nil {
				return nil, fmt.Errorf("pb_create: find collection %q: %w", tableName, err)
			}

			rec := core.NewRecord(col)
			for k, v := range data {
				rec.Set(k, v)
			}

			if err := app.Save(rec); err != nil {
				return nil, fmt.Errorf("pb_create: save: %w", err)
			}

			recMap := make(map[string]any)
			for _, f := range rec.Collection().Fields {
				recMap[f.GetName()] = rec.Get(f.GetName())
			}
			out := map[string]any{"id": rec.Id, "record": recMap}
			if rk, _ := input["result_key"].(string); rk != "" {
				return map[string]any{rk: out}, nil
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "PocketBase",
			Description: "Creates a new record in a PocketBase collection",
			InputFields: []workflow.FieldMeta{
				{Name: "table_name", Type: "string", Description: "PocketBase collection name", Required: true},
				{Name: "result_key", Type: "string", Description: "Context key to store result under (e.g. \"new_contact\")"},
				{Name: "data", Type: "object", Description: "Field values for the new record", Required: true},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "id", Type: "string", Description: "ID of the newly created record"},
				{Name: "record", Type: "object", Description: "All fields of the created record"},
			},
		},
	)

	// pb_update: updates an existing record in a PocketBase collection.
	eng.RegisterActivityWithMeta("pb_update",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			tableName, _ := input["table_name"].(string)
			if tableName == "" {
				return nil, fmt.Errorf("pb_update: table_name is required")
			}
			id, _ := input["id"].(string)
			if id == "" {
				return nil, fmt.Errorf("pb_update: id is required")
			}
			data, ok := input["data"].(map[string]any)
			if !ok || data == nil {
				return nil, fmt.Errorf("pb_update: data is required and must be an object")
			}

			rec, err := app.FindRecordById(tableName, id)
			if err != nil {
				return nil, fmt.Errorf("pb_update: find record %q in %q: %w", id, tableName, err)
			}

			for k, v := range data {
				rec.Set(k, v)
			}

			if err := app.Save(rec); err != nil {
				return nil, fmt.Errorf("pb_update: save: %w", err)
			}

			recMap := make(map[string]any)
			for _, f := range rec.Collection().Fields {
				recMap[f.GetName()] = rec.Get(f.GetName())
			}
			out := map[string]any{"id": rec.Id, "record": recMap}
			if rk, _ := input["result_key"].(string); rk != "" {
				return map[string]any{rk: out}, nil
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "PocketBase",
			Description: "Updates an existing record in a PocketBase collection",
			InputFields: []workflow.FieldMeta{
				{Name: "table_name", Type: "string", Description: "PocketBase collection name", Required: true},
				{Name: "result_key", Type: "string", Description: "Context key to store result under (e.g. \"updated_contact\")"},
				{Name: "id", Type: "string", Description: "Record ID to update", Required: true},
				{Name: "data", Type: "object", Description: "Field values to update on the record", Required: true},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "id", Type: "string", Description: "ID of the updated record"},
				{Name: "record", Type: "object", Description: "All fields of the updated record"},
			},
		},
	)

	// pb_delete: deletes a record from a PocketBase collection.
	eng.RegisterActivityWithMeta("pb_delete",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			tableName, _ := input["table_name"].(string)
			if tableName == "" {
				return nil, fmt.Errorf("pb_delete: table_name is required")
			}
			id, _ := input["id"].(string)
			if id == "" {
				return nil, fmt.Errorf("pb_delete: id is required")
			}

			rec, err := app.FindRecordById(tableName, id)
			if err != nil {
				return nil, fmt.Errorf("pb_delete: find record %q in %q: %w", id, tableName, err)
			}

			if err := app.Delete(rec); err != nil {
				return nil, fmt.Errorf("pb_delete: delete: %w", err)
			}

			out := map[string]any{"deleted": true, "id": id}
			if rk, _ := input["result_key"].(string); rk != "" {
				return map[string]any{rk: out}, nil
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "PocketBase",
			Description: "Deletes a record from a PocketBase collection",
			InputFields: []workflow.FieldMeta{
				{Name: "table_name", Type: "string", Description: "PocketBase collection name", Required: true},
				{Name: "result_key", Type: "string", Description: "Context key to store result under (e.g. \"deleted_contact\")"},
				{Name: "id", Type: "string", Description: "Record ID to delete", Required: true},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "deleted", Type: "bool", Description: "True if the record was deleted"},
				{Name: "id", Type: "string", Description: "ID of the deleted record"},
			},
		},
	)

	// pb_upsert: updates a record if id is provided, creates one if not.
	eng.RegisterActivityWithMeta("pb_upsert",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			tableName, _ := input["table_name"].(string)
			if tableName == "" {
				return nil, fmt.Errorf("pb_upsert: table_name is required")
			}
			data, ok := input["data"].(map[string]any)
			if !ok || data == nil {
				return nil, fmt.Errorf("pb_upsert: data is required and must be an object (got %T: %v)", input["data"], input["data"])
			}

			id, _ := input["id"].(string)

			// Debug: log resolved input
			dataJSON, _ := json.Marshal(data)
			fmt.Printf("[pb_upsert] table=%q id=%q data=%s\n", tableName, id, dataJSON)

			var rec *core.Record
			// where_filters takes priority over id for finding the record.
			whereFilters, _ := input["where_filters"].([]any)
			if len(whereFilters) > 0 {
				filterMode, _ := input["where_filter_mode"].(string)
				sep := " && "
				if filterMode == "or" {
					sep = " || "
				}
				parts := []string{}
				params := dbx.Params{}
				for i, rf := range whereFilters {
					rule, ok := rf.(map[string]any)
					if !ok {
						fmt.Printf("[pb_upsert] where_filters[%d] is not a map: %T %v\n", i, rf, rf)
						continue
					}
					field, _ := rule["field"].(string)
					operator, _ := rule["operator"].(string)
					value := rule["value"]
					fmt.Printf("[pb_upsert] where rule[%d]: field=%q op=%q value=%v (type %T)\n", i, field, operator, value, value)
					if field == "" || operator == "" {
						continue
					}
					key := fmt.Sprintf("v%d", i)
					parts = append(parts, fmt.Sprintf("%s %s {:%s}", field, pbQueryOp(operator), key))
					params[key] = value
				}
				if len(parts) == 0 {
					return nil, fmt.Errorf("pb_upsert: where_filters provided but no valid rules")
				}
				filterStr := strings.Join(parts, sep)
				fmt.Printf("[pb_upsert] filter=%q params=%v\n", filterStr, params)
				col, err := app.FindCollectionByNameOrId(tableName)
				if err != nil {
					return nil, fmt.Errorf("pb_upsert: find collection %q: %w", tableName, err)
				}
				records, err := app.FindRecordsByFilter(tableName, filterStr, "", 1, 0, params)
				if err != nil {
					return nil, fmt.Errorf("pb_upsert: where query: %w", err)
				}
				fmt.Printf("[pb_upsert] found %d existing records\n", len(records))
				if len(records) > 0 {
					rec = records[0]
				} else {
					rec = core.NewRecord(col)
				}
			} else if id != "" {
				// Update by id
				existing, err := app.FindRecordById(tableName, id)
				if err != nil {
					return nil, fmt.Errorf("pb_upsert: find record %q in %q: %w", id, tableName, err)
				}
				rec = existing
			} else {
				// Create path
				col, err := app.FindCollectionByNameOrId(tableName)
				if err != nil {
					return nil, fmt.Errorf("pb_upsert: find collection %q: %w", tableName, err)
				}
				rec = core.NewRecord(col)
			}

			fmt.Printf("[pb_upsert] setting %d data fields on record id=%q:\n", len(data), rec.Id)
			for k, v := range data {
				fmt.Printf("[pb_upsert]   data[%q] type=%T value=%#v\n", k, v, v)
				rec.Set(k, v)
			}
			fmt.Printf("[pb_upsert] pre-save record fields:\n")
			for _, f := range rec.Collection().Fields {
				fv := rec.Get(f.GetName())
				fmt.Printf("[pb_upsert]   field[%q] type=%T value=%#v\n", f.GetName(), fv, fv)
			}
			if err := app.Save(rec); err != nil {
				// Build a detailed error showing all data field types/values so it appears in [APP] logs
				details := make([]string, 0, len(data))
				for k, v := range data {
					details = append(details, fmt.Sprintf("%s=%T(%#v)", k, v, v))
				}
				return nil, fmt.Errorf("pb_upsert: save: %w | data: %s", err, strings.Join(details, ", "))
			}

			recMap := make(map[string]any)
			for _, f := range rec.Collection().Fields {
				recMap[f.GetName()] = rec.Get(f.GetName())
			}
			out := map[string]any{"id": rec.Id, "created": id == "", "record": recMap}
			if rk, _ := input["result_key"].(string); rk != "" {
				return map[string]any{rk: out}, nil
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "PocketBase",
			Description: "Updates an existing record if id is supplied, otherwise creates a new one",
			InputFields: []workflow.FieldMeta{
				{Name: "table_name", Type: "string", Description: "PocketBase collection name", Required: true},
				{Name: "result_key", Type: "string", Description: "Context key to store result under (e.g. \"contact\")"},
				{Name: "id", Type: "string", Description: "Record ID to update (omit to create)"},
				{Name: "where_filters", Type: "any", Description: "Structured filter rules to find record (use Where Builder). Takes priority over id."},
				{Name: "where_filter_mode", Type: "string", Description: "\"and\" or \"or\" (default: and)"},
				{Name: "data", Type: "object", Description: "Field values to set on the record", Required: true},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "id", Type: "string", Description: "ID of the upserted record"},
				{Name: "created", Type: "bool", Description: "True if a new record was created"},
				{Name: "record", Type: "object", Description: "All fields of the upserted record"},
			},
		},
	)

	// loop: stub for graph validation — engine handles the actual iteration logic.
	eng.RegisterActivityWithMeta("loop",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			// Never actually called — handled by the engine's runLoop.
			return input, nil
		},
		workflow.ActivityMeta{
			Category:    "Flow Control",
			Description: "Iterates over a list in the workflow context, running the 'body' path for each item. Connect a loop_next node to mark the end of each iteration. Use the 'done' transition for what comes after the loop.",
			InputFields: []workflow.FieldMeta{
				{Name: "list_key", Type: "string", Description: "Context key holding the array to iterate (default: 'records')"},
				{Name: "item_key", Type: "string", Description: "Context key to set for each iteration's current item (default: 'item')"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "loop_index", Type: "number", Description: "Current iteration index (0-based), available in body nodes"},
				{Name: "*", Type: "any", Description: "Item fields merged into context each iteration"},
			},
		},
	)

	// loop_next: stub for graph validation — marks the end of a loop body iteration.
	eng.RegisterActivityWithMeta("loop_next",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			// Never actually called — marks the end of a loop body iteration.
			return input, nil
		},
		workflow.ActivityMeta{
			Category:     "Flow Control",
			Description:  "Marks the end of a loop body. Place this at the end of the nodes inside a loop. The engine automatically loops back to the next item.",
			InputFields:  []workflow.FieldMeta{{Name: "*", Type: "any", Description: "Passed through from previous node"}},
			OutputFields: []workflow.FieldMeta{{Name: "*", Type: "any", Description: "Passed through unchanged"}},
		},
	)

	// human_input: pass-through registered so human-in-loop nodes pass graph validation.
	eng.RegisterActivityWithMeta("human_input",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			// Never actually called — human nodes are handled by the engine's handleHumanNode.
			// Registered so graph validation passes for is_human=true nodes.
			out := make(map[string]any, len(input))
			for k, v := range input {
				out[k] = v
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:     "Utility",
			Description:  "Human-in-the-loop: workflow pauses until a human provides input via the trigger API",
			InputFields:  []workflow.FieldMeta{{Name: "*", Type: "any", Description: "Passed to human reviewer"}},
			OutputFields: []workflow.FieldMeta{{Name: "*", Type: "any", Description: "Human-provided fields"}},
		},
	)

	// python_eval: pass-through stub — actual execution happens in the browser via Pyodide (WebAssembly).
	// The node is saved with is_human=true so the engine pauses and waits. WorkflowDetail detects
	// the pause, loads Pyodide, runs the code, and POSTs the result back via /trigger.
	eng.RegisterActivityWithMeta("python_eval",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			// Never actually called — handled by the engine's human-node pause mechanism.
			out := make(map[string]any, len(input))
			for k, v := range input {
				out[k] = v
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Runs Python code in-browser via Pyodide (WebAssembly). The workflow pauses; the browser executes the script and resumes with the result dict.",
			InputFields: []workflow.FieldMeta{
				{Name: "code", Type: "string", Description: "Python script. `input` dict contains workflow context. Must set `result` dict."},
				{Name: "*", Type: "any", Description: "Workflow context fields available as `input` in the script"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "*", Type: "any", Description: "Fields returned in the `result` dict"},
			},
		},
	)

	// http_request: makes an outbound HTTP request with template interpolation.
	eng.RegisterActivityWithMeta("http_request",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			rawURL, _ := input["url"].(string)
			if rawURL == "" {
				return nil, fmt.Errorf("http_request: url is required")
			}
			method, _ := input["method"].(string)
			if method == "" {
				method = "GET"
			}
			method = strings.ToUpper(method)

			// Template interpolation: replace {{key}} with input values.
			rawURL = interpolateTemplate(rawURL, input)

			// Build request body.
			var bodyReader io.Reader
			var finalBodyStr string
			if bodyVal, ok := input["body"]; ok && bodyVal != nil {
				var bodyStr string
				switch v := bodyVal.(type) {
				case string:
					bodyStr = interpolateTemplate(v, input)
				default:
					// Marshal map/slice to JSON.
					b, err := json.Marshal(v)
					if err == nil {
						bodyStr = string(b)
					}
				}
				// Coerce quoted numbers/booleans (e.g. "47.6" → 47.6) that result
				// from template placeholders being wrapped in JSON string quotes.
				if bodyStr != "" {
					var parsed any
					if err := json.Unmarshal([]byte(bodyStr), &parsed); err == nil {
						if b, err := json.Marshal(coerceJSONTypes(parsed)); err == nil {
							bodyStr = string(b)
						}
					}
					finalBodyStr = bodyStr
					bodyReader = strings.NewReader(bodyStr)
				}
			}

			timeout := 30 * time.Second
			if t, _ := input["timeout_seconds"].(float64); t > 0 {
				timeout = time.Duration(t) * time.Second
			}

			client := &http.Client{Timeout: timeout}
			req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
			if err != nil {
				return nil, fmt.Errorf("http_request: %w", err)
			}
			if bodyReader != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			var headersMap map[string]any
			if m, ok := input["headers"].(map[string]any); ok {
				headersMap = m
			} else if s, ok := input["headers"].(string); ok && s != "" {
				_ = json.Unmarshal([]byte(s), &headersMap)
			}
			for k, v := range headersMap {
				req.Header.Set(k, interpolateTemplate(fmt.Sprintf("%v", v), input))
			}

			// Build curl command for debugging -- always included in output.
			var curlParts []string
			curlParts = append(curlParts, fmt.Sprintf("curl -X %s", method))
			for hk, vals := range req.Header {
				curlParts = append(curlParts, fmt.Sprintf("-H %q", hk+": "+strings.Join(vals, ", ")))
			}
			if finalBodyStr != "" {
				curlParts = append(curlParts, fmt.Sprintf("-d %q", finalBodyStr))
			}
			curlParts = append(curlParts, fmt.Sprintf("%q", rawURL))
			curlCommand := strings.Join(curlParts, " \\\n  ")

			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("http_request: %w\ncurl: %s", err, curlCommand)
			}
			defer resp.Body.Close()

			bodyBytes, _ := io.ReadAll(resp.Body)

			resultKey, _ := input["result_key"].(string)
			ok := resp.StatusCode >= 200 && resp.StatusCode < 300

			var jsonBody map[string]any
			if err := json.Unmarshal(bodyBytes, &jsonBody); err != nil {
				// Not JSON — check if binary content type and base64-encode to preserve bytes.
				ct := resp.Header.Get("Content-Type")
				isBinary := strings.HasPrefix(ct, "image/") ||
					strings.HasPrefix(ct, "audio/") ||
					strings.HasPrefix(ct, "video/") ||
					ct == "application/octet-stream" ||
					ct == "application/pdf"
				out := map[string]any{
					"status_code":   resp.StatusCode,
					"ok":            ok,
					"curl":          curlCommand,
					"content_type":  ct,
				}
				if isBinary {
					out["body"] = base64.StdEncoding.EncodeToString(bodyBytes)
					out["body_encoding"] = "base64"
				} else {
					out["body"] = string(bodyBytes)
					out["response_body"] = string(bodyBytes) // backwards compat
				}
				return out, nil
			}

			if resultKey != "" {
				return map[string]any{
					"status_code": resp.StatusCode,
					"ok":          ok,
					"curl":        curlCommand,
					resultKey:     jsonBody,
				}, nil
			}

			// Spread parsed JSON keys directly into context
			out := map[string]any{"status_code": resp.StatusCode, "ok": ok, "curl": curlCommand}
			for k, v := range jsonBody {
				out[k] = v
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "HTTP",
			Description: "Makes an HTTP request. Use {{key}} in url/body/headers to interpolate workflow context values.",
			InputFields: []workflow.FieldMeta{
				{Name: "url", Type: "string", Description: "Request URL. Supports {{key}} template interpolation."},
				{Name: "method", Type: "string", Description: "HTTP method: GET, POST, PUT, PATCH, DELETE (default: GET)"},
				{Name: "body", Type: "string|object", Description: "Request body. String or JSON object. Supports {{key}} interpolation."},
				{Name: "headers", Type: "object", Description: "HTTP headers as key/value map. Values support {{key}} interpolation."},
				{Name: "timeout_seconds", Type: "number", Description: "Request timeout in seconds (default: 30)"},
				{Name: "result_key", Type: "string", Description: "If set, stores the parsed JSON response under this context key instead of spreading fields directly."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "status_code", Type: "number", Description: "HTTP response status code"},
				{Name: "ok", Type: "bool", Description: "True if status code is 200-299"},
				{Name: "*", Type: "any", Description: "Parsed JSON fields spread into context (or stored under result_key if set)"},
			},
		},
	)

	// muxer: stub for graph validation — engine handles the actual fan-out logic.
	eng.RegisterActivityWithMeta("muxer",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			// Handled by the engine as a fan-out. This stub is for graph validation only.
			return input, nil
		},
		workflow.ActivityMeta{
			Category:     "Flow Control",
			Description:  "Fan-out: runs all connected next nodes in parallel with the same input data. Connect to a condenser to merge results.",
			InputFields:  []workflow.FieldMeta{{Name: "*", Type: "any", Description: "Passed to all parallel branches"}},
			OutputFields: []workflow.FieldMeta{{Name: "*", Type: "any", Description: "Merged output from all branches"}},
		},
	)

	// condenser: stub for graph validation — engine handles the actual fan-in logic.
	eng.RegisterActivityWithMeta("condenser",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			// Handled by the engine as a fan-in. This stub is for graph validation only.
			return input, nil
		},
		workflow.ActivityMeta{
			Category:     "Flow Control",
			Description:  "Fan-in: merges outputs from all parallel branches into a single context and continues on one path.",
			InputFields:  []workflow.FieldMeta{{Name: "*", Type: "any", Description: "Merged outputs from parallel branches"}},
			OutputFields: []workflow.FieldMeta{{Name: "*", Type: "any", Description: "Merged map of all branch outputs"}},
		},
	)

	// cf_upsert_contact: removed — stub kept so existing workflows referencing it fail gracefully.
	eng.RegisterActivityWithMeta("cf_upsert_contact",
		func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return nil, fmt.Errorf("cf_upsert_contact has been removed — replace this node with cf_upsert to update contacts via the ClickFunnels API")
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Removed. Use cf_upsert instead.",
			InputFields:  []workflow.FieldMeta{},
			OutputFields: []workflow.FieldMeta{},
		},
	)

	// cf_upsert: creates or updates a contact in ClickFunnels via the API.
	eng.RegisterActivityWithMeta("cf_upsert",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			cfClient := getCF()
			if cfClient == nil {
				return nil, fmt.Errorf("cf_upsert: ClickFunnels not configured — add CF_API_KEY in Settings")
			}

			data, ok := input["data"].(map[string]any)
			if !ok || data == nil {
				return nil, fmt.Errorf("cf_upsert: data is required and must be an object (got %T)", input["data"])
			}

			strPtr := func(key string) *string {
				if v, ok := data[key]; ok {
					if s, ok := v.(string); ok && s != "" {
						return &s
					}
				}
				return nil
			}

			params := clickfunnels.ContactParams{
				EmailAddress: strPtr("email_address"),
				FirstName:    strPtr("first_name"),
				LastName:     strPtr("last_name"),
				PhoneNumber:  strPtr("phone_number"),
				TimeZone:     strPtr("time_zone"),
				FbURL:        strPtr("fb_url"),
				TwitterURL:   strPtr("twitter_url"),
				InstagramURL: strPtr("instagram_url"),
				LinkedinURL:  strPtr("linkedin_url"),
				WebsiteURL:   strPtr("website_url"),
			}

			// custom_attributes: accepts a native map or a JSON-encoded object string.
			switch ca := data["custom_attributes"].(type) {
			case map[string]any:
				attrs := make(map[string]string, len(ca))
				for k, v := range ca {
					attrs[k] = fmt.Sprintf("%v", v)
				}
				params.CustomAttributes = attrs
			case string:
				if ca != "" {
					var raw map[string]any
					if json.Unmarshal([]byte(ca), &raw) == nil {
						attrs := make(map[string]string, len(raw))
						for k, v := range raw {
							attrs[k] = fmt.Sprintf("%v", v)
						}
						params.CustomAttributes = attrs
					}
				}
			}

			var contact *clickfunnels.Contact
			var err error

			contactIDStr, _ := input["contact_id"].(string)
			if contactIDStr == "" {
				if v, ok := input["contact_id"].(float64); ok && v != 0 {
					contactIDStr = fmt.Sprintf("%d", int64(v))
				}
			}

			if contactIDStr != "" {
				id, parseErr := strconv.Atoi(contactIDStr)
				if parseErr != nil {
					return nil, fmt.Errorf("cf_upsert: contact_id must be numeric, got %q", contactIDStr)
				}
				contact, err = cfClient.UpdateContact(ctx, id, params)
				if err != nil {
					return nil, fmt.Errorf("cf_upsert: update contact: %w", err)
				}
			} else {
				contact, err = cfClient.CreateContact(ctx, params)
				if err != nil {
					return nil, fmt.Errorf("cf_upsert: create contact: %w", err)
				}
			}

			out := map[string]any{
				"id":         contact.ID,
				"public_id":  contact.PublicID,
				"is_active":  contact.IsActive,
				"tags":       contact.Tags,
				"custom_attributes": contact.CustomAttributes,
				"created_at": contact.CreatedAt,
				"updated_at": contact.UpdatedAt,
			}
			if contact.EmailAddress != nil {
				out["email_address"] = *contact.EmailAddress
			}
			if contact.FirstName != nil {
				out["first_name"] = *contact.FirstName
			}
			if contact.LastName != nil {
				out["last_name"] = *contact.LastName
			}
			if contact.PhoneNumber != nil {
				out["phone_number"] = *contact.PhoneNumber
			}
			if rk, _ := input["result_key"].(string); rk != "" {
				return map[string]any{rk: out}, nil
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Create or update a ClickFunnels contact via the API. Omit contact_id to create; supply it to update.",
			InputFields: []workflow.FieldMeta{
				{Name: "contact_id", Type: "string", Required: false, Description: "Numeric CF contact ID to update. Omit to create a new contact."},
				{Name: "result_key", Type: "string", Description: "Context key to store result under (e.g. \"cf_contact\")"},
				{Name: "data", Type: "object", Required: true, Description: "Contact fields: email_address, first_name, last_name, phone_number, time_zone, fb_url, twitter_url, instagram_url, linkedin_url, website_url, custom_attributes"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "id", Type: "number", Description: "ClickFunnels contact ID"},
				{Name: "public_id", Type: "string", Description: "ClickFunnels public ID"},
				{Name: "email_address", Type: "string", Description: "Contact email address"},
				{Name: "first_name", Type: "string", Description: "Contact first name"},
				{Name: "last_name", Type: "string", Description: "Contact last name"},
				{Name: "phone_number", Type: "string", Description: "Contact phone number"},
				{Name: "is_active", Type: "bool", Description: "Whether the contact is active"},
				{Name: "tags", Type: "array", Description: "Tags currently on the contact"},
				{Name: "custom_attributes", Type: "object", Description: "Custom attribute key/value pairs"},
				{Name: "created_at", Type: "string", Description: "CF record creation timestamp"},
				{Name: "updated_at", Type: "string", Description: "CF record last-updated timestamp"},
			},
		},
	)

	// cf_get_contact: fetches a ClickFunnels contact by numeric ID.
	// If cfClient is nil (CF_API_KEY not configured), the activity returns an error.
	eng.RegisterActivityWithMeta("cf_get_contact",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			resultVar, _ := input["result_var"].(string)
			if resultVar == "" {
				resultVar = "contact"
			}

			setError := func(msg string) (map[string]any, error) {
				return map[string]any{
					resultVar: map[string]any{
						"error":  msg,
						"status": "error",
					},
				}, nil
			}

			cfClient := getCF()
			if cfClient == nil {
				return setError("ClickFunnels client not configured (CF_API_KEY missing)")
			}
			contactID := int(toFloat64OrZero(input["contact_id"]))
			if contactID == 0 {
				return setError("contact_id is required and must be non-zero")
			}
			contact, err := cfClient.GetContact(ctx, contactID)
			if err != nil {
				return setError(err.Error())
			}
			if contact == nil {
				return setError("no contact returned")
			}
			email := ""
			if contact.EmailAddress != nil {
				email = *contact.EmailAddress
			}
			firstName := ""
			if contact.FirstName != nil {
				firstName = *contact.FirstName
			}
			lastName := ""
			if contact.LastName != nil {
				lastName = *contact.LastName
			}
			phoneNumber := ""
			if contact.PhoneNumber != nil {
				phoneNumber = *contact.PhoneNumber
			}
			return map[string]any{
				resultVar: map[string]any{
					"status":            "ok",
					"cf_id":             contact.ID,
					"cf_public_id":      contact.PublicID,
					"email_address":     email,
					"first_name":        firstName,
					"last_name":         lastName,
					"phone_number":      phoneNumber,
					"is_active":         contact.IsActive,
					"tags":              contact.Tags,
					"custom_attributes": contact.CustomAttributes,
				},
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Fetches a ClickFunnels contact by numeric ID. Result is stored under result_var (default: \"contact\"). On failure, result_var.error and result_var.status=\"error\" are set instead of failing the workflow.",
			InputFields: []workflow.FieldMeta{
				{Name: "contact_id", Type: "number", Description: "Numeric ClickFunnels contact ID", Required: true},
				{Name: "result_var", Type: "string", Description: "Context key to store the result under (default: \"contact\")"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "contact", Type: "object", Description: "Contact data object (or error object) stored under result_var. Contains: status, cf_id, cf_public_id, email_address, first_name, last_name, phone_number, is_active, tags, custom_attributes. On error: status=\"error\", error=\"<message>\"."},
			},
		},
	)

	// cf_search_contact: searches for CF contacts by partial email address.
	// Uses filter[email_address] on the API and does a case-insensitive contains filter client-side.
	eng.RegisterActivityWithMeta("cf_search_contact",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			resultVar, _ := input["result_var"].(string)
			if resultVar == "" {
				resultVar = "contacts"
			}

			setError := func(msg string) (map[string]any, error) {
				return map[string]any{
					resultVar: map[string]any{
						"error":  msg,
						"status": "error",
						"count":  0,
						"items":  []any{},
					},
				}, nil
			}

			cfClient := getCF()
			if cfClient == nil {
				return setError("ClickFunnels client not configured (CF_API_KEY missing)")
			}

			email, _ := input["email"].(string)
			if email == "" {
				return setError("email is required")
			}

			maxResults := int(toFloat64OrZero(input["max_results"]))
			if maxResults <= 0 {
				maxResults = 10
			}

			contacts, err := cfClient.SearchContactsByEmail(ctx, email, maxResults)
			if err != nil {
				return setError(err.Error())
			}

			items := make([]any, 0, len(contacts))
			for _, c := range contacts {
				item := map[string]any{
					"cf_id":             c.ID,
					"cf_public_id":      c.PublicID,
					"is_active":         c.IsActive,
					"tags":              c.Tags,
					"custom_attributes": c.CustomAttributes,
				}
				if c.EmailAddress != nil {
					item["email_address"] = *c.EmailAddress
				}
				if c.FirstName != nil {
					item["first_name"] = *c.FirstName
				}
				if c.LastName != nil {
					item["last_name"] = *c.LastName
				}
				if c.PhoneNumber != nil {
					item["phone_number"] = *c.PhoneNumber
				}
				items = append(items, item)
			}

			// Also expose the first match as a convenience at result_var.first
			var first any
			if len(items) > 0 {
				first = items[0]
			}

			return map[string]any{
				resultVar: map[string]any{
					"status": "ok",
					"count":  len(items),
					"items":  items,
					"first":  first,
				},
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Searches ClickFunnels contacts by partial email address (case-insensitive contains match). Results are stored under result_var (default: \"contacts\") with count, items array, and first match.",
			InputFields: []workflow.FieldMeta{
				{Name: "email", Type: "string", Description: "Email address to search for (partial match supported)", Required: true},
				{Name: "max_results", Type: "number", Description: "Maximum number of contacts to return (default: 10)"},
				{Name: "result_var", Type: "string", Description: "Context key to store the result under (default: \"contacts\")"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "contacts", Type: "object", Description: "Result stored under result_var. Contains: status, count, items (array of contact objects), first (first match). Each item has: cf_id, cf_public_id, email_address, first_name, last_name, phone_number, is_active, tags, custom_attributes."},
			},
		},
	)

	// cf_get_tags: fetches all contact tags defined in the ClickFunnels workspace.
	eng.RegisterActivityWithMeta("cf_get_tags",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			resultVar, _ := input["result_var"].(string)
			if resultVar == "" {
				resultVar = "tags"
			}

			cfClient := getCF()
			if cfClient == nil {
				return map[string]any{
					resultVar: map[string]any{
						"error":  "ClickFunnels client not configured (CF_API_KEY missing)",
						"status": "error",
						"count":  0,
						"items":  []any{},
					},
				}, nil
			}

			tags, err := cfClient.ListTags(ctx, "")
			if err != nil {
				return map[string]any{
					resultVar: map[string]any{
						"error":  err.Error(),
						"status": "error",
						"count":  0,
						"items":  []any{},
					},
				}, nil
			}

			items := make([]any, 0, len(tags))
			for _, t := range tags {
				items = append(items, map[string]any{
					"id":         t.ID,
					"public_id":  t.PublicID,
					"name":       t.Name,
					"color":      t.Color,
				})
			}

			return map[string]any{
				resultVar: map[string]any{
					"status": "ok",
					"count":  len(items),
					"items":  items,
				},
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Fetches all contact tags defined in the ClickFunnels workspace. Results stored under result_var (default: \"tags\") with count and items array.",
			InputFields: []workflow.FieldMeta{
				{Name: "result_var", Type: "string", Description: "Context key to store the result under (default: \"tags\")"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "tags", Type: "object", Description: "Result under result_var. Contains: status, count, items[]. Each item has: id, public_id, name, color."},
			},
		},
	)

	// cf_add_tag: applies one or more tags to a ClickFunnels contact.
	// tag_ids is a JSON array of tag IDs (ints), e.g. [123, 456].
	eng.RegisterActivityWithMeta("cf_add_tag",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			cfClient := getCF()
			if cfClient == nil {
				return nil, fmt.Errorf("cf_add_tag: ClickFunnels client not configured")
			}

			contactID := int(toFloat64OrZero(input["contact_id"]))
			if contactID == 0 {
				return nil, fmt.Errorf("cf_add_tag: contact_id is required")
			}

			tagIDs, err := parseIntSlice(input["tag_ids"])
			if err != nil || len(tagIDs) == 0 {
				return nil, fmt.Errorf("cf_add_tag: tag_ids must be a non-empty array of tag IDs")
			}

			var added []int
			var skipped []int
			var addErrs []string
			var appliedRecords []any
			for _, tagID := range tagIDs {
				applied, err := cfClient.AddAppliedTag(ctx, contactID, tagID)
				if err != nil {
					skipped = append(skipped, tagID)
					addErrs = append(addErrs, fmt.Sprintf("tag %d: %v", tagID, err))
				} else {
					added = append(added, tagID)
					if applied != nil {
						appliedRecords = append(appliedRecords, map[string]any{
							"id":         applied.ID,
							"tag_id":     applied.TagID,
							"tag_name":   applied.Tag.Name,
							"applied_at": applied.Tag.AppliedAt,
						})
					}
				}
			}

			out := map[string]any{
				"added":           added,
				"skipped":         skipped,
				"count":           len(added),
				"applied_records": appliedRecords,
			}
			if len(addErrs) > 0 {
				out["errors"] = addErrs
			}
			if rk, _ := input["result_key"].(string); rk != "" {
				return map[string]any{rk: out}, nil
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Applies one or more tags to a ClickFunnels contact. Use the tag picker to select tags.",
			InputFields: []workflow.FieldMeta{
				{Name: "contact_id", Type: "number", Description: "Numeric ClickFunnels contact ID", Required: true},
				{Name: "tag_ids", Type: "string", Description: "JSON array of tag IDs to apply, e.g. [123, 456]", Required: true},
				{Name: "result_key", Type: "string", Description: "Context key to store result under (e.g. \"cf_tag_result\")"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "added", Type: "array", Description: "Tag IDs successfully applied"},
				{Name: "skipped", Type: "array", Description: "Tag IDs that failed (already applied or error)"},
				{Name: "count", Type: "number", Description: "Number of tags successfully applied"},
				{Name: "applied_records", Type: "array", Description: "Raw CF response for each applied tag (id, tag_id, tag_name, applied_at)"},
			},
		},
	)

	// cf_remove_tag: removes one or more tags from a ClickFunnels contact.
	// Looks up the applied-tag record IDs first, then deletes each one.
	eng.RegisterActivityWithMeta("cf_remove_tag",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			cfClient := getCF()
			if cfClient == nil {
				return nil, fmt.Errorf("cf_remove_tag: ClickFunnels client not configured")
			}

			contactID := int(toFloat64OrZero(input["contact_id"]))
			if contactID == 0 {
				return nil, fmt.Errorf("cf_remove_tag: contact_id is required")
			}

			tagIDs, err := parseIntSlice(input["tag_ids"])
			if err != nil || len(tagIDs) == 0 {
				return nil, fmt.Errorf("cf_remove_tag: tag_ids must be a non-empty array of tag IDs")
			}

			// Fetch applied tags to map tag_id → applied_tag_id.
			applied, err := cfClient.ListAppliedTags(ctx, contactID)
			if err != nil {
				return nil, fmt.Errorf("cf_remove_tag: list applied tags: %w", err)
			}
			appliedByTagID := make(map[int]int, len(applied))
			for _, a := range applied {
				appliedByTagID[a.TagID] = a.ID
			}

			var removed, skipped []int
			for _, tagID := range tagIDs {
				appliedID, ok := appliedByTagID[tagID]
				if !ok {
					skipped = append(skipped, tagID) // tag not currently applied
					continue
				}
				if err := cfClient.RemoveAppliedTag(ctx, contactID, appliedID); err != nil {
					skipped = append(skipped, tagID)
				} else {
					removed = append(removed, tagID)
				}
			}

			out := map[string]any{
				"removed": removed,
				"skipped": skipped,
				"count":   len(removed),
			}
			if rk, _ := input["result_key"].(string); rk != "" {
				return map[string]any{rk: out}, nil
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Removes one or more tags from a ClickFunnels contact. Use the tag picker to select tags.",
			InputFields: []workflow.FieldMeta{
				{Name: "contact_id", Type: "number", Description: "Numeric ClickFunnels contact ID", Required: true},
				{Name: "tag_ids", Type: "string", Description: "JSON array of tag IDs to remove, e.g. [123, 456]", Required: true},
				{Name: "result_key", Type: "string", Description: "Context key to store result under (e.g. \"cf_remove_result\")"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "removed", Type: "array", Description: "Tag IDs successfully removed"},
				{Name: "skipped", Type: "array", Description: "Tag IDs not found on this contact or that failed"},
				{Name: "count", Type: "number", Description: "Number of tags successfully removed"},
			},
		},
	)

	eng.RegisterActivityWithMeta("cf_get_all",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			cfClient := getCF()
			if cfClient == nil {
				return nil, fmt.Errorf("cf_get_all: ClickFunnels client not configured (CF_API_KEY missing)")
			}
			table, _ := input["table"].(string)
			if table == "" {
				table = "contacts"
			}

			chunkSize := 50
			if cs := int(toFloat64OrZero(input["chunk_size"])); cs > 0 {
				chunkSize = cs
			}

			// Read cursor from context (set by previous chunk; 0 = first page)
			cursor := int(toFloat64OrZero(input["cf_cursor"]))

			switch table {
			case "contacts":
				var tagIDs []int
				if raw, ok := input["tag_ids"].([]any); ok {
					for _, v := range raw {
						if id := int(toFloat64OrZero(v)); id > 0 {
							tagIDs = append(tagIDs, id)
						}
					}
				}

				fetchCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				page, err := cfClient.ListContacts(fetchCtx, cursor, chunkSize, tagIDs)
				if err != nil {
					return nil, fmt.Errorf("cf_get_all contacts: %w", err)
				}

				fmt.Printf("[cf_get_all] fetched %d contacts (cursor=%d chunk_size=%d)\n", len(page), cursor, chunkSize)

				records := make([]any, 0, len(page))
				for _, c := range page {
					m := map[string]any{
						"cf_id":             c.ID,
						"cf_public_id":      c.PublicID,
						"is_active":         c.IsActive,
						"tags":              c.Tags,
						"custom_attributes": c.CustomAttributes,
					}
					if c.EmailAddress != nil { m["email_address"] = *c.EmailAddress }
					if c.FirstName != nil    { m["first_name"] = *c.FirstName }
					if c.LastName != nil     { m["last_name"] = *c.LastName }
					if c.PhoneNumber != nil  { m["phone_number"] = *c.PhoneNumber }
					records = append(records, m)
				}

				hasMore := len(page) == chunkSize
				nextCursor := 0
				if hasMore {
					nextCursor = page[len(page)-1].ID
				}

				out := map[string]any{
					"records":     records,
					"count":       len(records),
					"cf_cursor":   nextCursor,
					"cf_has_more": hasMore,
				}
				if rk, _ := input["result_key"].(string); rk != "" {
					return map[string]any{rk: out}, nil
				}
				return out, nil

			default:
				return nil, fmt.Errorf("cf_get_all: unsupported table %q (supported: contacts)", table)
			}
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Fetches one chunk of records from a ClickFunnels table. Outputs cf_cursor and cf_has_more so a cf_next_chunk node can loop back for the next chunk.",
			InputFields: []workflow.FieldMeta{
				{Name: "table", Type: "string", Description: "ClickFunnels table to fetch: contacts (default)", Required: true},
				{Name: "chunk_size", Type: "number", Description: "Records per chunk (default 50)"},
				{Name: "cf_cursor", Type: "number", Description: "Pagination cursor — leave blank; set automatically by previous chunk"},
				{Name: "tag_ids", Type: "array", Description: "Optional tag IDs to filter contacts by"},
				{Name: "result_key", Type: "string", Description: "Context key to nest results under"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "records", Type: "array", Description: "Records in this chunk — wire into a loop node"},
				{Name: "count", Type: "number", Description: "Number of records in this chunk"},
				{Name: "cf_cursor", Type: "number", Description: "Cursor for the next chunk (0 when done)"},
				{Name: "cf_has_more", Type: "boolean", Description: "True if more chunks remain — use with cf_next_chunk"},
			},
		},
	)

	// cf_next_chunk: closing node for chunked cf_get_all loops.
	// Routes back to cf_get_all when cf_has_more=true, or forwards when false.
	// Connect: cf_next_chunk → cf_get_all (condition: cf_has_more=true)
	//          cf_next_chunk → next step  (condition: cf_has_more=false or default)
	eng.RegisterActivityWithMeta("cf_next_chunk",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			hasMore, _ := input["cf_has_more"].(bool)
			cursor := int(toFloat64OrZero(input["cf_cursor"]))
			fmt.Printf("[cf_next_chunk] cf_has_more=%v cf_cursor=%d\n", hasMore, cursor)
			return map[string]any{
				"cf_has_more": hasMore,
				"cf_cursor":   cursor,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Closing node for chunked cf_get_all loops. Routes back to cf_get_all if cf_has_more=true, or continues forward when false.",
			InputFields: []workflow.FieldMeta{
				{Name: "cf_has_more", Type: "boolean", Description: "Automatically set by cf_get_all — do not set manually"},
				{Name: "cf_cursor", Type: "number", Description: "Automatically set by cf_get_all — do not set manually"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "cf_has_more", Type: "boolean", Description: "True = more chunks; False = all done"},
				{Name: "cf_cursor", Type: "number", Description: "Cursor passed back for next cf_get_all call"},
			},
		},
	)

	// mapper: resolves dot-notation paths from a source object into top-level context keys.
	eng.RegisterActivityWithMeta("mapper",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			sourceKey, _ := input["source_key"].(string)
			if sourceKey == "" {
				sourceKey = "json"
			}

			source, _ := input[sourceKey].(map[string]any)

			rawMappings, ok := input["mappings"]
			if !ok || rawMappings == nil {
				return nil, fmt.Errorf("mapper: mappings is required")
			}

			// mappings may arrive as []any (from JSON unmarshaling) or []map[string]any.
			mappingsRaw, ok := rawMappings.([]any)
			if !ok {
				return nil, fmt.Errorf("mapper: mappings must be an array of objects")
			}

			out := make(map[string]any)
			for i, m := range mappingsRaw {
				entry, ok := m.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("mapper: mappings[%d] must be an object with 'from' and 'to' fields", i)
				}
				from, _ := entry["from"].(string)
				to, _ := entry["to"].(string)
				if from == "" || to == "" {
					return nil, fmt.Errorf("mapper: mappings[%d] must have non-empty 'from' and 'to' fields", i)
				}
				out[to] = resolveDotPath(source, from)
			}

			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Maps dot-notation paths from a source context object to new top-level context keys",
			InputFields: []workflow.FieldMeta{
				{Name: "source_key", Type: "string", Description: "Context key holding the source object (default: 'json')"},
				{Name: "mappings", Type: "array", Description: "Array of {from, to} objects. 'from' is a dot-notation path into the source; 'to' is the output key name.", Required: true},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "*", Type: "any", Description: "One key per mapping entry, named by 'to', containing the resolved value"},
			},
		},
	)

	init_birth_converters(eng)
	registerLLMActivity(eng, app, logger)
}

// ── format_birth_date ─────────────────────────────────────────────────────────
// Converts various date string formats to YYYY-MM-DD.
// Handles: "1985-02-08", "02/08/1985", "2/8/1985", "February 8, 1985", "Feb 8 1985", etc.

// ── format_birth_time ─────────────────────────────────────────────────────────
// Converts 12-hour or 24-hour time strings to HH:MM (24-hour).
// Handles: "1:02 AM", "1:02:30 PM", "13:02", "1:02AM", etc.

func init_birth_converters(eng *workflow.Engine) {
	eng.RegisterActivityWithMeta("format_birth_date",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			raw, _ := input["date"].(string)
			if raw == "" {
				if v, ok := input["birthday"].(string); ok {
					raw = v
				}
			}
			if raw == "" {
				return nil, fmt.Errorf("format_birth_date: 'date' or 'birthday' input required")
			}
			formats := []string{
				"2006-01-02",
				"01/02/2006", "1/2/2006", "01/02/06", "1/2/06",
				"January 2, 2006", "Jan 2, 2006", "Jan 2 2006",
				"January 2 2006",
				"2 January 2006", "2 Jan 2006",
			}
			for _, f := range formats {
				if t, err := time.Parse(f, raw); err == nil {
					return map[string]any{"birth_date": t.Format("2006-01-02")}, nil
				}
			}
			return nil, fmt.Errorf("format_birth_date: unrecognized date format %q", raw)
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Converts a date string (various formats) to YYYY-MM-DD",
			InputFields: []workflow.FieldMeta{
				{Name: "date", Type: "string", Description: "Date string — also accepts 'birthday' key. Supports YYYY-MM-DD, MM/DD/YYYY, 'Month D YYYY', etc.", Required: true},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "birth_date", Type: "string", Description: "Date in YYYY-MM-DD format"},
			},
		},
	)

	eng.RegisterActivityWithMeta("format_birth_time",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			raw, _ := input["time"].(string)
			if raw == "" {
				if v, ok := input["birth_time"].(string); ok {
					raw = v
				}
			}
			if raw == "" {
				return nil, fmt.Errorf("format_birth_time: 'time' or 'birth_time' input required")
			}
			raw = strings.TrimSpace(raw)
			// Try 24-hour formats first (already correct, just normalize)
			for _, f := range []string{"15:04:05", "15:04"} {
				if t, err := time.Parse(f, raw); err == nil {
					return map[string]any{"birth_time": t.Format("15:04")}, nil
				}
			}
			// Normalize: remove spaces before AM/PM for time.Parse
			normalized := strings.ToUpper(raw)
			normalized = strings.ReplaceAll(normalized, " AM", "AM")
			normalized = strings.ReplaceAll(normalized, " PM", "PM")
			for _, f := range []string{"3:04:05PM", "3:04PM", "3:04:05 PM", "3:04 PM"} {
				if t, err := time.Parse(f, normalized); err == nil {
					return map[string]any{"birth_time": t.Format("15:04")}, nil
				}
				if t, err := time.Parse(f, raw); err == nil {
					return map[string]any{"birth_time": t.Format("15:04")}, nil
				}
			}
			return nil, fmt.Errorf("format_birth_time: unrecognized time format %q — expected formats like '1:02 AM', '13:02', '1:02:30 PM'", raw)
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Converts a time string (12h or 24h) to HH:MM in 24-hour format",
			InputFields: []workflow.FieldMeta{
				{Name: "time", Type: "string", Description: "Time string — also accepts 'birth_time' key. Supports '1:02 AM', '1:02:30 PM', '13:02', etc.", Required: true},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "birth_time", Type: "string", Description: "Time in HH:MM 24-hour format"},
			},
		},
	)
}

// resolveDotPath walks a nested map[string]any using a dot-separated path string.
// Returns nil if any segment is missing or the value is not a nested map.
func resolveDotPath(obj map[string]any, path string) any {
	if obj == nil {
		return nil
	}
	parts := strings.SplitN(path, ".", 2)
	val, ok := obj[parts[0]]
	if !ok {
		return nil
	}
	if len(parts) == 1 {
		return val
	}
	nested, ok := val.(map[string]any)
	if !ok {
		return nil
	}
	return resolveDotPath(nested, parts[1])
}

// ── template_fill ─────────────────────────────────────────────────────────────

func registerTemplateFill(eng *workflow.Engine) {
	eng.RegisterActivityWithMeta("template_fill",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			tmpl, _ := input["template"].(string)
			if tmpl == "" {
				return nil, fmt.Errorf("template_fill: template is required")
			}
			resultVar, _ := input["result_var"].(string)
			if resultVar == "" {
				resultVar = "filled_content"
			}

			// Replace {{key}} tokens using all string-valued context entries.
			// The workflow engine already interpolated any {{var}} reference in
			// the template field itself, so tmpl now contains the actual template
			// text with {{placeholder}} tokens waiting to be filled.
			skip := map[string]bool{"template": true, "result_var": true}
			result := tmpl
			for k, v := range input {
				if skip[k] || len(k) == 0 || k[0] == '_' {
					continue
				}
				if s, ok := v.(string); ok {
					result = strings.ReplaceAll(result, "{{"+k+"}}", s)
				}
			}
			return map[string]any{resultVar: result}, nil
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Fills a text template by replacing {{placeholder}} tokens with workflow context values. Set template to {{var_name}} to use a variable fetched earlier (e.g. from gdrive_get_file).",
			InputFields: []workflow.FieldMeta{
				{Name: "template", Type: "string", Required: true, Description: "Template text with {{placeholder}} tokens, or {{var_name}} to pull content from a context variable."},
				{Name: "result_var", Type: "string", Description: "Variable name to store the filled result in. Defaults to filled_content."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "<result_var>", Type: "string", Description: "The filled template text, stored under the name provided in result_var (default: filled_content)."},
			},
		},
	)
}

// ── text_substitute ────────────────────────────────────────────────────────────

func registerTextSubstitute(eng *workflow.Engine) {
	eng.RegisterActivityWithMeta("text_substitute",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			source, _ := input["source"].(string)
			if source == "" {
				return nil, fmt.Errorf("text_substitute: source is required")
			}
			resultVar, _ := input["result_var"].(string)
			if resultVar == "" {
				resultVar = "substituted"
			}

			// Auto-dereference: if source is a bare context key name (e.g. "cc_rr" instead of "{{cc_rr}}"),
			// look it up in the input map directly and default result_var to that same key.
			if val, ok := input[source]; ok {
				if s, ok2 := val.(string); ok2 && s != source {
					log.Printf("[text_substitute] auto-dereferencing source key %q", source)
					if resultVar == "substituted" {
						resultVar = source // write result back to same key
					}
					source = s
				}
			}

			// Normalize markdown-escaped placeholders: {{about\_your\_business}} → {{about_your_business}}
			escapedPlaceholder := regexp.MustCompile(`\{\{([^}]+)\}\}`)
			result := escapedPlaceholder.ReplaceAllStringFunc(source, func(match string) string {
				inner := match[2 : len(match)-2] // strip {{ and }}
				return "{{" + strings.ReplaceAll(inner, `\_`, "_") + "}}"
			})

			// Build vars map from input["vars"] — may be a JSON string or already-parsed map.
			vars := map[string]string{}
			switch v := input["vars"].(type) {
			case string:
				if err := json.Unmarshal([]byte(v), &vars); err != nil {
					log.Printf("[text_substitute] failed to parse vars JSON: %v", err)
				}
			case map[string]any:
				for k, val := range v {
					vars[k] = fmt.Sprintf("%v", val)
				}
			}

			log.Printf("[text_substitute] source len=%d vars=%v", len(result), vars)
			replaced := 0
			for k, val := range vars {
				before := result
				result = strings.ReplaceAll(result, "{{"+k+"}}", val)
				if result != before {
					replaced++
				}
			}
			log.Printf("[text_substitute] replaced %d/%d vars; result len=%d", replaced, len(vars), len(result))

			return map[string]any{resultVar: result}, nil
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Replaces {{placeholder}} tokens in a source text with mapped values. Use the Substitution Builder to map template variables to context keys.",
			InputFields: []workflow.FieldMeta{
				{Name: "source", Type: "string", Required: true, Description: "Template text with {{placeholder}} tokens. Supports {{ctx_var}} to pull from context."},
				{Name: "vars", Type: "json", Description: "JSON object mapping placeholder name to value. Engine interpolates {{ctx_key}} inside this string."},
				{Name: "result_var", Type: "string", Description: "Variable name to store the substituted result in. Defaults to substituted."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "<result_var>", Type: "string", Description: "The text after all substitutions, stored under the name provided in result_var (default: substituted)."},
			},
		},
	)
}

func registerFormatDate(eng *workflow.Engine) {
	parsLayouts := []string{
		"2006-01-02 15:04:05.000Z",
		"2006-01-02 15:04:05.999Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02",
		"01/02/2006",
		"January 2, 2006",
	}

	eng.RegisterActivityWithMeta("format_date",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			keyName, _ := input["value"].(string)
			if keyName == "" {
				return nil, fmt.Errorf("format_date: value (context key) is required")
			}
			format, _ := input["format"].(string)
			if format == "" {
				return nil, fmt.Errorf("format_date: format is required")
			}

			// Resolve the value via dot-path traversal (e.g. "pb_contact.records.0.birthday").
			raw := resolveNestedInput(input, keyName)

			var t time.Time
			switch v := raw.(type) {
			case time.Time:
				t = v
			case string:
				if v == "" {
					return nil, fmt.Errorf("format_date: context key %q is empty", keyName)
				}
				var parseErr error
				for _, layout := range parsLayouts {
					t, parseErr = time.Parse(layout, v)
					if parseErr == nil {
						break
					}
				}
				if parseErr != nil {
					return nil, fmt.Errorf("format_date: could not parse %q as a date", v)
				}
			default:
				return nil, fmt.Errorf("format_date: context key %q is not a string or time.Time (got %T)", keyName, raw)
			}

			setNestedInput(input, keyName, t.Format(format))
			// Return the top-level key so the engine merges the mutated structure back.
			topKey := strings.SplitN(keyName, ".", 2)[0]
			return map[string]any{topKey: input[topKey]}, nil
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Parses a date string from a context key and reformats it in-place using a Go layout string.",
			InputFields: []workflow.FieldMeta{
				{Name: "value", Type: "context_key", Required: true, Description: "Context key containing the date string to reformat (e.g. birthday)."},
				{Name: "format", Type: "string", Required: true, Description: "Go time layout string for output. Common formats: \"January 2, 2006\" · \"01/02/2006\" · \"02 Jan 2006\" · \"2006-01-02\" · \"Jan 2, 2006\" · \"Monday, January 2, 2006\""},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "<value>", Type: "string", Description: "The reformatted date written back to the same context key that was selected."},
			},
		},
	)
}

// resolveNestedInput looks up a dot-notation key (e.g. "pb_contact.records.0.birthday")
// from the activity input map, traversing nested maps and slices as needed.
func resolveNestedInput(input map[string]any, key string) any {
	if v, ok := input[key]; ok {
		return v
	}
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return nil
	}
	var cur any = map[string]any(input)
	for _, p := range parts {
		switch v := cur.(type) {
		case map[string]any:
			val, ok := v[p]
			if !ok {
				return nil
			}
			cur = val
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(v) {
				return nil
			}
			cur = v[i]
		default:
			return nil
		}
	}
	return cur
}

// setNestedInput writes value to a dot-notation path inside the input map,
// mutating nested maps/slices in-place (they share references with the workflow context).
func setNestedInput(input map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		input[key] = value
		return
	}
	var cur any = map[string]any(input)
	for i, p := range parts {
		isLast := i == len(parts)-1
		switch v := cur.(type) {
		case map[string]any:
			if isLast {
				v[p] = value
				return
			}
			cur = v[p]
		case []any:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(v) {
				return
			}
			if isLast {
				v[idx] = value
				return
			}
			cur = v[idx]
		default:
			return
		}
	}
}

// ── Graphs ────────────────────────────────────────────────────────────────────

func registerGraphs(eng *workflow.Engine, getCF func() *clickfunnels.Client) error {
	// demo_conditional: demonstrates conditional branching based on "value".
	//
	//  start (echo)
	//    → increment (echo_plus_1, sleep 1-10s)
	//        if value >= 3  → finish_high (echo)  → [end]
	//        else           → increment_again (echo_plus_1, sleep 1-10s)
	//                           → finish_low (echo) → [end]
	//
	// Try with {"value": 1} for the low path, {"value": 3} for the high path.

	if err := eng.RegisterGraph(&workflow.ActivityGraph{
		Name:      "demo_conditional",
		StartNode: "start",
		Nodes: map[string]*workflow.Node{
			"start": {
				ID:           "start",
				ActivityName: "echo",
				Transitions: []workflow.Transition{
					{NextNode: "increment"},
				},
			},
			"increment": {
				ID:           "increment",
				ActivityName: "echo_plus_1",
				MaxRetries:   2,
				Transitions: []workflow.Transition{
					{
						Label:      "value ≥ 3 → high",
						Conditions: []workflow.Condition{{Key: "value", Operator: "gte", Value: float64(3)}},
						NextNode:   "finish_high",
					},
					{
						Label:    "else → low",
						NextNode: "increment_again",
					},
				},
			},
			"increment_again": {
				ID:           "increment_again",
				ActivityName: "echo_plus_1",
				MaxRetries:   2,
				Transitions: []workflow.Transition{
					{NextNode: "finish_low"},
				},
			},
			"finish_high": {
				ID:           "finish_high",
				ActivityName: "echo",
				Transitions: []workflow.Transition{
					{NextNode: ""}, // end
				},
			},
			"finish_low": {
				ID:           "finish_low",
				ActivityName: "echo",
				Transitions: []workflow.Transition{
					{NextNode: ""}, // end
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("demo_conditional: %w", err)
	}

	// demo_linear: simple straight-line graph with 3 increment steps.
	// Good for watching retry behaviour and timing.
	if err := eng.RegisterGraph(&workflow.ActivityGraph{
		Name:      "demo_linear",
		StartNode: "step_1",
		Nodes: map[string]*workflow.Node{
			"step_1": {
				ID:           "step_1",
				ActivityName: "echo_plus_1",
				MaxRetries:   1,
				Transitions:  []workflow.Transition{{NextNode: "step_2"}},
			},
			"step_2": {
				ID:           "step_2",
				ActivityName: "echo_plus_1",
				MaxRetries:   1,
				Transitions:  []workflow.Transition{{NextNode: "step_3"}},
			},
			"step_3": {
				ID:           "step_3",
				ActivityName: "echo_plus_1",
				MaxRetries:   1,
				Transitions:  []workflow.Transition{{NextNode: ""}},
			},
		},
	}); err != nil {
		return fmt.Errorf("demo_linear: %w", err)
	}

	// demo_human: pauses for human input between two steps.
	if err := eng.RegisterGraph(&workflow.ActivityGraph{
		Name:      "demo_human",
		StartNode: "prepare",
		Nodes: map[string]*workflow.Node{
			"prepare": {
				ID:           "prepare",
				ActivityName: "echo_plus_1",
				MaxRetries:   1,
				Transitions:  []workflow.Transition{{NextNode: "review"}},
			},
			"review": {
				ID:           "review",
				ActivityName: "echo",
				IsHuman:      true,
				Transitions: []workflow.Transition{
					{
						Label:      "approved → finish",
						Conditions: []workflow.Condition{workflow.Eq("approved", true)},
						NextNode:   "finish_approved",
					},
					{
						Label:    "rejected → end",
						NextNode: "finish_rejected",
					},
				},
			},
			"finish_approved": {
				ID:           "finish_approved",
				ActivityName: "echo",
				Transitions:  []workflow.Transition{{NextNode: ""}},
			},
			"finish_rejected": {
				ID:           "finish_rejected",
				ActivityName: "echo",
				Transitions:  []workflow.Transition{{NextNode: ""}},
			},
		},
	}); err != nil {
		return fmt.Errorf("demo_human: %w", err)
	}

	// birth_process: triggered by the birth form webhook.
	// 1. get_contact  — fetch full contact from ClickFunnels by contact_id
	// 2. save_contact — upsert the contact into the cf_contacts PocketBase collection
	if err := eng.RegisterGraph(&workflow.ActivityGraph{
		Name:      "birth_process",
		StartNode: "get_contact",
		Nodes: map[string]*workflow.Node{
			"get_contact": {
				ID:           "get_contact",
				ActivityName: "cf_get_contact",
				MaxRetries:   2,
				Transitions:  []workflow.Transition{{NextNode: "save_contact"}},
			},
			"save_contact": {
				ID:           "save_contact",
				ActivityName: "cf_upsert_contact",
				MaxRetries:   2,
				Transitions:  []workflow.Transition{{NextNode: ""}},
			},
		},
	}); err != nil {
		return fmt.Errorf("birth_process: %w", err)
	}

	return nil
}

// pbQueryOp maps a UI operator name to a PocketBase filter operator.
func pbQueryOp(op string) string {
	switch op {
	case "eq":
		return "="
	case "neq":
		return "!="
	case "gt":
		return ">"
	case "gte":
		return ">="
	case "lt":
		return "<"
	case "lte":
		return "<="
	case "contains":
		return "~"
	case "not_contains":
		return "!~"
	default:
		return "="
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func inputKeys(m map[string]any) []string { return mapKeys(m) }

func toFloat64OrZero(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f
		}
	}
	return 0
}

// parseIntSlice accepts a []any (from JSON decode) or a JSON string like "[1,2,3]"
// and returns a slice of ints.
func parseIntSlice(v any) ([]int, error) {
	switch val := v.(type) {
	case []any:
		out := make([]int, 0, len(val))
		for _, item := range val {
			out = append(out, int(toFloat64OrZero(item)))
		}
		return out, nil
	case string:
		var raw []any
		if err := json.Unmarshal([]byte(val), &raw); err != nil {
			return nil, err
		}
		out := make([]int, 0, len(raw))
		for _, item := range raw {
			out = append(out, int(toFloat64OrZero(item)))
		}
		return out, nil
	}
	return nil, fmt.Errorf("parseIntSlice: unsupported type %T", v)
}

var templateRe = regexp.MustCompile(`\{\{([\w.]+)\}\}`)

// interpolateTemplate replaces {{path}} placeholders with values from data.
// Supports dot-notation and array indexing: {{results.0.lat}}, {{custom_attributes.birthday}}
func interpolateTemplate(s string, data map[string]any) string {
	return templateRe.ReplaceAllStringFunc(s, func(match string) string {
		path := match[2 : len(match)-2] // strip {{ }}
		// Strip |type hint suffix(es) (e.g. {{price|number|number}} → look up "price")
		if idx := strings.Index(path, "|"); idx != -1 {
			path = path[:idx]
		}
		val := resolvePath(data, path)
		if val == nil {
			return match // leave unreplaced if not found
		}
		return fmt.Sprintf("%v", val)
	})
}

// resolvePath walks dot-separated path through map[string]any and []any.
// coerceJSONTypes walks a decoded JSON value and converts any string that looks
// like a number or boolean into its native Go type. This fixes the case where
// the HTTP body builder wraps template placeholders in quotes ("{{lat}}" →
// "47.6") that should be sent as numbers/booleans.
func coerceJSONTypes(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			val[k] = coerceJSONTypes(child)
		}
		return val
	case []any:
		for i, child := range val {
			val[i] = coerceJSONTypes(child)
		}
		return val
	case string:
		if val == "true" {
			return true
		}
		if val == "false" {
			return false
		}
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
		return val
	default:
		return v
	}
}

// e.g. "results.0.lat" on {"results": [{"lat": 47.6}]} returns 47.6
func resolvePath(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var cur any = data
	for _, p := range parts {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[p]
		case []any:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil
			}
			cur = v[idx]
		default:
			return nil
		}
	}
	return cur
}

// ── run_workflow ───────────────────────────────────────────────────────────────

// registerRunWorkflow registers the "run_workflow" activity which synchronously
// runs a named sub-workflow and waits for it to complete, merging its output
// context back as this node's output.
func registerRunWorkflow(eng *workflow.Engine) {
	eng.RegisterActivityWithMeta("run_workflow",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			graphName, _ := input["graph_name"].(string)
			if graphName == "" {
				return nil, fmt.Errorf("run_workflow: graph_name is required")
			}

			// Build initial context for the sub-workflow.
			inputCtx, _ := input["input"].(map[string]any)

			subWF, err := eng.CreateWorkflow(ctx, "", graphName, inputCtx, nil)
			if err != nil {
				return nil, fmt.Errorf("run_workflow: failed to create sub-workflow %q: %w", graphName, err)
			}

			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-ticker.C:
					updated, err := eng.Store().GetWorkflow(ctx, subWF.ID)
					if err != nil {
						return nil, fmt.Errorf("run_workflow: failed to poll sub-workflow %s: %w", subWF.ID, err)
					}
					switch updated.Status {
					case workflow.StatusFailed:
						return nil, fmt.Errorf("sub-workflow %s (%s) failed", subWF.ID, graphName)
					case workflow.StatusCompleted:
						return updated.Context, nil
					}
				}
			}
		},
		workflow.ActivityMeta{
			Category:    "Utility",
			Description: "Run another workflow and wait for it to complete. The sub-workflow's output context is merged back as this node's output.",
			InputFields: []workflow.FieldMeta{
				{Name: "graph_name", Type: "workflow_graph", Required: true, Description: "Name of the workflow graph to run"},
				{Name: "input", Type: "object", Required: false, Description: "Extra context to pass into the sub-workflow (merged with current context)"},
			},
		},
	)
}

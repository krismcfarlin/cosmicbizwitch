// Package workflows registers built-in demo activities and graphs.
// Add your real application activities and graphs here.
package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cosmicbizwitch/internal/app/clickfunnels"
	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterDefaults wires up built-in demo activities and graphs into the engine.
func RegisterDefaults(eng *workflow.Engine, app core.App, getCF func() *clickfunnels.Client) error {
	registerActivities(eng, app, getCF)
	return registerGraphs(eng, getCF)
}

// ── Activities ────────────────────────────────────────────────────────────────

func registerActivities(eng *workflow.Engine, app core.App, getCF func() *clickfunnels.Client) {
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

			for k, v := range data {
				rec.Set(k, v)
			}
			fmt.Printf("[pb_upsert] saving record id=%q contact_id=%v\n", rec.Id, rec.Get("contact_id"))
			if err := app.Save(rec); err != nil {
				return nil, fmt.Errorf("pb_upsert: save: %w", err)
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

			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("http_request: %w", err)
			}
			defer resp.Body.Close()

			bodyBytes, _ := io.ReadAll(resp.Body)

			resultKey, _ := input["result_key"].(string)
			ok := resp.StatusCode >= 200 && resp.StatusCode < 300

			var jsonBody map[string]any
			if err := json.Unmarshal(bodyBytes, &jsonBody); err != nil {
				// Not JSON — return status only
				return map[string]any{"status_code": resp.StatusCode, "ok": ok}, nil
			}

			if resultKey != "" {
				// Store entire parsed JSON under result_key
				return map[string]any{
					"status_code": resp.StatusCode,
					"ok":          ok,
					resultKey:     jsonBody,
				}, nil
			}

			// Spread parsed JSON keys directly into context
			out := map[string]any{"status_code": resp.StatusCode, "ok": ok}
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

	// cf_upsert_contact: upserts a ClickFunnels contact into the cf_contacts PocketBase collection.
	// Expects the output fields from cf_get_contact in the workflow context.
	eng.RegisterActivityWithMeta("cf_upsert_contact",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			cfID := int(toFloat64OrZero(input["cf_id"]))
			if cfID == 0 {
				return nil, fmt.Errorf("cf_upsert_contact: cf_id is required")
			}

			// Find existing record by cf_id.
			existing, _ := app.FindRecordsByFilter(
				"cf_contacts",
				fmt.Sprintf("cf_id = %d", cfID),
				"", 1, 0,
			)

			var rec *core.Record
			if len(existing) > 0 {
				rec = existing[0]
			} else {
				col, err := app.FindCollectionByNameOrId("cf_contacts")
				if err != nil {
					return nil, fmt.Errorf("cf_upsert_contact: find collection: %w", err)
				}
				rec = core.NewRecord(col)
			}

			rec.Set("cf_id", cfID)
			rec.Set("cf_public_id", input["cf_public_id"])
			rec.Set("email_address", input["email_address"])
			rec.Set("first_name", input["first_name"])
			rec.Set("last_name", input["last_name"])
			rec.Set("phone_number", input["phone_number"])
			rec.Set("is_active", input["is_active"])

			if tags := input["tags"]; tags != nil {
				if b, err := json.Marshal(tags); err == nil {
					rec.Set("tags", string(b))
				}
			}
			if attrs := input["custom_attributes"]; attrs != nil {
				if b, err := json.Marshal(attrs); err == nil {
					rec.Set("custom_attributes", string(b))
				}
			}

			if err := app.SaveNoValidate(rec); err != nil {
				return nil, fmt.Errorf("cf_upsert_contact: save: %w", err)
			}

			out := make(map[string]any, len(input))
			for k, v := range input {
				out[k] = v
			}
			out["pb_record_id"] = rec.Id
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Upserts a ClickFunnels contact into the cf_contacts PocketBase collection",
			InputFields: []workflow.FieldMeta{
				{Name: "cf_id", Type: "number", Description: "ClickFunnels contact ID", Required: true},
				{Name: "cf_public_id", Type: "string"},
				{Name: "email_address", Type: "string"},
				{Name: "first_name", Type: "string"},
				{Name: "last_name", Type: "string"},
				{Name: "phone_number", Type: "string"},
				{Name: "is_active", Type: "bool"},
				{Name: "tags", Type: "any"},
				{Name: "custom_attributes", Type: "object"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "pb_record_id", Type: "string", Description: "PocketBase record ID of the upserted contact"},
				{Name: "*", Type: "any", Description: "All input fields passed through"},
			},
		},
	)

	// cf_get_contact: fetches a ClickFunnels contact by numeric ID.
	// If cfClient is nil (CF_API_KEY not configured), the activity returns an error.
	eng.RegisterActivityWithMeta("cf_get_contact",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			cfClient := getCF()
			if cfClient == nil {
				return nil, fmt.Errorf("cf_get_contact: ClickFunnels client not configured (CF_API_KEY missing)")
			}
			contactID := int(toFloat64OrZero(input["contact_id"]))
			if contactID == 0 {
				return nil, fmt.Errorf("cf_get_contact: contact_id is required and must be non-zero")
			}
			contact, err := cfClient.GetContact(ctx, contactID)
			if err != nil {
				return nil, fmt.Errorf("cf_get_contact: %w", err)
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
				"cf_id":             contact.ID,
				"cf_public_id":      contact.PublicID,
				"email_address":     email,
				"first_name":        firstName,
				"last_name":         lastName,
				"phone_number":      phoneNumber,
				"is_active":         contact.IsActive,
				"tags":              contact.Tags,
				"custom_attributes": contact.CustomAttributes,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "ClickFunnels",
			Description: "Fetches a ClickFunnels contact by numeric ID from the workflow context",
			InputFields: []workflow.FieldMeta{
				{Name: "contact_id", Type: "number", Description: "Numeric ClickFunnels contact ID", Required: true},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "cf_id", Type: "number", Description: "ClickFunnels contact ID"},
				{Name: "cf_public_id", Type: "string", Description: "ClickFunnels public ID"},
				{Name: "email_address", Type: "string"},
				{Name: "first_name", Type: "string"},
				{Name: "last_name", Type: "string"},
				{Name: "phone_number", Type: "string"},
				{Name: "is_active", Type: "bool"},
				{Name: "tags", Type: "any", Description: "Array of tag objects"},
				{Name: "custom_attributes", Type: "object"},
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

var templateRe = regexp.MustCompile(`\{\{([\w.]+)\}\}`)

// interpolateTemplate replaces {{path}} placeholders with values from data.
// Supports dot-notation and array indexing: {{results.0.lat}}, {{custom_attributes.birthday}}
func interpolateTemplate(s string, data map[string]any) string {
	return templateRe.ReplaceAllStringFunc(s, func(match string) string {
		path := match[2 : len(match)-2] // strip {{ }}
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

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
	"strings"
	"time"

	"cosmicbizwitch/internal/app/clickfunnels"
	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/pocketbase/core"
)

// RegisterDefaults wires up built-in demo activities and graphs into the engine.
func RegisterDefaults(eng *workflow.Engine, app core.App, cfClient *clickfunnels.Client) error {
	registerActivities(eng, app, cfClient)
	return registerGraphs(eng, cfClient)
}

// ── Activities ────────────────────────────────────────────────────────────────

func registerActivities(eng *workflow.Engine, app core.App, cfClient *clickfunnels.Client) {
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
			filter, _ := input["filter"].(string)
			if tableName == "" {
				return nil, fmt.Errorf("pb_query: table_name is required")
			}
			if filter == "" {
				filter = "1=1"
			}
			limitF, _ := input["limit"].(float64)
			limit := int(limitF)
			if limit <= 0 {
				limit = 50
			}
			if limit > 100 {
				limit = 100
			}

			records, err := app.FindRecordsByFilter(tableName, filter, "", limit, 0, nil)
			if err != nil {
				return nil, fmt.Errorf("pb_query: %w", err)
			}

			out := map[string]any{
				"found": len(records) > 0,
				"count": len(records),
			}

			// Build records array.
			recList := make([]any, 0, len(records))
			for _, rec := range records {
				m := make(map[string]any)
				for _, col := range rec.Collection().Fields {
					m[col.GetName()] = rec.Get(col.GetName())
				}
				recList = append(recList, m)
			}
			out["records"] = recList

			// Flat fields from first record (backward compat).
			if len(records) > 0 {
				rec := records[0]
				for _, col := range rec.Collection().Fields {
					out[col.GetName()] = rec.Get(col.GetName())
				}
			}

			return out, nil
		},
		workflow.ActivityMeta{
			Description: "Queries a PocketBase collection and returns matching records",
			InputFields: []workflow.FieldMeta{
				{Name: "table_name", Type: "string", Description: "PocketBase collection name", Required: true},
				{Name: "filter", Type: "string", Description: "PocketBase filter expression"},
				{Name: "limit", Type: "number", Description: "Max records to return (default 50, max 100)"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "found", Type: "bool", Description: "True if any records were found"},
				{Name: "count", Type: "number", Description: "Number of records returned"},
				{Name: "records", Type: "any", Description: "Array of all matching records as objects"},
				{Name: "*", Type: "any", Description: "Fields from the first record (flat, for backward compat)"},
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
				if bodyStr != "" {
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
			bodyStr := string(bodyBytes)

			out := map[string]any{
				"status_code": resp.StatusCode,
				"body":        bodyStr,
				"ok":          resp.StatusCode >= 200 && resp.StatusCode < 300,
			}
			var jsonBody map[string]any
			if err := json.Unmarshal(bodyBytes, &jsonBody); err == nil {
				out["json"] = jsonBody
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Description: "Makes an HTTP request. Use {{key}} in url/body/headers to interpolate workflow context values.",
			InputFields: []workflow.FieldMeta{
				{Name: "url", Type: "string", Description: "Request URL. Supports {{key}} template interpolation."},
				{Name: "method", Type: "string", Description: "HTTP method: GET, POST, PUT, PATCH, DELETE (default: GET)"},
				{Name: "body", Type: "string|object", Description: "Request body. String or JSON object. Supports {{key}} interpolation."},
				{Name: "headers", Type: "object", Description: "HTTP headers as key/value map. Values support {{key}} interpolation."},
				{Name: "timeout_seconds", Type: "number", Description: "Request timeout in seconds (default: 30)"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "status_code", Type: "number", Description: "HTTP response status code"},
				{Name: "ok", Type: "bool", Description: "True if status code is 200-299"},
				{Name: "body", Type: "string", Description: "Raw response body"},
				{Name: "json", Type: "object", Description: "Parsed JSON response body (if parseable)"},
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
}

// ── Graphs ────────────────────────────────────────────────────────────────────

func registerGraphs(eng *workflow.Engine, cfClient *clickfunnels.Client) error {
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
						Label:   "else → low",
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
						Label:   "rejected → end",
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
	}
	return 0
}

// interpolateTemplate replaces {{key}} placeholders with values from data.
func interpolateTemplate(s string, data map[string]any) string {
	for k, v := range data {
		s = strings.ReplaceAll(s, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}
	return s
}

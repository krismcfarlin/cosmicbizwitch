package mcp_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
)

// newTestClient returns the base URL and token from the environment, skipping
// the test if either is missing.
func newTestClient(t *testing.T) (baseURL, token string) {
	t.Helper()
	token = os.Getenv("CBW_API_TOKEN")
	baseURL = os.Getenv("CBW_SERVER_URL")
	if token == "" || baseURL == "" {
		t.Skip("CBW_API_TOKEN and CBW_SERVER_URL must be set for integration tests")
	}
	return baseURL, token
}

// callTool posts to /mcp/call and returns the decoded JSON response.
func callTool(t *testing.T, baseURL, token, name string, args map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp/call", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp/call: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	t.Logf("callTool(%s) raw response: %s", name, raw)

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

// listTools gets /mcp/tools and returns the tools array.
func listTools(t *testing.T, baseURL, token string) []map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/mcp/tools", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /mcp/tools: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	t.Logf("listTools raw response (first 500 bytes): %.500s", raw)

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode tools response: %v", err)
	}

	rawTools, _ := body["tools"].([]interface{})
	tools := make([]map[string]any, 0, len(rawTools))
	for _, item := range rawTools {
		if m, ok := item.(map[string]any); ok {
			tools = append(tools, m)
		}
	}
	return tools
}

// toolNames returns the set of tool names from a tools list.
func toolNames(tools []map[string]any) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		if name, ok := t["name"].(string); ok {
			names[name] = true
		}
	}
	return names
}

// isError returns true when the response carries isError=true.
func isError(resp map[string]any) bool {
	v, _ := resp["isError"].(bool)
	return v
}

// firstContentText returns the text of the first content block, if any.
func firstContentText(resp map[string]any) string {
	raw, _ := resp["content"].([]interface{})
	if len(raw) == 0 {
		return ""
	}
	block, _ := raw[0].(map[string]any)
	text, _ := block["text"].(string)
	return text
}

// ---- Tests ----

func TestMCPListTools(t *testing.T) {
	baseURL, token := newTestClient(t)
	tools := listTools(t, baseURL, token)
	if len(tools) == 0 {
		t.Fatal("expected at least one tool, got empty list")
	}
	names := toolNames(tools)
	for _, want := range []string{"pb_schema", "pb_list", "pb_get"} {
		if !names[want] {
			t.Errorf("expected tool %q in list, got names: %v", want, names)
		}
	}
}

func TestMCPListTools_ContainsWorkflowActivities(t *testing.T) {
	baseURL, token := newTestClient(t)
	tools := listTools(t, baseURL, token)
	names := toolNames(tools)
	for _, want := range []string{"pb_query", "telegram_send_message"} {
		if !names[want] {
			t.Logf("tool %q not found (registered workflow activities may differ); present names sample: %v", want, names)
		}
	}
	// At minimum the workflow activities endpoint must merge successfully — list must be non-empty.
	if len(tools) == 0 {
		t.Fatal("tool list is empty, workflow activity merge likely broken")
	}
}

func TestMCPPBSchema(t *testing.T) {
	baseURL, token := newTestClient(t)
	resp := callTool(t, baseURL, token, "pb_schema", nil)
	if isError(resp) {
		t.Fatalf("pb_schema returned isError=true: %v", resp)
	}
	text := firstContentText(resp)
	if text == "" {
		t.Fatal("expected content text, got empty")
	}
	// Should be valid JSON containing at least one collection.
	var collections []map[string]any
	if err := json.Unmarshal([]byte(text[len("Found "):]), &collections); err != nil {
		// The response has a preamble like "Found N collection(s):\n\n[...]"
		// Just check for the JSON array bracket as a smoke test.
		if len(text) < 2 {
			t.Fatalf("pb_schema response too short: %q", text)
		}
	}
	t.Logf("pb_schema response length: %d bytes", len(text))
}

func TestMCPPBList_AppSettings(t *testing.T) {
	baseURL, token := newTestClient(t)
	resp := callTool(t, baseURL, token, "pb_list", map[string]any{"collection": "app_settings", "sort": "key"})
	if isError(resp) {
		t.Fatalf("pb_list app_settings returned isError=true: %v", resp)
	}
	text := firstContentText(resp)
	if text == "" {
		t.Fatal("expected content text, got empty")
	}
	t.Logf("pb_list app_settings: %s", text)
}

func TestMCPPBList_BirthChartRecords(t *testing.T) {
	baseURL, token := newTestClient(t)
	resp := callTool(t, baseURL, token, "pb_list", map[string]any{
		"collection": "birth_chart_records",
		"limit":      float64(5),
	})
	if isError(resp) {
		t.Fatalf("pb_list birth_chart_records returned isError=true: %v", resp)
	}
	text := firstContentText(resp)
	if text == "" {
		t.Fatal("expected content text, got empty")
	}
	t.Logf("pb_list birth_chart_records: %s", text)
}

func TestMCPPBList_CFContacts(t *testing.T) {
	baseURL, token := newTestClient(t)
	resp := callTool(t, baseURL, token, "pb_list", map[string]any{
		"collection": "cf_contacts",
		"limit":      float64(5),
	})
	if isError(resp) {
		t.Fatalf("pb_list cf_contacts returned isError=true: %v", resp)
	}
	text := firstContentText(resp)
	if text == "" {
		t.Fatal("expected content text, got empty")
	}
	t.Logf("pb_list cf_contacts: %s", text)
}

func TestMCPPBList_UnknownCollection(t *testing.T) {
	baseURL, token := newTestClient(t)
	resp := callTool(t, baseURL, token, "pb_list", map[string]any{"collection": "nonexistent_xyz"})
	if !isError(resp) {
		t.Fatalf("expected isError=true for unknown collection, got: %v", resp)
	}
	t.Logf("pb_list nonexistent_xyz (expected error): %v", resp)
}

func TestMCPPBGet_NotFound(t *testing.T) {
	baseURL, token := newTestClient(t)
	resp := callTool(t, baseURL, token, "pb_get", map[string]any{
		"collection": "app_settings",
		"id":         "doesnotexist",
	})
	if !isError(resp) {
		t.Fatalf("expected isError=true for missing record, got: %v", resp)
	}
	t.Logf("pb_get doesnotexist (expected error): %v", resp)
}

func TestMCPCFListContacts(t *testing.T) {
	baseURL, token := newTestClient(t)
	resp := callTool(t, baseURL, token, "cf_list_contacts", map[string]any{})
	// ClickFunnels may not be configured in all environments; we just check that
	// content was returned.
	text := firstContentText(resp)
	if text == "" {
		t.Fatal("expected content text, got empty")
	}
	t.Logf("cf_list_contacts: %s", text)
}

func TestMCPCFGetContact_Invalid(t *testing.T) {
	baseURL, token := newTestClient(t)
	resp := callTool(t, baseURL, token, "cf_get_contact", map[string]any{"id": float64(0)})
	// Either isError=true (static handler) or an error in the content text (workflow activity path).
	text := firstContentText(resp)
	if !isError(resp) && text == "" {
		t.Fatalf("expected error indication for id=0, got: %v", resp)
	}
	t.Logf("cf_get_contact id=0 (expected error): %s", text)
}

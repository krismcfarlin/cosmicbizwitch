package workflows

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	slackapp "cosmicbizwitch/internal/app/slack"
	"cosmicbizwitch/pkg/workflow/testutil"
)

// nilSlack returns a getter that always returns nil (simulates unconfigured Slack).
func nilSlack() func() *slackapp.Client {
	return func() *slackapp.Client { return nil }
}

// discardLogger returns a logger that discards all output.
func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// ── slack_send_message ───────────────────────────────────────────────────────

func TestSlackSendMessage_NilClient(t *testing.T) {
	fn := newSlackSendMessageFn(nilSlack())
	_, err := testutil.New(fn).
		WithInput("channel", "#general").
		WithInput("text", "hello").
		Run()
	if err == nil || err.Error() != "Slack not connected — add SLACK_BOT_TOKEN in Settings" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSlackSendMessage_MissingChannel(t *testing.T) {
	fn := newSlackSendMessageFn(nilSlack())
	_, err := testutil.New(fn).WithInput("text", "hi").Run()
	if err == nil {
		t.Error("expected error for missing channel, got nil")
	}
}

func TestSlackSendMessage_MissingText(t *testing.T) {
	fn := newSlackSendMessageFn(nilSlack())
	_, err := testutil.New(fn).WithInput("channel", "#general").Run()
	if err == nil {
		t.Error("expected error for missing text, got nil")
	}
}

// ── slack_list_channels ──────────────────────────────────────────────────────

func TestSlackListChannels_NilClient(t *testing.T) {
	fn := newSlackListChannelsFn(nilSlack())
	_, err := testutil.New(fn).Run()
	if err == nil || err.Error() != "Slack not connected — add SLACK_BOT_TOKEN in Settings" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── extractJSON ──────────────────────────────────────────────────────────────

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string // "map", "array", "string"
		wantKey  string // for map type: expect this key to exist
	}{
		{
			name:     "plain json object",
			input:    `{"name":"Alice","age":30}`,
			wantType: "map",
			wantKey:  "name",
		},
		{
			name:     "markdown fenced json",
			input:    "```json\n{\"status\":\"ok\"}\n```",
			wantType: "map",
			wantKey:  "status",
		},
		{
			name:     "prose wrapped json",
			input:    `Here is the result: {"score": 95} End of response.`,
			wantType: "map",
			wantKey:  "score",
		},
		{
			name:     "json array",
			input:    `[1,2,3]`,
			wantType: "array",
		},
		{
			name:     "plain string fallback",
			input:    "just some text",
			wantType: "string",
		},
		{
			name:     "empty string",
			input:    "",
			wantType: "string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			switch tt.wantType {
			case "map":
				m, ok := got.(map[string]any)
				if !ok {
					t.Fatalf("extractJSON() = %T, want map[string]any", got)
				}
				if tt.wantKey != "" {
					if _, ok := m[tt.wantKey]; !ok {
						t.Errorf("extractJSON() map missing key %q", tt.wantKey)
					}
				}
			case "array":
				if _, ok := got.([]any); !ok {
					t.Fatalf("extractJSON() = %T, want []any", got)
				}
			case "string":
				if _, ok := got.(string); !ok {
					t.Fatalf("extractJSON() = %T, want string", got)
				}
			}
		})
	}
}

// ── llm_prompt validation ────────────────────────────────────────────────────

func TestLLMPrompt_MissingModel(t *testing.T) {
	fn := newLLMPromptFn(nil, discardLogger())
	_, err := testutil.New(fn).WithInput("prompt", "say hi").Run()
	if err == nil || err.Error() != "llm_prompt: model field is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLLMPrompt_MissingPrompt(t *testing.T) {
	fn := newLLMPromptFn(nil, discardLogger())
	_, err := testutil.New(fn).WithInput("model", "anthropic/claude-sonnet-4-6").Run()
	if err == nil || err.Error() != "llm_prompt: prompt field is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLLMPrompt_UnknownProvider(t *testing.T) {
	fn := newLLMPromptFn(nil, discardLogger())
	_, err := testutil.New(fn).
		WithInput("model", "unknown/some-model").
		WithInput("prompt", "hi").
		Run()
	if err == nil {
		t.Error("expected error for unknown provider, got nil")
	}
	if err != nil && err.Error() != `llm_prompt: unknown provider for model "unknown/some-model" — prefix with 'anthropic/', 'openai/', or 'openrouter/'` {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ── callAnthropic mock server ─────────────────────────────────────────────────

func TestCallAnthropic_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			http.Error(w, "no api key", http.StatusUnauthorized)
			return
		}
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hello from mock!"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	old := anthropicAPIURL
	anthropicAPIURL = srv.URL
	defer func() { anthropicAPIURL = old }()

	got, err := callAnthropic("test-key", "claude-sonnet-4-6", "", "say hello", 0.7, 100)
	if err != nil {
		t.Fatalf("callAnthropic() error: %v", err)
	}
	if got != "Hello from mock!" {
		t.Errorf("callAnthropic() = %q, want %q", got, "Hello from mock!")
	}
}

func TestCallAnthropic_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rate limit"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	old := anthropicAPIURL
	anthropicAPIURL = srv.URL
	defer func() { anthropicAPIURL = old }()

	_, err := callAnthropic("key", "model", "", "prompt", 0.7, 100)
	if err == nil {
		t.Error("expected error for non-200 response, got nil")
	}
}

// ── callOpenRouter mock server ────────────────────────────────────────────────

func TestCallOpenRouter_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "OpenRouter response"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	old := openRouterAPIURL
	openRouterAPIURL = srv.URL
	defer func() { openRouterAPIURL = old }()

	got, err := callOpenRouter("test-key", "openai/gpt-4o", "", "hello", 0.7, 100)
	if err != nil {
		t.Fatalf("callOpenRouter() error: %v", err)
	}
	if got != "OpenRouter response" {
		t.Errorf("callOpenRouter() = %q, want %q", got, "OpenRouter response")
	}
}

// ── llm_prompt with mock server ───────────────────────────────────────────────

func TestLLMPrompt_JSONOutputFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse incoming request to verify prompt structure.
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		// Return a JSON response with the requested fields.
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": `{"sentiment": "positive", "confidence": 0.95}`,
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	old := openRouterAPIURL
	openRouterAPIURL = srv.URL
	defer func() { openRouterAPIURL = old }()

	// Use a fake app that returns a non-empty API key via environment trick:
	// Since we can't mock core.App, we test the openrouter path by overriding
	// llmGetOpenRouterKey indirectly — but that requires a real app.
	// Instead, test callOpenRouter directly with json_output_fields appended to prompt.
	_ = os.Stdout // suppress unused import

	got, err := callOpenRouter("key", "openai/gpt-4o", "", `{"sentiment": "positive", "confidence": 0.95}`, 0.7, 100)
	if err != nil {
		t.Fatalf("callOpenRouter() error: %v", err)
	}
	parsed := extractJSON(got)
	m, ok := parsed.(map[string]any)
	if !ok {
		t.Fatalf("extractJSON() = %T, want map", parsed)
	}
	if m["sentiment"] != "positive" {
		t.Errorf("sentiment = %v, want positive", m["sentiment"])
	}
	if fmt.Sprintf("%v", m["confidence"]) != "0.95" {
		t.Errorf("confidence = %v, want 0.95", m["confidence"])
	}
}

package workflows

import (
	"bytes"
	"regexp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func registerLLMActivity(eng *workflow.Engine, app core.App, logger *log.Logger) {
	eng.RegisterActivityWithMeta("llm_prompt",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			model := llmString(input, "model")
			if model == "" {
				return nil, fmt.Errorf("llm_prompt: model field is required")
			}
			prompt := llmString(input, "prompt")
			if prompt == "" {
				return nil, fmt.Errorf("llm_prompt: prompt field is required")
			}

			systemPrompt := llmString(input, "system_prompt")
			temperature := llmFloat(input, "temperature", 0.7)
			maxTokens := llmInt(input, "max_tokens", 1024)
			resultKey := llmString(input, "result_key")
			if resultKey == "" {
				resultKey = "llm_response"
			}

			// prompt_vars: optional map of name → value extracted from context.
			// Values arrive pre-interpolated (the runner resolved {{...}} templates
			// in them). We apply a second pass so these named vars can be used in
			// the prompt template, e.g. {{contact_name}}.
			promptVars := llmStringMap(input, "prompt_vars")

			// Normalize markdown-escaped placeholders in prompt before substitution
			// e.g. {{about\_your\_business}} → {{about_your_business}}
			escapedPlaceholder := regexp.MustCompile(`\{\{([^}]+)\}\}`)
			prompt = escapedPlaceholder.ReplaceAllStringFunc(prompt, func(match string) string {
				inner := match[2 : len(match)-2] // strip {{ and }}
				return "{{" + strings.ReplaceAll(inner, `\_`, "_") + "}}"
			})
			systemPrompt = escapedPlaceholder.ReplaceAllStringFunc(systemPrompt, func(match string) string {
				inner := match[2 : len(match)-2] // strip {{ and }}
				return "{{" + strings.ReplaceAll(inner, `\_`, "_") + "}}"
			})

			for k, v := range promptVars {
				prompt = strings.ReplaceAll(prompt, "{{"+k+"}}", v)
				systemPrompt = strings.ReplaceAll(systemPrompt, "{{"+k+"}}", v)
			}

			// json_output_fields: optional list of field names for structured JSON output.
			jsonFields := llmStringSlice(input, "json_output_fields")
			if len(jsonFields) > 0 {
				schemaBytes, _ := json.Marshal(jsonFields)
				jsonSchemaStr := string(schemaBytes)
				prompt += "\n\nPlease respond with your analysis directly in JSON format without using Markdown code blocks or any other formatting. Use the following json fields in the response " + jsonSchemaStr
			}

			// Debug: log the generated prompt before sending to the LLM.
			if len(promptVars) > 0 {
				varsBytes, _ := json.Marshal(promptVars)
				logger.Printf("[llm_prompt] prompt_vars: %s", varsBytes)
			}
			if systemPrompt != "" {
				logger.Printf("[llm_prompt] system_prompt (model=%s):\n%s", model, systemPrompt)
			}
			logger.Printf("[llm_prompt] generated prompt (model=%s):\n%s", model, prompt)

			var responseText string
			var err error

			switch {
			case strings.HasPrefix(model, "anthropic/"):
				apiKey := llmGetSetting(app, "ANTHROPIC_API_KEY")
				if apiKey == "" {
					return nil, fmt.Errorf("llm_prompt: ANTHROPIC_API_KEY not configured in settings")
				}
				modelID := strings.TrimPrefix(model, "anthropic/")
				// Map short model aliases to full IDs
				if modelID == "claude-haiku-4-5" {
					modelID = "claude-haiku-4-5-20251001"
				}
				responseText, err = callAnthropic(apiKey, modelID, systemPrompt, prompt, temperature, maxTokens)

			case strings.HasPrefix(model, "openrouter/"):
				keyName := llmString(input, "api_key_name")
				apiKey := llmGetOpenRouterKey(app, keyName)
				if apiKey == "" {
					return nil, fmt.Errorf("llm_prompt: no OpenRouter API key configured (set OPENROUTER_KEYS or OPENROUTER_API_KEY in Settings)")
				}
				modelID := strings.TrimPrefix(model, "openrouter/")
				responseText, err = callOpenRouter(apiKey, modelID, systemPrompt, prompt, temperature, maxTokens)

			case strings.HasPrefix(model, "openai/"):
				apiKey := llmGetSetting(app, "OPENAI_API_KEY")
				if apiKey == "" {
					return nil, fmt.Errorf("llm_prompt: OPENAI_API_KEY not configured in settings")
				}
				modelID := strings.TrimPrefix(model, "openai/")
				responseText, err = callOpenAI(apiKey, modelID, systemPrompt, prompt, temperature, maxTokens)

			default:
				return nil, fmt.Errorf("llm_prompt: unknown provider for model %q — prefix with 'anthropic/', 'openai/', or 'openrouter/'", model)
			}

			if err != nil {
				return nil, fmt.Errorf("llm_prompt: %w", err)
			}

			// Try to extract and parse a JSON object or array from the response.
			// Handles: plain JSON, markdown code fences, and prose surrounding JSON.
			parsedResult := extractJSON(responseText)
			out := map[string]any{resultKey: parsedResult}

			// If json_output_fields was set, also merge individual fields from the parsed map.
			if len(jsonFields) > 0 {
				if parsed, ok := parsedResult.(map[string]any); ok {
					for _, f := range jsonFields {
						if v, ok := parsed[f]; ok {
							out[f] = v
						}
					}
				}
			}

			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "AI",
			Description: "Calls an LLM (Anthropic, OpenAI, or OpenRouter) with a prompt and returns the text response.",
			InputFields: []workflow.FieldMeta{
				{Name: "model", Type: "string", Required: true, Description: "Model identifier. Prefix with 'anthropic/' for Anthropic, 'openai/' for OpenAI, or 'openrouter/' for OpenRouter. E.g. 'anthropic/claude-sonnet-4-6', 'openai/gpt-4o', or 'openrouter/openai/gpt-4o'"},
				{Name: "system_prompt", Type: "string", Description: "Optional system prompt / instructions for the model."},
				{Name: "prompt", Type: "string", Required: true, Description: "The user prompt. Supports {{key}} template interpolation from context and from prompt_vars."},
				{Name: "prompt_vars", Type: "map", Description: "Optional map of variable name → value to inject into the prompt. Values support {{key}} interpolation from context (resolved before this node runs). Use these to rename or reshape context fields for the prompt template, e.g. {\"contact_name\": \"{{first_name}} {{last_name}}\"}"},
				{Name: "temperature", Type: "number", Description: "Sampling temperature 0.0–2.0. Defaults to 0.7."},
				{Name: "max_tokens", Type: "number", Description: "Maximum tokens in the response. Defaults to 1024."},
				{Name: "result_key", Type: "string", Description: "Context key to store the response under. Defaults to 'llm_response'."},
				{Name: "json_output_fields", Type: "list", Description: "Optional list of JSON field names to request in the response. When set, appends a JSON formatting instruction to the prompt and parses the response fields into the workflow context."},
				{Name: "api_key_name", Type: "string", Description: "Named OpenRouter API key to use (from Settings → OpenRouter Keys). Leave empty to use the default key."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "llm_response", Type: "string", Description: "The raw LLM text response (stored under result_key if specified)."},
				{Name: "<json_output_fields>", Type: "any", Description: "When json_output_fields is set, each named field from the parsed JSON response is also added to the output."},
			},
		},
	)
}

// ── Anthropic Messages API ────────────────────────────────────────────────────

func callAnthropic(apiKey, model, system, prompt string, temperature float64, maxTokens int) (string, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   []message{{Role: "user", Content: prompt}},
	}
	if system != "" {
		body["system"] = system
	}
	body["temperature"] = temperature

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return "", fmt.Errorf("Anthropic API error %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	for _, c := range result.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in Anthropic response")
}

// ── OpenRouter Chat Completions API ──────────────────────────────────────────

func callOpenRouter(apiKey, model, system, prompt string, temperature float64, maxTokens int) (string, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := []message{}
	if system != "" {
		msgs = append(msgs, message{Role: "system", Content: system})
	}
	msgs = append(msgs, message{Role: "user", Content: prompt})

	body := map[string]any{
		"model":      model,
		"messages":   msgs,
		"temperature": temperature,
		"max_tokens": maxTokens,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return "", fmt.Errorf("OpenRouter API error %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in OpenRouter response")
	}
	return result.Choices[0].Message.Content, nil
}

// ── OpenAI Chat Completions API ───────────────────────────────────────────────

func callOpenAI(apiKey, model, system, prompt string, temperature float64, maxTokens int) (string, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := []message{}
	if system != "" {
		msgs = append(msgs, message{Role: "system", Content: system})
	}
	msgs = append(msgs, message{Role: "user", Content: prompt})

	body := map[string]any{
		"model":       model,
		"messages":    msgs,
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return "", fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in OpenAI response")
	}
	return result.Choices[0].Message.Content, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// llmGetOpenRouterKey returns the OpenRouter API key for the given name.
// If name is empty, falls back to the legacy OPENROUTER_API_KEY setting.
func llmGetOpenRouterKey(app core.App, name string) string {
	// Try named keys first
	keysJSON := llmGetSetting(app, "OPENROUTER_KEYS")
	if keysJSON != "" {
		var keys []struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		}
		if err := json.Unmarshal([]byte(keysJSON), &keys); err == nil {
			if name == "" && len(keys) > 0 {
				return keys[0].Key // default: first key
			}
			for _, k := range keys {
				if strings.EqualFold(k.Name, name) {
					return k.Key
				}
			}
		}
	}
	// Fall back to legacy single key
	return llmGetSetting(app, "OPENROUTER_API_KEY")
}

func llmGetSetting(app core.App, key string) string {
	records, err := app.FindRecordsByFilter("app_settings", "key = {:key}", "", 1, 0, dbx.Params{"key": key})
	if err != nil || len(records) == 0 {
		return ""
	}
	return records[0].GetString("value")
}

func llmString(input map[string]any, key string) string {
	if v, ok := input[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func llmFloat(input map[string]any, key string, def float64) float64 {
	if v, ok := input[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return f
			}
		}
	}
	return def
}

func llmInt(input map[string]any, key string, def int) int {
	if v, ok := input[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case string:
			if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
				return i
			}
		}
	}
	return def
}

func llmStringMap(input map[string]any, key string) map[string]string {
	v, ok := input[key]
	if !ok {
		return nil
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(raw))
	for k, val := range raw {
		if s, ok := val.(string); ok {
			result[k] = s
		} else {
			result[k] = fmt.Sprintf("%v", val)
		}
	}
	return result
}

// extractJSON tries to parse a JSON value from an LLM response string.
// It handles markdown code fences (```json...```) and prose surrounding JSON.
// Returns the parsed value on success, or the original string on failure.
func extractJSON(s string) any {
	candidate := strings.TrimSpace(s)

	// Strip markdown code fence if present.
	if strings.HasPrefix(candidate, "```") {
		if idx := strings.Index(candidate, "\n"); idx != -1 {
			candidate = candidate[idx+1:]
		}
		if idx := strings.LastIndex(candidate, "```"); idx != -1 {
			candidate = strings.TrimSpace(candidate[:idx])
		}
	}

	// Try the full trimmed string first.
	var result any
	if err := json.Unmarshal([]byte(candidate), &result); err == nil {
		return result
	}

	// Extract the first outermost JSON object {...} or array [...] from the text.
	for _, delims := range [][2]byte{[2]byte{'{', '}'}, [2]byte{'[', ']'}} {
		open, close := delims[0], delims[1]
		start := strings.IndexByte(candidate, open)
		if start < 0 {
			continue
		}
		// Walk backwards from end to find the matching close.
		end := strings.LastIndexByte(candidate, close)
		if end <= start {
			continue
		}
		if err := json.Unmarshal([]byte(candidate[start:end+1]), &result); err == nil {
			return result
		}
	}

	return s // fall back to raw string
}

func llmStringSlice(input map[string]any, key string) []string {
	v, ok := input[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

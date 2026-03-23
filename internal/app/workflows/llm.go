package workflows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func registerLLMActivity(eng *workflow.Engine, app core.App) {
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
				apiKey := llmGetSetting(app, "OPENROUTER_API_KEY")
				if apiKey == "" {
					return nil, fmt.Errorf("llm_prompt: OPENROUTER_API_KEY not configured in settings")
				}
				modelID := strings.TrimPrefix(model, "openrouter/")
				responseText, err = callOpenRouter(apiKey, modelID, systemPrompt, prompt, temperature, maxTokens)

			default:
				return nil, fmt.Errorf("llm_prompt: unknown provider for model %q — prefix with 'anthropic/' or 'openrouter/'", model)
			}

			if err != nil {
				return nil, fmt.Errorf("llm_prompt: %w", err)
			}
			return map[string]any{resultKey: responseText}, nil
		},
		workflow.ActivityMeta{
			Category:    "AI",
			Description: "Calls an LLM (Anthropic or OpenRouter) with a prompt and returns the text response.",
			InputFields: []workflow.FieldMeta{
				{Name: "model", Type: "string", Required: true, Description: "Model identifier. Prefix with 'anthropic/' for direct Anthropic API or 'openrouter/' for OpenRouter. E.g. 'anthropic/claude-sonnet-4-6' or 'openrouter/openai/gpt-4o'"},
				{Name: "system_prompt", Type: "string", Description: "Optional system prompt / instructions for the model."},
				{Name: "prompt", Type: "string", Required: true, Description: "The user prompt. Supports {{key}} template interpolation."},
				{Name: "temperature", Type: "number", Description: "Sampling temperature 0.0–2.0. Defaults to 0.7."},
				{Name: "max_tokens", Type: "number", Description: "Maximum tokens in the response. Defaults to 1024."},
				{Name: "result_key", Type: "string", Description: "Context key to store the response under. Defaults to 'llm_response'."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "llm_response", Type: "string", Description: "The LLM text response (stored under result_key if specified)."},
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

// ── Helpers ───────────────────────────────────────────────────────────────────

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
		}
	}
	return def
}

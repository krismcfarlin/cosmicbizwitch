package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

type llmModelOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type llmModelGroup struct {
	Group  string           `json:"group"`
	Models []llmModelOption `json:"models"`
}

func (s *Server) handleLLMModels(w http.ResponseWriter, r *http.Request) {
	var groups []llmModelGroup

	if s.settings == nil {
		s.respondJSON(w, http.StatusOK, map[string]any{"groups": groups})
		return
	}

	// Anthropic
	if key := s.settings.Get("ANTHROPIC_API_KEY"); key != "" {
		models, err := fetchAnthropicModels(key)
		if err != nil {
			s.logger.Printf("[llm/models] anthropic error: %v", err)
		} else if len(models) > 0 {
			groups = append(groups, llmModelGroup{Group: "Anthropic (Direct)", Models: models})
		}
	}

	// OpenAI
	if key := s.settings.Get("OPENAI_API_KEY"); key != "" {
		models, err := fetchOpenAIModels(key)
		if err != nil {
			s.logger.Printf("[llm/models] openai error: %v", err)
		} else if len(models) > 0 {
			groups = append(groups, llmModelGroup{Group: "OpenAI (Direct)", Models: models})
		}
	}

	// OpenRouter — try OPENROUTER_API_KEY first, then the first entry in OPENROUTER_KEYS
	orKey := s.settings.Get("OPENROUTER_API_KEY")
	if orKey == "" {
		if keysJSON := s.settings.Get("OPENROUTER_KEYS"); keysJSON != "" {
			var keys []struct {
				Name string `json:"name"`
				Key  string `json:"key"`
			}
			if err := json.Unmarshal([]byte(keysJSON), &keys); err == nil && len(keys) > 0 {
				orKey = keys[0].Key
			}
		}
	}
	if orKey != "" {
		orGroups, err := fetchOpenRouterModels(orKey)
		if err != nil {
			s.logger.Printf("[llm/models] openrouter error: %v", err)
		} else {
			groups = append(groups, orGroups...)
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func fetchAnthropicModels(apiKey string) ([]llmModelOption, error) {
	req, err := http.NewRequest("GET", "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, llmTruncate(string(body), 200))
	}

	var result struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	opts := make([]llmModelOption, 0, len(result.Data))
	for _, m := range result.Data {
		label := m.DisplayName
		if label == "" {
			label = m.ID
		}
		opts = append(opts, llmModelOption{Value: "anthropic/" + m.ID, Label: label})
	}
	return opts, nil
}

func fetchOpenAIModels(apiKey string) ([]llmModelOption, error) {
	req, err := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, llmTruncate(string(body), 200))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	opts := []llmModelOption{}
	for _, m := range result.Data {
		if !llmIsChatModel(m.ID) {
			continue
		}
		opts = append(opts, llmModelOption{Value: "openai/" + m.ID, Label: m.ID})
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].Value < opts[j].Value })
	return opts, nil
}

// llmIsChatModel returns true for OpenAI models suitable for chat completions.
func llmIsChatModel(id string) bool {
	for _, p := range []string{"gpt-4", "gpt-3.5", "o1", "o3", "chatgpt"} {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

func fetchOpenRouterModels(apiKey string) ([]llmModelGroup, error) {
	req, err := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, llmTruncate(string(body), 200))
	}

	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	byProvider := map[string][]llmModelOption{}
	for _, m := range result.Data {
		parts := strings.SplitN(m.ID, "/", 2)
		provider := m.ID
		if len(parts) == 2 {
			provider = parts[0]
		}
		label := m.Name
		if label == "" {
			label = m.ID
		}
		byProvider[provider] = append(byProvider[provider], llmModelOption{
			Value: "openrouter/" + m.ID,
			Label: label,
		})
	}

	providers := make([]string, 0, len(byProvider))
	for p := range byProvider {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	groups := make([]llmModelGroup, 0, len(providers))
	for _, p := range providers {
		models := byProvider[p]
		sort.Slice(models, func(i, j int) bool { return models[i].Label < models[j].Label })
		groups = append(groups, llmModelGroup{
			Group:  "OpenRouter — " + llmTitleCase(p),
			Models: models,
		})
	}
	return groups, nil
}

func llmTitleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func llmTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// handleOpenRouterKeys returns the names of all configured OpenRouter API keys.
// Key values are never returned — only names. Returns an empty array if no keys are configured.
func (s *Server) handleOpenRouterKeys(w http.ResponseWriter, r *http.Request) {
	type keyName struct {
		Name string `json:"name"`
	}

	if s.settings == nil {
		s.respondJSON(w, http.StatusOK, []keyName{})
		return
	}

	keysJSON := s.settings.Get("OPENROUTER_KEYS")
	if keysJSON == "" {
		s.respondJSON(w, http.StatusOK, []keyName{})
		return
	}

	var raw []struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal([]byte(keysJSON), &raw); err != nil {
		s.logger.Printf("[openrouter/keys] parse OPENROUTER_KEYS: %v", err)
		s.respondJSON(w, http.StatusOK, []keyName{})
		return
	}

	out := make([]keyName, 0, len(raw))
	for _, k := range raw {
		if k.Name != "" {
			out = append(out, keyName{Name: k.Name})
		}
	}
	s.respondJSON(w, http.StatusOK, out)
}

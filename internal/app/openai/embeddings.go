// Package openai provides lightweight wrappers for the OpenAI Embeddings API.
// It avoids pulling in the full openai-go SDK by using plain HTTP calls,
// consistent with the pattern already used in internal/app/workflows/llm.go.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultEmbeddingModel = "text-embedding-3-small"
const embeddingsURL = "https://api.openai.com/v1/embeddings"

// EmbedText returns the embedding vector for a single text string.
func EmbedText(ctx context.Context, apiKey, text string) ([]float32, error) {
	vecs, err := EmbedBatch(ctx, apiKey, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("openai embeddings: no vectors returned")
	}
	return vecs[0], nil
}

// EmbedBatch returns one embedding vector per text in the input slice.
// OpenAI imposes a limit of 2048 inputs per request and a combined token limit;
// callers are responsible for keeping batches within those limits (~100 texts is safe).
func EmbedBatch(ctx context.Context, apiKey string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := map[string]any{
		"model": defaultEmbeddingModel,
		"input": texts,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai embeddings: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embeddingsURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai embeddings: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embeddings: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		preview := string(body)
		if len(preview) > 400 {
			preview = preview[:400]
		}
		return nil, fmt.Errorf("openai embeddings: API error %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("openai embeddings: parse response: %w", err)
	}

	// Result entries may not arrive in the same order as input; sort by index.
	vecs := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index >= 0 && d.Index < len(vecs) {
			vecs[d.Index] = d.Embedding
		}
	}

	// Verify no gaps.
	for i, v := range vecs {
		if v == nil {
			return nil, fmt.Errorf("openai embeddings: missing vector at index %d", i)
		}
	}

	return vecs, nil
}

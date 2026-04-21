package workflows

import (
	"context"
	"fmt"
	"strconv"
	"time"

	openaiapp "cosmicbizwitch/internal/app/openai"
	"cosmicbizwitch/internal/app/storage"
	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/pocketbase/core"
)

// registerTelegramVectorActivities registers the telegram_search and
// telegram_embed_message activities into the workflow engine.
func registerTelegramVectorActivities(eng *workflow.Engine, store *storage.Store, app core.App) {
	registerTelegramSearch(eng, store, app)
	registerTelegramEmbedMessage(eng, store, app)
}

// ── telegram_search ───────────────────────────────────────────────────────────

func registerTelegramSearch(eng *workflow.Engine, store *storage.Store, app core.App) {
	eng.RegisterActivityWithMeta("telegram_search",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			apiKey := llmGetSetting(app, "OPENAI_API_KEY")

			query, _ := input["query"].(string)
			firstName, _ := input["first_name"].(string)
			lastName, _ := input["last_name"].(string)
			startDateStr, _ := input["start_date"].(string)
			endDateStr, _ := input["end_date"].(string)

			limit := 10
			if v := toFloat64OrZero(input["limit"]); v > 0 {
				limit = int(v)
			}

			var startDate, endDate int64
			startDate = parseFlexDate(startDateStr)
			endDate = parseFlexDate(endDateStr)

			var queryVec []float32
			if query != "" {
				if apiKey == "" {
					return nil, fmt.Errorf("telegram_search: OPENAI_API_KEY not configured in settings — required for semantic search")
				}
				var err error
				queryVec, err = openaiapp.EmbedText(ctx, apiKey, query)
				if err != nil {
					return nil, fmt.Errorf("telegram_search: embed query: %w", err)
				}
			}

			params := storage.TelegramSearchParams{
				QueryVector: queryVec,
				FirstName:   firstName,
				LastName:    lastName,
				StartDate:   startDate,
				EndDate:     endDate,
				Limit:       limit,
			}

			results, err := store.SearchTelegramMessages(ctx, params)
			if err != nil {
				return nil, fmt.Errorf("telegram_search: %w", err)
			}

			items := make([]any, 0, len(results))
			for _, r := range results {
				items = append(items, map[string]any{
					"id":         r.ID,
					"message_id": r.MessageID,
					"from_id":    r.FromID,
					"chat_id":    r.ChatID,
					"chat_title": r.ChatTitle,
					"first_name": r.FirstName,
					"last_name":  r.LastName,
					"sent_at":    r.SentAt,
					"text":       r.Text,
					"score":      r.Score,
				})
			}

			return map[string]any{
				"results": items,
				"count":   len(items),
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Telegram",
			Description: "Search indexed Telegram messages by semantic similarity and/or name/date filters. Requires OPENAI_API_KEY in settings when a query is provided.",
			InputFields: []workflow.FieldMeta{
				{Name: "query", Type: "string", Description: "Natural-language search query (optional). When set, performs semantic vector search using OpenAI embeddings."},
				{Name: "first_name", Type: "string", Description: "Filter by sender first name (case-insensitive partial match)"},
				{Name: "last_name", Type: "string", Description: "Filter by sender last name (case-insensitive partial match)"},
				{Name: "start_date", Type: "string", Description: "Filter messages on or after this date. ISO date (2024-01-15) or unix timestamp string."},
				{Name: "end_date", Type: "string", Description: "Filter messages on or before this date. ISO date or unix timestamp string."},
				{Name: "limit", Type: "number", Description: "Maximum results to return (default 10, max 100)"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "results", Type: "array", Description: "Array of matching messages. Each item has: id, message_id, from_id, chat_id, chat_title, first_name, last_name, sent_at, text, score."},
				{Name: "count", Type: "number", Description: "Number of results returned"},
			},
		},
	)
}

// ── telegram_embed_message ────────────────────────────────────────────────────

func registerTelegramEmbedMessage(eng *workflow.Engine, store *storage.Store, app core.App) {
	eng.RegisterActivityWithMeta("telegram_embed_message",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			apiKey := llmGetSetting(app, "OPENAI_API_KEY")
			if apiKey == "" {
				return nil, fmt.Errorf("telegram_embed_message: OPENAI_API_KEY not configured in settings")
			}

			id, _ := input["id"].(string)
			if id == "" {
				return nil, fmt.Errorf("telegram_embed_message: id is required")
			}
			text, _ := input["text"].(string)
			if text == "" {
				return nil, workflow.ErrSkip // nothing to embed
			}

			record := storage.TelegramVectorRecord{
				ID:        id,
				MessageID: int64(toFloat64OrZero(input["message_id"])),
				FromID:    int64(toFloat64OrZero(input["from_id"])),
				ChatID:    int64(toFloat64OrZero(input["chat_id"])),
				ChatTitle: stringOrEmpty(input["chat_title"]),
				FirstName: stringOrEmpty(input["first_name"]),
				LastName:  stringOrEmpty(input["last_name"]),
				SentAt:    int64(toFloat64OrZero(input["sent_at"])),
				Text:      text,
			}

			vec, err := openaiapp.EmbedText(ctx, apiKey, text)
			if err != nil {
				return nil, fmt.Errorf("telegram_embed_message: embed text: %w", err)
			}
			record.Embedding = vec

			if err := store.UpsertTelegramVector(ctx, record); err != nil {
				return nil, fmt.Errorf("telegram_embed_message: upsert: %w", err)
			}

			return map[string]any{
				"indexed": true,
				"id":      id,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Telegram",
			Description: "Embeds a single Telegram message via OpenAI and stores it in the telegram_vectors table. Skipped automatically when text is empty. Call this after pb_upsert in a Telegram workflow to keep the index current.",
			InputFields: []workflow.FieldMeta{
				{Name: "id", Type: "string", Description: "PocketBase record ID from raw_telegram", Required: true},
				{Name: "text", Type: "string", Description: "Message text to embed", Required: true},
				{Name: "message_id", Type: "number", Description: "Telegram message_id"},
				{Name: "from_id", Type: "number", Description: "Telegram user ID of sender"},
				{Name: "chat_id", Type: "number", Description: "Telegram chat ID"},
				{Name: "chat_title", Type: "string", Description: "Telegram chat title"},
				{Name: "first_name", Type: "string", Description: "Sender first name"},
				{Name: "last_name", Type: "string", Description: "Sender last name"},
				{Name: "sent_at", Type: "number", Description: "Unix timestamp of message"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "indexed", Type: "bool", Description: "True when the message was successfully embedded and stored"},
				{Name: "id", Type: "string", Description: "The PocketBase record ID that was indexed"},
			},
		},
	)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// parseFlexDate converts a string to a unix timestamp.
// Accepts: ISO date "2024-01-15", RFC3339, or a raw unix timestamp string.
// Returns 0 if the string is empty or unparseable.
func parseFlexDate(s string) int64 {
	if s == "" {
		return 0
	}
	// Try numeric first (unix timestamp as string)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	// Try ISO date
	layouts := []string{
		"2006-01-02",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func stringOrEmpty(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}


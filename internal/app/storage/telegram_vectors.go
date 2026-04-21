package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// TelegramVectorRecord holds all data for a single telegram message vector entry.
type TelegramVectorRecord struct {
	ID        string    // raw_telegram PB record id
	MessageID int64
	FromID    int64
	ChatID    int64
	ChatTitle string
	FirstName string
	LastName  string
	SentAt    int64  // unix timestamp
	Text      string
	Embedding []float32 // nil if not yet embedded
}

// TelegramSearchParams controls telegram vector search behavior.
type TelegramSearchParams struct {
	QueryVector []float32 // nil = no semantic search, pure SQL filter
	FirstName   string
	LastName    string
	StartDate   int64 // unix timestamp, 0 = no filter
	EndDate     int64 // unix timestamp, 0 = no filter
	Limit       int   // default 10, max 100
}

// TelegramSearchResult is one row returned from SearchTelegramMessages.
type TelegramSearchResult struct {
	ID        string  `json:"id"`
	MessageID int64   `json:"message_id"`
	FromID    int64   `json:"from_id"`
	ChatID    int64   `json:"chat_id"`
	ChatTitle string  `json:"chat_title"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	SentAt    int64   `json:"sent_at"`
	Text      string  `json:"text"`
	Score     float64 `json:"score"` // 0 if no vector search
}

// UpsertTelegramVector inserts or replaces a telegram message record in telegram_vectors.
// If record.Embedding is nil the embedding column is set to NULL.
func (s *Store) UpsertTelegramVector(ctx context.Context, record TelegramVectorRecord) error {
	if len(record.Embedding) > 0 {
		vecStr := float32SliceToVectorStr(record.Embedding)
		_, err := s.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO telegram_vectors
				(id, message_id, from_id, chat_id, chat_title, first_name, last_name, sent_at, text, embedding, indexed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, vector(?), CURRENT_TIMESTAMP)
		`, record.ID, record.MessageID, record.FromID, record.ChatID, record.ChatTitle,
			record.FirstName, record.LastName, record.SentAt, record.Text, vecStr)
		if err != nil {
			return fmt.Errorf("upsert telegram vector with embedding: %w", err)
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO telegram_vectors
				(id, message_id, from_id, chat_id, chat_title, first_name, last_name, sent_at, text, embedding, indexed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, CURRENT_TIMESTAMP)
		`, record.ID, record.MessageID, record.FromID, record.ChatID, record.ChatTitle,
			record.FirstName, record.LastName, record.SentAt, record.Text)
		if err != nil {
			return fmt.Errorf("upsert telegram vector without embedding: %w", err)
		}
	}
	return nil
}

// SearchTelegramMessages performs either vector similarity search (when QueryVector is
// set) or a pure SQL filter search ordered by sent_at DESC.
func (s *Store) SearchTelegramMessages(ctx context.Context, params TelegramSearchParams) ([]TelegramSearchResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var (
		query   strings.Builder
		args    []any
		useVec  = len(params.QueryVector) > 0
	)

	if useVec {
		vecStr := float32SliceToVectorStr(params.QueryVector)
		query.WriteString(`
			SELECT id, message_id, from_id, chat_id, chat_title, first_name, last_name, sent_at, text,
			       vector_distance_cos(embedding, vector(?)) AS score
			FROM telegram_vectors
			WHERE embedding IS NOT NULL
		`)
		args = append(args, vecStr)
	} else {
		query.WriteString(`
			SELECT id, message_id, from_id, chat_id, chat_title, first_name, last_name, sent_at, text,
			       0.0 AS score
			FROM telegram_vectors
			WHERE 1=1
		`)
	}

	if params.FirstName != "" {
		query.WriteString(` AND LOWER(first_name) LIKE LOWER(?)`)
		args = append(args, "%"+params.FirstName+"%")
	}
	if params.LastName != "" {
		query.WriteString(` AND LOWER(last_name) LIKE LOWER(?)`)
		args = append(args, "%"+params.LastName+"%")
	}
	if params.StartDate > 0 {
		query.WriteString(` AND sent_at >= ?`)
		args = append(args, params.StartDate)
	}
	if params.EndDate > 0 {
		query.WriteString(` AND sent_at <= ?`)
		args = append(args, params.EndDate)
	}

	if useVec {
		query.WriteString(` ORDER BY score ASC`)
	} else {
		query.WriteString(` ORDER BY sent_at DESC`)
	}
	query.WriteString(` LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("search telegram messages: %w", err)
	}
	defer rows.Close()

	var results []TelegramSearchResult
	for rows.Next() {
		var r TelegramSearchResult
		var chatTitle, firstName, lastName sql.NullString
		if err := rows.Scan(&r.ID, &r.MessageID, &r.FromID, &r.ChatID,
			&chatTitle, &firstName, &lastName, &r.SentAt, &r.Text, &r.Score); err != nil {
			return nil, fmt.Errorf("scan telegram search result: %w", err)
		}
		r.ChatTitle = chatTitle.String
		r.FirstName = firstName.String
		r.LastName = lastName.String
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telegram search rows: %w", err)
	}
	return results, nil
}

// CountTelegramVectors returns the total number of rows in telegram_vectors.
func (s *Store) CountTelegramVectors(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM telegram_vectors`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count telegram vectors: %w", err)
	}
	return count, nil
}

// GetTelegramVectorIDs returns a set of all IDs currently stored in telegram_vectors.
// Used by the bulk indexer to determine which messages still need embedding.
func (s *Store) GetTelegramVectorIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM telegram_vectors`)
	if err != nil {
		return nil, fmt.Errorf("get telegram vector ids: %w", err)
	}
	defer rows.Close()

	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan telegram vector id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

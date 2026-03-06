package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Meeting represents a coaching meeting document
type Meeting struct {
	ID           string                 `json:"id"`
	ClientID     string                 `json:"client_id"`
	Title        string                 `json:"title"`
	Date         time.Time              `json:"date"`
	Duration     int                    `json:"duration_minutes"`
	Participants []string               `json:"participants"`
	Summary      map[string]interface{} `json:"summary"`
	ShareURL     string                 `json:"share_url,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// CreateCollection verifies the client exists. The meetings table is shared;
// no per-client collection creation is needed with libsql.
func (s *Store) CreateCollection(ctx context.Context, clientID string) error {
	_, err := s.GetClient(ctx, clientID)
	return err
}

// AddMeeting stores a meeting without an embedding vector
func (s *Store) AddMeeting(ctx context.Context, meeting *Meeting) (string, error) {
	now := time.Now()
	meeting.CreatedAt = now
	meeting.UpdatedAt = now
	content := s.formatMeetingForEmbedding(meeting)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO meetings (id, client_id, title, date, share_url, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			date = excluded.date,
			share_url = excluded.share_url,
			content = excluded.content,
			updated_at = excluded.updated_at
	`, meeting.ID, meeting.ClientID, meeting.Title, meeting.Date,
		meeting.ShareURL, content, meeting.CreatedAt, meeting.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to store meeting: %w", err)
	}

	s.logger.Printf("✅ Stored meeting %s for client %s", meeting.ID, meeting.ClientID)
	return meeting.ID, nil
}

// AddMeetingWithEmbedding stores a meeting with a pre-generated embedding vector
func (s *Store) AddMeetingWithEmbedding(ctx context.Context, meeting *Meeting, vector []float32) (string, error) {
	now := time.Now()
	meeting.CreatedAt = now
	meeting.UpdatedAt = now
	content := s.formatMeetingForEmbedding(meeting)
	vecStr := float32SliceToVectorStr(vector)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO meetings (id, client_id, title, date, share_url, content, embedding, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, vector(?), ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			date = excluded.date,
			share_url = excluded.share_url,
			content = excluded.content,
			embedding = excluded.embedding,
			updated_at = excluded.updated_at
	`, meeting.ID, meeting.ClientID, meeting.Title, meeting.Date,
		meeting.ShareURL, content, vecStr, meeting.CreatedAt, meeting.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to store meeting with embedding: %w", err)
	}

	s.logger.Printf("✅ Stored meeting %s with embedding for client %s", meeting.ID, meeting.ClientID)
	return meeting.ID, nil
}

// GetMeeting retrieves a meeting by ID
func (s *Store) GetMeeting(ctx context.Context, meetingID string) (*Meeting, error) {
	var m Meeting
	var shareURL sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, client_id, title, date, share_url, created_at, updated_at
		FROM meetings WHERE id = ?
	`, meetingID).Scan(&m.ID, &m.ClientID, &m.Title, &m.Date, &shareURL, &m.CreatedAt, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("meeting not found: %s", meetingID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get meeting: %w", err)
	}
	if shareURL.Valid {
		m.ShareURL = shareURL.String
	}
	return &m, nil
}

// ListMeetings retrieves recent meetings for a client
func (s *Store) ListMeetings(ctx context.Context, clientID string, limit int) ([]*Meeting, error) {
	if limit == 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, client_id, title, date, share_url, created_at, updated_at
		FROM meetings
		WHERE client_id = ?
		ORDER BY date DESC
		LIMIT ?
	`, clientID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list meetings: %w", err)
	}
	defer rows.Close()
	return s.scanMeetings(rows)
}

// SearchMeetings performs vector similarity search, falling back to date order if no vector provided
func (s *Store) SearchMeetings(ctx context.Context, query string, clientID string, limit int, queryVector []float32) ([]*Meeting, error) {
	if limit == 0 {
		limit = 10
	}

	if len(queryVector) == 0 {
		return s.ListMeetings(ctx, clientID, limit)
	}

	vecStr := float32SliceToVectorStr(queryVector)

	// Exact cosine similarity search — correct and efficient at current scale
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, client_id, title, date, share_url, created_at, updated_at
		FROM meetings
		WHERE client_id = ? AND embedding IS NOT NULL
		ORDER BY vector_distance_cos(embedding, vector(?))
		LIMIT ?
	`, clientID, vecStr, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}
	defer rows.Close()
	return s.scanMeetings(rows)
}

// UpdateMeeting updates meeting metadata, preserving the existing embedding
func (s *Store) UpdateMeeting(ctx context.Context, meeting *Meeting) error {
	meeting.UpdatedAt = time.Now()
	content := s.formatMeetingForEmbedding(meeting)

	_, err := s.db.ExecContext(ctx, `
		UPDATE meetings
		SET title = ?, date = ?, share_url = ?, content = ?, updated_at = ?
		WHERE id = ?
	`, meeting.Title, meeting.Date, meeting.ShareURL, content, meeting.UpdatedAt, meeting.ID)
	if err != nil {
		return fmt.Errorf("failed to update meeting: %w", err)
	}

	s.logger.Printf("✅ Updated meeting %s", meeting.ID)
	return nil
}

// DeleteMeeting removes a meeting
func (s *Store) DeleteMeeting(ctx context.Context, meetingID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM meetings WHERE id = ?`, meetingID)
	return err
}

// scanMeetings scans rows into Meeting structs
func (s *Store) scanMeetings(rows *sql.Rows) ([]*Meeting, error) {
	var meetings []*Meeting
	for rows.Next() {
		var m Meeting
		var shareURL sql.NullString
		if err := rows.Scan(&m.ID, &m.ClientID, &m.Title, &m.Date, &shareURL, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		if shareURL.Valid {
			m.ShareURL = shareURL.String
		}
		meetings = append(meetings, &m)
	}
	return meetings, rows.Err()
}

// float32SliceToVectorStr converts a float32 slice to libsql vector string format: "[1.0,2.0,...]"
func float32SliceToVectorStr(v []float32) string {
	var b strings.Builder
	b.WriteString("[")
	for i, f := range v {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteString("]")
	return b.String()
}

// formatMeetingForEmbedding creates a text representation optimized for embeddings
func (s *Store) formatMeetingForEmbedding(meeting *Meeting) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Meeting: %s\n", meeting.Title))
	builder.WriteString(fmt.Sprintf("Date: %s\n", meeting.Date.Format("2006-01-02")))
	builder.WriteString(fmt.Sprintf("Participants: %s\n\n", strings.Join(meeting.Participants, ", ")))

	if meeting.Summary != nil {
		if highlights, ok := meeting.Summary["high_level_summary"].([]interface{}); ok {
			builder.WriteString("Key Points:\n")
			for _, h := range highlights {
				if str, ok := h.(string); ok {
					builder.WriteString(fmt.Sprintf("- %s\n", str))
				}
			}
			builder.WriteString("\n")
		}

		summaryJSON, _ := json.Marshal(meeting.Summary)
		builder.WriteString(fmt.Sprintf("Full Summary:\n%s\n", string(summaryJSON)))
	}

	return builder.String()
}

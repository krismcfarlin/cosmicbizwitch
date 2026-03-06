package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// CreateJob creates a new background job and returns its ID.
func (s *Store) CreateJob(ctx context.Context, jobType string, payload map[string]interface{}) (string, error) {
	col, err := s.app.FindCollectionByNameOrId("jobs")
	if err != nil {
		return "", fmt.Errorf("find collection: %w", err)
	}

	rec := core.NewRecord(col)
	rec.Set("type", jobType)
	rec.Set("payload", payload)
	rec.Set("status", "pending")
	rec.Set("attempts", 0)
	rec.Set("max_attempts", 3)

	if err := s.app.Save(rec); err != nil {
		return "", fmt.Errorf("insert job: %w", err)
	}

	s.logger.Printf("📋 Created job %s (type: %s)", rec.Id, jobType)
	return rec.Id, nil
}

// GetJob retrieves a job by ID.
func (s *Store) GetJob(ctx context.Context, id string) (*Job, error) {
	rec, err := s.app.FindRecordById("jobs", id)
	if err != nil {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return recordToJob(rec)
}

// GetPendingJobs retrieves pending jobs for processing.
func (s *Store) GetPendingJobs(ctx context.Context, limit int) ([]*Job, error) {
	if limit == 0 {
		limit = 10
	}
	records, err := s.app.FindRecordsByFilter(
		"jobs",
		"status = 'pending' && attempts < max_attempts",
		"+created", limit, 0,
	)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	return recsToJobs(records)
}

// MarkJobStarted marks a job as started (status=processing, increments attempts).
func (s *Store) MarkJobStarted(ctx context.Context, id string) error {
	rec, err := s.app.FindRecordById("jobs", id)
	if err != nil {
		return fmt.Errorf("job not found: %s", id)
	}
	rec.Set("status", "processing")
	rec.Set("started_at", time.Now())
	rec.Set("attempts", rec.GetInt("attempts")+1)
	return s.app.Save(rec)
}

// MarkJobCompleted marks a job as completed.
func (s *Store) MarkJobCompleted(ctx context.Context, id string) error {
	rec, err := s.app.FindRecordById("jobs", id)
	if err != nil {
		return fmt.Errorf("job not found: %s", id)
	}
	rec.Set("status", "completed")
	rec.Set("completed_at", time.Now())
	if err := s.app.Save(rec); err != nil {
		return err
	}
	s.logger.Printf("✅ Job %s completed", id)
	return nil
}

// MarkJobFailed marks a job as failed or resets to pending for retry.
func (s *Store) MarkJobFailed(ctx context.Context, id string, errorMsg string) error {
	rec, err := s.app.FindRecordById("jobs", id)
	if err != nil {
		return fmt.Errorf("job not found: %s", id)
	}

	attempts := rec.GetInt("attempts")
	maxAttempts := rec.GetInt("max_attempts")

	status := "pending" // will retry
	if attempts >= maxAttempts {
		status = "failed"
		s.logger.Printf("❌ Job %s failed permanently after %d attempts", id, attempts)
	} else {
		s.logger.Printf("⚠️ Job %s failed (attempt %d/%d), will retry", id, attempts, maxAttempts)
	}

	rec.Set("status", status)
	rec.Set("error", errorMsg)
	rec.Set("completed_at", time.Now())
	return s.app.Save(rec)
}

// DeleteJob deletes a job.
func (s *Store) DeleteJob(ctx context.Context, id string) error {
	rec, err := s.app.FindRecordById("jobs", id)
	if err != nil {
		return fmt.Errorf("job not found: %s", id)
	}
	return s.app.Delete(rec)
}

// CleanupOldJobs deletes completed/failed jobs older than olderThan duration.
func (s *Store) CleanupOldJobs(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).UTC().Format("2006-01-02 15:04:05")
	records, err := s.app.FindRecordsByFilter(
		"jobs",
		"(status = 'completed' || status = 'failed') && completed_at != '' && completed_at < {:cutoff}",
		"", 0, 0,
		dbx.Params{"cutoff": cutoff},
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup query failed: %w", err)
	}

	count := 0
	for _, rec := range records {
		if err := s.app.Delete(rec); err != nil {
			s.logger.Printf("⚠️ Failed to delete job %s: %v", rec.Id, err)
			continue
		}
		count++
	}

	if count > 0 {
		s.logger.Printf("🧹 Cleaned up %d old jobs", count)
	}
	return count, nil
}

// JobStats returns a count map keyed by status plus a "total" key.
func (s *Store) JobStats(ctx context.Context) (map[string]int, error) {
	records, err := s.app.FindRecordsByFilter("jobs", "id != ''", "", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	stats := make(map[string]int)
	for _, rec := range records {
		stats[rec.GetString("status")]++
		stats["total"]++
	}
	return stats, nil
}

// ListJobs retrieves jobs with optional status filter.
func (s *Store) ListJobs(ctx context.Context, status string, limit int) ([]*Job, error) {
	if limit == 0 {
		limit = 50
	}
	filter := "id != ''"
	params := dbx.Params{}
	if status != "" {
		filter = "status = {:status}"
		params["status"] = status
	}
	records, err := s.app.FindRecordsByFilter("jobs", filter, "-created", limit, 0, params)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	return recsToJobs(records)
}

// RetryFailedJob resets a failed job to pending status.
func (s *Store) RetryFailedJob(ctx context.Context, id string) error {
	rec, err := s.app.FindRecordById("jobs", id)
	if err != nil {
		return fmt.Errorf("job not found: %s", id)
	}
	if rec.GetString("status") != "failed" {
		return fmt.Errorf("job %s is not in failed state", id)
	}
	rec.Set("status", "pending")
	rec.Set("attempts", 0)
	rec.Set("error", "")
	rec.Set("started_at", nil)
	rec.Set("completed_at", nil)
	if err := s.app.Save(rec); err != nil {
		return err
	}
	s.logger.Printf("🔄 Job %s reset for retry", id)
	return nil
}

// GetStuckJobs finds jobs that have been in processing state for too long.
func (s *Store) GetStuckJobs(ctx context.Context, timeout time.Duration) ([]*Job, error) {
	cutoff := time.Now().Add(-timeout).UTC().Format("2006-01-02 15:04:05")
	records, err := s.app.FindRecordsByFilter(
		"jobs",
		"status = 'processing' && started_at != '' && started_at < {:cutoff}",
		"+started_at", 0, 0,
		dbx.Params{"cutoff": cutoff},
	)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	return recsToJobs(records)
}

// UpdateFathomRecordingStatus updates the transcript_status of a fathom recording.
// The fathom recording's PocketBase ID is the fathom_id itself (set during migration).
func (s *Store) UpdateFathomRecordingStatus(ctx context.Context, fathomID, status string) error {
	rec, err := s.app.FindRecordById("fathom_recordings", fathomID)
	if err != nil {
		// Fall back to searching by fathom_id field (for recordings created post-migration)
		records, filterErr := s.app.FindRecordsByFilter(
			"fathom_recordings",
			"fathom_id = {:fid}",
			"", 1, 0,
			dbx.Params{"fid": fathomID},
		)
		if filterErr != nil || len(records) == 0 {
			return fmt.Errorf("fathom recording not found: %s", fathomID)
		}
		rec = records[0]
	}
	rec.Set("transcript_status", status)
	if err := s.app.Save(rec); err != nil {
		return fmt.Errorf("update fathom recording status: %w", err)
	}
	s.logger.Printf("📝 Fathom recording %s status → %s", fathomID, status)
	return nil
}

// UpdateFathomRecordingMeta updates fathom_summary, duration_seconds, and participants
// for a recording in both coaching.db and PocketBase (data.db).
// It is a no-op if both summary and durationSeconds carry no data.
// Returns nil when the PocketBase record is simply not found (not treated as an error).
func (s *Store) UpdateFathomRecordingMeta(ctx context.Context, fathomID, summary string, durationSeconds int, participants []string) error {
	// Skip records with no meaningful data to write.
	if summary == "" && durationSeconds == 0 {
		return nil
	}

	participantsJSON, err := json.Marshal(participants)
	if err != nil {
		return fmt.Errorf("marshal participants: %w", err)
	}

	// Update coaching.db (s.db).
	_, err = s.db.ExecContext(ctx,
		`UPDATE fathom_recordings SET fathom_summary=?, duration_seconds=?, participants=? WHERE fathom_id=?`,
		summary, durationSeconds, string(participantsJSON), fathomID,
	)
	if err != nil {
		return fmt.Errorf("update coaching.db fathom meta for %s: %w", fathomID, err)
	}

	// Update PocketBase (data.db).
	// Try FindRecordById first (fathom_id is used as PB ID during migration).
	rec, pbErr := s.app.FindRecordById("fathom_recordings", fathomID)
	if pbErr != nil {
		// Fall back to filter by fathom_id field.
		records, filterErr := s.app.FindRecordsByFilter(
			"fathom_recordings",
			"fathom_id = {:fid}",
			"", 1, 0,
			dbx.Params{"fid": fathomID},
		)
		if filterErr != nil || len(records) == 0 {
			// Not found in PocketBase — not an error.
			return nil
		}
		rec = records[0]
	}

	rec.Set("fathom_summary", summary)
	rec.Set("duration_seconds", durationSeconds)
	rec.Set("participants", participants)
	// SaveNoValidate bypasses TextField max-length validation (summaries can exceed 5000 chars).
	if err := s.app.SaveNoValidate(rec); err != nil {
		return fmt.Errorf("save pocketbase fathom meta for %s: %w", fathomID, err)
	}

	return nil
}

func recordToJob(rec *core.Record) (*Job, error) {
	job := &Job{
		ID:          rec.Id,
		Type:        rec.GetString("type"),
		Status:      rec.GetString("status"),
		Attempts:    rec.GetInt("attempts"),
		MaxAttempts: rec.GetInt("max_attempts"),
		CreatedAt:   rec.GetDateTime("created").Time(),
	}

	// Unmarshal payload
	payloadRaw := rec.Get("payload")
	switch v := payloadRaw.(type) {
	case map[string]interface{}:
		job.Payload = v
	case string:
		if err := json.Unmarshal([]byte(v), &job.Payload); err != nil {
			job.Payload = map[string]interface{}{"raw": v}
		}
	default:
		// re-marshal and unmarshal to get a plain map
		b, _ := json.Marshal(v)
		_ = json.Unmarshal(b, &job.Payload)
	}

	if v := rec.GetString("error"); v != "" {
		job.Error = &v
	}
	if dt := rec.GetDateTime("started_at"); !dt.IsZero() {
		t := dt.Time()
		job.StartedAt = &t
	}
	if dt := rec.GetDateTime("completed_at"); !dt.IsZero() {
		t := dt.Time()
		job.CompletedAt = &t
	}
	return job, nil
}

func recsToJobs(records []*core.Record) ([]*Job, error) {
	jobs := make([]*Job, 0, len(records))
	for _, rec := range records {
		job, err := recordToJob(rec)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

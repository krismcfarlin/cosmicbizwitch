package processor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"cosmicbizwitch/internal/app/storage"
)

// QueueProcessor handles background job processing
type QueueProcessor struct {
	store        *storage.Store
	logger       *log.Logger
	pollInterval time.Duration
	batchSize    int
	stopCh       chan struct{}
	wg           sync.WaitGroup
	running      bool
	mu           sync.Mutex
}

// QueueConfig holds configuration for the queue processor
type QueueConfig struct {
	PollInterval time.Duration // How often to check for new jobs
	BatchSize    int           // How many jobs to process at once
}

// DefaultQueueConfig returns default configuration
func DefaultQueueConfig() QueueConfig {
	return QueueConfig{
		PollInterval: 5 * time.Second,
		BatchSize:    5,
	}
}

// NewQueueProcessor creates a new queue processor
func NewQueueProcessor(store *storage.Store, config QueueConfig, logger *log.Logger) *QueueProcessor {
	if logger == nil {
		logger = log.Default()
	}

	if config.PollInterval == 0 {
		config.PollInterval = 5 * time.Second
	}

	if config.BatchSize == 0 {
		config.BatchSize = 5
	}

	return &QueueProcessor{
		store:        store,
		logger:       logger,
		pollInterval: config.PollInterval,
		batchSize:    config.BatchSize,
		stopCh:       make(chan struct{}),
	}
}

// Start begins processing jobs in the background
func (q *QueueProcessor) Start(ctx context.Context) error {
	q.mu.Lock()
	if q.running {
		q.mu.Unlock()
		return fmt.Errorf("queue processor already running")
	}
	q.running = true
	q.mu.Unlock()

	q.logger.Println("Starting queue processor")

	q.wg.Add(1)
	go q.processLoop(ctx)

	return nil
}

// Stop gracefully stops the queue processor
func (q *QueueProcessor) Stop() error {
	q.mu.Lock()
	if !q.running {
		q.mu.Unlock()
		return fmt.Errorf("queue processor not running")
	}
	q.running = false
	q.mu.Unlock()

	q.logger.Println("Stopping queue processor")
	close(q.stopCh)
	q.wg.Wait()
	q.logger.Println("Queue processor stopped")

	return nil
}

// IsRunning returns whether the processor is currently running
func (q *QueueProcessor) IsRunning() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.running
}

// processLoop is the main processing loop
func (q *QueueProcessor) processLoop(ctx context.Context) {
	defer q.wg.Done()

	ticker := time.NewTicker(q.pollInterval)
	defer ticker.Stop()

	q.logger.Printf("Queue processor polling every %v", q.pollInterval)

	for {
		select {
		case <-ctx.Done():
			q.logger.Println("Context cancelled, stopping processor")
			return
		case <-q.stopCh:
			q.logger.Println("Stop signal received")
			return
		case <-ticker.C:
			if err := q.processBatch(ctx); err != nil {
				q.logger.Printf("Error processing batch: %v", err)
			}
		}
	}
}

// processBatch processes a batch of pending jobs
func (q *QueueProcessor) processBatch(ctx context.Context) error {
	// Get pending jobs
	jobs, err := q.store.GetPendingJobs(ctx, q.batchSize)
	if err != nil {
		return fmt.Errorf("failed to get pending jobs: %w", err)
	}

	if len(jobs) == 0 {
		return nil // No jobs to process
	}

	q.logger.Printf("Processing batch of %d jobs", len(jobs))

	// Process each job
	for _, job := range jobs {
		if err := q.processJob(ctx, job); err != nil {
			q.logger.Printf("Job %s failed: %v", job.ID, err)
		}
	}

	return nil
}

// processJob processes a single job
func (q *QueueProcessor) processJob(ctx context.Context, job *storage.Job) error {
	// Mark job as started
	if err := q.store.MarkJobStarted(ctx, job.ID); err != nil {
		return fmt.Errorf("failed to mark job as started: %w", err)
	}

	q.logger.Printf("Processing job %s (type: %s, attempt: %d/%d)",
		job.ID, job.Type, job.Attempts+1, job.MaxAttempts)

	// TODO: Add your job type cases here, for example:
	//   case "send_email":
	//       err = q.processSendEmail(ctx, job.Payload)
	var err error
	switch job.Type {
	default:
		err = fmt.Errorf("unknown job type: %s", job.Type)
	}

	// Update job status
	if err != nil {
		if markErr := q.store.MarkJobFailed(ctx, job.ID, err.Error()); markErr != nil {
			q.logger.Printf("Failed to mark job as failed: %v", markErr)
		}
		return err
	}

	if err := q.store.MarkJobCompleted(ctx, job.ID); err != nil {
		q.logger.Printf("Failed to mark job as completed: %v", err)
		return err
	}

	return nil
}

// CleanupOldJobs removes completed jobs older than the specified duration
func (q *QueueProcessor) CleanupOldJobs(ctx context.Context, olderThan time.Duration) (int, error) {
	count, err := q.store.CleanupOldJobs(ctx, olderThan)
	if err != nil {
		return 0, fmt.Errorf("cleanup failed: %w", err)
	}

	if count > 0 {
		q.logger.Printf("Cleaned up %d old jobs", count)
	}

	return count, nil
}

// GetStats returns statistics about the job queue
func (q *QueueProcessor) GetStats(ctx context.Context) (map[string]int, error) {
	stats, err := q.store.JobStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return stats, nil
}

// CheckStuckJobs finds and handles jobs that have been processing too long
func (q *QueueProcessor) CheckStuckJobs(ctx context.Context, timeout time.Duration) error {
	jobs, err := q.store.GetStuckJobs(ctx, timeout)
	if err != nil {
		return fmt.Errorf("failed to get stuck jobs: %w", err)
	}

	if len(jobs) == 0 {
		return nil
	}

	q.logger.Printf("Found %d stuck jobs", len(jobs))

	for _, job := range jobs {
		// Mark as failed so it can be retried
		if err := q.store.MarkJobFailed(ctx, job.ID, "job timeout: stuck in processing"); err != nil {
			q.logger.Printf("Failed to mark stuck job %s as failed: %v", job.ID, err)
		} else {
			q.logger.Printf("Marked stuck job %s for retry", job.ID)
		}
	}

	return nil
}

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"
	_ "github.com/tursodatabase/go-libsql"
)

// Store is the unified database layer.
// app (data.db via PocketBase) stores all business collections.
// db (coaching.db via go-libsql) is used only for meetings with F32_BLOB vector ops.
type Store struct {
	app    core.App
	db     *sql.DB // coaching.db — meetings + vector ops only
	logger *log.Logger
}

// Config holds storage configuration.
type Config struct {
	Logger *log.Logger
}

// New creates a new Store, setting up PocketBase collections and running one-time data migration.
func New(app core.App, legacyDB *sql.DB, cfg Config) (*Store, error) {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	s := &Store{app: app, db: legacyDB, logger: cfg.Logger}

	// Configure coaching.db for concurrent access.
	// go-libsql's Exec() fails on PRAGMA statements that return rows; use QueryRow().Scan() instead.
	var journalMode string
	if err := legacyDB.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&journalMode); err != nil {
		cfg.Logger.Printf("⚠️ Failed to set WAL mode: %v", err)
	}
	var busyTimeout int
	if err := legacyDB.QueryRow(`PRAGMA busy_timeout=5000`).Scan(&busyTimeout); err != nil {
		cfg.Logger.Printf("⚠️ Failed to set busy timeout: %v", err)
	}
	legacyDB.SetMaxOpenConns(1)
	legacyDB.SetMaxIdleConns(1)

	// Ensure meetings table exists in coaching.db (vector ops)
	if _, err := legacyDB.Exec(`
		CREATE TABLE IF NOT EXISTS meetings (
			id        TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			title     TEXT NOT NULL,
			date      TIMESTAMP NOT NULL,
			share_url TEXT,
			content   TEXT NOT NULL DEFAULT '',
			embedding F32_BLOB(1536),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_meetings_client_id ON meetings(client_id);
		CREATE INDEX IF NOT EXISTS idx_meetings_date ON meetings(date);
	`); err != nil {
		cfg.Logger.Printf("⚠️ Failed to create meetings table: %v", err)
	}

	// Ensure prompt_usage_log stays in coaching.db
	if _, err := legacyDB.Exec(`
		CREATE TABLE IF NOT EXISTS prompt_usage_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			prompt_id TEXT NOT NULL,
			meeting_id TEXT,
			client_id TEXT,
			execution_time_ms INTEGER,
			success BOOLEAN,
			error_message TEXT,
			used_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_usage_prompt ON prompt_usage_log(prompt_id);
		CREATE INDEX IF NOT EXISTS idx_usage_meeting ON prompt_usage_log(meeting_id);
	`); err != nil {
		cfg.Logger.Printf("⚠️ Failed to create prompt_usage_log table: %v", err)
	}

	// Create PocketBase collections and migrate data from coaching.db
	if err := SetupCollections(app, legacyDB, cfg.Logger); err != nil {
		return nil, fmt.Errorf("setup collections: %w", err)
	}

	cfg.Logger.Println("✅ Storage initialized successfully")
	return s, nil
}

// Close closes the coaching.db connection.
func (s *Store) Close() error { return s.db.Close() }

// RawDB returns the raw coaching.db connection (for meetings + vector ops).
func (s *Store) RawDB() *sql.DB { return s.db }

// App returns the PocketBase core.App (used for auth validation).
func (s *Store) App() core.App { return s.app }

// Health checks coaching.db connectivity.
func (s *Store) Health(ctx context.Context) error { return s.db.PingContext(ctx) }

// ---- Data types ----

// Client represents a coaching client.
type Client struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Email     *string                `json:"email,omitempty"`
	Company   *string                `json:"company,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// ActionItem represents a task extracted from a meeting.
type ActionItem struct {
	ID              string     `json:"id"`
	MeetingID       string     `json:"meeting_id"`
	ClientID        string     `json:"client_id"`
	ActionText      string     `json:"action_text"`
	Owner           *string    `json:"owner,omitempty"`
	DueDate         *string    `json:"due_date,omitempty"`
	Priority        string     `json:"priority"`
	Status          string     `json:"status"`
	ClickUpTaskID   *string    `json:"clickup_task_id,omitempty"`
	ClickUpTaskURL  *string    `json:"clickup_task_url,omitempty"`
	ClickUpSyncedAt *time.Time `json:"clickup_synced_at,omitempty"`
	ApprovedBy      *string    `json:"approved_by,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// MeetingIssue represents a risk/concern/blocker from a meeting.
type MeetingIssue struct {
	ID              string     `json:"id"`
	MeetingID       string     `json:"meeting_id"`
	ClientID        string     `json:"client_id"`
	IssueType       string     `json:"issue_type"`
	Description     string     `json:"description"`
	Owner           *string    `json:"owner,omitempty"`
	Severity        string     `json:"severity"`
	Status          string     `json:"status"`
	ClickUpTaskID   *string    `json:"clickup_task_id,omitempty"`
	ClickUpTaskURL  *string    `json:"clickup_task_url,omitempty"`
	ClickUpSyncedAt *time.Time `json:"clickup_synced_at,omitempty"`
	ApprovedBy      *string    `json:"approved_by,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Job represents a background task.
type Job struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload"`
	Status      string                 `json:"status"`
	Attempts    int                    `json:"attempts"`
	MaxAttempts int                    `json:"max_attempts"`
	Error       *string                `json:"error,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

// ---- Client methods (PocketBase DAO) ----

// GetClient retrieves a client by ID.
func (s *Store) GetClient(ctx context.Context, clientID string) (*Client, error) {
	rec, err := s.app.FindRecordById("clients", clientID)
	if err != nil {
		return nil, fmt.Errorf("client not found: %s", clientID)
	}
	return recordToClient(rec), nil
}

// CreateClient creates a new client in PocketBase.
func (s *Store) CreateClient(ctx context.Context, client *Client) error {
	col, err := s.app.FindCollectionByNameOrId("clients")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	if client.ID != "" {
		rec.Id = client.ID
	}
	rec.Set("name", client.Name)
	if client.Email != nil {
		rec.Set("email", *client.Email)
	}
	if client.Company != nil {
		rec.Set("company", *client.Company)
	}
	return s.app.SaveNoValidate(rec)
}

// ListClients retrieves all clients ordered by name.
func (s *Store) ListClients(ctx context.Context) ([]*Client, error) {
	records, err := s.app.FindRecordsByFilter("clients", "id != ''", "+name", 0, 0)
	if err != nil {
		return nil, err
	}
	clients := make([]*Client, 0, len(records))
	for _, rec := range records {
		clients = append(clients, recordToClient(rec))
	}
	return clients, nil
}

func recordToClient(rec *core.Record) *Client {
	c := &Client{
		ID:        rec.Id,
		Name:      rec.GetString("name"),
		CreatedAt: rec.GetDateTime("created").Time(),
		UpdatedAt: rec.GetDateTime("updated").Time(),
	}
	if v := rec.GetString("email"); v != "" {
		c.Email = &v
	}
	if v := rec.GetString("company"); v != "" {
		c.Company = &v
	}
	return c
}

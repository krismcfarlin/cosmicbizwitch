package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"cosmicbizwitch/internal/app/clickfunnels"
	"cosmicbizwitch/internal/app/mcp"
	"cosmicbizwitch/internal/app/storage"
	"cosmicbizwitch/internal/app/triggers"
	"cosmicbizwitch/internal/ui"
	"cosmicbizwitch/pkg/workflow"
)

// cfTagsCache holds a snapshot of all workspace tags with a TTL.
type cfTagsCache struct {
	mu        sync.RWMutex
	tags      []clickfunnels.Tag
	fetchedAt time.Time
	ttl       time.Duration
}

func (c *cfTagsCache) get() ([]clickfunnels.Tag, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tags == nil || time.Since(c.fetchedAt) > c.ttl {
		return nil, false
	}
	return c.tags, true
}

func (c *cfTagsCache) set(tags []clickfunnels.Tag) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tags = tags
	c.fetchedAt = time.Now()
}

// Server represents the HTTP server
type Server struct {
	store           *storage.Store
	mcpServer       *mcp.MCPServer
	engine          *workflow.Engine
	triggers        *triggers.Manager
	settings        *storage.SettingsManager
	logger          *log.Logger
	logBuffer       *LogBuffer
	httpServer      *http.Server
	oauthStateMu    sync.Mutex
	oauthPending    map[string]pendingOAuth
	cfTags          cfTagsCache
}

// Config holds server configuration
type Config struct {
	Port      int
	Logger    *log.Logger
	LogBuffer *LogBuffer
	CFClient  *clickfunnels.Client
	Engine    *workflow.Engine
	Triggers  *triggers.Manager
	Settings  *storage.SettingsManager
}

// New creates a new server instance
func New(store *storage.Store, cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	s := &Server{
		store:        store,
		mcpServer:    mcp.NewMCPServer(store, cfg.Logger, cfg.CFClient),
		engine:       cfg.Engine,
		triggers:     cfg.Triggers,
		settings:     cfg.Settings,
		logger:       cfg.Logger,
		logBuffer:    cfg.LogBuffer,
		oauthPending: make(map[string]pendingOAuth),
		cfTags:       cfTagsCache{ttl: 5 * time.Minute},
	}

	// Create debug tables that are not managed by PocketBase collections.
	if store != nil {
		if _, err := store.App().DB().NewQuery(`
			CREATE TABLE IF NOT EXISTS telegram_messages (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				received_at TEXT NOT NULL,
				raw_json    TEXT NOT NULL
			)
		`).Execute(); err != nil {
			cfg.Logger.Printf("server: create telegram_messages table: %v", err)
		}
	}

	// Create HTTP server
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      s.loggingMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// registerRoutes sets up HTTP routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	// Auth
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("GET /logout", s.handleLogout)

	// Birth form submission (no auth required)
	mux.HandleFunc("POST /birthform", s.handleBirthForm)

	// MCP endpoints (require superuser auth token)
	mux.HandleFunc("/mcp/tools", s.requireMCPAuth(s.handleMCPListTools))
	mux.HandleFunc("/mcp/call", s.requireMCPAuth(s.handleMCPCallTool))

	// Static assets for React SPA (no auth — assets are fingerprinted)
	mux.Handle("/assets/", ui.Handler())

	// Logs viewer (auth required)
	mux.HandleFunc("GET /logs", s.requireAuth(s.handleLogsPage))
	mux.HandleFunc("GET /logs/data", s.requireAuth(s.handleLogsData))
	mux.HandleFunc("GET /logs/stream", s.requireAuth(s.handleLogsStream))

	// Workflow SPA pages (auth required)
	mux.HandleFunc("GET /workflows/builder", s.requireAuth(s.handleLogsPage))
	mux.HandleFunc("GET /workflows", s.requireAuth(s.handleLogsPage))
	mux.HandleFunc("GET /workflows/{path...}", s.requireAuth(s.handleLogsPage))
	mux.HandleFunc("GET /history", s.requireAuth(s.handleLogsPage))

	// Workflow API (auth required)
	mux.HandleFunc("GET /api/workflows/stream", s.requireAuth(s.handleWorkflowStream))
	mux.HandleFunc("GET /api/workflows/graphs", s.requireAuth(s.handleWorkflowGraphs))
	mux.HandleFunc("GET /api/workflows/activities", s.requireAuth(s.handleWorkflowActivitiesList))
	mux.HandleFunc("POST /api/workflows/graphs", s.requireAuth(s.handleGraphSave))
	mux.HandleFunc("POST /api/workflows/graphs/{name}/duplicate", s.requireAuth(s.handleGraphDuplicate))
	mux.HandleFunc("DELETE /api/workflows/graphs/{name}", s.requireAuth(s.handleGraphDelete))
	mux.HandleFunc("POST /api/workflows/execute-node", s.requireAuth(s.handleExecuteNode))
	mux.HandleFunc("GET /api/pb/collections/{name}/fields", s.requireAuth(s.handlePbCollectionFields))
	mux.HandleFunc("GET /api/llm/models", s.requireAuth(s.handleLLMModels))
	mux.HandleFunc("GET /api/openrouter/keys", s.requireAuth(s.handleOpenRouterKeys))
	mux.HandleFunc("GET /api/workflows/{id}", s.requireAuth(s.handleWorkflowGet))
	mux.HandleFunc("GET /api/workflows/{id}/activities", s.requireAuth(s.handleWorkflowActivities))
	mux.HandleFunc("GET /api/workflows", s.requireAuth(s.handleWorkflowList))
	mux.HandleFunc("POST /api/workflows", s.requireAuth(s.handleWorkflowCreate))
	mux.HandleFunc("POST /api/workflows/run-sync", s.requireAuth(s.handleWorkflowRunSync))
	mux.HandleFunc("DELETE /api/workflows/{id}", s.requireAuth(s.handleWorkflowDelete))
	mux.HandleFunc("POST /api/workflows/{id}/cancel", s.requireAuth(s.handleWorkflowCancel))
	mux.HandleFunc("POST /api/workflows/{id}/restart", s.requireAuth(s.handleWorkflowRestart))
	mux.HandleFunc("POST /api/workflows/{id}/trigger", s.requireAuth(s.handleWorkflowTrigger))

	// Webhook / trigger fire (no auth — public endpoints)
	mux.HandleFunc("POST /webhooks/{token}", s.handleWebhook)
	mux.HandleFunc("POST /trigger/{id}", s.handleTriggerFire)

	// Trigger CRUD (auth required)
	mux.HandleFunc("GET /api/triggers", s.requireAuth(s.handleTriggerList))
	mux.HandleFunc("POST /api/triggers", s.requireAuth(s.handleTriggerCreate))
	mux.HandleFunc("GET /api/triggers/{id}", s.requireAuth(s.handleTriggerGet))
	mux.HandleFunc("PUT /api/triggers/{id}", s.requireAuth(s.handleTriggerUpdate))
	mux.HandleFunc("DELETE /api/triggers/{id}", s.requireAuth(s.handleTriggerDelete))

	// Trigger pages (SPA)
	mux.HandleFunc("GET /triggers", s.requireAuth(s.handleLogsPage))
	mux.HandleFunc("GET /triggers/{path...}", s.requireAuth(s.handleLogsPage))

	// Google OAuth & Drive browser (auth required)
	mux.HandleFunc("GET /api/google/auth/start", s.requireAuth(s.handleGoogleAuthStart))
	mux.HandleFunc("GET /api/google/auth/callback", s.handleGoogleAuthCallback)
	mux.HandleFunc("GET /api/google/auth/status", s.requireAuth(s.handleGoogleAuthStatus))
	mux.HandleFunc("GET /api/google/drive/browse", s.requireAuth(s.handleGoogleDriveBrowse))
	mux.HandleFunc("GET /api/workflows/gdrive-content", s.requireAuth(s.handleGdriveGetContent))

	// Slack OAuth (auth required except callback which Slack redirects to)
	mux.HandleFunc("GET /api/slack/auth/start", s.requireAuth(s.handleSlackAuthStart))
	mux.HandleFunc("GET /api/slack/auth/callback", s.handleSlackAuthCallback)
	mux.HandleFunc("GET /api/slack/auth/status", s.requireAuth(s.handleSlackAuthStatus))

	// Telegram bot (webhook is public — no auth; management endpoints require auth)
	mux.HandleFunc("POST /webhooks/telegram", s.handleTelegramWebhook)
	mux.HandleFunc("GET /api/telegram/status", s.requireAuth(s.handleTelegramStatus))
	mux.HandleFunc("POST /api/telegram/set-webhook", s.requireAuth(s.handleTelegramSetWebhook))
	mux.HandleFunc("GET /api/telegram/messages", s.requireAuth(s.handleTelegramMessages))
	mux.HandleFunc("DELETE /api/telegram/messages", s.requireAuth(s.handleTelegramMessagesClear))
	mux.HandleFunc("POST /api/telegram/index", s.requireAuth(s.handleTelegramIndex))
	mux.HandleFunc("GET /api/telegram/vector-stats", s.requireAuth(s.handleTelegramVectorStats))

	// ClickFunnels API helpers (auth required)
	mux.HandleFunc("GET /api/cf/tags", s.requireAuth(s.handleCFTags))

	// Settings API (auth required)
	mux.HandleFunc("GET /app/settings", s.requireAuth(s.handleSettingsList))
	mux.HandleFunc("PUT /app/settings/{key}", s.requireAuth(s.handleSettingsUpdate))

	// Settings SPA page (auth required)
	mux.HandleFunc("GET /settings", s.requireAuth(s.handleLogsPage))
	mux.HandleFunc("GET /settings/{path...}", s.requireAuth(s.handleLogsPage))

	// Telegram SPA pages (auth required)
	mux.HandleFunc("GET /telegram", s.requireAuth(s.handleLogsPage))
	mux.HandleFunc("GET /telegram/{path...}", s.requireAuth(s.handleLogsPage))

	// Home (catch-all, auth required)
	mux.HandleFunc("/", s.handleHome)
}

// Handler returns the HTTP handler (mux with logging middleware).
// Used when PocketBase manages the HTTP server instead of our own.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// HTTP Handlers

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Check database health
	if err := s.store.Health(ctx); err != nil {
		s.respondError(w, http.StatusServiceUnavailable, "database unhealthy", err)
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"service": "cosmicbizwitch",
		"time":    time.Now().Format(time.RFC3339),
	})
}

// handleMCPListTools returns the list of available MCP tools, including workflow activities.
func (s *Server) handleMCPListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tools := s.mcpServer.ListTools()

	// Add run_workflow tool.
	if s.engine != nil {
		tools = append(tools, mcp.Tool{
			Name:        "run_workflow",
			Description: "Start a workflow by graph name with an optional JSON context payload. Returns the workflow ID and status.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"graph_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the registered workflow graph to run",
					},
					"context": map[string]interface{}{
						"type":        "object",
						"description": "JSON object passed as the initial workflow context/payload",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Optional human-readable name for this workflow run",
					},
				},
				"required": []string{"graph_name"},
			},
		})
	}

	// Merge in all registered workflow activities.
	if s.engine != nil {
		for _, info := range s.engine.ListActivities() {
			tools = append(tools, activityToMCPTool(info))
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"tools": tools,
	})
}

// handleMCPCallTool handles MCP tool calls, routing to workflow activities when applicable.
func (s *Server) handleMCPCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req mcp.ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Handle run_workflow tool — synchronous: poll until completed/failed or 60s timeout.
	if s.engine != nil && req.Name == "run_workflow" {
		graphName, _ := req.Arguments["graph_name"].(string)
		if graphName == "" {
			s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
				Content: []mcp.ContentBlock{{Type: "text", Text: "Error: graph_name is required"}},
				IsError: true,
			})
			return
		}
		wfCtx, _ := req.Arguments["context"].(map[string]interface{})
		name, _ := req.Arguments["name"].(string)
		wf, err := s.engine.CreateWorkflow(r.Context(), name, graphName, wfCtx, nil)
		if err != nil {
			s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
				Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
				IsError: true,
			})
			return
		}

		// Poll until terminal state or 60s timeout.
		deadline := time.Now().Add(60 * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		pollCtx := r.Context()
		for {
			select {
			case <-pollCtx.Done():
				s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
					Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("workflow timed out after 60s, id=%s", wf.ID)}},
					IsError: true,
				})
				return
			case <-ticker.C:
				if time.Now().After(deadline) {
					s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
						Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("workflow timed out after 60s, id=%s", wf.ID)}},
						IsError: true,
					})
					return
				}
				updated, pollErr := s.engine.Store().GetWorkflow(pollCtx, wf.ID)
				if pollErr != nil {
					s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
						Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Error polling workflow %s: %v", wf.ID, pollErr)}},
						IsError: true,
					})
					return
				}
				switch updated.Status {
				case workflow.StatusCompleted:
					outJSON, _ := json.Marshal(updated.Context)
					s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
						Content: []mcp.ContentBlock{{Type: "text", Text: string(outJSON)}},
					})
					return
				case workflow.StatusFailed:
					s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
						Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("workflow %s (%s) failed", wf.ID, graphName)}},
						IsError: true,
					})
					return
				}
			}
		}
	}

	// Route to workflow activity if registered.
	if s.engine != nil && s.engine.HasActivity(req.Name) {
		out, err := s.engine.ExecuteActivity(r.Context(), req.Name, req.Arguments)
		if err != nil {
			if errors.Is(err, workflow.ErrSkip) {
				s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
					Content: []mcp.ContentBlock{{Type: "text", Text: "Activity skipped"}},
				})
				return
			}
			s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
				Content: []mcp.ContentBlock{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
				IsError: true,
			})
			return
		}
		outJSON, _ := json.Marshal(out)
		s.respondJSON(w, http.StatusOK, mcp.ToolCallResponse{
			Content: []mcp.ContentBlock{{Type: "text", Text: string(outJSON)}},
		})
		return
	}

	// Delegate to static MCP server.
	resp, err := s.mcpServer.CallTool(r.Context(), req)
	if err != nil {
		s.logger.Printf("MCP tool call failed: %v", err)
		s.respondJSON(w, http.StatusOK, resp)
		return
	}

	s.respondJSON(w, http.StatusOK, resp)
}

// activityToMCPTool converts a workflow ActivityInfo into an MCP Tool definition.
func activityToMCPTool(info workflow.ActivityInfo) mcp.Tool {
	props := map[string]interface{}{}
	required := []string{}
	for _, f := range info.Meta.InputFields {
		prop := map[string]interface{}{"description": f.Description}
		switch f.Type {
		case "number":
			prop["type"] = "number"
		case "boolean":
			prop["type"] = "boolean"
		default:
			prop["type"] = "string"
		}
		if len(f.Options) > 0 {
			prop["enum"] = f.Options
		}
		props[f.Name] = prop
		if f.Required {
			required = append(required, f.Name)
		}
	}
	schema := map[string]interface{}{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return mcp.Tool{
		Name:        info.Name,
		Description: info.Meta.Description,
		InputSchema: schema,
	}
}

// handleSettingsList returns all app settings. Secret values are masked as empty strings.
func (s *Server) handleSettingsList(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		s.respondJSON(w, http.StatusOK, []interface{}{})
		return
	}
	settings, err := s.settings.All()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load settings", err)
		return
	}
	s.respondJSON(w, http.StatusOK, settings)
}

// handleSettingsUpdate writes a new value for the given setting key and reloads the CF client.
func (s *Server) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		http.Error(w, "settings not available", http.StatusServiceUnavailable)
		return
	}
	key := r.PathValue("key")
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.settings.Set(key, body.Value); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save setting", err)
		return
	}
	if err := s.settings.Reload(s.logger); err != nil {
		s.logger.Printf("settings reload after update: %v", err)
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCFTags returns all ClickFunnels workspace tags for the tag picker UI.
// Tags are cached in-memory for 5 minutes; the optional ?name= query param does
// case-insensitive substring filtering server-side (CF API filter[name] is exact-only).
func (s *Server) handleCFTags(w http.ResponseWriter, r *http.Request) {
	cf := s.settings.CFClient()
	if cf == nil {
		s.respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "ClickFunnels not configured"})
		return
	}

	all, cached := s.cfTags.get()
	if !cached {
		var err error
		all, err = cf.ListTags(r.Context(), "")
		if err != nil {
			s.respondError(w, http.StatusBadGateway, "failed to fetch CF tags", err)
			return
		}
		s.cfTags.set(all)
		s.logger.Printf("cf tags: fetched %d tags from API (cache miss)", len(all))
	}

	tags := all
	nameFilter := r.URL.Query().Get("name")
	if nameFilter != "" {
		lower := strings.ToLower(nameFilter)
		filtered := make([]clickfunnels.Tag, 0, len(all))
		for _, t := range all {
			if strings.Contains(strings.ToLower(t.Name), lower) {
				filtered = append(filtered, t)
			}
		}
		tags = filtered
	}

	s.respondJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// Helper functions

func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Printf("Failed to encode JSON response: %v", err)
	}
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string, err error) {
	s.logger.Printf("%s: %v", message, err)

	s.respondJSON(w, status, map[string]interface{}{
		"error":   message,
		"details": err.Error(),
	})
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		// Skip successful GET requests to reduce log noise
		if r.Method == http.MethodGet && wrapped.status >= 200 && wrapped.status < 300 {
			return
		}
		s.logger.Printf("%s %s - %d (%v)", r.Method, r.URL.Path, wrapped.status, duration)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher so SSE streams work through the logging middleware.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Printf("Starting server on %s", s.httpServer.Addr)

	// Start HTTP server in goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("HTTP server error: %v", err)
		}
	}()

	s.logger.Printf("Server started successfully on %s", s.httpServer.Addr)
	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Println("Shutting down server...")

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %w", err)
	}

	s.logger.Println("Server stopped successfully")
	return nil
}

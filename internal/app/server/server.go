package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"cosmicbizwitch/internal/app/clickfunnels"
	"cosmicbizwitch/internal/app/mcp"
	"cosmicbizwitch/internal/app/storage"
	"cosmicbizwitch/internal/app/triggers"
	"cosmicbizwitch/internal/ui"
	"cosmicbizwitch/pkg/workflow"
)

// Server represents the HTTP server
type Server struct {
	store      *storage.Store
	mcpServer  *mcp.MCPServer
	engine     *workflow.Engine
	triggers   *triggers.Manager
	settings   *storage.SettingsManager
	logger     *log.Logger
	logBuffer  *LogBuffer
	httpServer *http.Server
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
		store:     store,
		mcpServer: mcp.NewMCPServer(store, cfg.Logger, cfg.CFClient),
		engine:    cfg.Engine,
		triggers:  cfg.Triggers,
		settings:  cfg.Settings,
		logger:    cfg.Logger,
		logBuffer: cfg.LogBuffer,
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

	// Workflow API (auth required)
	mux.HandleFunc("GET /api/workflows/stream", s.requireAuth(s.handleWorkflowStream))
	mux.HandleFunc("GET /api/workflows/graphs", s.requireAuth(s.handleWorkflowGraphs))
	mux.HandleFunc("GET /api/workflows/activities", s.requireAuth(s.handleWorkflowActivitiesList))
	mux.HandleFunc("POST /api/workflows/graphs", s.requireAuth(s.handleGraphSave))
	mux.HandleFunc("POST /api/workflows/execute-node", s.requireAuth(s.handleExecuteNode))
	mux.HandleFunc("GET /api/pb/collections/{name}/fields", s.requireAuth(s.handlePbCollectionFields))
	mux.HandleFunc("GET /api/workflows/{id}", s.requireAuth(s.handleWorkflowGet))
	mux.HandleFunc("GET /api/workflows/{id}/activities", s.requireAuth(s.handleWorkflowActivities))
	mux.HandleFunc("GET /api/workflows", s.requireAuth(s.handleWorkflowList))
	mux.HandleFunc("POST /api/workflows", s.requireAuth(s.handleWorkflowCreate))
	mux.HandleFunc("DELETE /api/workflows/{id}", s.requireAuth(s.handleWorkflowDelete))
	mux.HandleFunc("POST /api/workflows/{id}/cancel", s.requireAuth(s.handleWorkflowCancel))
	mux.HandleFunc("POST /api/workflows/{id}/restart", s.requireAuth(s.handleWorkflowRestart))
	mux.HandleFunc("POST /api/workflows/{id}/trigger", s.requireAuth(s.handleWorkflowTrigger))

	// Webhook (no auth — public endpoint)
	mux.HandleFunc("POST /webhooks/{token}", s.handleWebhook)

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
	mux.HandleFunc("GET /api/google/auth/status", s.requireAuth(s.handleGoogleAuthStatus))
	mux.HandleFunc("GET /api/google/drive/browse", s.requireAuth(s.handleGoogleDriveBrowse))

	// Settings API (auth required)
	mux.HandleFunc("GET /app/settings", s.requireAuth(s.handleSettingsList))
	mux.HandleFunc("PUT /app/settings/{key}", s.requireAuth(s.handleSettingsUpdate))

	// Settings SPA page (auth required)
	mux.HandleFunc("GET /settings", s.requireAuth(s.handleLogsPage))
	mux.HandleFunc("GET /settings/{path...}", s.requireAuth(s.handleLogsPage))

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

// handleMCPListTools returns the list of available MCP tools
func (s *Server) handleMCPListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tools := s.mcpServer.ListTools()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"tools": tools,
	})
}

// handleMCPCallTool handles MCP tool calls
func (s *Server) handleMCPCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode request
	var req mcp.ToolCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Call tool
	resp, err := s.mcpServer.CallTool(r.Context(), req)
	if err != nil {
		s.logger.Printf("MCP tool call failed: %v", err)
		// Return the error response from MCP server
		s.respondJSON(w, http.StatusOK, resp)
		return
	}

	s.respondJSON(w, http.StatusOK, resp)
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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

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
	logger     *log.Logger
	logBuffer  *LogBuffer
	httpServer *http.Server
}

// Config holds server configuration
type Config struct {
	Port      int
	Logger    *log.Logger
	LogBuffer *LogBuffer
	Engine    *workflow.Engine
	Triggers  *triggers.Manager
}

// New creates a new server instance
func New(store *storage.Store, cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	s := &Server{
		store:     store,
		mcpServer: mcp.NewMCPServer(store, cfg.Logger),
		engine:    cfg.Engine,
		triggers:  cfg.Triggers,
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
	mux.HandleFunc("GET /api/workflows/{id}", s.requireAuth(s.handleWorkflowGet))
	mux.HandleFunc("GET /api/workflows/{id}/activities", s.requireAuth(s.handleWorkflowActivities))
	mux.HandleFunc("GET /api/workflows", s.requireAuth(s.handleWorkflowList))
	mux.HandleFunc("POST /api/workflows", s.requireAuth(s.handleWorkflowCreate))
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

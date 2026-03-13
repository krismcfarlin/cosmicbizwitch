package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"cosmicbizwitch/internal/ui"
)

// handleLogsPage serves the React log viewer SPA.
func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	content := ui.IndexHTML()
	if len(content) == 0 {
		http.Error(w, "React app not built. Run: cd web && npm install && npm run build", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content) //nolint:errcheck
}

// handleLogsData returns recent log lines as JSON.
// Query params: lines (int, default 200), filter (string).
func (s *Server) handleLogsData(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("lines"))
	if err != nil || n < 1 || n > 5000 {
		n = 200
	}
	filter := r.URL.Query().Get("filter")

	if s.logBuffer == nil {
		s.respondJSON(w, http.StatusOK, []LogLine{})
		return
	}

	lines := s.logBuffer.Lines(n, filter)
	// Reverse so newest is first.
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	s.respondJSON(w, http.StatusOK, lines)
}

// handleLogsStream is an SSE endpoint that streams log lines in real time.
// It first sends up to 500 existing lines, then streams new lines as they arrive.
func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	if s.logBuffer == nil {
		fmt.Fprint(w, "data: {}\n\n")
		flusher.Flush()
		return
	}

	// Send existing lines (oldest-first; React prepends each one so UI shows newest at top).
	for _, line := range s.logBuffer.Lines(500, "") {
		b, _ := json.Marshal(line)
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	flusher.Flush()

	// Subscribe to new lines.
	ch := s.logBuffer.Subscribe()
	defer s.logBuffer.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(line)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

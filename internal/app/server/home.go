package server

import (
	"net/http"
)

// handleHome redirects / to /workflows.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/workflows", http.StatusFound)
}

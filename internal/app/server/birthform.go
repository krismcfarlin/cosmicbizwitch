package server

import (
	"encoding/json"
	"net/http"

	"cosmicbizwitch/internal/app/storage"
)

// handleBirthForm receives a ClickFunnels webhook, extracts the CF contact ID,
// stores it, and triggers the downstream workflow (placeholder).
func (s *Server) handleBirthForm(w http.ResponseWriter, r *http.Request) {
	var payload storage.CFWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.SubjectID == 0 {
		http.Error(w, "missing subject_id", http.StatusBadRequest)
		return
	}

	s.logger.Printf("birthform webhook: contact_id=%d event_id=%s", payload.SubjectID, payload.EventID)

	if err := s.store.StoreBirthFormSubmission(r.Context(), payload); err != nil {
		s.logger.Printf("birthform: store failed: %v", err)
		s.respondError(w, http.StatusInternalServerError, "failed to store submission", err)
		return
	}

	// TODO: trigger workflow with payload.SubjectID

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

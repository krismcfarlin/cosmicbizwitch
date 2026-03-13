package server

import (
	"encoding/json"
	"net/http"

	"cosmicbizwitch/internal/app/triggers"
)

func (s *Server) handleTriggerList(w http.ResponseWriter, r *http.Request) {
	list, err := s.triggers.List()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "list triggers failed", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"triggers": list})
}

func (s *Server) handleTriggerCreate(w http.ResponseWriter, r *http.Request) {
	var t triggers.Trigger
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	created, err := s.triggers.Create(&t)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "create trigger failed", err)
		return
	}
	s.respondJSON(w, http.StatusCreated, map[string]any{"trigger": created})
}

func (s *Server) handleTriggerGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.triggers.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"trigger": t})
}

func (s *Server) handleTriggerUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var t triggers.Trigger
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	updated, err := s.triggers.Update(id, &t)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "update trigger failed", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"trigger": updated})
}

func (s *Server) handleTriggerDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.triggers.Delete(id); err != nil {
		s.respondError(w, http.StatusBadRequest, "delete trigger failed", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// handleWebhook is the public endpoint — no auth required.
// POST /webhooks/{token}
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if payload == nil {
		payload = map[string]any{}
	}
	wf, err := s.triggers.FireWebhook(r.Context(), token, payload)
	if err != nil {
		http.Error(w, "invalid token or trigger disabled", http.StatusNotFound)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"workflow": wf})
}

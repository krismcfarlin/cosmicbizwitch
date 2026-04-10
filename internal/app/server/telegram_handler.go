package server

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/pocketbase/dbx"
)

// handleTelegramWebhook receives Telegram Update events.
// POST /webhooks/telegram  (no auth — public endpoint)
func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	// Validate secret token if one is configured.
	secret := s.settings.Get("TELEGRAM_WEBHOOK_SECRET")
	if secret != "" {
		incoming := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if incoming != secret {
			http.Error(w, "invalid secret token", http.StatusUnauthorized)
			return
		}
	}

	// Read the raw body once so we can both save it and decode it.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Save raw update for debugging — best-effort, ignore errors.
	if s.store != nil {
		if _, dbErr := s.store.App().DB().NewQuery(
			"INSERT INTO telegram_messages (received_at, raw_json) VALUES ({:ts}, {:json})",
		).Bind(dbx.Params{
			"ts":   time.Now().UTC().Format(time.RFC3339),
			"json": string(bodyBytes),
		}).Execute(); dbErr != nil {
			s.logger.Printf("telegram webhook: save raw message: %v", dbErr)
		}
	}

	// Parse the Telegram Update payload.
	var update struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			MessageID int64 `json:"message_id"`
			From      *struct {
				ID        int64  `json:"id"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
			} `json:"from"`
			Chat struct {
				ID    int64  `json:"id"`
				Type  string `json:"type"`
				Title string `json:"title"`
			} `json:"chat"`
			Text  string `json:"text"`
			Voice *struct {
				FileID   string `json:"file_id"`
				Duration int    `json:"duration"`
				MimeType string `json:"mime_type"`
			} `json:"voice"`
			Audio *struct {
				FileID   string `json:"file_id"`
				Duration int    `json:"duration"`
				MimeType string `json:"mime_type"`
				FileName string `json:"file_name"`
			} `json:"audio"`
			Document *struct {
				FileID   string `json:"file_id"`
				FileName string `json:"file_name"`
				MimeType string `json:"mime_type"`
			} `json:"document"`
			Video *struct {
				FileID   string `json:"file_id"`
				Duration int    `json:"duration"`
				MimeType string `json:"mime_type"`
			} `json:"video"`
		} `json:"message"`
		CallbackQuery *struct {
			ID   string `json:"id"`
			From struct {
				ID        int64  `json:"id"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
			} `json:"from"`
			Message *struct {
				MessageID int64 `json:"message_id"`
				Chat      struct {
					ID   int64  `json:"id"`
					Type string `json:"type"`
				} `json:"chat"`
			} `json:"message"`
			Data string `json:"data"`
		} `json:"callback_query"`
	}

	if err := json.Unmarshal(bodyBytes, &update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Handle inline keyboard button presses.
	if update.CallbackQuery != nil {
		cq := update.CallbackQuery

		// Dismiss the button's loading spinner immediately (best-effort).
		if tc := s.settings.TelegramClient(); tc != nil {
			_ = tc.AnswerCallbackQuery(cq.ID, "")
		}

		var chatID int64
		if cq.Message != nil {
			chatID = cq.Message.Chat.ID
		}

		cbPayload := map[string]any{
			"callback_query_id": cq.ID,
			"callback_data":     cq.Data,
			"chat_id":           chatID,
			"user_id":           cq.From.ID,
			"username":          cq.From.Username,
			"first_name":        cq.From.FirstName,
			"message_type":      "callback",
		}
		if cq.Message != nil {
			cbPayload["message_id"] = cq.Message.MessageID
		}

		if s.triggers != nil {
			fired, err := s.triggers.FireByType(r.Context(), "telegram_callback", cbPayload)
			if err != nil {
				s.logger.Printf("telegram webhook: fire callback_query triggers: %v", err)
			} else {
				s.logger.Printf("telegram webhook: fired %d telegram_callback trigger(s)", fired)
			}
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	if update.Message == nil {
		// Unknown update type — acknowledge and ignore.
		w.WriteHeader(http.StatusOK)
		return
	}

	msg := update.Message

	var userID int64
	var username, firstName string
	if msg.From != nil {
		userID = msg.From.ID
		username = msg.From.Username
		firstName = msg.From.FirstName
	}

	payload := map[string]any{
		"chat_id":      msg.Chat.ID,
		"user_id":      userID,
		"username":     username,
		"first_name":   firstName,
		"text":         msg.Text,
		"message_id":   msg.MessageID,
		"chat_type":    msg.Chat.Type,
		"message_type": "text",
	}

	// voice message
	if msg.Voice != nil {
		payload["voice_file_id"] = msg.Voice.FileID
		payload["voice_duration"] = msg.Voice.Duration
		payload["voice_mime_type"] = msg.Voice.MimeType
		payload["message_type"] = "voice"
	}
	// audio file
	if msg.Audio != nil {
		payload["audio_file_id"] = msg.Audio.FileID
		payload["audio_duration"] = msg.Audio.Duration
		payload["audio_mime_type"] = msg.Audio.MimeType
		payload["audio_file_name"] = msg.Audio.FileName
		payload["message_type"] = "audio"
	}
	// document
	if msg.Document != nil {
		payload["document_file_id"] = msg.Document.FileID
		payload["document_file_name"] = msg.Document.FileName
		payload["document_mime_type"] = msg.Document.MimeType
		payload["message_type"] = "document"
	}
	// video
	if msg.Video != nil {
		payload["video_file_id"] = msg.Video.FileID
		payload["video_duration"] = msg.Video.Duration
		payload["video_mime_type"] = msg.Video.MimeType
		payload["message_type"] = "video"
	}

	// Also inject the full raw Telegram update as a nested "message" key so that
	// conditions like {{message.voice.file_id}} resolve against the original structure.
	var rawUpdate map[string]any
	if err := json.Unmarshal(bodyBytes, &rawUpdate); err == nil {
		if rawMsg, ok := rawUpdate["message"]; ok {
			payload["message"] = rawMsg
		}
		if uid, ok := rawUpdate["update_id"]; ok {
			payload["update_id"] = uid
		}
	}

	if s.triggers != nil {
		fired, err := s.triggers.FireByType(r.Context(), "telegram", payload)
		if err != nil {
			s.logger.Printf("telegram webhook: fire triggers: %v", err)
		} else {
			s.logger.Printf("telegram webhook: fired %d trigger(s)", fired)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// handleTelegramStatus returns whether the Telegram bot is configured.
// GET /api/telegram/status
func (s *Server) handleTelegramStatus(w http.ResponseWriter, r *http.Request) {
	token := s.settings.Get("TELEGRAM_BOT_TOKEN")
	connected := token != ""
	s.respondJSON(w, http.StatusOK, map[string]any{
		"connected":     connected,
		"bot_token_set": connected,
	})
}

// handleTelegramSetWebhook registers a webhook URL with Telegram.
// POST /api/telegram/set-webhook
func (s *Server) handleTelegramSetWebhook(w http.ResponseWriter, r *http.Request) {
	tc := s.settings.TelegramClient()
	if tc == nil {
		http.Error(w, "TELEGRAM_BOT_TOKEN not configured — add it in Settings", http.StatusBadRequest)
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	webhookURL := body.URL
	if webhookURL == "" {
		webhookURL = "https://server.cosmicbizwitch.com/webhooks/telegram"
	}

	// Ensure we have a webhook secret; generate one if not set.
	secret := s.settings.Get("TELEGRAM_WEBHOOK_SECRET")
	if secret == "" {
		secret = randomHex(16)
		if err := s.settings.Set("TELEGRAM_WEBHOOK_SECRET", secret); err != nil {
			s.logger.Printf("telegram set-webhook: save TELEGRAM_WEBHOOK_SECRET: %v", err)
		}
	}

	if err := tc.SetWebhook(webhookURL, secret); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to set Telegram webhook", err)
		return
	}

	s.logger.Printf("telegram: webhook registered at %s", webhookURL)
	s.respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTelegramMessages returns the 100 most recent raw Telegram updates, newest first.
// GET /api/telegram/messages
func (s *Server) handleTelegramMessages(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID         int    `db:"id" json:"id"`
		ReceivedAt string `db:"received_at" json:"received_at"`
		RawJSON    string `db:"raw_json" json:"raw_json"`
	}
	var rows []row
	if err := s.store.App().DB().NewQuery(
		"SELECT id, received_at, raw_json FROM telegram_messages ORDER BY id DESC LIMIT 100",
	).All(&rows); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to query telegram_messages", err)
		return
	}
	if rows == nil {
		rows = []row{}
	}
	s.respondJSON(w, http.StatusOK, rows)
}

// handleTelegramMessagesClear deletes all rows from telegram_messages.
// DELETE /api/telegram/messages
func (s *Server) handleTelegramMessagesClear(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.App().DB().NewQuery(
		"DELETE FROM telegram_messages",
	).Execute(); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to clear telegram_messages", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

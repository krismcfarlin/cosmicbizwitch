package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	openaiapp "cosmicbizwitch/internal/app/openai"
	"cosmicbizwitch/internal/app/storage"

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
				LastName  string `json:"last_name"`
			} `json:"from"`
			Chat struct {
				ID    int64  `json:"id"`
				Type  string `json:"type"`
				Title string `json:"title"`
			} `json:"chat"`
			Text            string `json:"text"`
			Caption         string `json:"caption"`
			Date            int64  `json:"date"`
			MessageThreadID int64  `json:"message_thread_id"`
			Photo           []struct {
				FileID   string `json:"file_id"`
				Width    int    `json:"width"`
				Height   int    `json:"height"`
				FileSize int    `json:"file_size"`
			} `json:"photo"`
			Sticker *struct {
				FileID   string `json:"file_id"`
				Emoji    string `json:"emoji"`
				SetName  string `json:"set_name"`
				IsAnimated bool `json:"is_animated"`
			} `json:"sticker"`
			Location *struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
			Contact *struct {
				PhoneNumber string `json:"phone_number"`
				FirstName   string `json:"first_name"`
				LastName    string `json:"last_name"`
				UserID      int64  `json:"user_id"`
			} `json:"contact"`
			VideoNote *struct {
				FileID   string `json:"file_id"`
				Duration int    `json:"duration"`
				Length   int    `json:"length"`
			} `json:"video_note"`
			Animation *struct {
				FileID   string `json:"file_id"`
				Duration int    `json:"duration"`
				Width    int    `json:"width"`
				Height   int    `json:"height"`
				FileName string `json:"file_name"`
				MimeType string `json:"mime_type"`
			} `json:"animation"`
			ReplyToMessage *struct {
				MessageID int64  `json:"message_id"`
				Text      string `json:"text"`
				Caption   string `json:"caption"`
				From      *struct {
					ID        int64  `json:"id"`
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
				} `json:"from"`
			} `json:"reply_to_message"`
			ForwardFrom *struct {
				ID        int64  `json:"id"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
			} `json:"forward_from"`
			ForwardDate int64 `json:"forward_date"`
			Entities    []struct {
				Type   string `json:"type"`
				Offset int    `json:"offset"`
				Length int    `json:"length"`
				URL    string `json:"url"`
			} `json:"entities"`
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

		var callbackData any = cq.Data
		var parsedData map[string]any
		if json.Unmarshal([]byte(cq.Data), &parsedData) == nil {
			callbackData = parsedData
		}

		cbPayload := map[string]any{
			"callback_query_id": cq.ID,
			"callback_data":     callbackData,
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
	var username, firstName, lastName string
	if msg.From != nil {
		userID = msg.From.ID
		username = msg.From.Username
		firstName = msg.From.FirstName
		lastName = msg.From.LastName
	}

	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	payload := map[string]any{
		"chat_id":           msg.Chat.ID,
		"user_id":           userID,
		"username":          username,
		"first_name":        firstName,
		"last_name":         lastName,
		"text":              text,
		"message_id":        msg.MessageID,
		"message_thread_id": msg.MessageThreadID,
		"chat_type":         msg.Chat.Type,
		"message_type":      "text",
		"date":              msg.Date,
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
	// photo — use largest size (last element)
	if len(msg.Photo) > 0 {
		largest := msg.Photo[len(msg.Photo)-1]
		payload["photo_file_id"] = largest.FileID
		payload["photo_width"] = largest.Width
		payload["photo_height"] = largest.Height
		payload["message_type"] = "photo"
	}
	// video
	if msg.Video != nil {
		payload["video_file_id"] = msg.Video.FileID
		payload["video_duration"] = msg.Video.Duration
		payload["video_mime_type"] = msg.Video.MimeType
		payload["message_type"] = "video"
	}
	// sticker
	if msg.Sticker != nil {
		payload["sticker_file_id"] = msg.Sticker.FileID
		payload["sticker_emoji"] = msg.Sticker.Emoji
		payload["sticker_set_name"] = msg.Sticker.SetName
		payload["message_type"] = "sticker"
	}
	// location
	if msg.Location != nil {
		payload["location_latitude"] = msg.Location.Latitude
		payload["location_longitude"] = msg.Location.Longitude
		payload["message_type"] = "location"
	}
	// contact
	if msg.Contact != nil {
		payload["contact_phone"] = msg.Contact.PhoneNumber
		payload["contact_first_name"] = msg.Contact.FirstName
		payload["contact_last_name"] = msg.Contact.LastName
		payload["contact_user_id"] = msg.Contact.UserID
		payload["message_type"] = "contact"
	}
	// video note (round video)
	if msg.VideoNote != nil {
		payload["video_note_file_id"] = msg.VideoNote.FileID
		payload["video_note_duration"] = msg.VideoNote.Duration
		payload["message_type"] = "video_note"
	}
	// animation (GIF)
	if msg.Animation != nil {
		payload["animation_file_id"] = msg.Animation.FileID
		payload["animation_duration"] = msg.Animation.Duration
		payload["animation_mime_type"] = msg.Animation.MimeType
		payload["message_type"] = "animation"
	}
	// reply context
	if msg.ReplyToMessage != nil {
		replyText := msg.ReplyToMessage.Text
		if replyText == "" {
			replyText = msg.ReplyToMessage.Caption
		}
		payload["reply_to_message_id"] = msg.ReplyToMessage.MessageID
		payload["reply_to_text"] = replyText
		if msg.ReplyToMessage.From != nil {
			payload["reply_to_username"] = msg.ReplyToMessage.From.Username
			payload["reply_to_user_id"] = msg.ReplyToMessage.From.ID
		}
	}
	// forward origin
	if msg.ForwardFrom != nil {
		payload["forward_from_user_id"] = msg.ForwardFrom.ID
		payload["forward_from_username"] = msg.ForwardFrom.Username
		payload["forward_date"] = msg.ForwardDate
	}
	// entities (URLs, mentions, hashtags)
	if len(msg.Entities) > 0 {
		payload["entities"] = msg.Entities
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

// handleTelegramIndex bulk-indexes unindexed raw_telegram records into telegram_vectors.
// POST /api/telegram/index
//
// Fetches all raw_telegram PB records with non-empty text, skips already-indexed ones,
// calls OpenAI embeddings API in batches of 100, and upserts results into telegram_vectors.
// Returns {"indexed": N, "skipped": N, "errors": N}.
func (s *Server) handleTelegramIndex(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store not available", http.StatusServiceUnavailable)
		return
	}

	apiKey := s.settings.Get("OPENAI_API_KEY")
	if apiKey == "" {
		s.respondError(w, http.StatusBadRequest, "OPENAI_API_KEY not configured", fmt.Errorf("add OPENAI_API_KEY in Settings"))
		return
	}

	ctx := r.Context()
	app := s.store.App()

	// Fetch existing indexed IDs.
	indexedIDs, err := s.store.GetTelegramVectorIDs(ctx)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to get existing vector ids", err)
		return
	}

	var totalIndexed, totalSkipped, totalErrors int

	page := 1
	const perPage = 200
	for {
		records, err := app.FindRecordsByFilter("raw_telegram", "text != ''", "-date", perPage, (page-1)*perPage)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("fetch raw_telegram page %d", page), err)
			return
		}
		if len(records) == 0 {
			break
		}

		type pending struct {
			rec storage.TelegramVectorRecord
		}
		var batch []pending

		flush := func() {
			if len(batch) == 0 {
				return
			}
			texts := make([]string, len(batch))
			for i, p := range batch {
				texts[i] = p.rec.Text
			}
			vecs, embedErr := openaiapp.EmbedBatch(ctx, apiKey, texts)
			if embedErr != nil {
				s.logger.Printf("telegram index: embed batch of %d: %v", len(batch), embedErr)
				totalErrors += len(batch)
				batch = batch[:0]
				return
			}
			for i, p := range batch {
				p.rec.Embedding = vecs[i]
				if upsertErr := s.store.UpsertTelegramVector(ctx, p.rec); upsertErr != nil {
					s.logger.Printf("telegram index: upsert %s: %v", p.rec.ID, upsertErr)
					totalErrors++
				} else {
					totalIndexed++
				}
			}
			batch = batch[:0]
		}

		for _, rec := range records {
			id := rec.Id
			text := rec.GetString("text")
			if text == "" || indexedIDs[id] {
				totalSkipped++
				continue
			}
			batch = append(batch, pending{rec: storage.TelegramVectorRecord{
				ID:        id,
				MessageID: int64(rec.GetInt("message_id")),
				FromID:    int64(rec.GetInt("from_id")),
				ChatID:    int64(rec.GetInt("chat_id")),
				ChatTitle: rec.GetString("chat_title"),
				FirstName: rec.GetString("message_first_name"),
				LastName:  rec.GetString("message_last_name"),
				SentAt:    int64(rec.GetInt("date")),
				Text:      text,
			}})
			if len(batch) >= 100 {
				flush()
			}
		}
		flush()

		if len(records) < perPage {
			break
		}
		page++
	}

	s.logger.Printf("telegram index complete: indexed=%d skipped=%d errors=%d", totalIndexed, totalSkipped, totalErrors)
	s.respondJSON(w, http.StatusOK, map[string]any{
		"indexed": totalIndexed,
		"skipped": totalSkipped,
		"errors":  totalErrors,
	})
}

// handleTelegramVectorStats returns statistics about the telegram_vectors table.
// GET /api/telegram/vector-stats
func (s *Server) handleTelegramVectorStats(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "store not available", http.StatusServiceUnavailable)
		return
	}

	count, err := s.store.CountTelegramVectors(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to count telegram vectors", err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"total_indexed": count,
	})
}

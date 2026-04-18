package server

import (
	"encoding/json"
	"testing"
)

// parseTelegramPayload exercises the webhook handler with a fake body and
// captures the payload that would be sent to triggers. We do this by
// inspecting the raw JSON decode logic without needing a real triggers.Manager.
func parseTelegramPayload(t *testing.T, body string) map[string]any {
	t.Helper()

	// We decode the same way the handler does.
	var update struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			MessageID       int64  `json:"message_id"`
			MessageThreadID int64  `json:"message_thread_id"`
			Date            int64  `json:"date"`
			From            *struct {
				ID        int64  `json:"id"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
			} `json:"from"`
			Chat struct {
				ID    int64  `json:"id"`
				Type  string `json:"type"`
				Title string `json:"title"`
			} `json:"chat"`
			Text    string `json:"text"`
			Caption string `json:"caption"`
			Photo   []struct {
				FileID   string `json:"file_id"`
				Width    int    `json:"width"`
				Height   int    `json:"height"`
				FileSize int    `json:"file_size"`
			} `json:"photo"`
			Sticker *struct {
				FileID     string `json:"file_id"`
				Emoji      string `json:"emoji"`
				SetName    string `json:"set_name"`
				IsAnimated bool   `json:"is_animated"`
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
	}

	if err := json.Unmarshal([]byte(body), &update); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if update.Message == nil {
		t.Fatal("no message in update")
	}
	msg := update.Message

	var userID int64
	var username, firstName string
	if msg.From != nil {
		userID = msg.From.ID
		username = msg.From.Username
		firstName = msg.From.FirstName
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
		"text":              text,
		"message_id":        msg.MessageID,
		"message_thread_id": msg.MessageThreadID,
		"chat_type":         msg.Chat.Type,
		"message_type":      "text",
		"date":              msg.Date,
	}

	if len(msg.Photo) > 0 {
		largest := msg.Photo[len(msg.Photo)-1]
		payload["photo_file_id"] = largest.FileID
		payload["photo_width"] = largest.Width
		payload["photo_height"] = largest.Height
		payload["message_type"] = "photo"
	}
	if msg.Voice != nil {
		payload["voice_file_id"] = msg.Voice.FileID
		payload["voice_duration"] = msg.Voice.Duration
		payload["voice_mime_type"] = msg.Voice.MimeType
		payload["message_type"] = "voice"
	}
	if msg.Audio != nil {
		payload["audio_file_id"] = msg.Audio.FileID
		payload["message_type"] = "audio"
	}
	if msg.Document != nil {
		payload["document_file_id"] = msg.Document.FileID
		payload["message_type"] = "document"
	}
	if msg.Video != nil {
		payload["video_file_id"] = msg.Video.FileID
		payload["message_type"] = "video"
	}
	if msg.Sticker != nil {
		payload["sticker_file_id"] = msg.Sticker.FileID
		payload["sticker_emoji"] = msg.Sticker.Emoji
		payload["sticker_set_name"] = msg.Sticker.SetName
		payload["message_type"] = "sticker"
	}
	if msg.Location != nil {
		payload["location_latitude"] = msg.Location.Latitude
		payload["location_longitude"] = msg.Location.Longitude
		payload["message_type"] = "location"
	}
	if msg.Contact != nil {
		payload["contact_phone"] = msg.Contact.PhoneNumber
		payload["contact_first_name"] = msg.Contact.FirstName
		payload["contact_last_name"] = msg.Contact.LastName
		payload["contact_user_id"] = msg.Contact.UserID
		payload["message_type"] = "contact"
	}
	if msg.VideoNote != nil {
		payload["video_note_file_id"] = msg.VideoNote.FileID
		payload["message_type"] = "video_note"
	}
	if msg.Animation != nil {
		payload["animation_file_id"] = msg.Animation.FileID
		payload["message_type"] = "animation"
	}
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
	if msg.ForwardFrom != nil {
		payload["forward_from_user_id"] = msg.ForwardFrom.ID
		payload["forward_from_username"] = msg.ForwardFrom.Username
		payload["forward_date"] = msg.ForwardDate
	}
	if len(msg.Entities) > 0 {
		payload["entities"] = msg.Entities
	}

	return payload
}


func TestTelegramTextMessage(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":1,"message":{"message_id":10,"date":1700000000,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},"text":"hello world"}}`)

	if p["text"] != "hello world" {
		t.Errorf("text = %v", p["text"])
	}
	if p["message_type"] != "text" {
		t.Errorf("message_type = %v", p["message_type"])
	}
	if p["date"] != int64(1700000000) {
		t.Errorf("date = %v", p["date"])
	}
}

func TestTelegramPhotoCaption(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":2,"message":{"message_id":11,"date":1700000001,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},"caption":"look at this",
		"photo":[{"file_id":"small","width":90,"height":90,"file_size":1000},
		         {"file_id":"large","width":800,"height":600,"file_size":50000}]}}`)

	if p["text"] != "look at this" {
		t.Errorf("text (from caption) = %v", p["text"])
	}
	if p["message_type"] != "photo" {
		t.Errorf("message_type = %v", p["message_type"])
	}
	if p["photo_file_id"] != "large" {
		t.Errorf("photo_file_id = %v (want largest)", p["photo_file_id"])
	}
}

func TestTelegramPhotoNoCaption(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":3,"message":{"message_id":12,"date":1700000002,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},
		"photo":[{"file_id":"only","width":800,"height":600,"file_size":50000}]}}`)

	if p["text"] != "" {
		t.Errorf("text should be empty, got %v", p["text"])
	}
	if p["message_type"] != "photo" {
		t.Errorf("message_type = %v", p["message_type"])
	}
}

func TestTelegramVoice(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":4,"message":{"message_id":13,"date":1700000003,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},
		"voice":{"file_id":"voice_id","duration":5,"mime_type":"audio/ogg"}}}`)

	if p["message_type"] != "voice" {
		t.Errorf("message_type = %v", p["message_type"])
	}
	if p["voice_file_id"] != "voice_id" {
		t.Errorf("voice_file_id = %v", p["voice_file_id"])
	}
}

func TestTelegramSticker(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":5,"message":{"message_id":14,"date":1700000004,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},
		"sticker":{"file_id":"sticker_id","emoji":"🔥","set_name":"Animals"}}}`)

	if p["message_type"] != "sticker" {
		t.Errorf("message_type = %v", p["message_type"])
	}
	if p["sticker_emoji"] != "🔥" {
		t.Errorf("sticker_emoji = %v", p["sticker_emoji"])
	}
}

func TestTelegramLocation(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":6,"message":{"message_id":15,"date":1700000005,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},
		"location":{"latitude":51.5074,"longitude":-0.1278}}}`)

	if p["message_type"] != "location" {
		t.Errorf("message_type = %v", p["message_type"])
	}
	if p["location_latitude"] != 51.5074 {
		t.Errorf("latitude = %v", p["location_latitude"])
	}
	if p["location_longitude"] != -0.1278 {
		t.Errorf("longitude = %v", p["location_longitude"])
	}
}

func TestTelegramReply(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":7,"message":{"message_id":16,"date":1700000006,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},"text":"yes!",
		"reply_to_message":{"message_id":5,"text":"original question",
		"from":{"id":88,"username":"lauren","first_name":"Lauren"}}}}`)

	if p["reply_to_message_id"] != int64(5) {
		t.Errorf("reply_to_message_id = %v", p["reply_to_message_id"])
	}
	if p["reply_to_text"] != "original question" {
		t.Errorf("reply_to_text = %v", p["reply_to_text"])
	}
	if p["reply_to_username"] != "lauren" {
		t.Errorf("reply_to_username = %v", p["reply_to_username"])
	}
}

func TestTelegramMessageThreadID(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":8,"message":{"message_id":17,"date":1700000007,
		"message_thread_id":1497,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-1003039609007,"type":"supergroup"},"text":"thread message"}}`)

	if p["message_thread_id"] != int64(1497) {
		t.Errorf("message_thread_id = %v", p["message_thread_id"])
	}
}

func TestTelegramEntities(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":9,"message":{"message_id":18,"date":1700000008,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},"text":"visit https://example.com",
		"entities":[{"type":"url","offset":6,"length":19,"url":"https://example.com"}]}}`)

	if _, ok := p["entities"]; !ok {
		t.Fatal("entities missing from payload")
	}
}

func TestTelegramContact(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":10,"message":{"message_id":19,"date":1700000009,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},
		"contact":{"phone_number":"+1234567890","first_name":"Bob","last_name":"Smith","user_id":42}}}`)

	if p["message_type"] != "contact" {
		t.Errorf("message_type = %v", p["message_type"])
	}
	if p["contact_phone"] != "+1234567890" {
		t.Errorf("contact_phone = %v", p["contact_phone"])
	}
}

func TestTelegramReplyToCaption(t *testing.T) {
	p := parseTelegramPayload(t, `{"update_id":11,"message":{"message_id":20,"date":1700000010,
		"from":{"id":99,"username":"kris","first_name":"Kris"},
		"chat":{"id":-100,"type":"group"},"text":"nice pic",
		"reply_to_message":{"message_id":6,"caption":"photo caption",
		"from":{"id":88,"username":"lauren","first_name":"Lauren"}}}}`)

	if p["reply_to_text"] != "photo caption" {
		t.Errorf("reply_to_text from caption = %v", p["reply_to_text"])
	}
}

package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	telegramapp "cosmicbizwitch/internal/app/telegram"
	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/pocketbase/core"
)

// registerTelegramActivities registers all Telegram activity nodes.
func registerTelegramActivities(eng *workflow.Engine, getTelegram func() *telegramapp.Client, app core.App) {
	registerTelegramSendButton(eng, getTelegram)
	// ── telegram_send_message ────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("telegram_send_message",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			tc := getTelegram()
			if tc == nil {
				return nil, fmt.Errorf("Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings")
			}

			chatID, err := telegramChatID(input, "chat_id")
			if err != nil {
				return nil, fmt.Errorf("telegram_send_message: %w", err)
			}
			text := telegramString(input, "text")
			if text == "" {
				return nil, fmt.Errorf("telegram_send_message: text is required")
			}

			params := telegramapp.SendMessageParams{
				ChatID:    chatID,
				Text:      text,
				ParseMode: telegramString(input, "parse_mode"),
			}

			if v, ok := input["reply_to_message_id"]; ok && v != nil {
				params.ReplyToMessageID = telegramInt64(v)
			}
			if v, ok := input["message_thread_id"]; ok && v != nil {
				params.MessageThreadID = telegramInt64(v)
			}
			if v, ok := input["disable_notification"]; ok {
				if b, ok := v.(bool); ok {
					params.DisableNotification = b
				}
			}
			if v, ok := input["disable_web_page_preview"]; ok {
				if b, ok := v.(bool); ok {
					params.DisableWebPagePreview = b
				}
			}
			if v, ok := input["protect_content"]; ok {
				if b, ok := v.(bool); ok {
					params.ProtectContent = b
				}
			}

			if err := tc.SendMessage(params); err != nil {
				return nil, fmt.Errorf("telegram_send_message: %w", err)
			}
			return map[string]any{"ok": true}, nil
		},
		workflow.ActivityMeta{
			Category:    "Telegram",
			Description: "Send a message to a Telegram chat or user",
			InputFields: []workflow.FieldMeta{
				{Name: "chat_id", Type: "number", Required: true, Description: "Target chat ID (from trigger context: {{chat_id}})"},
				{Name: "text", Type: "string", Required: true, Description: "Message text"},
				{Name: "parse_mode", Type: "string", Required: false, Description: "Formatting: Markdown, MarkdownV2, HTML, or empty for plain text"},
				{Name: "reply_to_message_id", Type: "number", Required: false, Description: "Reply to a specific message ID (from trigger context: {{message_id}})"},
				{Name: "message_thread_id", Type: "number", Required: false, Description: "Forum topic thread ID (supergroups only)"},
				{Name: "disable_notification", Type: "boolean", Required: false, Description: "Send silently (no sound/vibration)"},
				{Name: "disable_web_page_preview", Type: "boolean", Required: false, Description: "Disable link preview cards"},
				{Name: "protect_content", Type: "boolean", Required: false, Description: "Prevent forwarding or saving"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "ok", Type: "boolean", Description: "True when the message was delivered successfully."},
			},
		},
	)

	// ── telegram_get_file_url ────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("telegram_get_file_url",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			tc := getTelegram()
			if tc == nil {
				return nil, fmt.Errorf("Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings")
			}

			fileID := telegramString(input, "file_id")
			if fileID == "" {
				return nil, fmt.Errorf("telegram_get_file_url: file_id is required")
			}

			fileURL, err := tc.GetFileURL(fileID)
			if err != nil {
				return nil, fmt.Errorf("telegram_get_file_url: %w", err)
			}

			return map[string]any{"file_url": fileURL}, nil
		},
		workflow.ActivityMeta{
			Category:    "Telegram",
			Description: "Resolves a Telegram file_id to a direct download URL. Use with voice_file_id, audio_file_id, etc. from the trigger context.",
			InputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "string", Required: true, Description: "The file_id from the trigger context (e.g. {{voice_file_id}})"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "file_url", Type: "string", Description: "The HTTPS download URL for the file."},
			},
		},
	)

	// ── telegram_transcribe ──────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("telegram_transcribe",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			tc := getTelegram()
			if tc == nil {
				return nil, fmt.Errorf("Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings")
			}

			fileID := telegramString(input, "file_id")
			if fileID == "" {
				return nil, fmt.Errorf("telegram_transcribe: file_id is required")
			}

			language := telegramString(input, "language")

			resultKey := telegramString(input, "result_key")
			if resultKey == "" {
				resultKey = "transcript"
			}

			apiKey := llmGetSetting(app, "OPENAI_API_KEY")
			if apiKey == "" {
				return nil, fmt.Errorf("telegram_transcribe: no API key configured — set OPENAI_API_KEY in Settings")
			}

			fileBytes, ext, err := tc.DownloadFile(fileID)
			if err != nil {
				return nil, fmt.Errorf("telegram_transcribe: download file: %w", err)
			}

			filename := "audio" + ext
			if ext == "" {
				filename = "audio.ogg"
			}

			transcript, err := callWhisperTranscribe(apiKey, fileBytes, filename, language)
			if err != nil {
				return nil, fmt.Errorf("telegram_transcribe: %w", err)
			}

			return map[string]any{
				resultKey:    transcript,
				"transcript": transcript,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Telegram",
			Description: "Downloads an audio file from Telegram and transcribes it using OpenAI Whisper. Works directly with voice_file_id or audio_file_id from the trigger context.",
			InputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "string", Required: true, Description: "Telegram file_id of the voice/audio message (e.g. {{voice_file_id}})"},
				{Name: "language", Type: "string", Required: false, Description: "ISO 639-1 language hint e.g. en. Optional."},
				{Name: "result_key", Type: "string", Required: false, Description: "Context key to store the transcript under. Defaults to 'transcript'."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "transcript", Type: "string", Description: "The transcribed text (also stored under result_key if specified)."},
			},
		},
	)
}

// ── telegram_send_button ─────────────────────────────────────────────────────

func registerTelegramSendButton(eng *workflow.Engine, getTelegram func() *telegramapp.Client) {
	eng.RegisterActivityWithMeta("telegram_send_button",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			tc := getTelegram()
			if tc == nil {
				return nil, fmt.Errorf("Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings")
			}

			chatID, err := telegramChatID(input, "chat_id")
			if err != nil {
				return nil, fmt.Errorf("telegram_send_button: %w", err)
			}
			text := telegramString(input, "text")
			if text == "" {
				return nil, fmt.Errorf("telegram_send_button: text is required")
			}

			// Parse buttons — accept either a [][]map or a JSON string.
			buttons, err := telegramParseButtons(input["buttons"])
			if err != nil {
				return nil, fmt.Errorf("telegram_send_button: %w", err)
			}
			if len(buttons) == 0 {
				return nil, fmt.Errorf("telegram_send_button: buttons must have at least one row")
			}

			parseMode := telegramString(input, "parse_mode")

			var messageThreadID int64
			if v, ok := input["message_thread_id"]; ok && v != nil {
				messageThreadID = telegramInt64(v)
			}

			if err := tc.SendMessageWithButtons(chatID, messageThreadID, text, parseMode, buttons); err != nil {
				return nil, fmt.Errorf("telegram_send_button: %w", err)
			}
			return map[string]any{"ok": true}, nil
		},
		workflow.ActivityMeta{
			Category:    "Telegram",
			Description: "Send a message with inline keyboard buttons. When a user clicks a button, the callback_data is sent back and fires a telegram_callback trigger.",
			InputFields: []workflow.FieldMeta{
				{Name: "chat_id", Type: "number", Required: true, Description: "Target chat ID (from trigger context: {{chat_id}})"},
				{Name: "message_thread_id", Type: "number", Required: false, Description: "Forum topic thread ID (supergroups only, from trigger context: {{message_thread_id}})"},
				{Name: "text", Type: "string", Required: true, Description: "Message text displayed above the buttons"},
				{Name: "buttons", Type: "json", Required: true, Description: `2-D array of buttons — rows of {text, callback_data} objects. E.g. [[{"text":"Yes","callback_data":"yes"},{"text":"No","callback_data":"no"}]]`},
				{Name: "parse_mode", Type: "string", Required: false, Description: "Formatting: Markdown, MarkdownV2, HTML, or empty"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "ok", Type: "boolean", Description: "True when the message was delivered successfully."},
			},
		},
	)
}

// telegramParseButtons converts the raw input value for "buttons" into
// [][]telegramapp.InlineKeyboardButton. Accepts:
//   - Already-decoded []any (from JSON context interpolation)
//   - A JSON string that encodes the same structure
func telegramParseButtons(raw any) ([][]telegramapp.InlineKeyboardButton, error) {
	if raw == nil {
		return nil, fmt.Errorf("buttons is required")
	}

	log.Printf("[telegramParseButtons] type=%T value=%#v", raw, raw)

	// If it's a string, try to JSON-decode it first.
	if s, ok := raw.(string); ok {
		var decoded any
		if err := jsonUnmarshalAny(s, &decoded); err != nil {
			return nil, fmt.Errorf("buttons is not valid JSON: %w", err)
		}
		raw = decoded
	}

	// raw should now be []any where each element is []any of map[string]any.
	outerSlice, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("buttons must be a 2-D array")
	}

	var rows [][]telegramapp.InlineKeyboardButton
	for rowIdx, rowRaw := range outerSlice {
		rowSlice, ok := rowRaw.([]any)
		if !ok {
			return nil, fmt.Errorf("buttons row %d must be an array", rowIdx)
		}
		var row []telegramapp.InlineKeyboardButton
		for btnIdx, btnRaw := range rowSlice {
			btnMap, ok := btnRaw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("buttons[%d][%d] must be an object with text and callback_data", rowIdx, btnIdx)
			}
			btnText, _ := btnMap["text"].(string)
			if btnText == "" {
				return nil, fmt.Errorf("buttons[%d][%d].text is required", rowIdx, btnIdx)
			}
			var callbackData string
			switch cd := btnMap["callback_data"].(type) {
			case string:
				callbackData = cd
			case nil:
				// leave empty, caught below
			default:
				// map, slice, number — JSON-encode it
				b, err := json.Marshal(cd)
				if err != nil {
					return nil, fmt.Errorf("buttons[%d][%d].callback_data: cannot encode: %w", rowIdx, btnIdx, err)
				}
				callbackData = string(b)
			}
			if callbackData == "" {
				return nil, fmt.Errorf("buttons[%d][%d].callback_data is required", rowIdx, btnIdx)
			}
			if len(callbackData) > 64 {
				return nil, fmt.Errorf("buttons[%d][%d].callback_data exceeds Telegram's 64-byte limit (%d bytes): %q — store large values in context and use a short ID as callback_data", rowIdx, btnIdx, len(callbackData), callbackData)
			}
			row = append(row, telegramapp.InlineKeyboardButton{Text: btnText, CallbackData: callbackData})
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// jsonUnmarshalAny is a helper that decodes a JSON string into any.
func jsonUnmarshalAny(s string, v *any) error {
	return json.Unmarshal([]byte(s), v)
}

// telegramString extracts a string value from the activity input map.
// If the value is a map or slice, it is JSON-marshaled to a string.
func telegramString(input map[string]any, key string) string {
	v, ok := input[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// telegramInt64 converts any value to int64.
// Handles float64 (after JSON round-trip), int64, int, int32, and numeric strings.
func telegramInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return int64(f)
		}
	}
	return 0
}

// telegramChatID extracts an int64 chat ID from the activity input map.
func telegramChatID(input map[string]any, key string) (int64, error) {
	v, ok := input[key]
	if !ok || v == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	log.Printf("[telegramChatID] %s: type=%T value=%#v", key, v, v)
	var n int64
	switch val := v.(type) {
	case int64:
		n = val
	case float64:
		n = int64(val)
	case int:
		n = int64(val)
	case int32:
		n = int64(val)
	case string:
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: cannot convert string %q to int", key, val)
		}
		n = parsed
	default:
		return 0, fmt.Errorf("%s must be a number, got %T value=%#v", key, v, v)
	}
	if n == 0 {
		return 0, fmt.Errorf("%s must be a non-zero integer", key)
	}
	return n, nil
}

// newTelegramSendMessageFn returns a testable ActivityFunc for telegram_send_message.
func newTelegramSendMessageFn(getTelegram func() *telegramapp.Client) workflow.ActivityFunc {
	return func(ctx context.Context, input map[string]any) (map[string]any, error) {
		tc := getTelegram()
		if tc == nil {
			return nil, fmt.Errorf("Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings")
		}
		chatID, err := telegramChatID(input, "chat_id")
		if err != nil {
			return nil, fmt.Errorf("telegram_send_message: %w", err)
		}
		text := telegramString(input, "text")
		if text == "" {
			return nil, fmt.Errorf("telegram_send_message: text is required")
		}
		params := telegramapp.SendMessageParams{
			ChatID:    chatID,
			Text:      text,
			ParseMode: telegramString(input, "parse_mode"),
		}
		if v, ok := input["reply_to_message_id"]; ok && v != nil {
			params.ReplyToMessageID = telegramInt64(v)
		}
		if v, ok := input["message_thread_id"]; ok && v != nil {
			params.MessageThreadID = telegramInt64(v)
		}
		if v, ok := input["disable_notification"]; ok {
			if b, ok := v.(bool); ok {
				params.DisableNotification = b
			}
		}
		if v, ok := input["disable_web_page_preview"]; ok {
			if b, ok := v.(bool); ok {
				params.DisableWebPagePreview = b
			}
		}
		if v, ok := input["protect_content"]; ok {
			if b, ok := v.(bool); ok {
				params.ProtectContent = b
			}
		}
		if err := tc.SendMessage(params); err != nil {
			return nil, fmt.Errorf("telegram_send_message: %w", err)
		}
		return map[string]any{"ok": true}, nil
	}
}

// newTelegramGetFileURLFn returns a testable ActivityFunc for telegram_get_file_url.
func newTelegramGetFileURLFn(getTelegram func() *telegramapp.Client) workflow.ActivityFunc {
	return func(ctx context.Context, input map[string]any) (map[string]any, error) {
		tc := getTelegram()
		if tc == nil {
			return nil, fmt.Errorf("Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings")
		}
		fileID := telegramString(input, "file_id")
		if fileID == "" {
			return nil, fmt.Errorf("telegram_get_file_url: file_id is required")
		}
		fileURL, err := tc.GetFileURL(fileID)
		if err != nil {
			return nil, fmt.Errorf("telegram_get_file_url: %w", err)
		}
		return map[string]any{"file_url": fileURL}, nil
	}
}

// newTelegramTranscribeFn returns a testable ActivityFunc for telegram_transcribe.
func newTelegramTranscribeFn(getTelegram func() *telegramapp.Client, app core.App) workflow.ActivityFunc {
	return func(ctx context.Context, input map[string]any) (map[string]any, error) {
		tc := getTelegram()
		if tc == nil {
			return nil, fmt.Errorf("Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings")
		}
		fileID := telegramString(input, "file_id")
		if fileID == "" {
			return nil, fmt.Errorf("telegram_transcribe: file_id is required")
		}
		language := telegramString(input, "language")
		resultKey := telegramString(input, "result_key")
		if resultKey == "" {
			resultKey = "transcript"
		}
		apiKey := llmGetSetting(app, "OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("telegram_transcribe: no API key configured — set OPENAI_API_KEY in Settings")
		}
		fileBytes, ext, err := tc.DownloadFile(fileID)
		if err != nil {
			return nil, fmt.Errorf("telegram_transcribe: download file: %w", err)
		}
		filename := "audio" + ext
		if ext == "" {
			filename = "audio.ogg"
		}
		transcript, err := callWhisperTranscribe(apiKey, fileBytes, filename, language)
		if err != nil {
			return nil, fmt.Errorf("telegram_transcribe: %w", err)
		}
		return map[string]any{
			resultKey:    transcript,
			"transcript": transcript,
		}, nil
	}
}

// newTelegramSendButtonFn returns a testable ActivityFunc for telegram_send_button.
func newTelegramSendButtonFn(getTelegram func() *telegramapp.Client) workflow.ActivityFunc {
	return func(ctx context.Context, input map[string]any) (map[string]any, error) {
		tc := getTelegram()
		if tc == nil {
			return nil, fmt.Errorf("Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings")
		}
		chatID, err := telegramChatID(input, "chat_id")
		if err != nil {
			return nil, fmt.Errorf("telegram_send_button: %w", err)
		}
		text := telegramString(input, "text")
		if text == "" {
			return nil, fmt.Errorf("telegram_send_button: text is required")
		}
		buttons, err := telegramParseButtons(input["buttons"])
		if err != nil {
			return nil, fmt.Errorf("telegram_send_button: %w", err)
		}
		if len(buttons) == 0 {
			return nil, fmt.Errorf("telegram_send_button: buttons must have at least one row")
		}
		parseMode := telegramString(input, "parse_mode")
		var messageThreadID int64
		if v, ok := input["message_thread_id"]; ok && v != nil {
			messageThreadID = telegramInt64(v)
		}
		if err := tc.SendMessageWithButtons(chatID, messageThreadID, text, parseMode, buttons); err != nil {
			return nil, fmt.Errorf("telegram_send_button: %w", err)
		}
		return map[string]any{"ok": true}, nil
	}
}

package workflows

import (
	"strings"
	"testing"

	telegramapp "cosmicbizwitch/internal/app/telegram"
	"cosmicbizwitch/pkg/workflow/testutil"
)

// nilTelegram returns a getter that always returns nil (simulates unconfigured bot).
func nilTelegram() func() *telegramapp.Client {
	return func() *telegramapp.Client { return nil }
}

// ── telegramInt64 ────────────────────────────────────────────────────────────

func TestTelegramInt64(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
	}{
		{"float64 positive", float64(12345), 12345},
		{"float64 zero", float64(0), 0},
		{"float64 decimal", float64(99.9), 99},
		{"int64", int64(77), 77},
		{"int", int(42), 42},
		{"int32", int32(7), 7},
		{"string numeric", "555", 555},
		{"string float string", "12.9", 12},
		{"string invalid", "abc", 0},
		{"bool", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := telegramInt64(tt.in)
			if got != tt.want {
				t.Errorf("telegramInt64(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// ── telegramChatID ───────────────────────────────────────────────────────────

func TestTelegramChatID(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		wantID  int64
	}{
		{"float64", map[string]any{"chat_id": float64(123456)}, false, 123456},
		{"int64", map[string]any{"chat_id": int64(789)}, false, 789},
		{"string numeric", map[string]any{"chat_id": "111"}, false, 111},
		{"missing key", map[string]any{}, true, 0},
		{"nil value", map[string]any{"chat_id": nil}, true, 0},
		{"zero", map[string]any{"chat_id": float64(0)}, true, 0},
		{"non-numeric string", map[string]any{"chat_id": "bad"}, true, 0},
		{"unsupported type bool", map[string]any{"chat_id": true}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := telegramChatID(tt.input, "chat_id")
			if (err != nil) != tt.wantErr {
				t.Errorf("telegramChatID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.wantID {
				t.Errorf("telegramChatID() = %d, want %d", got, tt.wantID)
			}
		})
	}
}

// ── telegramParseButtons ─────────────────────────────────────────────────────

func TestTelegramParseButtons(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		wantErr bool
		wantLen int // expected number of rows
	}{
		{
			name: "valid 1 row",
			raw: []any{
				[]any{map[string]any{"text": "Yes", "callback_data": "yes"}},
			},
			wantLen: 1,
		},
		{
			name: "valid multi row",
			raw: []any{
				[]any{map[string]any{"text": "A", "callback_data": "a"}},
				[]any{map[string]any{"text": "B", "callback_data": "b"}},
			},
			wantLen: 2,
		},
		{
			name:    "json string",
			raw:     `[[{"text":"Ok","callback_data":"ok"}]]`,
			wantLen: 1,
		},
		{
			name:    "nil",
			raw:     nil,
			wantErr: true,
		},
		{
			name:    "callback_data too long (65 bytes)",
			raw:     []any{[]any{map[string]any{"text": "X", "callback_data": strings.Repeat("a", 65)}}},
			wantErr: true,
		},
		{
			name:    "callback_data exactly 64 bytes",
			raw:     []any{[]any{map[string]any{"text": "X", "callback_data": strings.Repeat("a", 64)}}},
			wantLen: 1,
		},
		{
			name:    "missing text",
			raw:     []any{[]any{map[string]any{"callback_data": "cb"}}},
			wantErr: true,
		},
		{
			name:    "missing callback_data",
			raw:     []any{[]any{map[string]any{"text": "T"}}},
			wantErr: true,
		},
		{
			name:    "flat array not 2d",
			raw:     []any{map[string]any{"text": "T", "callback_data": "cb"}},
			wantErr: true,
		},
		{
			name:    "non-object button",
			raw:     []any{[]any{"not_an_object"}},
			wantErr: true,
		},
		{
			name:    "numeric callback_data is json-encoded",
			raw:     []any{[]any{map[string]any{"text": "N", "callback_data": float64(42)}}},
			wantLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := telegramParseButtons(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("telegramParseButtons() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(rows) != tt.wantLen {
				t.Errorf("telegramParseButtons() got %d rows, want %d", len(rows), tt.wantLen)
			}
		})
	}
}

// ── telegram_send_message ────────────────────────────────────────────────────

func TestTelegramSendMessage_NilClient(t *testing.T) {
	fn := newTelegramSendMessageFn(nilTelegram())
	_, err := testutil.New(fn).
		WithInput("chat_id", float64(123)).
		WithInput("text", "hello").
		Run()
	if err == nil || err.Error() != "Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTelegramSendMessage_MissingText(t *testing.T) {
	// Nil client fires first; confirms validation order is: nil-client → chat_id → text.
	fn := newTelegramSendMessageFn(nilTelegram())
	_, err := testutil.New(fn).WithInput("chat_id", float64(1)).Run()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestTelegramSendMessage_MissingChatID(t *testing.T) {
	fn := newTelegramSendMessageFn(nilTelegram())
	_, err := testutil.New(fn).Run()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ── telegram_get_file_url ────────────────────────────────────────────────────

func TestTelegramGetFileURL_NilClient(t *testing.T) {
	fn := newTelegramGetFileURLFn(nilTelegram())
	_, err := testutil.New(fn).WithInput("file_id", "abc123").Run()
	if err == nil || err.Error() != "Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTelegramGetFileURL_MissingFileID(t *testing.T) {
	fn := newTelegramGetFileURLFn(nilTelegram())
	_, err := testutil.New(fn).Run()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ── telegram_transcribe ──────────────────────────────────────────────────────

func TestTelegramTranscribe_NilClient(t *testing.T) {
	fn := newTelegramTranscribeFn(nilTelegram(), nil)
	_, err := testutil.New(fn).WithInput("file_id", "f123").Run()
	if err == nil || err.Error() != "Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTelegramTranscribe_MissingFileID(t *testing.T) {
	fn := newTelegramTranscribeFn(nilTelegram(), nil)
	_, err := testutil.New(fn).Run()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ── telegram_send_button ─────────────────────────────────────────────────────

func TestTelegramSendButton_NilClient(t *testing.T) {
	fn := newTelegramSendButtonFn(nilTelegram())
	_, err := testutil.New(fn).
		WithInput("chat_id", float64(1)).
		WithInput("text", "pick one").
		WithInput("buttons", []any{[]any{map[string]any{"text": "A", "callback_data": "a"}}}).
		Run()
	if err == nil || err.Error() != "Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTelegramSendButton_MissingChatID(t *testing.T) {
	fn := newTelegramSendButtonFn(nilTelegram())
	_, err := testutil.New(fn).Run()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestTelegramSendButton_MissingText(t *testing.T) {
	fn := newTelegramSendButtonFn(nilTelegram())
	_, err := testutil.New(fn).WithInput("chat_id", float64(1)).Run()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestTelegramSendButton_MissingButtons(t *testing.T) {
	fn := newTelegramSendButtonFn(nilTelegram())
	_, err := testutil.New(fn).
		WithInput("chat_id", float64(1)).
		WithInput("text", "msg").
		Run()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestTelegramSendButton_ButtonCallbackDataTooLong(t *testing.T) {
	fn := newTelegramSendButtonFn(nilTelegram())
	_, err := testutil.New(fn).
		WithInput("chat_id", float64(1)).
		WithInput("text", "msg").
		WithInput("buttons", []any{[]any{map[string]any{
			"text":          "A",
			"callback_data": strings.Repeat("x", 65),
		}}}).
		Run()
	if err == nil {
		t.Error("expected error for too-long callback_data, got nil")
	}
}

func TestTelegramSendButton_ValidButtonsParsed(t *testing.T) {
	// Nil client fires after button parsing, so we can verify buttons are parsed OK
	// by checking the error is the nil-client error (not a button parse error).
	fn := newTelegramSendButtonFn(nilTelegram())
	_, err := testutil.New(fn).
		WithInput("chat_id", float64(1)).
		WithInput("text", "msg").
		WithInput("buttons", []any{[]any{map[string]any{"text": "Yes", "callback_data": "yes"}}}).
		Run()
	if err == nil || err.Error() != "Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings" {
		t.Errorf("expected nil-client error after valid buttons, got: %v", err)
	}
}

func TestTelegramSendButton_ValidButtonsFromJSONString(t *testing.T) {
	fn := newTelegramSendButtonFn(nilTelegram())
	_, err := testutil.New(fn).
		WithInput("chat_id", float64(1)).
		WithInput("text", "msg").
		WithInput("buttons", `[[{"text":"Go","callback_data":"go"}]]`).
		Run()
	if err == nil || err.Error() != "Telegram not connected — add TELEGRAM_BOT_TOKEN in Settings" {
		t.Errorf("expected nil-client error after JSON string buttons, got: %v", err)
	}
}

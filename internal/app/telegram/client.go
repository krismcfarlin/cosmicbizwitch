// Package telegram provides a lightweight Telegram Bot API client.
// Credentials (bot token) are stored in app_settings, not hard-coded.
package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const apiBase = "https://api.telegram.org/bot"

// Client makes authenticated requests to the Telegram Bot API using a bot token.
type Client struct {
	botToken string
	http     *http.Client
}

// NewClient creates a Client backed by the given bot token.
func NewClient(botToken string) *Client {
	return &Client{
		botToken: botToken,
		http:     http.DefaultClient,
	}
}

// SendMessageParams holds all parameters for the Telegram sendMessage API method.
// Fields with omitempty are omitted from the JSON body when they hold their zero value.
type SendMessageParams struct {
	ChatID                int64  `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`              // "Markdown", "MarkdownV2", "HTML", or ""
	ReplyToMessageID      int64  `json:"reply_to_message_id,omitempty"`
	MessageThreadID       int64  `json:"message_thread_id,omitempty"`
	DisableNotification   bool   `json:"disable_notification,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview,omitempty"`
	ProtectContent        bool   `json:"protect_content,omitempty"`
}

// SendMessage posts a message to a Telegram chat using the provided parameters.
func (c *Client) SendMessage(params SendMessageParams) error {
	return c.postMethodJSON("sendMessage", params)
}

// InlineKeyboardButton is one button in an inline keyboard row.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// SendMessageWithButtonsParams is like SendMessageParams but adds an inline keyboard.
type SendMessageWithButtonsParams struct {
	ChatID          int64          `json:"chat_id"`
	MessageThreadID int64          `json:"message_thread_id,omitempty"`
	Text            string         `json:"text"`
	ParseMode       string         `json:"parse_mode,omitempty"`
	ReplyMarkup     map[string]any `json:"reply_markup"`
}

// SendMessageWithButtons sends a message with an inline keyboard attached.
// buttons is a 2-D slice: each inner slice is a row of buttons.
func (c *Client) SendMessageWithButtons(chatID, messageThreadID int64, text, parseMode string, buttons [][]InlineKeyboardButton) error {
	params := SendMessageWithButtonsParams{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
		Text:            text,
		ParseMode:       parseMode,
		ReplyMarkup: map[string]any{
			"inline_keyboard": buttons,
		},
	}
	return c.postMethodJSON("sendMessage", params)
}

// AnswerCallbackQuery acknowledges a button press so Telegram removes the loading spinner.
func (c *Client) AnswerCallbackQuery(callbackQueryID string, text string) error {
	payload := map[string]any{
		"callback_query_id": callbackQueryID,
	}
	if text != "" {
		payload["text"] = text
	}
	return c.postMethodJSON("answerCallbackQuery", payload)
}

// SetWebhook registers a webhook URL with Telegram.
// secretToken is sent back by Telegram in the X-Telegram-Bot-Api-Secret-Token header.
func (c *Client) SetWebhook(webhookURL string, secretToken string) error {
	payload := map[string]any{
		"url":             webhookURL,
		"secret_token":    secretToken,
		"allowed_updates": []string{"message", "callback_query"},
	}
	return c.postMethodJSON("setWebhook", payload)
}

// DeleteWebhook removes the webhook registration from Telegram.
func (c *Client) DeleteWebhook() error {
	return c.postMethodJSON("deleteWebhook", map[string]any{})
}

// GetFileURL calls getFile to resolve a file_id to a download URL.
// Returns the full HTTPS download URL: https://api.telegram.org/file/bot{TOKEN}/{file_path}
func (c *Client) GetFileURL(fileID string) (string, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", c.botToken, fileID)
	resp, err := c.http.Get(url)
	if err != nil {
		return "", fmt.Errorf("getFile request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
		Description string `json:"description"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("getFile parse: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("getFile failed: %s", result.Description)
	}
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", c.botToken, result.Result.FilePath), nil
}

// DownloadFile resolves a file_id and downloads the file bytes.
// Returns the bytes and the file extension derived from the file_path.
func (c *Client) DownloadFile(fileID string) ([]byte, string, error) {
	fileURL, err := c.GetFileURL(fileID)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Get(fileURL)
	if err != nil {
		return nil, "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read file body: %w", err)
	}
	// derive extension from URL path
	ext := ""
	parts := strings.Split(fileURL, ".")
	if len(parts) > 1 {
		ext = "." + parts[len(parts)-1]
	}
	return data, ext, nil
}

// postMethodJSON marshals any value as JSON and posts it to the given Bot API method.
func (c *Client) postMethodJSON(method string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram %s: marshal payload: %w", method, err)
	}

	url := apiBase + c.botToken + "/" + method
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("telegram %s: build request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: HTTP POST: %w", method, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(body)
		if len(preview) > 400 {
			preview = preview[:400]
		}
		return fmt.Errorf("telegram %s: HTTP %d: %s", method, resp.StatusCode, preview)
	}

	var apiResp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("telegram %s: parse response: %w", method, err)
	}
	if !apiResp.OK {
		return fmt.Errorf("telegram %s: API error: %s", method, apiResp.Description)
	}
	return nil
}

// Package slack provides a lightweight Slack bot API client.
// Credentials (bot token) are stored in app_settings, not hard-coded.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Client makes authenticated requests to the Slack Web API using a bot token.
type Client struct {
	botToken string
}

// NewClient creates a Client backed by the given bot token.
func NewClient(botToken string) *Client {
	return &Client{botToken: botToken}
}

// Channel represents a Slack channel (public or private).
type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SendMessage posts a plain-text message to a channel.
// channel may be a channel ID (C01234) or a name prefixed with # (e.g. #general).
func (c *Client) SendMessage(ctx context.Context, channel, text string) error {
	payload := map[string]any{
		"channel": channel,
		"text":    text,
	}
	return c.postAPI(ctx, "chat.postMessage", payload)
}

// SendRichMessage posts a Block Kit message to a channel.
// blocks should be a slice of Slack block objects serialisable to JSON.
func (c *Client) SendRichMessage(ctx context.Context, channel string, blocks []map[string]any) error {
	payload := map[string]any{
		"channel": channel,
		"blocks":  blocks,
	}
	return c.postAPI(ctx, "chat.postMessage", payload)
}

// ListChannels returns all public and private channels the bot has access to.
// It pages through the Slack API until all channels have been retrieved.
func (c *Client) ListChannels(ctx context.Context) ([]Channel, error) {
	var channels []Channel
	cursor := ""

	for {
		params := url.Values{
			"types":            {"public_channel,private_channel"},
			"exclude_archived": {"true"},
			"limit":            {"200"},
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}

		rawURL := "https://slack.com/api/conversations.list?" + params.Encode()
		body, err := c.doRequest(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("slack ListChannels: %w", err)
		}

		var resp struct {
			OK               bool   `json:"ok"`
			Error            string `json:"error"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
			Channels []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"channels"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("slack ListChannels: parse response: %w", err)
		}
		if !resp.OK {
			return nil, fmt.Errorf("slack ListChannels: API error: %s", resp.Error)
		}

		for _, ch := range resp.Channels {
			channels = append(channels, Channel{ID: ch.ID, Name: ch.Name})
		}

		cursor = resp.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}

	return channels, nil
}

// postAPI marshals payload as JSON and posts it to the given Slack API method.
func (c *Client) postAPI(ctx context.Context, method string, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack %s: marshal payload: %w", method, err)
	}

	rawURL := "https://slack.com/api/" + method
	body, err := c.doRequest(ctx, http.MethodPost, rawURL, data)
	if err != nil {
		return fmt.Errorf("slack %s: %w", method, err)
	}

	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("slack %s: parse response: %w", method, err)
	}
	if !resp.OK {
		return fmt.Errorf("slack %s: API error: %s", method, resp.Error)
	}
	return nil
}

// doRequest performs an authenticated HTTP request and returns the response body.
// A non-2xx HTTP status is returned as an error.
func (c *Client) doRequest(ctx context.Context, method, rawURL string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.botToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, rawURL, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(respBody)
		if len(preview) > 400 {
			preview = preview[:400]
		}
		return nil, fmt.Errorf("Slack API HTTP %d: %s", resp.StatusCode, preview)
	}
	return respBody, nil
}

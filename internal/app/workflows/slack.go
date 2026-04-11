package workflows

import (
	"context"
	"fmt"

	slackapp "cosmicbizwitch/internal/app/slack"
	"cosmicbizwitch/pkg/workflow"
)

// registerSlackActivities registers all Slack activity nodes.
func registerSlackActivities(eng *workflow.Engine, getSlack func() *slackapp.Client) {
	// ── slack_send_message ───────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("slack_send_message",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			sc := getSlack()
			if sc == nil {
				return nil, fmt.Errorf("Slack not connected — add SLACK_BOT_TOKEN in Settings")
			}
			channel := slackString(input, "channel")
			if channel == "" {
				return nil, fmt.Errorf("slack_send_message: channel is required")
			}
			text := slackString(input, "text")
			if text == "" {
				return nil, fmt.Errorf("slack_send_message: text is required")
			}
			if err := sc.SendMessage(ctx, channel, text); err != nil {
				return nil, fmt.Errorf("slack_send_message: %w", err)
			}
			return map[string]any{"ok": true}, nil
		},
		workflow.ActivityMeta{
			Category:    "Slack",
			Description: "Send a message to a Slack channel",
			InputFields: []workflow.FieldMeta{
				{Name: "channel", Type: "string", Required: true, Description: "Channel ID or name (e.g. #general or C01234ABC)."},
				{Name: "text", Type: "string", Required: true, Description: "Message text. Supports {{key}} interpolation."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "ok", Type: "boolean", Description: "True when the message was delivered successfully."},
			},
		},
	)

	// ── slack_list_channels ──────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("slack_list_channels",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			sc := getSlack()
			if sc == nil {
				return nil, fmt.Errorf("Slack not connected — add SLACK_BOT_TOKEN in Settings")
			}
			channels, err := sc.ListChannels(ctx)
			if err != nil {
				return nil, fmt.Errorf("slack_list_channels: %w", err)
			}
			// Convert to []any so the workflow engine can serialise it.
			list := make([]any, len(channels))
			for i, ch := range channels {
				list[i] = map[string]any{
					"id":   ch.ID,
					"name": ch.Name,
				}
			}
			return map[string]any{"channels": list}, nil
		},
		workflow.ActivityMeta{
			Category:    "Slack",
			Description: "List available Slack channels",
			InputFields: []workflow.FieldMeta{},
			OutputFields: []workflow.FieldMeta{
				{Name: "channels", Type: "array", Description: "Array of channel objects, each with id and name fields."},
			},
		},
	)
}

// slackString extracts a string value from the activity input map.
func slackString(input map[string]any, key string) string {
	if v, ok := input[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

package storage

import (
	"fmt"
	"log"
	"os"
	"sync"

	"cosmicbizwitch/internal/app/clickfunnels"
	googleapp "cosmicbizwitch/internal/app/google"
	slackapp "cosmicbizwitch/internal/app/slack"
	telegramapp "cosmicbizwitch/internal/app/telegram"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Setting represents a single application configuration entry stored in PocketBase.
type Setting struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	IsSecret    bool   `json:"is_secret"`
	IsSet       bool   `json:"is_set"`
}

// SettingsManager manages application settings backed by the app_settings PocketBase collection.
// It holds cached API clients rebuilt whenever settings are reloaded.
type SettingsManager struct {
	app            core.App
	mu             sync.RWMutex
	cfClient       *clickfunnels.Client
	googleClient   *googleapp.Client
	slackClient    *slackapp.Client
	telegramClient *telegramapp.Client
}

// NewSettingsManager creates a new SettingsManager backed by the given PocketBase app.
func NewSettingsManager(app core.App) *SettingsManager {
	return &SettingsManager{app: app}
}

// Reload reads CF credentials from the app_settings collection and rebuilds the CF client.
// If CF_API_KEY is empty, cfClient is set to nil.
func (m *SettingsManager) Reload(logger *log.Logger) error {
	apiKey := m.Get("CF_API_KEY")
	subdomain := m.Get("CF_SUBDOMAIN")
	workspaceID := m.Get("CF_WORKSPACE_ID")

	var cf *clickfunnels.Client
	if apiKey != "" {
		cf = clickfunnels.NewClient(clickfunnels.Config{
			APIKey:      apiKey,
			Subdomain:   subdomain,
			WorkspaceID: workspaceID,
		})
		if logger != nil {
			logger.Println("ClickFunnels client configured from app_settings")
		}
	} else {
		if logger != nil {
			logger.Println("CF_API_KEY not set in app_settings — ClickFunnels client disabled")
		}
	}

	// Google Drive / Docs client — client ID/secret fall back to env then hardcoded defaults
	clientID := m.Get("GOOGLE_CLIENT_ID")
	if clientID == "" {
		clientID = os.Getenv("GOOGLE_CLIENT_ID")
	}
	clientSecret := m.Get("GOOGLE_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	}
	refreshToken := m.Get("GOOGLE_REFRESH_TOKEN")

	var gc *googleapp.Client
	if refreshToken != "" {
		tm := googleapp.NewTokenManager(clientID, clientSecret, refreshToken)
		gc = googleapp.NewClient(tm)
		if logger != nil {
			logger.Println("Google Drive client configured from app_settings")
		}
	} else {
		if logger != nil {
			logger.Println("Google credentials not fully configured — Google Drive disabled")
		}
	}

	// Slack client
	slackBotToken := m.Get("SLACK_BOT_TOKEN")
	var sc *slackapp.Client
	if slackBotToken != "" {
		sc = slackapp.NewClient(slackBotToken)
		if logger != nil {
			logger.Println("Slack client configured from app_settings")
		}
	} else {
		if logger != nil {
			logger.Println("SLACK_BOT_TOKEN not set in app_settings — Slack client disabled")
		}
	}

	// Telegram client
	telegramBotToken := m.Get("TELEGRAM_BOT_TOKEN")
	var tc *telegramapp.Client
	if telegramBotToken != "" {
		tc = telegramapp.NewClient(telegramBotToken)
		if logger != nil {
			logger.Println("Telegram client configured from app_settings")
		}
	} else {
		if logger != nil {
			logger.Println("TELEGRAM_BOT_TOKEN not set in app_settings — Telegram client disabled")
		}
	}

	m.mu.Lock()
	m.cfClient = cf
	m.googleClient = gc
	m.slackClient = sc
	m.telegramClient = tc
	m.mu.Unlock()
	return nil
}

// CFClient returns the current ClickFunnels client in a thread-safe manner.
// Returns nil if CF_API_KEY is not configured.
func (m *SettingsManager) CFClient() *clickfunnels.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfClient
}

// GoogleClient returns the current Google Drive/Docs client in a thread-safe manner.
// Returns nil if Google credentials are not fully configured.
func (m *SettingsManager) GoogleClient() *googleapp.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.googleClient
}

// SlackClient returns the current Slack bot client in a thread-safe manner.
// Returns nil if SLACK_BOT_TOKEN is not configured.
func (m *SettingsManager) SlackClient() *slackapp.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.slackClient
}

// TelegramClient returns the current Telegram bot client in a thread-safe manner.
// Returns nil if TELEGRAM_BOT_TOKEN is not configured.
func (m *SettingsManager) TelegramClient() *telegramapp.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.telegramClient
}

// Get retrieves a setting value from PocketBase by key.
// Returns empty string if the key does not exist.
func (m *SettingsManager) Get(key string) string {
	records, err := m.app.FindRecordsByFilter("app_settings", "key = {:key}", "", 1, 0, dbx.Params{"key": key})
	if err != nil || len(records) == 0 {
		return ""
	}
	return records[0].GetString("value")
}

// Set writes a setting value to PocketBase. The record is created if it does not exist.
func (m *SettingsManager) Set(key, value string) error {
	records, err := m.app.FindRecordsByFilter("app_settings", "key = {:key}", "", 1, 0, dbx.Params{"key": key})
	if err != nil {
		return fmt.Errorf("settings Set: find %q: %w", key, err)
	}
	if len(records) == 0 {
		col, err := m.app.FindCollectionByNameOrId("app_settings")
		if err != nil {
			return fmt.Errorf("settings Set: find collection: %w", err)
		}
		rec := core.NewRecord(col)
		rec.Set("key", key)
		rec.Set("label", key)
		rec.Set("description", "")
		rec.Set("is_secret", false)
		rec.Set("value", value)
		if err := m.app.Save(rec); err != nil {
			return fmt.Errorf("settings Set: create %q: %w", key, err)
		}
		return nil
	}
	rec := records[0]
	rec.Set("value", value)
	if err := m.app.Save(rec); err != nil {
		return fmt.Errorf("settings Set: save %q: %w", key, err)
	}
	return nil
}

// All returns all settings. Secret values are masked as empty strings.
func (m *SettingsManager) All() ([]Setting, error) {
	records, err := m.app.FindRecordsByFilter("app_settings", "id != ''", "key", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("settings All: %w", err)
	}
	out := make([]Setting, 0, len(records))
	for _, rec := range records {
		isSecret := rec.GetBool("is_secret")
		val := rec.GetString("value")
		isSet := val != ""
		if isSecret {
			val = ""
		}
		out = append(out, Setting{
			Key:         rec.GetString("key"),
			Value:       val,
			Label:       rec.GetString("label"),
			Description: rec.GetString("description"),
			IsSecret:    isSecret,
			IsSet:       isSet,
		})
	}
	return out, nil
}

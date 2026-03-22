package storage

import (
	"fmt"
	"log"
	"sync"

	"cosmicbizwitch/internal/app/clickfunnels"

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
}

// SettingsManager manages application settings backed by the app_settings PocketBase collection.
// It holds a cached ClickFunnels client rebuilt whenever settings are reloaded.
type SettingsManager struct {
	app      core.App
	mu       sync.RWMutex
	cfClient *clickfunnels.Client
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

	m.mu.Lock()
	m.cfClient = cf
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
		return fmt.Errorf("settings Set: key %q not found", key)
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
		if isSecret {
			val = ""
		}
		out = append(out, Setting{
			Key:         rec.GetString("key"),
			Value:       val,
			Label:       rec.GetString("label"),
			Description: rec.GetString("description"),
			IsSecret:    isSecret,
		})
	}
	return out, nil
}

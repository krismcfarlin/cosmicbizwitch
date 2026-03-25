package storage

import (
	"log"
	"os"

	"github.com/pocketbase/pocketbase/core"
)

func createContacts(app core.App) error {
	col := core.NewBaseCollection("contacts")
	col.Fields.Add(
		&core.TextField{Name: "first_name"},
		&core.TextField{Name: "last_name"},
		&core.EmailField{Name: "email"},
		&core.TextField{Name: "phone"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createBirthChartRecords(app core.App) error {
	col := core.NewBaseCollection("birth_chart_records")

	// relation to contacts collection
	if contactsCol, err := app.FindCollectionByNameOrId("contacts"); err == nil {
		col.Fields.Add(&core.RelationField{Name: "contact", CollectionId: contactsCol.Id, MaxSelect: 1})
	}

	col.Fields.Add(
		&core.TextField{Name: "birth_time"},
		&core.TextField{Name: "birth_place"},
		&core.SelectField{Name: "house_system", MaxSelect: 1, Values: []string{"placidus", "porphyry", "whole"}},
		&core.TextField{Name: "money_mode", Max: 100},
		&core.TextField{Name: "sun_code", Max: 100},
		&core.TextField{Name: "rising_code", Max: 100},
		&core.TextField{Name: "moon_code", Max: 100},
		&core.TextField{Name: "moon_phase", Max: 50},
		&core.TextField{Name: "sun_degree", Max: 50},
		&core.TextField{Name: "moon_sign", Max: 50},
		&core.TextField{Name: "moon_degree", Max: 50},
		&core.TextField{Name: "mercury_code", Max: 100},
		&core.TextField{Name: "venus_code", Max: 100},
		&core.TextField{Name: "mars_code", Max: 100},
		&core.TextField{Name: "jupiter_code", Max: 100},
		&core.TextField{Name: "saturn_code", Max: 100},
		&core.TextField{Name: "uranus_code", Max: 100},
		&core.TextField{Name: "neptune_code", Max: 100},
		&core.TextField{Name: "pluto_code", Max: 100},
		&core.TextField{Name: "house_1_code", Max: 100},
		&core.TextField{Name: "house_2_code", Max: 100},
		&core.TextField{Name: "house_3_code", Max: 100},
		&core.TextField{Name: "house_4_code", Max: 100},
		&core.TextField{Name: "house_5_code", Max: 100},
		&core.TextField{Name: "house_6_code", Max: 100},
		&core.TextField{Name: "house_7_code", Max: 100},
		&core.TextField{Name: "house_8_code", Max: 100},
		&core.TextField{Name: "house_9_code", Max: 100},
		&core.TextField{Name: "house_10_code", Max: 100},
		&core.TextField{Name: "house_11_code", Max: 100},
		&core.TextField{Name: "house_12_code", Max: 100},
		&core.TextField{Name: "midheaven_code", Max: 100},
		&core.TextField{Name: "north_node", Max: 100},
		&core.TextField{Name: "south_node", Max: 100},
		&core.TextField{Name: "chiron_code", Max: 100},
		&core.TextField{Name: "chart_url", Max: 500},
		&core.TextField{Name: "chart_notes", Max: 100000},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createNewMoonIntentions(app core.App) error {
	col := core.NewBaseCollection("new_moon_intentions")
	col.Fields.Add(
		&core.TextField{Name: "cf_contact_id"},
		&core.TextField{Name: "intention"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createShares(app core.App) error {
	col := core.NewBaseCollection("shares")
	col.Fields.Add(
		&core.TextField{Name: "token", Required: true},
		&core.TextField{Name: "resource_type"},
		&core.TextField{Name: "resource_id"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createReportLinks(app core.App) error {
	col := core.NewBaseCollection("report_links")
	col.Fields.Add(
		&core.TextField{Name: "cf_contact_id"},
		&core.TextField{Name: "url"},
		&core.TextField{Name: "report_type"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

func createUtmSource(app core.App) error {
	col := core.NewBaseCollection("utm_source")
	col.Fields.Add(
		&core.TextField{Name: "source"},
		&core.TextField{Name: "medium"},
		&core.TextField{Name: "campaign"},
		&core.TextField{Name: "cf_contact_id"},
	)
	addAutodateFields(col)
	return app.Save(col)
}

// migrateBirthChartRecords ensures the existing birth_chart_records collection
// matches the server schema: removes old fields and adds any missing ones.
func migrateBirthChartRecords(app core.App, logger *log.Logger) error {
	col, err := app.FindCollectionByNameOrId("birth_chart_records")
	if err != nil {
		return nil // doesn't exist yet, createCollections will handle it
	}

	changed := false

	// Remove legacy fields that no longer exist in the server schema
	for _, oldName := range []string{"cf_contact_id", "chart_data"} {
		if f := col.Fields.GetByName(oldName); f != nil {
			col.Fields.RemoveById(f.GetId())
			logger.Printf("birth_chart_records: removing field %q", oldName)
			changed = true
		}
	}

	// Add contact relation if missing
	if col.Fields.GetByName("contact") == nil {
		if contactsCol, err := app.FindCollectionByNameOrId("contacts"); err == nil {
			col.Fields.Add(&core.RelationField{Name: "contact", CollectionId: contactsCol.Id, MaxSelect: 1})
			logger.Printf("birth_chart_records: adding field %q", "contact")
			changed = true
		}
	}

	// Text fields to ensure exist: name -> max
	textFields := []struct {
		name string
		max  int
	}{
		{"birth_time", 0},
		{"birth_place", 0},
		{"money_mode", 100},
		{"sun_code", 100},
		{"rising_code", 100},
		{"moon_code", 100},
		{"moon_phase", 50},
		{"sun_degree", 50},
		{"moon_sign", 50},
		{"moon_degree", 50},
		{"mercury_code", 100},
		{"venus_code", 100},
		{"mars_code", 100},
		{"jupiter_code", 100},
		{"saturn_code", 100},
		{"uranus_code", 100},
		{"neptune_code", 100},
		{"pluto_code", 100},
		{"house_1_code", 100},
		{"house_2_code", 100},
		{"house_3_code", 100},
		{"house_4_code", 100},
		{"house_5_code", 100},
		{"house_6_code", 100},
		{"house_7_code", 100},
		{"house_8_code", 100},
		{"house_9_code", 100},
		{"house_10_code", 100},
		{"house_11_code", 100},
		{"house_12_code", 100},
		{"midheaven_code", 100},
		{"north_node", 100},
		{"south_node", 100},
		{"chiron_code", 100},
		{"chart_url", 500},
		{"chart_notes", 100000},
	}
	for _, f := range textFields {
		if col.Fields.GetByName(f.name) == nil {
			col.Fields.Add(&core.TextField{Name: f.name, Max: f.max})
			logger.Printf("birth_chart_records: adding field %q", f.name)
			changed = true
		}
	}

	// Select field for house_system
	if col.Fields.GetByName("house_system") == nil {
		col.Fields.Add(&core.SelectField{Name: "house_system", MaxSelect: 1, Values: []string{"placidus", "porphyry", "whole"}})
		logger.Printf("birth_chart_records: adding field %q", "house_system")
		changed = true
	}

	if changed {
		return app.Save(col)
	}
	return nil
}

// createAppSettings creates the app_settings collection with fields for key/value configuration.
func createAppSettings(app core.App) error {
	col := core.NewBaseCollection("app_settings")
	col.Fields.Add(
		&core.TextField{Name: "key", Required: true, Min: 1},
		&core.TextField{Name: "value", Max: 10000},
		&core.TextField{Name: "label"},
		&core.TextField{Name: "description"},
		&core.BoolField{Name: "is_secret"},
	)
	return app.Save(col)
}

// seedAppSettings creates default app_settings records if they don't already exist.
// When creating a record, it bootstraps the value from the matching environment variable
// so existing .env-based deployments work on first boot.
func seedAppSettings(app core.App, logger *log.Logger) error {
	seeds := []struct {
		key          string
		label        string
		description  string
		isSecret     bool
		defaultValue string
	}{
		{"CF_API_KEY", "ClickFunnels API Key", "API key from your ClickFunnels account", true, ""},
		{"CF_SUBDOMAIN", "ClickFunnels Subdomain", "Your ClickFunnels subdomain (e.g. yourbrand)", false, "cosmicbizwitchllc"},
		{"CF_WORKSPACE_ID", "ClickFunnels Workspace ID", "Your ClickFunnels workspace ID", false, ""},
		{"OPENROUTER_API_KEY", "OpenRouter API Key", "API key from openrouter.ai — enables access to many models via one key", true, ""},
		{"ANTHROPIC_API_KEY", "Anthropic API Key", "API key from console.anthropic.com — for direct Anthropic API access", true, ""},
		{"GOOGLE_CLIENT_ID", "Google OAuth Client ID", "OAuth2 Client ID from Google Cloud Console", false, ""},
		{"GOOGLE_CLIENT_SECRET", "Google OAuth Client Secret", "OAuth2 Client Secret from Google Cloud Console", true, ""},
		{"GOOGLE_REFRESH_TOKEN", "Google OAuth Refresh Token", "Obtained via /api/google/auth/start — do not set manually", true, ""},
	}

	col, err := app.FindCollectionByNameOrId("app_settings")
	if err != nil {
		return nil // collection not ready yet
	}

	for _, s := range seeds {
		existing, _ := app.FindRecordsByFilter("app_settings", "key = {:key}", "", 1, 0, map[string]any{"key": s.key})
		if len(existing) > 0 {
			// Backfill: if the record exists but value is empty, apply env var or default.
			rec := existing[0]
			if rec.GetString("value") == "" {
				val := os.Getenv(s.key)
				if val == "" {
					val = s.defaultValue
				}
				if val != "" {
					rec.Set("value", val)
					if err := app.Save(rec); err != nil {
						return err
					}
					logger.Printf("app_settings: backfilled key %q", s.key)
				}
			}
			continue
		}
		rec := core.NewRecord(col)
		rec.Set("key", s.key)
		rec.Set("label", s.label)
		rec.Set("description", s.description)
		rec.Set("is_secret", s.isSecret)
		val := os.Getenv(s.key)
		if val == "" {
			val = s.defaultValue
		}
		if val != "" {
			rec.Set("value", val)
		}
		if err := app.Save(rec); err != nil {
			return err
		}
		logger.Printf("app_settings: seeded key %q", s.key)
	}
	return nil
}

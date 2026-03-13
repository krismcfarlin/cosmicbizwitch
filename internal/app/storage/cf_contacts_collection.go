package storage

import (
	"github.com/pocketbase/pocketbase/core"
)

// createCFContactsCollection creates the cf_contacts PocketBase collection.
// Contacts are matched by cf_id for upsert operations.
func createCFContactsCollection(app core.App) error {
	col := core.NewBaseCollection("cf_contacts")

	col.Fields.Add(
		// cf_id is the ClickFunnels numeric ID, used as unique key for upsert.
		&core.NumberField{Name: "cf_id", Required: true},

		&core.TextField{Name: "cf_public_id"},
		&core.EmailField{Name: "email_address"},
		&core.TextField{Name: "first_name"},
		&core.TextField{Name: "last_name"},
		&core.TextField{Name: "phone_number"},
		&core.TextField{Name: "time_zone"},
		&core.NumberField{Name: "anonymous"},
		&core.BoolField{Name: "is_active"},
		&core.TextField{Name: "unsubscribed_at"},
		&core.TextField{Name: "email_suppression_reason"},
		&core.TextField{Name: "fb_url"},
		&core.TextField{Name: "twitter_url"},
		&core.TextField{Name: "instagram_url"},
		&core.TextField{Name: "linkedin_url"},
		&core.TextField{Name: "website_url"},

		// tags and custom_attributes are stored as JSON strings.
		&core.JSONField{Name: "tags"},
		&core.JSONField{Name: "custom_attributes"},

		&core.TextField{Name: "cf_created_at"},
		&core.TextField{Name: "cf_updated_at"},
	)

	addAutodateFields(col)

	return app.Save(col)
}

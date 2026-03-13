package storage

import (
	"github.com/pocketbase/pocketbase/core"
)

// createBirthFormCollection creates the birth_form_submissions PocketBase collection.
// Stores only the CF contact ID and webhook event ID — the rest of the payload is not saved.
func createBirthFormCollection(app core.App) error {
	col := core.NewBaseCollection("birth_form_submissions")

	col.Fields.Add(
		// cf_contact_id is the ClickFunnels subject_id from the webhook.
		&core.NumberField{Name: "cf_contact_id", Required: true},
		// event_id is the unique webhook event_id for deduplication.
		&core.TextField{Name: "event_id"},
	)

	addAutodateFields(col)

	return app.Save(col)
}

package storage

import (
	"database/sql"
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// SetupCollections creates all PocketBase collections if they don't exist.
// No data migration is needed in the starter — collections are defined fresh.
func SetupCollections(app core.App, legacyDB *sql.DB, logger *log.Logger) error {
	if err := createCollections(app, logger); err != nil {
		return err
	}
	if err := ensureAutodateFields(app); err != nil {
		return err
	}
	return nil
}

// createCollections registers application collections.
// Add your collection creation functions here and call them from this loop.
func createCollections(app core.App, logger *log.Logger) error {
	// TODO: Add your collection creators here, for example:
	//   {"my_collection", createMyCollection},
	collections := []struct {
		name   string
		create func(core.App) error
	}{}

	for _, c := range collections {
		existing, _ := app.FindCollectionByNameOrId(c.name)
		if existing != nil {
			continue // already exists
		}
		logger.Printf("Creating PocketBase collection: %s", c.name)
		if err := c.create(app); err != nil {
			return err
		}
	}
	return nil
}

// ensureAutodateFields adds created/updated AutodateFields to any collection missing them.
// This is needed so PocketBase's filter/sort resolver can use "created"/"updated" as sort keys.
func ensureAutodateFields(app core.App) error {
	// List the names of all your collections here so they get autodate fields.
	names := []string{}

	for _, name := range names {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		changed := false
		if col.Fields.GetByName("created") == nil {
			col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			changed = true
		}
		if col.Fields.GetByName("updated") == nil {
			col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
			changed = true
		}
		if changed {
			if err := app.Save(col); err != nil {
				return err
			}
		}
	}
	return nil
}

// addAutodateFields adds created/updated autodate fields to a collection.
// Call this inside your createXxx functions before saving the collection.
func addAutodateFields(col *core.Collection) {
	col.Fields.Add(
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
}

// setMigrationTimestamps sets created/updated on a record before SaveNoValidate.
// AutodateField.FindSetter returns noopSetter so rec.Set() is ignored; SetRaw bypasses it.
func setMigrationTimestamps(rec *core.Record) {
	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	rec.SetRaw("created", now)
	rec.SetRaw("updated", now)
}

// alreadyMigrated returns true if the collection already has records.
// Use this guard at the top of migration functions to make them idempotent.
func alreadyMigrated(app core.App, collection string) bool {
	count, err := app.CountRecords(collection)
	return err == nil && count > 0
}

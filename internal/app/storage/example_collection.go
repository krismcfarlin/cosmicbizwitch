// This is an example file. Copy this pattern, rename to your domain, and add to SetupCollections.
// Delete this file when done.
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// ExampleItem is a sample domain struct. Replace with your own fields.
type ExampleItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`    // e.g. "active", "inactive"
	Category    string    `json:"category"`  // e.g. "type_a", "type_b"
	RelatedID   string    `json:"related_id"` // relation to another collection
	DueDate     string    `json:"due_date"`  // date string
	Score       float64   `json:"score"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// createExampleCollection shows how to define a PocketBase collection in code.
// Register it in createCollections() in collections.go:
//
//	{"example_items", createExampleCollection},
func createExampleCollection(app core.App) error {
	col := core.NewBaseCollection("example_items")

	col.Fields.Add(
		// TextField: basic string, with optional max length and required constraint
		&core.TextField{Name: "name", Required: true, Max: 255},

		// SelectField: enum-style field with allowed values
		&core.SelectField{Name: "status", MaxSelect: 1, Values: []string{
			"active", "inactive", "archived",
		}},

		// SelectField: another enum (can be multi-select by increasing MaxSelect)
		&core.SelectField{Name: "category", MaxSelect: 1, Values: []string{
			"type_a", "type_b", "type_c",
		}},

		// RelationField: foreign key to another collection
		// Replace "other_collection_id_here" with the actual PocketBase collection ID.
		// In practice, look it up: otherCol, _ := app.FindCollectionByNameOrId("other_items")
		// then use otherCol.Id
		// &core.RelationField{Name: "related_id", CollectionId: "other_collection_id_here", MaxSelect: 1},

		// DateField: ISO date/datetime
		&core.DateField{Name: "due_date"},

		// NumberField: integer or float; set OnlyInt: true for integers
		&core.NumberField{Name: "score", OnlyInt: false},

		// BoolField: true/false
		&core.BoolField{Name: "is_active"},

		// JSONField: arbitrary JSON blob
		&core.JSONField{Name: "metadata"},
	)

	// Always add autodate fields so created/updated are managed automatically.
	addAutodateFields(col)

	return app.Save(col)
}

// recordToExampleItem converts a PocketBase record to an ExampleItem struct.
func recordToExampleItem(rec *core.Record) *ExampleItem {
	return &ExampleItem{
		ID:        rec.Id,
		Name:      rec.GetString("name"),
		Status:    rec.GetString("status"),
		Category:  rec.GetString("category"),
		RelatedID: rec.GetString("related_id"),
		DueDate:   rec.GetString("due_date"),
		Score:     rec.GetFloat("score"),
		IsActive:  rec.GetBool("is_active"),
		CreatedAt: rec.GetDateTime("created").Time(),
		UpdatedAt: rec.GetDateTime("updated").Time(),
	}
}

// CreateExampleItem creates a new record in the example_items collection.
func (s *Store) CreateExampleItem(ctx context.Context, item *ExampleItem) error {
	col, err := s.app.FindCollectionByNameOrId("example_items")
	if err != nil {
		return fmt.Errorf("find collection: %w", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", item.Name)
	rec.Set("status", item.Status)
	rec.Set("category", item.Category)
	rec.Set("due_date", item.DueDate)
	rec.Set("score", item.Score)
	rec.Set("is_active", item.IsActive)
	if err := s.app.Save(rec); err != nil {
		return fmt.Errorf("save example_item: %w", err)
	}
	item.ID = rec.Id
	return nil
}

// GetExampleItem retrieves a single example item by ID.
func (s *Store) GetExampleItem(ctx context.Context, id string) (*ExampleItem, error) {
	rec, err := s.app.FindRecordById("example_items", id)
	if err != nil {
		return nil, fmt.Errorf("example_item not found: %s", id)
	}
	return recordToExampleItem(rec), nil
}

// ListExampleItems retrieves all example items, ordered by creation date descending.
func (s *Store) ListExampleItems(ctx context.Context, limit int) ([]*ExampleItem, error) {
	if limit == 0 {
		limit = 50
	}
	records, err := s.app.FindRecordsByFilter("example_items", "id != ''", "-created", limit, 0)
	if err != nil {
		return nil, fmt.Errorf("list example_items: %w", err)
	}
	items := make([]*ExampleItem, 0, len(records))
	for _, rec := range records {
		items = append(items, recordToExampleItem(rec))
	}
	return items, nil
}

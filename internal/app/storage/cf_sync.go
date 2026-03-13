package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmicbizwitch/internal/app/clickfunnels"
	"github.com/pocketbase/pocketbase/core"
)


// SyncCFContacts fetches all contacts from ClickFunnels and upserts them into
// the cf_contacts PocketBase collection. Contacts are matched by cf_id.
func (s *Store) SyncCFContacts(ctx context.Context, client *clickfunnels.Client) (added, updated int, err error) {
	afterID := 0

	for {
		contacts, fetchErr := client.ListContacts(ctx, afterID, nil)
		if fetchErr != nil {
			return added, updated, fmt.Errorf("list contacts (afterID=%d): %w", afterID, fetchErr)
		}

		for _, contact := range contacts {
			// Check for an existing record to determine add vs update.
			existingRecs, _ := s.app.FindRecordsByFilter(
				"cf_contacts",
				fmt.Sprintf("cf_id = %d", contact.ID),
				"", 1, 0,
			)
			var existingRec *core.Record
			if len(existingRecs) > 0 {
				existingRec = existingRecs[0]
			}

			if saveErr := s.upsertCFContact(ctx, contact, existingRec); saveErr != nil {
				return added, updated, fmt.Errorf("upsert contact id=%d: %w", contact.ID, saveErr)
			}

			if existingRec == nil {
				added++
			} else {
				updated++
			}
		}

		if len(contacts) < 20 {
			break
		}
		afterID = contacts[len(contacts)-1].ID
	}

	return added, updated, nil
}

// upsertCFContact inserts or updates a single ClickFunnels contact in PocketBase.
// Pass an existing record to perform an update; pass nil to create a new record.
func (s *Store) upsertCFContact(ctx context.Context, contact clickfunnels.Contact, existing *core.Record) error {
	tagsJSON, err := json.Marshal(contact.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	attrsJSON, err := json.Marshal(contact.CustomAttributes)
	if err != nil {
		return fmt.Errorf("marshal custom_attributes: %w", err)
	}

	var rec *core.Record
	if existing != nil {
		rec = existing
	} else {
		col, colErr := s.app.FindCollectionByNameOrId("cf_contacts")
		if colErr != nil {
			return fmt.Errorf("find cf_contacts collection: %w", colErr)
		}
		rec = core.NewRecord(col)
	}

	rec.Set("cf_id", contact.ID)
	rec.Set("cf_public_id", contact.PublicID)
	rec.Set("email_address", strOrEmpty(contact.EmailAddress))
	rec.Set("first_name", strOrEmpty(contact.FirstName))
	rec.Set("last_name", strOrEmpty(contact.LastName))
	rec.Set("phone_number", strOrEmpty(contact.PhoneNumber))
	rec.Set("time_zone", strOrEmpty(contact.TimeZone))
	if contact.Anonymous != nil {
		rec.Set("anonymous", *contact.Anonymous)
	} else {
		rec.Set("anonymous", 0)
	}
	rec.Set("is_active", contact.IsActive)
	rec.Set("unsubscribed_at", strOrEmpty(contact.UnsubscribedAt))
	rec.Set("email_suppression_reason", strOrEmpty(contact.EmailSuppressionReason))
	rec.Set("fb_url", strOrEmpty(contact.FbURL))
	rec.Set("twitter_url", strOrEmpty(contact.TwitterURL))
	rec.Set("instagram_url", strOrEmpty(contact.InstagramURL))
	rec.Set("linkedin_url", strOrEmpty(contact.LinkedinURL))
	rec.Set("website_url", strOrEmpty(contact.WebsiteURL))
	rec.Set("tags", string(tagsJSON))
	rec.Set("custom_attributes", string(attrsJSON))
	rec.Set("cf_created_at", contact.CreatedAt)
	rec.Set("cf_updated_at", contact.UpdatedAt)

	return s.app.SaveNoValidate(rec)
}

// strOrEmpty dereferences a *string, returning "" if nil.
func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// CFWebhookPayload is the shape of a ClickFunnels webhook POST body.
// Only subject_id and event_id are used; everything else is ignored.
type CFWebhookPayload struct {
	SubjectID int    `json:"subject_id"`
	EventID   string `json:"event_id"`
}

// StoreBirthFormSubmission saves the CF contact ID and event ID from a webhook.
func (s *Store) StoreBirthFormSubmission(ctx context.Context, payload CFWebhookPayload) error {
	col, err := s.app.FindCollectionByNameOrId("birth_form_submissions")
	if err != nil {
		return fmt.Errorf("find birth_form_submissions collection: %w", err)
	}

	rec := core.NewRecord(col)
	rec.Set("cf_contact_id", payload.SubjectID)
	rec.Set("event_id", payload.EventID)

	if err := s.app.SaveNoValidate(rec); err != nil {
		return fmt.Errorf("save birth_form_submission: %w", err)
	}
	return nil
}

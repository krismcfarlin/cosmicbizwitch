// Package clickfunnels provides an API client for the ClickFunnels 2.0 REST API.
package clickfunnels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Config holds the credentials and workspace settings for the ClickFunnels API.
type Config struct {
	APIKey      string
	Subdomain   string
	WorkspaceID string
}

// Client is an authenticated HTTP client for the ClickFunnels 2.0 API.
type Client struct {
	Config
	http *http.Client
}

// NewClient constructs a Client with a default HTTP client.
func NewClient(cfg Config) *Client {
	return &Client{
		Config: cfg,
		http:   &http.Client{},
	}
}

// baseURL returns the API base URL for the configured subdomain.
func (c *Client) baseURL() string {
	return fmt.Sprintf("https://%s.myclickfunnels.com/api/v2", c.Subdomain)
}

// do executes an HTTP request, injecting the required auth and user-agent headers.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("User-Agent", "cosmicbizwitch/1.0")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return c.http.Do(req)
}

// Tag represents a ClickFunnels contact tag.
type Tag struct {
	ID        int    `json:"id"`
	PublicID  string `json:"public_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	AppliedAt string `json:"applied_at"`
}

// AppliedTag represents a tag that has been applied to a specific contact.
// The ID here is the applied-tag record ID, not the tag ID.
type AppliedTag struct {
	ID    int `json:"id"`
	TagID int `json:"tag_id"`
	Tag   Tag `json:"tag"`
}

// Contact represents a ClickFunnels contact record.
type Contact struct {
	ID                          int               `json:"id"`
	PublicID                    string            `json:"public_id"`
	WorkspaceID                 int               `json:"workspace_id"`
	UUID                        string            `json:"uuid"`
	Anonymous                   *int              `json:"anonymous"`
	EmailAddress                *string           `json:"email_address"`
	FirstName                   *string           `json:"first_name"`
	LastName                    *string           `json:"last_name"`
	PhoneNumber                 *string           `json:"phone_number"`
	TimeZone                    *string           `json:"time_zone"`
	FbURL                       *string           `json:"fb_url"`
	TwitterURL                  *string           `json:"twitter_url"`
	InstagramURL                *string           `json:"instagram_url"`
	LinkedinURL                 *string           `json:"linkedin_url"`
	WebsiteURL                  *string           `json:"website_url"`
	UnsubscribedAt              *string           `json:"unsubscribed_at"`
	LastNotificationEmailSentAt *string           `json:"last_notification_email_sent_at"`
	EmailSuppressionReason      *string           `json:"email_suppression_reason"`
	IsActive                    bool              `json:"is_active"`
	Tags                        []Tag             `json:"tags"`
	CustomAttributes            map[string]interface{} `json:"custom_attributes"`
	CreatedAt                   string            `json:"created_at"`
	UpdatedAt                   string            `json:"updated_at"`
}

// ContactParams holds the writable fields for creating or updating a contact.
type ContactParams struct {
	EmailAddress     *string           `json:"email_address,omitempty"`
	FirstName        *string           `json:"first_name,omitempty"`
	LastName         *string           `json:"last_name,omitempty"`
	PhoneNumber      *string           `json:"phone_number,omitempty"`
	TimeZone         *string           `json:"time_zone,omitempty"`
	FbURL            *string           `json:"fb_url,omitempty"`
	TwitterURL       *string           `json:"twitter_url,omitempty"`
	InstagramURL     *string           `json:"instagram_url,omitempty"`
	LinkedinURL      *string           `json:"linkedin_url,omitempty"`
	WebsiteURL       *string           `json:"website_url,omitempty"`
	TagIDs           []int             `json:"tag_ids,omitempty"`
	CustomAttributes map[string]string `json:"custom_attributes,omitempty"`
}


// ListContacts fetches a page of contacts from the workspace.
// afterID is the cursor ID for pagination (0 means first page).
// perPage controls page size (0 uses the API default).
// tagIDs filters results to contacts with all specified tag IDs.
func (c *Client) ListContacts(ctx context.Context, afterID int, perPage int, tagIDs []int) ([]Contact, error) {
	endpoint := fmt.Sprintf("%s/workspaces/%s/contacts", c.baseURL(), c.WorkspaceID)

	params := url.Values{}
	if afterID > 0 {
		params.Set("after", strconv.Itoa(afterID))
	}
	if perPage > 0 {
		params.Set("per_page", strconv.Itoa(perPage))
	}
	if len(tagIDs) > 0 {
		parts := make([]string, len(tagIDs))
		for i, id := range tagIDs {
			parts[i] = strconv.Itoa(id)
		}
		params.Set("tag_ids", strings.Join(parts, ","))
	}
	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build list contacts request: %w", err)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: list contacts request (url=%s): %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clickfunnels: list contacts: status %d url=%s body=%s", resp.StatusCode, endpoint, string(body))
	}

	var contacts []Contact
	if err := json.NewDecoder(resp.Body).Decode(&contacts); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode list contacts response: %w", err)
	}
	return contacts, nil
}

// GetAllContacts fetches contacts, paginating until maxRecords is reached or results are exhausted.
// perPage controls page size. maxRecords caps the total (0 = no cap, use carefully).
// onPage is called after each page with the running total so callers can log progress.
func (c *Client) GetAllContacts(ctx context.Context, perPage int, maxRecords int, tagIDs []int, onPage func(pageNum, total int)) ([]Contact, error) {
	var all []Contact
	afterID := 0
	pageNum := 0
	for {
		page, err := c.ListContacts(ctx, afterID, perPage, tagIDs)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		all = append(all, page...)
		pageNum++
		if onPage != nil {
			onPage(pageNum, len(all))
		}
		if maxRecords > 0 && len(all) >= maxRecords {
			all = all[:maxRecords]
			break
		}
		afterID = page[len(page)-1].ID
	}
	return all, nil
}

// GetContact fetches a single contact by its numeric ID.
func (c *Client) GetContact(ctx context.Context, id int) (*Contact, error) {
	endpoint := fmt.Sprintf("%s/contacts/%d", c.baseURL(), id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build get contact request: %w", err)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: get contact request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("clickfunnels: get contact %d: unexpected status %d", id, resp.StatusCode)
	}

	var contact Contact
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode get contact response: %w", err)
	}
	return &contact, nil
}

// listTagsPage fetches one page of tags with optional after-cursor and name filter.
func (c *Client) listTagsPage(ctx context.Context, afterID int, nameFilter string) ([]Tag, error) {
	base := fmt.Sprintf("%s/workspaces/%s/contacts/tags", c.baseURL(), c.WorkspaceID)
	params := url.Values{}
	if afterID > 0 {
		params.Set("after", strconv.Itoa(afterID))
	}
	if nameFilter != "" {
		params.Set("filter[name]", nameFilter)
	}
	endpoint := base
	if len(params) > 0 {
		endpoint = base + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build list tags request: %w", err)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: list tags request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clickfunnels: list tags: status %d body=%s", resp.StatusCode, string(body))
	}

	var tags []Tag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode list tags response: %w", err)
	}
	return tags, nil
}

// ListTags fetches all contact tags from the workspace, paginating via after-cursor.
// nameFilter is passed as filter[name] to the CF API for server-side filtering; pass "" to get all tags.
func (c *Client) ListTags(ctx context.Context, nameFilter string) ([]Tag, error) {
	var all []Tag
	afterID := 0
	seen := make(map[int]struct{})
	for {
		page, err := c.listTagsPage(ctx, afterID, nameFilter)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		// Guard against infinite loops if the API returns overlapping pages.
		newItems := 0
		for _, t := range page {
			if _, dup := seen[t.ID]; !dup {
				seen[t.ID] = struct{}{}
				all = append(all, t)
				newItems++
			}
		}
		if newItems == 0 {
			break
		}
		afterID = page[len(page)-1].ID
	}
	return all, nil
}

// ListAppliedTags returns all applied-tag records for a contact.
func (c *Client) ListAppliedTags(ctx context.Context, contactID int) ([]AppliedTag, error) {
	endpoint := fmt.Sprintf("%s/contacts/%d/applied_tags", c.baseURL(), contactID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build list applied tags request: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: list applied tags request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clickfunnels: list applied tags: status %d body=%s", resp.StatusCode, string(body))
	}

	var tags []AppliedTag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode applied tags response: %w", err)
	}
	return tags, nil
}

// AddAppliedTag applies a single tag to a contact and returns the applied-tag record from CF.
func (c *Client) AddAppliedTag(ctx context.Context, contactID int, tagID int) (*AppliedTag, error) {
	endpoint := fmt.Sprintf("%s/contacts/%d/applied_tags", c.baseURL(), contactID)

	body, err := json.Marshal(map[string]any{"contacts_applied_tag": map[string]any{"tag_id": tagID}})
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: marshal add applied tag body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build add applied tag request: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: add applied tag request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clickfunnels: add applied tag (contact=%d tag=%d): status %d body=%s", contactID, tagID, resp.StatusCode, string(b))
	}

	var applied AppliedTag
	if err := json.NewDecoder(resp.Body).Decode(&applied); err != nil {
		// Non-fatal: CF may return 201 with no body on duplicate; return empty struct.
		return &AppliedTag{TagID: tagID}, nil
	}
	return &applied, nil
}

// RemoveAppliedTag removes an applied-tag record by its applied-tag ID (not the tag ID).
func (c *Client) RemoveAppliedTag(ctx context.Context, contactID int, appliedTagID int) error {
	endpoint := fmt.Sprintf("%s/contacts/%d/applied_tags/%d", c.baseURL(), contactID, appliedTagID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("clickfunnels: build remove applied tag request: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("clickfunnels: remove applied tag request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clickfunnels: remove applied tag (contact=%d applied=%d): status %d body=%s", contactID, appliedTagID, resp.StatusCode, string(b))
	}
	return nil
}

// SearchContactsByEmail searches for contacts whose email address contains the query string.
// It passes filter[email_address] to the API (which may do prefix/exact matching server-side)
// and additionally filters results client-side with a case-insensitive contains check.
// maxResults caps the returned slice (0 = no cap).
func (c *Client) SearchContactsByEmail(ctx context.Context, emailQuery string, maxResults int) ([]Contact, error) {
	endpoint := fmt.Sprintf("%s/workspaces/%s/contacts", c.baseURL(), c.WorkspaceID)
	params := url.Values{}
	params.Set("filter[email_address]", emailQuery)
	params.Set("per_page", "100")
	endpoint = endpoint + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build search contacts request: %w", err)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: search contacts request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("clickfunnels: search contacts: status %d body=%s", resp.StatusCode, string(body))
	}

	var contacts []Contact
	if err := json.NewDecoder(resp.Body).Decode(&contacts); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode search contacts response: %w", err)
	}

	// Client-side partial match filter (case-insensitive) in case the API does exact-only matching.
	lower := strings.ToLower(emailQuery)
	var matched []Contact
	for _, c := range contacts {
		if c.EmailAddress != nil && strings.Contains(strings.ToLower(*c.EmailAddress), lower) {
			matched = append(matched, c)
			if maxResults > 0 && len(matched) >= maxResults {
				break
			}
		}
	}
	return matched, nil
}

// CreateContact creates a new contact in the workspace.
func (c *Client) CreateContact(ctx context.Context, params ContactParams) (*Contact, error) {
	endpoint := fmt.Sprintf("%s/workspaces/%s/contacts", c.baseURL(), c.WorkspaceID)

	body, err := json.Marshal(map[string]ContactParams{"contact": params})
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: marshal create contact body: %w", err)
	}

	log.Printf("[cf] POST %s body=%s", endpoint, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build create contact request: %w", err)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: create contact request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("clickfunnels: create contact: unexpected status %d", resp.StatusCode)
	}

	var contact Contact
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode create contact response: %w", err)
	}
	return &contact, nil
}

// UpdateContact updates an existing contact by its numeric ID.
func (c *Client) UpdateContact(ctx context.Context, id int, params ContactParams) (*Contact, error) {
	endpoint := fmt.Sprintf("%s/contacts/%d", c.baseURL(), id)

	body, err := json.Marshal(map[string]ContactParams{"contact": params})
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: marshal update contact body: %w", err)
	}

	log.Printf("[cf] PUT %s body=%s", endpoint, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build update contact request: %w", err)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: update contact request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[cf] PUT %s response status=%d body=%s", endpoint, resp.StatusCode, respBody)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("clickfunnels: update contact %d: unexpected status %d body=%s", id, resp.StatusCode, respBody)
	}

	var contact Contact
	if err := json.Unmarshal(respBody, &contact); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode update contact response: %w", err)
	}
	return &contact, nil
}

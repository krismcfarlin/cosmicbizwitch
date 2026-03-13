// Package clickfunnels provides an API client for the ClickFunnels 2.0 REST API.
package clickfunnels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Config holds the credentials and workspace settings for the ClickFunnels API.
type Config struct {
	APIKey      string
	Subdomain   string
	WorkspaceID int
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
// tagIDs filters results to contacts with all specified tag IDs.
func (c *Client) ListContacts(ctx context.Context, afterID int, tagIDs []int) ([]Contact, error) {
	endpoint := fmt.Sprintf("%s/workspaces/%d/contacts", c.baseURL(), c.WorkspaceID)

	params := url.Values{}
	if afterID > 0 {
		params.Set("after", strconv.Itoa(afterID))
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
		return nil, fmt.Errorf("clickfunnels: list contacts request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("clickfunnels: list contacts: unexpected status %d", resp.StatusCode)
	}

	var contacts []Contact
	if err := json.NewDecoder(resp.Body).Decode(&contacts); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode list contacts response: %w", err)
	}
	return contacts, nil
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

// CreateContact creates a new contact in the workspace.
func (c *Client) CreateContact(ctx context.Context, params ContactParams) (*Contact, error) {
	endpoint := fmt.Sprintf("%s/workspaces/%d/contacts", c.baseURL(), c.WorkspaceID)

	body, err := json.Marshal(map[string]ContactParams{"contact": params})
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: marshal create contact body: %w", err)
	}

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: build update contact request: %w", err)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("clickfunnels: update contact request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("clickfunnels: update contact %d: unexpected status %d", id, resp.StatusCode)
	}

	var contact Contact
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return nil, fmt.Errorf("clickfunnels: decode update contact response: %w", err)
	}
	return &contact, nil
}

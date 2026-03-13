package clickfunnels

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	apiKey := os.Getenv("CF_API_KEY")
	if apiKey == "" {
		apiKey = "g-wmRl3iXx9x6TIklIxODr0A8KoDGqw0TXoowJpJ2Rc"
	}
	if os.Getenv("CF_API_KEY") == "" && apiKey == "g-wmRl3iXx9x6TIklIxODr0A8KoDGqw0TXoowJpJ2Rc" {
		// allow running with the hardcoded key; skip only if explicitly unset with no fallback
	}

	subdomain := os.Getenv("CF_SUBDOMAIN")
	if subdomain == "" {
		subdomain = "cosmicbizwitchllc"
	}

	workspaceID := 39498
	if raw := os.Getenv("CF_WORKSPACE_ID"); raw != "" {
		if id, err := strconv.Atoi(raw); err == nil {
			workspaceID = id
		}
	}

	return NewClient(Config{
		APIKey:      apiKey,
		Subdomain:   subdomain,
		WorkspaceID: workspaceID,
	})
}

func testCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func TestListContacts(t *testing.T) {
	if os.Getenv("CF_API_KEY") == "" {
		t.Skip("CF_API_KEY not set")
	}
	client := testClient(t)
	ctx, cancel := testCtx()
	defer cancel()

	contacts, err := client.ListContacts(ctx, 0, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(contacts), 1, "expected at least one contact")
	assert.NotEmpty(t, contacts[0].EmailAddress, "first contact should have an email address")
}

func TestGetContact(t *testing.T) {
	if os.Getenv("CF_API_KEY") == "" {
		t.Skip("CF_API_KEY not set")
	}
	client := testClient(t)
	ctx, cancel := testCtx()
	defer cancel()

	contact, err := client.GetContact(ctx, 1166841989)
	require.NoError(t, err)
	require.NotNil(t, contact)
	assert.Equal(t, 1166841989, contact.ID)
	require.NotNil(t, contact.EmailAddress)
	assert.Equal(t, "poppinsfresh+bd23@gmail.com", *contact.EmailAddress)
}

func TestCreateContact(t *testing.T) {
	if os.Getenv("CF_API_KEY") == "" {
		t.Skip("CF_API_KEY not set")
	}
	client := testClient(t)
	ctx, cancel := testCtx()
	defer cancel()

	email := strPtr(fmt.Sprintf("cftest+%d@cosmicbizwitch.com", time.Now().Unix()))
	firstName := strPtr("CFTest")
	lastName := strPtr("Integration")

	contact, err := client.CreateContact(ctx, ContactParams{
		EmailAddress: email,
		FirstName:    firstName,
		LastName:     lastName,
	})
	require.NoError(t, err)
	require.NotNil(t, contact)
	assert.NotZero(t, contact.ID, "created contact should have a non-zero ID")
	require.NotNil(t, contact.EmailAddress)
	assert.Equal(t, *email, *contact.EmailAddress)
}

func TestUpdateContact(t *testing.T) {
	if os.Getenv("CF_API_KEY") == "" {
		t.Skip("CF_API_KEY not set")
	}
	client := testClient(t)
	ctx, cancel := testCtx()
	defer cancel()

	websiteURL := strPtr("https://example.com/test")

	contact, err := client.UpdateContact(ctx, 1166841989, ContactParams{
		WebsiteURL: websiteURL,
	})
	require.NoError(t, err)
	require.NotNil(t, contact)
	require.NotNil(t, contact.WebsiteURL)
	assert.Equal(t, "https://example.com/test", *contact.WebsiteURL)
}

func strPtr(s string) *string { return &s }

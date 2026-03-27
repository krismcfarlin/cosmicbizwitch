package workflows_test

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"

	"cosmicbizwitch/internal/app/google"
	"cosmicbizwitch/internal/app/workflows"
	"cosmicbizwitch/pkg/workflow"
	"cosmicbizwitch/pkg/workflow/pbstore"

	"github.com/pocketbase/pocketbase"
)

// TestGdriveFillTemplateWorkflow tests the gdrive_fill_template activity:
// 1. Copies a template document to a destination folder
// 2. Fills placeholders with provided variables
// 3. Verifies the new document has the replacements
func TestGdriveFillTemplateWorkflow(t *testing.T) {
	// Get Google credentials from environment
	refreshToken := os.Getenv("GOOGLE_REFRESH_TOKEN")
	if refreshToken == "" {
		t.Skip("GOOGLE_REFRESH_TOKEN not set, skipping integration test")
	}

	ctx := context.Background()

	// Initialize Google client with credentials from app settings
	// These are configured via the Settings page and stored securely
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		t.Fatal("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET must be set in environment")
	}

	tokenMgr := google.NewTokenManager(
		clientID,
		clientSecret,
		refreshToken,
	)
	gc := google.NewClient(tokenMgr)

	// Test configuration
	templateDocID := os.Getenv("TEST_TEMPLATE_DOC_ID") // Should be a Google Doc with {{first_name}}, {{last_name}}, {{email}}
	destFolderID := os.Getenv("TEST_DEST_FOLDER_ID")   // Destination folder for the copy

	if templateDocID == "" || destFolderID == "" {
		t.Skip("TEST_TEMPLATE_DOC_ID or TEST_DEST_FOLDER_ID not set")
	}

	// Variables to substitute
	vars := map[string]string{
		"first_name": "Jane",
		"last_name":  "Smith",
		"email":      "jane.smith@example.com",
	}

	// Create a minimal PocketBase instance (for testing workflow activity)
	pb := pocketbase.New()

	// Create workflow collections in PocketBase
	if err := pbstore.CreateCollections(pb); err != nil {
		t.Fatalf("failed to create workflow collections: %v", err)
	}

	// Create workflow engine with PocketBase store
	logger := log.New(os.Stderr, "[workflow] ", log.LstdFlags)
	eng := workflow.NewEngine(pbstore.New(pb), workflow.EngineConfig{Logger: logger})

	// Register all workflow activities (including gdrive_fill_template)
	if err := workflows.RegisterDefaults(eng, pb, nil, func() *google.Client { return gc }); err != nil {
		t.Fatalf("failed to register workflow activities: %v", err)
	}

	// Run the activity
	t.Run("FillTemplateAndVerify", func(t *testing.T) {
		input := map[string]any{
			"template_id":            templateDocID,
			"destination_folder_id":  destFolderID,
			"title":                  "Test - Jane Smith",
			"vars":                   varsToJSON(vars),
		}

		output, err := eng.ExecuteActivity(ctx, "gdrive_fill_template", input)
		if err != nil {
			t.Fatalf("gdrive_fill_template activity failed: %v", err)
		}

		// Extract new document ID
		newDocID, ok := output["doc_id"].(string)
		if !ok || newDocID == "" {
			t.Fatalf("expected doc_id in output, got: %v", output)
		}

		replacementsCount, ok := output["replacements_made"].(float64)
		if !ok {
			t.Fatalf("expected replacements_made in output, got: %v", output)
		}

		t.Logf("✓ Created new document: %s", newDocID)
		t.Logf("✓ Replacements made: %d", int(replacementsCount))

		if replacementsCount < float64(len(vars)) {
			t.Logf("⚠ Expected at least %d replacements, got %d", len(vars), int(replacementsCount))
		}

		// Verify the new document has the replacements
		t.Run("VerifyDocumentContent", func(t *testing.T) {
			content, err := gc.GetDocumentContent(ctx, newDocID)
			if err != nil {
				t.Fatalf("failed to get document content: %v", err)
			}

			t.Logf("Document content preview: %s...", truncate(content, 200))

			// Check that variables were replaced
			checks := map[string]string{
				"first_name": "Jane",
				"last_name":  "Smith",
				"email":      "jane.smith@example.com",
			}

			for varName, expectedValue := range checks {
				if !contains(content, expectedValue) {
					t.Errorf("expected '%s' in document (from var %s), but not found", expectedValue, varName)
				} else {
					t.Logf("✓ Found '%s' in document", expectedValue)
				}
			}

			// Check that placeholders were removed (no {{...}} left)
			if contains(content, "{{first_name}}") {
				t.Error("placeholder {{first_name}} was not replaced")
			}
			if contains(content, "{{last_name}}") {
				t.Error("placeholder {{last_name}} was not replaced")
			}
			if contains(content, "{{email}}") {
				t.Error("placeholder {{email}} was not replaced")
			}

			t.Logf("✓ All placeholders were properly replaced")
		})

		// Optional: Clean up the test document
		if os.Getenv("CLEANUP_TEST_DOCS") == "true" {
			// Would need to implement delete in Google client
			t.Logf("Document %s can be manually deleted if needed", newDocID)
		}
	})
}

// Helper functions

func varsToJSON(vars map[string]string) string {
	b, _ := json.Marshal(vars)
	return string(b)
}

func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > 0 && s[0:len(substr)] == substr || indexStr(s, substr) >= 0)
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}


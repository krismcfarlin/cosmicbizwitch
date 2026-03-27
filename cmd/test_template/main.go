package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"cosmicbizwitch/internal/app/google"
)

func main() {
	ctx := context.Background()

	// Get credentials from environment or set via flags
	clientID := "YOUR_GOOGLE_CLIENT_ID"
	clientSecret := "YOUR_GOOGLE_CLIENT_SECRET"
	refreshToken := "YOUR_GOOGLE_REFRESH_TOKEN"

	tokenMgr := google.NewTokenManager(
		clientID,
		clientSecret,
		refreshToken,
	)
	gc := google.NewClient(tokenMgr)

	templateID := "11MRPXqn3UYep7VYQ521Bd0tE6l9aXgcZAuQlMhgBY_c"
	destFolderID := "1FUksSTIv0usjsaHU7J4Vngl7GP1C49kn" // Lauren folder (same as template)

	vars := map[string]string{
		"about_your_business": "test",
		"money_share":         "test1",
		"moon_code":           "test2",
		"mars_code":           "test3",
		"mercury_code":        "test4",
		"jupiter_code":        "test5",
		"venus_code":          "test6",
		"saturn_code":         "test7",
		"sun_code":            "test9",
	}

	fmt.Println("Copying template...")
	docID, webURL, err := gc.CopyFile(ctx, templateID, "TEST - CC for RRwinter", destFolderID)
	if err != nil {
		log.Fatalf("CopyFile failed: %v", err)
	}
	fmt.Printf("Created doc: %s\nURL: %s\n", docID, webURL)

	fmt.Printf("Replacing %d variables...\n", len(vars))
	count, err := gc.ReplaceAllText(ctx, docID, vars)
	if err != nil {
		log.Fatalf("ReplaceAllText failed: %v", err)
	}
	fmt.Printf("Replacements made: %d\n", count)

	fmt.Println("\nFetching document content...")
	content, err := gc.GetDocContent(ctx, docID, "application/vnd.google-apps.document")
	if err != nil {
		log.Fatalf("GetDocContent failed: %v", err)
	}

	// Show first 500 chars and check for remaining placeholders
	preview := content
	if len(preview) > 800 {
		preview = preview[:800]
	}
	fmt.Printf("\n--- Content Preview ---\n%s\n---\n", preview)

	// Check for unreplaced placeholders
	remaining := findPlaceholders(content)
	if len(remaining) > 0 {
		b, _ := json.MarshalIndent(remaining, "", "  ")
		fmt.Printf("\nUnreplaced placeholders still in doc:\n%s\n", b)
	} else {
		fmt.Println("\nAll provided placeholders were replaced!")
	}

	fmt.Printf("\nDoc URL: %s\n", webURL)
}

func findPlaceholders(content string) []string {
	var found []string
	i := 0
	for i < len(content)-1 {
		if content[i] == '{' && content[i+1] == '{' {
			end := i + 2
			for end < len(content)-1 {
				if content[end] == '}' && content[end+1] == '}' {
					found = append(found, content[i:end+2])
					i = end + 2
					break
				}
				end++
			}
			if end >= len(content)-1 {
				break
			}
		} else {
			i++
		}
	}
	return found
}

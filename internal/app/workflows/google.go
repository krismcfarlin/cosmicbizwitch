package workflows

import (
	"context"
	"encoding/json"
	"fmt"

	googleapp "cosmicbizwitch/internal/app/google"
	"cosmicbizwitch/pkg/workflow"
)

// registerGoogleActivities registers all Google Drive and Docs activity nodes.
func registerGoogleActivities(eng *workflow.Engine, getGoogle func() *googleapp.Client) {
	// ── gdrive_get_file ───────────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("gdrive_get_file",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}
			fileID := gdriveString(input, "file_id")
			if fileID == "" {
				return nil, fmt.Errorf("gdrive_get_file: file_id is required")
			}
			f, err := gc.GetFile(ctx, fileID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_get_file: %w", err)
			}
			content, err := gc.GetDocContent(ctx, fileID, f.MimeType)
			if err != nil {
				return nil, fmt.Errorf("gdrive_get_file: fetch content: %w", err)
			}
			// Store content under result_var if set, otherwise under "content".
			contentKey := gdriveString(input, "result_var")
			if contentKey == "" {
				contentKey = "content"
			}
			return map[string]any{
				"file_id":   fileID,
				"doc_id":    fileID, // alias used by gdrive_fill_template and gdrive_export_pdf
				"file_name": f.Name,
				"file_url":  f.WebViewLink,
				"mime_type": f.MimeType,
				contentKey:  content,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Downloads a Google Drive file as markdown and puts the content into the workflow context.",
			InputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "gdrive_file", Required: true, Description: "Google Drive file — use the Browse button to pick."},
				{Name: "result_var", Type: "string", Description: "Context key to store content under (default: content)."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "string", Description: "Google Drive file ID."},
				{Name: "doc_id", Type: "string", Description: "Alias for file_id — use with gdrive_fill_template and gdrive_export_pdf."},
				{Name: "file_name", Type: "string", Description: "File name."},
				{Name: "file_url", Type: "string", Description: "Web view URL."},
				{Name: "mime_type", Type: "string", Description: "MIME type."},
				{Name: "content", Type: "string", Description: "File contents as markdown (or result_var if set)."},
			},
		},
	)

	// ── gdrive_copy_template ─────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("gdrive_copy_template",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}
			templateID := gdriveString(input, "template_id")
			if templateID == "" {
				return nil, fmt.Errorf("gdrive_copy_template: template_id is required")
			}
			folderID := gdriveString(input, "destination_folder_id")
			if folderID == "" {
				return nil, fmt.Errorf("gdrive_copy_template: destination_folder_id is required")
			}
			title := gdriveString(input, "title")
			if title == "" {
				// Fall back to original file's name.
				orig, err := gc.GetFile(ctx, templateID)
				if err != nil {
					return nil, fmt.Errorf("gdrive_copy_template: fetch original title: %w", err)
				}
				title = orig.Name
			}

			newID, webURL, err := gc.CopyFile(ctx, templateID, title, folderID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_copy_template: %w", err)
			}
			return map[string]any{
				"doc_id":    newID,
				"doc_url":   webURL,
				"doc_title": title,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Copies a Google Doc template to a destination folder, leaving the original untouched.",
			InputFields: []workflow.FieldMeta{
				{Name: "template_id", Type: "gdrive_file", Required: true, Description: "The Google Doc to copy — use Browse to pick."},
				{Name: "destination_folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder for the copy — use Browse to pick."},
				{Name: "title", Type: "string", Description: "Name for the new document. Supports {{key}} interpolation. Defaults to the original document name."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Description: "Drive ID of the new document copy."},
				{Name: "doc_url", Type: "string", Description: "Web view URL of the new document."},
				{Name: "doc_title", Type: "string", Description: "Title used for the new document."},
			},
		},
	)

	// ── gdrive_fill_template ─────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("gdrive_fill_template",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}
			templateID := gdriveString(input, "template_id")
			if templateID == "" {
				return nil, fmt.Errorf("gdrive_fill_template: template_id is required")
			}
			folderID := gdriveString(input, "destination_folder_id")
			if folderID == "" {
				return nil, fmt.Errorf("gdrive_fill_template: destination_folder_id is required")
			}
			title := gdriveString(input, "title")
			if title == "" {
				// Fall back to original file's name.
				orig, err := gc.GetFile(ctx, templateID)
				if err != nil {
					return nil, fmt.Errorf("gdrive_fill_template: fetch original title: %w", err)
				}
				title = orig.Name
			}

			// Copy template to destination folder.
			fmt.Printf("[gdrive_fill_template] Copying template %s to folder %s\n", templateID, folderID)
			docID, webURL, err := gc.CopyFile(ctx, templateID, title, folderID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_fill_template: copy template: %w", err)
			}
			fmt.Printf("[gdrive_fill_template] Created new document: %s\n", docID)

			// Build the replacement map.
			vars := make(map[string]string)

			varsRaw := gdriveString(input, "vars")
			if varsRaw != "" {
				// User provided explicit JSON overrides (string form).
				if err := json.Unmarshal([]byte(varsRaw), &vars); err != nil {
					return nil, fmt.Errorf("gdrive_fill_template: vars is not valid JSON: %w", err)
				}
			} else if varsMap, ok := input["vars"].(map[string]any); ok {
				// execute-node debug panel coerces JSON strings to native maps — handle that here.
				for k, v := range varsMap {
					if s, ok := v.(string); ok {
						vars[k] = s
					}
				}
			} else {
				// Use all string-valued keys from the workflow context, excluding internals.
				skip := map[string]bool{
					"template_id": true, "destination_folder_id": true, "title": true, "vars": true,
				}
				for k, v := range input {
					if len(k) > 0 && k[0] == '_' {
						continue // skip _source, _trigger, etc.
					}
					if skip[k] {
						continue
					}
					if s, ok := v.(string); ok {
						vars[k] = s
					}
				}
			}

			fmt.Printf("[gdrive_fill_template] Replacing %d variables in document\n", len(vars))
			for k, v := range vars {
				fmt.Printf("  {{%s}} -> %s\n", k, v)
			}
			count, err := gc.ReplaceAllText(ctx, docID, vars)
			if err != nil {
				return nil, fmt.Errorf("gdrive_fill_template: fill template: %w", err)
			}
			fmt.Printf("[gdrive_fill_template] Completed - %d replacements made\n", count)
			return map[string]any{
				"doc_id":            docID,
				"doc_url":           webURL,
				"doc_title":         title,
				"replacements_made": count,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Copies a Google Doc template to a destination folder and replaces {{variable}} placeholders using provided vars.",
			InputFields: []workflow.FieldMeta{
				{Name: "template_id", Type: "gdrive_file", Required: true, Description: "The Google Doc to copy — use Browse to pick."},
				{Name: "destination_folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder for the copy — use Browse to pick."},
				{Name: "title", Type: "string", Description: "Name for the new document. Supports {{key}} interpolation. Defaults to the original document name."},
				{Name: "vars", Type: "json", Description: "JSON object mapping placeholder name to value. Engine interpolates {{ctx_key}} inside this string."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Description: "Drive ID of the new document copy."},
				{Name: "doc_url", Type: "string", Description: "Web view URL of the new document."},
				{Name: "doc_title", Type: "string", Description: "Title used for the new document."},
				{Name: "replacements_made", Type: "number", Description: "Total number of placeholder occurrences replaced."},
			},
		},
	)

	// ── gdrive_export_pdf ────────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("gdrive_export_pdf",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}
			docID := gdriveString(input, "doc_id")
			if docID == "" {
				return nil, fmt.Errorf("gdrive_export_pdf: doc_id is required")
			}
			folderID := gdriveString(input, "destination_folder_id")
			if folderID == "" {
				return nil, fmt.Errorf("gdrive_export_pdf: destination_folder_id is required")
			}
			filename := gdriveString(input, "filename")
			if filename == "" {
				// Default to the document's title + ".pdf".
				f, err := gc.GetFile(ctx, docID)
				if err != nil {
					return nil, fmt.Errorf("gdrive_export_pdf: fetch doc title: %w", err)
				}
				filename = f.Name + ".pdf"
			}

			pdf, err := gc.ExportAndSavePDF(ctx, docID, folderID, filename)
			if err != nil {
				return nil, fmt.Errorf("gdrive_export_pdf: %w", err)
			}
			return map[string]any{
				"pdf_id":       pdf.ID,
				"pdf_url":      pdf.WebViewLink,
				"pdf_filename": pdf.Name,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Exports a Google Doc as a PDF and saves it to a Drive folder.",
			InputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Required: true, Description: "ID of the Google Doc to export. Pipe in {{doc_id}} from earlier steps."},
				{Name: "destination_folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder to save the PDF into — use Browse to pick."},
				{Name: "filename", Type: "string", Description: "PDF filename. Supports {{key}} interpolation. Defaults to document title + \".pdf\"."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "pdf_id", Type: "string", Description: "Drive ID of the saved PDF."},
				{Name: "pdf_url", Type: "string", Description: "Web view URL of the PDF."},
				{Name: "pdf_filename", Type: "string", Description: "Filename of the saved PDF."},
			},
		},
	)

	// ── convert_to_pdf ──────────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("convert_to_pdf",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}
			docID := gdriveString(input, "doc_id")
			if docID == "" {
				return nil, fmt.Errorf("convert_to_pdf: doc_id is required")
			}
			folderID := gdriveString(input, "folder_id")
			if folderID == "" {
				return nil, fmt.Errorf("convert_to_pdf: folder_id is required")
			}
			filename := gdriveString(input, "filename")
			if filename == "" {
				// Default to the document's title + ".pdf".
				f, err := gc.GetFile(ctx, docID)
				if err != nil {
					return nil, fmt.Errorf("convert_to_pdf: fetch doc title: %w", err)
				}
				filename = f.Name + ".pdf"
			}

			fmt.Printf("[convert_to_pdf] Converting doc %s to PDF\n", docID)
			pdf, err := gc.ExportAndSavePDF(ctx, docID, folderID, filename)
			if err != nil {
				return nil, fmt.Errorf("convert_to_pdf: %w", err)
			}
			fmt.Printf("[convert_to_pdf] PDF saved: %s\n", pdf.Name)
			return map[string]any{
				"pdf_id":       pdf.ID,
				"pdf_url":      pdf.WebViewLink,
				"pdf_filename": pdf.Name,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Converts a Google Doc to PDF and saves it to a Drive folder.",
			InputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Required: true, Description: "ID of the Google Doc to convert. Pipe in {{doc_id}} from earlier steps."},
				{Name: "folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder to save the PDF into — use Browse to pick."},
				{Name: "filename", Type: "string", Description: "PDF filename. Supports {{key}} interpolation. Defaults to document title + \".pdf\"."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "pdf_id", Type: "string", Description: "Drive ID of the saved PDF."},
				{Name: "pdf_url", Type: "string", Description: "Web view URL of the PDF."},
				{Name: "pdf_filename", Type: "string", Description: "Filename of the saved PDF."},
			},
		},
	)

	// ── gdrive_share_file ────────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("gdrive_share_file",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}
			fileID := gdriveString(input, "file_id")
			if fileID == "" {
				return nil, fmt.Errorf("gdrive_share_file: file_id is required")
			}
			email := gdriveString(input, "email")
			if email == "" {
				// No email provided — skip silently.
				return map[string]any{"shared": false}, nil
			}
			role := gdriveString(input, "role")
			if role == "" {
				role = "reader"
			}

			permID, err := gc.ShareFile(ctx, fileID, email, role)
			if err != nil {
				return nil, fmt.Errorf("gdrive_share_file: %w", err)
			}
			return map[string]any{
				"shared":        true,
				"permission_id": permID,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Silently shares a Google Drive file with an email address (no notification sent).",
			InputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "string", Required: true, Description: "Drive file or folder ID to share. Pipe in {{pdf_id}} or {{doc_id}}."},
				{Name: "email", Type: "string", Description: "Email address to share with. Leave blank to skip sharing."},
				{Name: "role", Type: "string", Options: []string{"reader", "writer", "commenter"}, Description: "Permission level to grant. Defaults to reader."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "shared", Type: "boolean", Description: "True if sharing was performed."},
				{Name: "permission_id", Type: "string", Description: "Google permission ID (present when shared is true)."},
			},
		},
	)

	// ── gdrive_create_doc ────────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("gdrive_create_doc",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}
			folderID := gdriveString(input, "folder_id")
			if folderID == "" {
				return nil, fmt.Errorf("gdrive_create_doc: folder_id is required")
			}
			title := gdriveString(input, "title")
			if title == "" {
				return nil, fmt.Errorf("gdrive_create_doc: title is required")
			}
			content := gdriveString(input, "content")

			f, err := gc.CreateDoc(ctx, title, content, folderID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_create_doc: %w", err)
			}
			return map[string]any{
				"doc_id":  f.ID,
				"doc_url": f.WebViewLink,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Creates a new Google Doc in a Drive folder with content from the workflow context.",
			InputFields: []workflow.FieldMeta{
				{Name: "folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder to create the document in — use Browse to pick."},
				{Name: "title", Type: "string", Required: true, Description: "Document title. Supports {{key}} interpolation."},
				{Name: "content", Type: "string", Description: "Text content for the document. Use {{var_name}} to pull from a context variable (e.g. {{filled_content}})."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Description: "Drive ID of the created document."},
				{Name: "doc_url", Type: "string", Description: "Web view URL of the created document."},
			},
		},
	)
}

// gdriveString extracts a string value from the activity input map.
func gdriveString(input map[string]any, key string) string {
	if v, ok := input[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

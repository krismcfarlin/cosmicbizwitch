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
			out := map[string]any{
				"file_name": f.Name,
				"file_url":  f.WebViewLink,
				"mime_type": f.MimeType,
				"is_folder": f.IsFolder,
			}

			// If result_var is set, fetch the file content and store it.
			resultVar := gdriveString(input, "result_var")
			if resultVar != "" {
				content, err := gc.GetDocContent(ctx, fileID, f.MimeType)
				if err != nil {
					return nil, fmt.Errorf("gdrive_get_file: fetch content: %w", err)
				}
				out[resultVar] = content
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Fetches a Google Drive file's metadata and optionally its text content into a context variable.",
			InputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "gdrive_file", Required: true, Description: "Google Drive file — use the Browse button to pick."},
				{Name: "result_var", Type: "string", Description: "If set, fetches the file's text content and stores it under this variable name (e.g. doc_content)."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "file_name", Type: "string", Description: "File or folder name."},
				{Name: "file_url", Type: "string", Description: "Web view URL."},
				{Name: "mime_type", Type: "string", Description: "MIME type."},
				{Name: "is_folder", Type: "boolean", Description: "True if this is a folder."},
				{Name: "<result_var>", Type: "string", Description: "Text content of the file, stored under the name you provide in result_var."},
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
			docID := gdriveString(input, "doc_id")
			if docID == "" {
				return nil, fmt.Errorf("gdrive_fill_template: doc_id is required")
			}

			// Build the replacement map.
			vars := make(map[string]string)

			varsRaw := gdriveString(input, "vars")
			if varsRaw != "" {
				// User provided explicit JSON overrides.
				if err := json.Unmarshal([]byte(varsRaw), &vars); err != nil {
					return nil, fmt.Errorf("gdrive_fill_template: vars is not valid JSON: %w", err)
				}
			} else {
				// Use all string-valued keys in the workflow context, excluding internals.
				skip := map[string]bool{
					"doc_id": true, "vars": true,
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

			count, err := gc.ReplaceAllText(ctx, docID, vars)
			if err != nil {
				return nil, fmt.Errorf("gdrive_fill_template: %w", err)
			}
			return map[string]any{
				"doc_id":            docID,
				"replacements_made": count,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Replaces {{variable}} placeholders in a Google Doc using workflow context values.",
			InputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Required: true, Description: "ID of the Google Doc to fill in. Pipe in {{doc_id}} from gdrive_copy_template."},
				{Name: "vars", Type: "string", Description: "Optional JSON object of {placeholder: value} overrides. If omitted, all string values from the workflow context are used."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Description: "Pass-through document ID."},
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

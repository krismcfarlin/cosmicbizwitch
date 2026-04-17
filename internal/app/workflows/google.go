package workflows

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	googleapp "cosmicbizwitch/internal/app/google"
	"cosmicbizwitch/pkg/workflow"
)

// googleDocIDRe extracts the document ID from a Google Docs URL.
var googleDocIDRe = regexp.MustCompile(`/document/d/([a-zA-Z0-9_-]+)`)

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
			vars, err := buildFillTemplateVars(input)
			if err != nil {
				return nil, fmt.Errorf("gdrive_fill_template: %w", err)
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

			// Process image_vars if provided.
			imagesReplaced := 0
			if rawImageVars := input["image_vars"]; rawImageVars != nil {
				var imageVars map[string]string
				switch iv := rawImageVars.(type) {
				case map[string]any:
					imageVars = make(map[string]string, len(iv))
					for k, v := range iv {
						if s, ok := v.(string); ok {
							imageVars[k] = s
						}
					}
				case string:
					if iv != "" {
						var parsed map[string]string
						if err := json.Unmarshal([]byte(iv), &parsed); err == nil {
							imageVars = parsed
						}
					}
				}
				if len(imageVars) > 0 {
					n, err := gc.ReplaceImages(ctx, docID, imageVars)
					if err != nil {
						log.Printf("[gdrive_fill_template] image replacement failed: %v", err)
					} else {
						imagesReplaced = n
						fmt.Printf("[gdrive_fill_template] Replaced %d images\n", n)
					}
				}
			}
			return map[string]any{
				"doc_id":            docID,
				"doc_url":           webURL,
				"doc_title":         title,
				"replacements_made": count,
				"images_replaced":   imagesReplaced,
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
					{Name: "image_vars", Type: "json", Description: "JSON object mapping alt-text key to image URL. In the template doc, set each placeholder image's alt text to the key name. The node replaces those images with the resolved URLs. Example: {\"chart_url\": \"{{chart_url}}\"}"},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Description: "Drive ID of the new document copy."},
				{Name: "doc_url", Type: "string", Description: "Web view URL of the new document."},
				{Name: "doc_title", Type: "string", Description: "Title used for the new document."},
				{Name: "replacements_made", Type: "number", Description: "Total number of placeholder occurrences replaced."},
				{Name: "images_replaced", Type: "number", Description: "Number of placeholder images replaced."},
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
				// Accept a full Google Docs URL and extract the doc ID from it.
				if u := gdriveString(input, "doc_url"); u != "" {
					// URL form: https://docs.google.com/document/d/{ID}/...
					if m := googleDocIDRe.FindStringSubmatch(u); len(m) > 1 {
						docID = m[1]
					}
				}
			}
			if docID == "" {
				return nil, fmt.Errorf("convert_to_pdf: doc_id or doc_url is required")
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

			visibility := gdriveString(input, "visibility")

			fmt.Printf("[convert_to_pdf] Converting doc %s to PDF\n", docID)
			pdf, err := gc.ExportAndSavePDF(ctx, docID, folderID, filename)
			if err != nil {
				return nil, fmt.Errorf("convert_to_pdf: %w", err)
			}
			fmt.Printf("[convert_to_pdf] PDF saved: %s\n", pdf.Name)

			if visibility == "public" {
				if err := gc.MakePublic(ctx, pdf.ID); err != nil {
					return nil, fmt.Errorf("convert_to_pdf: make public: %w", err)
				}
			}

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
				{Name: "doc_id", Type: "string", Description: "ID of the Google Doc to convert. Use this OR doc_url — one is required."},
				{Name: "doc_url", Type: "string", Description: "Full Google Docs URL (e.g. {{doc_url}} from gdrive_fill_template). The doc ID is extracted automatically."},
				{Name: "folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder to save the PDF into — use Browse to pick."},
				{Name: "filename", Type: "string", Description: "PDF filename. Supports {{key}} interpolation. Defaults to document title + \".pdf\"."},
				{Name: "visibility", Type: "string", Options: []string{"private", "public"}, Description: "Set to 'public' to make the PDF accessible to anyone with the link. Defaults to private."},
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
			out := map[string]any{
				"doc_id":  f.ID,
				"doc_url": f.WebViewLink,
			}
			makePublic, _ := input["make_public"].(bool)
			if !makePublic {
				if s, ok := input["make_public"].(string); ok {
					makePublic = s == "true"
				}
			}
			if makePublic {
				if err := gc.MakePublic(ctx, f.ID); err != nil {
					log.Printf("[gdrive_create_doc] make public failed: %v", err)
				} else {
					out["public_url"] = "https://drive.google.com/uc?id=" + f.ID
				}
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Creates a new Google Doc in a Drive folder with content from the workflow context.",
			InputFields: []workflow.FieldMeta{
				{Name: "folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder to create the document in — use Browse to pick."},
				{Name: "title", Type: "string", Required: true, Description: "Document title. Supports {{key}} interpolation."},
				{Name: "content", Type: "string", Description: "Text content for the document. Use {{var_name}} to pull from a context variable (e.g. {{filled_content}})."},
				{Name: "make_public", Type: "boolean", Description: "If true, grants anyone-with-link read access and returns a public_url."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "doc_id", Type: "string", Description: "Drive ID of the created document."},
				{Name: "doc_url", Type: "string", Description: "Web view URL of the created document."},
				{Name: "public_url", Type: "string", Description: "Direct public URL (only set when make_public is true)."},
			},
		},
	)

	// ── gdrive_save_file ─────────────────────────────────────────────────────

	eng.RegisterActivityWithMeta("gdrive_save_file",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}
			folderID := gdriveString(input, "folder_id")
			if folderID == "" {
				return nil, fmt.Errorf("gdrive_save_file: folder_id is required")
			}
			filename := gdriveString(input, "filename")
			if filename == "" {
				return nil, fmt.Errorf("gdrive_save_file: filename is required")
			}
			content := gdriveString(input, "content")
			if content == "" {
				return nil, fmt.Errorf("gdrive_save_file: content is required")
			}
			mimeType := gdriveString(input, "mime_type")
			if mimeType == "" {
				mimeType = "text/plain"
			}

			// Decode base64 if explicitly requested via content_encoding field.
			contentBytes := []byte(content)
			if enc := gdriveString(input, "content_encoding"); enc == "base64" {
				if decoded, err := base64.StdEncoding.DecodeString(content); err == nil {
					contentBytes = decoded
				}
			}

			f, err := gc.UploadRawFile(ctx, contentBytes, filename, mimeType, folderID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_save_file: %w", err)
			}
			out := map[string]any{
				"file_id":   f.ID,
				"file_url":  f.WebViewLink,
				"filename":  f.Name,
				"mime_type": mimeType,
			}
			makePublic, _ := input["make_public"].(bool)
			if !makePublic {
				if s, ok := input["make_public"].(string); ok {
					makePublic = s == "true"
				}
			}
			if makePublic {
				if err := gc.MakePublic(ctx, f.ID); err != nil {
					log.Printf("[gdrive_save_file] make public failed: %v", err)
				} else {
					out["public_url"] = "https://drive.google.com/uc?id=" + f.ID
				}
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Saves raw content (text, SVG, HTML, etc.) as a file in a Google Drive folder. Use after http_request to store the response body.",
			InputFields: []workflow.FieldMeta{
				{Name: "folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder to save the file in — use Browse to pick."},
				{Name: "filename", Type: "string", Required: true, Description: "Name of the file including extension, e.g. chart.svg"},
				{Name: "content", Type: "string", Required: true, Description: "File content — use {{body}} to pass the http_request response body."},
				{Name: "content_encoding", Type: "string", Description: "Set to 'base64' when content is a binary http_request response (images, PDFs). Leave blank for text.", Options: []string{"", "base64"}},
				{Name: "mime_type", Type: "string", Required: false, Description: "MIME type of the file. Defaults to text/plain.", Options: []string{
					"image/svg+xml",
					"text/html",
					"text/plain",
					"text/csv",
					"text/markdown",
					"application/json",
					"application/xml",
					"application/pdf",
					"image/png",
					"image/jpeg",
				}},
				{Name: "make_public", Type: "boolean", Description: "If true, grants anyone-with-link read access and returns a public_url."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "string", Description: "Drive ID of the uploaded file."},
				{Name: "file_url", Type: "string", Description: "Web view URL of the uploaded file."},
				{Name: "filename", Type: "string", Description: "Filename as saved."},
				{Name: "public_url", Type: "string", Description: "Direct public URL (only set when make_public is true)."},
			},
		},
	)

	// ── gdrive_fetch_url ──────────────────────────────────────────────────────
	// Fetches a URL as raw bytes and saves directly to Drive — no string
	// conversion, so binary files (PNG, JPEG, PDF) are never corrupted.

	eng.RegisterActivityWithMeta("gdrive_fetch_url",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return map[string]any{"error": "Google Drive not configured — add credentials in Settings"}, nil
			}

			rawURL := gdriveString(input, "url")
			if rawURL == "" {
				return nil, fmt.Errorf("gdrive_fetch_url: url is required")
			}
			folderID := gdriveString(input, "folder_id")
			if folderID == "" {
				return nil, fmt.Errorf("gdrive_fetch_url: folder_id is required")
			}
			filename := gdriveString(input, "filename")
			mimeType := gdriveString(input, "mime_type")

			method := strings.ToUpper(gdriveString(input, "method"))
			if method == "" {
				method = "GET"
			}

			rawURL = interpolateTemplate(rawURL, input)

			// Build request body.
			var bodyReader io.Reader
			if bodyVal, ok := input["body"]; ok && bodyVal != nil {
				var bodyStr string
				switch v := bodyVal.(type) {
				case string:
					bodyStr = interpolateTemplate(v, input)
				default:
					if b, err := json.Marshal(v); err == nil {
						bodyStr = string(b)
					}
				}
				if bodyStr != "" {
					bodyReader = strings.NewReader(bodyStr)
				}
			}

			timeout := 60 * time.Second
			if t, _ := input["timeout_seconds"].(float64); t > 0 {
				timeout = time.Duration(t) * time.Second
			}

			client := &http.Client{Timeout: timeout}
			req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
			if err != nil {
				return nil, fmt.Errorf("gdrive_fetch_url: build request: %w", err)
			}
			if bodyReader != nil {
				req.Header.Set("Content-Type", "application/json")
			}

			// Apply custom headers.
			var headersMap map[string]any
			if m, ok := input["headers"].(map[string]any); ok {
				headersMap = m
			} else if s, ok := input["headers"].(string); ok && s != "" {
				_ = json.Unmarshal([]byte(s), &headersMap)
			}
			for k, v := range headersMap {
				req.Header.Set(k, interpolateTemplate(fmt.Sprintf("%v", v), input))
			}

			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("gdrive_fetch_url: fetch %s: %w", rawURL, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return nil, fmt.Errorf("gdrive_fetch_url: HTTP %d from %s", resp.StatusCode, rawURL)
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("gdrive_fetch_url: read body: %w", err)
			}

			// Infer mime type from Content-Type if not provided.
			if mimeType == "" {
				mimeType = resp.Header.Get("Content-Type")
				if mimeType == "" {
					mimeType = "application/octet-stream"
				}
			}

			// Default filename from URL if not provided.
			if filename == "" {
				parts := strings.Split(strings.TrimRight(rawURL, "/"), "/")
				filename = parts[len(parts)-1]
				if filename == "" {
					filename = "file"
				}
			}

			log.Printf("[gdrive_fetch_url] fetched %d bytes (%s) → saving as %s", len(data), mimeType, filename)

			f, err := gc.UploadRawFile(ctx, data, filename, mimeType, folderID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_fetch_url: upload: %w", err)
			}

			out := map[string]any{
				"file_id":   f.ID,
				"file_url":  f.WebViewLink,
				"filename":  f.Name,
				"mime_type": mimeType,
			}

			makePublic, _ := input["make_public"].(bool)
			if !makePublic {
				if s, ok := input["make_public"].(string); ok {
					makePublic = s == "true"
				}
			}
			if makePublic {
				if err := gc.MakePublic(ctx, f.ID); err != nil {
					return nil, fmt.Errorf("gdrive_fetch_url: make public failed: %w", err)
				}
				out["public_url"] = "https://drive.google.com/uc?id=" + f.ID
			}

			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Fetches a URL as raw bytes and saves directly to Google Drive. Use this instead of http_request + gdrive_save_file for binary files (PNG, JPEG, PDF) to avoid corruption.",
			InputFields: []workflow.FieldMeta{
				{Name: "url", Type: "string", Required: true, Description: "URL to fetch. Supports {{key}} interpolation."},
				{Name: "method", Type: "string", Description: "HTTP method. Defaults to GET.", Options: []string{"GET", "POST", "PUT", "PATCH", "DELETE"}},
				{Name: "headers", Type: "object", Description: `Request headers as JSON, e.g. {"Authorization": "Bearer {{token}}"}.`},
				{Name: "body", Type: "textarea", Description: "Request body (for POST/PUT). Supports {{key}} interpolation."},
				{Name: "folder_id", Type: "gdrive_folder", Required: true, Description: "Drive folder to save into — use Browse to pick."},
				{Name: "filename", Type: "string", Description: "Filename to save as (e.g. chart.png). Defaults to last segment of the URL."},
				{Name: "mime_type", Type: "string", Description: "MIME type override. Defaults to Content-Type from the server response.", Options: []string{
					"image/png",
					"image/jpeg",
					"image/svg+xml",
					"image/gif",
					"image/webp",
					"application/pdf",
					"application/octet-stream",
				}},
				{Name: "make_public", Type: "boolean", Description: "If true, grants anyone-with-link read access and returns a public_url."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "string", Description: "Drive ID of the saved file."},
				{Name: "file_url", Type: "string", Description: "Web view URL of the saved file."},
				{Name: "filename", Type: "string", Description: "Filename as saved."},
				{Name: "mime_type", Type: "string", Description: "MIME type used."},
				{Name: "public_url", Type: "string", Description: "Direct public URL (only set when make_public is true)."},
			},
		},
	)
}

// buildFillTemplateVars builds the {{placeholder}} → value map for gdrive_fill_template.
//
// Priority:
//  1. "vars" is a JSON string  → unmarshal it
//  2. "vars" is already a map  → use it (debug panel coerces JSON to map)
//  3. no "vars"                → scan all top-level string keys from context
//     (skips _* internals and the four reserved node input keys)
func buildFillTemplateVars(input map[string]any) (map[string]string, error) {
	vars := make(map[string]string)

	varsRaw := gdriveString(input, "vars")
	if varsRaw != "" {
		if err := json.Unmarshal([]byte(varsRaw), &vars); err != nil {
			return nil, fmt.Errorf("vars is not valid JSON: %w", err)
		}
		return vars, nil
	}

	if varsMap, ok := input["vars"].(map[string]any); ok {
		for k, v := range varsMap {
			if s, ok := v.(string); ok {
				vars[k] = s
			}
		}
		return vars, nil
	}

	// Auto-scan: only top-level string values are picked up.
	// Nested objects (e.g. pb_birth_chart_records.chart_url) are NOT included.
	skip := map[string]bool{
		"template_id": true, "destination_folder_id": true, "title": true, "vars": true,
	}
	for k, v := range input {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		if skip[k] {
			continue
		}
		if s, ok := v.(string); ok {
			vars[k] = s
		}
	}
	return vars, nil
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

package google

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

// Client makes authenticated requests to Google Drive and Docs APIs.
type Client struct {
	tm *TokenManager
}

// NewClient creates a Client backed by the given TokenManager.
func NewClient(tm *TokenManager) *Client {
	return &Client{tm: tm}
}

// DriveFile represents a file or folder in Google Drive.
type DriveFile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MimeType    string `json:"mimeType"`
	WebViewLink string `json:"webViewLink"`
	IsFolder    bool   `json:"is_folder"`
}

const folderMimeType = "application/vnd.google-apps.folder"

// ListFiles returns the files and folders directly inside parentID.
// If parentID is empty, lists items in the Drive root.
func (c *Client) ListFiles(ctx context.Context, parentID string) ([]DriveFile, error) {
	if parentID == "" {
		parentID = "root"
	}
	// Escape single quotes in the parentID for the Drive query language.
	escapedID := strings.ReplaceAll(parentID, "'", `\'`)
	q := fmt.Sprintf("'%s' in parents and trashed=false", escapedID)

	u := "https://www.googleapis.com/drive/v3/files?" + url.Values{
		"q":       {q},
		"fields":  {"files(id,name,mimeType,webViewLink)"},
		"orderBy": {"folder,name"},
		"pageSize": {"100"},
	}.Encode()

	body, err := c.doRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Files []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			MimeType    string `json:"mimeType"`
			WebViewLink string `json:"webViewLink"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse ListFiles response: %w", err)
	}

	out := make([]DriveFile, len(resp.Files))
	for i, f := range resp.Files {
		out[i] = DriveFile{
			ID:          f.ID,
			Name:        f.Name,
			MimeType:    f.MimeType,
			WebViewLink: f.WebViewLink,
			IsFolder:    f.MimeType == folderMimeType,
		}
	}
	return out, nil
}

// GetFile returns metadata for a single Drive file or folder.
func (c *Client) GetFile(ctx context.Context, fileID string) (*DriveFile, error) {
	u := "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(fileID) +
		"?fields=id,name,mimeType,webViewLink"
	body, err := c.doRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var f struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		MimeType    string `json:"mimeType"`
		WebViewLink string `json:"webViewLink"`
	}
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("parse GetFile response: %w", err)
	}
	return &DriveFile{
		ID:          f.ID,
		Name:        f.Name,
		MimeType:    f.MimeType,
		WebViewLink: f.WebViewLink,
		IsFolder:    f.MimeType == folderMimeType,
	}, nil
}

// CopyFile duplicates fileID into folderID with the given title.
// Returns (newFileID, webViewLink, error).
func (c *Client) CopyFile(ctx context.Context, fileID, title, folderID string) (string, string, error) {
	payload := map[string]any{
		"name":    title,
		"parents": []string{folderID},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	u := "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(fileID) + "/copy?fields=id,webViewLink"
	body, err := c.doRequest(ctx, http.MethodPost, u, data)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		ID          string `json:"id"`
		WebViewLink string `json:"webViewLink"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("parse CopyFile response: %w", err)
	}
	return resp.ID, resp.WebViewLink, nil
}

// ReplaceAllText replaces every occurrence of {{key}} in the document with
// the corresponding value from vars. Returns the total occurrences changed.
func (c *Client) ReplaceAllText(ctx context.Context, docID string, vars map[string]string) (int, error) {
	type containsText struct {
		Text      string `json:"text"`
		MatchCase bool   `json:"matchCase"`
	}
	type replaceAllText struct {
		ContainsText containsText `json:"containsText"`
		ReplaceText  string       `json:"replaceText"`
	}
	type request struct {
		ReplaceAllText replaceAllText `json:"replaceAllText"`
	}

	requests := make([]request, 0, len(vars))
	for k, v := range vars {
		requests = append(requests, request{
			ReplaceAllText: replaceAllText{
				ContainsText: containsText{
					Text:      "{{" + k + "}}",
					MatchCase: true,
				},
				ReplaceText: v,
			},
		})
	}

	payload := map[string]any{"requests": requests}
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	u := "https://docs.googleapis.com/v1/documents/" + url.PathEscape(docID) + ":batchUpdate"
	body, err := c.doRequest(ctx, http.MethodPost, u, data)
	if err != nil {
		return 0, err
	}

	var resp struct {
		Replies []struct {
			ReplaceAllText *struct {
				OccurrencesChanged int `json:"occurrencesChanged"`
			} `json:"replaceAllText"`
		} `json:"replies"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse ReplaceAllText response: %w", err)
	}

	total := 0
	for _, r := range resp.Replies {
		if r.ReplaceAllText != nil {
			total += r.ReplaceAllText.OccurrencesChanged
		}
	}
	return total, nil
}

// ExportAndSavePDF exports docID as a PDF and uploads it to folderID with the
// given filename. Returns the resulting DriveFile.
func (c *Client) ExportAndSavePDF(ctx context.Context, docID, folderID, filename string) (*DriveFile, error) {
	// Step 1: export the Doc as PDF bytes.
	exportURL := "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(docID) +
		"/export?mimeType=application/pdf"
	pdfBytes, err := c.doRequest(ctx, http.MethodGet, exportURL, nil)
	if err != nil {
		return nil, fmt.Errorf("export PDF: %w", err)
	}

	// Step 2: upload via multipart to preserve the folder parent and filename.
	const boundary = "cbw_boundary"
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.SetBoundary(boundary)

	// Metadata part.
	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, err := mw.CreatePart(metaHeader)
	if err != nil {
		return nil, err
	}
	metaPayload := map[string]any{
		"name":     filename,
		"mimeType": "application/pdf",
		"parents":  []string{folderID},
	}
	if err := json.NewEncoder(metaPart).Encode(metaPayload); err != nil {
		return nil, err
	}

	// PDF bytes part.
	pdfHeader := make(textproto.MIMEHeader)
	pdfHeader.Set("Content-Type", "application/pdf")
	pdfPart, err := mw.CreatePart(pdfHeader)
	if err != nil {
		return nil, err
	}
	if _, err := pdfPart.Write(pdfBytes); err != nil {
		return nil, err
	}
	mw.Close()

	uploadURL := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&fields=id,name,webViewLink"
	tok, err := c.tm.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload PDF: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(respBody)
		if len(preview) > 400 {
			preview = preview[:400]
		}
		return nil, fmt.Errorf("upload PDF: HTTP %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		WebViewLink string `json:"webViewLink"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}
	return &DriveFile{
		ID:          result.ID,
		Name:        result.Name,
		MimeType:    "application/pdf",
		WebViewLink: result.WebViewLink,
	}, nil
}

// GetDocContent downloads a Drive file and returns its content as markdown.
// Native Google Docs use the Docs feed export endpoint with exportFormat=markdown.
// Office formats (.docx, etc.) are converted to a temp Google Doc, exported, then the temp is deleted.
// Other Google Workspace types (Sheets, Slides) are exported as plain text.
func (c *Client) GetDocContent(ctx context.Context, fileID, mimeType string) (string, error) {
	exportID := fileID

	switch mimeType {
	case "application/vnd.google-apps.document":
		// already a Google Doc — export directly

	case "application/vnd.google-apps.spreadsheet",
		"application/vnd.google-apps.presentation",
		"application/vnd.google-apps.drawing":
		rawURL := "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(fileID) +
			"/export?mimeType=text/plain"
		body, err := c.doRequest(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return "", err
		}
		return string(body), nil

	default:
		// Office format (.docx, .xlsx, etc.) — convert to Google Doc, export, delete temp.
		log.Printf("[gdrive] converting %q (%s) to Google Doc for markdown export", fileID, mimeType)
		tmpID, err := c.copyAsGoogleDoc(ctx, fileID)
		if err != nil {
			return "", fmt.Errorf("convert to Google Doc: %w", err)
		}
		defer func() {
			if delErr := c.deleteFile(ctx, tmpID); delErr != nil {
				log.Printf("[gdrive] failed to delete temp doc %s: %v", tmpID, delErr)
			}
		}()
		exportID = tmpID
	}

	rawURL := "https://docs.google.com/feeds/download/documents/export/Export?exportFormat=markdown&id=" + url.QueryEscape(exportID)
	log.Printf("[gdrive] GetDocContent export url=%s", rawURL)
	body, err := c.doRequest(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	log.Printf("[gdrive] GetDocContent got %d bytes", len(body))
	return string(body), nil
}

// copyAsGoogleDoc copies fileID to a new Google Doc (Drive converts the format).
// Returns the new file's ID.
func (c *Client) copyAsGoogleDoc(ctx context.Context, fileID string) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"mimeType": "application/vnd.google-apps.document",
		"name":     "_tmp_export",
	})
	u := "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(fileID) + "/copy?fields=id"
	body, err := c.doRequest(ctx, http.MethodPost, u, payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse copy response: %w", err)
	}
	if resp.ID == "" {
		return "", fmt.Errorf("copy response contained no file ID")
	}
	return resp.ID, nil
}

// deleteFile permanently deletes a Drive file by ID.
func (c *Client) deleteFile(ctx context.Context, fileID string) error {
	u := "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(fileID)
	_, err := c.doRequest(ctx, http.MethodDelete, u, nil)
	return err
}

// CreateDoc creates a new Google Doc in folderID by uploading content as plain text.
// Drive converts the text/plain content to a Google Doc automatically.
// Returns the resulting DriveFile (id, name, webViewLink).
func (c *Client) CreateDoc(ctx context.Context, title, content, folderID string) (*DriveFile, error) {
	const boundary = "cbw_create_doc"
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.SetBoundary(boundary)

	// Metadata part.
	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, err := mw.CreatePart(metaHeader)
	if err != nil {
		return nil, err
	}
	metaPayload := map[string]any{
		"name":     title,
		"mimeType": "application/vnd.google-apps.document",
		"parents":  []string{folderID},
	}
	if err := json.NewEncoder(metaPart).Encode(metaPayload); err != nil {
		return nil, err
	}

	// Content part.
	contentHeader := make(textproto.MIMEHeader)
	contentHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	contentPart, err := mw.CreatePart(contentHeader)
	if err != nil {
		return nil, err
	}
	if _, err := contentPart.Write([]byte(content)); err != nil {
		return nil, err
	}
	mw.Close()

	uploadURL := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&fields=id,name,webViewLink"
	tok, err := c.tm.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create doc: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(respBody)
		if len(preview) > 400 {
			preview = preview[:400]
		}
		return nil, fmt.Errorf("create doc: HTTP %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		WebViewLink string `json:"webViewLink"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse create doc response: %w", err)
	}
	return &DriveFile{
		ID:          result.ID,
		Name:        result.Name,
		MimeType:    "application/vnd.google-apps.document",
		WebViewLink: result.WebViewLink,
	}, nil
}

// ShareFile grants email the given role on fileID silently (no notification email).
// Returns the permission ID created by Google.
func (c *Client) ShareFile(ctx context.Context, fileID, email, role string) (string, error) {
	payload := map[string]any{
		"role":         role,
		"type":         "user",
		"emailAddress": email,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	u := "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(fileID) +
		"/permissions?sendNotificationEmail=false&fields=id"
	body, err := c.doRequest(ctx, http.MethodPost, u, data)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse ShareFile response: %w", err)
	}
	return resp.ID, nil
}

// GetDocumentContent fetches the text content of a Google Doc.
// Extracts all text from the document body elements.
func (c *Client) GetDocumentContent(ctx context.Context, docID string) (string, error) {
	u := "https://docs.googleapis.com/v1/documents/" + url.PathEscape(docID)
	respBody, err := c.doRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("fetch document: %w", err)
	}

	var doc struct {
		Body struct {
			Content []struct {
				Paragraph *struct {
					Elements []struct {
						TextRun *struct {
							Content string `json:"content"`
						} `json:"textRun"`
					} `json:"elements"`
				} `json:"paragraph"`
				Table *struct{} `json:"table"`
			} `json:"content"`
		} `json:"body"`
	}

	if err := json.Unmarshal(respBody, &doc); err != nil {
		return "", fmt.Errorf("parse document: %w", err)
	}

	var text strings.Builder
	for _, elem := range doc.Body.Content {
		if elem.Paragraph != nil {
			for _, textElem := range elem.Paragraph.Elements {
				if textElem.TextRun != nil {
					text.WriteString(textElem.TextRun.Content)
				}
			}
		}
	}

	return text.String(), nil
}

// doRequest performs an authenticated HTTP request and returns the response body.
// A non-2xx status is returned as an error with a body preview.
func (c *Client) doRequest(ctx context.Context, method, rawURL string, body []byte) ([]byte, error) {
	tok, err := c.tm.GetAccessToken()
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, rawURL, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(respBody)
		if len(preview) > 400 {
			preview = preview[:400]
		}
		return nil, fmt.Errorf("Google API error %d: %s", resp.StatusCode, preview)
	}
	return respBody, nil
}

package workflows

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"

	googleapp "cosmicbizwitch/internal/app/google"
	"cosmicbizwitch/pkg/workflow"
)

// newGoogleNodesEngine builds an Engine with Google, image, and transcribe activities
// all registered using nil clients/app — exercises the "not configured" branch of each node.
func newGoogleNodesEngine() *workflow.Engine {
	nilGoogle := func() *googleapp.Client { return nil }
	eng := workflow.NewEngine(nil, workflow.EngineConfig{Logger: log.New(io.Discard, "", 0)})
	registerGoogleActivities(eng, nilGoogle)
	registerImageActivities(eng, nil, nilGoogle)
	registerTranscribeActivities(eng, nil, nilGoogle)
	return eng
}

func execGoogle(t *testing.T, name string, input map[string]any) (map[string]any, error) {
	t.Helper()
	return newGoogleNodesEngine().ExecuteActivity(context.Background(), name, input)
}

// helper: assert a soft-error response (no error returned, "error" key in output map)
func assertSoftError(t *testing.T, out map[string]any, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected soft error (nil err), got: %v", err)
	}
	if _, ok := out["error"]; !ok {
		t.Errorf("expected 'error' key in output map, got %v", out)
	}
}

// ── gdrive_get_file ──────────────────────────────────────────────────────────

func TestGdriveGetFile_NilClient(t *testing.T) {
	out, err := execGoogle(t, "gdrive_get_file", map[string]any{"file_id": "abc123"})
	assertSoftError(t, out, err)
}

// ── gdrive_copy_template ─────────────────────────────────────────────────────

func TestGdriveCopyTemplate_NilClient(t *testing.T) {
	out, err := execGoogle(t, "gdrive_copy_template", map[string]any{
		"template_id":           "tmpl1",
		"destination_folder_id": "folder1",
	})
	assertSoftError(t, out, err)
}

// ── gdrive_fill_template ─────────────────────────────────────────────────────

func TestGdriveFillTemplate_NilClient(t *testing.T) {
	out, err := execGoogle(t, "gdrive_fill_template", map[string]any{
		"template_id":           "tmpl1",
		"destination_folder_id": "folder1",
	})
	assertSoftError(t, out, err)
}

// ── gdrive_export_pdf ────────────────────────────────────────────────────────

func TestGdriveExportPDF_NilClient(t *testing.T) {
	out, err := execGoogle(t, "gdrive_export_pdf", map[string]any{
		"doc_id":                "doc1",
		"destination_folder_id": "folder1",
	})
	assertSoftError(t, out, err)
}

// ── convert_to_pdf ───────────────────────────────────────────────────────────

func TestConvertToPDF_NilClient(t *testing.T) {
	out, err := execGoogle(t, "convert_to_pdf", map[string]any{
		"doc_id":    "doc1",
		"folder_id": "folder1",
	})
	assertSoftError(t, out, err)
}

func TestConvertToPDF_ExtractsDocIDFromURL(t *testing.T) {
	// Nil client → soft error, but the URL→docID extraction runs first.
	// We can at least verify the node accepts doc_url without panicking.
	out, err := execGoogle(t, "convert_to_pdf", map[string]any{
		"doc_url":   "https://docs.google.com/document/d/DOCIDABC/edit",
		"folder_id": "folder1",
	})
	assertSoftError(t, out, err)
}

// ── gdrive_share_file ────────────────────────────────────────────────────────

func TestGdriveShareFile_NilClient(t *testing.T) {
	out, err := execGoogle(t, "gdrive_share_file", map[string]any{
		"file_id": "file1",
		"email":   "x@x.com",
	})
	assertSoftError(t, out, err)
}

// ── gdrive_create_doc ────────────────────────────────────────────────────────

func TestGdriveCreateDoc_NilClient(t *testing.T) {
	out, err := execGoogle(t, "gdrive_create_doc", map[string]any{
		"folder_id": "folder1",
		"title":     "My Doc",
	})
	assertSoftError(t, out, err)
}

// ── gdrive_save_file ─────────────────────────────────────────────────────────

func TestGdriveSaveFile_NilClient(t *testing.T) {
	out, err := execGoogle(t, "gdrive_save_file", map[string]any{
		"folder_id": "folder1",
		"filename":  "test.txt",
		"content":   "hello",
	})
	assertSoftError(t, out, err)
}

// ── gdrive_fetch_url ──────────────────────────────────────────────────────────

func TestGdriveFetchURL_NilClient(t *testing.T) {
	out, err := execGoogle(t, "gdrive_fetch_url", map[string]any{
		"url":       "https://example.com/file.txt",
		"folder_id": "folder1",
	})
	assertSoftError(t, out, err)
}

// ── gdrive_svg_to_png ────────────────────────────────────────────────────────

func TestGdriveSVGToPNG_NilClient(t *testing.T) {
	_, err := execGoogle(t, "gdrive_svg_to_png", map[string]any{
		"file_id":   "file1",
		"folder_id": "folder1",
	})
	if err == nil {
		t.Fatal("expected hard error for nil Google client")
	}
	if !strings.Contains(err.Error(), "Google not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── image_generate ────────────────────────────────────────────────────────────

func TestImageGenerate_MissingPrompt(t *testing.T) {
	_, err := execGoogle(t, "image_generate", map[string]any{"model": "openai/dall-e-3"})
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestImageGenerate_MissingModel(t *testing.T) {
	_, err := execGoogle(t, "image_generate", map[string]any{"prompt": "a cat"})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── transcribe_from_url ──────────────────────────────────────────────────────

func TestTranscribeFromURL_MissingURL(t *testing.T) {
	_, err := execGoogle(t, "transcribe_from_url", map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── gdrive_transcribe ────────────────────────────────────────────────────────

func TestGdriveTranscribe_NilClient(t *testing.T) {
	_, err := execGoogle(t, "gdrive_transcribe", map[string]any{
		"file_id": "file1",
	})
	if err == nil {
		t.Fatal("expected hard error for nil Google client")
	}
	if !strings.Contains(err.Error(), "Google Drive not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── filenameFromMimeType ─────────────────────────────────────────────────────

func TestFilenameFromMimeType(t *testing.T) {
	cases := []struct {
		mimeType string
		want     string
	}{
		{"audio/mpeg", "audio.mp3"},
		{"audio/mp3", "audio.mp3"},
		{"audio/mp4", "audio.mp4"},
		{"video/mp4", "audio.mp4"},
		{"audio/webm", "audio.webm"},
		{"video/webm", "audio.webm"},
		{"audio/wav", "audio.wav"},
		{"audio/x-wav", "audio.wav"},
		{"audio/ogg", "audio.ogg"},
		{"video/ogg", "audio.ogg"},
		{"audio/flac", "audio.flac"},
		{"audio/x-m4a", "audio.m4a"},
		{"video/mpeg", "audio.mpeg"},
		{"application/octet-stream", "audio.mp3"}, // unknown → default
		{"", "audio.mp3"},                          // empty → default
	}
	for _, tc := range cases {
		got := filenameFromMimeType(tc.mimeType, "fallback_id")
		if got != tc.want {
			t.Errorf("filenameFromMimeType(%q) = %q, want %q", tc.mimeType, got, tc.want)
		}
	}
}

// ── gdriveString ─────────────────────────────────────────────────────────────

func TestGdriveString(t *testing.T) {
	input := map[string]any{
		"key1": "value1",
		"key2": 42,     // non-string: should return ""
		"key3": nil,    // nil: should return ""
	}
	if got := gdriveString(input, "key1"); got != "value1" {
		t.Errorf("got %q, want %q", got, "value1")
	}
	if got := gdriveString(input, "key2"); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
	if got := gdriveString(input, "missing"); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

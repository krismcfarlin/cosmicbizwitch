package workflows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	googleapp "cosmicbizwitch/internal/app/google"
	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/pocketbase/core"
)

// registerTranscribeActivities registers AI transcription workflow nodes.
func registerTranscribeActivities(eng *workflow.Engine, app core.App, getGoogle func() *googleapp.Client) {
	eng.RegisterActivityWithMeta("transcribe_from_url",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			fileURL := llmString(input, "url")
			if fileURL == "" {
				return nil, fmt.Errorf("transcribe_from_url: url is required")
			}
			language := llmString(input, "language")
			resultKey := llmString(input, "result_key")
			if resultKey == "" {
				resultKey = "transcript"
			}

			keyName := llmString(input, "api_key_name")
			var apiKey string
			if keyName != "" {
				apiKey = llmGetOpenRouterKey(app, keyName)
			}
			if apiKey == "" {
				apiKey = llmGetSetting(app, "OPENAI_API_KEY")
			}
			if apiKey == "" {
				return nil, fmt.Errorf("transcribe_from_url: no API key configured — set OPENAI_API_KEY in Settings")
			}

			resp, err := http.Get(fileURL)
			if err != nil {
				return nil, fmt.Errorf("transcribe_from_url: download: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("transcribe_from_url: download returned HTTP %d", resp.StatusCode)
			}
			fileBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("transcribe_from_url: read body: %w", err)
			}

			// derive filename from URL for Whisper's content-type detection
			filename := "audio.mp3"
			if u := fileURL; len(u) > 4 {
				for _, ext := range []string{".mp3", ".mp4", ".m4a", ".wav", ".webm", ".ogg", ".flac", ".mpeg"} {
					if len(u) >= len(ext) && u[len(u)-len(ext):] == ext {
						filename = "audio" + ext
						break
					}
				}
			}

			transcript, err := callWhisperTranscribe(apiKey, fileBytes, filename, language)
			if err != nil {
				return nil, fmt.Errorf("transcribe_from_url: %w", err)
			}

			return map[string]any{
				resultKey:    transcript,
				"transcript": transcript,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "AI",
			Description: "Downloads an audio or video file from any public URL and transcribes it using OpenAI Whisper.",
			InputFields: []workflow.FieldMeta{
				{Name: "url", Type: "string", Required: true, Description: "Public URL of the audio or video file to transcribe. Works with Telegram file URLs, S3, CDN links, etc."},
				{Name: "api_key_name", Type: "string", Description: "Named OpenRouter key to use. If empty, falls back to OPENAI_API_KEY setting."},
				{Name: "language", Type: "string", Description: "ISO 639-1 language code hint (e.g. en, fr, es). Optional."},
				{Name: "result_key", Type: "string", Description: "Context key to store the transcript under. Defaults to 'transcript'."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "transcript", Type: "string", Description: "The transcribed text."},
			},
		},
	)

	eng.RegisterActivityWithMeta("gdrive_transcribe",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gc := getGoogle()
			if gc == nil {
				return nil, fmt.Errorf("gdrive_transcribe: Google Drive not configured — add credentials in Settings")
			}

			fileID := llmString(input, "file_id")
			if fileID == "" {
				return nil, fmt.Errorf("gdrive_transcribe: file_id is required")
			}

			language := llmString(input, "language")

			resultKey := llmString(input, "result_key")
			if resultKey == "" {
				resultKey = "transcript"
			}

			// Determine the API key to use. Try named OpenRouter key first,
			// then fall back to the dedicated OPENAI_API_KEY setting.
			keyName := llmString(input, "api_key_name")
			var apiKey string
			if keyName != "" {
				apiKey = llmGetOpenRouterKey(app, keyName)
			}
			if apiKey == "" {
				apiKey = llmGetSetting(app, "OPENAI_API_KEY")
			}
			if apiKey == "" {
				return nil, fmt.Errorf("gdrive_transcribe: no API key configured — set OPENAI_API_KEY in Settings or provide api_key_name referencing an OpenRouter key")
			}

			// Download the audio/video file from Google Drive.
			fileBytes, mimeType, err := gc.DownloadFile(ctx, fileID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_transcribe: download file: %w", err)
			}

			// Pick a sensible filename for the multipart upload.
			filename := filenameFromMimeType(mimeType, fileID)

			transcript, err := callWhisperTranscribe(apiKey, fileBytes, filename, language)
			if err != nil {
				return nil, fmt.Errorf("gdrive_transcribe: %w", err)
			}

			return map[string]any{
				resultKey:    transcript,
				"transcript": transcript,
			}, nil
		},
		workflow.ActivityMeta{
			Category:    "AI",
			Description: "Transcribes an audio or video file from Google Drive using OpenAI Whisper. Returns the text transcript.",
			InputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "gdrive_file", Required: true, Description: "Google Drive file ID of the audio or video file to transcribe."},
				{Name: "api_key_name", Type: "string", Description: "Named OpenRouter key to use. If empty, falls back to OPENAI_API_KEY setting."},
				{Name: "language", Type: "string", Description: "ISO 639-1 language code hint for the audio (e.g. en, fr). Optional."},
				{Name: "result_key", Type: "string", Description: "Context key to store the transcript under. Defaults to 'transcript'."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "transcript", Type: "string", Description: "The transcribed text (also stored under result_key if specified)."},
			},
		},
	)
}

// callWhisperTranscribe sends audio bytes to the OpenAI Whisper transcription endpoint
// and returns the transcript text.
func callWhisperTranscribe(apiKey string, fileBytes []byte, filename, language string) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	filePart, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := filePart.Write(fileBytes); err != nil {
		return "", fmt.Errorf("write file bytes: %w", err)
	}

	if err := mw.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("write model field: %w", err)
	}

	if language != "" {
		if err := mw.WriteField("language", language); err != nil {
			return "", fmt.Errorf("write language field: %w", err)
		}
	}

	mw.Close()

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/transcriptions", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return "", fmt.Errorf("Whisper API error %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return result.Text, nil
}

// filenameFromMimeType returns a sensible filename for the Whisper API based on the MIME type.
func filenameFromMimeType(mimeType, fallbackID string) string {
	switch mimeType {
	case "audio/mpeg", "audio/mp3":
		return "audio.mp3"
	case "audio/mp4", "video/mp4":
		return "audio.mp4"
	case "audio/webm", "video/webm":
		return "audio.webm"
	case "audio/wav", "audio/x-wav":
		return "audio.wav"
	case "audio/ogg", "video/ogg":
		return "audio.ogg"
	case "audio/flac":
		return "audio.flac"
	case "audio/x-m4a":
		return "audio.m4a"
	case "video/mpeg":
		return "audio.mpeg"
	default:
		return "audio.mp3"
	}
}

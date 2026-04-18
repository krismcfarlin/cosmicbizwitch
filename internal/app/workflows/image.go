package workflows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	googleapp "cosmicbizwitch/internal/app/google"
	"cosmicbizwitch/pkg/workflow"

	"github.com/pocketbase/pocketbase/core"
)

// registerImageActivities registers image-related workflow nodes.
func registerImageActivities(eng *workflow.Engine, app core.App, getGoogle func() *googleapp.Client) {
	eng.RegisterActivityWithMeta("image_generate",
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			prompt := llmString(input, "prompt")
			if prompt == "" {
				return nil, fmt.Errorf("image_generate: prompt is required")
			}
			model := llmString(input, "model")
			if model == "" {
				return nil, fmt.Errorf("image_generate: model is required")
			}

			keyName := llmString(input, "api_key_name")
			apiKey := llmGetOpenRouterKey(app, keyName)
			if apiKey == "" {
				return nil, fmt.Errorf("image_generate: no OpenRouter API key configured (set OPENROUTER_KEYS or OPENROUTER_API_KEY in Settings)")
			}

			size := llmString(input, "size")
			if size == "" {
				size = "1024x1024"
			}
			n := llmInt(input, "n", 1)
			if n < 1 {
				n = 1
			}

			resultKey := llmString(input, "result_key")
			if resultKey == "" {
				resultKey = "image_url"
			}

			urls, err := callOpenRouterImageGenerate(apiKey, model, prompt, size, n)
			if err != nil {
				return nil, fmt.Errorf("image_generate: %w", err)
			}
			if len(urls) == 0 {
				return nil, fmt.Errorf("image_generate: no image URLs returned")
			}

			out := map[string]any{
				resultKey:    urls[0],
				"image_url":  urls[0],
				"image_urls": urls,
			}
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "AI",
			Description: "Generates images using an AI model via OpenRouter. Returns the URL(s) of the generated image(s).",
			InputFields: []workflow.FieldMeta{
				{Name: "prompt", Type: "string", Required: true, Description: "Text description of the image to generate."},
				{Name: "model", Type: "string", Required: true, Description: "Image model identifier, e.g. openai/dall-e-3, openai/dall-e-2, stability-ai/stable-diffusion-xl."},
				{Name: "api_key_name", Type: "string", Description: "Named OpenRouter API key to use (from Settings → OpenRouter Keys). Leave empty to use the default key."},
				{Name: "size", Type: "string", Description: "Image dimensions. Options: 1024x1024 (default), 512x512, 256x256, 1792x1024, 1024x1792."},
				{Name: "n", Type: "number", Description: "Number of images to generate. Defaults to 1."},
				{Name: "result_key", Type: "string", Description: "Context key to store the first image URL under. Defaults to 'image_url'."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "image_url", Type: "string", Description: "URL of the first generated image (also stored under result_key if specified)."},
				{Name: "image_urls", Type: "array", Description: "All generated image URLs when n > 1."},
			},
		},
	)

	eng.RegisterActivityWithMeta("gdrive_svg_to_png",
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			gClient := getGoogle()
			if gClient == nil {
				return nil, fmt.Errorf("gdrive_svg_to_png: Google not configured")
			}

			fileID := llmString(input, "file_id")
			if fileID == "" {
				return nil, fmt.Errorf("gdrive_svg_to_png: file_id is required")
			}
			folderID := llmString(input, "folder_id")
			if folderID == "" {
				return nil, fmt.Errorf("gdrive_svg_to_png: folder_id is required")
			}
			filename := llmString(input, "filename")
			width := llmString(input, "width")

			// Download SVG from Drive.
			svgBytes, _, err := gClient.DownloadFile(ctx, fileID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_svg_to_png: download SVG: %w", err)
			}

			// Default filename: source name with .png extension.
			if filename == "" {
				f, fErr := gClient.GetFile(ctx, fileID)
				if fErr == nil && f.Name != "" {
					base := strings.TrimSuffix(f.Name, filepath.Ext(f.Name))
					filename = base + ".png"
				} else {
					filename = "output.png"
				}
			}

			// Write SVG to temp directory.
			tmpDir, err := os.MkdirTemp("", "svg2png-")
			if err != nil {
				return nil, fmt.Errorf("gdrive_svg_to_png: create temp dir: %w", err)
			}
			defer os.RemoveAll(tmpDir)

			svgPath := filepath.Join(tmpDir, "input.svg")
			pngPath := filepath.Join(tmpDir, "output.png")
			if err := os.WriteFile(svgPath, svgBytes, 0644); err != nil {
				return nil, fmt.Errorf("gdrive_svg_to_png: write SVG: %w", err)
			}

			// Convert SVG → transparent PNG via rsvg-convert (librsvg2-bin).
			args := []string{"-f", "png", "-o", pngPath}
			if width != "" {
				args = append(args, "-w", width)
			}
			args = append(args, svgPath)
			cmd := exec.CommandContext(ctx, "rsvg-convert", args...)
			if out, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
				return nil, fmt.Errorf("gdrive_svg_to_png: rsvg-convert failed: %w: %s", cmdErr, strings.TrimSpace(string(out)))
			}

			pngBytes, err := os.ReadFile(pngPath)
			if err != nil {
				return nil, fmt.Errorf("gdrive_svg_to_png: read PNG: %w", err)
			}

			// Upload PNG to Drive.
			result, err := gClient.UploadRawFile(ctx, pngBytes, filename, "image/png", folderID)
			if err != nil {
				return nil, fmt.Errorf("gdrive_svg_to_png: upload PNG: %w", err)
			}

			out := map[string]any{
				"file_id":  result.ID,
				"file_url": result.WebViewLink,
				"filename": result.Name,
			}

			// Optionally make the file publicly readable.
			makePublic, _ := input["make_public"].(bool)
			if !makePublic {
				if s, ok := input["make_public"].(string); ok {
					makePublic = s == "true"
				}
			}
			if makePublic {
				if err := gClient.MakePublic(ctx, result.ID); err != nil {
					return nil, fmt.Errorf("gdrive_svg_to_png: make public failed: %w", err)
				}
				out["public_url"] = "https://drive.google.com/uc?id=" + result.ID
			}

			log.Printf("[gdrive_svg_to_png] %s → %s (%d bytes) public=%v", fileID, result.ID, len(pngBytes), makePublic)
			return out, nil
		},
		workflow.ActivityMeta{
			Category:    "Google Workspace",
			Description: "Downloads an SVG from Google Drive, converts it to a transparent-background PNG using rsvg-convert, and uploads the result to a Drive folder.",
			InputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "gdrive_file", Required: true, Description: "Google Drive file ID of the source SVG. Browse to select or type {{key}}."},
				{Name: "folder_id", Type: "gdrive_folder", Required: true, Description: "Destination Google Drive folder for the output PNG."},
				{Name: "filename", Type: "string", Description: "Output filename. Defaults to source filename with .png extension."},
				{Name: "width", Type: "string", Description: "Output width in pixels. Height scales proportionally. Leave empty to use the SVG's natural size."},
				{Name: "make_public", Type: "boolean", Description: "If true, grants anyone-with-link read access to the uploaded PNG and returns a public_url."},
			},
			OutputFields: []workflow.FieldMeta{
				{Name: "file_id", Type: "string", Description: "Google Drive file ID of the uploaded PNG."},
				{Name: "file_url", Type: "string", Description: "Web view URL of the uploaded PNG."},
				{Name: "filename", Type: "string", Description: "Filename of the uploaded PNG."},
				{Name: "public_url", Type: "string", Description: "Direct public download URL (only set when make_public is true)."},
			},
		},
	)
}

// callOpenRouterImageGenerate calls the OpenRouter images/generations endpoint and returns image URLs.
func callOpenRouterImageGenerate(apiKey, model, prompt, size string, n int) ([]string, error) {
	body := map[string]any{
		"model":  model,
		"prompt": prompt,
		"n":      n,
		"size":   size,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/images/generations", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return nil, fmt.Errorf("OpenRouter image API error %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	urls := make([]string, 0, len(result.Data))
	for _, d := range result.Data {
		if d.URL != "" {
			urls = append(urls, d.URL)
		}
	}
	return urls, nil
}

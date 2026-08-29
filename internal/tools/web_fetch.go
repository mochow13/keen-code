package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/mochow13/keen-code/internal/filesystem"
)

const (
	webFetchTimeout          = 30 * time.Second
	maxInlineWebFetchSize    = 16 * 1024
	webFetchPreviewHeadSize  = 4 * 1024
	webFetchPreviewTailSize  = 2 * 1024
	webFetchArtifactFileMode = 0600
)

type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{}
}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

func (t *WebFetchTool) Description() string {
	return `Fetch content from a URL and return it as text.

Use this through the tool API whenever you say you will fetch, open, read, check, or inspect a URL or public web content. Do not merely describe fetching web content in assistant text.

HTML pages are automatically converted to Markdown for readability. Other content
types (JSON, plain text, XML) are returned as-is.

Use this for: reading documentation, fetching API specs, checking URLs, reading
README files from GitHub, or any public web content.

Limitations:
- JavaScript-rendered pages (SPAs) will return the pre-JS skeleton, not the
  dynamically loaded content.
- Auth-gated pages will return a redirect or login page.
- If the result is very large, the full output is saved to a file and the response includes
  truncated: true, artifact_path, and a preview in content. Use read_file with offset/limit or
  grep with path set to artifact_path to inspect the saved result incrementally.`
}

func (t *WebFetchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch",
			},
		},
		"required":             []string{"url"},
		"additionalProperties": false,
	}
}

func (t *WebFetchTool) ValidateInput(_ context.Context, input any) error {
	params, ok := input.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	url, ok := params["url"].(string)
	if !ok || url == "" {
		if _, exists := params["url"]; !exists {
			return missingRequiredParameter("web_fetch", "url", `{"url":"https://example.com"}`, "Provide the complete public URL to fetch")
		}
		return fmt.Errorf("invalid input: url must be a non-empty string")
	}
	return nil
}

func (t *WebFetchTool) Execute(ctx context.Context, input any) (any, error) {
	params := input.(map[string]any)
	url := params["url"].(string)

	client := &http.Client{Timeout: webFetchTimeout}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "keen-code/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	content := string(bodyBytes)
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		if md, err := htmltomarkdown.ConvertString(content); err == nil {
			content = md
		}
	}

	output := map[string]any{"status_code": resp.StatusCode}
	if len(content) <= maxInlineWebFetchSize {
		output["content"] = content
		return output, nil
	}

	summary, err := summarizeWebFetchResult(content)
	if err != nil {
		return nil, fmt.Errorf("failed to write web fetch result artifact: %w", err)
	}

	output["content"] = summary.preview
	output["truncated"] = true
	output["artifact_path"] = summary.path
	return output, nil
}

type webFetchResultSummary struct {
	preview string
	path    string
}

func summarizeWebFetchResult(content string) (webFetchResultSummary, error) {
	data := []byte(content)
	path, err := writeWebFetchArtifact(data)
	if err != nil {
		return webFetchResultSummary{}, err
	}

	headSize := min(webFetchPreviewHeadSize, len(data))
	tailSize := min(webFetchPreviewTailSize, len(data)-headSize)
	tailStart := len(data) - tailSize
	omitted := len(data) - headSize - tailSize

	preview := fmt.Sprintf(
		"%s\n\n... (%d bytes omitted; full result saved to artifact_path) ...\n\n%s",
		string(data[:headSize]),
		omitted,
		string(data[tailStart:]),
	)

	return webFetchResultSummary{
		preview: preview,
		path:    path,
	}, nil
}

func writeWebFetchArtifact(data []byte) (string, error) {
	dir, err := filesystem.KeenWebFetchArtifactsDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve web fetch artifacts directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create web fetch artifacts directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to secure web fetch artifacts directory %q: %w", dir, err)
	}

	file, err := os.CreateTemp(dir, "keen-web-fetch-*"+".txt")
	if err != nil {
		return "", fmt.Errorf("failed to create web fetch artifact file: %w", err)
	}
	path := file.Name()
	defer file.Close()

	if err := file.Chmod(webFetchArtifactFileMode); err != nil {
		return "", fmt.Errorf("failed to secure web fetch artifact file %q: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("failed to write web fetch artifact file %q: %w", path, err)
	}
	return path, nil
}

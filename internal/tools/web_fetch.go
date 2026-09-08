package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/mochow13/keen-code/internal/filesystem"
)

const (
	webFetchTimeout          = 30 * time.Second
	maxInlineWebFetchSize    = 16 * 1024
	webFetchPreviewHeadSize  = 4 * 1024
	webFetchPreviewTailSize  = 2 * 1024
	webFetchArtifactFileMode = 0600
	webFetchCacheLRUCapacity = 512
	webFetchCacheFilePrefix  = "keen-web-fetch-"
)

type WebFetchTool struct {
	cache *lru.Cache[string, webFetchCacheEntry]
}

func NewWebFetchTool() *WebFetchTool {
	cache, _ := lru.New[string, webFetchCacheEntry](webFetchCacheLRUCapacity)
	return &WebFetchTool{cache: cache}
}

func (t *WebFetchTool) Name() string {
	return WebFetchToolName
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
			"checkCache": map[string]any{
				"type":        "boolean",
				"description": "Set true when the same URL was fetched recently and the result is not expected to have changed; reuses the cached result. Set false or omit to fetch and refresh the cache",
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
			return missingRequiredParameter(
				WebFetchToolName,
				"url",
				`{"url":"https://example.com"}`,
				"Provide the complete public URL to fetch",
			)
		}
		return fmt.Errorf("invalid input: url must be a non-empty string")
	}
	if raw, exists := params["checkCache"]; exists && raw != nil {
		if _, ok := raw.(bool); !ok {
			return fmt.Errorf("invalid input: %q must be a boolean", "checkCache")
		}
	}
	return nil
}

func (t *WebFetchTool) Execute(ctx context.Context, input any) (any, error) {
	params := input.(map[string]any)
	url := params["url"].(string)
	checkCache, _ := params["checkCache"].(bool)

	key := webFetchCacheKey(url)
	if checkCache {
		if cached, ok := t.lookupCache(key); ok {
			return cached, nil
		}
	}

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
	cacheable := resp.StatusCode == http.StatusOK

	if len(content) <= maxInlineWebFetchSize {
		if cacheable {
			_, _ = t.storeCache(key, content)
		}
		output["content"] = content
		return output, nil
	}

	var path string
	if cacheable {
		path, err = t.storeCache(key, content)
	} else {
		path, err = writeWebFetchArtifact([]byte(content))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to write web fetch result artifact: %w", err)
	}

	output["content"] = buildWebFetchPreview([]byte(content))
	output["truncated"] = true
	output["artifact_path"] = path
	return output, nil
}

type webFetchCacheEntry struct {
	content  string
	cachedAt time.Time
}

func webFetchCacheKey(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:16])
}

func webFetchCachePath(key string) (string, error) {
	dir, err := filesystem.KeenWebFetchArtifactsDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve web fetch artifacts directory: %w", err)
	}
	return filepath.Join(dir, webFetchCacheFilePrefix+key+".txt"), nil
}

func (t *WebFetchTool) lookupCache(key string) (map[string]any, bool) {
	if cache := t.cache; cache != nil {
		if entry, ok := cache.Get(key); ok {
			return map[string]any{
				"status_code": http.StatusOK,
				"content":     entry.content,
				"cached_at":   entry.cachedAt.UTC().Format(time.RFC3339),
			}, true
		}
	}

	path, err := webFetchCachePath(key)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}

	output := map[string]any{
		"status_code": http.StatusOK,
		"content":     string(data),
		"cached_at":   info.ModTime().UTC().Format(time.RFC3339),
	}
	if len(data) > maxInlineWebFetchSize {
		output["content"] = buildWebFetchPreview(data)
		output["truncated"] = true
		output["artifact_path"] = path
	}
	return output, true
}

func (t *WebFetchTool) storeCache(key, content string) (string, error) {
	if len(content) <= maxInlineWebFetchSize {
		if cache := t.cache; cache != nil {
			cache.Add(key, webFetchCacheEntry{content: content, cachedAt: time.Now()})
		}
		return "", nil
	}

	path, err := webFetchCachePath(key)
	if err != nil {
		return "", err
	}
	if err := writeWebFetchCacheFile(path, []byte(content)); err != nil {
		return "", err
	}
	return path, nil
}

func buildWebFetchPreview(data []byte) string {
	headSize := min(webFetchPreviewHeadSize, len(data))
	tailSize := min(webFetchPreviewTailSize, len(data)-headSize)
	tailStart := len(data) - tailSize
	omitted := len(data) - headSize - tailSize

	return fmt.Sprintf(
		"%s\n\n... (%d bytes omitted; full result saved to artifact_path) ...\n\n%s",
		string(data[:headSize]),
		omitted,
		string(data[tailStart:]),
	)
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

func writeWebFetchCacheFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create web fetch artifacts directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("failed to secure web fetch artifacts directory %q: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, ".keen-web-fetch-cache-*")
	if err != nil {
		return fmt.Errorf("failed to create web fetch cache file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(webFetchArtifactFileMode); err != nil {
		temp.Close()
		return fmt.Errorf("failed to secure web fetch cache file %q: %w", tempPath, err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("failed to write web fetch cache file %q: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to write web fetch cache file %q: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to store web fetch cache file %q: %w", path, err)
	}
	return nil
}

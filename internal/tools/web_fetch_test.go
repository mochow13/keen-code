package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.Name() != "web_fetch" {
		t.Errorf("expected name 'web_fetch', got %q", tool.Name())
	}
}

func TestWebFetchTool_Description(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
}

func TestWebFetchTool_InputSchema(t *testing.T) {
	tool := NewWebFetchTool()
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Error("schema type should be 'object'")
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}

	if _, ok := properties["url"]; !ok {
		t.Error("url property should exist")
	}
	if _, ok := properties["checkCache"]; !ok {
		t.Error("checkCache property should exist")
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required should be a []string")
	}

	if len(required) != 1 || required[0] != "url" {
		t.Errorf("expected required=[\"url\"], got %v", required)
	}

	if schema["additionalProperties"] != false {
		t.Error("additionalProperties should be false")
	}
}

func TestWebFetchTool_ValidateInput_MissingURL(t *testing.T) {
	tool := NewWebFetchTool()
	err := tool.ValidateInput(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for missing url")
	}
}

func TestWebFetchTool_ValidateInput_InvalidURLType(t *testing.T) {
	tool := NewWebFetchTool()
	err := tool.ValidateInput(context.Background(), map[string]any{"url": 42})
	if err == nil {
		t.Error("expected error for non-string url")
	}
}

func TestWebFetchTool_ValidateInput_InvalidInput(t *testing.T) {
	tool := NewWebFetchTool()
	err := tool.ValidateInput(context.Background(), "not a map")
	if err == nil {
		t.Error("expected error for invalid input type")
	}
}

func TestWebFetchTool_Execute_HTMLResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><h1>Hello World</h1><p>Some <strong>text</strong> here.</p></body></html>`))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := result.(map[string]any)
	if output["status_code"] != 200 {
		t.Errorf("expected status_code 200, got %v", output["status_code"])
	}

	content, ok := output["content"].(string)
	if !ok || content == "" {
		t.Fatal("expected non-empty content string")
	}

	if strings.Contains(content, "<h1>") || strings.Contains(content, "<p>") {
		t.Errorf("expected HTML to be converted to Markdown, got raw HTML: %q", content)
	}

	if !strings.Contains(content, "Hello World") {
		t.Errorf("expected content to contain 'Hello World', got %q", content)
	}
}

func TestWebFetchTool_Execute_PlainTextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("just plain text"))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := result.(map[string]any)
	content, _ := output["content"].(string)
	if content != "just plain text" {
		t.Errorf("expected plain text returned as-is, got %q", content)
	}
}

func TestWebFetchTool_Execute_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("expected no error for non-2xx response, got: %v", err)
	}

	output := result.(map[string]any)
	if output["status_code"] != 404 {
		t.Errorf("expected status_code 404, got %v", output["status_code"])
	}

	content, _ := output["content"].(string)
	if content != "not found" {
		t.Errorf("expected body returned for non-2xx, got %q", content)
	}
}

func TestWebFetchTool_Execute_LargeResponseSpillsToArtifact(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	large := strings.Repeat("a", maxInlineWebFetchSize+1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(large))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := result.(map[string]any)
	if output["truncated"] != true {
		t.Errorf("truncated = %v, want true", output["truncated"])
	}

	artifactPath, ok := output["artifact_path"].(string)
	if !ok || artifactPath == "" {
		t.Fatal("artifact_path missing or empty")
	}

	content, ok := output["content"].(string)
	if !ok || !strings.Contains(content, "...") {
		t.Error("content preview missing omission marker")
	}

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("failed to read artifact %q: %v", artifactPath, err)
	}
	if string(data) != large {
		t.Errorf("artifact content length = %d, want %d", len(data), len(large))
	}
}

func TestWebFetchTool_CheckCacheMissFallsThroughToLiveFetch(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("fresh"))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"url":        server.URL,
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1", hits.Load())
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["content"] != "fresh" {
		t.Errorf("content = %v, want %q", m["content"], "fresh")
	}
	if _, exists := m["cached_at"]; exists {
		t.Errorf("fresh response must not include cached_at")
	}
}

func TestWebFetchTool_MemoryCacheHitSkipsLiveFetch(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("cached payload"))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	if _, err := tool.Execute(context.Background(), map[string]any{"url": server.URL}); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"url":        server.URL,
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("cached Execute() error = %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("server hits = %d, want 1 (cache hit must not fetch)", hits.Load())
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["content"] != "cached payload" {
		t.Errorf("content = %v, want %q", m["content"], "cached payload")
	}
	if m["status_code"] != http.StatusOK {
		t.Errorf("status_code = %v, want 200", m["status_code"])
	}
	cachedAt, ok := m["cached_at"].(string)
	if !ok {
		t.Fatalf("cached_at missing on cached response")
	}
	if _, err := time.Parse(time.RFC3339, cachedAt); err != nil {
		t.Errorf("cached_at = %q is not RFC3339: %v", cachedAt, err)
	}
}

func TestWebFetchTool_DiskCacheHitReturnsPreviewAndPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	large := strings.Repeat("x", maxInlineWebFetchSize+1)
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(large))
	}))
	defer server.Close()

	first := NewWebFetchTool()
	if _, err := first.Execute(context.Background(), map[string]any{"url": server.URL}); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	second := NewWebFetchTool()
	result, err := second.Execute(context.Background(), map[string]any{
		"url":        server.URL,
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("cached Execute() error = %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("server hits = %d, want 1 (disk cache hit must not fetch)", hits.Load())
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["truncated"] != true {
		t.Errorf("truncated = %v, want true", m["truncated"])
	}
	preview, ok := m["content"].(string)
	if !ok || !strings.Contains(preview, "bytes omitted") {
		t.Errorf("cached large result must return a preview")
	}
	path, ok := m["artifact_path"].(string)
	if !ok || path == "" {
		t.Fatalf("artifact_path missing on cached large result")
	}
	wantSuffix := webFetchCacheFilePrefix + webFetchCacheKey(server.URL) + ".txt"
	if !strings.HasSuffix(path, wantSuffix) {
		t.Errorf("artifact_path = %q, want deterministic cache file ending in %q", path, wantSuffix)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read cache file %q: %v", path, err)
	}
	if string(data) != large {
		t.Errorf("cache file content mismatch")
	}
	if _, ok := m["cached_at"].(string); !ok {
		t.Errorf("cached_at missing on cached response")
	}
}

func TestWebFetchTool_FreshCallRefreshesCache(t *testing.T) {
	responses := []string{"first", "second"}
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(responses[hits.Add(1)-1]))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	for range responses {
		if _, err := tool.Execute(context.Background(), map[string]any{"url": server.URL}); err != nil {
			t.Fatalf("fresh Execute() error = %v", err)
		}
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"url":        server.URL,
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("cached Execute() error = %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("server hits = %d, want 2", hits.Load())
	}
	m := result.(map[string]any)
	if m["content"] != "second" {
		t.Errorf("cache was not refreshed, content = %v, want %q", m["content"], "second")
	}
}

func TestWebFetchTool_NonOKStatusNotCached(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	for range 2 {
		result, err := tool.Execute(context.Background(), map[string]any{
			"url":        server.URL,
			"checkCache": true,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		m := result.(map[string]any)
		if m["status_code"] != http.StatusNotFound {
			t.Errorf("status_code = %v, want 404", m["status_code"])
		}
		if m["content"] != "not found" {
			t.Errorf("content = %v, want %q", m["content"], "not found")
		}
		if _, exists := m["cached_at"]; exists {
			t.Errorf("non-200 response must not include cached_at")
		}
	}
	if hits.Load() != 2 {
		t.Errorf("server hits = %d, want 2 (non-200 responses must not be cached)", hits.Load())
	}
}

func TestWebFetchTool_MemoryCacheLRUEviction(t *testing.T) {
	tool := NewWebFetchTool()

	keys := make([]string, 0, webFetchCacheLRUCapacity+1)
	for i := 0; i <= webFetchCacheLRUCapacity; i++ {
		key := webFetchCacheKey(fmt.Sprintf("https://example.com/%d", i))
		if _, err := tool.storeCache(key, "payload"); err != nil {
			t.Fatalf("storeCache() error = %v", err)
		}
		keys = append(keys, key)
	}

	if _, ok := tool.cache.Get(keys[0]); ok {
		t.Errorf("oldest entry should have been evicted")
	}
	if _, ok := tool.cache.Get(keys[len(keys)-1]); !ok {
		t.Errorf("newest entry should remain cached")
	}
}

func TestWebFetchCacheKeyIsDeterministic(t *testing.T) {
	key := webFetchCacheKey("https://example.com/docs")

	if key != webFetchCacheKey("https://example.com/docs") {
		t.Errorf("key must be stable for identical URLs")
	}
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32 hex characters", len(key))
	}
	if key == webFetchCacheKey("https://example.com/other") {
		t.Errorf("URL path must affect the key")
	}
	if key == webFetchCacheKey("https://example.com/docs#section") {
		t.Errorf("URL fragment must affect the key")
	}
}

func TestWebFetchTool_ValidateInputCheckCacheMustBeBool(t *testing.T) {
	tool := NewWebFetchTool()

	err := tool.ValidateInput(context.Background(), map[string]any{
		"url":        "https://example.com",
		"checkCache": "yes",
	})
	if err == nil {
		t.Error("expected error for non-bool checkCache")
	}
}

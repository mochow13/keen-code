package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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

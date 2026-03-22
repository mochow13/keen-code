package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/keen-code/internal/filesystem"
)

func TestWriteFileTool_Name(t *testing.T) {
	tool := NewWriteFileTool(nil, nil, nil)
	if tool.Name() != "write_file" {
		t.Errorf("expected name 'write_file', got %q", tool.Name())
	}
}

func TestWriteFileTool_Description(t *testing.T) {
	tool := NewWriteFileTool(nil, nil, nil)
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
}

func TestWriteFileTool_InputSchema(t *testing.T) {
	tool := NewWriteFileTool(nil, nil, nil)
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Error("schema type should be 'object'")
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}

	pathProp, ok := properties["path"].(map[string]any)
	if !ok {
		t.Fatal("path property should be a map")
	}

	if pathProp["type"] != "string" {
		t.Error("path type should be 'string'")
	}

	contentProp, ok := properties["content"].(map[string]any)
	if !ok {
		t.Fatal("content property should be a map")
	}

	if contentProp["type"] != "string" {
		t.Error("content type should be 'string'")
	}

	required, ok := schema["required"].([]string)
	if !ok {
		requiredIface, ok := schema["required"].([]interface{})
		if !ok {
			t.Fatal("required should be a slice")
		}
		if len(requiredIface) != 2 {
			t.Errorf("expected 2 required fields, got %d", len(requiredIface))
		}
	} else {
		if len(required) != 2 {
			t.Errorf("expected 2 required fields, got %d", len(required))
		}
	}

	if schema["additionalProperties"] != false {
		t.Error("additionalProperties should be false")
	}
}

func TestWriteFileTool_Execute_InvalidInput(t *testing.T) {
	tool := NewWriteFileTool(nil, nil, nil)
	ctx := context.Background()

	tests := []struct {
		name  string
		input any
	}{
		{"nil input", nil},
		{"string input", "not a map"},
		{"int input", 42},
		{"missing path", map[string]any{"content": "test"}},
		{"non-string path", map[string]any{"path": 123, "content": "test"}},
		{"empty path", map[string]any{"path": "", "content": "test"}},
		{"missing content", map[string]any{"path": "/tmp/test.txt"}},
		{"non-string content", map[string]any{"path": "/tmp/test.txt", "content": 123}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(ctx, tt.input)
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

func TestWriteFileTool_Execute_GrantedWrite(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Hello, World!"

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewWriteFileTool(guard, nil, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "content": content}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["path"] != testFile {
		t.Errorf("expected path %q, got %q", testFile, resultMap["path"])
	}

	if resultMap["bytes_written"] != len(content) {
		t.Errorf("expected bytes_written %d, got %v", len(content), resultMap["bytes_written"])
	}

	if resultMap["created"] != true {
		t.Errorf("expected created true for new file, got %v", resultMap["created"])
	}

	writtenContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(writtenContent) != content {
		t.Errorf("expected content %q, got %q", content, string(writtenContent))
	}
}

func TestWriteFileTool_Execute_PendingWrite_Allow(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	content := "Hello from other dir!"

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewWriteFileTool(guard, nil, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "content": content}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mockPR.called {
		t.Error("permission requester should have been called")
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["bytes_written"] != len(content) {
		t.Errorf("expected bytes_written %d, got %v", len(content), resultMap["bytes_written"])
	}

	writtenContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(writtenContent) != content {
		t.Errorf("expected content %q, got %q", content, string(writtenContent))
	}
}

func TestWriteFileTool_Execute_PendingWrite_Deny(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	content := "test content"

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: false}
	tool := NewWriteFileTool(guard, nil, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "content": content}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for denied permission")
	}

	if !mockPR.called {
		t.Error("permission requester should have been called")
	}

	if err.Error() != fmt.Sprintf("permission denied by user: write access rejected for path %q", testFile) {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestWriteFileTool_Execute_PermissionDenied(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewWriteFileTool(guard, nil, nil)
	ctx := context.Background()

	input := map[string]any{"path": "/etc/test.txt", "content": "test"}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for blocked path")
	}

	if err.Error() != "permission denied by policy: path \"/etc/test.txt\" is blocked" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestWriteFileTool_Execute_CreateParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nested", "deep", "test.txt")
	content := "Nested file content"

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewWriteFileTool(guard, nil, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "content": content}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["created"] != true {
		t.Errorf("expected created true, got %v", resultMap["created"])
	}

	writtenContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(writtenContent) != content {
		t.Errorf("expected content %q, got %q", content, string(writtenContent))
	}
}

func TestWriteFileTool_Execute_OverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "existing.txt")
	originalContent := "Original content"
	newContent := "New content"

	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewWriteFileTool(guard, nil, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "content": newContent}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["created"] != false {
		t.Errorf("expected created false for overwritten file, got %v", resultMap["created"])
	}

	writtenContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(writtenContent) != newContent {
		t.Errorf("expected content %q, got %q", newContent, string(writtenContent))
	}
}

func TestWriteFileTool_Execute_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := "relative.txt"
	content := "Relative path content"

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewWriteFileTool(guard, nil, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "content": content}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	expectedPath := filepath.Join(tmpDir, testFile)
	if resultMap["path"] != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, resultMap["path"])
	}

	writtenContent, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(writtenContent) != content {
		t.Errorf("expected content %q, got %q", content, string(writtenContent))
	}
}

func TestWriteFileTool_Execute_EmptyContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")
	content := ""

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewWriteFileTool(guard, nil, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "content": content}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["bytes_written"] != 0 {
		t.Errorf("expected bytes_written 0, got %v", resultMap["bytes_written"])
	}

	writtenContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(writtenContent) != "" {
		t.Errorf("expected empty content, got %q", string(writtenContent))
	}
}

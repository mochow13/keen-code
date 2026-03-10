package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/keen-code/internal/filesystem"
)

type mockPermissionRequester struct {
	allow  bool
	called bool
}

func (m *mockPermissionRequester) RequestPermission(ctx context.Context, toolName, path, resolvedPath, operation string, isDangerous bool) (bool, error) {
	m.called = true
	return m.allow, nil
}

func TestReadFileTool_Name(t *testing.T) {
	tool := NewReadFileTool(nil, nil)
	if tool.Name() != "read_file" {
		t.Errorf("expected name 'read_file', got %q", tool.Name())
	}
}

func TestReadFileTool_Description(t *testing.T) {
	tool := NewReadFileTool(nil, nil)
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
}

func TestReadFileTool_InputSchema(t *testing.T) {
	tool := NewReadFileTool(nil, nil)
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

	if schema["additionalProperties"] != false {
		t.Error("additionalProperties should be false")
	}
}

func TestReadFileTool_Execute_InvalidInput(t *testing.T) {
	tool := NewReadFileTool(nil, nil)
	ctx := context.Background()

	tests := []struct {
		name  string
		input any
	}{
		{"nil input", nil},
		{"string input", "not a map"},
		{"int input", 42},
		{"missing path", map[string]any{}},
		{"non-string path", map[string]any{"path": 123}},
		{"empty path", map[string]any{"path": ""}},
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

func TestReadFileTool_Execute_GrantedRead(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["content"] != content {
		t.Errorf("expected content %q, got %q", content, resultMap["content"])
	}

	if resultMap["bytes_read"] != len(content) {
		t.Errorf("expected bytes_read %d, got %v", len(content), resultMap["bytes_read"])
	}
}

func TestReadFileTool_Execute_PendingRead_Allow(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	content := "Hello from other dir!"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewReadFileTool(guard, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
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

	if resultMap["content"] != content {
		t.Errorf("expected content %q, got %q", content, resultMap["content"])
	}
}

func TestReadFileTool_Execute_PendingRead_Deny(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: false}
	tool := NewReadFileTool(guard, mockPR)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for denied permission")
	}

	if !mockPR.called {
		t.Error("permission requester should have been called")
	}

	if err.Error() != fmt.Sprintf("permission denied by user: read access rejected for path %q", testFile) {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestReadFileTool_Execute_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentFile := filepath.Join(tmpDir, "nonexistent.txt")

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": nonExistentFile}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestReadFileTool_Execute_FileTooLarge(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.txt")

	largeContent := make([]byte, maxFileSize+1)
	if err := os.WriteFile(testFile, largeContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for large file")
	}
}

func TestReadFileTool_Execute_NotAFile(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": subDir}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for directory")
	}
}

func TestReadFileTool_Execute_BinaryFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "binary.bin")

	binaryContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if err := os.WriteFile(testFile, binaryContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for binary file")
	}
}

func TestReadFileTool_Execute_NullByteContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "null.txt")

	content := []byte("Hello\x00World")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for null byte content")
	}
}

func TestReadFileTool_Execute_InvalidUTF8(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.txt")

	content := []byte("Hello\xff\xfeWorld")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for invalid UTF-8")
	}
}

func TestReadFileTool_Execute_JSONFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")
	content := `{"key": "value"}`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["content"] != content {
		t.Errorf("expected content %q, got %q", content, resultMap["content"])
	}
}

func TestReadFileTool_Execute_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := "test.txt"
	content := "Hello, relative path!"
	if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["content"] != content {
		t.Errorf("expected content %q, got %q", content, resultMap["content"])
	}
}

func TestContainsNullByte(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{"no null", []byte("Hello World"), false},
		{"has null", []byte("Hello\x00World"), true},
		{"null at start", []byte("\x00Hello"), true},
		{"null at end", []byte("Hello\x00"), true},
		{"only null", []byte("\x00"), true},
		{"empty", []byte{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsNullByte(tt.content)
			if result != tt.expected {
				t.Errorf("containsNullByte(%q) = %v, expected %v", tt.content, result, tt.expected)
			}
		})
	}
}

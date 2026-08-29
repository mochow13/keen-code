package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/filesystem"
)

type mockPermissionRequester struct {
	allow  bool
	called bool
}

func (m *mockPermissionRequester) RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error) {
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

	offsetProp, ok := properties["offset"].(map[string]any)
	if !ok {
		t.Fatal("offset property should be a map")
	}

	if offsetProp["type"] != "integer" {
		t.Error("offset type should be 'integer'")
	}
	limitProp, ok := properties["limit"].(map[string]any)
	if !ok {
		t.Fatal("limit property should be a map")
	}

	if limitProp["type"] != "integer" {
		t.Error("limit type should be 'integer'")
	}

	if schema["additionalProperties"] != false {
		t.Error("additionalProperties should be false")
	}
}

func TestReadFileTool_ValidateInput_InvalidInput(t *testing.T) {
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
		{"zero offset", map[string]any{"path": "test.txt", "offset": 0}},
		{"fractional offset", map[string]any{"path": "test.txt", "offset": 1.5}},
		{"zero limit", map[string]any{"path": "test.txt", "limit": 0}},
		{"string limit", map[string]any{"path": "test.txt", "limit": "10"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.ValidateInput(ctx, tt.input)
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

	expectedContent := "1:5ae|Hello, World!"
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
	}

	if resultMap["bytes_read"] != len(content) {
		t.Errorf("expected bytes_read %d, got %v", len(content), resultMap["bytes_read"])
	}
	if resultMap["lines_read"] != 1 {
		t.Errorf("expected lines_read 1, got %v", resultMap["lines_read"])
	}

	if resultMap["total_lines"] != 1 {
		t.Errorf("expected total_lines 1, got %v", resultMap["total_lines"])
	}

	if resultMap["truncated"] != false {
		t.Errorf("expected truncated false, got %v", resultMap["truncated"])
	}
}

func TestReadFileTool_Execute_KeenBashOutputReadDoesNotRequestPermission(t *testing.T) {
	workingDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bashDir := filepath.Join(home, ".keen", "bash")
	if err := os.MkdirAll(bashDir, 0700); err != nil {
		t.Fatalf("failed to create bash output dir: %v", err)
	}

	testFile := filepath.Join(bashDir, "keen-bash-random.stdout")
	content := "captured output"
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create bash output file: %v", err)
	}

	guard := filesystem.NewGuard(workingDir, nil)
	mockPR := &mockPermissionRequester{allow: false}
	tool := NewReadFileTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{"path": testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockPR.called {
		t.Fatal("permission requester should not have been called")
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}
	if resultMap["content"] != "1:8e1|captured output" {
		t.Fatalf("unexpected content: %v", resultMap["content"])
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

	expectedContent := "1:eca|Hello from other dir!"
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
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
		t.Fatal("expected error for non-existent file")
	}
	if got, want := err.Error(), fmt.Sprintf("not found: file %q does not exist", nonExistentFile); got != want {
		t.Errorf("expected error %q, got %q", want, got)
	}
}

func TestReadFileContent_FileRemovedBeforeRead(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentFile := filepath.Join(tmpDir, "nonexistent.txt")

	_, err := readFileContent(nonExistentFile)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if got, want := err.Error(), fmt.Sprintf("not found: file %q does not exist", nonExistentFile); got != want {
		t.Errorf("expected error %q, got %q", want, got)
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

	expectedContent := `1:1f2|{"key": "value"}`
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
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

	expectedContent := "1:e21|Hello, relative path!"
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
	}
}

func TestReadFileTool_Execute_OffsetAndLimit(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line one\nline two\nline three\nline four\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "offset": 2, "limit": 2}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	expectedContent := "2:deb|line two\n3:8c8|line three"
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
	}

	if resultMap["total_lines"] != 4 {
		t.Errorf("expected total_lines 4, got %v", resultMap["total_lines"])
	}

	if resultMap["lines_read"] != 2 || resultMap["bytes_read"] != len("line two")+len("line three") {
		t.Errorf("expected 2 lines and 18 bytes read, got %#v", resultMap)
	}

	if resultMap["truncated"] != true {
		t.Errorf("expected truncated true, got %v", resultMap["truncated"])
	}
}

func TestReadFileTool_Execute_OffsetOutOfRange(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("only line"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"path": testFile, "offset": 2}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for out-of-range offset")
	}
}

func TestReadFileTool_Execute_DuplicateLinesStayDistinct(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "return nil\nreturn nil\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)

	result, err := tool.Execute(context.Background(), map[string]any{"path": testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultMap := result.(map[string]any)

	// Identical content yields the same hash on both lines; the line number
	// is what makes each anchor distinct.
	expectedContent := "1:48d|return nil\n2:48d|return nil"
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
	}
}

func TestReadFileTool_Execute_BlankLines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "a\n\nb\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)

	result, err := tool.Execute(context.Background(), map[string]any{"path": testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultMap := result.(map[string]any)

	expectedContent := "1:e40|a\n2:811|\n3:e70|b"
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
	}
}

func TestReadFileTool_Execute_LongLineHashCoversFullContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	longLine := strings.Repeat("x", maxLineLength+100)
	if err := os.WriteFile(testFile, []byte(longLine), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)

	result, err := tool.Execute(context.Background(), map[string]any{"path": testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultMap := result.(map[string]any)

	fullHash := computeLineHash([]byte(longLine))
	truncatedDisplay := strings.Repeat("x", maxLineLength) + "... (line truncated)"
	if fullHash == computeLineHash([]byte(truncatedDisplay)) {
		t.Fatal("test fixture is invalid: truncated display must hash differently from the full line")
	}

	expectedContent := "1:" + fullHash + "|" + truncatedDisplay
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
	}
}

func TestReadFileTool_Execute_CRLF(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "line one\r\nline two\r\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)

	result, err := tool.Execute(context.Background(), map[string]any{"path": testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultMap := result.(map[string]any)

	// CRLF's \r is part of the delimiter: it is excluded from both the
	// displayed line and its hash, matching splitRawLines.
	expectedContent := "1:dab|line one\n2:deb|line two"
	if resultMap["content"] != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, resultMap["content"])
	}
}

func TestReadFileTool_Execute_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewReadFileTool(guard, nil)

	result, err := tool.Execute(context.Background(), map[string]any{"path": testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultMap := result.(map[string]any)

	if resultMap["content"] != "" {
		t.Errorf("expected empty content, got %q", resultMap["content"])
	}
	if resultMap["total_lines"] != 0 {
		t.Errorf("expected total_lines 0, got %v", resultMap["total_lines"])
	}

	if resultMap["lines_read"] != 0 || resultMap["bytes_read"] != 0 {
		t.Errorf("expected no content read, got %#v", resultMap)
	}
	if resultMap["truncated"] != false {
		t.Errorf("expected truncated false, got %v", resultMap["truncated"])
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

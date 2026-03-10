package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/keen-code/internal/filesystem"
)

func TestGlobTool_Name(t *testing.T) {
	tool := NewGlobTool(nil, nil)
	if tool.Name() != "glob" {
		t.Errorf("expected name 'glob', got %q", tool.Name())
	}
}

func TestGlobTool_Description(t *testing.T) {
	tool := NewGlobTool(nil, nil)
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
}

func TestGlobTool_InputSchema(t *testing.T) {
	tool := NewGlobTool(nil, nil)
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Error("schema type should be 'object'")
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}

	patternProp, ok := properties["pattern"].(map[string]any)
	if !ok {
		t.Fatal("pattern property should be a map")
	}

	if patternProp["type"] != "string" {
		t.Error("pattern type should be 'string'")
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

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required should be a string slice")
	}

	if len(required) != 1 || required[0] != "pattern" {
		t.Error("only 'pattern' should be required")
	}
}

func TestGlobTool_Execute_InvalidInput(t *testing.T) {
	tool := NewGlobTool(nil, nil)
	ctx := context.Background()

	tests := []struct {
		name  string
		input any
	}{
		{"nil input", nil},
		{"string input", "not a map"},
		{"int input", 42},
		{"missing pattern", map[string]any{}},
		{"non-string pattern", map[string]any{"pattern": 123}},
		{"empty pattern", map[string]any{"pattern": ""}},
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

func TestGlobTool_Execute_InvalidPattern(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGlobTool(guard, nil)
	ctx := context.Background()

	tests := []struct {
		name    string
		pattern string
	}{
		{"unclosed bracket", "[abc"},
		{"bad syntax", "**[*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]any{"pattern": tt.pattern}
			_, err := tool.Execute(ctx, input)
			if err == nil {
				t.Error("expected error for invalid pattern")
			}
		})
	}
}

func TestGlobTool_Execute_GrantedSearch(t *testing.T) {
	tmpDir := t.TempDir()

	testFile1 := filepath.Join(tmpDir, "test1.go")
	testFile2 := filepath.Join(tmpDir, "test2.go")
	testFile3 := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(testFile1, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile3, []byte("text"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGlobTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"pattern": "*.go"}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["pattern"] != "*.go" {
		t.Errorf("expected pattern '*.go', got %q", resultMap["pattern"])
	}

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be a string slice")
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}

	count, ok := resultMap["count"].(int)
	if !ok {
		t.Fatal("count should be an int")
	}

	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestGlobTool_Execute_RecursivePattern(t *testing.T) {
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	nestedDir := filepath.Join(subDir, "nested")
	if err := os.Mkdir(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested directory: %v", err)
	}

	testFile1 := filepath.Join(tmpDir, "root.md")
	testFile2 := filepath.Join(subDir, "sub.md")
	testFile3 := filepath.Join(nestedDir, "nested.md")

	if err := os.WriteFile(testFile1, []byte("root"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("sub"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile3, []byte("nested"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGlobTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"pattern": "**/*.md"}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be a string slice")
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(files), files)
	}
}

func TestGlobTool_Execute_PendingSearch_Allow(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()

	testFile := filepath.Join(otherDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewGlobTool(guard, mockPR)
	ctx := context.Background()

	input := map[string]any{
		"pattern": "*.txt",
		"path":    otherDir,
	}
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

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be a string slice")
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestGlobTool_Execute_PendingSearch_Deny(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()

	testFile := filepath.Join(otherDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: false}
	tool := NewGlobTool(guard, mockPR)
	ctx := context.Background()

	input := map[string]any{
		"pattern": "*.txt",
		"path":    otherDir,
	}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for denied permission")
	}

	if !mockPR.called {
		t.Error("permission requester should have been called")
	}

	if err.Error() != fmt.Sprintf("permission denied by user: read access rejected for path %q", otherDir) {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGlobTool_Execute_NoMatches(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGlobTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"pattern": "*.go"}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be a string slice")
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}

	count, ok := resultMap["count"].(int)
	if !ok {
		t.Fatal("count should be an int")
	}

	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

func TestGlobTool_Execute_AbsolutePathPattern(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()

	testFile := filepath.Join(otherDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	mockPR := &mockPermissionRequester{allow: true}
	tool := NewGlobTool(guard, mockPR)
	ctx := context.Background()

	input := map[string]any{
		"pattern": "*.txt",
		"path":    otherDir,
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be a string slice")
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestGlobTool_Execute_FileLimit(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 0; i <= maxFileLimit; i++ {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("test%d.txt", i))
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGlobTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"pattern": "*.txt"}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for exceeding file limit")
	}

	if err.Error() != fmt.Sprintf("search failed: search too broad: found more than %d files matching pattern \"*.txt\"", maxFileLimit) {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGlobTool_Execute_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	testFile := filepath.Join(subDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGlobTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{
		"pattern": "*.txt",
		"path":    "subdir",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be a string slice")
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestGlobTool_Execute_ComplexPattern(t *testing.T) {
	tmpDir := t.TempDir()

	testDir := filepath.Join(tmpDir, "src")
	if err := os.Mkdir(testDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	testFile1 := filepath.Join(testDir, "main_test.go")
	testFile2 := filepath.Join(testDir, "helper_test.go")
	testFile3 := filepath.Join(testDir, "main.go")

	if err := os.WriteFile(testFile1, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile3, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGlobTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"pattern": "src/**/*_test.go"}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be a string slice")
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(files), files)
	}
}

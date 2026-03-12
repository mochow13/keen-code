package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/keen-code/internal/filesystem"
)

type mockGrepPermissionRequester struct {
	allow bool
}

func (m *mockGrepPermissionRequester) RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error) {
	return m.allow, nil
}

func TestGrepTool_Name(t *testing.T) {
	guard := filesystem.NewGuard("/tmp", nil)
	tool := NewGrepTool(guard, nil)

	if got := tool.Name(); got != "grep" {
		t.Errorf("Name() = %q, want %q", got, "grep")
	}
}

func TestGrepTool_Description(t *testing.T) {
	guard := filesystem.NewGuard("/tmp", nil)
	tool := NewGrepTool(guard, nil)

	if got := tool.Description(); got == "" {
		t.Error("Description() should not be empty")
	}
}

func TestGrepTool_InputSchema(t *testing.T) {
	guard := filesystem.NewGuard("/tmp", nil)
	tool := NewGrepTool(guard, nil)

	schema := tool.InputSchema()
	if schema == nil {
		t.Fatal("InputSchema() returned nil")
	}

	if schema["type"] != "object" {
		t.Errorf("schema[type] = %v, want 'object'", schema["type"])
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema[properties] should be map[string]any")
	}

	if _, ok := properties["pattern"]; !ok {
		t.Error("schema should have 'pattern' property")
	}

	if _, ok := properties["path"]; !ok {
		t.Error("schema should have 'path' property")
	}

	if _, ok := properties["include"]; !ok {
		t.Error("schema should have 'include' property")
	}

	if _, ok := properties["output_mode"]; !ok {
		t.Error("schema should have 'output_mode' property")
	}

	required, ok := schema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "pattern" {
		t.Error("schema[required] should be ['pattern']")
	}
}

func TestGrepTool_Execute_InvalidInput(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	tests := []struct {
		name  string
		input any
	}{
		{
			name:  "nil input",
			input: nil,
		},
		{
			name:  "string input",
			input: "not a map",
		},
		{
			name:  "missing pattern",
			input: map[string]any{},
		},
		{
			name:  "empty pattern",
			input: map[string]any{"pattern": ""},
		},
		{
			name:  "non-string pattern",
			input: map[string]any{"pattern": 123},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(ctx, tt.input)
			if err == nil {
				t.Error("Execute() should return error for invalid input")
			}
		})
	}
}

func TestGrepTool_Execute_InvalidPattern(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{"pattern": "[invalid("}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("Execute() should return error for invalid regex")
	}
}

func TestGrepTool_Execute_InvalidOutputMode(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{
		"pattern":     "test",
		"output_mode": "invalid",
	}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("Execute() should return error for invalid output_mode")
	}
}

func TestGrepTool_Execute_ContentMode(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	content := "line one\nfunc Test() {}\nline three\nfunc Another() {}\n"
	err := os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern":     "^func",
		"output_mode": "content",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	if resultMap["output_mode"] != "content" {
		t.Errorf("output_mode = %v, want 'content'", resultMap["output_mode"])
	}

	matches, ok := resultMap["matches"].([]map[string]any)
	if !ok {
		t.Fatal("matches should be []map[string]any")
	}

	if len(matches) != 2 {
		t.Errorf("len(matches) = %d, want 2", len(matches))
	}

	if resultMap["count"] != 2 {
		t.Errorf("count = %v, want 2", resultMap["count"])
	}
}

func TestGrepTool_Execute_FileMode(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	err := os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("func A() {}"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("func B() {}"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "c.txt"), []byte("not matching"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern":     "func",
		"output_mode": "file",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	if resultMap["output_mode"] != "file" {
		t.Errorf("output_mode = %v, want 'file'", resultMap["output_mode"])
	}

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be []string")
	}

	if len(files) != 2 {
		t.Errorf("len(files) = %d, want 2", len(files))
	}

	if resultMap["count"] != 2 {
		t.Errorf("count = %v, want 2", resultMap["count"])
	}
}

func TestGrepTool_Execute_DefaultContentMode(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("match here"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern": "match",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	if resultMap["output_mode"] != "content" {
		t.Errorf("output_mode = %v, want 'content'", resultMap["output_mode"])
	}

	matches, ok := resultMap["matches"].([]map[string]any)
	if !ok {
		t.Fatal("matches should be []map[string]any")
	}

	if len(matches) != 1 {
		t.Errorf("len(matches) = %d, want 1", len(matches))
	}
}

func TestGrepTool_Execute_IncludeFilter(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	err := os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("test content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("test content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern": "test",
		"include": "*.go",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	matches, ok := resultMap["matches"].([]map[string]any)
	if !ok {
		t.Fatal("matches should be []map[string]any")
	}

	if len(matches) != 1 {
		t.Errorf("len(matches) = %d, want 1", len(matches))
	}

	if matches[0]["file"] != filepath.Join(tmpDir, "a.go") {
		t.Errorf("matched wrong file: %v", matches[0]["file"])
	}
}

func TestGrepTool_Execute_RecursiveSearch(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	subDir := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "root.go"), []byte("func Root() {}"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(subDir, "nested.go"), []byte("func Nested() {}"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern":     "func",
		"output_mode": "file",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	files, ok := resultMap["files"].([]string)
	if !ok {
		t.Fatal("files should be []string")
	}

	if len(files) != 2 {
		t.Errorf("len(files) = %d, want 2", len(files))
	}
}

func TestGrepTool_Execute_NoMatches(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("no matching content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern": "xyznotfound",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	matches, ok := resultMap["matches"].([]map[string]any)
	if !ok {
		t.Fatal("matches should be []map[string]any")
	}

	if len(matches) != 0 {
		t.Errorf("len(matches) = %d, want 0", len(matches))
	}

	if resultMap["count"] != 0 {
		t.Errorf("count = %v, want 0", resultMap["count"])
	}
}

func TestGrepTool_Execute_BinaryFileSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	binaryContent := []byte{0x00, 0x01, 0x02, 0x03}
	err := os.WriteFile(filepath.Join(tmpDir, "binary.bin"), binaryContent, 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "text.txt"), []byte("test content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern": "test",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	matches, ok := resultMap["matches"].([]map[string]any)
	if !ok {
		t.Fatal("matches should be []map[string]any")
	}

	if len(matches) != 1 {
		t.Errorf("len(matches) = %d, want 1", len(matches))
	}

	if matches[0]["file"] != filepath.Join(tmpDir, "text.txt") {
		t.Errorf("matched wrong file: %v", matches[0]["file"])
	}
}

func TestGrepTool_Execute_PendingSearch_Allow(t *testing.T) {
	tmpDir := t.TempDir()
	parentDir := filepath.Dir(tmpDir)
	guard := filesystem.NewGuard(tmpDir, nil)
	requester := &mockGrepPermissionRequester{allow: true}
	tool := NewGrepTool(guard, requester)
	ctx := context.Background()

	testDir := filepath.Join(parentDir, "outside")
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(testDir, "test.txt"), []byte("test content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	relPath, err := filepath.Rel(tmpDir, testDir)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern": "test",
		"path":    relPath,
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	if resultMap["count"] != 1 {
		t.Errorf("count = %v, want 1", resultMap["count"])
	}
}

func TestGrepTool_Execute_PendingSearch_Deny(t *testing.T) {
	tmpDir := t.TempDir()
	parentDir := filepath.Dir(tmpDir)
	guard := filesystem.NewGuard(tmpDir, nil)
	requester := &mockGrepPermissionRequester{allow: false}
	tool := NewGrepTool(guard, requester)
	ctx := context.Background()

	testDir := filepath.Join(parentDir, "outside")
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	relPath, err := filepath.Rel(tmpDir, testDir)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern": "test",
		"path":    relPath,
	}
	_, err = tool.Execute(ctx, input)
	if err == nil {
		t.Error("Execute() should return error when permission denied")
	}
}

func TestGrepTool_Execute_BlockedPath(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{
		"pattern": "test",
		"path":    "/etc",
	}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("Execute() should return error for blocked path")
	}
}

func TestGrepTool_Execute_MatchLimit(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	for i := 0; i < 1001; i++ {
		filename := fmt.Sprintf("file%d.txt", i)
		err := os.WriteFile(filepath.Join(tmpDir, filename), []byte("match"), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	input := map[string]any{
		"pattern":     "match",
		"output_mode": "file",
	}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("Execute() should return error when match limit exceeded")
	}
}

func TestGrepTool_Execute_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(subDir, "test.txt"), []byte("test content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{
		"pattern": "test",
		"path":    "subdir",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	matches, ok := resultMap["matches"].([]map[string]any)
	if !ok {
		t.Fatal("matches should be []map[string]any")
	}

	if len(matches) != 1 {
		t.Errorf("len(matches) = %d, want 1", len(matches))
	}

	if matches[0]["file"] != filepath.Join(subDir, "test.txt") {
		t.Errorf("matched wrong file: %v", matches[0]["file"])
	}
}

func TestGrepTool_Execute_InvalidIncludeGlob(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	input := map[string]any{
		"pattern": "test",
		"include": "[invalid(",
	}
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("Execute() should return error for invalid include glob")
	}
}

func TestGrepTool_Execute_LargeFileSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	guard := filesystem.NewGuard(tmpDir, nil)
	tool := NewGrepTool(guard, nil)
	ctx := context.Background()

	largeContent := make([]byte, maxFileSize+1)
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	err := os.WriteFile(filepath.Join(tmpDir, "large.txt"), largeContent, 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "small.txt"), []byte("test"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"pattern": "x",
	}
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be map[string]any")
	}

	count, ok := resultMap["count"].(int)
	if !ok {
		t.Fatalf("count should be int, got %T", resultMap["count"])
	}

	if count != 0 {
		t.Errorf("count = %d, want 0 (large file should be skipped)", count)
	}
}

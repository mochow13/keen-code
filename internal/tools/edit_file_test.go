package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/keen-code/internal/filesystem"
)

type mockDiffEmitter struct {
	emitted []EditDiffLine
	called  bool
}

func (m *mockDiffEmitter) EmitDiff(lines []EditDiffLine) {
	m.called = true
	m.emitted = lines
}

func TestEditFileTool_Name(t *testing.T) {
	tool := NewEditFileTool(nil, nil, nil)
	if tool.Name() != "edit_file" {
		t.Errorf("expected name 'edit_file', got %q", tool.Name())
	}
}

func TestEditFileTool_Description(t *testing.T) {
	tool := NewEditFileTool(nil, nil, nil)
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
}

func TestEditFileTool_InputSchema(t *testing.T) {
	tool := NewEditFileTool(nil, nil, nil)
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Error("schema type should be 'object'")
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}
	for _, key := range []string{"path", "oldString", "newString", "shouldReplaceAll"} {
		if _, ok := properties[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required should be []string")
	}
	if len(required) != 3 {
		t.Errorf("expected 3 required fields, got %d", len(required))
	}

	if schema["additionalProperties"] != false {
		t.Error("additionalProperties should be false")
	}
}

func TestEditFileTool_Execute_InvalidInput(t *testing.T) {
	tool := NewEditFileTool(nil, nil, nil)
	ctx := context.Background()

	tests := []struct {
		name  string
		input any
	}{
		{"nil input", nil},
		{"string input", "not a map"},
		{"missing path", map[string]any{"oldString": "a", "newString": "b"}},
		{"empty path", map[string]any{"path": "", "oldString": "a", "newString": "b"}},
		{"non-string path", map[string]any{"path": 123, "oldString": "a", "newString": "b"}},
		{"missing oldString", map[string]any{"path": "/tmp/x.txt", "newString": "b"}},
		{"non-string oldString", map[string]any{"path": "/tmp/x.txt", "oldString": 123, "newString": "b"}},
		{"missing newString", map[string]any{"path": "/tmp/x.txt", "oldString": "a"}},
		{"non-string newString", map[string]any{"path": "/tmp/x.txt", "oldString": "a", "newString": 123}},
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

func TestEditFileTool_Execute_SingleReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	guard := newGuard(tmpDir)
	de := &mockDiffEmitter{}
	pr := &mockPermissionRequester{allow: true}
	tool := NewEditFileTool(guard, de, pr)

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":      testFile,
		"oldString": "world",
		"newString": "Go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]any)
	if resultMap["success"] != true {
		t.Error("expected success true")
	}
	if resultMap["replacementCount"] != 1 {
		t.Errorf("expected replacementCount 1, got %v", resultMap["replacementCount"])
	}

	got, _ := os.ReadFile(testFile)
	if string(got) != "hello Go\n" {
		t.Errorf("unexpected file content: %q", string(got))
	}

	if !de.called {
		t.Error("EmitDiff should have been called")
	}
}

func TestEditFileTool_Execute_ReplaceAll(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("foo foo foo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	guard := newGuard(tmpDir)
	de := &mockDiffEmitter{}
	tool := NewEditFileTool(guard, de, &mockPermissionRequester{allow: true})

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":             testFile,
		"oldString":        "foo",
		"newString":        "bar",
		"shouldReplaceAll": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]any)
	if resultMap["replacementCount"] != 3 {
		t.Errorf("expected replacementCount 3, got %v", resultMap["replacementCount"])
	}

	got, _ := os.ReadFile(testFile)
	if string(got) != "bar bar bar\n" {
		t.Errorf("unexpected file content: %q", string(got))
	}
}

func TestEditFileTool_Execute_ReplaceFirst(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("foo foo foo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	guard := newGuard(tmpDir)
	tool := NewEditFileTool(guard, &mockDiffEmitter{}, &mockPermissionRequester{allow: true})

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":      testFile,
		"oldString": "foo",
		"newString": "bar",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]any)
	if resultMap["replacementCount"] != 1 {
		t.Errorf("expected replacementCount 1, got %v", resultMap["replacementCount"])
	}

	got, _ := os.ReadFile(testFile)
	if string(got) != "bar foo foo\n" {
		t.Errorf("unexpected file content: %q", string(got))
	}
}

func TestEditFileTool_Execute_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	guard := newGuard(tmpDir)
	tool := NewEditFileTool(guard, &mockDiffEmitter{}, &mockPermissionRequester{allow: true})

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":      filepath.Join(tmpDir, "nonexistent.txt"),
		"oldString": "foo",
		"newString": "bar",
	})
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestEditFileTool_Execute_OldStringNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	guard := newGuard(tmpDir)
	tool := NewEditFileTool(guard, &mockDiffEmitter{}, &mockPermissionRequester{allow: true})

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":      testFile,
		"oldString": "notpresent",
		"newString": "bar",
	})
	if err == nil {
		t.Error("expected error for missing oldString")
	}
}

func TestEditFileTool_Execute_PermissionDeniedByPolicy(t *testing.T) {
	tmpDir := t.TempDir()
	guard := newGuard(tmpDir)
	tool := NewEditFileTool(guard, &mockDiffEmitter{}, nil)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":      "/etc/hosts",
		"oldString": "localhost",
		"newString": "remotehost",
	})
	if err == nil {
		t.Error("expected error for blocked path")
	}
}

func TestEditFileTool_Execute_PermissionDeniedByUser(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	guard := newGuard(tmpDir)
	de := &mockDiffEmitter{}
	pr := &mockPermissionRequester{allow: false}
	tool := NewEditFileTool(guard, de, pr)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path":      testFile,
		"oldString": "world",
		"newString": "Go",
	})
	if err == nil {
		t.Error("expected error for denied permission")
	}

	if !de.called {
		t.Error("EmitDiff should be called before RequestPermission")
	}
	if !pr.called {
		t.Error("permission requester should have been called")
	}
}

func TestEditFileTool_Execute_EmitDiffBeforePermission(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var order []string
	de := &mockDiffEmitter{}
	origEmit := de.EmitDiff
	_ = origEmit

	callOrder := &callOrderTracker{}
	tool := NewEditFileTool(newGuard(tmpDir), callOrder, callOrder)

	tool.Execute(context.Background(), map[string]any{
		"path":      testFile,
		"oldString": "world",
		"newString": "Go",
	})

	_ = order
	if len(callOrder.calls) < 2 || callOrder.calls[0] != "diff" || callOrder.calls[1] != "permission" {
		t.Errorf("expected diff before permission, got %v", callOrder.calls)
	}
}

type callOrderTracker struct {
	calls []string
}

func (c *callOrderTracker) EmitDiff(lines []EditDiffLine) {
	c.calls = append(c.calls, "diff")
}

func (c *callOrderTracker) RequestPermission(_ context.Context, _, _, _ string, _ bool) (bool, error) {
	c.calls = append(c.calls, "permission")
	return true, nil
}

func TestComputeEditDiff_SingleLineChange(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nchanged\nline3\n"

	lines := computeEditDiff(old, new)
	if len(lines) == 0 {
		t.Fatal("expected non-empty diff")
	}

	var hasHunk, hasRemoved, hasAdded bool
	for _, l := range lines {
		switch l.Kind {
		case DiffLineHunk:
			hasHunk = true
		case DiffLineRemoved:
			hasRemoved = true
			if l.Content != "line2" {
				t.Errorf("expected removed content 'line2', got %q", l.Content)
			}
		case DiffLineAdded:
			hasAdded = true
			if l.Content != "changed" {
				t.Errorf("expected added content 'changed', got %q", l.Content)
			}
		}
	}

	if !hasHunk {
		t.Error("expected at least one hunk header")
	}
	if !hasRemoved {
		t.Error("expected at least one removed line")
	}
	if !hasAdded {
		t.Error("expected at least one added line")
	}
}

func newGuard(dir string) *filesystem.Guard {
	return filesystem.NewGuard(dir, nil)
}

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
	for _, key := range []string{"path", "ops"} {
		if _, ok := properties[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
	for _, legacy := range []string{"oldString", "newString", "shouldReplaceAll"} {
		if _, ok := properties[legacy]; ok {
			t.Errorf("legacy property %q should have been removed", legacy)
		}
	}

	ops, ok := properties["ops"].(map[string]any)
	if !ok {
		t.Fatal("ops property should be a map")
	}
	if ops["type"] != "array" {
		t.Errorf("ops type should be 'array', got %v", ops["type"])
	}
	if ops["minItems"] != 1 {
		t.Errorf("ops minItems should be 1, got %v", ops["minItems"])
	}

	items, ok := ops["items"].(map[string]any)
	if !ok {
		t.Fatal("ops items should be a map")
	}
	opProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatal("op items properties should be a map")
	}
	for _, key := range []string{"op", "start", "end", "text"} {
		if _, ok := opProps[key]; !ok {
			t.Errorf("missing op property %q", key)
		}
	}
	opSchema, ok := opProps["op"].(map[string]any)
	if !ok {
		t.Fatal("op property schema should be a map")
	}
	enum, ok := opSchema["enum"].([]string)
	if !ok {
		t.Fatal("op enum should be []string")
	}
	for _, kind := range []string{"replace", "insert_after", "insert_before", "insert_head", "insert_tail"} {
		found := false
		for _, e := range enum {
			if e == kind {
				found = true
			}
		}
		if !found {
			t.Errorf("op enum missing %q (got %v)", kind, enum)
		}
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required should be []string")
	}
	if len(required) != 2 || required[0] != "path" || required[1] != "ops" {
		t.Errorf("expected required [path ops], got %v", required)
	}

	if schema["additionalProperties"] != false {
		t.Error("additionalProperties should be false")
	}
}

func TestEditFileTool_ValidateInput_InvalidInput(t *testing.T) {
	tool := NewEditFileTool(nil, nil, nil)
	ctx := context.Background()

	tests := []struct {
		name  string
		input any
	}{
		{"nil input", nil},
		{"string input", "not a map"},
		{"missing path", map[string]any{"ops": []any{map[string]any{"start": "1:a3f", "text": "b"}}}},
		{"empty path", map[string]any{"path": "", "ops": []any{map[string]any{"start": "1:a3f", "text": "b"}}}},
		{"non-string path", map[string]any{"path": 123, "ops": []any{map[string]any{"start": "1:a3f", "text": "b"}}}},
		{"missing ops", map[string]any{"path": "/tmp/x.txt"}},
		{"non-array ops", map[string]any{"path": "/tmp/x.txt", "ops": "replace line 3"}},
		{"empty ops", map[string]any{"path": "/tmp/x.txt", "ops": []any{}}},
		{"op not an object", map[string]any{"path": "/tmp/x.txt", "ops": []any{"nope"}}},
		{"non-string op name", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"op": 123, "start": "1:a3f"}}}},
		{"unsupported op name", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"op": "replace_all", "start": "1:a3f"}}}},
		{"missing start for replace", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"text": "b"}}}},
		{"missing start for insert_after", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"op": "insert_after", "text": "b"}}}},
		{"misplaced start on insert_head", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"op": "insert_head", "start": "1:a3f", "text": "b"}}}},
		{"misplaced start on insert_tail", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"op": "insert_tail", "start": "1:a3f", "text": "b"}}}},
		{"misplaced end on insert_after", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"op": "insert_after", "start": "1:a3f", "end": "2:9c1", "text": "b"}}}},
		{"misplaced end on insert_before", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"op": "insert_before", "start": "1:a3f", "end": "2:9c1", "text": "b"}}}},
		{"non-string text", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"start": "1:a3f", "text": 123}}}},
		{"non-string start", map[string]any{"path": "/tmp/x.txt", "ops": []any{map[string]any{"start": 7, "text": "b"}}}},
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

func TestEditFileTool_ValidateInput_ValidOps(t *testing.T) {
	tool := NewEditFileTool(nil, nil, nil)

	valid := []struct {
		name  string
		input map[string]any
	}{
		{
			name:  "single replace with explicit op",
			input: map[string]any{"path": "a.go", "ops": []any{map[string]any{"op": "replace", "start": "1:a3f", "text": "package main"}}},
		},
		{
			name:  "replace defaults op and end optional",
			input: map[string]any{"path": "a.go", "ops": []any{map[string]any{"start": "1:a3f", "end": "3:e47", "text": "package main"}}},
		},
		{
			name: "all insert kinds",
			input: map[string]any{"path": "a.go", "ops": []any{
				map[string]any{"op": "insert_after", "start": "2:9c1", "text": "x"},
				map[string]any{"op": "insert_before", "start": "3:e47", "text": "y"},
				map[string]any{"op": "insert_head", "text": "z"},
				map[string]any{"op": "insert_tail", "text": "w"},
			}},
		},
		{
			name:  "empty text allowed",
			input: map[string]any{"path": "a.go", "ops": []any{map[string]any{"start": "1:a3f"}}},
		},
		{
			name:  "anchor format is opaque during input validation",
			input: map[string]any{"path": "a.go", "ops": []any{map[string]any{"start": "opaque", "end": "also opaque", "text": "x"}}},
		},
	}

	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			if err := tool.ValidateInput(context.Background(), tt.input); err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestEditFileTool_ValidateInput_MissingFieldsAreInstructional(t *testing.T) {
	tool := NewEditFileTool(nil, nil, nil)
	tests := []struct {
		name    string
		input   map[string]any
		missing string
	}{
		{name: "path", input: map[string]any{"ops": []any{map[string]any{"start": "1:a3f", "text": "b"}}}, missing: "path"},
		{name: "ops", input: map[string]any{"path": "a.go"}, missing: "ops"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.ValidateInput(context.Background(), tt.input)
			if err == nil {
				t.Fatal("expected validation error")
			}
			for _, want := range []string{`missing required "` + tt.missing + `" parameter`, "Retry edit_file with all required fields", `"path"`, `"ops"`, "Read the file first", "one ops array"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestParseEditOps_ParsesFields(t *testing.T) {
	ops, err := parseEditOps([]any{
		map[string]any{"start": "1:a3f", "end": "3:e47", "text": "a\nb"},
		map[string]any{"op": "insert_after", "start": "4:9c1", "text": "c"},
		map[string]any{"op": "insert_before", "start": "5:2b8"},
		map[string]any{"op": "insert_head", "text": "h"},
		map[string]any{"op": "insert_tail", "text": "t"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 5 {
		t.Fatalf("expected 5 ops, got %d", len(ops))
	}

	want := []EditOp{
		{Op: "replace", Start: "1:a3f", End: "3:e47", Text: "a\nb"},
		{Op: "insert_after", Start: "4:9c1", Text: "c"},
		{Op: "insert_before", Start: "5:2b8"},
		{Op: "insert_head", Text: "h"},
		{Op: "insert_tail", Text: "t"},
	}
	for i := range want {
		if ops[i] != want[i] {
			t.Errorf("op %d = %+v, want %+v", i, ops[i], want[i])
		}
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
		"path": testFile,
		"ops":  []any{map[string]any{"start": anchorForLine(t, "hello world\n", 1), "text": "hello Go"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]any)
	if resultMap["edited"] != true {
		t.Errorf("expected edited=true, got %#v", resultMap)
	}

	got, _ := os.ReadFile(testFile)
	if string(got) != "hello Go\n" {
		t.Errorf("unexpected file content: %q", string(got))
	}

	if !de.called {
		t.Error("EmitDiff should have been called")
	}
}

func TestEditFileTool_Execute_MultiOpSameFileEdit(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewEditFileTool(newGuard(tmpDir), &mockDiffEmitter{}, &mockPermissionRequester{allow: true})

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": testFile,
		"ops": []any{
			map[string]any{"start": anchorForLine(t, content, 2), "text": "TWO"},
			map[string]any{"start": anchorForLine(t, content, 4), "text": "FOUR"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap := result.(map[string]any)
	if resultMap["edited"] != true {
		t.Errorf("expected edited=true, got %#v", resultMap)
	}

	got, _ := os.ReadFile(testFile)
	if string(got) != "one\nTWO\nthree\nFOUR\n" {
		t.Errorf("unexpected file content: %q", string(got))
	}
}

func TestEditFileTool_Execute_MultiOpEditProducesExpectedDiff(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	de := &mockDiffEmitter{}
	tool := NewEditFileTool(newGuard(tmpDir), de, &mockPermissionRequester{allow: true})

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": testFile,
		"ops": []any{
			map[string]any{"start": anchorForLine(t, content, 2), "text": "TWO"},
			map[string]any{"start": anchorForLine(t, content, 4), "text": "FOUR"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !de.called {
		t.Fatal("EmitDiff should have been called")
	}

	var hasHunk, removedTwo, addedTwo, removedFour, addedFour bool
	for _, l := range de.emitted {
		switch l.Kind {
		case DiffLineHunk:
			hasHunk = true
		case DiffLineRemoved:
			removedTwo = removedTwo || l.Content == "two"
			removedFour = removedFour || l.Content == "four"
		case DiffLineAdded:
			addedTwo = addedTwo || l.Content == "TWO"
			addedFour = addedFour || l.Content == "FOUR"
		}
	}
	if !hasHunk || !removedTwo || !addedTwo || !removedFour || !addedFour {
		t.Errorf("expected unified diff to cover both ops; hunk=%v two(-%v/+%v) four(-%v/+%v)",
			hasHunk, removedTwo, addedTwo, removedFour, addedFour)
	}
}

func TestEditFileTool_Execute_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	guard := newGuard(tmpDir)
	tool := NewEditFileTool(guard, &mockDiffEmitter{}, &mockPermissionRequester{allow: true})

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": filepath.Join(tmpDir, "nonexistent.txt"),
		"ops":  []any{map[string]any{"start": "1:a3f", "text": "bar"}},
	})
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestEditFileTool_Execute_AnchorMismatchRejectsWithoutWriteOrPermission(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	original := "hello world\n"
	if err := os.WriteFile(testFile, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	de := &mockDiffEmitter{}
	pr := &mockPermissionRequester{allow: true}
	tool := NewEditFileTool(newGuard(tmpDir), de, pr)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": testFile,
		"ops":  []any{map[string]any{"start": "1:fff", "text": "bar"}},
	})
	if err == nil || !strings.Contains(err.Error(), "does not exist in the current file snapshot") {
		t.Fatalf("expected missing anchor, got %v", err)
	}
	if de.called {
		t.Error("diff should not be emitted when anchor validation fails")
	}
	if pr.called {
		t.Error("permission should not be requested when anchor validation fails")
	}
	got, _ := os.ReadFile(testFile)
	if string(got) != original {
		t.Errorf("failed validation must not change content, got %q", string(got))
	}
}

func TestEditFileTool_Execute_PermissionDeniedByPolicy(t *testing.T) {
	tmpDir := t.TempDir()
	guard := newGuard(tmpDir)
	tool := NewEditFileTool(guard, &mockDiffEmitter{}, nil)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": "/etc/hosts",
		"ops":  []any{map[string]any{"start": "1:a3f", "text": "remotehost"}},
	})
	if err == nil {
		t.Error("expected error for blocked path")
	}
}

func TestEditFileTool_Execute_PermissionDeniedByUser(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	original := "hello world\n"
	if err := os.WriteFile(testFile, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	guard := newGuard(tmpDir)
	de := &mockDiffEmitter{}
	pr := &mockPermissionRequester{allow: false}
	tool := NewEditFileTool(guard, de, pr)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": testFile,
		"ops":  []any{map[string]any{"start": anchorForLine(t, original, 1), "text": "hello Go"}},
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
	got, _ := os.ReadFile(testFile)
	if string(got) != original {
		t.Errorf("denied permission must leave content unchanged, got %q", string(got))
	}
}

func TestEditFileTool_Execute_EmitDiffBeforePermission(t *testing.T) {
	tmpDir := t.TempDir()
	otherDir := t.TempDir()
	testFile := filepath.Join(otherDir, "test.txt")
	original := "hello world\n"
	if err := os.WriteFile(testFile, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	callOrder := &callOrderTracker{}
	tool := NewEditFileTool(newGuard(tmpDir), callOrder, callOrder)

	tool.Execute(context.Background(), map[string]any{
		"path": testFile,
		"ops":  []any{map[string]any{"start": anchorForLine(t, original, 1), "text": "hello Go"}},
	})

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

func TestEditFileTool_Execute_ProjectMemoryPathRejectsSecret(t *testing.T) {
	tmpDir := t.TempDir()
	memFile := filepath.Join(tmpDir, ".keen", "MEMORY.md")
	original := "- existing note\n"
	if err := os.MkdirAll(filepath.Dir(memFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memFile, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	guard := newGuard(tmpDir)
	tool := NewEditFileTool(guard, &mockDiffEmitter{}, &mockPermissionRequester{allow: true})

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": ".keen/MEMORY.md",
		"ops":  []any{map[string]any{"start": anchorForLine(t, original, 1), "text": "api_key: sk-1234567890abcdefghijklmnopqrstuvwxyz"}},
	})
	if err == nil {
		t.Fatal("expected error editing secret into project memory file")
	}

	got, _ := os.ReadFile(memFile)
	if string(got) != original {
		t.Errorf("secret rejection must leave content unchanged, got %q", string(got))
	}
}

// TestEditFileTool_Execute_WritesThroughSymlinkReferent verifies the atomic
// writer is used: editing via a symlink updates the referent while the symlink
// itself remains a symlink.
func TestEditFileTool_Execute_WritesThroughSymlinkReferent(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "target.txt")
	link := filepath.Join(tmpDir, "link.txt")
	original := "hello world\n"
	if err := os.WriteFile(target, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	tool := NewEditFileTool(newGuard(tmpDir), &mockDiffEmitter{}, &mockPermissionRequester{allow: true})
	_, err := tool.Execute(context.Background(), map[string]any{
		"path": link,
		"ops":  []any{map[string]any{"start": anchorForLine(t, original, 1), "text": "hello Go"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(target)
	if string(got) != "hello Go\n" {
		t.Errorf("referent not updated: %q", string(got))
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected %q to remain a symlink (lstat err %v)", link, err)
	}
}

func newGuard(dir string) *filesystem.Guard {
	return filesystem.NewGuard(dir, nil)
}

// anchorForLine builds the LINE:HASH anchor for the nth line of content.
func anchorForLine(t *testing.T, content string, line int) string {
	t.Helper()
	lines := splitRawLines([]byte(content))
	if line < 1 || line > len(lines) {
		t.Fatalf("line %d out of range for %d-line content %q", line, len(lines), content)
	}
	return fmt.Sprintf("%d:%s", line, computeLineHash(lines[line-1]))
}

func TestApplyEditOps_AppliesOpsToSnapshot(t *testing.T) {
	content := "one\ntwo\nthree\nfour\nfive\n"
	a := func(n int) string { return anchorForLine(t, content, n) }

	tests := []struct {
		name string
		ops  []EditOp
		want string
	}{
		{
			name: "single-line replace",
			ops:  []EditOp{{Op: opReplace, Start: a(2), Text: "TWO"}},
			want: "one\nTWO\nthree\nfour\nfive\n",
		},
		{
			name: "multi-line replace",
			ops:  []EditOp{{Op: opReplace, Start: a(2), Text: "x\ny"}},
			want: "one\nx\ny\nthree\nfour\nfive\n",
		},
		{
			name: "multi-line range replace",
			ops:  []EditOp{{Op: opReplace, Start: a(2), End: a(3), Text: "x\ny"}},
			want: "one\nx\ny\nfour\nfive\n",
		},
		{
			name: "insert_after",
			ops:  []EditOp{{Op: opInsertAfter, Start: a(2), Text: "x\ny"}},
			want: "one\ntwo\nx\ny\nthree\nfour\nfive\n",
		},
		{
			name: "insert_before",
			ops:  []EditOp{{Op: opInsertBefore, Start: a(2), Text: "x\ny"}},
			want: "one\nx\ny\ntwo\nthree\nfour\nfive\n",
		},
		{
			name: "insert_head",
			ops:  []EditOp{{Op: opInsertHead, Text: "x\ny"}},
			want: "x\ny\none\ntwo\nthree\nfour\nfive\n",
		},
		{
			name: "insert_tail",
			ops:  []EditOp{{Op: opInsertTail, Text: "x\ny"}},
			want: "one\ntwo\nthree\nfour\nfive\nx\ny\n",
		},
		{
			name: "deletion via empty text",
			ops:  []EditOp{{Op: opReplace, Start: a(2), Text: ""}},
			want: "one\nthree\nfour\nfive\n",
		},
		{
			name: "range deletion via empty text",
			ops:  []EditOp{{Op: opReplace, Start: a(2), End: a(3), Text: ""}},
			want: "one\nfour\nfive\n",
		},
		{
			name: "reversed range normalized",
			ops:  []EditOp{{Op: opReplace, Start: a(3), End: a(1), Text: "x"}},
			want: "x\nfour\nfive\n",
		},
		{
			name: "same-call multi-spot edits",
			ops: []EditOp{
				{Op: opReplace, Start: a(1), Text: "ONE"},
				{Op: opInsertAfter, Start: a(3), Text: "THREE.5"},
				{Op: opReplace, Start: a(5), Text: "FIVE"},
			},
			want: "ONE\ntwo\nthree\nTHREE.5\nfour\nFIVE\n",
		},
		{
			name: "descending application preserves lower addresses",
			ops: []EditOp{
				{Op: opReplace, Start: a(1), Text: "ONE"},
				{Op: opInsertBefore, Start: a(3), Text: "TWO.5"},
				{Op: opReplace, Start: a(5), End: a(5), Text: "FIVE"},
			},
			want: "ONE\ntwo\nTWO.5\nthree\nfour\nFIVE\n",
		},
		{
			name: "insert before replacement start boundary",
			ops: []EditOp{
				{Op: opInsertBefore, Start: a(2), Text: "before"},
				{Op: opReplace, Start: a(2), End: a(4), Text: "replacement"},
			},
			want: "one\nbefore\nreplacement\nfive\n",
		},
		{
			name: "insert after replacement end boundary",
			ops: []EditOp{
				{Op: opReplace, Start: a(2), End: a(4), Text: "replacement"},
				{Op: opInsertAfter, Start: a(4), Text: "after"},
			},
			want: "one\nreplacement\nafter\nfive\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyEditOps([]byte(content), tt.ops)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("content = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestApplyEditOps_RejectsInvalidAnchors(t *testing.T) {
	content := "one\ntwo\nthree\nfour\nfive\n"
	a := func(n int) string { return anchorForLine(t, content, n) }

	tests := []struct {
		name string
		ops  []EditOp
		want string
	}{
		{
			name: "wrong line number",
			ops:  []EditOp{{Op: opReplace, Start: "99:abc", Text: "x"}},
			want: `op 1: anchor "99:abc" does not exist in the current file snapshot; re-read the file and retry`,
		},
		{
			name: "wrong hash",
			ops:  []EditOp{{Op: opReplace, Start: "2:fff", Text: "x"}},
			want: `op 1: anchor "2:fff" does not exist in the current file snapshot`,
		},
		{
			name: "missing anchor names the failing op",
			ops: []EditOp{
				{Op: opReplace, Start: a(1), Text: "ONE"},
				{Op: opReplace, Start: "3:bad", Text: "y"},
			},
			want: `op 2: anchor "3:bad" does not exist in the current file snapshot`,
		},
		{
			name: "opaque anchor does not exist",
			ops:  []EditOp{{Op: opInsertBefore, Start: "0:abc", Text: "x"}},
			want: `op 1: anchor "0:abc" does not exist in the current file snapshot`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyEditOps([]byte(content), tt.ops)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got != nil {
				t.Errorf("expected no content on error, got %q", string(got))
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}

}

func TestApplyEditOps_AllOrNothingValidation(t *testing.T) {
	content := "one\ntwo\nthree\nfour\n"
	a := func(n int) string { return anchorForLine(t, content, n) }

	_, err := applyEditOps([]byte(content), []EditOp{
		{Op: opReplace, Start: a(1), Text: "ONE"},
		{Op: opReplace, Start: "2:zzz", Text: "TWO"},
	})
	if err == nil || !strings.Contains(err.Error(), "op 2") {
		t.Fatalf("expected failure to name op 2, got %v", err)
	}
}

func TestApplyEditOps_RejectsOverlapsAndContradictions(t *testing.T) {
	content := "one\ntwo\nthree\nfour\nfive\nsix\n"
	a := func(n int) string { return anchorForLine(t, content, n) }

	tests := []struct {
		name string
		ops  []EditOp
		want string
	}{
		{
			name: "overlapping ranges",
			ops: []EditOp{
				{Op: opReplace, Start: a(2), End: a(4), Text: "x"},
				{Op: opReplace, Start: a(3), End: a(5), Text: "y"},
			},
			want: "ops 1 and 2 have overlapping ranges (lines 2-4 and 3-5)",
		},
		{
			name: "insert inside replace range",
			ops: []EditOp{
				{Op: opReplace, Start: a(2), End: a(4), Text: "x"},
				{Op: opInsertAfter, Start: a(3), Text: "y"},
			},
			want: "ops 1 and 2 conflict: insertion at line 3 overlaps replacement range (lines 2-4)",
		},
		{
			name: "two inserts at the same boundary",
			ops: []EditOp{
				{Op: opInsertAfter, Start: a(2), Text: "x"},
				{Op: opInsertBefore, Start: a(3), Text: "y"},
			},
			want: "ops 1 and 2 conflict: both insert at the same position; combine them into one op",
		},
		{
			name: "two inserts at the same anchor",
			ops: []EditOp{
				{Op: opInsertAfter, Start: a(2), Text: "x"},
				{Op: opInsertAfter, Start: a(2), Text: "y"},
			},
			want: "ops 1 and 2 conflict: both insert at the same position; combine them into one op",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyEditOps([]byte(content), tt.ops)
			if err == nil {
				t.Fatal("expected conflict error, got nil")
			}
			if got != nil {
				t.Errorf("expected no content on error, got %q", string(got))
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestApplyEditOps_EmptyFile(t *testing.T) {
	got, err := applyEditOps(nil, []EditOp{{Op: opInsertHead, Text: "x\ny"}})
	if err != nil {
		t.Fatalf("insert_head on empty file: unexpected error: %v", err)
	}
	if string(got) != "x\ny" {
		t.Errorf("content = %q, want %q", string(got), "x\ny")
	}

	for name, op := range map[string]EditOp{
		"insert_tail":   {Op: opInsertTail, Text: "x"},
		"replace":       {Op: opReplace, Start: "1:a3f", Text: "x"},
		"insert_after":  {Op: opInsertAfter, Start: "1:a3f", Text: "x"},
		"insert_before": {Op: opInsertBefore, Start: "1:a3f", Text: "x"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := applyEditOps(nil, []EditOp{op})
			if err == nil {
				t.Fatal("expected error on empty file, got nil")
			}
			if got != nil {
				t.Errorf("expected no content on error, got %q", string(got))
			}
		})
	}

	_, err = applyEditOps(nil, []EditOp{{Op: opInsertTail, Text: "x"}})
	if err == nil || !strings.Contains(err.Error(), "only insert_head is valid for an empty file") {
		t.Fatalf("insert_tail error %v does not name the empty-file rule", err)
	}
}

func TestApplyEditOps_StaleAnchorsFail(t *testing.T) {
	// Target content changed in place: the old hash no longer matches.
	original := "one\ntwo\nthree\n"
	stale := anchorForLine(t, original, 2)
	got, err := applyEditOps([]byte("one\nTWO\nthree\n"), []EditOp{{Op: opReplace, Start: stale, Text: "x"}})
	if err == nil || !strings.Contains(err.Error(), "does not exist in the current file snapshot") {
		t.Fatalf("changed target: expected missing anchor, got content %q err %v", string(got), err)
	}

	// Target shifted down by an insertion above it: the stale anchor now
	// points at the inserted line, whose hash differs.
	shifted := "one\ninserted\ntwo\nthree\n"
	staleThree := anchorForLine(t, original, 3)
	got, err = applyEditOps([]byte(shifted), []EditOp{{Op: opReplace, Start: staleThree, Text: "x"}})
	if err == nil || !strings.Contains(err.Error(), "does not exist in the current file snapshot") {
		t.Fatalf("shifted target: expected missing anchor, got content %q err %v", string(got), err)
	}

	// A fresh anchor at the shifted line succeeds.
	fresh := anchorForLine(t, shifted, 4)
	got, err = applyEditOps([]byte(shifted), []EditOp{{Op: opReplace, Start: fresh, Text: "THREE"}})
	if err != nil {
		t.Fatalf("fresh anchor: unexpected error: %v", err)
	}
	if string(got) != "one\ninserted\ntwo\nTHREE\n" {
		t.Errorf("content = %q, want %q", string(got), "one\ninserted\ntwo\nTHREE\n")
	}
}

func TestApplyEditOps_UnrelatedChangeFarFromTarget(t *testing.T) {
	original := "one\ntwo\nthree\nfour\n"
	changed := "ONE\ntwo\nthree\nfour\n" // external modification of line 1

	// An anchor far from the external change stays valid.
	ops := []EditOp{{Op: opReplace, Start: anchorForLine(t, original, 4), Text: "FOUR"}}
	got, err := applyEditOps([]byte(changed), ops)
	if err != nil {
		t.Fatalf("unrelated change should not invalidate a distant anchor: %v", err)
	}
	if string(got) != "ONE\ntwo\nthree\nFOUR\n" {
		t.Errorf("content = %q, want %q", string(got), "ONE\ntwo\nthree\nFOUR\n")
	}

	// Multi-op call against the changed snapshot: op 1 matches the new line 1,
	// op 2 matches the untouched line 4.
	ops = []EditOp{
		{Op: opReplace, Start: anchorForLine(t, changed, 1), Text: "first"},
		{Op: opReplace, Start: anchorForLine(t, original, 4), Text: "last"},
	}
	got, err = applyEditOps([]byte(changed), ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "first\ntwo\nthree\nlast\n" {
		t.Errorf("content = %q, want %q", string(got), "first\ntwo\nthree\nlast\n")
	}
}

func TestApplyEditOps_PreservesLineEndingStyle(t *testing.T) {
	crlf := "one\r\ntwo\r\nthree\r\n"
	got, err := applyEditOps([]byte(crlf), []EditOp{{Op: opReplace, Start: anchorForLine(t, crlf, 2), Text: "TWO"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "one\r\nTWO\r\nthree\r\n" {
		t.Errorf("CRLF content = %q, want %q", string(got), "one\r\nTWO\r\nthree\r\n")
	}

	got, err = applyEditOps([]byte(crlf), []EditOp{{Op: opInsertAfter, Start: anchorForLine(t, crlf, 2), Text: "x\ny"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "one\r\ntwo\r\nx\r\ny\r\nthree\r\n" {
		t.Errorf("CRLF insertion = %q, want %q", string(got), "one\r\ntwo\r\nx\r\ny\r\nthree\r\n")
	}
}

func TestApplyEditOps_PreservesTrailingNewlineState(t *testing.T) {
	noTrailing := "one\ntwo\nthree"
	got, err := applyEditOps([]byte(noTrailing), []EditOp{{Op: opInsertAfter, Start: anchorForLine(t, noTrailing, 2), Text: "x"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "one\ntwo\nx\nthree" {
		t.Errorf("content = %q, want %q", string(got), "one\ntwo\nx\nthree")
	}

	got, err = applyEditOps([]byte(noTrailing), []EditOp{{Op: opReplace, Start: anchorForLine(t, noTrailing, 3), Text: ""}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "one\ntwo" {
		t.Errorf("content = %q, want %q", string(got), "one\ntwo")
	}
}

func TestApplyEditOps_EmptyInsertTextIsNoop(t *testing.T) {
	content := "one\ntwo\nthree\n"
	got, err := applyEditOps([]byte(content), []EditOp{{Op: opInsertAfter, Start: anchorForLine(t, content, 2), Text: ""}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != content {
		t.Errorf("content = %q, want %q", string(got), content)
	}
}

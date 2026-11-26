package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/filesystem"
	"github.com/mochow13/keen-code/internal/memory"
)

func newTestMemoryTool(t *testing.T, allow bool) (*MemoryTool, *mockDiffEmitter, *mockPermissionRequester, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	diff := &mockDiffEmitter{}
	perm := &mockPermissionRequester{allow: allow}
	tool := NewMemoryTool(filesystem.NewGuard(workDir, nil), workDir, diff, perm)
	return tool, diff, perm, workDir
}

func readMemory(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read memory: %v", err)
	}
	return string(data)
}

func TestMemoryTool_AddCreatesProjectMemory(t *testing.T) {
	tool, diff, perm, workDir := newTestMemoryTool(t, true)
	entry := "Integration tests:\n```sh\ngo test -tags=integration ./...\n```"
	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "add", "scope": "project", "section": "Commands", "content": entry,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := readMemory(t, memory.ProjectPath(workDir))
	if got != "## Commands\n\n"+entry+"\n" {
		t.Fatalf("unexpected memory content: %q", got)
	}
	if !perm.called {
		t.Fatal("expected permission request")
	}
	if !diff.called {
		t.Fatal("expected diff to be emitted")
	}
	var added []string
	for _, line := range diff.emitted {
		if line.Kind == DiffLineAdded {
			added = append(added, line.Content)
		}
	}
	if !strings.Contains(strings.Join(added, "\n"), entry) {
		t.Fatalf("diff should show the full entry, got %q", added)
	}
	res := result.(map[string]any)
	if res["scope"] != "project" || res["bytes"].(int) != len(got) {
		t.Fatalf("unexpected result: %#v", res)
	}
	if !strings.Contains(res["message"].(string), "of 16 KB") {
		t.Fatalf("expected size in message, got %q", res["message"])
	}
}

func TestMemoryTool_AddGlobal(t *testing.T) {
	tool, _, _, _ := newTestMemoryTool(t, true)
	if _, err := tool.Execute(context.Background(), map[string]any{
		"action": "add", "scope": "global", "section": "Preferences", "content": "- prefers brief replies",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := readMemory(t, memory.GlobalPath()); !strings.Contains(got, "- prefers brief replies") {
		t.Fatalf("unexpected global memory: %q", got)
	}
}

func TestMemoryTool_UpdateAndRemove(t *testing.T) {
	tool, diff, _, workDir := newTestMemoryTool(t, true)
	path := memory.ProjectPath(workDir)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("## Commands\n\n- make build\n- make test\n"), 0o644)

	if _, err := tool.Execute(context.Background(), map[string]any{
		"action": "update", "scope": "project", "old_content": "- make test", "content": "- make test-race",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	var removed, added bool
	for _, line := range diff.emitted {
		removed = removed || (line.Kind == DiffLineRemoved && line.Content == "- make test")
		added = added || (line.Kind == DiffLineAdded && line.Content == "- make test-race")
	}
	if !removed || !added {
		t.Fatalf("update diff should show old and new entry: %#v", diff.emitted)
	}

	if _, err := tool.Execute(context.Background(), map[string]any{
		"action": "remove", "scope": "project", "old_content": "- make build",
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := readMemory(t, path); got != "## Commands\n\n- make test-race\n" {
		t.Fatalf("unexpected memory: %q", got)
	}
}

func TestMemoryTool_RejectsSecret(t *testing.T) {
	tool, diff, perm, workDir := newTestMemoryTool(t, true)
	_, err := tool.Execute(context.Background(), map[string]any{
		"action": "add", "scope": "project", "content": "api_key: sk-1234567890abcdefghijklmnopqrstuvwxyz",
	})
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("expected secret error, got %v", err)
	}
	if diff.called || perm.called {
		t.Fatal("secret should be rejected before diff or permission")
	}
	if _, err := os.Stat(memory.ProjectPath(workDir)); !os.IsNotExist(err) {
		t.Fatal("memory file should not be created")
	}
}

func TestMemoryTool_RejectsOverCap(t *testing.T) {
	tool, _, perm, workDir := newTestMemoryTool(t, true)
	path := memory.ProjectPath(workDir)
	os.MkdirAll(filepath.Dir(path), 0o755)
	existing := "- " + strings.Repeat("x", memory.ProjectMaxBytes-10) + "\n"
	os.WriteFile(path, []byte(existing), 0o644)

	_, err := tool.Execute(context.Background(), map[string]any{
		"action": "add", "scope": "project", "content": "- one more entry that does not fit",
	})
	if err == nil || !strings.Contains(err.Error(), "of 16 KB") || !strings.Contains(err.Error(), "merge, trim, or remove") {
		t.Fatalf("expected actionable cap error, got %v", err)
	}
	if perm.called {
		t.Fatal("over-cap write should not ask for permission")
	}
	if readMemory(t, path) != existing {
		t.Fatal("memory should be unchanged")
	}
}

func TestMemoryTool_NearCapNudge(t *testing.T) {
	tool, _, _, workDir := newTestMemoryTool(t, true)
	path := memory.ProjectPath(workDir)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("- "+strings.Repeat("x", memory.ProjectMaxBytes*85/100)+"\n"), 0o644)

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "add", "scope": "project", "content": "- small entry",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg := result.(map[string]any)["message"].(string); !strings.Contains(msg, "near its cap") {
		t.Fatalf("expected near-cap nudge, got %q", msg)
	}
}

func TestMemoryTool_PermissionDeniedLeavesFileUnchanged(t *testing.T) {
	tool, _, perm, workDir := newTestMemoryTool(t, false)
	_, err := tool.Execute(context.Background(), map[string]any{
		"action": "add", "scope": "project", "content": "- something",
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected permission error, got %v", err)
	}
	if !perm.called {
		t.Fatal("expected permission request")
	}
	if _, err := os.Stat(memory.ProjectPath(workDir)); !os.IsNotExist(err) {
		t.Fatal("memory file should not be created")
	}
}

func TestMemoryTool_ValidateInput(t *testing.T) {
	tool, _, _, _ := newTestMemoryTool(t, true)
	cases := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"missing action", map[string]any{"scope": "project"}, `"action"`},
		{"bad action", map[string]any{"action": "append", "scope": "project"}, "action must be"},
		{"missing scope", map[string]any{"action": "add", "content": "x"}, `"scope"`},
		{"bad scope", map[string]any{"action": "add", "scope": "team", "content": "x"}, "scope must be"},
		{"add without content", map[string]any{"action": "add", "scope": "project"}, `"content"`},
		{"update without old", map[string]any{"action": "update", "scope": "project", "content": "x"}, `"old_content"`},
		{"update without content", map[string]any{"action": "update", "scope": "project", "old_content": "x"}, "remove"},
		{"remove without old", map[string]any{"action": "remove", "scope": "project"}, `"old_content"`},
		{"whitespace-only update content", map[string]any{"action": "update", "scope": "project", "old_content": "x", "content": " \n"}, "remove"},
		{"whitespace-only add content", map[string]any{"action": "add", "scope": "project", "content": "  "}, `"content"`},
		{"whitespace-only old_content", map[string]any{"action": "remove", "scope": "project", "old_content": "\n"}, `"old_content"`},
		{"non-string content", map[string]any{"action": "add", "scope": "project", "content": 3}, "must be a string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tool.ValidateInput(context.Background(), tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
	if err := tool.ValidateInput(context.Background(), map[string]any{"action": "remove", "scope": "global", "old_content": "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMemoryTool_UpdateNotFound(t *testing.T) {
	tool, _, perm, _ := newTestMemoryTool(t, true)
	_, err := tool.Execute(context.Background(), map[string]any{
		"action": "update", "scope": "project", "old_content": "- missing", "content": "- new",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
	if perm.called {
		t.Fatal("failed update should not ask for permission")
	}
}

func TestMemoryTool_OverCapFileCanShrink(t *testing.T) {
	tool, _, _, workDir := newTestMemoryTool(t, true)
	path := memory.ProjectPath(workDir)
	os.MkdirAll(filepath.Dir(path), 0o755)
	big := "- " + strings.Repeat("x", memory.ProjectMaxBytes) + "\n"
	os.WriteFile(path, []byte(big+"- old entry\n- keep this one\n"), 0o644)

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "remove", "scope": "project", "old_content": "- old entry",
	})
	if err != nil {
		t.Fatalf("shrinking remove should be allowed above cap: %v", err)
	}
	if msg := result.(map[string]any)["message"].(string); !strings.Contains(msg, "still over its cap") {
		t.Fatalf("expected still-over-cap message, got %q", msg)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{
		"action": "update", "scope": "project", "old_content": "- keep this one", "content": "- kept",
	}); err != nil {
		t.Fatalf("shrinking update should be allowed above cap: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{
		"action": "update", "scope": "project", "old_content": "- kept", "content": "- kept but longer",
	}); err == nil {
		t.Fatal("growing update above cap should be rejected")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{
		"action": "add", "scope": "project", "content": "- new",
	}); err == nil {
		t.Fatal("add above cap should be rejected")
	}
}

func TestMemoryTool_RejectsSecretAssembledFromExistingContent(t *testing.T) {
	tool, _, perm, workDir := newTestMemoryTool(t, true)
	path := memory.ProjectPath(workDir)
	os.MkdirAll(filepath.Dir(path), 0o755)
	original := "- token\n- abcd1234efgh\n"
	os.WriteFile(path, []byte(original), 0o644)

	_, err := tool.Execute(context.Background(), map[string]any{
		"action": "update", "scope": "project", "old_content": "- token\n- ", "content": "- token: ",
	})
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("expected secret error, got %v", err)
	}
	if perm.called || readMemory(t, path) != original {
		t.Fatal("memory should be unchanged and no permission requested")
	}
}

func TestMemoryTool_RemoveAllowedWhenFileAlreadyHasSecret(t *testing.T) {
	tool, _, _, workDir := newTestMemoryTool(t, true)
	path := memory.ProjectPath(workDir)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("- api_key: sk-1234567890abcdefghijklmnopqrstuvwxyz\n- stale entry\n"), 0o644)

	if _, err := tool.Execute(context.Background(), map[string]any{
		"action": "remove", "scope": "project", "old_content": "- stale entry",
	}); err != nil {
		t.Fatalf("remove should work on a file that already contains a secret: %v", err)
	}
}

package history_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/cli/repl/history"
)

func TestInputHistory_Push_BlankIgnored(t *testing.T) {
	var h history.InputHistory
	h.Push("")
	h.Push("   ")
	// no panic and NavigateUp returns false
	_, ok := h.NavigateUp("")
	if ok {
		t.Fatal("expected no entries for blank inputs")
	}
}

func TestInputHistory_Push_SlashCommandStored(t *testing.T) {
	var h history.InputHistory
	h.Push("/clear")
	h.Push("/help")

	val, ok := h.NavigateUp("")
	if !ok {
		t.Fatal("expected slash commands to be stored")
	}
	if val != "/help" {
		t.Fatalf("expected '/help', got %q", val)
	}
	val, ok = h.NavigateUp(val)
	if !ok {
		t.Fatal("expected second slash command to be stored")
	}
	if val != "/clear" {
		t.Fatalf("expected '/clear', got %q", val)
	}
}

func TestInputHistory_Push_DuplicateLastIgnored(t *testing.T) {
	var h history.InputHistory
	h.Push("hello")
	h.Push("hello")
	// Only one entry: navigating up twice from start should fail on second
	h.NavigateUp("")
	_, ok := h.NavigateUp("")
	if ok {
		t.Fatal("expected only 1 entry after duplicate push")
	}
}

func TestInputHistory_Push_MaxSizeTrimmed(t *testing.T) {
	var h history.InputHistory
	for i := range history.MaxHistorySize + 10 {
		h.Push(strings.Repeat("x", i+1))
	}
	// Navigate up MaxHistorySize times should succeed, one more should fail
	for i := 0; i < history.MaxHistorySize; i++ {
		_, ok := h.NavigateUp("")
		if !ok {
			t.Fatalf("expected ok=true at step %d", i)
		}
	}
	_, ok := h.NavigateUp("")
	if ok {
		t.Fatal("expected ok=false after exhausting max history")
	}
}

func TestInputHistory_Push_ResetsNavigation(t *testing.T) {
	var h history.InputHistory
	h.Push("first")
	h.Push("second")
	h.NavigateUp("draft")
	h.Push("third")
	// After push, navigateDown should return false (already in draft mode)
	_, ok := h.NavigateDown()
	if ok {
		t.Fatal("expected idx to be reset to -1 after push")
	}
}

func TestInputHistory_NavigateUp_SavesDraft(t *testing.T) {
	var h history.InputHistory
	h.Push("first")
	h.Push("second")

	val, ok := h.NavigateUp("my draft")
	if !ok {
		t.Fatal("expected ok=true on first NavigateUp")
	}
	if val != "second" {
		t.Fatalf("expected newest entry 'second', got %q", val)
	}
}

func TestInputHistory_NavigateUp_StopsAtOldest(t *testing.T) {
	var h history.InputHistory
	h.Push("only")

	h.NavigateUp("")
	_, ok := h.NavigateUp("")
	if ok {
		t.Fatal("expected ok=false when already at oldest entry")
	}
}

func TestInputHistory_NavigateUp_EmptyHistory(t *testing.T) {
	var h history.InputHistory
	_, ok := h.NavigateUp("draft")
	if ok {
		t.Fatal("expected ok=false for empty history")
	}
}

func TestInputHistory_NavigateDown_RestoresDraft(t *testing.T) {
	var h history.InputHistory
	h.Push("first")

	h.NavigateUp("my draft")
	val, ok := h.NavigateDown()
	if !ok {
		t.Fatal("expected ok=true when navigating back to draft")
	}
	if val != "my draft" {
		t.Fatalf("expected draft restored, got %q", val)
	}
	// Now in draft mode — another NavigateDown should return false
	_, ok = h.NavigateDown()
	if ok {
		t.Fatal("expected ok=false after restoring draft")
	}
}

func TestInputHistory_NavigateDown_AlreadyInDraftMode(t *testing.T) {
	var h history.InputHistory
	h.Push("first")

	_, ok := h.NavigateDown()
	if ok {
		t.Fatal("expected ok=false when already in draft mode")
	}
}

func TestInputHistory_NavigateDown_StepsForward(t *testing.T) {
	var h history.InputHistory
	h.Push("first")
	h.Push("second")
	h.Push("third")

	h.NavigateUp("")
	h.NavigateUp("")
	// now at "second"
	val, ok := h.NavigateDown()
	if !ok {
		t.Fatal("expected ok=true when stepping forward")
	}
	if val != "third" {
		t.Fatalf("expected 'third', got %q", val)
	}
}

func TestInputHistory_Reset(t *testing.T) {
	var h history.InputHistory
	h.Push("first")
	h.NavigateUp("draft")
	h.Reset()
	// After reset, NavigateDown should return false (draft mode)
	_, ok := h.NavigateDown()
	if ok {
		t.Fatal("expected idx=-1 after reset")
	}
}

func TestInputHistory_LoadFromFile_MissingFileIsNoop(t *testing.T) {
	var h history.InputHistory
	err := h.LoadFromFile(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	_, ok := h.NavigateUp("")
	if ok {
		t.Fatal("expected empty entries")
	}
}

func TestInputHistory_LoadFromFile_DeduplicatesConsecutive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input-history")
	content := "hello\nhello\nworld\nworld\nhello\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	var h history.InputHistory
	if err := h.LoadFromFile(path); err != nil {
		t.Fatal(err)
	}

	// hello, world, hello (consecutive dups removed) — navigate up 3 times
	for i := 0; i < 3; i++ {
		_, ok := h.NavigateUp("")
		if !ok {
			t.Fatalf("expected ok=true at step %d", i)
		}
	}
	_, ok := h.NavigateUp("")
	if ok {
		t.Fatal("expected exactly 3 entries after dedup")
	}
}

func TestInputHistory_LoadFromFile_UnescapesNewlines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input-history")
	if err := os.WriteFile(path, []byte(`line1\nline2`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var h history.InputHistory
	if err := h.LoadFromFile(path); err != nil {
		t.Fatal(err)
	}

	val, ok := h.NavigateUp("")
	if !ok {
		t.Fatal("expected 1 entry")
	}
	if val != "line1\nline2" {
		t.Fatalf("expected unescaped newline, got %q", val)
	}
}

func TestInputHistory_AppendToFile_EscapesNewlines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input-history")

	var h history.InputHistory
	if err := h.LoadFromFile(path); err != nil {
		t.Fatal(err)
	}
	if err := h.AppendToFile("line1\nline2"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `line1\nline2`) {
		t.Fatalf("expected escaped newline in file, got %q", string(data))
	}
}

func TestInputHistory_AppendToFile_NoFilePath(t *testing.T) {
	var h history.InputHistory
	err := h.AppendToFile("hello")
	if err != nil {
		t.Fatalf("expected no error for empty filePath, got %v", err)
	}
}

func TestInputHistory_Flush_RewritesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input-history")

	var h history.InputHistory
	_ = h.LoadFromFile(path)
	h.Push("first")
	h.Push("second")
	h.Push("third")

	if err := h.Flush(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines in flushed file, got %d: %v", len(lines), lines)
	}
	if lines[0] != "first" || lines[1] != "second" || lines[2] != "third" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestInputHistory_Flush_NoFilePath(t *testing.T) {
	var h history.InputHistory
	h.Push("hello")
	if err := h.Flush(); err != nil {
		t.Fatalf("expected no error for empty filePath, got %v", err)
	}
}

func TestInputHistory_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input-history")

	var h history.InputHistory
	_ = h.LoadFromFile(path)
	h.Push("first")
	h.Push("second")
	h.Push("third")

	v1, ok := h.NavigateUp("draft")
	if !ok || v1 != "third" {
		t.Fatalf("expected 'third', got %q ok=%v", v1, ok)
	}
	v2, ok := h.NavigateUp(v1)
	if !ok || v2 != "second" {
		t.Fatalf("expected 'second', got %q ok=%v", v2, ok)
	}
	v3, ok := h.NavigateUp(v2)
	if !ok || v3 != "first" {
		t.Fatalf("expected 'first', got %q ok=%v", v3, ok)
	}

	h.NavigateDown()
	h.NavigateDown()
	restored, ok := h.NavigateDown()
	if !ok {
		t.Fatal("expected ok=true when restoring draft")
	}
	if restored != "draft" {
		t.Fatalf("expected draft 'draft', got %q", restored)
	}
	_, ok = h.NavigateDown()
	if ok {
		t.Fatal("expected ok=false after full round-trip")
	}
}

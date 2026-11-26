package memory

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAddToEmptyWithSection(t *testing.T) {
	got, err := Add("", "Commands", "- run go test ./...")
	if err != nil {
		t.Fatal(err)
	}
	want := "## Commands\n\n- run go test ./...\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddWithoutSectionAppends(t *testing.T) {
	got, err := Add("- one\n", "", "- two")
	if err != nil {
		t.Fatal(err)
	}
	if got != "- one\n\n- two\n" {
		t.Fatalf("got %q", got)
	}
}

func TestAddToExistingSection(t *testing.T) {
	content := "## Preferences\n\n- brief replies\n\n## Commands\n\n- make build\n"
	got, err := Add(content, "preferences", "- no emojis")
	if err != nil {
		t.Fatal(err)
	}
	want := "## Preferences\n\n- brief replies\n- no emojis\n\n## Commands\n\n- make build\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddToLastSection(t *testing.T) {
	content := "## Preferences\n\n- brief replies\n\n## Commands\n\n- make build\n"
	got, err := Add(content, "## Commands", "- make test")
	if err != nil {
		t.Fatal(err)
	}
	want := "## Preferences\n\n- brief replies\n\n## Commands\n\n- make build\n- make test\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddToEmptySection(t *testing.T) {
	content := "## Gotchas\n\n## Commands\n\n- make build\n"
	got, err := Add(content, "Gotchas", "- CI needs Go 1.24")
	if err != nil {
		t.Fatal(err)
	}
	want := "## Gotchas\n\n- CI needs Go 1.24\n\n## Commands\n\n- make build\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddCreatesMissingSection(t *testing.T) {
	got, err := Add("## Preferences\n\n- brief replies\n", "Commands", "- make build")
	if err != nil {
		t.Fatal(err)
	}
	want := "## Preferences\n\n- brief replies\n\n## Commands\n\n- make build\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddIgnoresHeadingsInsideCodeBlocks(t *testing.T) {
	content := "## Commands\n\n```sh\n## not a heading\nmake build\n```\n\n## Gotchas\n\n- slow CI\n"
	got, err := Add(content, "Commands", "- make test")
	if err != nil {
		t.Fatal(err)
	}
	want := "## Commands\n\n```sh\n## not a heading\nmake build\n```\n- make test\n\n## Gotchas\n\n- slow CI\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAddMultilineEntryWithCodeBlock(t *testing.T) {
	entry := "Integration tests:\n```sh\ngo test -tags=integration ./...\n```"
	got, err := Add("", "Commands", entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, entry) {
		t.Fatalf("entry not preserved: %q", got)
	}
}

func TestAddRejectsDuplicateAndEmpty(t *testing.T) {
	if _, err := Add("## Commands\n\n- make build\n", "Commands", "- make build"); err == nil {
		t.Fatal("expected duplicate error")
	}
	if _, err := Add("", "Commands", "   "); err == nil {
		t.Fatal("expected empty entry error")
	}
}

func TestAddAllowsEntryThatIsSubstringOfAnother(t *testing.T) {
	if _, err := Add("- make build all\n", "", "- make build"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReplaceUpdatesEntry(t *testing.T) {
	got, err := Replace("- a\n- b\n- c\n", "- b", "- B")
	if err != nil {
		t.Fatal(err)
	}
	if got != "- a\n- B\n- c\n" {
		t.Fatalf("got %q", got)
	}
}

func TestReplaceMergesEntries(t *testing.T) {
	got, err := Replace("- use pnpm\n- run pnpm test\n- other\n", "- use pnpm\n- run pnpm test", "- use pnpm; test with pnpm test")
	if err != nil {
		t.Fatal(err)
	}
	if got != "- use pnpm; test with pnpm test\n- other\n" {
		t.Fatalf("got %q", got)
	}
}

func TestReplaceRemovesEntryAndBlankLines(t *testing.T) {
	cases := []struct {
		name, content, old, want string
	}{
		{"middle bullet", "- a\n- b\n- c\n", "- b", "- a\n- c\n"},
		{"paragraph", "A\n\nB\n\nC\n", "B", "A\n\nC\n"},
		{"first", "- a\n- b\n", "- a", "- b\n"},
		{"last", "- a\n- b\n", "- b", "- a\n"},
		{"only", "- a\n", "- a", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Replace(tc.content, tc.old, "")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReplaceRequiresExactlyOneMatch(t *testing.T) {
	if _, err := Replace("- a\n", "- missing", "x"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
	if _, err := Replace("- a\n- a\n", "- a", "x"); err == nil || !strings.Contains(err.Error(), "matches 2 places") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	if _, err := Replace("- a\n", "  ", "x"); err == nil {
		t.Fatal("expected empty old_content error")
	}
}

func TestCapForAndFormatSize(t *testing.T) {
	if CapFor(ScopeGlobal) != GlobalMaxBytes || CapFor(ScopeProject) != ProjectMaxBytes {
		t.Fatal("unexpected caps")
	}
	for n, want := range map[int]string{512: "512 B", 8 * 1024: "8 KB", 9421: "9.2 KB"} {
		if got := FormatSize(n); got != want {
			t.Fatalf("FormatSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestCapCutsOnLineBoundary(t *testing.T) {
	content := "- first line\n- second line\n- third line"
	got := cap(content, 20)
	if !strings.HasPrefix(got, "- first line\n[truncated") {
		t.Fatalf("got %q", got)
	}
}

func TestCapNeverSplitsRune(t *testing.T) {
	content := strings.Repeat("é", 10)
	got := cap(content, 5)
	prefix := strings.SplitN(got, "\n", 2)[0]
	if !utf8.ValidString(prefix) || prefix != "éé" {
		t.Fatalf("got %q", prefix)
	}
}

package filesearch

import (
	"os/exec"
	"testing"
	"time"
)

func TestSearchTrackedFilesAndDirectories(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	writeFile(t, dir, "internal/pkg/file.go", "package pkg")
	writeFile(t, dir, "README.md", "readme")
	runGit(t, dir, "init")
	runGit(t, dir, "add", ".")

	searcher := NewFileSearcher(dir, nil)
	got := searcher.Search("PKG", 10)
	if !containsPath(got, "internal/pkg") || !containsPath(got, "internal/pkg/file.go") {
		t.Fatalf("Search() = %q, want directory and file", got)
	}
	if got := searcher.Search("", 10); got != nil {
		t.Fatalf("empty Search() = %q, want nil", got)
	}
	if got := searcher.Search(".", 1); len(got) != 1 {
		t.Fatalf("limited Search() = %q, want one result", got)
	}
}

func TestRefreshAddsUntrackedFiles(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	writeFile(t, dir, "tracked.txt", "tracked")
	runGit(t, dir, "init")
	runGit(t, dir, "add", "tracked.txt")
	searcher := NewFileSearcher(dir, nil)
	_ = searcher.Search("tracked", 10)
	writeFile(t, dir, "new-file.txt", "new")
	searcher.refreshInterval = 0
	searcher.updatedAt = time.Time{}
	_ = searcher.Search("new-file", 10)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if containsPath(searcher.Search("new-file", 10), "new-file.txt") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("background refresh did not add untracked file")
}

func TestGitLsFilesFailure(t *testing.T) {
	if paths, ok := gitLsFiles(t.TempDir()); ok || paths != nil {
		t.Fatalf("gitLsFiles() = %q, %v, want nil false", paths, ok)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

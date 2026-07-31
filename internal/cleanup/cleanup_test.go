package cleanup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/user/keen-code/internal/session"
)

func TestPruneExpiredFilesBefore_RemovesOnlyExpiredRegularFiles(t *testing.T) {
	dir := t.TempDir()
	expired := filepath.Join(dir, "expired.txt")
	recent := filepath.Join(dir, "recent.txt")
	subdir := filepath.Join(dir, "subdir")
	link := filepath.Join(dir, "link.txt")

	for _, path := range []string{expired, recent} {
		if err := os.WriteFile(path, []byte(path), 0600); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
	}
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	if err := os.Symlink(recent, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	oldTime := cutoff.Add(-time.Hour)
	if err := os.Chtimes(expired, oldTime, oldTime); err != nil {
		t.Fatalf("age expired file: %v", err)
	}

	removed, err := pruneExpiredFilesBefore(dir, cutoff)
	if err != nil {
		t.Fatalf("prune expired files: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired file exists, err = %v", err)
	}
	for _, path := range []string{recent, subdir, link} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("expected %q to remain: %v", path, err)
		}
	}
}

func TestPruneExpiredFilesBefore_RejectsSymlinkRoot(t *testing.T) {
	target := t.TempDir()
	root := filepath.Join(t.TempDir(), "cleanup-root")
	if err := os.Symlink(target, root); err != nil {
		t.Fatalf("create symlink root: %v", err)
	}
	if _, err := pruneExpiredFilesBefore(root, time.Now()); err == nil {
		t.Fatal("expected symlink root error")
	}
}

func TestTrimHistory_KeepsNewestEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input-history")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := TrimHistory(path, 2); err != nil {
		t.Fatalf("TrimHistory() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "two\nthree\n"; got != want {
		t.Fatalf("history = %q, want %q", got, want)
	}
}

func TestRun_CleansManagedData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, name := range []string{"bash", "mcp-artifacts", "web-fetch-artifacts", "logs"} {
		dir := filepath.Join(home, ".keen", name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, "expired.txt")
		if err := os.WriteFile(file, []byte("expired"), 0600); err != nil {
			t.Fatal(err)
		}
		age := artifactRetention + time.Hour
		if name == "logs" {
			age = logRetention + time.Hour
		}
		oldTime := time.Now().Add(-age)
		if err := os.Chtimes(file, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}

	store, err := session.NewStore(filepath.Join(home, "project"))
	if err != nil {
		t.Fatal(err)
	}
	expiredSession, err := store.Create()
	if err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-sessionRetention - time.Hour)
	if err := os.Chtimes(expiredSession.TranscriptPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	recentSession, err := store.Create()
	if err != nil {
		t.Fatal(err)
	}

	historyPath := filepath.Join(home, ".keen", "input-history")
	entries := make([]string, historyLimit+1)
	for i := range entries {
		entries[i] = "entry"
	}
	if err := os.WriteFile(historyPath, []byte(strings.Join(entries, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, name := range []string{"bash", "mcp-artifacts", "web-fetch-artifacts", "logs"} {
		if _, err := os.Stat(filepath.Join(home, ".keen", name, "expired.txt")); !os.IsNotExist(err) {
			t.Errorf("expired %s file exists, err = %v", name, err)
		}
	}
	if _, err := os.Stat(expiredSession.Directory); !os.IsNotExist(err) {
		t.Errorf("expired session exists, err = %v", err)
	}
	if _, err := os.Stat(recentSession.Directory); err != nil {
		t.Errorf("recent session missing, err = %v", err)
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")); got != historyLimit {
		t.Errorf("history entries = %d, want %d", got, historyLimit)
	}
}

package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteFileAtomic_ReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("after")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after" {
		t.Fatalf("expected content %q, got %q", "after", string(got))
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("expected regular file, got mode %v", fi.Mode())
	}
}

func TestWriteFileAtomic_CreatesNewFileWithDefaultMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("expected content %q, got %q", "new", string(got))
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0644 {
		t.Fatalf("expected mode 0644 for new file, got %v", fi.Mode().Perm())
	}
}

func TestWriteFileAtomic_PreservesExistingMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("after")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("expected mode 0600 preserved, got %v", fi.Mode().Perm())
	}
}

func TestWriteFileAtomic_UpdatesSymlinkReferent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	referent := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(referent, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(referent, link); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(link, []byte("after")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected link to remain a symlink")
	}
	got, err := os.ReadFile(referent)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after" {
		t.Fatalf("expected referent content %q, got %q", "after", string(got))
	}
}

func TestWriteFileAtomic_RemovesTempOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	// A directory as the target makes the final rename fail.
	target := filepath.Join(dir, "targetdir")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(target, []byte("data")); err == nil {
		t.Fatal("expected error when renaming over a directory")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "."+filepath.Base(target)+".tmp-") {
			t.Fatalf("temporary file %q left behind after failure", e.Name())
		}
	}
}

func TestResolveWriteTarget_NonExistentPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.txt")

	got, err := resolveWriteTarget(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != path {
		t.Fatalf("expected %q, got %q", path, got)
	}
}

func TestResolveWriteTarget_RegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveWriteTarget(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != path {
		t.Fatalf("expected %q, got %q", path, got)
	}
}

func TestResolveWriteTarget_SymlinkChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	dir := t.TempDir()
	referent := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(referent, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	link1 := filepath.Join(dir, "link1.txt")
	link2 := filepath.Join(dir, "link2.txt")
	if err := os.Symlink(referent, link1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(link1, link2); err != nil {
		t.Fatal(err)
	}

	got, err := resolveWriteTarget(link2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != referent {
		t.Fatalf("expected referent %q, got %q", referent, got)
	}
}

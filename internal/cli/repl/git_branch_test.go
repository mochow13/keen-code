package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func initGitRepo(t *testing.T, dir string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	return repo
}

func TestReadGitBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	if got := readGitBranch(dir); got != "master" {
		t.Fatalf("expected master, got %q", got)
	}

	if got := readGitBranch(t.TempDir()); got != "" {
		t.Fatalf("expected empty outside a repo, got %q", got)
	}
}

func TestReadGitBranch_DetachedHead(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	headFile := filepath.Join(dir, ".git", "HEAD")
	if err := os.WriteFile(headFile, []byte("1234567890abcdef1234567890abcdef12345678\n"), 0644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if got := readGitBranch(dir); got != "1234567" {
		t.Fatalf("expected short hash for detached HEAD, got %q", got)
	}
}

func TestReadGitBranch_NestedDir(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	nested := filepath.Join(dir, "sub", "dir")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if got := readGitBranch(nested); got != "master" {
		t.Fatalf("expected branch from parent repo, got %q", got)
	}
}

func TestReadGitBranch_WorktreePointer(t *testing.T) {
	main := t.TempDir()
	initGitRepo(t, main)

	worktree := t.TempDir()
	pointer := "gitdir: " + filepath.Join(main, ".git") + "\n"
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte(pointer), 0644); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}
	if got := readGitBranch(worktree); got != "master" {
		t.Fatalf("expected branch through worktree pointer, got %q", got)
	}
}

func TestRefreshGitBranch(t *testing.T) {
	dir := t.TempDir()
	repo := initGitRepo(t, dir)

	m := newTestModel()
	m.ctx.workingDir = dir
	m.refreshGitBranch()
	if m.gitBranch != "master" {
		t.Fatalf("expected branch master, got %q", m.gitBranch)
	}

	headFile := filepath.Join(dir, ".git", "HEAD")
	ref := plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/other")
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatalf("set reference: %v", err)
	}
	if err := os.WriteFile(headFile, []byte("ref: refs/heads/other\n"), 0644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	m.refreshGitBranch()
	if m.gitBranch != "other" {
		t.Fatalf("expected refreshed branch other, got %q", m.gitBranch)
	}
}

func TestInputMetaView_TwoLinesWithBranch(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.ctx.workingDir = t.TempDir()
	m.gitBranch = "main"

	lines := strings.Split(m.inputMetaView(), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 meta lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "main") {
		t.Fatalf("expected branch on first line, got %q", lines[0])
	}
	if strings.Contains(lines[1], "main") {
		t.Fatalf("expected no branch on second line, got %q", lines[1])
	}
}

func TestInputMetaLocationLine_ShowsProviderPrefix(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.ctx.workingDir = t.TempDir()
	m.ctx.cfg.Provider = "anthropic"
	m.ctx.cfg.Model = "claude-sonnet-4-5"

	lines := strings.Split(m.inputMetaView(), "\n")
	if !strings.Contains(lines[0], "anthropic/claude-sonnet-4-5") {
		t.Fatalf("expected provider/model on location line, got %q", lines[0])
	}
	if strings.Contains(lines[1], "anthropic/claude-sonnet-4-5") {
		t.Fatalf("expected no provider/model on status line, got %q", lines[1])
	}
}

func TestInputMetaView_NoBranchOutsideGitRepo(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.ctx.workingDir = t.TempDir()

	lines := strings.Split(m.inputMetaView(), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 meta lines, got %d", len(lines))
	}
}

func TestInputMetaView_TruncatesLongLocationLine(t *testing.T) {
	m := newTestModel()
	m.width = 30
	m.ctx.workingDir = filepath.Join(t.TempDir(), "a-very-long-directory-name")
	m.gitBranch = "a-very-long-branch-name"

	line := strings.Split(m.inputMetaView(), "\n")[0]
	if !strings.Contains(line, "…") {
		t.Fatalf("expected truncated location line, got %q", line)
	}
}

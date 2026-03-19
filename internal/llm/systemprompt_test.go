package llm

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuild_ContainsIdentity(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir)
	if !strings.Contains(result, "Keen Code") {
		t.Error("expected output to contain 'Keen Code'")
	}
}

func TestBuild_ContainsWorkingDir(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir)
	if !strings.Contains(result, dir) {
		t.Errorf("expected output to contain working dir %q", dir)
	}
}

func TestBuild_ContainsPlatform(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir)
	if !strings.Contains(result, runtime.GOOS) {
		t.Errorf("expected output to contain platform %q", runtime.GOOS)
	}
}

func TestBuild_ContainsDate(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir)
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(result, today) {
		t.Errorf("expected output to contain today's date %q", today)
	}
}

func TestBuild_GitRepo(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if err := cmd.Run(); err != nil {
		t.Skip("git not available")
	}

	result := envBlock(dir)
	if !strings.Contains(result, "Is git repo: yes") {
		t.Error("expected 'Is git repo: yes' for git-initialized directory")
	}
}

func TestBuild_NoGitRepo(t *testing.T) {
	dir := t.TempDir()
	result := envBlock(dir)
	if !strings.Contains(result, "Is git repo: no") {
		t.Error("expected 'Is git repo: no' for directory without .git")
	}
}

func TestBuild_DirListing(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "cmd"), 0755)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	result := Build(dir)
	if !strings.Contains(result, "cmd/") {
		t.Error("expected directory listing to contain 'cmd/'")
	}
	if !strings.Contains(result, "go.mod") {
		t.Error("expected directory listing to contain 'go.mod'")
	}
}

func TestBuild_DirListing_Empty(t *testing.T) {
	dir := t.TempDir()
	result := dirListing(dir)
	if result != "" {
		t.Errorf("expected empty listing for empty dir, got %q", result)
	}
}

func TestBuild_DirListing_Unreadable(t *testing.T) {
	result := dirListing("/nonexistent/path/that/does/not/exist")
	if result != "" {
		t.Errorf("expected empty listing for unreadable dir, got %q", result)
	}
}

func TestBuild_AgentsMd_Found(t *testing.T) {
	dir := t.TempDir()
	content := "## My Project\nSome instructions here."
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644)

	result := Build(dir)
	if !strings.Contains(result, "# Project Instructions") {
		t.Error("expected project instructions section")
	}
	if !strings.Contains(result, "My Project") {
		t.Error("expected AGENTS.md content in output")
	}
}

func TestBuild_AgentsMd_WalkUp(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "subdir")
	os.MkdirAll(child, 0755)
	os.WriteFile(filepath.Join(parent, "AGENTS.md"), []byte("parent instructions"), 0644)

	result := Build(child)
	if !strings.Contains(result, "parent instructions") {
		t.Error("expected AGENTS.md from parent directory")
	}
}

func TestBuild_ClaudeMd_Fallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("claude instructions"), 0644)

	result := Build(dir)
	if !strings.Contains(result, "claude instructions") {
		t.Error("expected CLAUDE.md content as fallback")
	}
}

func TestBuild_NoInstructionFile(t *testing.T) {
	dir := t.TempDir()
	result := Build(dir)
	if strings.Contains(result, "# Project Instructions") {
		t.Error("expected no project instructions section when no file exists")
	}
}

func TestBuild_AgentsMd_Truncation(t *testing.T) {
	dir := t.TempDir()
	content := strings.Repeat("x", 10*1024)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644)

	result := Build(dir)
	if !strings.Contains(result, "[truncated") {
		t.Error("expected truncation note for large AGENTS.md")
	}
}

func TestBuild_AgentsMd_Empty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(""), 0644)

	result := Build(dir)
	if strings.Contains(result, "# Project Instructions") {
		t.Error("expected no project instructions for empty AGENTS.md")
	}
}

func TestBuild_FreshOnEachCall(t *testing.T) {
	dir := t.TempDir()
	result1 := Build(dir)
	result2 := Build(dir)
	if result1 != result2 {
		t.Error("expected identical output from two Build calls with same args")
	}
}

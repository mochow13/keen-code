package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestProjectPath(t *testing.T) {
	got := ProjectPath("/work/repo")
	want := filepath.Join("/work/repo", ".keen", "memory", "MEMORY.md")
	if got != want {
		t.Fatalf("ProjectPath = %q, want %q", got, want)
	}
	if ProjectPath("") != "" {
		t.Fatalf("ProjectPath(\"\") should be empty")
	}
}

func TestGlobalPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".keen", "memory", "global", "MEMORY.md")
	if got := GlobalPath(); got != want {
		t.Fatalf("GlobalPath = %q, want %q", got, want)
	}
}

func TestFilesAtEmpty(t *testing.T) {
	global, project := FilesAt("", "")
	if global.Exists || project.Exists {
		t.Fatalf("expected both non-existent")
	}
	if global.Content != "" || project.Content != "" {
		t.Fatalf("expected empty content")
	}
}

func TestFilesAtMissing(t *testing.T) {
	global, project := FilesAt("/nonexistent/global/MEMORY.md", "/nonexistent/project/MEMORY.md")
	if global.Exists || project.Exists {
		t.Fatalf("missing files should not exist")
	}
}

func TestFilesAtGlobalOnly(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global", "MEMORY.md")
	writeTestFile(t, globalPath, "- prefers brief responses")
	global, project := FilesAt(globalPath, filepath.Join(dir, "project", "MEMORY.md"))
	if !global.Exists {
		t.Fatalf("global should exist")
	}
	if project.Exists {
		t.Fatalf("project should not exist")
	}
	if global.Content != "- prefers brief responses" {
		t.Fatalf("global content = %q", global.Content)
	}
}

func TestFilesAtProjectOnly(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project", "MEMORY.md")
	writeTestFile(t, projectPath, "- run go test -race")
	global, project := FilesAt(filepath.Join(dir, "global", "MEMORY.md"), projectPath)
	if global.Exists {
		t.Fatalf("global should not exist")
	}
	if !project.Exists {
		t.Fatalf("project should exist")
	}
	if project.Content != "- run go test -race" {
		t.Fatalf("project content = %q", project.Content)
	}
}

func TestFilesAtBoth(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global", "MEMORY.md")
	projectPath := filepath.Join(dir, "project", "MEMORY.md")
	writeTestFile(t, globalPath, "g")
	writeTestFile(t, projectPath, "p")
	global, project := FilesAt(globalPath, projectPath)
	if !global.Exists || !project.Exists {
		t.Fatalf("both should exist")
	}
}

func TestLoadAtEmpty(t *testing.T) {
	if got := LoadAt("", ""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestLoadAtMissing(t *testing.T) {
	if got := LoadAt("/nope/g", "/nope/p"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestLoadAtGlobalOnly(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global", "MEMORY.md")
	writeTestFile(t, globalPath, "- brief")
	got := LoadAt(globalPath, filepath.Join(dir, "project", "MEMORY.md"))
	if !strings.Contains(got, "# Global memory") {
		t.Fatalf("missing global header: %q", got)
	}
	if !strings.Contains(got, "- brief") {
		t.Fatalf("missing global content: %q", got)
	}
	if strings.Contains(got, "Project") {
		t.Fatalf("should not contain project section: %q", got)
	}
}

func TestLoadAtProjectOnly(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project", "MEMORY.md")
	writeTestFile(t, projectPath, "- go test")
	got := LoadAt(filepath.Join(dir, "global", "MEMORY.md"), projectPath)
	if !strings.Contains(got, "# Project memory") {
		t.Fatalf("missing project header: %q", got)
	}
	if strings.Contains(got, "Global") {
		t.Fatalf("should not contain global section: %q", got)
	}
}

func TestLoadAtBoth(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global", "MEMORY.md")
	projectPath := filepath.Join(dir, "project", "MEMORY.md")
	writeTestFile(t, globalPath, "- g")
	writeTestFile(t, projectPath, "- p")
	got := LoadAt(globalPath, projectPath)
	if !strings.Contains(got, "# Global memory") {
		t.Fatalf("missing global header: %q", got)
	}
	if !strings.Contains(got, "# Project memory") {
		t.Fatalf("missing project header: %q", got)
	}
}

func TestLoadAtTruncatesOversizedContent(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project", "MEMORY.md")
	big := strings.Repeat("x", ProjectMaxBytes+100)
	writeTestFile(t, projectPath, big)
	got := LoadAt(filepath.Join(dir, "g", "MEMORY.md"), projectPath)
	if !strings.Contains(got, "[truncated") {
		t.Fatalf("expected truncation marker: %q", got)
	}
}

func TestLoadEmptyFileSkipped(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global", "MEMORY.md")
	writeTestFile(t, globalPath, "   \n  \n")
	got := LoadAt(globalPath, filepath.Join(dir, "p", "MEMORY.md"))
	if got != "" {
		t.Fatalf("empty/whitespace file should be skipped, got %q", got)
	}
}

func TestLoadAtHeadingShowsPathAndSize(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project", "MEMORY.md")
	writeTestFile(t, projectPath, "- run make test")
	got := LoadAt(filepath.Join(dir, "global", "MEMORY.md"), projectPath)
	want := "# Project memory (" + projectPath + ", 15 B of 16 KB)"
	if !strings.Contains(got, want) {
		t.Fatalf("expected heading %q in %q", want, got)
	}
}

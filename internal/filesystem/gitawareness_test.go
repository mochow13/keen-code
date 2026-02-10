package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitAwareness_IsIgnored(t *testing.T) {
	tests := []struct {
		name     string
		patterns string
		path     string
		ignored  bool
	}{
		{"node_modules dir", "node_modules/\n", "node_modules/lodash", true},
		{"log files", "*.log\n", "debug.log", true},
		{"nested path", "build/\n", "build/output.js", true},
		{"not ignored", "*.log\n", "main.go", false},
		{"different extension", "*.txt\n", "file.log", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			gitignorePath := filepath.Join(tmpDir, ".gitignore")
			if err := os.WriteFile(gitignorePath, []byte(tt.patterns), 0644); err != nil {
				t.Fatalf("failed to write .gitignore: %v", err)
			}

			ga := NewGitAwareness()
			if err := ga.LoadGitignore(gitignorePath); err != nil {
				t.Fatalf("failed to load gitignore: %v", err)
			}

			// Use full path for matching
			fullPath := filepath.Join(tmpDir, tt.path)
			got := ga.IsIgnored(fullPath)
			if got != tt.ignored {
				t.Errorf("IsIgnored(%q) = %v, want %v", fullPath, got, tt.ignored)
			}
		})
	}
}

func TestGitAwareness_LoadGitignore_NotExist(t *testing.T) {
	ga := NewGitAwareness()
	err := ga.LoadGitignore("/nonexistent/.gitignore")
	if err != nil {
		t.Errorf("expected no error for non-existent file, got %v", err)
	}
}

func TestGitAwareness_FilterPaths(t *testing.T) {
	tmpDir := t.TempDir()
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content := "*.log\nnode_modules/\n"
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	ga := NewGitAwareness()
	if err := ga.LoadGitignore(gitignorePath); err != nil {
		t.Fatalf("failed to load gitignore: %v", err)
	}

	paths := []string{
		filepath.Join(tmpDir, "main.go"),
		filepath.Join(tmpDir, "debug.log"),
		filepath.Join(tmpDir, "node_modules/lodash"),
		filepath.Join(tmpDir, "src/app.go"),
	}
	got := ga.FilterPaths(paths)
	want := []string{
		filepath.Join(tmpDir, "main.go"),
		filepath.Join(tmpDir, "src/app.go"),
	}

	if len(got) != len(want) {
		t.Errorf("FilterPaths() returned %d paths, want %d: got %v", len(got), len(want), got)
	}

	for i, p := range want {
		if i >= len(got) || got[i] != p {
			t.Errorf("FilterPaths()[%d] = %q, want %q", i, got[i], p)
		}
	}
}

func TestGitAwareness_CommentsAndEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content := "# This is a comment\n\n*.log\n  \n*.tmp\n"
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	ga := NewGitAwareness()
	if err := ga.LoadGitignore(gitignorePath); err != nil {
		t.Fatalf("failed to load gitignore: %v", err)
	}

	if !ga.IsIgnored(filepath.Join(tmpDir, "test.log")) {
		t.Error("expected test.log to be ignored")
	}
	if !ga.IsIgnored(filepath.Join(tmpDir, "test.tmp")) {
		t.Error("expected test.tmp to be ignored")
	}
	if ga.IsIgnored(filepath.Join(tmpDir, "test.go")) {
		t.Error("expected test.go to not be ignored")
	}
}

func TestGitAwareness_LoadGitignoreRecursive(t *testing.T) {
	tmpDir := t.TempDir()

	rootGitignore := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(rootGitignore, []byte("*.log\n"), 0644); err != nil {
		t.Fatalf("failed to write root .gitignore: %v", err)
	}
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	srcGitignore := filepath.Join(srcDir, ".gitignore")
	if err := os.WriteFile(srcGitignore, []byte("internal.go\n"), 0644); err != nil {
		t.Fatalf("failed to write src/.gitignore: %v", err)
	}

	utilsDir := filepath.Join(srcDir, "utils")
	if err := os.MkdirAll(utilsDir, 0755); err != nil {
		t.Fatalf("failed to create utils dir: %v", err)
	}
	utilsGitignore := filepath.Join(utilsDir, ".gitignore")
	if err := os.WriteFile(utilsGitignore, []byte("*.tmp\n"), 0644); err != nil {
		t.Fatalf("failed to write utils/.gitignore: %v", err)
	}

	ga := NewGitAwareness()
	if err := ga.LoadGitignoreRecursive(tmpDir); err != nil {
		t.Fatalf("failed to load gitignore recursively: %v", err)
	}
	if !ga.IsIgnored(filepath.Join(tmpDir, "debug.log")) {
		t.Error("expected debug.log to be ignored by root .gitignore")
	}
	if !ga.IsIgnored(filepath.Join(srcDir, "internal.go")) {
		t.Error("expected src/internal.go to be ignored by src/.gitignore")
	}
	if !ga.IsIgnored(filepath.Join(utilsDir, "cache.tmp")) {
		t.Error("expected utils/cache.tmp to be ignored by utils/.gitignore")
	}
}

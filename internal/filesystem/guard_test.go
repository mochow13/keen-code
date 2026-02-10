package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuard_CheckPath_ReadInWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGuard(tmpDir, nil)

	got := g.CheckPath("main.go", "read")
	if got != PermissionGranted {
		t.Errorf("CheckPath(main.go, read) = %v, want PermissionGranted", got)
	}
}

func TestGuard_CheckPath_WriteInWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGuard(tmpDir, nil)

	got := g.CheckPath("main.go", "write")
	if got != PermissionPending {
		t.Errorf("CheckPath(main.go, write) = %v, want PermissionPending", got)
	}
}

func TestGuard_CheckPath_ReadOutsideWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGuard(tmpDir, nil)

	got := g.CheckPath("/tmp/file.txt", "read")
	if got != PermissionPending {
		t.Errorf("CheckPath(/tmp/file.txt, read) = %v, want PermissionPending", got)
	}
}

func TestGuard_CheckPath_OutsideWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGuard(tmpDir, nil)

	// Access to parent directory should be PermissionPending (ask user), not denied
	got := g.CheckPath("../other-project/main.go", "read")
	if got != PermissionPending {
		t.Errorf("CheckPath(../other-project/main.go, read) = %v, want PermissionPending", got)
	}
}

func TestGuard_CheckPath_SensitivePath(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGuard(tmpDir, nil)

	got := g.CheckPath("/etc/passwd", "read")
	if got != PermissionDenied {
		t.Errorf("CheckPath(/etc/passwd, read) = %v, want PermissionDenied", got)
	}
}

func TestGuard_IsBlocked_Gitignore(t *testing.T) {
	tmpDir := t.TempDir()
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("*.log\n"), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	ga := NewGitAwareness()
	if err := ga.LoadGitignore(gitignorePath); err != nil {
		t.Fatalf("failed to load gitignore: %v", err)
	}

	g := NewGuard(tmpDir, ga)

	if !g.IsBlocked(filepath.Join(tmpDir, "debug.log")) {
		t.Error("expected debug.log to be blocked by gitignore")
	}
	if g.IsBlocked(filepath.Join(tmpDir, "main.go")) {
		t.Error("expected main.go to not be blocked")
	}
}

func TestGuard_CheckPath_ParentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGuard(tmpDir, nil)

	// Access to parent directory should be PermissionPending (ask user), not denied
	got := g.CheckPath("../other-project/main.go", "read")
	if got != PermissionPending {
		t.Errorf("CheckPath(../other-project/main.go, read) = %v, want PermissionPending", got)
	}
}

func TestGuard_ResolvePath(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGuard(tmpDir, nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"main.go", filepath.Join(tmpDir, "main.go")},
		{"./main.go", filepath.Join(tmpDir, "main.go")},
		{"src/main.go", filepath.Join(tmpDir, "src", "main.go")},
		{"/absolute/path", "/absolute/path"},
	}

	for _, tt := range tests {
		got, err := g.ResolvePath(tt.input)
		if err != nil {
			t.Errorf("ResolvePath(%q) error = %v", tt.input, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("ResolvePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestGuard_IsInWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGuard(tmpDir, nil)

	tests := []struct {
		path     string
		expected bool
	}{
		{tmpDir, true},
		{filepath.Join(tmpDir, "main.go"), true},
		{filepath.Join(tmpDir, "src", "main.go"), true},
		{"/etc/passwd", false},
		{filepath.Join(tmpDir, "..", "outside"), false},
	}

	for _, tt := range tests {
		got := g.IsInWorkingDir(tt.path)
		if got != tt.expected {
			t.Errorf("IsInWorkingDir(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

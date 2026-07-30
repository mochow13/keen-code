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

func TestGuard_CheckPath_AgentsSkillsDirGrantsReadOutsideWorkingDir(t *testing.T) {
	home := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(workingDir, nil)
	skillPath := filepath.Join(home, ".agents", "skills", "demo", "SKILL.md")
	got := g.CheckPath(skillPath, "read")
	if got != PermissionGranted {
		t.Errorf("CheckPath(~/.agents/skills skill, read) = %v, want PermissionGranted", got)
	}
}

func TestGuard_CheckPath_AgentsSkillsDirDoesNotGrantWrite(t *testing.T) {
	home := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(workingDir, nil)
	skillPath := filepath.Join(home, ".agents", "skills", "demo", "SKILL.md")
	got := g.CheckPath(skillPath, "write")
	if got != PermissionPending {
		t.Errorf("CheckPath(~/.agents/skills skill, write) = %v, want PermissionPending", got)
	}
}

func TestGuard_IsBlocked_SkillDirsAllowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(t.TempDir(), nil)
	for _, skillPath := range []string{
		filepath.Join(home, ".agents", "skills", "demo", "SKILL.md"),
		filepath.Join(home, ".keen", "skills", "builtin", "demo", "SKILL.md"),
		filepath.Join(home, ".claude", "skills", "demo", "SKILL.md"),
	} {
		if g.IsBlocked(skillPath) {
			t.Errorf("expected skill path to not be blocked: %s", skillPath)
		}
	}
}

func TestGuard_IsBlocked_AgentsOutsideSkillsDenied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(t.TempDir(), nil)
	path := filepath.Join(home, ".agents", "config.json")
	if !g.IsBlocked(path) {
		t.Error("expected ~/.agents path outside skills to be blocked")
	}
}

func TestGuard_CheckPath_KeenSkillsDirGrantsReadOutsideWorkingDir(t *testing.T) {
	home := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(workingDir, nil)
	skillPath := filepath.Join(home, ".keen", "skills", "builtin", "demo", "SKILL.md")
	got := g.CheckPath(skillPath, "read")
	if got != PermissionGranted {
		t.Errorf("CheckPath(~/.keen/skills skill, read) = %v, want PermissionGranted", got)
	}
}

func TestGuard_CheckPath_ClaudeSkillsDirGrantsReadOutsideWorkingDir(t *testing.T) {
	home := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(workingDir, nil)
	skillPath := filepath.Join(home, ".claude", "skills", "demo", "SKILL.md")
	got := g.CheckPath(skillPath, "read")
	if got != PermissionGranted {
		t.Errorf("CheckPath(~/.claude/skills skill, read) = %v, want PermissionGranted", got)
	}
}

func TestGuard_CheckPath_KeenBashDirGrantsRead(t *testing.T) {
	workingDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bashDir := filepath.Join(home, ".keen", "bash")
	if err := os.MkdirAll(bashDir, 0700); err != nil {
		t.Fatalf("failed to create bash output dir: %v", err)
	}

	g := NewGuard(workingDir, nil)
	if got := g.CheckPath(bashDir, "read"); got != PermissionGranted {
		t.Errorf("CheckPath(~/.keen/bash, read) = %v, want PermissionGranted", got)
	}

	path := filepath.Join(bashDir, "any-file.log")
	if err := os.WriteFile(path, []byte("output"), 0600); err != nil {
		t.Fatalf("failed to write bash output file: %v", err)
	}
	got := g.CheckPath(path, "read")
	if got != PermissionGranted {
		t.Errorf("CheckPath(keen bash file, read) = %v, want PermissionGranted", got)
	}
}

func TestGuard_CheckPath_WebFetchArtifactsDirGrantsRead(t *testing.T) {
	workingDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	artifactsDir := filepath.Join(home, ".keen", "web-fetch-artifacts")
	if err := os.MkdirAll(artifactsDir, 0700); err != nil {
		t.Fatalf("failed to create web fetch artifacts dir: %v", err)
	}

	path := filepath.Join(artifactsDir, "keen-web-fetch-123.txt")
	if err := os.WriteFile(path, []byte("output"), 0600); err != nil {
		t.Fatalf("failed to write web fetch artifact: %v", err)
	}

	g := NewGuard(workingDir, nil)
	if got := g.CheckPath(path, "read"); got != PermissionGranted {
		t.Errorf("CheckPath(web fetch artifact, read) = %v, want PermissionGranted", got)
	}
}

func TestGuard_CheckPath_WebFetchArtifactsDirRejectsSymlink(t *testing.T) {
	workingDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	artifactsDir := filepath.Join(home, ".keen", "web-fetch-artifacts")
	if err := os.MkdirAll(artifactsDir, 0700); err != nil {
		t.Fatalf("failed to create web fetch artifacts dir: %v", err)
	}

	target := filepath.Join(workingDir, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatalf("failed to write target file: %v", err)
	}

	path := filepath.Join(artifactsDir, "linked-artifact")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	g := NewGuard(workingDir, nil)
	if got := g.CheckPath(path, "read"); got != PermissionDenied {
		t.Errorf("CheckPath(web fetch artifact symlink, read) = %v, want PermissionDenied", got)
	}
}

func TestGuard_CheckPath_KeenBashDirDoesNotGrantWrite(t *testing.T) {
	workingDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bashDir := filepath.Join(home, ".keen", "bash")
	if err := os.MkdirAll(bashDir, 0700); err != nil {
		t.Fatalf("failed to create bash output dir: %v", err)
	}

	g := NewGuard(workingDir, nil)
	path := filepath.Join(bashDir, "any-file.log")
	if err := os.WriteFile(path, []byte("output"), 0600); err != nil {
		t.Fatalf("failed to write bash output file: %v", err)
	}
	got := g.CheckPath(path, "write")
	if got != PermissionPending {
		t.Errorf("CheckPath(keen bash file, write) = %v, want PermissionPending", got)
	}
}

func TestGuard_CheckPath_KeenBashDirRejectsSymlink(t *testing.T) {
	workingDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bashDir := filepath.Join(home, ".keen", "bash")
	if err := os.MkdirAll(bashDir, 0700); err != nil {
		t.Fatalf("failed to create bash output dir: %v", err)
	}

	target := filepath.Join(workingDir, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatalf("failed to write target file: %v", err)
	}

	path := filepath.Join(bashDir, "linked-output")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	g := NewGuard(workingDir, nil)
	got := g.CheckPath(path, "read")
	if got != PermissionDenied {
		t.Errorf("CheckPath(keen bash symlink, read) = %v, want PermissionDenied", got)
	}
}

func TestGuard_CheckPath_KeenBashDirRequiresExactDir(t *testing.T) {
	workingDir := t.TempDir()
	home := t.TempDir()
	otherDir := t.TempDir()
	t.Setenv("HOME", home)
	bashDir := filepath.Join(home, ".keen", "bash")
	if err := os.MkdirAll(bashDir, 0700); err != nil {
		t.Fatalf("failed to create bash output dir: %v", err)
	}

	g := NewGuard(workingDir, nil)
	for _, tc := range []struct {
		path string
		want Permission
	}{
		{filepath.Join(otherDir, "keen-bash-123.stdout"), PermissionPending},
		{filepath.Join(home, ".keen", "not-bash", "keen-bash-123.stdout"), PermissionDenied},
		{filepath.Join(bashDir+"-other", "output"), PermissionDenied},
	} {
		if err := os.MkdirAll(filepath.Dir(tc.path), 0700); err != nil {
			t.Fatalf("failed to create parent dir: %v", err)
		}
		if err := os.WriteFile(tc.path, []byte("output"), 0600); err != nil {
			t.Fatalf("failed to write candidate file: %v", err)
		}
		got := g.CheckPath(tc.path, "read")
		if got != tc.want {
			t.Errorf("CheckPath(%q, read) = %v, want %v", tc.path, got, tc.want)
		}
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

func TestGuard_CheckPath_MemoryDirGrantsReadOutsideWorkingDir(t *testing.T) {
	home := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(workingDir, nil)
	memPath := filepath.Join(home, ".keen", "memory", "global", "MEMORY.md")
	got := g.CheckPath(memPath, "read")
	if got != PermissionGranted {
		t.Errorf("CheckPath(memory file, read) = %v, want PermissionGranted", got)
	}
}

func TestGuard_CheckPath_MemoryDirWriteIsPending(t *testing.T) {
	home := t.TempDir()
	workingDir := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(workingDir, nil)
	memPath := filepath.Join(home, ".keen", "memory", "global", "MEMORY.md")
	got := g.CheckPath(memPath, "write")
	if got != PermissionPending {
		t.Errorf("CheckPath(memory file, write) = %v, want PermissionPending", got)
	}
}

func TestGuard_IsBlocked_MemoryDirAllowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(t.TempDir(), nil)
	memPath := filepath.Join(home, ".keen", "memory", "global", "MEMORY.md")
	if g.IsBlocked(memPath) {
		t.Errorf("expected memory path to not be blocked: %s", memPath)
	}
}

func TestGuard_IsBlocked_KeenOutsideMemoryDenied(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	g := NewGuard(t.TempDir(), nil)
	path := filepath.Join(home, ".keen", "configs.json")
	if !g.IsBlocked(path) {
		t.Error("expected ~/.keen path outside memory to be blocked")
	}
}

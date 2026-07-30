package filesystem

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Permission int

const (
	PermissionDenied Permission = iota
	PermissionGranted
	PermissionPending
)

type Guard struct {
	workingDir   string
	blockedPaths []string
	gitignore    *GitAwareness
}

func NewGuard(workingDir string, gitignore *GitAwareness) *Guard {
	return &Guard{
		workingDir:   workingDir,
		gitignore:    gitignore,
		blockedPaths: defaultBlockedPaths(),
	}
}

func defaultBlockedPaths() []string {
	return []string{
		"/etc",
		"/usr",
		"/bin",
		"/sbin",
		"/lib",
		"/lib64",
		"/proc",
		"/sys",
		"/dev",
		"/root",
	}
}

func (g *Guard) CheckPath(path string, operation string) Permission {
	if g.IsBlocked(path) {
		slog.Debug("path blocked", "path", path)
		return PermissionDenied
	}

	resolved, err := g.ResolvePath(path)
	if err != nil {
		slog.Debug("path resolution failed", "path", path, "error", err)
		return PermissionDenied
	}

	if operation == "read" && (g.IsInSkillDir(resolved) || g.IsInKeenBashDir(resolved) || g.IsInMCPArtifactsDir(resolved) || g.IsInWebFetchArtifactsDir(resolved) || g.IsInMemoryDir(resolved)) {
		return PermissionGranted
	}

	if operation == "write" || operation == "edit" {
		if g.IsInMemoryDir(resolved) {
			return PermissionPending
		}
	}

	inWorkingDir := g.IsInWorkingDir(resolved)

	switch operation {
	case "read":
		if inWorkingDir {
			return PermissionGranted
		}
		return PermissionPending
	case "write", "edit":
		return PermissionPending
	default:
		return PermissionDenied
	}
}

func (g *Guard) IsBlocked(path string) bool {
	resolved, err := g.ResolvePath(path)
	if err != nil {
		return true
	}

	if g.gitignore != nil && (g.gitignore.IsIgnored(path) || g.gitignore.IsIgnored(resolved)) {
		return true
	}

	if g.IsInSkillDir(resolved) || g.IsInKeenBashDir(resolved) || g.IsInMCPArtifactsDir(resolved) || g.IsInWebFetchArtifactsDir(resolved) || g.IsInMemoryDir(resolved) {
		return false
	}

	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(resolved, home+string(filepath.Separator)+".") {
		return true
	}

	for _, blocked := range g.blockedPaths {
		if strings.HasPrefix(resolved, blocked) {
			return true
		}
	}

	return false
}

func (g *Guard) ResolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		cleaned := filepath.Clean(path)
		return cleaned, nil
	}

	resolved := filepath.Join(g.workingDir, path)
	cleaned := filepath.Clean(resolved)
	return cleaned, nil
}

func (g *Guard) IsInKeenBashDir(path string) bool {
	cleaned := filepath.Clean(path)
	bashDir, err := KeenBashOutputDir()
	if err != nil {
		return false
	}
	bashDir = filepath.Clean(bashDir)
	if cleaned == bashDir {
		return true
	}
	prefix := bashDir + string(filepath.Separator)
	if !strings.HasPrefix(cleaned+string(filepath.Separator), prefix) {
		return false
	}

	info, err := os.Lstat(cleaned)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return true
}

func KeenBashOutputDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return filepath.Join(home, ".keen", "bash"), nil
}

func KeenMCPArtifactsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return filepath.Join(home, ".keen", "mcp-artifacts"), nil
}

func (g *Guard) IsInMCPArtifactsDir(path string) bool {
	cleaned := filepath.Clean(path)
	artifactsDir, err := KeenMCPArtifactsDir()
	if err != nil {
		return false
	}
	artifactsDir = filepath.Clean(artifactsDir)
	if cleaned == artifactsDir {
		return true
	}
	prefix := artifactsDir + string(filepath.Separator)
	if !strings.HasPrefix(cleaned+string(filepath.Separator), prefix) {
		return false
	}

	info, err := os.Lstat(cleaned)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return true
}

func KeenWebFetchArtifactsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return filepath.Join(home, ".keen", "web-fetch-artifacts"), nil
}

func (g *Guard) IsInWebFetchArtifactsDir(path string) bool {
	cleaned := filepath.Clean(path)
	artifactsDir, err := KeenWebFetchArtifactsDir()
	if err != nil {
		return false
	}
	artifactsDir = filepath.Clean(artifactsDir)
	if cleaned == artifactsDir {
		return true
	}
	prefix := artifactsDir + string(filepath.Separator)
	if !strings.HasPrefix(cleaned+string(filepath.Separator), prefix) {
		return false
	}

	info, err := os.Lstat(cleaned)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return true
}

func (g *Guard) IsInWorkingDir(path string) bool {
	cleaned := filepath.Clean(path)
	workingDirClean := filepath.Clean(g.workingDir)

	if cleaned == workingDirClean {
		return true
	}

	prefix := workingDirClean + string(filepath.Separator)
	return strings.HasPrefix(cleaned+string(filepath.Separator), prefix)
}

func (g *Guard) IsInSkillDir(path string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}

	cleaned := filepath.Clean(path)
	for _, dir := range []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".keen", "skills"),
		filepath.Join(home, ".claude", "skills"),
	} {
		if cleaned == dir {
			return true
		}
		prefix := dir + string(filepath.Separator)
		if strings.HasPrefix(cleaned+string(filepath.Separator), prefix) {
			return true
		}
	}
	return false
}

func (g *Guard) IsInMemoryDir(path string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	memoryDir := filepath.Join(home, ".keen", "memory")
	cleaned := filepath.Clean(path)
	if cleaned == memoryDir {
		return true
	}
	prefix := memoryDir + string(filepath.Separator)
	return strings.HasPrefix(cleaned+string(filepath.Separator), prefix)
}

func (g *Guard) IsMemoryPath(path string) bool {
	cleaned := filepath.Clean(path)
	if g.IsInMemoryDir(cleaned) {
		return true
	}
	if g.workingDir == "" {
		return false
	}
	projectMemory := filepath.Join(filepath.Clean(g.workingDir), ".keen", "MEMORY.md")
	return cleaned == projectMemory
}

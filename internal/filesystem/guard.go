package filesystem

import (
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
		slog.Info("path resolution failed", "path", path, "error", err)
		return PermissionDenied
	}

	inWorkingDir := g.IsInWorkingDir(resolved)

	switch operation {
	case "read":
		if inWorkingDir {
			return PermissionGranted
		}
		return PermissionPending
	case "write":
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

func (g *Guard) IsInWorkingDir(path string) bool {
	cleaned := filepath.Clean(path)
	workingDirClean := filepath.Clean(g.workingDir)

	if cleaned == workingDirClean {
		return true
	}

	prefix := workingDirClean + string(filepath.Separator)
	return strings.HasPrefix(cleaned+string(filepath.Separator), prefix)
}

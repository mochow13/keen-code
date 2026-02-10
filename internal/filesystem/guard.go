package filesystem

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Guard struct {
	workingDir   string
	blockedPaths []string
}

func NewGuard(workingDir string, blockedPaths []string) *Guard {
	return &Guard{
		workingDir:   workingDir,
		blockedPaths: blockedPaths,
	}
}

func (g *Guard) ValidatePath(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains directory traversal: %s", path)
	}

	// TODO: Check blocked paths
	return nil
}

func (g *Guard) ResolvePath(path string) (string, error) {
	if err := g.ValidatePath(path); err != nil {
		return "", err
	}

	resolved := filepath.Join(g.workingDir, path)
	cleaned := filepath.Clean(resolved)
	return cleaned, nil
}

package tools

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/user/keen-code/internal/filesystem"
)

const maxFileLimit = 1000

type GlobTool struct {
	guard               *filesystem.Guard
	permissionRequester PermissionRequester
}

func NewGlobTool(guard *filesystem.Guard, permissionRequester PermissionRequester) *GlobTool {
	return &GlobTool{
		guard:               guard,
		permissionRequester: permissionRequester,
	}
}

func (t *GlobTool) Name() string {
	return "glob"
}

func (t *GlobTool) Description() string {
	return "Search for files matching a glob pattern after filesystem policy + user permission checks."
}

func (t *GlobTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match files (e.g., '*.go', '**/*.md', '/absolute/path/*.txt')",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional base directory for the search (defaults to working directory)",
			},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	}
}

func (t *GlobTool) Execute(ctx context.Context, input any) (any, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}

	patternValue, ok := params["pattern"]
	if !ok {
		return nil, fmt.Errorf("invalid input: missing required 'pattern' parameter")
	}

	pattern, ok := patternValue.(string)
	if !ok || pattern == "" {
		return nil, fmt.Errorf("invalid input: pattern must be a non-empty string")
	}

	if !doublestar.ValidatePattern(pattern) {
		return nil, fmt.Errorf("invalid pattern: malformed glob pattern %q", pattern)
	}

	basePath := ""
	if pathValue, exists := params["path"]; exists {
		if pathStr, ok := pathValue.(string); ok {
			basePath = pathStr
		}
	}

	resolvedBasePath, err := t.guard.ResolvePath(basePath)
	if err != nil {
		return nil, fmt.Errorf("path resolution failed: %w", err)
	}

	permission := t.guard.CheckPath(resolvedBasePath, "read")

	switch permission {
	case filesystem.PermissionDenied:
		return nil, fmt.Errorf("permission denied by policy: path %q is blocked", basePath)
	case filesystem.PermissionPending:
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required but not available")
		}
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), basePath, resolvedBasePath, false)
		if err != nil {
			return nil, fmt.Errorf("permission request failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("permission denied by user: read access rejected for path %q", basePath)
		}
	}

	files, err := t.searchFiles(resolvedBasePath, pattern)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"pattern":   pattern,
		"base_path": resolvedBasePath,
		"files":     files,
		"count":     len(files),
	}, nil
}

func (t *GlobTool) searchFiles(basePath, pattern string) ([]string, error) {
	var matches []string
	seen := make(map[string]bool)

	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if t.guard.IsBlocked(path) {
				return fs.SkipDir
			}
			return nil
		}

		if t.guard.IsBlocked(path) {
			return nil
		}

		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return nil
		}

		matched, err := doublestar.Match(pattern, filepath.ToSlash(relPath))
		if err != nil {
			return err
		}

		if matched {
			if len(matches) >= maxFileLimit {
				return fmt.Errorf("search too broad: found more than %d files matching pattern %q", maxFileLimit, pattern)
			}
			normalizedPath := filepath.Clean(path)
			if !seen[normalizedPath] {
				seen[normalizedPath] = true
				matches = append(matches, normalizedPath)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return matches, nil
}

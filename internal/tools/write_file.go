package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/user/keen-code/internal/filesystem"
)

type WriteFileTool struct {
	guard               *filesystem.Guard
	permissionRequester PermissionRequester
}

func NewWriteFileTool(guard *filesystem.Guard, permissionRequester PermissionRequester) *WriteFileTool {
	return &WriteFileTool{
		guard:               guard,
		permissionRequester: permissionRequester,
	}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Write content to a file. Creates parent directories if needed. Overwrites existing files."
}

func (t *WriteFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file to write",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required":             []string{"path", "content"},
		"additionalProperties": false,
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, input any) (any, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}

	pathValue, ok := params["path"]
	if !ok {
		return nil, fmt.Errorf("invalid input: missing required 'path' parameter")
	}

	path, ok := pathValue.(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("invalid input: path must be a non-empty string")
	}

	contentValue, ok := params["content"]
	if !ok {
		return nil, fmt.Errorf("invalid input: missing required 'content' parameter")
	}

	content, ok := contentValue.(string)
	if !ok {
		return nil, fmt.Errorf("invalid input: content must be a string")
	}

	resolvedPath, err := t.guard.ResolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("path resolution failed: %w", err)
	}

	permission := t.guard.CheckPath(path, "write")

	switch permission {
	case filesystem.PermissionDenied:
		return nil, fmt.Errorf("permission denied by policy: path %q is blocked", path)
	case filesystem.PermissionPending:
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required but not available")
		}
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), path, resolvedPath, "write", false)
		if err != nil {
			return nil, fmt.Errorf("permission request failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("permission denied by user: write access rejected for path %q", path)
		}
	}

	created, err := writeFileContent(resolvedPath, content)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"path":          resolvedPath,
		"bytes_written": len(content),
		"created":       created,
	}, nil
}

func writeFileContent(path string, content string) (bool, error) {
	parentDir := filepath.Dir(path)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create parent directories: %w", err)
	}

	_, err := os.Stat(path)
	created := os.IsNotExist(err)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return false, fmt.Errorf("write failed: %w", err)
	}

	return created, nil
}

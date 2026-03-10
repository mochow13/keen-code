package tools

import (
	"context"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/user/keen-code/internal/filesystem"
)

const (
	maxFileSize = 10_485_760 // 10MB
)

type ReadFileTool struct {
	guard               *filesystem.Guard
	permissionRequester PermissionRequester
}

func NewReadFileTool(guard *filesystem.Guard, permissionRequester PermissionRequester) *ReadFileTool {
	return &ReadFileTool{
		guard:               guard,
		permissionRequester: permissionRequester,
	}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read a UTF-8 text file after filesystem policy + user permission checks."
}

func (t *ReadFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file to read",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, input any) (any, error) {
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

	resolvedPath, err := t.guard.ResolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("path resolution failed: %w", err)
	}

	permission := t.guard.CheckPath(path, "read")

	switch permission {
	case filesystem.PermissionDenied:
		return nil, fmt.Errorf("permission denied by policy: path %q is blocked", path)
	case filesystem.PermissionPending:
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required but not available")
		}
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), path, resolvedPath, "read", false)
		if err != nil {
			return nil, fmt.Errorf("permission request failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("permission denied by user: read access rejected for path %q", path)
		}
	}

	content, err := readFileContent(resolvedPath)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"path":       resolvedPath,
		"content":    string(content),
		"bytes_read": len(content),
	}, nil
}

func readFileContent(path string) ([]byte, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not found: file %q does not exist", path)
		}
		return nil, fmt.Errorf("not accessible: %w", err)
	}

	if stat.IsDir() {
		return nil, fmt.Errorf("not a file: %q is a directory", path)
	}

	if stat.Size() > maxFileSize {
		return nil, fmt.Errorf("file too large: %q is %d bytes (max %d)", path, stat.Size(), maxFileSize)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	if containsNullByte(content) {
		return nil, fmt.Errorf("not a text file: %q contains null bytes (likely binary)", path)
	}

	if !utf8.Valid(content) {
		return nil, fmt.Errorf("not a text file: %q contains invalid UTF-8", path)
	}

	return content, nil
}

func containsNullByte(content []byte) bool {
	for _, b := range content {
		if b == 0x00 {
			return true
		}
	}
	return false
}

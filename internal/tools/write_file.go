package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mochow13/keen-code/internal/filesystem"
	"github.com/mochow13/keen-code/internal/memory"
)

type WriteFileTool struct {
	guard               *filesystem.Guard
	diffEmitter         DiffEmitter
	permissionRequester PermissionRequester
}

func NewWriteFileTool(guard *filesystem.Guard, diffEmitter DiffEmitter, permissionRequester PermissionRequester) *WriteFileTool {
	return &WriteFileTool{
		guard:               guard,
		diffEmitter:         diffEmitter,
		permissionRequester: permissionRequester,
	}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return `Write content to a file. Creates parent directories if needed. Overwrites existing files.

Use this through the tool API whenever you say you will create, write, replace, or overwrite a file. Do not merely describe file writing in assistant text.

- Use this for creating new files or completely replacing file contents
- Do not use this for targeted modifications to existing files — use edit_file instead,
which is safer and preserves surrounding content

IMPORTANT:
- This tool performs a full overwrite. If the file already exists, all previous
content is replaced. Always verify the target path before overwriting.`
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

func (t *WriteFileTool) ValidateInput(_ context.Context, input any) error {
	params, ok := input.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	path, ok := params["path"].(string)
	if !ok || path == "" {
		if _, exists := params["path"]; !exists {
			return missingWriteFileParameter("path")
		}
		return fmt.Errorf("invalid input: path must be a non-empty string")
	}
	if _, exists := params["content"]; !exists {
		return missingWriteFileParameter("content")
	}
	if _, ok := params["content"].(string); !ok {
		return fmt.Errorf("invalid input: content must be a string")
	}
	return nil
}

func missingWriteFileParameter(name string) error {
	return missingRequiredParameter(
		"write_file",
		name,
		`{"path":"<file path>","content":"<complete file content>"}`,
		"content may be empty, but it must be provided",
	)
}

func (t *WriteFileTool) Execute(ctx context.Context, input any) (any, error) {
	params := input.(map[string]any)
	path := params["path"].(string)
	content := params["content"].(string)

	resolvedPath, err := t.guard.ResolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("path resolution failed: %w", err)
	}

	oldContent := ""
	if data, err := os.ReadFile(resolvedPath); err == nil {
		oldContent = string(data)
	}

	if t.diffEmitter != nil {
		t.diffEmitter.EmitDiff(computeEditDiff(oldContent, content))
	}

	permission := t.guard.CheckPath(path, "write")

	if t.guard.IsMemoryPath(resolvedPath) && memory.ContainsSecret(content) {
		return nil, fmt.Errorf("refusing to write memory file: content appears to contain a secret, token, or credential")
	}

	switch permission {
	case filesystem.PermissionDenied:
		return nil, fmt.Errorf("permission denied by policy: path %q is blocked", path)
	case filesystem.PermissionPending:
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required but not available")
		}
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), path, resolvedPath, false)
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

	return map[string]any{"created": created}, nil
}

func writeFileContent(path string, content string) (bool, error) {
	parentDir := filepath.Dir(path)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create parent directories: %w", err)
	}

	_, err := os.Stat(path)
	created := os.IsNotExist(err)

	if err := writeFileAtomic(path, []byte(content)); err != nil {
		return false, err
	}

	return created, nil
}

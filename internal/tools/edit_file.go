package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	udiff "github.com/aymanbagabas/go-udiff"
	"github.com/user/keen-code/internal/filesystem"
)

type EditFileTool struct {
	guard               *filesystem.Guard
	diffEmitter         DiffEmitter
	permissionRequester PermissionRequester
}

func NewEditFileTool(guard *filesystem.Guard, diffEmitter DiffEmitter, permissionRequester PermissionRequester) *EditFileTool {
	return &EditFileTool{
		guard:               guard,
		diffEmitter:         diffEmitter,
		permissionRequester: permissionRequester,
	}
}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return `Edit a file by replacing occurrences of a string. The file must already exist.

Use this for targeted modifications to existing files. Prefer this over write_file
when you only need to change part of a file.

IMPORTANT:
- Always read the file first to get the exact current content.
- oldString must match the file content exactly, including whitespace and indentation.
- If oldString is not found, the edit fails. Copy text precisely from read_file output.
- Do not use line numbers in oldString; use the actual text content.`
}

func (t *EditFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file to edit",
			},
			"oldString": map[string]any{
				"type":        "string",
				"description": "The exact text to find and replace. Must match file content precisely, including whitespace and indentation. Include enough surrounding context to make the match unique.",
			},
			"newString": map[string]any{
				"type":        "string",
				"description": "The replacement text. Can be empty to delete the matched text.",
			},
			"shouldReplaceAll": map[string]any{
				"type":        "boolean",
				"description": "Whether to replace all occurrences (default: false, replaces only the first). Only set to true when every occurrence should be changed.",
			},
		},
		"required":             []string{"path", "oldString", "newString"},
		"additionalProperties": false,
	}
}

func (t *EditFileTool) Execute(ctx context.Context, input any) (any, error) {
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

	oldStringValue, ok := params["oldString"]
	if !ok {
		return nil, fmt.Errorf("invalid input: missing required 'oldString' parameter")
	}
	oldString, ok := oldStringValue.(string)
	if !ok {
		return nil, fmt.Errorf("invalid input: oldString must be a string")
	}

	newStringValue, ok := params["newString"]
	if !ok {
		return nil, fmt.Errorf("invalid input: missing required 'newString' parameter")
	}
	newString, ok := newStringValue.(string)
	if !ok {
		return nil, fmt.Errorf("invalid input: newString must be a string")
	}

	shouldReplaceAll := false
	if v, ok := params["shouldReplaceAll"]; ok {
		if b, ok := v.(bool); ok {
			shouldReplaceAll = b
		}
	}

	resolvedPath, err := t.guard.ResolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("path resolution failed: %w", err)
	}

	permission := t.guard.CheckPath(path, "edit")
	if permission == filesystem.PermissionDenied {
		return nil, fmt.Errorf("permission denied by policy: path %q is blocked", path)
	}

	contentBytes, err := readFileContent(resolvedPath)
	if err != nil {
		return nil, err
	}
	oldContent := string(contentBytes)

	if !strings.Contains(oldContent, oldString) {
		return nil, fmt.Errorf("oldString not found in file %q", path)
	}

	var newContent string
	var replacementCount int
	if shouldReplaceAll {
		newContent = strings.ReplaceAll(oldContent, oldString, newString)
		replacementCount = strings.Count(oldContent, oldString)
	} else {
		newContent = strings.Replace(oldContent, oldString, newString, 1)
		replacementCount = 1
	}

	t.diffEmitter.EmitDiff(computeEditDiff(oldContent, newContent))

	if permission == filesystem.PermissionPending {
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required but not available")
		}
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), path, resolvedPath, false)
		if err != nil {
			return nil, fmt.Errorf("permission request failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("permission denied by user: edit access rejected for path %q", path)
		}
	}

	if err := os.WriteFile(resolvedPath, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("write failed: %w", err)
	}

	return map[string]any{
		"success":          true,
		"path":             resolvedPath,
		"replacementCount": replacementCount,
	}, nil
}

func computeEditDiff(oldContent, newContent string) []EditDiffLine {
	edits := udiff.Strings(oldContent, newContent)
	unified, err := udiff.ToUnifiedDiff("old", "new", oldContent, edits, 3)
	if err != nil {
		return nil
	}

	var out []EditDiffLine
	for _, hunk := range unified.Hunks {
		fromCount, toCount := 0, 0
		for _, l := range hunk.Lines {
			switch l.Kind {
			case udiff.Delete:
				fromCount++
			case udiff.Insert:
				toCount++
			default:
				fromCount++
				toCount++
			}
		}
		out = append(out, EditDiffLine{
			Kind:    DiffLineHunk,
			Content: fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.FromLine, fromCount, hunk.ToLine, toCount),
		})

		oldLine := hunk.FromLine
		newLine := hunk.ToLine
		for _, l := range hunk.Lines {
			content := strings.TrimRight(l.Content, "\n")
			switch l.Kind {
			case udiff.Equal:
				out = append(out, EditDiffLine{Kind: DiffLineContext, OldLineNum: oldLine, NewLineNum: newLine, Content: content})
				oldLine++
				newLine++
			case udiff.Delete:
				out = append(out, EditDiffLine{Kind: DiffLineRemoved, OldLineNum: oldLine, Content: content})
				oldLine++
			case udiff.Insert:
				out = append(out, EditDiffLine{Kind: DiffLineAdded, NewLineNum: newLine, Content: content})
				newLine++
			}
		}
	}
	return out
}

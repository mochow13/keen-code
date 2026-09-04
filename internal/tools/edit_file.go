package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	udiff "github.com/aymanbagabas/go-udiff"
	"github.com/mochow13/keen-code/internal/filesystem"
	"github.com/mochow13/keen-code/internal/memory"
)

const (
	opReplace      = "replace"
	opInsertAfter  = "insert_after"
	opInsertBefore = "insert_before"
	opInsertHead   = "insert_head"
	opInsertTail   = "insert_tail"
)

type EditOp struct {
	Op    string
	Start string
	End   string
	Text  string
}

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
	return EditFileToolName
}

func (t *EditFileTool) Description() string {
	return `Edit a file with hash-anchored operations. The file must already exist.

Use this for targeted modifications to existing files. Prefer this over write_file
when you only need to change part of a file.

Each op addresses lines with LINE:HASH anchors (line number + 3-character hash) taken from read_file or grep output:
- replace (default): replaces the line at start, or the inclusive start..end range, with text. Empty text deletes the range.
- insert_after / insert_before: insert text after or before the anchored line.
- insert_head / insert_tail: insert text at the start or end of the file; these ops take no anchor.

IMPORTANT:
- Read the file first to obtain current N:HASH| anchors; a stale hash is rejected, so re-read after any change.
- Put all edits to one file in one ops array. The tool validates every anchor against one file snapshot and applies the edits atomically.
- You may emit separate calls for different files in one turn. Read the file again before making a later same-file edit.
- On a line/hash mismatch, re-read the affected area and retry with fresh anchors.`
}

func (t *EditFileTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file to edit",
			},
			"ops": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "Non-empty array of edit operations for this one file, all validated against one file snapshot and applied atomically.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"op": map[string]any{
							"type":        "string",
							"enum":        []string{opReplace, opInsertAfter, opInsertBefore, opInsertHead, opInsertTail},
							"description": "Operation kind, defaults to \"replace\". replace swaps the anchored line (or start..end range) for text; insert_after and insert_before insert text relative to the anchored line; insert_head and insert_tail insert text at the file start/end and take no anchor.",
						},
						"start": map[string]any{
							"type":        "string",
							"description": "LINE:HASH anchor for replace, insert_after, and insert_before. Required for those ops; not used by insert_head/insert_tail.",
						},
						"end": map[string]any{
							"type":        "string",
							"description": "Optional inclusive end LINE:HASH anchor; allowed only for replace, making it a range replacement.",
						},
						"text": map[string]any{
							"type":        "string",
							"description": "Replacement or inserted text. May be multiline. Empty text deletes a replace range.",
						},
					},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"path", "ops"},
		"additionalProperties": false,
	}
}

func (t *EditFileTool) ValidateInput(_ context.Context, input any) error {
	params, ok := input.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	path, ok := params["path"].(string)
	if !ok || path == "" {
		if _, exists := params["path"]; !exists {
			return missingEditFileParameter("path")
		}
		return fmt.Errorf("invalid input: path must be a non-empty string")
	}
	if _, exists := params["ops"]; !exists {
		return missingEditFileParameter("ops")
	}
	if _, err := parseEditOps(params["ops"]); err != nil {
		return err
	}
	return nil
}

func missingEditFileParameter(name string) error {
	return missingRequiredParameter(
		EditFileToolName,
		name,
		`{"path":"<existing file path>","ops":[{"start":"1:a3f","text":"<replacement text>"}]}`,
		"Read the file first; put all edits to one file in one ops array, validated against one snapshot and applied atomically",
	)
}

func applyEditOps(content []byte, ops []EditOp) ([]byte, error) {
	validated, err := validateEditOps(content, ops)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(validated, func(i, j int) bool {
		if validated[i].start != validated[j].start {
			return validated[i].start > validated[j].start
		}
		return validated[i].end > validated[j].end
	})

	lines := splitRawLines(content)
	work := make([][]byte, len(lines))
	copy(work, lines)
	for _, v := range validated {
		ins := splitRawLines([]byte(v.op.Text))
		head := work[:v.start]
		tail := work[v.end:]
		merged := make([][]byte, 0, len(head)+len(ins)+len(tail))
		merged = append(merged, head...)
		merged = append(merged, ins...)
		merged = append(merged, tail...)
		work = merged
	}
	return joinLines(work, content), nil
}

func (t *EditFileTool) Execute(ctx context.Context, input any) (any, error) {
	params := input.(map[string]any)

	requestJSON, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal edit_file request: %w", err)
	}
	slog.Debug(EditFileToolName, "request", string(requestJSON))

	path := params["path"].(string)
	ops, err := parseEditOps(params["ops"])
	if err != nil {
		return nil, err
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

	finalContent, err := applyEditOps(contentBytes, ops)
	if err != nil {
		return nil, err
	}
	newContent := string(finalContent)

	if t.guard.IsMemoryPath(resolvedPath) && memory.ContainsSecret(newContent) {
		return nil, fmt.Errorf("refusing to write memory file: content appears to contain a secret, token, or credential")
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

	if err := writeFileAtomic(resolvedPath, finalContent); err != nil {
		return nil, err
	}

	return map[string]any{
		"edited": true,
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

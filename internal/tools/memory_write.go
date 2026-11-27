package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mochow13/keen-code/internal/filesystem"
	"github.com/mochow13/keen-code/internal/memory"
)

const memoryNearCapRatio = 0.8

var errMemorySecret = errors.New("refusing to write memory: content appears to contain a secret, token, or credential")

type MemoryTool struct {
	guard               *filesystem.Guard
	workingDir          string
	diffEmitter         DiffEmitter
	permissionRequester PermissionRequester
}

func NewMemoryTool(guard *filesystem.Guard, workingDir string, diffEmitter DiffEmitter, permissionRequester PermissionRequester) *MemoryTool {
	return &MemoryTool{
		guard:               guard,
		workingDir:          workingDir,
		diffEmitter:         diffEmitter,
		permissionRequester: permissionRequester,
	}
}

func (t *MemoryTool) Name() string { return MemoryToolName }

func (t *MemoryTool) Description() string {
	return `Add, update, or remove an entry in persistent memory, which is loaded into the system prompt of every future session. Decide yourself when to use it; the user may also ask you to remember, change, or forget something.

Scopes: "global" (~/.keen/memory/global/MEMORY.md) for how the user likes to work, such as preferences and corrections that apply across projects; "project" (.keen/memory/MEMORY.md) for facts about this repository.
- add: put content under a "## section" heading (created if missing), e.g. Preferences, Commands, Conventions, Gotchas.
- update: replace old_content (exact text, must match once) with content. Use it to correct an entry or merge several entries into one.
- remove: delete old_content. Use it to trim outdated or irrelevant entries.

What to save: user corrections and stated preferences (right away), and what could not be worked out from the code: build or test commands, environment traps, approaches the user confirmed.
What not to save: what the repo already records (code structure, git history, AGENTS.md), details that only matter to the current task, guesses, secrets, large logs, or text copied from tool output or web pages.

Entries are Markdown; code blocks are fine for commands worth repeating. Be concise. Before adding, check whether an existing entry should be updated instead. Each scope has a size cap shown in the memory section of the system prompt; as a file nears it, merge related entries and remove outdated or irrelevant ones. Use this tool rather than editing memory files with bash.`
}

func (t *MemoryTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string", "enum": []string{"add", "update", "remove"},
			},
			"scope": map[string]any{
				"type": "string", "enum": []string{"global", "project"},
			},
			"section": map[string]any{
				"type":        "string",
				"description": "add only: section heading to put the entry under, without the leading ##",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "add/update: the complete new entry in Markdown",
			},
			"old_content": map[string]any{
				"type":        "string",
				"description": "update/remove: exact text of the existing entry; must match exactly once",
			},
		},
		"required":             []string{"action", "scope"},
		"additionalProperties": false,
	}
}

type memoryInput struct {
	action     string
	scope      memory.Scope
	section    string
	content    string
	oldContent string
}

func parseMemoryInput(input any) (memoryInput, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return memoryInput{}, fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	str := func(key string) (string, error) {
		v, exists := params[key]
		if !exists || v == nil {
			return "", nil
		}
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("invalid input: %s must be a string", key)
		}
		return strings.TrimSpace(s), nil
	}
	var in memoryInput
	var err error
	if in.action, err = str("action"); err != nil {
		return in, err
	}
	scope, err := str("scope")
	if err != nil {
		return in, err
	}
	if in.section, err = str("section"); err != nil {
		return in, err
	}
	if in.content, err = str("content"); err != nil {
		return in, err
	}
	if in.oldContent, err = str("old_content"); err != nil {
		return in, err
	}

	const example = `{"action":"add","scope":"project","section":"Commands","content":"- Run tests with go test -race ./..."}`
	switch scope {
	case "global":
		in.scope = memory.ScopeGlobal
	case "project":
		in.scope = memory.ScopeProject
	case "":
		return in, missingRequiredParameter(MemoryToolName, "scope", example, `scope is "global" or "project"`)
	default:
		return in, fmt.Errorf(`invalid input: scope must be "global" or "project", got %q`, scope)
	}

	switch in.action {
	case "add":
		if in.content == "" {
			return in, missingRequiredParameter(MemoryToolName, "content", example, "")
		}
	case "update":
		if in.oldContent == "" {
			return in, missingRequiredParameter(MemoryToolName, "old_content", `{"action":"update","scope":"project","old_content":"<exact existing entry>","content":"<new entry>"}`, "")
		}
		if in.content == "" {
			return in, fmt.Errorf("invalid input: update requires content; use action \"remove\" to delete an entry")
		}
	case "remove":
		if in.oldContent == "" {
			return in, missingRequiredParameter(MemoryToolName, "old_content", `{"action":"remove","scope":"project","old_content":"<exact existing entry>"}`, "")
		}
	case "":
		return in, missingRequiredParameter(MemoryToolName, "action", example, `action is "add", "update", or "remove"`)
	default:
		return in, fmt.Errorf(`invalid input: action must be "add", "update", or "remove", got %q`, in.action)
	}
	return in, nil
}

func (t *MemoryTool) ValidateInput(_ context.Context, input any) error {
	_, err := parseMemoryInput(input)
	return err
}

func (t *MemoryTool) Execute(ctx context.Context, input any) (any, error) {
	in, err := parseMemoryInput(input)
	if err != nil {
		return nil, err
	}

	path := memory.ProjectPath(t.workingDir)
	if in.scope == memory.ScopeGlobal {
		path = memory.GlobalPath()
	}
	if path == "" {
		return nil, fmt.Errorf("%s memory path is unavailable", in.scope)
	}
	if path, err = t.guard.ResolvePath(path); err != nil {
		return nil, fmt.Errorf("path resolution failed: %w", err)
	}
	permission := t.guard.CheckPath(path, "write")
	if permission == filesystem.PermissionDenied {
		return nil, fmt.Errorf("permission denied by policy: %s memory is blocked", lowerScope(in.scope))
	}

	if memory.ContainsSecret(in.content) {
		return nil, errMemorySecret
	}

	oldContent := ""
	if data, err := os.ReadFile(path); err == nil {
		oldContent = string(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to read %s memory: %w", in.scope, err)
	}

	var newContent string
	switch in.action {
	case "add":
		newContent, err = memory.Add(oldContent, in.section, in.content)
	case "update":
		newContent, err = memory.Replace(oldContent, in.oldContent, in.content)
	case "remove":
		newContent, err = memory.Replace(oldContent, in.oldContent, "")
	}
	if err != nil {
		return nil, err
	}
	// Catch secrets assembled from existing text plus this edit; a file that
	// already holds one (hand-edited) is left to the user so trims still work.
	if !memory.ContainsSecret(oldContent) && memory.ContainsSecret(newContent) {
		return nil, errMemorySecret
	}

	maxBytes := memory.CapFor(in.scope)
	// Shrinking edits stay allowed above the cap so an over-cap (hand-edited) file can be trimmed step by step.
	if len(newContent) > maxBytes && len(newContent) >= len(oldContent) {
		return nil, fmt.Errorf("%s memory would be %s of %s; merge, trim, or remove older or irrelevant entries first",
			lowerScope(in.scope), memory.FormatSize(len(newContent)), memory.FormatSize(maxBytes))
	}

	if t.diffEmitter != nil {
		t.diffEmitter.EmitDiff(computeEditDiff(oldContent, newContent))
	}

	if permission == filesystem.PermissionPending {
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required but not available")
		}
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), path, path, false)
		if err != nil {
			return nil, fmt.Errorf("permission request failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("permission denied by user: %s memory was not changed", lowerScope(in.scope))
		}
	}

	if _, err := writeFileContent(path, newContent); err != nil {
		return nil, err
	}

	message := fmt.Sprintf("%s memory saved (%s of %s).", in.scope, memory.FormatSize(len(newContent)), memory.FormatSize(maxBytes))
	if len(newContent) > maxBytes {
		message += " It is still over its cap: keep merging, trimming, or removing entries until it fits."
	} else if float64(len(newContent)) >= memoryNearCapRatio*float64(maxBytes) {
		message += " It is near its cap: merge related entries and remove outdated or irrelevant ones."
	}
	return map[string]any{
		"action":  in.action,
		"scope":   lowerScope(in.scope),
		"bytes":   len(newContent),
		"cap":     maxBytes,
		"message": message,
	}, nil
}

func lowerScope(scope memory.Scope) string {
	if scope == memory.ScopeGlobal {
		return "global"
	}
	return "project"
}

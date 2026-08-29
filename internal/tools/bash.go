package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mochow13/keen-code/internal/filesystem"
)

const (
	bashTimeout             = 300 * time.Second
	maxInlineOutputSize     = 64 * 1024
	outputPreviewHeadSize   = 32 * 1024
	outputPreviewTailSize   = 16 * 1024
	truncatedOutputFileMode = 0600
)

type BashTool struct {
	guard               *filesystem.Guard
	permissionRequester PermissionRequester
}

func NewBashTool(guard *filesystem.Guard, permissionRequester PermissionRequester) *BashTool {
	return &BashTool{
		guard:               guard,
		permissionRequester: permissionRequester,
	}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Description() string {
	return `Execute bash commands in the terminal. Use this through the tool API whenever you say you will run, execute, test, build, install, or invoke a shell command. Do not merely describe command execution in assistant text.

This is a fallback tool — prefer
dedicated tools when they exist:
- read_file for reading files
- write_file for creating files
- edit_file for modifying files
- glob for finding files by name
- grep for searching file contents

Do not use bash for reading files, file search, or content search when read_file,
glob, or grep can do it. For read-only investigation, prefer dedicated tools
because their output is structured for later reasoning.

Use this for: running tests, installing dependencies, git operations, build commands,
and other shell-native tasks not covered by dedicated tools.

IMPORTANT:
- Set isDangerous=true for any potentially dangerous commands. This will always prompt for user permission.
- Examples of dangerous commands:
  - removing files or directories
  - git operations that modify the repository like commit, push, reset, rebase, etc.
  - killing processes
  - modifying system files
  - accessing environment variables
- Examples of non-dangerous commands:
  - adding to git stage (git add)
  - linting code
  - running tests
  - building the project
  - running the project
  - installing dependencies
- Commands time out after 300 seconds.
- Quote paths that may contain spaces.
- Prefer single commands over long chains. For independent commands, use parallel
  tool calls instead of chaining with &&.

Large output handling:
- Large stdout is truncated in the tool result and saved to stdout_file.
- Stderr is returned only when the command exits non-zero; large captured stderr may be saved to stderr_file.
- If truncated is true, do not rerun the same broad command just to see more output.
- Inspect any saved stdout_file/stderr_file with read_file offset/limit or grep for targeted follow-up.
- Prefer narrowing the original command or searching the captured file before reading many chunks.`
}

func (t *BashTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The bash command to execute. Quote paths with spaces. Prefer simple single commands over long chains.",
			},
			"isDangerous": map[string]any{
				"type":        "boolean",
				"description": "Set to true if the command may modify files or system state (e.g., rm, mv, git commit, package install). This will always prompt for user permission.",
			},
			"summary": map[string]any{
				"type":        "string",
				"description": "A brief 5-10 word description of what the command does",
			},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	}
}

func (t *BashTool) ValidateInput(_ context.Context, input any) error {
	params, ok := input.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	command, ok := params["command"].(string)
	if !ok || command == "" {
		if _, exists := params["command"]; !exists {
			return missingRequiredParameter("bash", "command", `{"command":"<shell command>"}`, "Provide the exact command to execute; include isDangerous when it may modify files or system state")
		}
		return fmt.Errorf("invalid input: command must be a non-empty string")
	}
	return nil
}

func (t *BashTool) Execute(ctx context.Context, input any) (any, error) {
	params := input.(map[string]any)
	command := params["command"].(string)

	isDangerous := false
	if isDangerousValue, exists := params["isDangerous"]; exists {
		if isDangerousBool, ok := isDangerousValue.(bool); ok {
			isDangerous = isDangerousBool
		}
	}

	summary := ""
	if summaryValue, exists := params["summary"]; exists {
		if summaryStr, ok := summaryValue.(string); ok {
			summary = summaryStr
		}
	}

	permission := t.guard.CheckPath(".", "read")

	switch permission {
	case filesystem.PermissionDenied:
		return nil, fmt.Errorf("permission denied by policy")
	case filesystem.PermissionPending:
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required but not available")
		}
		resolvedPath, _ := t.guard.ResolvePath(".")
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), ".", resolvedPath, false)
		if err != nil {
			return nil, fmt.Errorf("permission request failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("permission denied by user: bash execution rejected")
		}
	}

	if isDangerous || IsDangerousCommand(command) {
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required for dangerous command but not available")
		}
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), command, "", true)
		if err != nil {
			return nil, fmt.Errorf("permission request failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("permission denied by user: dangerous command execution rejected")
		}
	}

	return t.executeCommand(ctx, command, summary)
}

func (t *BashTool) executeCommand(ctx context.Context, command, summary string) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	stdout, err := cmd.Output()

	exitCode := 0
	var stderr []byte

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("command timed out after %v", bashTimeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderr = exitErr.Stderr
		} else {
			return nil, fmt.Errorf("command execution failed: %w", err)
		}
	}

	stdoutResult, err := summarizeCommandOutput("stdout", stdout)
	if err != nil {
		return nil, err
	}
	stderrResult, err := summarizeCommandOutput("stderr", stderr)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"exit_code": exitCode,
		"stdout":    stdoutResult.content,
		"truncated": stdoutResult.truncated || stderrResult.truncated,
	}
	if stderrResult.content != "" {
		result["stderr"] = stderrResult.content
	}
	if stdoutResult.file != "" {
		result["stdout_file"] = stdoutResult.file
	}
	if stderrResult.file != "" {
		result["stderr_file"] = stderrResult.file
	}

	return result, nil
}

type commandOutputSummary struct {
	content   string
	file      string
	truncated bool
}

func summarizeCommandOutput(stream string, data []byte) (commandOutputSummary, error) {
	if len(data) <= maxInlineOutputSize {
		return commandOutputSummary{content: string(data)}, nil
	}

	file, err := writeCommandOutputFile(stream, data)
	if err != nil {
		return commandOutputSummary{}, err
	}

	headSize := min(outputPreviewHeadSize, len(data))
	tailSize := min(outputPreviewTailSize, len(data)-headSize)
	tailStart := len(data) - tailSize
	omitted := len(data) - headSize - tailSize

	content := fmt.Sprintf(
		"%s\n\n... (%d bytes omitted; full %s saved to %s) ...\n\n%s",
		string(data[:headSize]),
		omitted,
		stream,
		file,
		string(data[tailStart:]),
	)

	return commandOutputSummary{
		content:   content,
		file:      file,
		truncated: true,
	}, nil
}

func writeCommandOutputFile(stream string, data []byte) (string, error) {
	dir, err := filesystem.KeenBashOutputDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve bash output directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create bash output directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to secure bash output directory %q: %w", dir, err)
	}

	file, err := os.CreateTemp(dir, "keen-bash-*"+"."+stream)
	if err != nil {
		return "", fmt.Errorf("failed to create %s output file: %w", stream, err)
	}
	path := file.Name()
	defer file.Close()

	if err := file.Chmod(truncatedOutputFileMode); err != nil {
		return "", fmt.Errorf("failed to secure %s output file %q: %w", stream, path, err)
	}
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("failed to write %s output file %q: %w", stream, path, err)
	}
	return path, nil
}

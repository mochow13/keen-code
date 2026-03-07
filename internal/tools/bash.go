package tools

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/user/keen-cli/internal/filesystem"
)

const (
	bashTimeout   = 60 * time.Second
	maxOutputSize = 10 * 1024 * 1024 // 10MB
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
	return `Execute bash commands in the terminal. Use isDangerous=true for commands
		that modify files or system state. Do not use this tool for writing or editing files.`
}

func (t *BashTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The bash command to execute",
			},
			"isDangerous": map[string]any{
				"type":        "boolean",
				"description": "Set to true if the command may modify files or system state. This will always prompt for user permission.",
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

func (t *BashTool) Execute(ctx context.Context, input any) (any, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}

	commandValue, ok := params["command"]
	if !ok {
		return nil, fmt.Errorf("invalid input: missing required 'command' parameter")
	}

	command, ok := commandValue.(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("invalid input: command must be a non-empty string")
	}

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
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), ".", resolvedPath, "execute", false)
		if err != nil {
			return nil, fmt.Errorf("permission request failed: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("permission denied by user: bash execution rejected")
		}
	}

	if isDangerous {
		if t.permissionRequester == nil {
			return nil, fmt.Errorf("permission denied: user approval required for dangerous command but not available")
		}
		allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), command, "", "execute", true)
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

	stdoutStr := string(stdout)
	stderrStr := string(stderr)

	if len(stdout) > maxOutputSize {
		stdoutStr = stdoutStr[:maxOutputSize] + "\n... (output truncated)"
	}
	if len(stderr) > maxOutputSize {
		stderrStr = stderrStr[:maxOutputSize] + "\n... (stderr truncated)"
	}

	result := map[string]any{
		"command":   command,
		"exit_code": exitCode,
		"stdout":    stdoutStr,
		"stderr":    stderrStr,
	}

	if summary != "" {
		result["summary"] = summary
	}

	return result, nil
}

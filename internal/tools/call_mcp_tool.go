package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mochow13/keen-code/internal/filesystem"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxInlineMCPResultSize   = 64 * 1024
	mcpResultPreviewHeadSize = 4 * 1024
	mcpResultPreviewTailSize = 2 * 1024
	mcpArtifactFileMode      = 0600
)

type CallMCPTool struct {
	manager             keenmcp.Runtime
	permissionRequester PermissionRequester
}

func NewCallMCPTool(manager keenmcp.Runtime, permissionRequester PermissionRequester) *CallMCPTool {
	return &CallMCPTool{
		manager:             manager,
		permissionRequester: permissionRequester,
	}
}

func (t *CallMCPTool) Name() string {
	return "call_mcp_tool"
}

func (t *CallMCPTool) Description() string {
	return `Call a tool on a connected MCP (Model Context Protocol) server.

Use this through the tool API whenever you say you will call, query, use, or check an MCP server or MCP tool. Do not merely describe MCP tool usage in assistant text.

Before calling, you must read the server's skill file to discover available tools, then you must read
the tool's schema file to understand the required arguments:
- Skill file:   ~/.keen/skills/mcp:<server>/SKILL.md
- Schema file:  ~/.keen/skills/mcp:<server>/schemas/<tool>.json

IMPORTANT:
- Use the bare configured server name, for example "context7", not the skill name
  "mcp:context7" and not a combined path like "mcp:context7/resolve-library-id".
- Use the exact MCP tool name as it appears in the skill's "Available tools"
  table, for example "resolve-library-id". Do not guess, abbreviate, or
  transform names (e.g. swap "-" for "_"). If a call fails with "tool not
  found", re-read the skill file and use a name from that table.
- Arguments must match the tool's input schema exactly.
- Set checkCache to false or omit it (reserved for future use).
- If the result is very large, the full output is saved to a file and the response includes
  truncated: true, artifact_path, and a preview in content. Use read_file with offset/limit or
  grep with path set to artifact_path to inspect the saved result incrementally.`
}

func (t *CallMCPTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "The bare MCP server name as configured, for example context7",
			},
			"tool": map[string]any{
				"type":        "string",
				"description": "The exact MCP tool name to call on the server, for example resolve-library-id",
			},
			"arguments": map[string]any{
				"type":        "object",
				"description": "Key-value arguments matching the tool's input schema",
			},
			"checkCache": map[string]any{
				"type":        "boolean",
				"description": "Reserved for future caching; set to false or omit",
			},
		},
		"required":             []string{"server", "tool"},
		"additionalProperties": false,
	}
}

func (t *CallMCPTool) ValidateInput(ctx context.Context, input any) error {
	params, ok := input.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	server, err := requiredString(params, "server")
	if err != nil {
		if _, exists := params["server"]; !exists {
			return missingRequiredParameter("call_mcp_tool", "server", `{"server":"<configured server name>","tool":"<exact tool name>"}`, "Read the server skill and tool schema before retrying; arguments must match that schema")
		}
		return err
	}
	tool, err := requiredString(params, "tool")
	if err != nil {
		if _, exists := params["tool"]; !exists {
			return missingRequiredParameter("call_mcp_tool", "tool", `{"server":"<configured server name>","tool":"<exact tool name>"}`, "Read the server skill and tool schema before retrying; arguments must match that schema")
		}
		return err
	}
	server, tool, err = normalizeMCPCallTarget(server, tool)
	if err != nil {
		return err
	}
	var arguments map[string]any
	if raw, exists := params["arguments"]; exists && raw != nil {
		arguments, _ = raw.(map[string]any)
	}
	return t.validateRequiredArguments(ctx, server, tool, arguments)
}

func (t *CallMCPTool) Execute(ctx context.Context, input any) (any, error) {
	params := input.(map[string]any)
	server := params["server"].(string)
	tool := params["tool"].(string)
	server, tool, _ = normalizeMCPCallTarget(server, tool)

	var arguments map[string]any
	if raw, exists := params["arguments"]; exists && raw != nil {
		arguments, _ = raw.(map[string]any)
	}

	_ = params["checkCache"] // reserved, no-op

	argsJSON := ""
	if len(arguments) > 0 {
		data, jsonErr := json.MarshalIndent(arguments, "", "  ")
		if jsonErr == nil {
			argsJSON = string(data)
		}
	}

	if t.permissionRequester == nil {
		return nil, fmt.Errorf("permission denied: user approval required but not available")
	}
	allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), server+"/"+tool, argsJSON, false)
	if err != nil {
		return nil, fmt.Errorf("permission request failed: %w", err)
	}
	if !allowed {
		return nil, fmt.Errorf("permission denied by user: call_mcp_tool rejected for %s/%s", server, tool)
	}

	result, err := t.manager.CallTool(ctx, server, tool, arguments)
	if err != nil {
		if result != nil && len(result.Content) > 0 {
			content := formatMCPContent(result.Content)
			if content != "" {
				return nil, fmt.Errorf("%w\n%s", err, content)
			}
		}
		return nil, err
	}

	content := formatMCPContent(result.Content)
	output := make(map[string]any)

	if len(content) <= maxInlineMCPResultSize {
		output["content"] = content
		return output, nil
	}

	summary, err := summarizeMCPResult(content)
	if err != nil {
		return nil, fmt.Errorf("failed to write MCP result artifact: %w", err)
	}

	output["content"] = summary.preview
	output["truncated"] = true
	output["artifact_path"] = summary.path
	return output, nil
}

func requiredString(params map[string]any, name string) (string, error) {
	v, ok := params[name]
	if !ok {
		return "", fmt.Errorf("invalid input: missing required %q parameter", name)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("invalid input: %q must be a non-empty string", name)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("invalid input: %q must be a non-empty string", name)
	}
	return s, nil
}

func normalizeMCPCallTarget(server, tool string) (string, string, error) {
	const skillPrefix = "mcp:"

	server = strings.TrimSpace(server)
	tool = strings.TrimSpace(tool)
	server = strings.TrimPrefix(server, skillPrefix)
	if strings.Contains(server, "/") {
		return "", "", fmt.Errorf("invalid input: server must be a bare MCP server name, got %q", server)
	}

	if strings.HasPrefix(tool, skillPrefix+server+"/") {
		tool = strings.TrimPrefix(tool, skillPrefix+server+"/")
	} else if strings.HasPrefix(tool, server+"/") {
		tool = strings.TrimPrefix(tool, server+"/")
	}
	tool = strings.TrimSpace(tool)

	if server == "" {
		return "", "", fmt.Errorf("invalid input: %q must be a non-empty string", "server")
	}
	if tool == "" {
		return "", "", fmt.Errorf("invalid input: %q must be a non-empty string", "tool")
	}
	return server, tool, nil
}

func formatMCPContent(content []mcpsdk.Content) string {
	parts := make([]string, 0, len(content))
	for _, item := range content {
		switch c := item.(type) {
		case *mcpsdk.TextContent:
			if c.Text != "" {
				parts = append(parts, c.Text)
			}
		default:
			data, err := json.Marshal(item)
			if err == nil {
				parts = append(parts, string(data))
			}
		}
	}
	return strings.Join(parts, "\n")
}

type mcpResultSummary struct {
	preview string
	path    string
}

func summarizeMCPResult(content string) (mcpResultSummary, error) {
	data := []byte(content)
	path, err := writeMCPArtifact(data)
	if err != nil {
		return mcpResultSummary{}, err
	}

	headSize := min(mcpResultPreviewHeadSize, len(data))
	tailSize := min(mcpResultPreviewTailSize, len(data)-headSize)
	tailStart := len(data) - tailSize
	omitted := len(data) - headSize - tailSize

	preview := fmt.Sprintf(
		"%s\n\n... (%d bytes omitted; full result saved to artifact_path) ...\n\n%s",
		string(data[:headSize]),
		omitted,
		string(data[tailStart:]),
	)

	return mcpResultSummary{
		preview: preview,
		path:    path,
	}, nil
}

func writeMCPArtifact(data []byte) (string, error) {
	dir, err := filesystem.KeenMCPArtifactsDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve MCP artifacts directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create MCP artifacts directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to secure MCP artifacts directory %q: %w", dir, err)
	}

	file, err := os.CreateTemp(dir, "keen-mcp-*"+".txt")
	if err != nil {
		return "", fmt.Errorf("failed to create MCP artifact file: %w", err)
	}
	path := file.Name()
	defer file.Close()

	if err := file.Chmod(mcpArtifactFileMode); err != nil {
		return "", fmt.Errorf("failed to secure MCP artifact file %q: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("failed to write MCP artifact file %q: %w", path, err)
	}
	return path, nil
}

func (t *CallMCPTool) validateRequiredArguments(ctx context.Context, server, tool string, arguments map[string]any) error {
	tools, err := t.manager.ListTools(ctx, server)
	if err != nil {
		return err
	}
	for _, candidate := range tools {
		if candidate.Name != tool {
			continue
		}
		missing := missingRequiredArguments(candidate.InputSchema, arguments)
		if len(missing) == 0 {
			return nil
		}
		return fmt.Errorf(`invalid input: arguments missing required fields for %s/%s: %s. 
			Read schema file: ~/.keen/skills/mcp:%s/schemas/%s.json`, server, tool, strings.Join(missing, ", "), server, tool)
	}
	if len(tools) > 0 {
		return toolNotFoundError(server, tool)
	}
	return nil
}

func toolNotFoundError(server, requested string) error {
	return fmt.Errorf(`invalid input: tool %q not found on MCP server %q.
		Re-read ~/.keen/skills/mcp:%s/SKILL.md and use an exact name from its Available tools table`, requested, server, server)
}

func missingRequiredArguments(schema any, arguments map[string]any) []string {
	required := requiredFields(schema)
	if len(required) == 0 {
		return nil
	}

	missing := make([]string, 0, len(required))
	for _, field := range required {
		if _, ok := arguments[field]; !ok {
			missing = append(missing, field)
		}
	}
	return missing
}

func requiredFields(schema any) []string {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var decoded struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil
	}
	return decoded.Required
}

package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/mochow13/keen-code/internal/filesystem"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxInlineMCPResultSize       = 64 * 1024
	maxMemoryCachedMCPResultSize = 16 * 1024
	mcpResultPreviewHeadSize     = 4 * 1024
	mcpResultPreviewTailSize     = 2 * 1024
	mcpArtifactFileMode          = 0600
	mcpCacheLRUCapacity          = 512
	mcpCacheFilePrefix           = "keen-mcp-"
)

type CallMCPTool struct {
	manager             keenmcp.Runtime
	permissionRequester PermissionRequester
	cache               *lru.Cache[string, mcpCacheEntry]
}

func NewCallMCPTool(manager keenmcp.Runtime, permissionRequester PermissionRequester) *CallMCPTool {
	cache, _ := lru.New[string, mcpCacheEntry](mcpCacheLRUCapacity)
	return &CallMCPTool{
		manager:             manager,
		permissionRequester: permissionRequester,
		cache:               cache,
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
				"description": "Set true when the same call was made recently and the result is not expected to have changed; reuses the cached result. Set false or omit to call the server and refresh the cache",
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
	if raw, exists := params["checkCache"]; exists && raw != nil {
		if _, ok := raw.(bool); !ok {
			return fmt.Errorf("invalid input: %q must be a boolean", "checkCache")
		}
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

	checkCache, _ := params["checkCache"].(bool)

	key := mcpCacheKey(server, tool, arguments)
	if checkCache {
		if cached, ok := t.lookupCache(key); ok {
			return cached, nil
		}
	}

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

	if len(content) <= maxInlineMCPResultSize {
		_, _ = t.storeCache(key, content)
		return map[string]any{"content": content}, nil
	}

	path, err := t.storeCache(key, content)
	if err != nil {
		return nil, fmt.Errorf("failed to write MCP result artifact: %w", err)
	}

	return map[string]any{
		"content":       buildMCPPreview([]byte(content)),
		"truncated":     true,
		"artifact_path": path,
	}, nil
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

type mcpCacheEntry struct {
	content  string
	cachedAt time.Time
}

func mcpCacheKey(server, tool string, arguments map[string]any) string {
	argsJSON, err := json.Marshal(arguments)
	if err != nil {
		argsJSON = []byte(fmt.Sprintf("%v", arguments))
	}
	sum := sha256.Sum256([]byte(server + "\x00" + tool + "\x00" + string(argsJSON)))
	// first 16 bytes keep 128-bit collision resistance while keeping filenames short
	return hex.EncodeToString(sum[:16])
}

func mcpCachePath(key string) (string, error) {
	dir, err := filesystem.KeenMCPArtifactsDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve MCP artifacts directory: %w", err)
	}
	return filepath.Join(dir, mcpCacheFilePrefix+key+".txt"), nil
}

func (t *CallMCPTool) lookupCache(key string) (map[string]any, bool) {
	if cache := t.cache; cache != nil {
		if entry, ok := cache.Get(key); ok {
			return map[string]any{
				"content":   entry.content,
				"cached_at": entry.cachedAt.UTC().Format(time.RFC3339),
			}, true
		}
	}

	path, err := mcpCachePath(key)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}

	output := map[string]any{
		"content":   string(data),
		"cached_at": info.ModTime().UTC().Format(time.RFC3339),
	}
	if len(data) > maxInlineMCPResultSize {
		output["content"] = buildMCPPreview(data)
		output["truncated"] = true
		output["artifact_path"] = path
	}
	return output, true
}

func (t *CallMCPTool) storeCache(key, content string) (string, error) {
	if len(content) <= maxMemoryCachedMCPResultSize {
		if cache := t.cache; cache != nil {
			cache.Add(key, mcpCacheEntry{content: content, cachedAt: time.Now()})
		}
		return "", nil
	}

	path, err := mcpCachePath(key)
	if err != nil {
		return "", err
	}
	if err := writeMCPCacheFile(path, []byte(content)); err != nil {
		return "", err
	}
	return path, nil
}

func buildMCPPreview(data []byte) string {
	headSize := min(mcpResultPreviewHeadSize, len(data))
	tailSize := min(mcpResultPreviewTailSize, len(data)-headSize)
	tailStart := len(data) - tailSize
	omitted := len(data) - headSize - tailSize

	return fmt.Sprintf(
		"%s\n\n... (%d bytes omitted; full result saved to artifact_path) ...\n\n%s",
		string(data[:headSize]),
		omitted,
		string(data[tailStart:]),
	)
}

func writeMCPCacheFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create MCP artifacts directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("failed to secure MCP artifacts directory %q: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, ".keen-mcp-cache-*")
	if err != nil {
		return fmt.Errorf("failed to create MCP cache file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(mcpArtifactFileMode); err != nil {
		temp.Close()
		return fmt.Errorf("failed to secure MCP cache file %q: %w", tempPath, err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("failed to write MCP cache file %q: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to write MCP cache file %q: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to store MCP cache file %q: %w", path, err)
	}
	return nil
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

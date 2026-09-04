package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type mockMCPRuntime struct {
	callToolFn  func(ctx context.Context, server, tool string, arguments map[string]any) (*keenmcp.ToolResult, error)
	listToolsFn func(ctx context.Context, server string) ([]keenmcp.Tool, error)
}

func (m *mockMCPRuntime) Start(ctx context.Context) error { return nil }
func (m *mockMCPRuntime) Close() error                    { return nil }
func (m *mockMCPRuntime) Servers() []keenmcp.ServerStatus { return nil }
func (m *mockMCPRuntime) Status(string) keenmcp.ServerStatus {
	return keenmcp.ServerStatus{}
}
func (m *mockMCPRuntime) WaitInitialScan(ctx context.Context) error { return nil }
func (m *mockMCPRuntime) ListTools(ctx context.Context, server string) ([]keenmcp.Tool, error) {
	if m.listToolsFn != nil {
		return m.listToolsFn(ctx, server)
	}
	return nil, nil
}
func (m *mockMCPRuntime) Refresh(ctx context.Context, server string, opts ...keenmcp.RefreshOption) error {
	return nil
}
func (m *mockMCPRuntime) CallTool(ctx context.Context, server, tool string, arguments map[string]any) (*keenmcp.ToolResult, error) {
	if m.callToolFn != nil {
		return m.callToolFn(ctx, server, tool, arguments)
	}
	return &keenmcp.ToolResult{}, nil
}

func TestCallMCPTool_Name(t *testing.T) {
	tool := NewCallMCPTool(&mockMCPRuntime{}, &mockPermissionRequester{allow: true})
	if tool.Name() != "call_mcp_tool" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "call_mcp_tool")
	}
}

func TestCallMCPTool_ExecuteSuccess(t *testing.T) {
	runtime := &mockMCPRuntime{
		callToolFn: func(_ context.Context, server, tool string, _ map[string]any) (*keenmcp.ToolResult, error) {
			if server != "github" || tool != "list_issues" {
				t.Errorf("unexpected call: server=%s tool=%s", server, tool)
			}
			return &keenmcp.ToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: "issue #1"},
				},
			}, nil
		},
	}
	callTool := NewCallMCPTool(runtime, &mockPermissionRequester{allow: true})

	result, err := callTool.Execute(context.Background(), map[string]any{
		"server": "github",
		"tool":   "list_issues",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Execute() result type = %T, want map[string]any", result)
	}
	if m["content"] != "issue #1" {
		t.Errorf("content = %q, want %q", m["content"], "issue #1")
	}
}

func TestCallMCPTool_RejectsMissingRequiredArguments(t *testing.T) {
	runtime := &mockMCPRuntime{
		listToolsFn: func(_ context.Context, server string) ([]keenmcp.Tool, error) {
			if server != "context7" {
				t.Fatalf("unexpected list tools server=%q", server)
			}
			return []keenmcp.Tool{
				{
					Name: "resolve-library-id",
					InputSchema: map[string]any{
						"type":     "object",
						"required": []string{"libraryName"},
						"properties": map[string]any{
							"libraryName": map[string]any{"type": "string"},
						},
					},
				},
			}, nil
		},
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			t.Fatal("CallTool should not be called with missing required arguments")
			return nil, nil
		},
	}
	permissions := &mockPermissionRequester{allow: true}
	callTool := NewCallMCPTool(runtime, permissions)

	err := callTool.ValidateInput(context.Background(), map[string]any{
		"server":    "context7",
		"tool":      "resolve-library-id",
		"arguments": map[string]any{},
	})
	if err == nil {
		t.Fatal("ValidateInput() expected error for missing required arguments")
	}
	if !strings.Contains(err.Error(), "libraryName") {
		t.Fatalf("ValidateInput() error = %v, want missing field name", err)
	}
	if !strings.Contains(err.Error(), "~/.keen/skills/mcp:context7/schemas/resolve-library-id.json") {
		t.Fatalf("ValidateInput() error = %v, want schema path", err)
	}
	if permissions.called {
		t.Fatal("permission should not be requested for invalid MCP arguments")
	}
}

func TestCallMCPTool_PermissionDenied(t *testing.T) {
	callTool := NewCallMCPTool(&mockMCPRuntime{}, &mockPermissionRequester{allow: false})

	_, err := callTool.Execute(context.Background(), map[string]any{
		"server": "github",
		"tool":   "create_issue",
	})
	if err == nil {
		t.Fatal("Execute() expected error for denied permission")
	}
}

func TestCallMCPTool_PropagatesMCPError(t *testing.T) {
	runtime := &mockMCPRuntime{
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			return nil, errors.New("server disconnected")
		},
	}
	callTool := NewCallMCPTool(runtime, &mockPermissionRequester{allow: true})

	_, err := callTool.Execute(context.Background(), map[string]any{
		"server": "github",
		"tool":   "create_issue",
	})
	if err == nil {
		t.Fatal("Execute() expected error from MCP runtime")
	}
}

func TestCallMCPTool_ValidateInput_MissingServer(t *testing.T) {
	callTool := NewCallMCPTool(&mockMCPRuntime{}, &mockPermissionRequester{allow: true})
	err := callTool.ValidateInput(context.Background(), map[string]any{
		"tool": "create_issue",
	})
	if err == nil {
		t.Fatal("ValidateInput() expected error for missing server")
	}
}

func TestCallMCPTool_ValidateInput_MissingTool(t *testing.T) {
	callTool := NewCallMCPTool(&mockMCPRuntime{}, &mockPermissionRequester{allow: true})
	err := callTool.ValidateInput(context.Background(), map[string]any{
		"server": "github",
	})
	if err == nil {
		t.Fatal("ValidateInput() expected error for missing tool")
	}
}

func TestCallMCPTool_NilPermissionRequester(t *testing.T) {
	callTool := NewCallMCPTool(&mockMCPRuntime{}, nil)
	_, err := callTool.Execute(context.Background(), map[string]any{
		"server": "github",
		"tool":   "list_issues",
	})
	if err == nil {
		t.Fatal("Execute() expected error when permissionRequester is nil")
	}
}

type countingPermissionRequester struct {
	calls int
	allow bool
}

func (m *countingPermissionRequester) RequestPermission(context.Context, string, string, string, bool) (bool, error) {
	m.calls++
	return m.allow, nil
}

func TestCallMCPTool_CheckCacheMissFallsThroughToLiveCall(t *testing.T) {
	liveCalls := 0
	runtime := &mockMCPRuntime{
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			liveCalls++
			return &keenmcp.ToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "fresh"}},
			}, nil
		},
	}
	callTool := NewCallMCPTool(runtime, &mockPermissionRequester{allow: true})

	result, err := callTool.Execute(context.Background(), map[string]any{
		"server":     "github",
		"tool":       "list_issues",
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if liveCalls != 1 {
		t.Fatalf("live calls = %d, want 1", liveCalls)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["content"] != "fresh" {
		t.Errorf("content = %v, want %q", m["content"], "fresh")
	}
	if _, exists := m["cached_at"]; exists {
		t.Errorf("fresh response must not include cached_at")
	}
}

func TestCallMCPTool_MemoryCacheHitSkipsLiveCallAndPrompt(t *testing.T) {
	liveCalls := 0
	runtime := &mockMCPRuntime{
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			liveCalls++
			return &keenmcp.ToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "cached payload"}},
			}, nil
		},
	}
	permissions := &countingPermissionRequester{allow: true}
	callTool := NewCallMCPTool(runtime, permissions)

	if _, err := callTool.Execute(context.Background(), map[string]any{
		"server": "github",
		"tool":   "list_issues",
	}); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	result, err := callTool.Execute(context.Background(), map[string]any{
		"server":     "github",
		"tool":       "list_issues",
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("cached Execute() error = %v", err)
	}
	if liveCalls != 1 {
		t.Errorf("live calls = %d, want 1 (cache hit must not call the server)", liveCalls)
	}
	if permissions.calls != 1 {
		t.Errorf("permission prompts = %d, want 1 (cache hit must not prompt)", permissions.calls)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["content"] != "cached payload" {
		t.Errorf("content = %v, want %q", m["content"], "cached payload")
	}
	cachedAt, ok := m["cached_at"].(string)
	if !ok {
		t.Fatalf("cached_at missing on cached response")
	}
	if _, err := time.Parse(time.RFC3339, cachedAt); err != nil {
		t.Errorf("cached_at = %q is not RFC3339: %v", cachedAt, err)
	}
}

func TestCallMCPTool_DiskCacheHitReturnsFullContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	content := strings.Repeat("m", 20*1024)
	liveCalls := 0
	runtime := &mockMCPRuntime{
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			liveCalls++
			return &keenmcp.ToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: content}},
			}, nil
		},
	}

	first := NewCallMCPTool(runtime, &countingPermissionRequester{allow: true})
	if _, err := first.Execute(context.Background(), map[string]any{
		"server": "github",
		"tool":   "list_issues",
	}); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	second := NewCallMCPTool(runtime, &countingPermissionRequester{allow: true})
	result, err := second.Execute(context.Background(), map[string]any{
		"server":     "github",
		"tool":       "list_issues",
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("cached Execute() error = %v", err)
	}
	if liveCalls != 1 {
		t.Errorf("live calls = %d, want 1 (disk cache hit must not call the server)", liveCalls)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if served, ok := m["content"].(string); !ok || served != content {
		t.Errorf("mid-size cached result should be returned in full")
	}
	if _, exists := m["truncated"]; exists {
		t.Errorf("mid-size cached result must not be truncated")
	}
	if _, ok := m["cached_at"].(string); !ok {
		t.Errorf("cached_at missing on cached response")
	}
}

func TestCallMCPTool_DiskCacheHitLargeResultReturnsPreviewAndPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	large := strings.Repeat("x", maxInlineMCPResultSize+1)
	runtime := &mockMCPRuntime{
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			return &keenmcp.ToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: large}},
			}, nil
		},
	}

	first := NewCallMCPTool(runtime, &countingPermissionRequester{allow: true})
	if _, err := first.Execute(context.Background(), map[string]any{
		"server": "context7",
		"tool":   "get-library-docs",
	}); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	second := NewCallMCPTool(runtime, &countingPermissionRequester{allow: true})
	result, err := second.Execute(context.Background(), map[string]any{
		"server":     "context7",
		"tool":       "get-library-docs",
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("cached Execute() error = %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["truncated"] != true {
		t.Errorf("truncated = %v, want true", m["truncated"])
	}
	preview, ok := m["content"].(string)
	if !ok || !strings.Contains(preview, "bytes omitted") {
		t.Errorf("cached large result must return a preview")
	}
	path, ok := m["artifact_path"].(string)
	if !ok || path == "" {
		t.Fatalf("artifact_path missing on cached large result")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read cache file %q: %v", path, err)
	}
	if string(data) != large {
		t.Errorf("cache file content mismatch")
	}
	if _, ok := m["cached_at"].(string); !ok {
		t.Errorf("cached_at missing on cached response")
	}
}

func TestCallMCPTool_FreshCallRefreshesCache(t *testing.T) {
	responses := []string{"first", "second"}
	liveCalls := 0
	runtime := &mockMCPRuntime{
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			text := responses[liveCalls]
			liveCalls++
			return &keenmcp.ToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: text}},
			}, nil
		},
	}
	callTool := NewCallMCPTool(runtime, &countingPermissionRequester{allow: true})

	for range responses {
		if _, err := callTool.Execute(context.Background(), map[string]any{
			"server": "github",
			"tool":   "list_issues",
		}); err != nil {
			t.Fatalf("fresh Execute() error = %v", err)
		}
	}

	result, err := callTool.Execute(context.Background(), map[string]any{
		"server":     "github",
		"tool":       "list_issues",
		"checkCache": true,
	})
	if err != nil {
		t.Fatalf("cached Execute() error = %v", err)
	}
	if liveCalls != 2 {
		t.Fatalf("live calls = %d, want 2", liveCalls)
	}
	m := result.(map[string]any)
	if m["content"] != "second" {
		t.Errorf("cache was not refreshed, content = %v, want %q", m["content"], "second")
	}
}

func TestCallMCPTool_MemoryCacheLRUEviction(t *testing.T) {
	callTool := NewCallMCPTool(&mockMCPRuntime{}, &countingPermissionRequester{allow: true})

	keys := make([]string, 0, mcpCacheLRUCapacity+1)
	for i := 0; i <= mcpCacheLRUCapacity; i++ {
		key := mcpCacheKey("github", fmt.Sprintf("tool-%d", i), nil)
		if _, err := callTool.storeCache(key, "payload"); err != nil {
			t.Fatalf("storeCache() error = %v", err)
		}
		keys = append(keys, key)
	}

	if _, ok := callTool.cache.Get(keys[0]); ok {
		t.Errorf("oldest entry should have been evicted")
	}
	if _, ok := callTool.cache.Get(keys[len(keys)-1]); !ok {
		t.Errorf("newest entry should remain cached")
	}
}

func TestMCPCacheKeyIsDeterministic(t *testing.T) {
	args := map[string]any{"b": 2, "a": map[string]any{"y": 1, "x": 2}}
	key := mcpCacheKey("github", "list_issues", args)

	if key != mcpCacheKey("github", "list_issues", map[string]any{"a": map[string]any{"x": 2, "y": 1}, "b": 2}) {
		t.Errorf("key must be stable across map iteration order")
	}
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32 hex characters", len(key))
	}
	if key == mcpCacheKey("gitlab", "list_issues", args) {
		t.Errorf("server must affect the key")
	}
	if key == mcpCacheKey("github", "list_prs", args) {
		t.Errorf("tool must affect the key")
	}
	if key == mcpCacheKey("github", "list_issues", map[string]any{"b": 2, "a": map[string]any{"y": 1, "x": 3}}) {
		t.Errorf("argument values must affect the key")
	}
}

func TestCallMCPTool_ValidateInputCheckCacheMustBeBool(t *testing.T) {
	callTool := NewCallMCPTool(&mockMCPRuntime{}, &mockPermissionRequester{allow: true})

	err := callTool.ValidateInput(context.Background(), map[string]any{
		"server":     "github",
		"tool":       "list_issues",
		"checkCache": "yes",
	})
	if err == nil || !strings.Contains(err.Error(), "boolean") {
		t.Fatalf("ValidateInput() error = %v, want boolean type error", err)
	}

	if err := callTool.ValidateInput(context.Background(), map[string]any{
		"server":     "github",
		"tool":       "list_issues",
		"checkCache": true,
	}); err != nil {
		t.Fatalf("ValidateInput() error = %v, want nil", err)
	}
}

func TestCallMCPTool_NotFound(t *testing.T) {
	runtime := &mockMCPRuntime{
		listToolsFn: func(_ context.Context, _ string) ([]keenmcp.Tool, error) {
			return []keenmcp.Tool{
				{Name: "resolve-library-id"},
				{Name: "get-library-docs"},
			}, nil
		},
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			t.Fatal("CallTool should not be called for unknown tool")
			return nil, nil
		},
	}
	callTool := NewCallMCPTool(runtime, &mockPermissionRequester{allow: true})

	err := callTool.ValidateInput(context.Background(), map[string]any{
		"server": "context7",
		"tool":   "resolve_library_id",
	})
	if err == nil {
		t.Fatal("ValidateInput() expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ValidateInput() error = %v, want not found", err)
	}
	if !strings.Contains(err.Error(), "~/.keen/skills/mcp:context7/SKILL.md") {
		t.Fatalf("Execute() error = %v, want skill file path", err)
	}
}

func TestCallMCPTool_LargeResultSpillsToArtifact(t *testing.T) {
	largeContent := strings.Repeat("x", maxInlineMCPResultSize+1)
	runtime := &mockMCPRuntime{
		callToolFn: func(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
			return &keenmcp.ToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: largeContent},
				},
			}, nil
		},
	}
	callTool := NewCallMCPTool(runtime, &mockPermissionRequester{allow: true})

	result, err := callTool.Execute(context.Background(), map[string]any{
		"server": "context7",
		"tool":   "get-library-docs",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}

	if m["truncated"] != true {
		t.Errorf("truncated = %v, want true", m["truncated"])
	}

	artifactPath, ok := m["artifact_path"].(string)
	if !ok || artifactPath == "" {
		t.Fatalf("artifact_path missing or empty")
	}

	base := filepath.Base(artifactPath)
	if !strings.HasPrefix(base, mcpCacheFilePrefix) || !strings.HasSuffix(base, ".txt") || len(base) != len(mcpCacheFilePrefix)+32+len(".txt") {
		t.Errorf("artifact name = %q, want %s<32-char hash>.txt", base, mcpCacheFilePrefix)
	}

	preview, ok := m["content"].(string)
	if !ok || !strings.Contains(preview, "...") {
		t.Errorf("content preview missing omission marker")
	}

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("failed to read artifact %q: %v", artifactPath, err)
	}
	if string(data) != largeContent {
		t.Errorf("artifact content length = %d, want %d", len(data), len(largeContent))
	}

	_ = os.Remove(artifactPath)
}

func TestCallMCPTool_Metadata(t *testing.T) {
	tool := NewCallMCPTool(&mockMCPRuntime{}, &mockPermissionRequester{allow: true})
	if description := tool.Description(); !strings.Contains(description, "Model Context Protocol") || !strings.Contains(description, "SKILL.md") {
		t.Fatalf("Description() missing usage guidance: %q", description)
	}

	schema := tool.InputSchema()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("InputSchema() = %#v", schema)
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "server" || required[1] != "tool" {
		t.Fatalf("required = %#v", schema["required"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 4 {
		t.Fatalf("properties = %#v", schema["properties"])
	}
}

func TestCallMCPTool_ValidateInputTypesAndTargets(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{name: "non-map", input: "invalid", want: "expected map[string]any"},
		{name: "non-string server", input: map[string]any{"server": 42, "tool": "read"}, want: `"server" must be a non-empty string`},
		{name: "empty server", input: map[string]any{"server": "  ", "tool": "read"}, want: `"server" must be a non-empty string`},
		{name: "non-string tool", input: map[string]any{"server": "docs", "tool": 42}, want: `"tool" must be a non-empty string`},
		{name: "empty tool", input: map[string]any{"server": "docs", "tool": "  "}, want: `"tool" must be a non-empty string`},
		{name: "qualified server", input: map[string]any{"server": "docs/read", "tool": "read"}, want: "bare MCP server name"},
	}

	tool := NewCallMCPTool(&mockMCPRuntime{}, &mockPermissionRequester{allow: true})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.ValidateInput(context.Background(), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateInput() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCallMCPTool_NormalizesQualifiedTarget(t *testing.T) {
	tests := []struct {
		name   string
		server string
		tool   string
	}{
		{name: "skill prefix", server: "mcp:docs", tool: "mcp:docs/read"},
		{name: "server prefix", server: "docs", tool: "docs/read"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotServer, gotTool string
			runtime := &mockMCPRuntime{callToolFn: func(_ context.Context, server, tool string, _ map[string]any) (*keenmcp.ToolResult, error) {
				gotServer, gotTool = server, tool
				return &keenmcp.ToolResult{}, nil
			}}
			callTool := NewCallMCPTool(runtime, &mockPermissionRequester{allow: true})
			if err := callTool.ValidateInput(context.Background(), map[string]any{"server": tt.server, "tool": tt.tool}); err != nil {
				t.Fatalf("ValidateInput() error = %v", err)
			}
			if _, err := callTool.Execute(context.Background(), map[string]any{"server": tt.server, "tool": tt.tool}); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if gotServer != "docs" || gotTool != "read" {
				t.Fatalf("target = %q/%q, want docs/read", gotServer, gotTool)
			}
		})
	}
}

func TestCallMCPTool_ValidationListErrorAndEmptyCatalog(t *testing.T) {
	t.Run("list error", func(t *testing.T) {
		callTool := NewCallMCPTool(&mockMCPRuntime{listToolsFn: func(context.Context, string) ([]keenmcp.Tool, error) {
			return nil, errors.New("scan failed")
		}}, &mockPermissionRequester{allow: true})
		err := callTool.ValidateInput(context.Background(), map[string]any{"server": "docs", "tool": "read"})
		if err == nil || !strings.Contains(err.Error(), "scan failed") {
			t.Fatalf("ValidateInput() error = %v", err)
		}
	})

	t.Run("empty catalog permits deferred discovery", func(t *testing.T) {
		callTool := NewCallMCPTool(&mockMCPRuntime{}, &mockPermissionRequester{allow: true})
		if err := callTool.ValidateInput(context.Background(), map[string]any{"server": "docs", "tool": "read"}); err != nil {
			t.Fatalf("ValidateInput() error = %v", err)
		}
	})
}

func TestCallMCPTool_PermissionAndResultErrors(t *testing.T) {
	t.Run("permission error", func(t *testing.T) {
		callTool := NewCallMCPTool(&mockMCPRuntime{}, errorPermissionRequester{})
		_, err := callTool.Execute(context.Background(), map[string]any{"server": "docs", "tool": "read"})
		if err == nil || !strings.Contains(err.Error(), "permission request failed") {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	t.Run("runtime error includes content", func(t *testing.T) {
		runtime := &mockMCPRuntime{callToolFn: func(context.Context, string, string, map[string]any) (*keenmcp.ToolResult, error) {
			return &keenmcp.ToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "remote detail"}}}, errors.New("call failed")
		}}
		callTool := NewCallMCPTool(runtime, &mockPermissionRequester{allow: true})
		_, err := callTool.Execute(context.Background(), map[string]any{"server": "docs", "tool": "read"})
		if err == nil || !strings.Contains(err.Error(), "call failed\nremote detail") {
			t.Fatalf("Execute() error = %v", err)
		}
	})
}

func TestCallMCPTool_RequiredFieldHelpers(t *testing.T) {
	if got := missingRequiredArguments(nil, nil); got != nil {
		t.Fatalf("missingRequiredArguments(nil) = %#v", got)
	}
	if got := requiredFields(make(chan int)); got != nil {
		t.Fatalf("requiredFields(unmarshalable) = %#v", got)
	}
	if got := requiredFields("not an object"); got != nil {
		t.Fatalf("requiredFields(string) = %#v", got)
	}

	schema := map[string]any{"required": []any{"first", "second"}}
	got := missingRequiredArguments(schema, map[string]any{"first": true})
	if len(got) != 1 || got[0] != "second" {
		t.Fatalf("missingRequiredArguments() = %#v", got)
	}
}

type errorPermissionRequester struct{}

func (errorPermissionRequester) RequestPermission(context.Context, string, string, string, bool) (bool, error) {
	return false, errors.New("prompt unavailable")
}

package subagents

import (
	"context"

	"github.com/mochow13/keen-code/internal/filesystem"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/tools"
)

type AutoApprover struct{}

func (AutoApprover) RequestPermission(context.Context, string, string, string, bool) (bool, error) {
	return true, nil
}

type NoopDiffEmitter struct{}

func (NoopDiffEmitter) EmitDiff([]tools.EditDiffLine) {}

type ToolFactory struct {
	Guard      *filesystem.Guard
	MCPRuntime keenmcp.Runtime
}

func (f ToolFactory) Registry(profile Profile, parent *tools.Registry) *tools.Registry {
	registry := tools.NewRegistry()
	if f.Guard == nil {
		return registry
	}
	approver := AutoApprover{}
	available := map[string]tools.Tool{
		tools.ReadFileToolName:  tools.NewReadFileTool(f.Guard, approver),
		tools.GlobToolName:      tools.NewGlobTool(f.Guard, approver),
		tools.GrepToolName:      tools.NewGrepTool(f.Guard, approver),
		tools.WriteFileToolName: tools.NewWriteFileTool(f.Guard, NoopDiffEmitter{}, approver),
		tools.EditFileToolName:  tools.NewEditFileTool(f.Guard, NoopDiffEmitter{}, approver),
		tools.BashToolName:      tools.NewBashTool(f.Guard, approver),
		tools.WebFetchToolName:  tools.NewWebFetchTool(),
	}
	for _, name := range permissionToolNames(profile, registryNames(parent)) {
		if tool, ok := available[name]; ok {
			_ = registry.Register(tool)
		}
	}
	if f.MCPRuntime != nil {
		_ = registry.Register(tools.NewCallMCPTool(f.MCPRuntime, approver))
	}
	return registry
}

func registryNames(registry *tools.Registry) []string {
	if registry == nil {
		return nil
	}
	all := registry.All()
	names := make([]string, 0, len(all))
	for _, tool := range all {
		names = append(names, tool.Name())
	}
	return names
}

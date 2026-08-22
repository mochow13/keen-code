package tooling

import (
	"path/filepath"

	replappstate "github.com/mochow13/keen-code/internal/cli/repl/appstate"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/filesystem"
	"github.com/mochow13/keen-code/internal/llm"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/subagents"
	"github.com/mochow13/keen-code/internal/tools"
)

func SetupToolRegistry(
	workingDir string,
	appState *replappstate.AppState,
	permissionRequester *replpermissions.Requester,
	diffEmitter *DiffEmitter,
	askUserRequester *replaskuser.Requester,
	mcpRuntime keenmcp.Runtime,
	cfg *config.ResolvedConfig,
	globalCfg *config.GlobalConfig,
	activity chan<- subagents.ToolActivity,
) {
	gitAwareness := filesystem.NewGitAwareness()
	_ = gitAwareness.LoadGitignore(filepath.Join(workingDir, ".gitignore"))
	guard := filesystem.NewGuard(workingDir, gitAwareness)

	readFileTool := tools.NewReadFileTool(guard, permissionRequester)
	appState.RegisterTool(readFileTool)

	globTool := tools.NewGlobTool(guard, permissionRequester)
	appState.RegisterTool(globTool)

	grepTool := tools.NewGrepTool(guard, permissionRequester)
	appState.RegisterTool(grepTool)

	writeFileTool := tools.NewWriteFileTool(guard, diffEmitter, permissionRequester)
	appState.RegisterTool(writeFileTool)

	editFileTool := tools.NewEditFileTool(guard, diffEmitter, permissionRequester)
	appState.RegisterTool(editFileTool)

	bashTool := tools.NewBashTool(guard, permissionRequester)
	appState.RegisterTool(bashTool)

	webFetchTool := tools.NewWebFetchTool()
	appState.RegisterTool(webFetchTool)

	if askUserRequester != nil {
		appState.RegisterTool(tools.NewAskUserTool(askUserRequester))
	}
	if mcpRuntime != nil {
		appState.RegisterTool(tools.NewCallMCPTool(mcpRuntime, permissionRequester))
	}

	toolFactory := subagents.ToolFactory{Guard: guard, MCPRuntime: mcpRuntime}
	runner := &subagents.Runner{
		WorkingDir: workingDir,
		Config:     cfg,
		GetProfiles: func() []subagents.Profile {
			return appState.GetSubagents().Profiles
		},
		NewClient:   llm.NewClient,
		GetRegistry: appState.EffectiveToolRegistry,
		NewRegistry: toolFactory.Registry,
		ResolveConfig: func(profile subagents.Profile) (*config.ResolvedConfig, error) {
			return config.ResolveProvider(globalCfg, profile.Provider, profile.Model, profile.ThinkingEffort)
		},
		ProjectContext:   func() string { return llm.ProjectInstructions(workingDir) },
		GetSkillsCatalog: appState.SkillsCatalog,
		Activity:         activity,
	}
	profiles := appState.GetSubagents().Profiles
	agentNames := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if !profile.Hidden {
			agentNames = append(agentNames, profile.Name)
		}
	}
	if len(agentNames) > 0 {
		appState.RegisterTool(tools.NewDelegateTool(runner, agentNames))
	}
}

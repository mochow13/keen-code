package agentcore

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/filesystem"
	"github.com/mochow13/keen-code/internal/llm"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/subagents"
	"github.com/mochow13/keen-code/internal/tools"
)

type PermissionRequester interface {
	RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error)
}

type DiffEmitter interface {
	EmitDiff(lines []EditDiffLine)
}

type AskUserRequester interface {
	RequestUser(ctx context.Context, questionnaire AskUserRequest) (AskUserResult, error)
}

type permissionRequesterAdapter struct {
	inner PermissionRequester
}

func (p *permissionRequesterAdapter) RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error) {
	if p == nil || isNilInterface(p.inner) {
		return false, errors.New("permission approval required but no permission requester is available")
	}
	return p.inner.RequestPermission(ctx, toolName, path, resolvedPath, isDangerous)
}

type diffEmitterAdapter struct {
	inner DiffEmitter
}

func (d *diffEmitterAdapter) EmitDiff(lines []tools.EditDiffLine) {
	if d.inner != nil {
		d.inner.EmitDiff(FromToolDiffLines(lines))
	}
}

type askUserRequesterAdapter struct {
	inner AskUserRequester
}

func (a *askUserRequesterAdapter) RequestUser(ctx context.Context, request tools.AskUserRequest) (tools.AskUserResult, error) {
	result, err := a.inner.RequestUser(ctx, toAskUserRequest(request))
	return fromAskUserResult(result), err
}

func (a *adapter) SetupTools(
	ctx context.Context,
	permissionRequester PermissionRequester,
	diffEmitter DiffEmitter,
	askUserRequester AskUserRequester,
	mcpRuntime keenmcp.Runtime,
	forwardActivity bool,
) <-chan ToolActivity {
	appState := a.appState
	workingDir := appState.WorkingDir()
	cfg := a.Config()
	gitAwareness := filesystem.NewGitAwareness()
	_ = gitAwareness.LoadGitignore(filepath.Join(workingDir, ".gitignore"))
	guard := filesystem.NewGuard(workingDir, gitAwareness)

	permAdapter := &permissionRequesterAdapter{inner: permissionRequester}

	readFileTool := tools.NewReadFileTool(guard, permAdapter)
	appState.RegisterTool(readFileTool)

	globTool := tools.NewGlobTool(guard, permAdapter)
	appState.RegisterTool(globTool)

	grepTool := tools.NewGrepTool(guard, permAdapter)
	appState.RegisterTool(grepTool)

	writeFileTool := tools.NewWriteFileTool(guard, &diffEmitterAdapter{inner: diffEmitter}, permAdapter)
	appState.RegisterTool(writeFileTool)

	editFileTool := tools.NewEditFileTool(guard, &diffEmitterAdapter{inner: diffEmitter}, permAdapter)
	appState.RegisterTool(editFileTool)

	bashTool := tools.NewBashTool(guard, permAdapter)
	appState.RegisterTool(bashTool)

	webFetchTool := tools.NewWebFetchTool()
	appState.RegisterTool(webFetchTool)

	if !isNilInterface(askUserRequester) {
		appState.RegisterTool(tools.NewAskUserTool(&askUserRequesterAdapter{inner: askUserRequester}))
	}
	if mcpRuntime != nil {
		appState.RegisterTool(tools.NewCallMCPTool(mcpRuntime, permAdapter))
	}

	subagentActivity, out := subagentActivityChannels(ctx, forwardActivity)

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
			return config.ResolveProvider(a.GlobalConfig(), profile.Provider, profile.Model, profile.ThinkingEffort)
		},
		ProjectContext:   func() string { return llm.ProjectInstructions(workingDir) },
		GetSkillsCatalog: appState.SkillsCatalog,
		Activity:         subagentActivity,
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

	return out
}

func subagentActivityChannels(ctx context.Context, forward bool) (chan subagents.ToolActivity, <-chan ToolActivity) {
	if !forward {
		return nil, nil
	}
	subagentActivity := make(chan subagents.ToolActivity, 64)
	out := make(chan ToolActivity, 64)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				drainSubagentActivity(subagentActivity)
				return
			case activity, ok := <-subagentActivity:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					drainSubagentActivity(subagentActivity)
					return
				case out <- ConvertToolActivity(activity):
				}
			}
		}
	}()
	return subagentActivity, out
}

func drainSubagentActivity(ch <-chan subagents.ToolActivity) {
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	}
	return false
}

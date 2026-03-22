package repl

import (
	"github.com/user/keen-code/internal/filesystem"
	"github.com/user/keen-code/internal/tools"
)

func setupToolRegistry(
	workingDir string,
	appState *AppState,
	permissionRequester *REPLPermissionRequester,
	diffEmitter *REPLDiffEmitter,
) {
	gitAwareness := filesystem.NewGitAwareness()
	_ = gitAwareness.LoadGitignoreRecursive(workingDir)
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
}

package repl

import (
	"github.com/user/keen-cli/internal/filesystem"
	"github.com/user/keen-cli/internal/tools"
)

func setupToolRegistry(
	workingDir string,
	appState *AppState,
	permissionRequester *REPLPermissionRequester,
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
}

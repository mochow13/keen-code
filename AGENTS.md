## Keen Code
CLI-based coding agent powered by AI using Firebase Genkit for LLM interactions.

## Architecture
- **internal/tools** - LLM tools (read_file, glob)
- **internal/filesystem** - Guard for safe file access
- **internal/cli/repl** - Interactive REPL UI
- **internal/llm** - Genkit-based LLM client

## Adding a Tool
1. Create `internal/tools/{name}.go` implementing `tools.Tool` interface
2. Inject dependencies (guard, permissionRequester) via constructor
3. Register in `internal/cli/repl/repl.go` in `initialModel()`
4. Add tests in `internal/tools/{name}_test.go`

Check existing tools in @internal/tools/ for reference.

## Permission System
Guard checks paths before filesystem operations:
- `PermissionGranted` - Allowed (working directory)
- `PermissionPending` - User approval required (outside working dir)
- `PermissionDenied` - Blocked (system paths, .gitignore files)

## Important Guidelines
- **Minimal comments** - Only when strictly necessary
- **Test critical paths** - Not aiming for 100% coverage
- **Inject dependencies** - Use constructors
- **Guard checks first** - Always validate before filesystem ops
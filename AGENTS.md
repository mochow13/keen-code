## Keen Code
CLI-based coding agent powered by AI using Firebase Genkit for LLM interactions.

## Important Guidelines
- Minimal comments only when strictly necessary
- Test critical paths, not aiming for 100% coverage
- Always run `go test -race ./...` after finalising a change
- Always run `go mod tidy` after each change
- Always run `gofmt` on modified Go files before committing
- Commit messages should be concise and focus on the key changes with bullet points
- Commit messages should follow the `feat(category): description` format
- Always check both tracked and untracked files for creating the commit message
- Never add co-authors or made-with AI tags to the commit message

## Architecture
- **internal/tools** - LLM tools (read_file, write_file, edit_file, glob, grep, bash)
- **internal/filesystem** - Guard for safe file access
- **internal/cli/repl** - Interactive REPL UI
- **internal/llm** - Genkit-based LLM client

## Permission System
Guard checks paths before filesystem operations:
- `PermissionGranted` - Allowed (working directory)
- `PermissionPending` - User approval required (outside working dir)
- `PermissionDenied` - Blocked (system paths, .gitignore files)
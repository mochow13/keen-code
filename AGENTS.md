## Keen Code
CLI-based coding agent powered by AI using Firebase Genkit for LLM interactions.

## Architecture
- **internal/tools** - LLM tools (read_file, write_file, edit_file, glob, grep, bash)
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

## Releasing
1. Bump versions together:
   - `cmd/main.go`
   - `npm/package.json`
2. Update `CHANGELOG.md`:
   - Move `[Unreleased]` entries under a new `[X.Y.Z] - YYYY-MM-DD` heading
   - Add a new empty `[Unreleased]` section at the top
   - Update the comparison links at the bottom of the file
3. Run the tests:
   - `go test ./...`
4. Verify the npm wrapper package:
   - `cd npm && npm pack --dry-run`
5. Commit the version bump.
6. Create and push a tag in the form `vX.Y.Z`.
6. Push `main` and the tag to GitHub.
7. GitHub Actions will:
   - run GoReleaser for the tagged release
   - publish the npm package from `npm/` after the release job succeeds
8. The Git tag must match `npm/package.json` version exactly.
9. GitHub repository secrets must include `NPM_TOKEN` for npm publishing to work.

## Important Guidelines
- Minimal comments only when strictly necessary
- Test critical paths, not aiming for 100% coverage
- Always run the tests after each change
- Commit messages should be concise and focus on the key changes with bullet points

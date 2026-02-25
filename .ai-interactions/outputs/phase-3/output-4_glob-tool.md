# Phase-3: `Glob` Tool Design

This design adds a permission-gated `Glob` tool that allows LLMs to search for files using glob patterns while enforcing filesystem guard boundaries and maintaining the consistent REPL approval UX.

## 1) Requirements Mapping

| Prompt Requirement | Design Decision |
|---|---|
| Ask permission on `Glob` invocation | Use `Guard.CheckPath(path, "read")`: `Granted` = no prompt (for working dir), `Denied` = reject, `Pending` = REPL `Allow/Deny` prompt every invocation (for paths outside working dir). |
| Permission prompt via REPL arrows + Enter | Reuse existing permission request mechanism (same as `read_file`). Options: `Allow`, `Deny`, `Allow for this session`. |
| Search files based on pattern only | Tool accepts a `pattern` parameter and uses glob matching to find files. |
| Respect `internal/filesystem/guard.go` | All paths are resolved and checked against guard boundaries. Blocked paths are never traversed. |
| 1000 file limit | Return error if glob results exceed 1000 files. |
| Error on invalid/inaccessible patterns | Return explicit errors for invalid glob patterns, permission denied, IO errors, or corrupted filesystems. |
| Relative + absolute paths | Accept both; use `guard.ResolvePath` to normalize and resolve the base path. |
| Search from different directories | Allowed only when not blocked and user approves `Pending` paths. |

---

## 2) Current Architecture Fit

### Existing hooks
- Tool contract: `internal/tools/tool.go` (`Tool` + `Registry`).
- Tool execution loop: `internal/llm/genkit.go` (`executeTools(...)`).
- REPL permission requester: `internal/cli/repl/permission_requester.go`.
- Path boundary policy: `internal/filesystem/guard.go`.
- Existing read_file tool pattern: `internal/tools/read_file.go`.

### New pieces (design)
1. `internal/tools/glob.go`
   - Implements `tools.Tool` as `glob`.
   - Contains pattern validation, guard checks, file globbing, result limiting.
2. Permission mediation (reuse existing)
   - Use `PermissionRequester` interface to request user approval for pending paths.
3. Glob library dependency
   - Add `github.com/bmatcuk/doublestar/v4` for robust glob pattern support including `**` recursive patterns.

---

## 3) Tool Contract

### Tool name
`glob`

### Description
Search for files matching a glob pattern after filesystem policy + user permission checks.

### Input schema
```json
{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "Glob pattern to match files (e.g., '*.go', '**/*.md', '/absolute/path/*.txt')"
    },
    "path": {
      "type": "string",
      "description": "Optional base directory for the search (defaults to working directory)"
    }
  },
  "required": ["pattern"],
  "additionalProperties": false
}
```

### Success output
```json
{
  "pattern": "**/*.go",
  "base_path": "/resolved/search/path",
  "files": [
    "/resolved/search/path/main.go",
    "/resolved/search/path/internal/tools/glob.go"
  ],
  "count": 2
}
```

### Error output behavior
`Execute(...)` returns `error`; existing tool loop wraps as:
```json
{ "error": "...message..." }
```

---

## 4) Permission and Interaction Flow

## Guard-first decision flow
1. Parse `pattern` and optional `path` from tool input.
2. Determine base path for search:
   - If `path` provided: `basePath := guard.ResolvePath(path)`
   - Else: use working directory from guard
3. `perm := guard.CheckPath(basePath, "read")`.
4. Branch:
   - `PermissionDenied` -> fail immediately (blocked path).
   - `PermissionGranted` -> continue search.
   - `PermissionPending` -> request REPL approval.

## REPL approval flow (for `PermissionPending`)
Reuse existing mechanism from `read_file`:
1. Emit permission request event with:
   - tool name (`glob`),
   - requested base path,
   - resolved base path,
   - operation (`read`).
2. REPL shows permission selector (`Allow`/`Deny`/`Allow for this session`).
3. User navigates and confirms.
4. Choice sent back to tool execution.
5. Tool execution resumes based on response.

### UX copy (proposed)
- Title: `Allow Glob Search?`
- Body:
  - `Tool: glob`
  - `Pattern: <pattern>`
  - `Base Path: <original input or working dir>`
  - `Resolved: <normalized absolute path>`
- Hint: `[↑/↓ to navigate, Enter to confirm]`
- Options:
  - `Allow`
  - `Deny`
  - `Allow for this session`

---

## 5) File Validation and Safety Rules

## Path handling
- Accept relative and absolute input paths via `path` parameter.
- Resolve base path using `guard.ResolvePath`.
- Pattern is applied relative to resolved base path.
- Never traverse paths that fail `guard.CheckPath` or are blocked.

## Glob pattern support
- Use `doublestar` library for comprehensive glob support.
- Support patterns like:
  - `*.go` - all Go files in current dir
  - `**/*.md` - all markdown files recursively
  - `src/**/*_test.go` - test files in src directory recursively
  - `/home/user/projects/*.txt` - absolute path patterns

## 1000 file limit
- Count matches as they are collected.
- If count exceeds 1000, stop and return error.
- Error message: "search too broad: found more than 1000 files matching pattern"

## Guard boundary enforcement
- Before traversing any directory, check if it's within allowed boundaries.
- Skip directories that are blocked by guard policy.
- Never descend into blocked paths even if pattern matches them.

---

## 6) Error Taxonomy

Standardized error categories/messages:
1. `invalid input`: missing/empty/non-string `pattern`.
2. `invalid pattern`: malformed glob pattern syntax.
3. `permission denied by policy`: blocked/sensitive path (`PermissionDenied`).
4. `permission denied by user`: user selected `Deny` for pending path.
5. `path not found`: base directory does not exist.
6. `path not accessible`: permission/OS access error for base path.
7. `search too broad`: more than 1000 files matched.
8. `search failed`: IO error during filesystem traversal.

Error text should include pattern and path context when safe and useful.

---

## 7) Implementation Details

### Glob library choice
Use `github.com/bmatcuk/doublestar/v4` - industry-standard Go glob library:
- Supports `**` (recursive) patterns
- Handles edge cases and pattern validation
- Well-maintained with good test coverage

Add to go.mod:
```
require github.com/bmatcuk/doublestar/v4 v4.8.1
```

### Key implementation considerations
1. **Pattern validation**: Use `doublestar.ValidatePattern()` to check pattern syntax.
2. **Safe traversal**: Walk directory tree using `filepath.WalkDir` with guard checks at each level.
3. **Early termination**: Stop walking if file count exceeds 1000.
4. **Symlink handling**: Do not follow symlinks to avoid escaping guard boundaries.
5. **Result deduplication**: Ensure no duplicate paths in results.

---

## 8) Granular Implementation Todo List

1. Add `github.com/bmatcuk/doublestar/v4` to dependencies.
2. Create `internal/tools/glob.go` with `GlobTool` implementing `tools.Tool`.
3. Define schema (`pattern` required string, `path` optional string; no extra properties).
4. Inject dependencies into `GlobTool`:
   - filesystem guard,
   - permission requester callback/interface.
5. Implement input parsing/validation for `pattern` and `path`.
6. Resolve base path using guard resolver.
7. Evaluate guard permission (`read`) and branch by `Denied/Granted/Pending`.
8. Implement permission request via existing `PermissionRequester`.
9. Validate glob pattern using `doublestar.ValidatePattern()`.
10. Implement directory traversal with guard boundary checks.
11. Apply glob matching using `doublestar.Match()`.
12. Enforce 1000 file limit during traversal.
13. Handle errors: invalid pattern, not found, not accessible, too many files.
14. Return success payload `{pattern, base_path, files, count}`.
15. Register `GlobTool` in REPL initialization alongside existing tools.
16. Add unit tests for `GlobTool` critical paths:
    - granted search in working dir,
    - pending search + user allow,
    - pending search + user deny,
    - blocked path denied,
    - invalid pattern,
    - recursive patterns (`**/*`),
    - absolute path patterns,
    - relative path patterns,
    - directory not found,
    - >1000 files limit.
17. Add integration tests for permission flow with glob tool.
18. Verify output UX shows clear pattern context and file count.

---

## 9) Definition of Done

- `glob` tool can search for files using glob patterns.
- Supports both relative and absolute base paths.
- All searches respect filesystem guard boundaries.
- Pending paths always trigger REPL Allow/Deny prompt.
- Denied policy or denied user choice never performs search.
- Patterns are validated and rejected if malformed.
- Search limited to 1000 files with clear error on overflow.
- Supports recursive patterns (`**`) via doublestar library.
- Tests cover critical success/error paths and permission interaction behavior.
- Tool is registered and available in the REPL.

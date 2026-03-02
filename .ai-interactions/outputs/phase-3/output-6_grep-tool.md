# Phase-3: `GrepTool` Design

This design adds a permission-gated `GrepTool` that allows LLMs to search for text patterns within files while enforcing filesystem guard boundaries and maintaining the consistent REPL approval UX.

## 1) Requirements Mapping

| Prompt Requirement | Design Decision |
|---|---|
| Same permission mechanism as `ReadFile` and `Glob` | Use `Guard.CheckPath(path, "read")`: `Granted` = no prompt (working dir), `Denied` = reject, `Pending` = REPL `Allow/Deny` prompt. |
| Recursive search from working directory | Walk the directory tree using `filepath.WalkDir`, skipping blocked/gitignored paths via `Guard.IsBlocked`. |
| `pattern` parameter (required) | Regex pattern string compiled via `regexp.Compile`. |
| `path` parameter (optional, defaults to working dir) | Resolved via `guard.ResolvePath`; defaults to `""` which resolves to working dir. |
| `include` parameter (optional glob filter) | When provided, only files matching the glob (via `doublestar.Match`) are searched. When empty, all non-blocked text files are included. |
| `output_mode` parameter (`file` or `content`) | `file`: return list of matching file paths. `content`: return list of `{file, line_number, line}` entries. |
| Errors sent back to LLM | `Execute(...)` returns `error`; existing tool loop wraps as `{"error": "...message..."}`. |

---

## 2) Current Architecture Fit

### Existing hooks
- Tool contract: `internal/tools/tool.go` (`Tool` + `Registry`).
- Tool execution loop: `internal/llm/genkit.go` (`executeTools(...)`).
- REPL permission requester: `internal/cli/repl/permission_requester.go`.
- Path boundary policy: `internal/filesystem/guard.go`.
- Git awareness filtering: `internal/filesystem/gitawareness.go`.
- Existing tool patterns: `internal/tools/read_file.go`, `internal/tools/glob.go`.
- Glob library already in use: `github.com/bmatcuk/doublestar/v4`.

### New pieces
1. `internal/tools/grep.go`
   - Implements `tools.Tool` as `grep`.
   - Contains pattern compilation, guard checks, recursive file walking, line-level matching, result formatting.
2. Permission mediation (reuse existing)
   - Use `PermissionRequester` interface for pending paths.
3. No new dependencies
   - Uses stdlib `regexp` for pattern matching.
   - Reuses `doublestar/v4` (already a dependency) for `include` glob filtering.
   - Reuses `readFileContent` helper from `read_file.go` for text file validation (or similar logic for binary/null-byte checks).

---

## 3) Tool Contract

### Tool name
`grep`

### Description
Search for text patterns in files recursively after filesystem policy + user permission checks.

### Input schema
```json
{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "Regular expression pattern to search for in file contents"
    },
    "path": {
      "type": "string",
      "description": "Optional base directory for the search (defaults to working directory)"
    },
    "include": {
      "type": "string",
      "description": "Optional glob pattern to filter which files to search (e.g., '*.go', '**/*.md')"
    },
    "output_mode": {
      "type": "string",
      "enum": ["file", "content"],
      "description": "Output mode: 'file' returns matching file paths, 'content' returns matching lines with file and line number (defaults to 'content')"
    }
  },
  "required": ["pattern"],
  "additionalProperties": false
}
```

### Success output — `file` mode
```json
{
  "pattern": "func.*Handler",
  "base_path": "/resolved/search/path",
  "output_mode": "file",
  "files": [
    "/resolved/search/path/handler.go",
    "/resolved/search/path/internal/server.go"
  ],
  "count": 2
}
```

### Success output — `content` mode
```json
{
  "pattern": "func.*Handler",
  "base_path": "/resolved/search/path",
  "output_mode": "content",
  "matches": [
    {
      "file": "/resolved/search/path/handler.go",
      "line_number": 15,
      "line": "func NewHandler(cfg Config) *Handler {"
    },
    {
      "file": "/resolved/search/path/internal/server.go",
      "line_number": 42,
      "line": "func RegisterHandler(mux *http.ServeMux) {"
    }
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

### Guard-first decision flow
1. Parse `pattern`, optional `path`, optional `include`, optional `output_mode` from tool input.
2. Determine base path for search:
   - If `path` provided: `basePath := guard.ResolvePath(path)`
   - Else: resolve `""` (working directory).
3. `perm := guard.CheckPath(basePath, "read")`.
4. Branch:
   - `PermissionDenied` -> fail immediately (blocked path).
   - `PermissionGranted` -> continue search.
   - `PermissionPending` -> request REPL approval.

### REPL approval flow (for `PermissionPending`)
Reuse existing mechanism from `read_file` and `glob`:
1. Emit permission request event with:
   - tool name (`grep`),
   - requested base path,
   - resolved base path,
   - operation (`read`).
2. REPL shows permission selector (`Allow`/`Deny`/`Allow for this session`).
3. User navigates and confirms.
4. Choice sent back to tool execution.
5. Tool execution resumes based on response.

---

## 5) Search Algorithm

### File discovery
1. Walk the directory tree from the resolved base path using `filepath.WalkDir`.
2. At each directory entry:
   - If directory and `guard.IsBlocked(path)` -> `fs.SkipDir`.
   - If file and `guard.IsBlocked(path)` -> skip file.
3. If `include` glob is provided, check each file against the glob pattern using `doublestar.Match` (applied to the relative path from base).
4. For each candidate file, attempt to read contents:
   - Skip binary files (contains null bytes).
   - Skip non-UTF-8 files.
   - Skip files exceeding 1MB (reuse `maxFileSize` constant from `read_file.go`).

### Pattern matching
1. Compile `pattern` using `regexp.Compile` once before traversal.
2. For each valid text file, scan line by line.
3. For `file` mode: on first match in a file, record the file path and move to the next file.
4. For `content` mode: record every matching line with file path and line number.

### Result limiting
- Cap total matches at 1000 (reuse `maxFileLimit` concept from `glob.go`).
- In `file` mode: cap at 1000 matching files.
- In `content` mode: cap at 1000 matching lines.
- On overflow, return error: "search too broad: found more than 1000 matches".

---

## 6) Error Taxonomy

Standardized error categories/messages:
1. `invalid input`: missing/empty/non-string `pattern`, invalid `output_mode` value.
2. `invalid pattern`: regex compilation failure (include the regex error message).
3. `permission denied by policy`: blocked/sensitive path (`PermissionDenied`).
4. `permission denied by user`: user selected `Deny` for pending path.
5. `path resolution failed`: cannot resolve the base path.
6. `search too broad`: more than 1000 matches found.
7. `search failed`: IO error during filesystem traversal.

Error text should include pattern and path context when safe and useful.

---

## 7) Key Design Decisions

1. **Regex over literal strings**: Use `regexp` for flexibility. LLMs can use `regexp.QuoteMeta`-style exact strings if needed. The regex gives more power with minimal complexity.
2. **Shared text file validation**: Reuse the binary/null-byte/UTF-8/size checks from `read_file.go`. Extract or duplicate the helper (`isTextFile` logic) to avoid coupling. Since `readFileContent` is unexported and tightly coupled, duplicate the lightweight check inline.
3. **Line-by-line scanning**: Use `bufio.Scanner` for memory efficiency — never load entire file into memory for grep (unlike `read_file` which reads whole file). This allows searching files close to the 1MB limit without excessive memory use.
4. **Default output mode**: Default to `content` since it's the most useful for LLMs (they get context immediately without a follow-up `read_file` call).
5. **Include glob uses doublestar**: Reuse the existing `doublestar` dependency for consistency with `glob` tool behavior.
6. **No `exclude` parameter**: Keep the interface simple. The `include` glob combined with guard/gitignore filtering covers the common cases. Can be added later if needed.

---

## 8) Granular Implementation Todo List

1. Create `internal/tools/grep.go` with `GrepTool` struct.
2. Implement constructor `NewGrepTool(guard, permissionRequester)`.
3. Implement `Name()` returning `"grep"`.
4. Implement `Description()` returning search description.
5. Implement `InputSchema()` with `pattern` (required), `path` (optional), `include` (optional), `output_mode` (optional enum).
6. Implement `Execute()`:
   a. Parse and validate input map (pattern required, non-empty string).
   b. Parse optional `path` (default `""`).
   c. Parse optional `include` glob; validate with `doublestar.ValidatePattern` if provided.
   d. Parse optional `output_mode` (default `"content"`); reject invalid values.
   e. Compile regex pattern via `regexp.Compile`.
   f. Resolve base path via `guard.ResolvePath`.
   g. Check permission via `guard.CheckPath` — branch on `Denied`/`Granted`/`Pending`.
   h. For `Pending`, call `permissionRequester.RequestPermission`.
7. Implement `searchFiles()` private method:
   a. Walk directory tree with `filepath.WalkDir`.
   b. Skip blocked directories/files via `guard.IsBlocked`.
   c. If `include` glob set, filter files via `doublestar.Match` on relative path.
   d. For each candidate file, call `searchInFile()`.
   e. Enforce 1000 match limit; return error on overflow.
8. Implement `searchInFile()` private method:
   a. Open file, check size <= 1MB.
   b. Use `bufio.Scanner` to read line by line.
   c. Check first bytes for null byte (binary detection) — read a small buffer first, or check as lines are scanned.
   d. Match each line against compiled regex.
   e. For `file` mode: return on first match.
   f. For `content` mode: collect all matching lines with line numbers.
9. Assemble return payload:
   a. `file` mode: `{pattern, base_path, output_mode, files, count}`.
   b. `content` mode: `{pattern, base_path, output_mode, matches, count}`.
10. Register `GrepTool` in `internal/cli/repl/repl.go` in `initialModel()` alongside `ReadFileTool` and `GlobTool`.
11. Create `internal/tools/grep_test.go` with tests:
    a. `TestGrepTool_Name` — returns `"grep"`.
    b. `TestGrepTool_Description` — non-empty.
    c. `TestGrepTool_InputSchema` — validates schema structure.
    d. `TestGrepTool_Execute_InvalidInput` — nil, wrong type, missing pattern, empty pattern.
    e. `TestGrepTool_Execute_InvalidPattern` — bad regex syntax.
    f. `TestGrepTool_Execute_InvalidOutputMode` — unsupported output_mode value.
    g. `TestGrepTool_Execute_FileMode` — matches correct files, returns paths only.
    h. `TestGrepTool_Execute_ContentMode` — matches correct lines with file/line_number/line.
    i. `TestGrepTool_Execute_DefaultContentMode` — omitting output_mode defaults to content.
    j. `TestGrepTool_Execute_IncludeFilter` — only searches files matching include glob.
    k. `TestGrepTool_Execute_RecursiveSearch` — finds matches in nested directories.
    l. `TestGrepTool_Execute_NoMatches` — returns empty results, no error.
    m. `TestGrepTool_Execute_BinaryFileSkipped` — binary files silently skipped.
    n. `TestGrepTool_Execute_PendingSearch_Allow` — permission granted by user.
    o. `TestGrepTool_Execute_PendingSearch_Deny` — permission denied by user.
    p. `TestGrepTool_Execute_BlockedPath` — guard-blocked path returns error.
    q. `TestGrepTool_Execute_MatchLimit` — >1000 matches returns error.
    r. `TestGrepTool_Execute_RelativePath` — relative base path resolves correctly.
12. Run `go test ./internal/tools/...` and verify all tests pass.
13. Run `go build ./...` and verify compilation.

---

## 9) Definition of Done

- `grep` tool can search for regex patterns in files recursively.
- Supports both `file` and `content` output modes.
- Supports optional `include` glob filter for file selection.
- Supports both relative and absolute base paths.
- All searches respect filesystem guard boundaries and gitignore rules.
- Binary and non-UTF-8 files are silently skipped.
- Pending paths always trigger REPL Allow/Deny prompt.
- Denied policy or denied user choice never performs search.
- Patterns are validated (regex compilation) and rejected if invalid.
- Search limited to 1000 matches with clear error on overflow.
- Tests cover critical success/error paths and permission interaction behavior.
- Tool is registered and available in the REPL.

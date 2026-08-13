# Tools

Keen Code provides a set of built-in tools that the LLM can use to interact with the codebase. Tools are registered in a central registry and exposed to the LLM with their schemas.

## Tool Registry

```go
// internal/tools/tool.go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(ctx context.Context, input any) (any, error)
}

type Registry struct {
    tools map[string]Tool
}
```

The registry manages all available tools and converts them to provider-specific tool formats (Anthropic, OpenAI, etc.).

## Available Tools

| Tool | Purpose | Key Parameters |
|------|---------|----------------|
| `read_file` | Read file contents | `path`, `offset`, `limit` |
| `write_file` | Create/overwrite files | `path`, `content` |
| `edit_file` | Hash-anchored multi-op edits | `path`, `ops` |
| `glob` | Find files by pattern | `pattern`, `path` |
| `grep` | Search file contents | `pattern`, `path`, `include` |
| `bash` | Execute shell commands | `command`, `isDangerous`, `summary` |
| `web_fetch` | Fetch content from a URL | `url` |
| `call_mcp_tool` | Call a tool on an MCP server | `server`, `tool`, `arguments` |
| `delegate_task` | Delegate a task to a configured subagent | `profile`, `task` |

## read_file

Reads a UTF-8 text file with permission checks and validation.

```go
type ReadFileTool struct {
    guard               *filesystem.Guard
    permissionRequester PermissionRequester
}
```

**Parameters:**
- `path` (string, required): Absolute or relative path to the file
- `offset` (integer, optional): 1-based line number to start reading from (defaults to 1)
- `limit` (integer, optional): Maximum number of lines to return (defaults to 1000)

**Validation:**
- File must be valid UTF-8 text
- File must be under 25MB; `offset` and `limit` bound returned lines, not the initial file-size check
- Binary files are rejected
- Long lines are truncated to 1000 runes to keep tool results bounded

**Anchors**

Every displayed line is prefixed with an `N:HASH|` anchor: the 1-based line number and a three-character FNV-1a hash of that line's full raw content (excluding the line-ending delimiter; a CRLF `\r` is part of the delimiter). The hash is computed before display truncation, so it always covers the complete line and stays valid for `edit_file`. A terminal line ending creates no extra empty line, and there is no file-level hash footer.

**Returns:**
```json
{
  "path": "/absolute/path/to/file",
  "content": "1:69c|package main\n2:811|\n3:9a9|import \"fmt\"",
  "bytes_read": 1234,
  "offset": 1,
  "limit": 1000,
  "total_lines": 10,
  "truncated": false
}
```

Files under `~/.keen/bash/` can be read without an extra permission prompt. Use the returned `stdout_file` and `stderr_file` paths from `bash` rather than guessing artifact names.

## write_file

Creates a new file or overwrites existing content.

```go
type WriteFileTool struct {
    guard               *filesystem.Guard
    diffEmitter         DiffEmitter
    permissionRequester PermissionRequester
}
```

**Parameters:**
- `path` (string, required): Target file path
- `content` (string, required): Content to write

**Behavior:**
- Creates parent directories if needed
- Overwrites existing files completely
- Emits diff for display via `DiffEmitter`

**Returns:**
```json
{
  "path": "/absolute/path/to/file",
  "bytes_written": 1234,
  "created": true
}
```

## edit_file

Performs hash-anchored multi-op edits on existing files.

```go
type EditFileTool struct {
    guard               *filesystem.Guard
    diffEmitter         DiffEmitter
    permissionRequester PermissionRequester
}
```

**Parameters:**
- `path` (string, required): Target file path
- `ops` (array, required, non-empty): Edit operations for this one file. Every anchor validates against one file snapshot and the ops apply atomically.

**Op fields:**
- `op` (string, optional): `replace` (default), `insert_after`, `insert_before`, `insert_head`, or `insert_tail`
- `start` (string, optional): `LINE:HASH` anchor. Required for `replace`, `insert_after`, and `insert_before`; not used by `insert_head`/`insert_tail`.
- `end` (string, optional): Inclusive end `LINE:HASH` anchor for a range replacement; allowed only for `replace`.
- `text` (string, optional): Replacement or inserted text; may be multiline; empty text deletes a `replace` range.

**Anchors**

Anchors come from `read_file` output (`N:HASH|` prefixes) or `grep` matches (`line_number` + `line_hash`). The line number is 1-based; the hash is the first three lowercase hex characters of an FNV-1a 32-bit digest over the line's raw content bytes, excluding the line-ending delimiter (a CRLF `\r` is part of the delimiter). There is **no file-level hash**: an unrelated change elsewhere in the file does not invalidate a valid local anchor.

**Behavior:**
- File must already exist
- All ops validate against one immutable pre-edit snapshot before anything is written: every anchor must be in range and hash-match its current line, overlapping replace ranges are rejected, and insertions at the same position are rejected
- Validated ops apply bottom-up and the final content is written atomically (temporary file + rename)
- Reversed `start`/`end` range anchors are normalized
- Only `insert_head` is valid for an empty file
- A plain full-file unified diff is emitted via `DiffEmitter` before any permission prompt and before writing
- Memory files are secret-scanned before permission and writing

**Example — single-line replacement and insertion:**

```json
{
  "path": "internal/tools/example.go",
  "ops": [
    {"start": "6:dae", "text": "\treturn fmt.Sprintf(\"Hello, %s!\", name)"},
    {"op": "insert_after", "start": "7:f80", "text": "\n// done"}
  ]
}
```

**Example — range replacement with empty text (deletion) and head insertion:**

```json
{
  "path": "internal/tools/example.go",
  "ops": [
    {"op": "insert_head", "text": "// Package example demonstrates hashline edits.\n"},
    {"start": "1:69c", "end": "3:9a9", "text": "package main"}
  ]
}
```

**Limits**

- **No file hash** — a change far from an anchored line does not reject the edit.
- **No anchor relocation** — a line/hash mismatch fails loudly; the tool never searches for a "close enough" line.
- **One snapshot per call** — every op validates against the same pre-edit file state, so a failed op rejects the whole call and writes nothing.
- **Re-read before a later same-file edit** — anchors come only from `read_file`/`grep`; put all known same-file edits in one `ops` array, and call `read_file` again before a later edit to the same file.

**Errors** identify the failing op when an anchor is not present in the current snapshot:

```
Error: op 2: anchor "42:7f0" not found in file snapshot; re-read the file to obtain current anchors
```

**Returns:**
```json
{
  "success": true,
  "path": "/absolute/path/to/file",
  "replacementCount": 1
}
```

`file_changed` is included in the result when the content actually changed. No fresh anchors are returned — call `read_file` again before another same-file edit.

## glob

Finds files matching a glob pattern.

```go
type GlobTool struct {
    guard               *filesystem.Guard
    permissionRequester PermissionRequester
}
```

**Parameters:**
- `pattern` (string, required): Glob pattern (e.g., `*.go`, `**/*.md`)
- `path` (string, optional): Base directory (defaults to working directory)

**Limits:**
- Maximum 1000 files returned

**Returns:**
```json
{
  "pattern": "*.go",
  "base_path": "/project",
  "files": ["/project/main.go", "/project/pkg/foo.go"],
  "count": 2
}
```

## grep

Searches file contents using regular expressions.

```go
type GrepTool struct {
    guard               *filesystem.Guard
    permissionRequester PermissionRequester
}
```

**Parameters:**
- `pattern` (string, required): Regex pattern (Go/RE2 syntax)
- `path` (string, optional): Base directory
- `include` (string, optional): Glob filter for file types
- `output_mode` (string, optional): `"file"` or `"content"` (default)

**Limits:**
- Maximum 1000 matches

**Returns (content mode):**
```json
{
  "pattern": "func foo",
  "base_path": "/project",
  "output_mode": "content",
  "matches": [
    {"file": "/project/main.go", "line_number": 10, "line": "func foo() {", "line_hash": "719"},
    {"file": "/project/main.go", "line_number": 25, "line": "func foo() error {", "line_hash": "452"}
  ],
  "count": 2
}
```

The `line_number` and `line_hash` pair forms a `LINE:HASH` edit anchor usable directly in `edit_file` — a grep result needs no intermediate `read_file` purely to obtain global state. Read nearby context when you need it. Repeated lines share a hash but keep distinct line numbers.

## bash

Executes shell commands with timeout and bounded inline output. Large stdout is saved to an artifact file so the model can inspect it later without flooding the prompt.

```go
type BashTool struct {
    guard               *filesystem.Guard
    permissionRequester PermissionRequester
}
```

**Parameters:**
- `command` (string, required): Bash command to execute
- `isDangerous` (boolean, optional): Always prompts for permission if true
- `summary` (string, optional): Brief description for the UI

**Limits:**
- Timeout: 300 seconds
- Inline output: 64KB max per stream before truncation
- Truncated output preview: head/tail excerpt with omitted-byte count
- Full truncated stdout is written to randomly named files under `~/.keen/bash/`, such as `keen-bash-*.stdout`
- Stderr is returned only when the command exits non-zero; large captured stderr may be saved to `stderr_file`

When `truncated` is true, the agent should not rerun the same broad command just to see more output. It should inspect any returned `stdout_file` or `stderr_file` with `read_file` using targeted `offset`/`limit` values, or use `grep` for targeted follow-up.

**Dangerous commands (always prompt):**
- File removal (`rm`, `rm -rf`)
- Git operations that modify repo (`git commit`, `git push`, `git reset`, `git rebase`)
- Process termination (`kill`)
- System modifications

**Returns:**
```json
{
  "command": "go test ./...",
  "exit_code": 0,
  "stdout": "PASS\nok      github.com/user/keen-code    0.015s",
  "truncated": false,
  "summary": "Run Go tests"
}
```

**Returns (truncated output):**
```json
{
  "command": "grep -R plan.md ~/.keen/sessions",
  "exit_code": 0,
  "stdout": "first preview...\n\n... (1048576 bytes omitted; full stdout saved to /Users/alice/.keen/bash/keen-bash-abc123.stdout) ...\n\nlast preview...",
  "truncated": true,
  "stdout_file": "/Users/alice/.keen/bash/keen-bash-abc123.stdout"
}
```

## web_fetch

Fetches content from a URL and returns it as text.

```go
type WebFetchTool struct{}
```

**Parameters:**
- `url` (string, required): The URL to fetch

**Behavior:**
- HTML pages are automatically converted to Markdown for readability
- Other content types (JSON, plain text, XML) are returned as-is
- JavaScript-rendered pages (SPAs) return the pre-JS skeleton only

**Limits:**
- Timeout: 30 seconds
- Maximum response size: 128KB (truncated if exceeded)

**Returns:**
```json
{
  "url": "https://example.com",
  "status_code": 200,
  "content": "markdown or raw content..."
}
```

## call_mcp_tool

Calls a tool on a connected MCP (Model Context Protocol) server.

```go
type CallMCPTool struct {
    manager             keenmcp.Runtime
    permissionRequester PermissionRequester
}
```

**Parameters:**
- `server` (string, required): The MCP server name as configured
- `tool` (string, required): The exact tool name to call on the server
- `arguments` (object, optional): Key-value arguments matching the tool's input schema
- `checkCache` (boolean, optional): Reserved for future caching; set to false or omit

**Behavior:**
- Requires user permission before execution
- Server name must match a configured MCP server
- Arguments must match the tool's input schema exactly
- Skill file at `~/.keen/skills/mcp:<server>/SKILL.md` describes available tools
- Schema file at `~/.keen/skills/mcp:<server>/schemas/<tool>.json` describes required arguments

**Returns:**
```json
{
  "server": "server-name",
  "tool": "tool-name",
  "content": "tool output text"
}
```

## DiffEmitter

The `DiffEmitter` interface allows tools to emit diff output for display:

```go
// internal/tools/diff.go
type DiffEmitter interface {
    EmitDiff(lines []EditDiffLine)
}

type EditDiffLine struct {
    Kind       EditDiffLineKind
    OldLineNum int
    NewLineNum int
    Content    string
}

const (
    DiffLineContext EditDiffLineKind = iota
    DiffLineAdded
    DiffLineRemoved
    DiffLineHunk
)
```

## Permission Integration

All tools integrate with the permission system through `PermissionRequester`:

```go
// internal/tools/permission.go
type PermissionRequester interface {
    RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error)
}
```

Tools check permissions before execution and may request user approval for:
- Paths outside the working directory
- Dangerous operations (marked with `isDangerous=true`)
- First-time access to certain paths

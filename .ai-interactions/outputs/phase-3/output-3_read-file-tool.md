# Phase-3: `ReadFile` Tool Design

This design adds a permission-gated `ReadFile` tool that can read text files safely across directories while enforcing filesystem guard boundaries and a consistent REPL approval UX.

## 1) Requirements Mapping

| Prompt Requirement | Design Decision |
|---|---|
| Ask permission on `ReadFile` invocation | Use `Guard.CheckPath(path, "read")`: `Granted` = no prompt, `Denied` = reject, `Pending` = REPL `Allow/Deny` prompt every invocation. |
| Permission prompt via REPL arrows + Enter | Add a lightweight selection model in REPL (same interaction style as model selection) with 2 options: `Allow`, `Deny`. |
| Text files only | Enforce content-based text validation with UTF-8 validity + null-byte rejection. |
| Respect `internal/filesystem/guard.go` | All reads run through `guard.ResolvePath` + `guard.CheckPath(..., "read")` before file access. |
| 1MB max size | `os.Stat` before read; reject if `size > 1_048_576`. |
| Error on any read failure | Return explicit errors for invalid input, denied permission, not found, inaccessible, too large, binary/non-text, and generic IO errors. |
| Relative + absolute paths | Accept both; use `guard.ResolvePath` to normalize and resolve. |
| Read from different directories | Allowed only when not blocked and user approves `Pending` paths. |
| Ask every time (for now) | No caching in v1; session-level allow is explicitly future work. |

---

## 2) Current Architecture Fit

### Existing hooks
- Tool contract: `internal/tools/tool.go` (`Tool` + `Registry`).
- Tool execution loop: `internal/llm/genkit.go` (`executeTools(...)`).
- REPL event loop + key handling: `internal/cli/repl/repl.go`, `handlers.go`.
- Existing arrow-key selection pattern: `internal/cli/modelselection/model.go`.
- Path boundary policy: `internal/filesystem/guard.go`.

### New pieces (design)
1. `internal/tools/read_file.go`
   - Implements `tools.Tool` as `read_file`.
   - Contains path validation, guard checks, size checks, text checks, file read.
2. Permission mediation between tool execution and REPL
   - `Pending` reads require synchronous user decision from REPL.
   - Design introduces a request/response bridge so tool execution waits for decision.
3. REPL permission selector
   - New small model with options `Allow`/`Deny`, navigated by arrows + Enter.
   - Matches model-selection UX style (single-focus interaction while prompt is active).

---

## 3) Tool Contract

### Tool name
`read_file`

### Description
Read a UTF-8 text file after filesystem policy + user permission checks.

### Input schema
```json
{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute or relative path to the file to read"
    }
  },
  "required": ["path"],
  "additionalProperties": false
}
```

### Success output
```json
{
  "path": "/resolved/or/absolute/path",
  "content": "...file text...",
  "bytes_read": 1234
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
1. Parse `path` from tool input.
2. `resolvedPath := guard.ResolvePath(path)`.
3. `perm := guard.CheckPath(path, "read")` (or resolved path, consistently).
4. Branch:
   - `PermissionDenied` -> fail immediately.
   - `PermissionGranted` -> continue read.
   - `PermissionPending` -> request REPL approval.

## REPL approval flow (for `PermissionPending`)
1. LLM tool execution emits a permission request event carrying:
   - tool name (`read_file`),
   - requested path,
   - resolved path,
   - operation (`read`).
2. REPL enters permission selector state (focus lock like model selection).
3. User navigates:
   - `↑/↓` (or `j/k`) toggles `Allow`/`Deny`.
   - `Enter` confirms choice.
4. Choice is sent back to tool execution bridge.
5. Tool execution resumes:
   - `Allow` -> read proceeds.
   - `Deny` -> return permission denied error.

### UX copy (proposed)
- Title: `Allow ReadFile?`
- Body:
  - `Tool: read_file`
  - `Path: <original input>`
  - `Resolved: <normalized absolute path>`
- Hint: `[↑/↓ to navigate, Enter to confirm]`
- Options:
  - `Allow`
  - `Deny`

### Important behavior guarantees
- Every `Pending` invocation asks again (no remembered decision).
- While prompt is active, normal REPL input is paused.
- Cancel key (Esc) maps to safe default: `Deny`.

---

## 5) File Validation and Safety Rules

## Path handling
- Accept relative and absolute input paths.
- Normalize with `filepath.Clean` through `guard.ResolvePath`.
- Never bypass `guard.CheckPath` result.

## Size limit
- Max bytes: `1_048_576` (1MB).
- Check via `os.Stat` before `os.ReadFile`.

## Text-only detection (chosen strategy)
Content-based check:
1. Reject if invalid UTF-8.
2. Reject if null byte (`0x00`) exists.

This keeps validation simple while still filtering obvious non-text/binary content.

---

## 6) Error Taxonomy

Standardized error categories/messages:
1. `invalid input`: missing/empty/non-string `path`.
2. `permission denied by policy`: blocked/sensitive/gitignored path (`PermissionDenied`).
3. `permission denied by user`: user selected `Deny` for pending path.
4. `file too large`: file size exceeds 1MB.
5. `not found`: file does not exist.
6. `not accessible`: permission/OS access error.
7. `not a text file`: UTF-8/null-byte checks fail.
8. `read failed`: fallback IO/read errors.

Error text should include path context when safe and useful.

---

## 7) Stream/Event Integration Notes

Current stream events already surface `tool_start` and `tool_end`.
For permission UX, introduce explicit permission request/decision messages in the REPL/LLM bridge so the user sees an actionable prompt before read execution.

Design intent:
- Keep existing tool lifecycle visibility unchanged.
- Add minimal extra event/state plumbing only for interactive permission.
- Preserve sequential tool execution behavior in current `executeTools(...)` flow.

---

## 8) Future Enhancement (Not in this phase)

Add a third option in prompt:
- `Allow for this session`

Planned behavior later:
- cache decision by operation + path scope,
- expire on process exit,
- keep current per-call default as secure baseline.

---

## 9) Granular Implementation Todo List

1. Create `internal/tools/read_file.go` with `ReadFileTool` implementing `tools.Tool`.
2. Define schema (`path` required string; no extra properties).
3. Inject dependencies into `ReadFileTool`:
   - filesystem guard,
   - permission requester callback/interface.
4. Implement input parsing/validation for `path`.
5. Resolve path using guard resolver.
6. Evaluate guard permission (`read`) and branch by `Denied/Granted/Pending`.
7. Implement permission-request bridge contract for `Pending` decisions.
8. Add REPL permission selector model (2-option list: Allow/Deny).
9. Route key events in REPL for prompt mode (`up/down/enter/esc`).
10. Wire prompt result back to waiting tool execution path.
11. Add file existence/stat checks.
12. Enforce 1MB limit before read.
13. Read file bytes.
14. Apply UTF-8 validity + null-byte checks.
15. Return success payload `{path, content, bytes_read}`.
16. Return structured, actionable errors for all failure branches.
17. Register `ReadFileTool` in REPL initialization alongside existing tools.
18. Add unit tests for `ReadFileTool` critical paths:
    - granted read in working dir,
    - pending read + user allow,
    - pending read + user deny,
    - blocked path denied,
    - missing file,
    - inaccessible file,
    - >1MB file,
    - binary/non-text file.
19. Add focused REPL/flow tests for permission prompt behavior and key handling.
20. Add focused LLM execution flow tests for permission wait/resume semantics.
21. Verify output UX still shows tool start/end and clear errors.

---

## 10) Definition of Done

- `read_file` tool can read text files with both relative and absolute paths.
- All reads respect filesystem guard boundaries.
- Pending paths always trigger REPL Allow/Deny prompt.
- Denied policy or denied user choice never reads file content.
- Files >1MB and binary/non-text files are rejected with clear errors (UTF-8/null-byte validation).
- Tests cover critical success/error paths and permission interaction behavior.

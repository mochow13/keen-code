# `edit_file` Tool Implementation Plan

Based on the PRD at `.ai-interactions/prompts/phase-3/prompt-8_edit-file-tool.md`.

## Overview

The `edit_file` tool allows the LLM to perform string-replacement edits on existing files. Its key differentiator from `write_file` is a **diff-first UX**: the user always sees a git-style diff of the proposed changes before (or as) they are applied. The tool hooks into the existing permission architecture while introducing a new REPL interaction mode for diff review.

---

## Requirements Mapping

| PRD Requirement | Design Decision |
|---|---|
| File must exist | `os.Stat` before reading; return error if not found |
| 4 inputs: `path`, `oldString`, `newString`, `shouldReplaceAll` | Standard tool schema; all required except `shouldReplaceAll` (defaults to `false`) |
| Return `success`, `path`, `replacementCount` | Success result map with those three fields |
| Use existing permission mechanisms | Guard + new `EditPermissionRequester` interface extending `PermissionRequester` |
| Show git-style diff before applying | Inline diff generator; rendered in `PermissionSelector` or as a viewport notification |
| Ask confirm if permission not auto-granted | Extended `PermissionSelector` renders diff + Allow/AllowSession/Deny |
| If permission already granted (session), show diff and auto-apply | Notification-only path: diff posted to output without blocking |
| Seamless UX | No double confirmation; diff and permission decision are one step |

---

## Architecture

### Key insight: two permission paths for writes

The Guard always returns `PermissionPending` for `"write"` operations (never `PermissionGranted`). However, `REPLPermissionRequester` short-circuits for session-allowed tools — returning `true` immediately without entering the REPL interaction loop. For `edit_file`:

- **Session-approved** (`sessionAllowedTools["edit_file"] == true`): permission is auto-granted, but we must still **show the diff** (non-blocking, displayed in viewport).
- **Not session-approved**: permission is pending — show the diff **inside** the interactive permission prompt, so the user reviews the actual change when deciding.
- **Blocked path** (`PermissionDenied`): return error immediately, no diff shown.

---

## New Pieces Required

### 1. `internal/tools/edit_file.go` — Core Tool

Struct `EditFileTool` with:
- `guard *filesystem.Guard`
- `permissionRequester EditPermissionRequester`

Execution flow:
1. Validate inputs (`path`, `oldString`, `newString`, `shouldReplaceAll`).
2. Check `guard.IsBlocked(path)` → `PermissionDenied`: return error.
3. `resolvedPath := guard.ResolvePath(path)`.
4. Check file exists: `os.Stat(resolvedPath)` → error if missing.
5. Read current file content.
6. Compute replacement: `strings.Replace` (first) or `strings.ReplaceAll`.
7. If `oldString` not found → return error with `replacementCount: 0`.
8. Generate diff: `generateDiff(oldContent, newContent)`.
9. Call `permissionRequester.RequestEditPermission(ctx, "edit_file", path, resolvedPath, diff)`.
   - Returns `(bool, error)`: `false` = user denied or error.
10. If approved: write new content to file.
11. Return `{success: true, path: resolvedPath, replacementCount: N}`.

### 2. `internal/tools/diff.go` — Diff Generator

Function `generateDiff(oldContent, newContent string) string`:
- Splits both strings into lines.
- Produces a unified-style diff:
  ```
  @@ changes @@
  - removed line
  + added line
    context line
  ```
- Uses a simple line-by-line LCS (longest common subsequence) or patience-diff approach.
- Returns plain text; colorization happens in the REPL renderer.

### 3. `internal/tools/edit_permission_requester.go` — New Interface

```go
// EditPermissionRequester extends PermissionRequester for edit_file's diff-aware flow.
type EditPermissionRequester interface {
    PermissionRequester
    RequestEditPermission(ctx context.Context, toolName, path, resolvedPath, diff string) (bool, error)
}
```

All other tools continue using plain `PermissionRequester`. Only `edit_file` requires `EditPermissionRequester`. `REPLPermissionRequester` will implement both.

### 4. Extensions to `internal/cli/repl/permission_requester.go`

**Add to `PermissionRequest` struct:**
```go
DiffPreview string  // non-empty for edit_file requests
```

**Add to `REPLPermissionRequester`:**
```go
diffNotifyChan chan string  // for session-approved edits (non-blocking diff display)
```

**New method `RequestEditPermission`:**
```go
func (r *REPLPermissionRequester) RequestEditPermission(
    ctx context.Context, toolName, path, resolvedPath, diff string,
) (bool, error) {
    // 1. Check session-allow (same as existing shortcut in RequestPermission)
    r.mu.Lock()
    if r.sessionAllowedTools[toolName] {
        r.mu.Unlock()
        // Non-blocking: send diff to notification channel for display
        select {
        case r.diffNotifyChan <- diff:
        default:
        }
        return true, nil
    }
    r.mu.Unlock()

    // 2. Not session-approved: go through interactive REPL prompt with diff embedded
    req := &PermissionRequest{
        ToolName:     toolName,
        Path:         path,
        ResolvedPath: resolvedPath,
        Operation:    "write",
        IsDangerous:  false,
        DiffPreview:  diff,
        ResponseChan: make(chan bool, 1),
    }
    // ... rest same as RequestPermission
}
```

### 5. Extensions to `internal/cli/repl/permission_selector.go`

When `PermissionSelector` is created with a non-empty `DiffPreview`, `ViewString()` renders the diff **above** the options, with colored lines:
- Lines starting with `+` → green (`diffAddStyle`)
- Lines starting with `-` → red (`diffRemoveStyle`)
- Other lines → muted/normal

The header becomes "Review Edit?" with path/resolved info, then the diff block, then the Allow/AllowSession/Deny options.

### 6. Extensions to `internal/cli/repl/repl.go`

**Add `diffNotifyChan` polling** in `updateNormalMode`:
- Poll `permissionRequester.GetDiffNotifyChan()` alongside the existing `consumePermissionRequest`.
- When a diff notification arrives: render the colored diff as lines in `m.output` (non-interactive), scroll to bottom.
- New helper: `consumeDiffNotification(msg)` following the same pattern as `consumePermissionRequest`.

**Add `GetDiffNotifyChan()`** to `REPLPermissionRequester`:
```go
func (r *REPLPermissionRequester) GetDiffNotifyChan() <-chan string {
    return r.diffNotifyChan
}
```

### 7. Extensions to `internal/cli/repl/handlers.go`

Special-case `edit_file` in `handleToolStart` similar to how `bash` is handled:
```go
if toolCall.Name == "edit_file" {
    path, _ := toolCall.Input["path"].(string)
    cmd = m.streamHandler.HandleEditFileStart(path)
}
```
This shows a brief "Editing <path>..." indicator in the stream rather than generic tool output.

### 8. `internal/cli/repl/tool_registry.go` — Registration

```go
editFileTool := tools.NewEditFileTool(guard, permissionRequester)
appState.RegisterTool(editFileTool)
```

---

## Diff UI Flows

### Flow A: Session-approved edit (auto-applied, diff shown in output)

```
[Viewport output]
  Editing src/main.go...

  @@ changes @@
  - old line A
  + new line A
    context B

  ✓ Edit applied to src/main.go (1 replacement)
```

No blocking; the tool proceeds automatically.

### Flow B: First-time / pending edit (interactive diff+confirm)

```
[Full-screen permission mode]
  Review Edit?

  Tool:     edit_file
  Path:     src/main.go
  Resolved: /home/user/project/src/main.go

  @@ changes @@
  - old line A
  + new line A
    context B

> Allow
  Allow for this session
  Deny

  [↑/↓ to navigate, Enter to confirm, Esc to cancel]
```

User confirms → edit is applied → output shows "✓ Permission granted + edit applied".

---

## Error Taxonomy

1. `invalid input`: missing/wrong-type input parameter.
2. `invalid input: oldString must be non-empty`: empty `oldString` would replace nothing meaningful.
3. `permission denied by policy`: blocked path.
4. `permission denied by user`: user selected Deny.
5. `not found`: file does not exist.
6. `not accessible`: OS permission or I/O error when reading.
7. `string not found`: `oldString` not present in file content.
8. `write failed`: I/O error when writing replacement.

---

## Files to Create / Modify

| File | Action |
|---|---|
| `internal/tools/edit_file.go` | Create |
| `internal/tools/edit_file_test.go` | Create |
| `internal/tools/diff.go` | Create |
| `internal/tools/diff_test.go` | Create |
| `internal/tools/edit_permission_requester.go` | Create (new interface) |
| `internal/cli/repl/permission_requester.go` | Modify: add `DiffPreview` to `PermissionRequest`, add `diffNotifyChan`, add `RequestEditPermission`, add `GetDiffNotifyChan` |
| `internal/cli/repl/permission_selector.go` | Modify: render diff when `DiffPreview` non-empty, add diff line styles |
| `internal/cli/repl/repl.go` | Modify: add `consumeDiffNotification` polling in `updateNormalMode` |
| `internal/cli/repl/handlers.go` | Modify: special-case `edit_file` in `handleToolStart` |
| `internal/cli/repl/streaming.go` | Modify: add `HandleEditFileStart` method to `StreamHandler` |
| `internal/cli/repl/styles.go` | Modify: add `diffAddStyle`, `diffRemoveStyle` |
| `internal/cli/repl/tool_registry.go` | Modify: register `EditFileTool` |

---

## Fine-Grained Todo List

### Phase 1: Diff generator

- [ ] 1. Create `internal/tools/diff.go`
  - [ ] 1.1 Implement `generateDiff(oldContent, newContent string) string` using LCS line diff
  - [ ] 1.2 Format output as unified diff with `@@` header, `+`/`-`/space prefix per line
  - [ ] 1.3 Include a few lines of context around each change block

- [ ] 2. Create `internal/tools/diff_test.go`
  - [ ] 2.1 Test identical content → empty/no-change diff
  - [ ] 2.2 Test single line replacement
  - [ ] 2.3 Test multi-line replacement with context
  - [ ] 2.4 Test addition at end of file
  - [ ] 2.5 Test deletion of lines

### Phase 2: New interface

- [ ] 3. Create `internal/tools/edit_permission_requester.go`
  - [ ] 3.1 Define `EditPermissionRequester` interface extending `PermissionRequester` with `RequestEditPermission`

### Phase 3: Core tool

- [ ] 4. Create `internal/tools/edit_file.go`
  - [ ] 4.1 Define `EditFileTool` struct with `guard` and `permissionRequester EditPermissionRequester`
  - [ ] 4.2 Implement `NewEditFileTool` constructor
  - [ ] 4.3 Implement `Name()` returning `"edit_file"`
  - [ ] 4.4 Implement `Description()` method
  - [ ] 4.5 Implement `InputSchema()` with `path`, `oldString`, `newString` (required) and `shouldReplaceAll` (optional boolean, default false)
  - [ ] 4.6 Implement `Execute()`: validate and extract all 4 inputs
  - [ ] 4.7 Add guard blocked-path check → return error if denied
  - [ ] 4.8 Resolve path with `guard.ResolvePath`
  - [ ] 4.9 Check file existence with `os.Stat` → error if missing
  - [ ] 4.10 Read file content with `os.ReadFile`
  - [ ] 4.11 Apply replacement (`strings.Replace` or `strings.ReplaceAll`) and count occurrences
  - [ ] 4.12 Return error if `oldString` not found in content
  - [ ] 4.13 Call `generateDiff(oldContent, newContent)` to produce diff string
  - [ ] 4.14 Call `permissionRequester.RequestEditPermission(ctx, ...)` with the diff
  - [ ] 4.15 Write new content to file if approved
  - [ ] 4.16 Return `{success: true, path: resolvedPath, replacementCount: N}`

- [ ] 5. Create `internal/tools/edit_file_test.go`
  - [ ] 5.1 `TestEditFileTool_Name`
  - [ ] 5.2 `TestEditFileTool_Description`
  - [ ] 5.3 `TestEditFileTool_InputSchema`
  - [ ] 5.4 `TestEditFileTool_Execute_InvalidInput` (nil, missing path, missing oldString, missing newString, non-string types)
  - [ ] 5.5 `TestEditFileTool_Execute_FileNotFound`
  - [ ] 5.6 `TestEditFileTool_Execute_BlockedPath`
  - [ ] 5.7 `TestEditFileTool_Execute_StringNotFound`
  - [ ] 5.8 `TestEditFileTool_Execute_ReplaceFirst` (shouldReplaceAll=false, multiple matches → only first replaced)
  - [ ] 5.9 `TestEditFileTool_Execute_ReplaceAll` (shouldReplaceAll=true)
  - [ ] 5.10 `TestEditFileTool_Execute_UserAllows` (permission pending, user approves)
  - [ ] 5.11 `TestEditFileTool_Execute_UserDenies` (permission pending, user denies)
  - [ ] 5.12 `TestEditFileTool_Execute_SessionApproved` (RequestEditPermission returns true immediately)
  - [ ] 5.13 Verify `replacementCount` is correct in success results

### Phase 4: REPL permission_requester extensions

- [ ] 6. Modify `internal/cli/repl/permission_requester.go`
  - [ ] 6.1 Add `DiffPreview string` field to `PermissionRequest` struct
  - [ ] 6.2 Add `diffNotifyChan chan string` field to `REPLPermissionRequester`
  - [ ] 6.3 Initialize `diffNotifyChan` in `NewREPLPermissionRequester` (buffered, size 1)
  - [ ] 6.4 Implement `RequestEditPermission` method:
    - Check session-allow → if yes, send diff to `diffNotifyChan` non-blocking, return `true, nil`
    - Otherwise, build `PermissionRequest` with `DiffPreview` set, send via `requestChan`, block for response
  - [ ] 6.5 Add `GetDiffNotifyChan() <-chan string` accessor method

### Phase 5: REPL permission_selector extensions

- [ ] 7. Modify `internal/cli/repl/permission_selector.go`
  - [ ] 7.1 Add `diffPreview string` field to `PermissionSelector`
  - [ ] 7.2 Update `NewPermissionSelector` to accept and store `diffPreview` from `PermissionRequest.DiffPreview`
  - [ ] 7.3 Update `ViewString()` to render diff block when `diffPreview` is non-empty:
    - Insert diff section between path info and the options list
    - Apply `diffAddStyle` (green) to lines prefixed with `+`
    - Apply `diffRemoveStyle` (red) to lines prefixed with `-`
    - Prefix `@@` header line with a muted style
  - [ ] 7.4 Change prompt title to "Review Edit?" when `diffPreview` is non-empty

### Phase 6: REPL styles

- [ ] 8. Modify `internal/cli/repl/styles.go`
  - [ ] 8.1 Add `diffAddStyle` (green foreground)
  - [ ] 8.2 Add `diffRemoveStyle` (red foreground)
  - [ ] 8.3 Add `diffHeaderStyle` (muted/cyan for `@@` lines)

### Phase 7: REPL normal-mode diff notification

- [ ] 9. Modify `internal/cli/repl/repl.go`
  - [ ] 9.1 Add `consumeDiffNotification(msg tea.Msg) (replModel, tea.Cmd, bool)` method — polls `permissionRequester.GetDiffNotifyChan()` non-blocking
  - [ ] 9.2 When a diff notification is received: render the diff as colored output lines in `m.output`, scroll to bottom
  - [ ] 9.3 Call `consumeDiffNotification` in `updateNormalMode` alongside existing `consumePermissionRequest`

### Phase 8: REPL streaming display for edit_file

- [ ] 10. Modify `internal/cli/repl/streaming.go`
  - [ ] 10.1 Add `segmentEditFile streamSegmentType = "edit_file"` constant
  - [ ] 10.2 Add `HandleEditFileStart(path string) tea.Cmd` to `StreamHandler`
  - [ ] 10.3 Add rendering for `segmentEditFile` in `renderViewLines` / `renderTranscriptLines`: show "Editing <path>..." indicator

- [ ] 11. Modify `internal/cli/repl/handlers.go`
  - [ ] 11.1 Special-case `edit_file` in `handleToolStart`: call `m.streamHandler.HandleEditFileStart(path)` instead of generic `HandleToolStart`

### Phase 9: Tool registration

- [ ] 12. Modify `internal/cli/repl/tool_registry.go`
  - [ ] 12.1 Add `editFileTool := tools.NewEditFileTool(guard, permissionRequester)` and `appState.RegisterTool(editFileTool)`

### Phase 10: Verify

- [ ] 13. Run tool unit tests: `go test ./internal/tools/... -v -run EditFile`
- [ ] 14. Run diff unit tests: `go test ./internal/tools/... -v -run Diff`
- [ ] 15. Run all tool tests: `go test ./internal/tools/...`
- [ ] 16. Run all REPL tests: `go test ./internal/cli/repl/...`
- [ ] 17. Run full test suite: `go test ./...`
- [ ] 18. Build: `go build -o keen-cli ./cmd/keen`
- [ ] 19. Manually test in REPL: edit within working dir (session-approved flow) and outside (interactive diff+confirm flow)

---

## Key Design Decisions

1. **No double confirmation**: The diff and the Allow/Deny prompt are shown together in one `PermissionSelector` screen. Users review the exact change and decide in a single step.

2. **Session-approved = non-blocking diff display**: When the user has previously chosen "Allow for this session", subsequent edits auto-apply but the diff is still displayed in the viewport as a record of what changed.

3. **Extend, don't replace**: `EditPermissionRequester` extends the existing `PermissionRequester` interface. Only `edit_file` requires the new interface; all other tools are unaffected.

4. **`DiffPreview` in `PermissionRequest`**: Adding the diff to the existing request struct keeps the channel-based bridge intact. The `PermissionSelector` conditionally renders the diff based on whether `DiffPreview` is non-empty — no new REPL modes required.

5. **Diff generation is self-contained**: `diff.go` has no external dependencies. A simple LCS-based line diff is sufficient for code editing; no need to import a diff library.

6. **`shouldReplaceAll` defaults to `false`**: In code editing, unintended multi-replacement is a common source of bugs. Safe default requires explicit opt-in for replace-all.

7. **Error on string-not-found**: Rather than silently succeeding with 0 replacements, return an error. This prevents the LLM from thinking an edit succeeded when it didn't match.

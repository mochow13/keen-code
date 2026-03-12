# Plan: `edit_file` Tool

## Context

The agent currently has `read_file` and `write_file` tools but no way to make targeted edits to existing files. The `edit_file` tool enables the LLM to replace strings in files with a Git-style diff shown inline in the REPL UI before the edit is applied.

## Files to Modify/Create

**New:**
- `internal/tools/edit_file.go`
- `internal/tools/edit_file_test.go`

**Modified:**
- `internal/tools/permission.go` — add `EditDiffLine` types + `EditPermissionRequester` interface
- `internal/cli/repl/permission_requester.go` — add `DiffLines` to `PermissionRequest`, implement `RequestEditPermission`
- `internal/cli/repl/streaming.go` — render diff lines in permission card
- `internal/cli/repl/styles.go` — add 5 diff-specific styles
- `internal/cli/repl/output.go` — add `edit_file` to `formatToolInput` special cases
- `internal/cli/repl/repl.go` — auto-respond in `consumePermissionRequest` when `AutoApproved`
- `internal/cli/repl/tool_registry.go` — register `EditFileTool`

---

## Step 1: Types & Interface — `internal/tools/permission.go`

Add after the existing `PermissionRequester` interface:

```go
type EditDiffLineKind int

const (
    DiffLineContext EditDiffLineKind = iota
    DiffLineAdded
    DiffLineRemoved
    DiffLineHunk
)

type EditDiffLine struct {
    Kind       EditDiffLineKind
    OldLineNum int    // 0 for added lines and hunk headers
    NewLineNum int    // 0 for removed lines and hunk headers
    Content    string // raw line content without +/- prefix
}

type EditPermissionRequester interface {
    PermissionRequester
    RequestEditPermission(ctx context.Context, path, resolvedPath string, diffLines []EditDiffLine) (bool, error)
}
```

---

## Step 2: `internal/tools/edit_file.go`

### Structure

```go
type EditFileTool struct {
    guard               *filesystem.Guard
    permissionRequester EditPermissionRequester
}

func NewEditFileTool(guard *filesystem.Guard, permissionRequester EditPermissionRequester) *EditFileTool
```

### `InputSchema`

Properties: `path` (string, required), `oldString` (string, required), `newString` (string, required), `shouldReplaceAll` (bool, optional).

### `Execute` logic

1. Parse inputs; validate `path`, `oldString`, `newString` (same pattern as `write_file.go:50-74`)
2. `resolvedPath, err := t.guard.ResolvePath(path)`
3. Check `t.guard.CheckPath(path, "edit")` — deny if `PermissionDenied`
4. Read file using `readFileContent(resolvedPath)` (reuse from `read_file.go`) — returns error if file doesn't exist
5. Validate `strings.Contains(oldContent, oldString)` — error if not found
6. Apply replacement: `strings.ReplaceAll` or `strings.Replace(..., 1)` depending on `shouldReplaceAll`; track `replacementCount`
7. Compute diff: `diffLines := computeEditDiff(oldContent, newContent, oldString, newString, shouldReplaceAll)`
8. If `t.permissionRequester == nil`, return error
9. `allowed, err := t.permissionRequester.RequestEditPermission(ctx, path, resolvedPath, diffLines)`
10. If not allowed, return error
11. Write: `os.WriteFile(resolvedPath, []byte(newContent), 0644)`
12. Return `map[string]any{"success": true, "path": resolvedPath, "replacementCount": replacementCount}`

### `computeEditDiff` (unexported, same file)

Use **`github.com/aymanbagabas/go-udiff`** (already in `go.sum` as a transitive dep; `go get` promotes it to direct).

```go
import "github.com/aymanbagabas/go-udiff"

func computeEditDiff(oldContent, newContent string) []EditDiffLine {
    edits := udiff.Strings(oldContent, newContent)
    unified, err := udiff.ToUnified("old", "new", oldContent, edits, 3)
    if err != nil || unified == nil {
        return nil
    }
    var out []EditDiffLine
    for _, hunk := range unified.Hunks {
        out = append(out, EditDiffLine{
            Kind:    DiffLineHunk,
            Content: fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.FromLine, hunk.FromCount, hunk.ToLine, hunk.ToCount),
        })
        oldLine := hunk.FromLine
        newLine := hunk.ToLine
        for _, line := range hunk.Lines {
            switch line.Kind {
            case udiff.Equal:
                out = append(out, EditDiffLine{Kind: DiffLineContext, OldLineNum: oldLine, NewLineNum: newLine, Content: line.Content})
                oldLine++; newLine++
            case udiff.Delete:
                out = append(out, EditDiffLine{Kind: DiffLineRemoved, OldLineNum: oldLine, Content: line.Content})
                oldLine++
            case udiff.Insert:
                out = append(out, EditDiffLine{Kind: DiffLineAdded, NewLineNum: newLine, Content: line.Content})
                newLine++
            }
        }
    }
    return out
}
```

Call site in `Execute`: `diffLines := computeEditDiff(oldContent, newContent)` — no longer needs `oldString`/`newString`/`shouldReplaceAll` since those only affect `newContent`.

---

## Step 3: `internal/cli/repl/permission_requester.go`

### Add `DiffLines` to `PermissionRequest`

```go
DiffLines []tools.EditDiffLine  // nil for non-edit tools
```

Add `"github.com/user/keen-code/internal/tools"` import.

### Add `RequestEditPermission` to `REPLPermissionRequester`

```go
func (r *REPLPermissionRequester) RequestEditPermission(
    ctx context.Context, path, resolvedPath string, diffLines []tools.EditDiffLine,
) (bool, error) {
    autoApproved := r.sessionAllowedTools["edit_file"]
    id := atomic.AddUint64(&permissionRequestCounter, 1)
    req := &PermissionRequest{
        RequestID:    fmt.Sprintf("%d", id),
        ToolName:     "edit_file",
        Path:         path,
        ResolvedPath: resolvedPath,
        DiffLines:    diffLines,
        AutoApproved: autoApproved,
        Status:       PermissionStatusPending,
        ResponseChan: make(chan bool, 1),
    }
    r.pending = req
    select {
    case r.requestChan <- req:
        select {
        case response := <-req.ResponseChan:
            r.pending = nil
            return response, nil
        case <-ctx.Done():
            r.pending = nil
            return false, ctx.Err()
        }
    case <-ctx.Done():
        r.pending = nil
        return false, ctx.Err()
    }
}
```

Unlike `RequestPermission`, this always goes through `requestChan` — even when auto-approved — so the diff is always shown in the UI.

---

## Step 4: `internal/cli/repl/repl.go` — `consumePermissionRequest`

After `m.streamHandler.HandlePermissionRequest(req)`, add auto-approve handling:

```go
if req.AutoApproved {
    m.streamHandler.ResolvePendingPermission(PermissionStatusAutoAllowedSession)
    m.permissionRequester.SendResponse(PermissionChoiceAllowSession, req.ToolName)
}
```

This immediately unblocks the tool goroutine while still causing the diff card to render as "Auto-approved" in the transcript.

---

## Step 5: `internal/cli/repl/styles.go`

Add after `bashSummaryStyle`:

```go
diffAddStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
    Light: lipgloss.Color("#166534"), Dark: lipgloss.Color("#4ADE80"),
})
diffRemoveStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
    Light: lipgloss.Color("#991B1B"), Dark: lipgloss.Color("#F87171"),
})
diffContextStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
    Light: lipgloss.Color("#374151"), Dark: lipgloss.Color("#9CA3AF"),
})
diffHunkStyle = lipgloss.NewStyle().
    Foreground(compat.AdaptiveColor{
        Light: lipgloss.Color("#1D4ED8"), Dark: lipgloss.Color("#60A5FA"),
    }).Bold(true)
diffLineNumStyle = lipgloss.NewStyle().Foreground(mutedColor)
```

---

## Step 6: `internal/cli/repl/streaming.go`

### Add `renderDiffLine` helper (package-level function)

```go
func renderDiffLine(dl tools.EditDiffLine, width int) string {
    switch dl.Kind {
    case tools.DiffLineHunk:
        return diffHunkStyle.Render(dl.Content)
    case tools.DiffLineAdded:
        lineNum := fmt.Sprintf("%4d", dl.NewLineNum)
        line := diffAddStyle.Render("+ " + dl.Content)
        return diffLineNumStyle.Render("     "+lineNum) + " " + line
    case tools.DiffLineRemoved:
        lineNum := fmt.Sprintf("%4d", dl.OldLineNum)
        line := diffRemoveStyle.Render("- " + dl.Content)
        return diffLineNumStyle.Render(lineNum+"     ") + " " + line
    default: // DiffLineContext
        old := fmt.Sprintf("%4d", dl.OldLineNum)
        new := fmt.Sprintf("%4d", dl.NewLineNum)
        line := diffContextStyle.Render("  " + dl.Content)
        return diffLineNumStyle.Render(old+" "+new) + " " + line
    }
}
```

### Modify `renderPermissionCard`

Replace the `Preview` rendering block (lines 470-485) with:

```go
if req.DiffLines != nil {
    sb.WriteString("\n")
    for _, dl := range req.DiffLines {
        sb.WriteString(renderDiffLine(dl, width) + "\n")
    }
    sb.WriteString("\n")
} else if req.Preview != "" {
    // existing preview rendering unchanged
    ...
}
```

Add `"github.com/user/keen-code/internal/tools"` import.

---

## Step 7: `internal/cli/repl/output.go`

Change the `if toolName == "write_file"` block in `formatToolInput` to a `switch`:

```go
switch toolName {
case "write_file", "edit_file":
    if path, ok := input["path"]; ok {
        return fmt.Sprintf("path=%v", path)
    }
    return ""
}
```

---

## Step 8: `internal/cli/repl/tool_registry.go`

After registering `writeFileTool`:

```go
editFileTool := tools.NewEditFileTool(guard, permissionRequester)
appState.RegisterTool(editFileTool)
```

`permissionRequester` is `*REPLPermissionRequester` which satisfies `tools.EditPermissionRequester` after Step 3.

---

## Step 9: `internal/tools/edit_file_test.go`

Use a `mockEditPermissionRequester` implementing both `RequestPermission` and `RequestEditPermission`.

Test cases:
- Input validation (nil/wrong types, missing fields, empty path)
- `Execute` success: single replacement, replace-all, replace-first (two occurrences)
- `Execute` errors: file not found, oldString not found, permission denied by policy, permission denied by user, nil requester
- `computeEditDiff` smoke test: verify added/removed/context lines are present and hunk header is emitted for a simple single-line change

---

## Implementation Order

1. `tools/permission.go` → 2. `tools/edit_file.go` → 3. `tools/edit_file_test.go` → 4. `repl/styles.go` → 5. `repl/permission_requester.go` → 6. `repl/streaming.go` → 7. `repl/output.go` → 8. `repl/repl.go` → 9. `repl/tool_registry.go`

---

## Verification

1. `go build ./...` — no compile errors
2. `go test ./internal/tools/...` — all tests pass
3. `go test ./internal/cli/repl/...` — existing tests still pass
4. Manual test: start the agent, ask it to edit a file. Verify:
   - Tool call shows `⚙ edit_file(path=...)...`
   - Diff card appears with colored `+`/`-` lines and line numbers
   - Permission choices appear if not session-approved
   - "Allow for this session" → subsequent edits auto-approve and show diff-only card
   - File content is correctly updated on disk

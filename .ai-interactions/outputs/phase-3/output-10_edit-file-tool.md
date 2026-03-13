# Plan: `edit_file` Tool

## Context

The agent currently has `read_file` and `write_file` tools but no way to make targeted edits to existing files. The `edit_file` tool enables the LLM to replace strings in files with a Git-style diff shown inline in the REPL UI before the edit is applied.

## Architecture: DiffEmitter

The diff is shown as its own independent segment in the transcript, decoupled from permission handling. The tool:
1. Calls `diffEmitter.EmitDiff(lines)` — blocks until the REPL creates the segment
2. Then calls the standard `RequestPermission` — shows permission card only if not session-approved

This keeps `PermissionRequest` clean (no diff data), eliminates the need for a special `EditPermissionRequester` interface, and makes auto-approve transparent (standard session logic handles it).

## Files to Modify/Create

**New:**
- `internal/tools/edit_file.go`
- `internal/tools/edit_file_test.go`
- `internal/tools/diff.go` — `EditDiffLine` types + `DiffEmitter` interface

**Modified:**
- `internal/tools/permission.go` — no changes
- `internal/cli/repl/diff_emitter.go` — new `REPLDiffEmitter` type implementing `DiffEmitter`
- `internal/cli/repl/permission_requester.go` — no changes (stays focused on yes/no only)
- `internal/cli/repl/streaming.go` — add `segmentDiff` type, `HandleDiff`, `renderDiffSegment`
- `internal/cli/repl/styles.go` — add 5 diff-specific styles
- `internal/cli/repl/output.go` — add `edit_file` to `formatToolInput` special cases
- `internal/cli/repl/repl.go` — consume from `diffEmitter.GetDiffChan()` in update loop
- `internal/cli/repl/tool_registry.go` — register `EditFileTool`

---

## Step 1: `internal/tools/diff.go` (new file)

```go
package tools

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

type DiffEmitter interface {
    EmitDiff(lines []EditDiffLine)
}
```

---

## Step 2: `internal/tools/edit_file.go`

### Structure

```go
type EditFileTool struct {
    guard               *filesystem.Guard
    diffEmitter         DiffEmitter
    permissionRequester PermissionRequester
}

func NewEditFileTool(guard *filesystem.Guard, diffEmitter DiffEmitter, permissionRequester PermissionRequester) *EditFileTool
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
7. `t.diffEmitter.EmitDiff(computeEditDiff(oldContent, newContent))` — blocks until REPL acknowledges
8. `allowed, err := t.permissionRequester.RequestPermission(ctx, "edit_file", path, resolvedPath, "edit", false)`
9. If not allowed, return error
10. Write: `os.WriteFile(resolvedPath, []byte(newContent), 0644)`
11. Return `map[string]any{"success": true, "path": resolvedPath, "replacementCount": replacementCount}`

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

---

## Step 3: `internal/cli/repl/diff_emitter.go` (new file)

A standalone type responsible solely for shuttling diff lines from a tool goroutine to the REPL renderer. No knowledge of permissions.

```go
type diffEmitRequest struct {
    lines []tools.EditDiffLine
    done  chan struct{}
}

type REPLDiffEmitter struct {
    diffChan chan diffEmitRequest
}

func NewREPLDiffEmitter() *REPLDiffEmitter {
    return &REPLDiffEmitter{
        diffChan: make(chan diffEmitRequest, 1),
    }
}

func (e *REPLDiffEmitter) EmitDiff(lines []tools.EditDiffLine) {
    done := make(chan struct{})
    e.diffChan <- diffEmitRequest{lines: lines, done: done}
    <-done  // block until REPL acknowledges segment creation
}

func (e *REPLDiffEmitter) GetDiffChan() <-chan diffEmitRequest {
    return e.diffChan
}
```

`*REPLDiffEmitter` satisfies `tools.DiffEmitter`. `REPLPermissionRequester` is untouched.

---

## Step 4: `internal/cli/repl/repl.go`

In the update loop, add consumption of `diffEmitter.GetDiffChan()`. The REPL model holds `diffEmitter *REPLDiffEmitter` alongside `permissionRequester *REPLPermissionRequester`:

```go
case req := <-m.diffEmitter.GetDiffChan():
    m.streamHandler.HandleDiff(req.lines)
    close(req.done)
    return m, nil
```

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

### Add `segmentDiff` type

```go
segmentDiff streamSegmentType = "diff"
```

Add `diffLines []tools.EditDiffLine` field to `streamSegment`.

### Add `HandleDiff`

```go
func (sh *StreamHandler) HandleDiff(lines []tools.EditDiffLine) {
    sh.segments = append(sh.segments, streamSegment{
        kind:      segmentDiff,
        diffLines: lines,
    })
}
```

### Add `renderDiffSegment` and `renderDiffLine`

```go
func renderDiffLine(dl tools.EditDiffLine) string {
    switch dl.Kind {
    case tools.DiffLineHunk:
        return diffHunkStyle.Render(dl.Content)
    case tools.DiffLineAdded:
        lineNum := fmt.Sprintf("%4d", dl.NewLineNum)
        return diffLineNumStyle.Render("     "+lineNum) + " " + diffAddStyle.Render("+ "+dl.Content)
    case tools.DiffLineRemoved:
        lineNum := fmt.Sprintf("%4d", dl.OldLineNum)
        return diffLineNumStyle.Render(lineNum+"     ") + " " + diffRemoveStyle.Render("- "+dl.Content)
    default: // DiffLineContext
        return diffLineNumStyle.Render(fmt.Sprintf("%4d %4d", dl.OldLineNum, dl.NewLineNum)) + " " + diffContextStyle.Render("  "+dl.Content)
    }
}

func renderDiffSegment(seg streamSegment) []string {
    var lines []string
    for _, dl := range seg.diffLines {
        lines = append(lines, renderDiffLine(dl))
    }
    return lines
}
```

### Wire into `renderViewLines` and `renderTranscriptLines`

Both functions have a `switch seg.kind` block that renders each segment. Add a `case segmentDiff:` branch to both:

```go
case segmentDiff:
    lines = append(lines, renderDiffSegment(seg)...)
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

Create `REPLDiffEmitter` separately and pass it alongside `permissionRequester`:

```go
diffEmitter := NewREPLDiffEmitter()
editFileTool := tools.NewEditFileTool(guard, diffEmitter, permissionRequester)
appState.RegisterTool(editFileTool)
```

Also store `diffEmitter` on the REPL model so `repl.go` can consume from `GetDiffChan()`.

---

## Step 9: `internal/tools/edit_file_test.go`

Use a `mockDiffEmitter` (captures emitted lines) + standard `mockPermissionRequester`.

Test cases:
- Input validation (nil/wrong types, missing fields, empty path)
- `Execute` success: single replacement, replace-all, replace-first (two occurrences)
- `Execute` errors: file not found, oldString not found, permission denied by policy, permission denied by user
- Verify `EmitDiff` is called before `RequestPermission` in success path
- `computeEditDiff` smoke test: verify added/removed/context lines are present and hunk header is emitted for a simple single-line change

---

## Implementation Order

1. `tools/diff.go` → 2. `tools/edit_file.go` → 3. `tools/edit_file_test.go` → 4. `repl/styles.go` → 5. `repl/diff_emitter.go` → 6. `repl/streaming.go` → 7. `repl/output.go` → 8. `repl/repl.go` → 9. `repl/tool_registry.go`

---

## Verification

1. `go build ./...` — no compile errors
2. `go test ./internal/tools/...` — all tests pass
3. `go test ./internal/cli/repl/...` — existing tests still pass
4. Manual test: start the agent, ask it to edit a file. Verify:
   - Tool call shows `⚙ edit_file(path=...)...`
   - Diff segment appears with colored `+`/`-` lines and line numbers
   - Permission card appears separately below the diff (if not session-approved)
   - "Allow for this session" → subsequent edits show diff only, no permission card
   - File content is correctly updated on disk

# `edit_file` Tool Implementation Plan (Simplified)

Based on the PRD at `.ai-interactions/prompts/phase-3/prompt-8_edit-file-tool.md`.

## Overview

The `edit_file` tool allows the LLM to perform string-replacement edits on existing files. Its key differentiator from `write_file` is a **diff-first UX**: the user always sees a git-style diff of the proposed changes before they are applied. The tool hooks into the existing permission architecture while introducing a minimal REPL extension for diff review.

---

## Requirements Mapping

| PRD Requirement | Design Decision |
|---|---|
| File must exist | `os.Stat` before reading; return error if not found |
| 4 inputs: `path`, `oldString`, `newString`, `shouldReplaceAll` | Standard tool schema; all required except `shouldReplaceAll` (defaults to `false`) |
| Return `success`, `path`, `replacementCount` | Success result map with those three fields |
| Use existing permission mechanisms | Extend `PermissionRequest` with `DiffPreview` field |
| Show git-style diff before applying | Use `github.com/sergi/go-diff/diffmatchpatch` library |
| Ask confirm if permission not auto-granted | Diff embedded in `PermissionRequest`, rendered by `PermissionSelector` |
| If permission already granted (session), show diff and auto-apply | Same flow - diff generated and returned in result, displayed in output |
| Seamless UX | No double confirmation; diff and permission decision are one step |

---

## Key Simplifications from Original Plan

1. **Use existing diff library** (`github.com/sergi/go-diff/diffmatchpatch`) instead of custom LCS algorithm
2. **Extend `PermissionRequest`** with `DiffPreview` field instead of creating new `EditPermissionRequester` interface
3. **Extend existing `RequestPermission()`** method with optional `diffPreview` parameter
4. **Skip special notification channel** - diff renders inline with permission prompt
5. **Skip "Editing..." streaming indicator** - unnecessary ceremony
6. **Combine REPL phases** into cohesive updates

---

## Architecture

```
edit_file.Execute():
  1. Validate inputs (path, oldString, newString, shouldReplaceAll)
  2. Check guard permissions (blocked paths)
  3. Resolve path and verify file exists
  4. Read file content
  5. Perform string replacement (strings.Replace or ReplaceAll)
  6. Generate unified diff using go-diff library
  7. Call permissionRequester.RequestPermission(ctx, toolName, path, resolvedPath, "write", false, diffString)
  8. If approved: write new content to file
  9. Return {success, path, replacementCount}
```

---

## Files to Create / Modify

| File | Action | Details |
|------|--------|---------|
| `internal/tools/edit_file.go` | Create | Tool implementation with diff generation |
| `internal/tools/edit_file_test.go` | Create | Unit tests for all scenarios |
| `internal/cli/repl/permission_requester.go` | Modify | Add `DiffPreview` to `PermissionRequest`; extend `RequestPermission()` signature with optional `diffPreview` parameter |
| `internal/cli/repl/permission_selector.go` | Modify | Add `diffPreview` field; render colored diff in `ViewString()` when present |
| `internal/cli/repl/styles.go` | Modify | Add `diffAddStyle` (green) and `diffRemoveStyle` (red) |
| `internal/cli/repl/tool_registry.go` | Modify | Register `EditFileTool` |
| `go.mod` | Modify | Add `github.com/sergi/go-diff/diffmatchpatch` dependency |

---

## Phase 1: Core Tool

### 1. Create `internal/tools/edit_file.go`

**Imports:**
- Standard: `context`, `fmt`, `os`, `strings`
- Internal: `github.com/user/keen-cli/internal/filesystem`
- External: `github.com/sergi/go-diff/diffmatchpatch`

**Struct `EditFileTool`:**
```go
type EditFileTool struct {
    guard               *filesystem.Guard
    permissionRequester PermissionRequester
}
```

**Constructor:**
```go
func NewEditFileTool(guard *filesystem.Guard, permissionRequester PermissionRequester) *EditFileTool
```

**Interface methods:**
- `Name()` → `"edit_file"`
- `Description()` → "Edit a file by replacing a string with another string. Shows a diff before applying changes."
- `InputSchema()` → Object with properties: `path` (string, required), `oldString` (string, required), `newString` (string, required), `shouldReplaceAll` (boolean, optional, default false)
- `Execute(ctx, input)` → Main logic

**Execute flow:**
1. Validate input is `map[string]any`
2. Extract and validate: `path` (string, non-empty), `oldString` (string, non-empty), `newString` (string), `shouldReplaceAll` (bool, default false)
3. Resolve path with guard
4. Check if blocked: `guard.IsBlocked(path)` → return error if denied
5. Check file exists: `os.Stat(resolvedPath)` → error if not found
6. Read file: `os.ReadFile(resolvedPath)`
7. Perform replacement:
   - If `shouldReplaceAll`: `strings.ReplaceAll(content, oldString, newString)`
   - Else: `strings.Replace(content, oldString, newString, 1)`
   - Count replacements made
8. If `oldString` not found → return error: "string not found"
9. Generate diff using `diffmatchpatch`:
   - Create DMP instance
   - Compute line-mode diff
   - Format as unified diff with `@@` headers
10. Request permission with diff: `permissionRequester.RequestPermission(ctx, toolName, path, resolvedPath, "write", false, diffString)`
11. If not approved → return "permission denied by user" error
12. Write file: `os.WriteFile(resolvedPath, []byte(newContent), 0644)`
13. Return success result:
    ```go
    map[string]any{
        "success":           true,
        "path":              resolvedPath,
        "replacementCount":  replacementCount,
    }
    ```

### 2. Create `internal/tools/edit_file_test.go`

**Test cases:**
- `TestEditFileTool_Name` → verify returns "edit_file"
- `TestEditFileTool_Description` → verify non-empty
- `TestEditFileTool_InputSchema` → verify required fields and types
- `TestEditFileTool_Execute_InvalidInput` → nil input, missing path, missing oldString, missing newString, wrong types
- `TestEditFileTool_Execute_FileNotFound` → path that doesn't exist
- `TestEditFileTool_Execute_BlockedPath` → path blocked by guard
- `TestEditFileTool_Execute_StringNotFound` → oldString not in file
- `TestEditFileTool_Execute_ReplaceFirst` → shouldReplaceAll=false with multiple matches, only first replaced
- `TestEditFileTool_Execute_ReplaceAll` → shouldReplaceAll=true, all matches replaced
- `TestEditFileTool_Execute_UserAllows` → permission pending, mock returns true
- `TestEditFileTool_Execute_UserDenies` → permission pending, mock returns false
- `TestEditFileTool_Execute_SessionApproved` → RequestPermission returns true immediately
- `TestEditFileTool_Execute_VerifyCount` → verify replacementCount is correct

---

## Phase 2: Permission System Extensions

### 3. Modify `internal/cli/repl/permission_requester.go`

**Add to `PermissionRequest` struct:**
```go
type PermissionRequest struct {
    ToolName     string
    Path         string
    ResolvedPath string
    Operation    string
    IsDangerous  bool
    DiffPreview  string  // NEW: diff for edit_file tool
    ResponseChan chan bool
}
```

**Extend `RequestPermission` signature:**
```go
// Update existing method signature to include optional diffPreview
func (r *REPLPermissionRequester) RequestPermission(
    ctx context.Context, 
    toolName, path, resolvedPath, operation string, 
    isDangerous bool, diffPreview string,  // Add diffPreview parameter
) (bool, error) {
    // Same logic as before but includes diff in request
    r.mu.Lock()
    if !isDangerous && r.sessionAllowedTools[toolName] {
        r.mu.Unlock()
        return true, nil
    }
    r.mu.Unlock()

    req := &PermissionRequest{
        ToolName:     toolName,
        Path:         path,
        ResolvedPath: resolvedPath,
        Operation:    operation,
        IsDangerous:  isDangerous,
        DiffPreview:  diffPreview,  // Include diff (empty string for non-edit tools)
        ResponseChan: make(chan bool, 1),
    }

    r.mu.Lock()
    r.pending = req
    r.mu.Unlock()

    select {
    case r.requestChan <- req:
        select {
        case response := <-req.ResponseChan:
            r.mu.Lock()
            r.pending = nil
            r.mu.Unlock()
            return response, nil
        case <-ctx.Done():
            r.mu.Lock()
            r.pending = nil
            r.mu.Unlock()
            return false, ctx.Err()
        }
    case <-ctx.Done():
        r.mu.Lock()
        r.pending = nil
        r.mu.Unlock()
        return false, ctx.Err()
    }
}
```

**Update `PermissionRequester` interface:**
```go
// In internal/tools/permission_requester.go or wherever the interface is defined
type PermissionRequester interface {
    RequestPermission(ctx context.Context, toolName, path, resolvedPath, operation string, isDangerous bool, diffPreview string) (bool, error)
}
```

**Update existing tool calls:**
All existing tools calling `RequestPermission` need to pass empty string for `diffPreview`:
```go
// Example: write_file tool
allowed, err := t.permissionRequester.RequestPermission(ctx, t.Name(), path, resolvedPath, "write", false, "")
```

### 4. Modify `internal/cli/repl/permission_selector.go`

**Add to `PermissionSelector` struct:**
```go
type PermissionSelector struct {
    toolName     string
    path         string
    resolvedPath string
    operation    string
    isDangerous  bool
    diffPreview  string  // NEW
    cursor       int
    choices      []string
}
```

**Update `NewPermissionSelector`:**
```go
func NewPermissionSelector(toolName, path, resolvedPath, operation string, isDangerous bool, diffPreview string) *PermissionSelector {
    choices := []string{"Allow", "Allow for this session", "Deny"}
    if isDangerous {
        choices = []string{"Allow", "Deny"}
    }
    return &PermissionSelector{
        toolName:     toolName,
        path:         path,
        resolvedPath: resolvedPath,
        operation:    operation,
        isDangerous:  isDangerous,
        diffPreview:  diffPreview,  // Store diff
        cursor:       0,
        choices:      choices,
    }
}
```

**Update `ViewString()` to render diff:**
```go
func (ps *PermissionSelector) ViewString() string {
    var view strings.Builder

    // Title based on whether there's a diff
    if ps.diffPreview != "" {
        view.WriteString(titleStyle.Render("Review Edit?"))
    } else if ps.isDangerous {
        view.WriteString(warningTitleStyle.Render("⚠️ Allow Dangerous Command?"))
        view.WriteString("\n")
        view.WriteString(warningTextStyle.Render("The LLM flagged this command as potentially dangerous"))
    } else {
        view.WriteString(titleStyle.Render(fmt.Sprintf("Allow %s?", ps.toolName)))
    }
    view.WriteString("\n\n")

    // Tool/Path info
    view.WriteString("  " + infoLabelStyle.Render("Tool:") + " " + infoValueStyle.Render(ps.toolName))
    view.WriteString("\n")
    if ps.isDangerous {
        view.WriteString("  " + infoLabelStyle.Render("Command:") + " " + infoValueStyle.Render(ps.path))
    } else {
        view.WriteString("  " + infoLabelStyle.Render("Path:") + " " + infoValueStyle.Render(ps.path))
    }
    view.WriteString("\n")
    if ps.resolvedPath != "" {
        view.WriteString("  " + infoLabelStyle.Render("Resolved:") + " " + infoValueStyle.Render(ps.resolvedPath))
        view.WriteString("\n")
    }

    // Diff preview (if present)
    if ps.diffPreview != "" {
        view.WriteString("\n")
        // Split diff into lines and colorize
        lines := strings.Split(ps.diffPreview, "\n")
        for _, line := range lines {
            if strings.HasPrefix(line, "+") {
                view.WriteString(diffAddStyle.Render(line) + "\n")
            } else if strings.HasPrefix(line, "-") {
                view.WriteString(diffRemoveStyle.Render(line) + "\n")
            } else {
                view.WriteString(line + "\n")
            }
        }
    }

    view.WriteString("\n")

    // Choices
    for i, choice := range ps.choices {
        cursorStr := "  "
        style := normalStyle
        if i == ps.cursor {
            cursorStr = "> "
            style = selectionStyle
        }
        view.WriteString(cursorStr + style.Render(choice) + "\n")
    }

    view.WriteString("\n" + hintStyle.Render("[↑/↓ to navigate, Enter to confirm, Esc to cancel]"))

    return view.String()
}
```

### 5. Modify `internal/cli/repl/styles.go`

**Add diff styles:**
```go
var (
    // ... existing styles ...
    
    diffAddStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("42"))  // Green
    
    diffRemoveStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("196")) // Red
)
```

---

## Phase 3: Tool Registration

### 6. Modify `internal/cli/repl/tool_registry.go`

Add registration in `initialModel()`:
```go
editFileTool := tools.NewEditFileTool(guard, permissionRequester)
appState.RegisterTool(editFileTool)
```

---

## Phase 4: Verification

### 7. Run tests and build

```bash
# Add dependency
go get github.com/sergi/go-diff/diffmatchpatch

# Run tool tests
go test ./internal/tools/... -v -run EditFile

# Run REPL tests
go test ./internal/cli/repl/... -v

# Run all tests
go test ./...

# Build
go build -o keen-cli ./cmd/keen
```

---

## Error Taxonomy

1. `invalid input`: missing/wrong-type input parameter
2. `invalid input: oldString must be non-empty`: empty `oldString`
3. `permission denied by policy`: blocked path
4. `permission denied by user`: user selected Deny
5. `not found`: file does not exist
6. `not accessible`: OS permission or I/O error when reading
7. `string not found`: `oldString` not present in file content
8. `write failed`: I/O error when writing replacement

---

## UX Flow Examples

### Interactive Permission with Diff

```
Review Edit?

  Tool:     edit_file
  Path:     src/main.go
  Resolved: /home/user/project/src/main.go

@@ -10,7 +10,7 @@
   func main() {
-    oldCode()
+    newCode()
   }

> Allow
  Allow for this session
  Deny

[↑/↓ to navigate, Enter to confirm, Esc to cancel]
```

### Session-Approved (Auto-applied)

Diff is still shown in the viewport output after the edit completes:

```
✓ Edit applied to src/main.go (1 replacement)

@@ -10,7 +10,7 @@
   func main() {
-    oldCode()
+    newCode()
   }
```

---

## Design Decisions

1. **Use existing diff library**: `go-diff` is battle-tested, no need to implement LCS ourselves
2. **Single permission interface**: Extend `PermissionRequest` with optional `DiffPreview` field rather than creating new interface
3. **Unified UX**: Diff and permission prompt shown together in one screen
4. **Color-coded diff**: Green for additions (+), red for deletions (-)
5. **Safe default**: `shouldReplaceAll` defaults to `false` to prevent unintended multi-replacement
6. **Error on string-not-found**: Return error rather than silently succeeding with 0 replacements

---

## Summary

**Original plan**: 10 phases, custom diff algorithm, new interface, special notification channel, streaming indicator

**Simplified plan**: 3 phases, existing library, extended struct, unified rendering

The simplified plan maintains all UX requirements while reducing complexity and maintenance burden.

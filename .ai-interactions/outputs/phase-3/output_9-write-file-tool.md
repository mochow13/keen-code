# `write_file` Tool Implementation Plan

Based on the PRD at `.ai-interactions/prompts/phase-3/prompt-7_write-file-tool.md`.

## Overview

The `write_file` tool will enable LLMs to write files to the filesystem. It follows the established patterns used by existing tools (`read_file`, `glob`, `grep`) and integrates with the existing permission mechanisms.

## Requirements

1. The tool must enable LLMs to write files to the filesystem
2. The tool must use the existing permission mechanisms (Guard + PermissionRequester)
3. The tool takes 2 inputs: `path` (absolute path to file), `content` (content to write)
4. If the file already exists, it will be overwritten

## Implementation Plan

### Phase 1: Core Tool Implementation

#### 1.1 Create `internal/tools/write_file.go`

Create the main tool file implementing the `tools.Tool` interface:

**Key Components:**
- `WriteFileTool` struct with `guard` and `permissionRequester` dependencies (injected via constructor)
- `Name()` returns `"write_file"`
- `Description()` explains the tool's purpose
- `InputSchema()` defines schema with `path` and `content` string properties (both required)
- `Execute()` method that:
  1. Validates and extracts `path` and `content` from input map
  2. Resolves path using `guard.ResolvePath(path)`
  3. Checks permission using `guard.CheckPath(path, "write")`
  4. Handles permission states:
     - `PermissionDenied`: Return error
     - `PermissionPending`: Call `permissionRequester.RequestPermission()`, proceed only if allowed
     - `PermissionGranted`: Proceed directly
  5. Creates parent directories if they don't exist (`os.MkdirAll`)
  6. Writes content to file (`os.WriteFile`)
  7. Returns result map with `path`, `bytes_written`, and `created` (bool indicating if file was new)

**Error Handling:**
- Invalid input type (not a map)
- Missing or invalid `path` parameter
- Missing or invalid `content` parameter (must be string)
- Path resolution failures
- Permission denied (by policy or user)
- Directory creation failures
- File write failures

#### 1.2 Create `internal/tools/write_file_test.go`

Comprehensive test coverage following existing patterns:

**Test Cases:**
- `TestWriteFileTool_Name` - Verify tool name
- `TestWriteFileTool_Description` - Verify description is non-empty
- `TestWriteFileTool_InputSchema` - Verify schema structure (type, properties, required fields)
- `TestWriteFileTool_Execute_InvalidInput` - Test various invalid inputs (nil, string, missing path, non-string path, missing content, non-string content, empty path)
- `TestWriteFileTool_Execute_GrantedWrite` - Write file within working directory (permission granted automatically)
- `TestWriteFileTool_Execute_PendingWrite_Allow` - Write outside working dir with user approval
- `TestWriteFileTool_Execute_PendingWrite_Deny` - Write outside working dir with user denial
- `TestWriteFileTool_Execute_PermissionDenied` - Write to blocked path (should fail without user prompt)
- `TestWriteFileTool_Execute_CreateParentDirs` - Write to nested path where parent dirs don't exist
- `TestWriteFileTool_Execute_OverwriteExisting` - Write to existing file (should overwrite)
- `TestWriteFileTool_Execute_RelativePath` - Write using relative path from working directory
- `TestWriteFileTool_Execute_EmptyContent` - Write empty string to file

### Phase 2: Tool Registration

#### 2.1 Register Tool in `internal/cli/repl/tool_registry.go`

Add to `setupToolRegistry` function:

```go
writeFileTool := tools.NewWriteFileTool(guard, permissionRequester)
appState.RegisterTool(writeFileTool)
```

### Phase 3: Verification

#### 3.1 Run Tests

```bash
go test ./internal/tools/... -v -run WriteFile
```

#### 3.2 Build and Test Manually

```bash
go build -o keen-cli ./cmd/keen
./keen
```

Then test in the REPL that the tool appears in the LLM's available tools and can write files.

## Fine-Grained Todo List

### Implementation Tasks

- [ ] 1. Create `internal/tools/write_file.go`
  - [ ] 1.1 Define `WriteFileTool` struct with guard and permissionRequester fields
  - [ ] 1.2 Implement `NewWriteFileTool` constructor
  - [ ] 1.3 Implement `Name()` method returning "write_file"
  - [ ] 1.4 Implement `Description()` method
  - [ ] 1.5 Implement `InputSchema()` method with path and content properties
  - [ ] 1.6 Implement `Execute()` method with input validation
  - [ ] 1.7 Add path resolution logic
  - [ ] 1.8 Add permission checking logic (granted/pending/denied)
  - [ ] 1.9 Add parent directory creation logic
  - [ ] 1.10 Add file writing logic
  - [ ] 1.11 Add result formatting (path, bytes_written, created)

- [ ] 2. Create `internal/tools/write_file_test.go`
  - [ ] 2.1 Create mockPermissionRequester helper (or reuse from read_file_test.go)
  - [ ] 2.2 Write TestWriteFileTool_Name test
  - [ ] 2.3 Write TestWriteFileTool_Description test
  - [ ] 2.4 Write TestWriteFileTool_InputSchema test
  - [ ] 2.5 Write TestWriteFileTool_Execute_InvalidInput test with sub-tests
  - [ ] 2.6 Write TestWriteFileTool_Execute_GrantedWrite test
  - [ ] 2.7 Write TestWriteFileTool_Execute_PendingWrite_Allow test
  - [ ] 2.8 Write TestWriteFileTool_Execute_PendingWrite_Deny test
  - [ ] 2.9 Write TestWriteFileTool_Execute_PermissionDenied test
  - [ ] 2.10 Write TestWriteFileTool_Execute_CreateParentDirs test
  - [ ] 2.11 Write TestWriteFileTool_Execute_OverwriteExisting test
  - [ ] 2.12 Write TestWriteFileTool_Execute_RelativePath test
  - [ ] 2.13 Write TestWriteFileTool_Execute_EmptyContent test

- [ ] 3. Register tool in REPL
  - [ ] 3.1 Import tools package in tool_registry.go (if not already)
  - [ ] 3.2 Add writeFileTool creation and registration in setupToolRegistry

- [ ] 4. Verify implementation
  - [ ] 4.1 Run all write_file tests: `go test ./internal/tools/... -v -run WriteFile`
  - [ ] 4.2 Run all tool tests: `go test ./internal/tools/...`
  - [ ] 4.3 Build binary: `go build -o keen-cli ./cmd/keen`
  - [ ] 4.4 Test in REPL that tool is available and functional

## Key Design Decisions

1. **Permission Model**: Uses the existing "write" operation type in Guard, which always returns `PermissionPending` (even within working directory). This is consistent with the Guard's current implementation and ensures user approval for all write operations.

2. **Directory Creation**: Automatically creates parent directories if they don't exist. This is a common expectation for file writing tools and simplifies the LLM's task.

3. **Overwrite Behavior**: If file exists, it is overwritten without warning. This is explicitly required by the PRD.

4. **Result Format**: Returns a map with:
   - `path`: The resolved absolute path of the written file
   - `bytes_written`: Number of bytes written
   - `created`: Boolean indicating if this was a new file (false if overwritten)

5. **Error Handling**: Returns descriptive errors for all failure cases, consistent with existing tools.

## Files to Modify

| File | Action |
|------|--------|
| `internal/tools/write_file.go` | Create new |
| `internal/tools/write_file_test.go` | Create new |
| `internal/cli/repl/tool_registry.go` | Add registration |

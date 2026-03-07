# Bash Tool Implementation Plan

## Overview

This document outlines the implementation plan for the `bash` tool for Keen Code CLI. The `bash` tool will enable LLMs to execute bash commands in the terminal with proper permission controls and real-time output streaming.

## Requirements Summary

1. Execute any bash commands that can run in a bash shell
2. Use existing permission mechanisms with additional safeguards
3. Tool inputs: `command` (string), `isDangerous` (boolean), `summary` (string)
4. `isDangerous=true` always requires user permission, bypassing session-level grants
5. `summary` is a 5-10 word description of the command's intent
6. UI has two parts: command display (with summary) and real-time output streaming, both in markdown code blocks


## Key Components

### 1. Permission System Extension

The bash tool requires extending the existing permission system to support the `isDangerous` flag:

```
Permission Flow:
┌─────────────┐     ┌─────────────┐     ┌─────────────────┐
│  isDangerous│────▶│   Always    │────▶│  User Prompt    │
│   = true    │     │   Prompt    │     │  (skip session) │
└─────────────┘     └─────────────┘     └─────────────────┘

┌─────────────┐     ┌─────────────┐     ┌─────────────────┐
│  isDangerous│────▶│  Existing   │────▶│  Session/       │
│   = false   │     │  Mechanism  │     │  Per-command    │
└─────────────┘     └─────────────┘     └─────────────────┘
```

### 2. UI Components

The bash tool UI differs from other tools with a two-part display:

**Part 1: Command Display**
```markdown
```bash
$ ls -la
```

**Summary:** List all files with details
```

**Part 2: Output Streaming**
```bash
total 128
drwxr-xr-x  5 user user  4096 Mar  7 10:00 .
drwxr-xr-x  3 user user  4096 Mar  7 09:00 ..
...
```

## Implementation Plan

### Phase 1: Core Tool Implementation

#### Task 1.1: Create Bash Tool Structure
- **File**: `internal/tools/bash.go`
- **Description**: Implement the `BashTool` struct and constructor
- **Key Components**:
  - `BashTool` struct with `guard` and `permissionRequester` dependencies
  - `NewBashTool()` constructor following existing patterns
  - `Name()`, `Description()`, `InputSchema()` methods
  - `InputSchema()` defines the 3 parameters: `command`, `isDangerous`, `summary`

#### Task 1.2: Implement Command Execution
- **File**: `internal/tools/bash.go`
- **Description**: Implement the command execution logic with streaming support
- **Key Components**:
  - `Execute()` method to parse input and run commands
  - Use `os/exec` package with `CommandContext` for cancellation support
  - Capture both stdout and stderr
  - Set timeout (e.g., 60 seconds) to prevent hanging commands
  - Return structured output with exit code, stdout, stderr, and execution duration

#### Task 1.3: Add Permission Integration
- **File**: `internal/tools/bash.go`
- **Description**: Integrate with permission system
- **Key Components**:
  - Check if command is within working directory scope using guard
  - Pass `isDangerous` flag to permission requester
  - Handle permission denial gracefully

#### Task 1.4: Create Unit Tests
- **File**: `internal/tools/bash_test.go`
- **Description**: Comprehensive tests for the bash tool
- **Test Cases**:
  - Valid command execution
  - Command with arguments
  - Command with quoted strings
  - Invalid/non-existent command
  - Command timeout
  - Permission denied scenarios
  - `isDangerous` flag handling
  - Missing required parameters
  - Invalid input types

### Phase 2: Permission System Extension

#### Task 2.1: Extend Permission Requester Interface
- **File**: `internal/tools/read_file.go` (interface definition)
- **Description**: Extend `PermissionRequester` interface to support dangerous operations
- **Changes**:
  - Add `isDangerous` parameter to `RequestPermission()` method signature
  - Or create a new method `RequestDangerousPermission()`

#### Task 2.2: Update Existing Tools
- **Files**: `internal/tools/read_file.go`, `internal/tools/glob.go`, `internal/tools/grep.go`
- **Description**: Update existing tools to use the extended interface
- **Changes**:
  - Pass `false` for `isDangerous` in all existing permission requests

#### Task 2.3: Implement Dangerous Permission Logic
- **File**: `internal/cli/repl/permission_requester.go`
- **Description**: Implement the dangerous permission bypass logic
- **Changes**:
  - Modify `RequestPermission()` to accept `isDangerous` parameter
  - When `isDangerous=true`, always show permission prompt regardless of session grants
  - Update `REPLPermissionRequester` struct if needed

#### Task 2.4: Update Permission Selector UI
- **File**: `internal/cli/repl/permission_selector.go`
- **Description**: Update UI to handle dangerous command warnings
- **Changes**:
  - Add warning indicator for dangerous commands
  - Show "⚠️ LLM thinks the command may be dangerous" message
  - Adjust styling to highlight risk

#### Task 2.5: Update Permission Tests
- **File**: `internal/cli/repl/permission_requester_test.go` (create if doesn't exist)
- **Description**: Add tests for dangerous permission logic
- **Test Cases**:
  - Dangerous command always prompts
  - Session grant bypassed for dangerous commands
  - Non-dangerous commands respect session grants

### Phase 3: UI/Streaming Integration

#### Task 3.1: Create Bash Stream Segment Type
- **File**: `internal/cli/repl/streaming.go`
- **Description**: Add new segment type for bash tool streaming
- **Changes**:
  - Add `segmentBash` to `streamSegmentType` enum
  - Extend `streamSegment` struct to hold bash-specific data (command, summary, output)
  - Update segment handling in `renderViewLines()` and `renderTranscriptLines()`

#### Task 3.2: Implement Bash Tool Start Handler
- **File**: `internal/cli/repl/streaming.go`
- **Description**: Handle bash tool start events
- **Changes**:
  - In `HandleToolStart()`, detect bash tool and create appropriate segment
  - Format command display with markdown code block
  - Show summary below the command

#### Task 3.3: Implement Real-time Output Streaming
- **File**: `internal/cli/repl/streaming.go`
- **Description**: Handle real-time bash output streaming
- **Changes**:
  - Add `HandleBashOutput(chunk string)` method
  - Update segment content incrementally
  - Render output in markdown code block
  - Handle stream completion

#### Task 3.4: Update Output Formatting
- **File**: `internal/cli/repl/output.go`
- **Description**: Add bash-specific formatting functions
- **Changes**:
  - Add `formatBashToolStart()` for command display
  - Add `formatBashToolOutput()` for streaming output
  - Ensure proper markdown code block formatting

#### Task 3.5: Update Styles
- **File**: `internal/cli/repl/styles.go`
- **Description**: Add styles for bash tool UI
- **Changes**:
  - Add `bashCommandStyle` for command display
  - Add `bashOutputStyle` for output streaming
  - Add `bashSummaryStyle` for summary text
  - Add warning style for dangerous command indicator

### Phase 4: Tool Registration and Integration

#### Task 4.1: Register Bash Tool
- **File**: `internal/cli/repl/tool_registry.go`
- **Description**: Register the bash tool in the tool registry
- **Changes**:
  - Import bash tool package
  - Create `NewBashTool()` instance with guard and permission requester
  - Call `appState.RegisterTool(bashTool)`

#### Task 4.2: Update LLM Client Integration
- **File**: `internal/llm/` (check tool handling)
- **Description**: Ensure LLM client can handle bash tool responses
- **Changes**:
  - Verify tool response handling supports the bash tool output format
  - No changes expected if tool interface is properly implemented

### Phase 5: Testing and Validation

#### Task 5.1: Integration Testing
- **File**: Manual testing via REPL
- **Description**: Test end-to-end bash tool functionality
- **Test Scenarios**:
  - Simple command: `ls -la`
  - Command with pipes: `cat file.txt | grep pattern`
  - Command with arguments: `find . -name "*.go"`
  - Dangerous command with `isDangerous=true`
  - Command outside working directory
  - Long-running command
  - Command producing errors

#### Task 5.2: Permission Flow Testing
- **File**: Manual testing via REPL
- **Description**: Test permission flows
- **Test Scenarios**:
  - Non-dangerous command with no prior permission
  - Non-dangerous command after "Allow for this session"
  - Dangerous command after "Allow for this session" (should still prompt)
  - Denying a dangerous command

#### Task 5.3: UI/Streaming Testing
- **File**: Manual testing via REPL
- **Description**: Test UI rendering
- **Test Scenarios**:
  - Command display formatting
  - Real-time output streaming
  - Output with special characters
  - Large output handling
  - Error output display

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/tools/bash.go` | Create | Main bash tool implementation |
| `internal/tools/bash_test.go` | Create | Bash tool unit tests |
| `internal/tools/read_file.go` | Modify | Update PermissionRequester interface |
| `internal/tools/glob.go` | Modify | Update permission calls |
| `internal/tools/grep.go` | Modify | Update permission calls |
| `internal/cli/repl/permission_requester.go` | Modify | Add dangerous permission logic |
| `internal/cli/repl/permission_selector.go` | Modify | Add dangerous command warning |
| `internal/cli/repl/streaming.go` | Modify | Add bash segment type and streaming |
| `internal/cli/repl/output.go` | Modify | Add bash formatting functions |
| `internal/cli/repl/styles.go` | Modify | Add bash-specific styles |
| `internal/cli/repl/tool_registry.go` | Modify | Register bash tool |

## Fine-Grained Todo List

1. Create `internal/tools/bash.go` with `BashTool` implementing `tools.Tool` interface.
2. Define input schema with `command` (required string), `isDangerous` (boolean), `summary` (string).
3. Inject dependencies into `BashTool`: filesystem guard and permission requester.
4. Implement `Execute()` with command parsing, validation, and execution via `exec.CommandContext`.
5. Add 60-second timeout and capture stdout, stderr, and exit code.
6. Extend `PermissionRequester` interface to accept `isDangerous` parameter.
7. Update existing tools (read_file, glob, grep) to pass `false` for `isDangerous`.
8. Implement dangerous permission logic: bypass session grants when `isDangerous=true`.
9. Update `PermissionSelector` UI to show warning for dangerous commands.
10. Add `segmentBash` type to `streaming.go` for bash-specific UI handling.
11. Implement two-part UI display: command with summary, and real-time output in markdown code blocks.
12. Add bash-specific styles in `styles.go` for command, output, summary, and warning indicators.
13. Update output formatting functions in `output.go` for bash tool display.
14. Register `BashTool` in `tool_registry.go` alongside existing tools.
15. Add unit tests for `BashTool`:
    - Valid command execution with arguments and pipes,
    - Invalid/non-existent command handling,
    - Command timeout behavior,
    - Input validation (missing command, wrong types),
    - Permission denied scenarios.
16. Add permission flow tests:
    - Non-dangerous commands respect "Allow for this session",
    - Dangerous commands always prompt regardless of session grants,
    - User denial handling for dangerous commands.
17. Manual integration testing:
    - Simple commands, commands with pipes, long-running commands,
    - Real-time output streaming,
    - Dangerous command warning display.

## Technical Considerations

### Security

1. **Command Injection Prevention**: The bash tool will execute commands as-is. We rely on:
   - LLM to provide safe commands
   - User permission review before execution
   - `isDangerous` flag for additional scrutiny

2. **Path Restrictions**: Use guard to validate commands don't access blocked paths

3. **Timeout**: Set reasonable timeout (60s) to prevent resource exhaustion

4. **Resource Limits**: Consider output size limits to prevent memory issues

### Performance

1. **Streaming**: Use io.Pipe or similar for real-time output streaming
2. **Buffering**: Implement reasonable buffer sizes for output
3. **Cancellation**: Support context cancellation for long-running commands

### Error Handling

1. **Command Not Found**: Return clear error message
2. **Permission Denied**: Return user-friendly message
3. **Timeout**: Indicate command was terminated due to timeout
4. **Non-zero Exit**: Include exit code in response

## Success Criteria

- [ ] Bash tool successfully executes commands
- [ ] Permission system correctly handles `isDangerous` flag
- [ ] UI displays command and summary correctly
- [ ] Output streams in real-time
- [ ] All tests pass
- [ ] Manual testing scenarios pass
- [ ] Code follows existing patterns and style

## Future Enhancements (Out of Scope)

- Environment variable support
- Working directory specification
- Parallel command execution
- Command history/replay
- Script execution mode

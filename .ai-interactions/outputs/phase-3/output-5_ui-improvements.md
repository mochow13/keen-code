# REPL Chronological UI Improvements (Implemented)

This update implements chronological in-flight rendering by moving to ordered stream segments inside `StreamHandler` (instead of introducing a separate top-level `Timeline` type).

## Status
- **Implemented** and validated.
- Test command: `go test ./internal/cli/repl`.

## What Changed

### 1) Chronological event model inside `StreamHandler`
- Replaced `toolCalls []*llm.ToolCall` with `segments []streamSegment`.
- Added segment kinds:
  - `assistant`
  - `tool_start`
  - `tool_end`
- Assistant chunks are merged when contiguous, but still preserve sequence relative to tool events.

### 2) Rendering now follows event order
- `View()` now renders `segments` in insertion order.
- `HandleDone()` and `HandleError()` both flush transcript lines from the same ordered segments.
- Spinner remains a status line rendered after event lines.

### 3) Handler behavior aligned to prevent duplication
- `handleToolStart` / `handleToolEnd` now update stream state only during active streaming (no immediate duplicate persistent output append).
- `handleLLMChunk` now updates stream state before viewport refresh.
- `handleLLMError` now flushes pending stream lines, then appends error line.

### 4) Tests updated and expanded
- Updated existing handler/stream tests for new behavior.
- Added regression coverage for mixed sequence ordering:
  - assistant chunk -> tool start -> assistant chunk -> tool end.

## Files Updated
- `internal/cli/repl/streaming.go`
- `internal/cli/repl/handlers.go`
- `internal/cli/repl/streaming_test.go`
- `internal/cli/repl/handlers_test.go`

## Acceptance Criteria Check
- [x] Mixed assistant/tool stream renders in event order.
- [x] No more “tool calls all on top, assistant text at bottom” during active stream.
- [x] Removed duplicate in-flight tool rendering path.
- [x] Existing REPL flow remains functional with updated tests passing.

## Follow-up (Optional)
- Add collapsible tool detail blocks (compact by default, expandable params/output).
- Improve stable formatting of tool args (sorted keys) for deterministic display.
- Add a “new events” indicator when user has scrolled up.

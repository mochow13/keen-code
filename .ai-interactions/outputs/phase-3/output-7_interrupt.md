# ESC Interrupt Plan for REPL Streaming/Tool Execution

## Goal
Enable users to interrupt an active LLM stream or in-progress tool execution by pressing `esc`, without exiting the REPL.

---

## Current state (relevant locations)

- Stream starts from `handleEnterKey` with `context.Background()`:
  - `internal/cli/repl/repl.go`
- Key handling is centralized in `handleKeyMsg`:
  - `internal/cli/repl/handlers.go`
- Stream lifecycle/UI updates are driven by:
  - `handleLLMChunk`, `handleLLMDone`, `handleLLMError`, `handleToolStart`, `handleToolEnd`
- Stream active state is tracked by `StreamHandler`.

Because stream startup uses a non-cancelable context, there is no current mechanism for keyboard interruption.

---

## Implementation plan

### 1) Add in-flight cancellation state to `replModel`
- Add `streamCancel context.CancelFunc` to `replModel`.
- Add small helpers to set/clear this cancel func cleanly.

### 2) Start stream with cancelable context
- In `handleEnterKey`, replace `context.Background()` with `context.WithCancel(...)`.
- Store the cancel func in `replModel` before calling `StreamChat`.

### 3) Add `esc` key handling in `handleKeyMsg`
- Add key constant for `esc` in `handlers.go`.
- In `handleKeyMsg`, on `esc`:
  - If stream is active (`m.streamHandler.IsActive()`):
    - call `m.streamCancel()` if non-nil,
    - stop spinner,
    - interrupt/reset stream handler state,
    - append muted status line like `Interrupted (Esc)`,
    - refresh viewport.
  - If stream is not active: no-op.

### 4) Add explicit stream interruption/reset path
- Add `Interrupt()` (or equivalent) to `StreamHandler` for immediate state reset.
- Use it from ESC path so UI updates instantly.

### 5) Handle cancellation errors gracefully
- In `handleLLMError`, detect context cancellation and treat it as expected interruption.
- Avoid showing cancellation as a red failure error.
- Avoid duplicate interruption messages.

### 6) Ensure cancellation cleanup
- Clear stored cancel func on all terminal paths:
  - normal done,
  - stream error,
  - ESC interruption.

---

## Granular TODO list

- [ ] Add `streamCancel context.CancelFunc` to `replModel`.
- [ ] Add helper methods to set/clear stream cancel state.
- [ ] Update `handleEnterKey` to create `context.WithCancel` and persist cancel func.
- [ ] Add `keyEsc` constant in `handlers.go`.
- [ ] Add `esc` branch to `handleKeyMsg`.
- [ ] Gate ESC interruption with `m.streamHandler.IsActive()`.
- [ ] In ESC branch: call cancel func, set `showSpinner=false`.
- [ ] Add `StreamHandler.Interrupt()` (or equivalent reset) and call it.
- [ ] Add muted interruption line to output and update viewport.
- [ ] In `handleLLMError`, detect/suppress expected context-canceled error UX.
- [ ] Clear cancel func in done/error/interrupt paths.
- [ ] Add tests:
  - [ ] `Esc` during active stream interrupts and does not quit.
  - [ ] `Esc` while idle is no-op.
  - [ ] Cancellation path does not render failure-style error.
  - [ ] Existing `ctrl+c` behavior remains intact.

---

## Suggested validation checklist

1. Start a prompt that streams tokens; press `esc` mid-stream.
   - Stream stops quickly.
   - REPL remains interactive.
2. Trigger a long-running tool call; press `esc`.
   - Tool flow is interrupted.
   - No stuck spinner.
3. Confirm no duplicate interruption/error lines.
4. Confirm normal completion path still works unchanged.

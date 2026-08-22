# GPT-Terra Handoff: Issue 73 `ask_user`

## Start here

Implement GitHub issue [#73](https://github.com/mochow13/keen-code/issues/73) using the finalized plan in:

- `.ai-interactions/tasks/issue-73/output-1_ask-user-tool.md`

That plan is authoritative for scope and acceptance criteria. This handoff summarizes the decisions, points to the current architecture, and calls out implementation traps so work can begin without rediscovery.

## Final product contract

### Availability

- Tool name: `ask_user`.
- Register it only for the primary interactive REPL agent.
- Do not register it in headless mode.
- Do not register it in delegated subagent registries.
- Only one questionnaire may be pending at a time.

### Suggested input schema

Use plain strings and preserve order. There are no question IDs, option objects, descriptions, or separate recommendation fields.

```json
{
  "questions": [
    {
      "question": "Which database should the service use?",
      "options": ["PostgreSQL", "SQLite"]
    },
    {
      "question": "Which API style should we expose?",
      "options": ["REST", "GraphQL", "gRPC"]
    }
  ]
}
```

Constraints:

- `questions`: required array with 1–10 entries.
- `question`: required, trimmed non-empty string.
- `options`: required array with 2–6 entries.
- Every option is a trimmed non-empty string.
- Options must be unique within a question. Prefer exact string uniqueness unless existing project conventions clearly favor case-insensitive comparison.
- The first option is the model's recommendation. Describe this in the tool description/schema and preselect it in the UI.
- Avoid silently normalizing returned values beyond validating surrounding whitespace; the LLM should receive the exact selected option string from the validated request.

### Result contract

Always return the same shape:

```json
{
  "answers": ["PostgreSQL", "custom answer"],
  "cancelled": false
}
```

- Answers are strings in input-question order.
- On successful completion, there is one answer per question.
- `Esc` cancels only the questionnaire, not the assistant stream.
- Cancellation is a successful tool result with `cancelled: true` and answers completed before cancellation.
- Do not add placeholders for unanswered questions.
- Context cancellation/overall stream interruption should return `ctx.Err()` and unblock all waiting goroutines rather than masquerading as user cancellation.

### Sequential UI behavior

- Show one question at a time with `Question N of M`.
- Render all 2–6 options plus a final `Type your answer` row.
- Initially select the first option.
- Up/Down moves selection through options and the free-form row.
- Enter on an option records its string and advances to the next question.
- Typing any printable input while a questionnaire is active moves selection to `Type your answer`, activates/uses the free-form input, and inserts the typed content.
- Moving away from the free-form row preserves its draft.
- Enter on `Type your answer` while it is merely selected activates the typing cursor; it does not submit immediately.
- Enter while editing submits a non-empty custom answer and advances.
- While a questionnaire is pending, ordinary prompt submission, commands, suggestions, and queued-input creation must not consume the text.
- `Ctrl+C` remains overall stream interruption and must unblock the tool.
- After completion/cancellation, render a compact resolved summary and restore normal input behavior.
- Saved-session replay must also use a compact summary, not recreate an interactive questionnaire.

## Architecture map

### Tool execution

- Tool interface and registry: `internal/tools/tool.go`.
- Provider-neutral validation/start execution: `internal/llm/tool_execution.go`.
- All providers call `executeValidatedTool`, so a synchronously blocking `Execute` works across providers while Bubble Tea continues on its own goroutine.
- Provider loops already retain raw results for the current tool loop via `historicalToolActivity`.

Recommended structure:

- Add `internal/tools/ask_user.go` and tests.
- Put dependency-neutral request/result structs and a small requester interface in `internal/tools`, for example an interface that accepts a validated questionnaire and blocks for a result.
- Put the channel-backed interactive implementation in a new `internal/cli/repl/askuser` package. This avoids importing REPL code into `internal/tools` and avoids package cycles.
- Model the blocking/cancellation mechanics after `internal/cli/repl/permissions/requester.go`, but do not reuse permission types or semantics.
- Protect pending state with a mutex or equivalent synchronization. Unlike the current permission requester, this new requester should be race-safe because tool execution and Bubble Tea access it concurrently and the required final command is `go test -race ./...`.
- Use a buffered request channel and buffered response channel. On all send/wait paths select on `ctx.Done()`.
- Reject a second pending questionnaire immediately with a clear error; do not queue it.

### Registration

- Primary registry setup: `internal/cli/repl/tooling/tool_registry.go`.
- Interactive construction/call: `internal/cli/repl/repl.go` around `initialModel` and `SetupToolRegistry`.
- Headless construction/call: `internal/cli/repl/headless_run.go`; pass no requester so `ask_user` is omitted.
- Subagent factory: `internal/subagents/tool_factory.go`. It builds an explicit allowlist and should remain without `ask_user`. Do not pass the requester into `ToolFactory`.
- Update `internal/cli/repl/tooling/tool_registry_test.go` for registration with a requester and omission without one. Add a headless assertion if useful, but avoid over-coupling tests to internal setup signatures.

### Bubble Tea async routing

- Async multiplexer: `waitForAsyncEvent` in `internal/cli/repl/repl_helpers.go`.
- Model wrapper that supplies channels: `replModel.waitForAsyncEvent` in `internal/cli/repl/repl.go`.
- Async message types: `internal/cli/repl/stream_msgs.go`.
- Main update dispatch: `internal/cli/repl/repl.go` and keyboard handling in `internal/cli/repl/handlers.go`.

Add the requester channel to the existing select alongside LLM, permission, diff, and subagent channels. Nil channels are safe in a Go `select`, which keeps headless/noninteractive omission simple.

Important sequencing detail:

1. The provider emits generic `tool_start` before `ask_user.Execute` blocks.
2. `Execute` publishes the questionnaire request.
3. Bubble Tea receives and renders the questionnaire.
4. The UI sends a result through the request's response channel.
5. `Execute` returns; the provider emits generic `tool_end` and continues the same tool loop.

Do not invent a second provider event protocol unless necessary. The request channel is UI coordination; normal tool start/end events remain the durable tool transcript source.

### Stream UI

- Segment definition: `internal/cli/repl/stream_segments.go`.
- Permission-card precedent: `internal/cli/repl/stream_permission.go`.
- Segment rendering: `internal/cli/repl/stream_render.go`.
- Stream handler: `internal/cli/repl/stream_handler.go`.
- Input key ordering and permission interception: `internal/cli/repl/handlers.go`, especially `handleKeyMsg`.
- Main textarea initialization/rendering: `internal/cli/repl/repl.go`.

A practical design is a dedicated ask-user segment or state containing:

- request pointer/data,
- current question index,
- selected row,
- ordered completed answers,
- per-current-question free-form draft,
- whether free-form editing is active,
- pending/resolved/cancelled status.

Intercept questionnaire keys before suggestions, mode toggles, normal Enter handling, and stream-level `Esc`. Otherwise `Esc` will interrupt the whole stream and typed text may become a queued prompt.

The existing textarea can be reused as the editing buffer if state restoration is handled carefully. On entry, preserve/reset any irrelevant value; while active, route updates only to the custom answer; after resolution, reset it and restore normal focus/placeholder behavior. An isolated text-input model is also acceptable if it integrates cleanly with existing rendering. Follow the user's exact two-stage Enter behavior for the free-form row.

Keep generic `ask_user` tool start/end segments for persistence and turn-memory collection, but hide their ordinary status lines when the dedicated questionnaire summary is rendered. Otherwise users will see duplicate `Running ask_user`/`Ran ask_user` lines plus the card. During saved replay, generic transcript start/end data can be recognized by tool name and rendered as one compact summary.

Be careful when cloning stream segments in `cloneStreamSegments`: deep-copy any mutable questionnaire state needed after stream completion. Do not leave persisted/replayed state dependent on a live response channel.

### Turn memory and exact saved-session retention

This is the issue's primary non-UI requirement.

Current behavior:

- `internal/cli/repl/turn_memory.go` retains selected tool inputs but normally omits outputs unless `/tool-history full` is enabled.
- `llm.HistoricalToolActivity.RawOutput` and `HasRawOutput` are explicitly excluded from JSON in `internal/llm/message.go`.
- `historicalToolResult` in `internal/llm/message_format.go` falls back to status-only output when raw output is absent.
- Session events serialize assistant `TurnMemory`, and `session.BuildConversation` restores it (`internal/cli/repl/session_state.go`, `internal/session/event.go`, `internal/session/projection.go`).

Implement a narrow durable-output path for `ask_user`; do not begin persisting every tool's raw output. One reasonable representation is an optional JSON-serialized field on `HistoricalToolActivity` used only for outputs that must survive sessions. Pick names consistent with the code, such as `RetainedOutput`, and ensure:

- cloning deep-copies it,
- normal in-memory provider reconstruction uses it,
- session JSON includes it,
- old sessions without it remain valid,
- `historicalToolResult` prefers exact available output over status-only fallback,
- context accounting counts it,
- `turn_memory.go` always records `ask_user` input and exact output even when full tool history is disabled.

Because a valid ask-user result is always a non-nil object, an `omitempty` durable output field can distinguish absent ordinary output from a retained ask-user result without requiring broad schema changes. If a separate presence bit is added, serialize it too and test backward compatibility.

Retain the full validated tool input (`questions` and ordered options), because positional answers are not interpretable without it. The existing 4 KiB per-input-field bounding logic in `turn_memory.go` may reject/drop a large nested `questions` field. Explicitly account for this: either define a safe questionnaire-specific bound large enough for the schema limits or preserve the validated ask input via a dedicated path. Do not let `boundedHistoricalToolInput` silently remove `questions`.

### Context reduction

- Reducers: `internal/llm/context_reducer.go`.
- Provider message formatting/reconstruction is provider-specific, but all starts from `HistoricalToolActivity`.

The finalized requirement says exact answers remain available on subsequent turns. Generic context reduction currently targets provider tool-result messages without regard to tool importance. Ensure `ask_user` retained results are not replaced by `removedToolResultPlaceholder` during ordinary reduction.

Implement this explicitly rather than relying on the answer being small. Depending on provider representation, pair each tool result with its invocation/tool name or carry a protected marker while building reduction targets. Cover all supported paths:

- OpenAI chat completions,
- OpenAI Responses/Codex where applicable,
- Anthropic,
- Bedrock,
- Genkit.

Do not require exact structured retention after manual/automatic compaction; a compaction summary may preserve the semantics. Still ensure a pending questionnaire cannot deadlock or be lost during cancellation/compaction boundaries.

### Sessions and replay

- Event format: `internal/session/event.go`.
- Build/persist transcript: `internal/cli/repl/session_state.go`.
- Restore provider conversation: `internal/session/projection.go`.
- Replay visible transcript: `internal/cli/repl/session_replay.go`.

Generic tool start/end payloads already persist input and output. Keep those as the durable transcript source unless a specialized payload materially simplifies compact replay. On replay, detect `ask_user`, pair its input/output, and render one concise summary containing answered question/answer pairs plus a cancelled marker when applicable. Avoid rendering a live selectable card. TurnMemory must independently preserve the exact result for provider context after resume; transcript persistence alone is not sufficient because `BuildConversation` uses assistant messages and TurnMemory.

## Suggested implementation order

1. Add typed contract, strict validation, requester interface, and tool tests.
2. Add the race-safe REPL requester and its concurrency/cancellation tests.
3. Wire conditional primary registration; confirm headless and subagent omission.
4. Add async request routing and a minimal resolvable questionnaire state.
5. Implement keyboard behavior and active/resolved rendering with focused UI tests.
6. Add durable turn-memory output and session round-trip tests.
7. Add compact replay behavior.
8. Protect retained answers in context reducers and add provider-path tests.
9. Run formatting, tidy, full race tests, then manually exercise the interactive flow.

This order establishes the contract first and avoids building UI against unstable types. Commit only after the complete issue is working unless instructed otherwise.

## Critical tests

At minimum cover:

- Schema shape and tool description documenting first-option recommendation.
- 0/11 questions rejected; 1/10 accepted.
- 1/7 options rejected; 2/6 accepted.
- Empty and duplicate options rejected.
- Ordered predefined/custom answers returned.
- Partial answers plus `cancelled: true`.
- Context cancellation unblocks execution.
- A concurrent second request is rejected and race tests pass.
- Interactive registry includes `ask_user`; nil/headless setup omits it.
- Subagent `ToolFactory.Registry` never includes it, even if the parent registry does.
- First option preselected; Up/Down navigation.
- Typing selects/edits free-form; moving away preserves draft.
- First Enter on free-form activates editing; editing Enter submits only non-empty text.
- `Esc` returns cancellation without interrupting stream; `Ctrl+C` interrupts and unblocks.
- Normal input behavior is restored afterward.
- Later provider request reconstructs exact questions and result while tool-history mode is off.
- Session store/load plus `BuildConversation` preserves exact result.
- Replay renders a compact complete/cancelled summary.
- Context reducers do not prune `ask_user` results.
- Existing permission, queue, retry, interruption, and headless tests remain green.

## Project requirements

Follow `/AGENTS.md`:

- Minimal comments only where necessary.
- Use project conventions and avoid unnecessary dependencies.
- Run `gofmt` on all modified Go files.
- Run `go mod tidy` after changes.
- Final required test command: `go test -race ./...`.

No dependency is expected for this feature.

## Definition of done

The implementation is complete when the acceptance criteria in `output-1_ask-user-tool.md` pass, the primary interactive model can complete/cancel a sequential questionnaire without ending its tool loop, exact results survive a later turn and saved-session resume, compact replay works, headless/subagents omit the tool, no blocked goroutines remain on interruption, and `go test -race ./...` passes.
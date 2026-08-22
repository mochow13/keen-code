# Issue 73 Plan: Ask-user tool

Issue: [Tool for allowing LLMs to ask questions to the user](https://github.com/mochow13/keen-code/issues/73)

## Goal

Add a blocking, structured tool similar to Claude's `AskUserQuestionTool`. One invocation can contain multiple questions. The model can ask the interactive user for clarification during an active tool loop, receive all answers as a structured tool result, and retain that result in conversation/session history so later provider requests and saved-session resume do not lose it.

## Current architecture findings

- Tools implement `tools.Tool` and execute synchronously through `executeValidatedTool`; a blocking tool can therefore wait for a UI response while the provider/tool loop remains active (`internal/tools/tool.go:9`, `internal/llm/tool_execution.go:27`).
- Interactive permission prompts already provide a useful channel-based pattern: execution publishes a request, the Bubble Tea loop renders it and sends a response back (`internal/cli/repl/permissions/requester.go:24`, `internal/cli/repl/repl.go:544`, `internal/cli/repl/handlers.go:784`). The ask-user flow should be separate from permissions because its data, behavior, and persistence semantics differ.
- Stream rendering is segment-based, with special segments for permissions and diffs (`internal/cli/repl/stream_segments.go:9`, `internal/cli/repl/stream_render.go:41`). A question card can follow this pattern.
- Tool outputs are available to the current provider tool loop, but normal REPL turn history intentionally drops raw outputs unless full tool history is enabled (`internal/cli/repl/turn_memory.go:27`, `internal/cli/repl/turn_memory.go:65`). `HistoricalToolActivity.RawOutput` is also excluded from session JSON (`internal/llm/message.go:23`). Issue 73's retention requirement therefore needs an explicit exception or durable result representation for this tool.
- Session replay stores generic tool start/end transcript items, so an ask-user call and result can be replayed if represented as tool activity; however, the historical provider context must also preserve the answer (`internal/session/event.go:39`, `internal/cli/repl/session_state.go:152`, `internal/session/projection.go:5`).
- Headless mode currently shares the interactive tool registry and auto-approves permissions (`internal/cli/repl/headless_run.go:84`). A blocking user tool must either be omitted there or receive a noninteractive answer source.
- The repository's earlier design analysis recommends a minimal `ask_user` schema with a question, options, optional recommendation, optional multi-select/custom answer; interactive-only operation; safe cancellation; structured output; and one active ask at a time (`OMP_ANALYSIS.md:871`).

## Clarified requirements

- One tool call may contain multiple questions, displayed sequentially as `Question N of M`.
- A call contains 1–10 questions. Each question has a non-empty prompt and 2–6 unique, non-empty string options.
- Options are plain strings: no IDs, labels, descriptions, or separate values.
- The agent places its recommended answer first. The UI preselects the first option, so no separate recommendation field is needed.
- Every question also has a final `Type your answer` row for a custom string response.
- Typing while the questionnaire is active selects `Type your answer` and fills its field.
- Up/Down moves among the options and free-form row. Enter on a predefined option confirms it. Enter on `Type your answer` first activates its typing cursor; Enter while editing submits the entered text.
- The result is an ordered multi-answer contract using string values, matching the input question order: `{"answers":["PostgreSQL","custom text"],"cancelled":false}`.
- Cancelling returns control to the LLM instead of cancelling the assistant stream. It returns completed answers with `cancelled: true`; unanswered questions have no entries in `answers`.
- Exact question inputs and answer results remain available in subsequent turns and after saved-session resume. Exact retention through compaction is not required.
- Resumed sessions render completed questionnaires compactly.
- Omit the tool from headless mode and all delegated subagents. Only the primary interactive agent may invoke it.
- Only one questionnaire may be pending globally in the REPL.

## Proposed implementation plan

The plan below reflects the clarified v1 contract.

### 1. Define the ask-user contract and blocking requester

- Add an `ask_user` tool under `internal/tools` with a strict schema containing `questions`, an array of 1–10 objects. Each object contains a non-empty `question` string and 2–6 unique, non-empty string `options`.
- Keep options intentionally simple and ordered. Document that the first option is the agent's recommendation and is preselected by the UI.
- Define typed request and result structures rather than passing unvalidated maps throughout the UI. The result is `{"answers": [string, ...], "cancelled": bool}`, with answers ordered to match completed input questions.
- Introduce a requester interface/callback owned by the interactive REPL. `Execute` submits the questionnaire and blocks on its response channel or `ctx.Done()`.
- Reject malformed question/option counts, empty prompts/options, duplicate options, and a second concurrent questionnaire with actionable errors.
- On cancellation, return completed answers plus `cancelled: true` as a successful tool result so the LLM can decide how to proceed. Context cancellation still aborts execution safely.
- Add focused unit tests for schema and limits, ordered successful responses, custom answers, partial cancellation, context cancellation, and concurrent-request rejection.

Likely files:

- `internal/tools/ask_user.go` (new)
- `internal/tools/ask_user_test.go` (new)
- A small requester package under `internal/cli/repl/askuser/` (new), unless the requester types can remain dependency-neutral in `internal/tools`

### 2. Register only where interaction is available

- Extend `SetupToolRegistry` to accept the ask-user requester and register the tool only when that requester is non-nil.
- Pass the requester from primary interactive REPL initialization.
- Pass `nil` in `RunHeadless`, preventing advertisement or invocation in noninteractive runs.
- Keep the tool out of every delegated subagent registry. Subagents report clarification needs in their result so the primary agent can ask.
- Update registry tests to assert primary interactive registration and headless/subagent omission.

Likely files:

- `internal/cli/repl/tooling/tool_registry.go`
- `internal/cli/repl/tooling/tool_registry_test.go`
- `internal/cli/repl/repl.go`
- `internal/cli/repl/headless_run.go`
- Potentially `internal/subagents/*` tests if registry construction changes

### 3. Integrate requests into Bubble Tea's async event loop

- Add the ask-user request channel to `waitForAsyncEvent` alongside permission and diff channels.
- Add a dedicated Bubble Tea message carrying the questionnaire request.
- Store the pending request, current question index, selected row, completed answers, free-form draft, and editing state in dedicated stream state/segments.
- While unresolved, route normal typing to the current free-form field instead of queueing or submitting a new user prompt.
- Ensure stream interruption and context cancellation unblock `Execute` without leaking goroutines or leaving stale questionnaire state.
- Enforce one pending questionnaire globally.

Likely files:

- `internal/cli/repl/stream_msgs.go`
- `internal/cli/repl/repl.go`
- `internal/cli/repl/repl_helpers.go` or the file containing `waitForAsyncEvent`
- `internal/cli/repl/stream_segments.go`
- `internal/cli/repl/handlers.go`

### 4. Build the question UI and keyboard behavior

- Add a question card showing `Question N of M`, the question text, 2–6 options, and a final `Type your answer` row. Preselect the first option as the recommendation.
- Support Up/Down navigation across all rows. Typing selects the free-form row and inserts text; moving away keeps the draft available.
- Enter on an option records its string and advances. Enter on the free-form row first activates its typing cursor; Enter while editing submits a non-empty custom string and advances.
- `Esc` resolves the tool with completed ordered answers and `cancelled: true`; it does not interrupt the provider stream. Keep `Ctrl+C` as overall stream interruption.
- Disable ordinary prompt submission, slash-command dispatch, and queued-input creation while the questionnaire is active because typed text belongs to the free-form field.
- After each answer, advance the progress indicator. Once resolved or cancelled, replace the active card with a compact summary and continue displaying output from the same assistant turn.
- Add tests for sequential progress, default first-option selection, typing-to-free-form, cursor/edit transitions, draft preservation, option/custom submission, partial cancellation, narrow-width wrapping, and restoration of normal input behavior.

Likely files:

- `internal/cli/repl/stream_ask_user.go` (new)
- `internal/cli/repl/stream_render.go`
- `internal/cli/repl/handlers.go`
- `internal/cli/repl/repl.go`
- `internal/cli/repl/theme/styles.go` only if existing styles are insufficient
- Corresponding `*_test.go` files

### 5. Preserve the answer through turn memory and provider reconstruction

- Add `ask_user` to retained historical inputs, preserving ordered questions/options so later providers can interpret positional answers.
- Introduce an explicit durable result field or per-activity retention policy so this tool's `answers`/`cancelled` result is stored even when full tool-history mode is off. Avoid globally persisting every raw tool output.
- Ensure `historicalToolResult` reconstructs the exact ordered result for all providers on later turns.
- Make cloning and token accounting include the durable result.
- Protect retained ask-user results from ordinary tool-result pruning so the subsequent-turn guarantee holds; compaction may summarize them.
- Add provider-neutral message-format and context-reduction tests proving a saved questionnaire/result becomes a historical tool invocation/result rather than a status-only or removed placeholder.

Likely files:

- `internal/llm/message.go`
- `internal/llm/message_format.go`
- `internal/cli/repl/turn_memory.go`
- `internal/llm/message_format_test.go`
- `internal/cli/repl/turn_memory_test.go`
- `internal/llm/context_breakdown.go` if a new durable field is introduced

### 6. Persist and replay resolved questions

- Extend session serialization so exact ordered questions and answers survive process restart. Prefer a generic durable tool-result field on historical activity if backward-compatible, rather than a one-off top-level event.
- Include `ask_user` start/end in assistant transcripts, but special-case replay rendering as a compact resolved summary rather than reconstructing the interactive card.
- For cancelled questionnaires, show completed answers and a concise cancelled marker.
- Keep old session files loadable when the new fields are absent.
- Add round-trip tests covering multiple/custom/cancelled answers through append, store/load, `BuildConversation`, provider reconstruction, and compact transcript replay.

Likely files:

- `internal/llm/message.go`
- `internal/session/event.go` only if a specialized transcript payload is selected
- `internal/cli/repl/session_state.go`
- `internal/cli/repl/session_replay.go`
- `internal/session/projection_test.go`
- `internal/cli/repl/session_state_test.go`
- `internal/cli/repl/session_replay_test.go`

### 7. Respect retention boundaries during context management

- Tag ask-user results as required/non-prunable during ordinary context-window tool-result reduction because subsequent turns must retain the exact result.
- Verify automatic/manual compaction may summarize completed answers; exact structured retention after compaction is outside v1.
- Ensure a pending questionnaire cannot overlap a compaction transition in a way that loses or deadlocks the request.
- Add focused context-reduction and auto-compaction boundary tests.

Likely files:

- `internal/llm/context_reducer.go`
- `internal/llm/context_reducer_test.go`
- REPL auto-compaction tests where checkpoint persistence is covered

### 8. Final validation

- Run `gofmt` on modified Go files.
- Run `go mod tidy` (no new dependency is expected).
- Run targeted tests while implementing, then the required full suite: `go test -race ./...`.
- Manually verify a multi-question sequence: the first option is preselected, typing activates the custom answer field, answers advance in order, partial cancellation returns to the LLM, a later turn remembers the exact result, and the result remains after session resume.
- Verify headless runs and delegated agents do not advertise the tool and cannot hang waiting for unavailable UI.

## Acceptance criteria

- `ask_user` is advertised only to the primary agent in interactive REPL mode, not in headless mode or delegated subagents.
- The schema accepts 1–10 questions, each with 2–6 unique non-empty string options; the first option is preselected.
- A valid invocation blocks tool execution without blocking Bubble Tea rendering or input handling.
- Questions appear one at a time with progress, option navigation, and a `Type your answer` field driven by normal typing.
- The result returns ordered string answers and a cancellation flag to the active provider tool loop.
- `Esc` returns completed answers with `cancelled: true` and lets the LLM continue; `Ctrl+C` can still interrupt the stream.
- Exact question inputs and resolved results are available to later turns, protected from ordinary result pruning, and survive session save/resume; compaction may summarize them.
- Session replay displays a compact resolved questionnaire summary.
- Only one questionnaire can be pending, and interruption/context cancellation leaves no blocked goroutine.
- Unit/integration tests cover limits and validation, UI transitions, sequential answers, partial cancellation, history persistence, provider reconstruction, replay, and unavailable-mode behavior.
- `go mod tidy`, `gofmt`, and `go test -race ./...` pass.

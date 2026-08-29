# Turn Memory in Keen

## Table of Contents

- [Purpose](#purpose)
- [The State Layers](#the-state-layers)
- [What TurnMemory Contains](#what-turnmemory-contains)
- [Tool Output History](#tool-output-history)
- [Turn Lifecycle](#turn-lifecycle)
- [How Historical Activity Is Replayed](#how-historical-activity-is-replayed)
- [Placement and Validation](#placement-and-validation)
- [Pending Provider State](#pending-provider-state)
- [Sessions, Retries, and Compaction](#sessions-retries-and-compaction)
- [Tradeoffs](#tradeoffs)
- [Summary](#summary)

## Purpose

A coding agent needs the complete tool exchange while it is solving the current task. Carrying every tool request and result into every later turn, however, makes the prompt larger and leaves old observations looking more current than they are.

Keen separates the active tool loop from the conversation history used for later turns. `TurnMemory` is the compact historical activity attached to an assistant `Message`. By default, it preserves that tools actually ran, where they occurred in the assistant's prose, the bounded inputs used to invoke them, and whether the invocation succeeded—but not the tool's result contents. Users can opt into retaining full results for future turns with `/tool-history full`.

`TurnMemory` is not a transcript, hidden chain of thought, or planner database. It is also not the same as the session transcript: sessions retain a richer rendering of a turn for display and replay, while future model requests are projected from assistant prose and `TurnMemory`.

## Tool Output History

> [!IMPORTANT]
> **Need the model to retain exact tool results while you explore or ideate?** Run `/tool-history full`. Tool results from turns that start after this command are retained in cross-turn model history. Run `/tool-history none` to return to the compact default. Use `/tool-history` to show the current setting.

`none` is the session default. The setting applies only to future turns: enabling it cannot recover outputs that were already omitted, and disabling it does not remove raw outputs retained by earlier turns. Those earlier outputs remain until ordinary context reduction or compaction removes them.

Full output history gives later turns the actual file contents, command output, search results, errors, and other tool results that were observed earlier. It is useful for ideation and other work that depends on revisiting earlier evidence, but increases prompt size, token cost, and the likelihood of context reduction. `none` favors fresh tool calls, lower cost, and less stale context.

## The State Layers

Keen has several distinct representations of a turn:

| Layer | Lifetime | Contents | Purpose |
|---|---|---|---|
| Provider-native active state | One `StreamChat` call and its tool-loop iterations | Native assistant messages, tool calls, tool results, reasoning, and provider-specific fields | Full-fidelity reasoning and tool chaining while the turn is active |
| `TurnMemory` | Attached to an assistant message; persisted with the conversation | Ordered, bounded `HistoricalToolActivity` records | Compact cross-turn reconstruction of tool activity |
| Session transcript | Persisted session events | Visible assistant text, reasoning, tool inputs and outputs, bash output, and diffs | UI rendering and session replay; not the normal future-turn model projection |
| Pending provider state | In-memory until recovery, reset, or replacement | Provider-native messages accumulated by an incomplete tool loop | Resume an interrupted loop without converting it into lossy generic messages |

The session transcript can therefore contain raw tool details even though those details are not sent as part of ordinary later-turn conversation history. When `/tool-history full` is enabled, `TurnMemory` also holds raw results in memory for later turns in the current session. `TurnMemory` remains JSON-serializable; its internal `RawOutput` fields are explicitly excluded from JSON, so raw results are not persisted with a saved session.

## What `TurnMemory` Contains

The persisted representation is a `TurnMemory` containing zero or more `HistoricalToolActivity` values:

| Field | Meaning |
|---|---|
| `text_offset` | Byte offset in the flattened visible assistant prose where the activity is replayed |
| `tool` | Tool name, including wrapper tools such as `call_mcp_tool` |
| `input` | Bounded copy of the tool input, when the tool is eligible for retention |
| `status` | `success` when the tool invocation completed without a tool error; otherwise `error` |
| `exit_code` | Non-zero exit code extracted from a bash result, when available |
| `raw_output` | Full tool result retained only in memory when `/tool-history full` was enabled for the turn; excluded from persisted JSON |

The current REPL collector retains inputs for `read_file`, `grep`, `glob`, `web_fetch`, `bash`, `delegate_task`, `call_mcp_tool`, `write_file`, and `edit_file`. Each retained top-level field is bounded to 4 KiB: non-string values are kept only when their JSON encoding fits, while string fields for `write_file` and `edit_file` are truncated to 4 KiB at a valid UTF-8 boundary. Oversized fields are omitted. Absolute paths for the file and search tools are made relative to the working directory when possible. Other than those bounds and path relativization, inputs are not converted into a sanitized or redacted form.

This means that turn memory can include a bounded portion of file content, replacement text, command text, URLs, MCP wrapper arguments, and other tool inputs. By default, it does **not** include the corresponding tool output merely because the output contains useful metadata. With `/tool-history full`, it additionally retains the complete raw result in memory for future turns in the current session.

In the default mode, a changed file may still be recognizable from a retained `write_file` or `edit_file` input, but the collector does not infer or persist a change outcome. A bash command that runs and exits non-zero is still a successful tool invocation from Keen's perspective, so it is represented as `{"status":"success","exit_code":1}`. A tool execution error is represented as `{"status":"error"}` without the full error text. In full mode, the original result—including error text—is retained as raw output instead.

The distinction between tool error and command failure is intentional:

- `status: "error"` means the tool could not complete normally, such as an invalid tool request or execution error.
- `status: "success"` means the tool invocation completed, not that its result was useful or that an external mutation had the desired effect.
- A non-zero bash exit code describes the completed bash invocation and is retained separately.

While an active provider loop is running, provider clients also use `HistoricalToolActivity` values containing `HasRawOutput` and `RawOutput` for in-memory context accounting and compaction. The REPL uses the same fields for cross-turn historical replay when `/tool-history full` is enabled. These fields are not persisted in JSON.

## Turn Lifecycle

1. A new user turn starts with the projected conversation messages, any persisted `TurnMemory`, and possibly in-memory pending provider state from an earlier incomplete call.
2. The LLM client sends provider-native messages and may loop through many assistant responses and tool executions.
3. The REPL stream handler records visible assistant prose and an ordered segment stream. It also records tool ends and bash segments; permission prompts, diffs, reasoning, and subagent display segments are not themselves historical tool activities.
4. When the stream reaches a terminal event, the REPL walks the final surviving segments and creates activities at the byte length of the preceding visible assistant segments.
5. The assistant's visible response is stored as `Message.Content`, and the collected activities are attached as `Message.TurnMemory`.
6. For a later provider request, Keen formats the assistant prose and inserts native tool-call and tool-result blocks at the recorded offsets.
7. The session UI and transcript continue to use the original rendered segments. The reconstructed native blocks are a provider request representation, not a replacement for the visible response.
8. If the provider reports an incomplete turn, provider-native pending state is kept separately so the next call can resume the unfinished exchange.

A normal historical exchange is therefore conceptually:

```text
assistant: I will inspect the parser.
assistant tool call: read_file({"path":"internal/parser.go"})
tool result: {"status":"success"}
assistant: The parser already handles the new token shape.
```

For a bash command that exits with code 1, the result is instead compactly represented as:

```json
{"status":"success","exit_code":1}
```

The actual retained input is used in the historical tool call. Empty placeholder arguments are not used by the current implementation.

## How Historical Activity Is Replayed

`FormatMessageForProvider` deliberately returns only `Message.Content`. It does not append XML, JSON, or a textual memory block to the assistant message. Provider adapters call the shared historical-message formatter and translate its steps into their native protocols:

- OpenAI Chat Completions uses assistant tool calls followed by tool messages.
- OpenAI Responses and Codex use function-call and function-output items.
- Anthropic uses assistant tool-use blocks followed by user tool-result blocks.
- Bedrock uses assistant tool-use and user tool-result content blocks.
- Genkit uses model tool-request and tool-response parts.

Each replayed activity receives a synthetic ID of the form `historical_<message index>_<activity index>` so the provider can pair the call with its result. Original provider call IDs are not persisted in `TurnMemory`.

Historical results normally contain only the compact status object and optional `exit_code`. If an in-memory activity has `HasRawOutput`, the provider formatter uses that raw output instead. This is always used for active-turn provider state and compaction, and is used for later REPL turns when `/tool-history full` was enabled when the activity was collected.

In the default mode, the model receives evidence that an earlier invocation occurred and the bounded arguments that were used, but should refresh files, search results, command output, MCP responses, and other mutable state when it needs them again. In full mode, it also receives the earlier raw result, which remains historical evidence rather than a substitute for refreshing mutable state.

## Placement and Validation

`text_offset` is a byte offset, not a character or token offset. The REPL computes it by adding the byte lengths of visible `segmentAssistant` content before each completed `segmentToolEnd` or `segmentBash`. Reasoning segments do not contribute to the offset because reasoning is not part of `Message.Content`.

Activities at the same offset are grouped into one native assistant tool-call batch and one corresponding result batch. Their original activity order is retained.

When formatting a persisted message, Keen skips an activity when:

- its offset is negative, beyond the assistant content, or earlier than the current cursor;
- its tool name is empty; or
- its offset falls inside a UTF-8 encoding rather than at a rune boundary.

The formatter also ignores activities that arrive out of order by offset. Invalid persisted memory is therefore discarded locally rather than making provider formatting fail.

## Pending Provider State

A provider client keeps an incomplete native exchange separately from `TurnMemory`. The concrete type is provider-specific: for example, OpenAI-compatible clients keep chat message parameters, Responses and Codex clients keep response input items, Anthropic and Bedrock keep native message values, and Genkit keeps native Genkit messages.

When a non-one-shot `StreamChat` call begins, pending state is inserted immediately before the newest user message and then cleared from the client. If the resumed call accumulates more native state and ends incompletely again, the combined pending exchange is saved again. This prevents the next call from treating a completed tool loop as a new request and executing its side effects again.

The terminal event has these meanings:

| Event | Meaning | Pending-state behavior |
|---|---|---|
| `done` | Normal completion with no remaining native exchange | No pending state remains |
| `incomplete` | A native message exchange was accumulated but the turn ended abnormally, or the tool-loop limit was reached | Saved for the next non-one-shot call |
| `error` | Failure before recoverable native exchange accumulated | Not saved; existing state is cleared in the corresponding failure path |

Pending state is in-memory only. It does not survive a process crash or restart, and `Reset` clears it. One-shot calls, including automatic compaction requests, do not save pending state. Automatic compaction also replaces the provider history and clears pending state because the old native exchange is no longer the active context.

The REPL can persist a partial assistant message and its `TurnMemory` for an incomplete or interrupted turn. That persistence is useful for the visible session history, but pending native state remains the authoritative mechanism for resuming the provider tool loop.

## Sessions, Retries, and Compaction

### Sessions

When an assistant turn is persisted, Keen stores both a transcript event and the assistant message projection. The transcript event can contain the detailed tool rendering needed to display or replay the session. `session.BuildConversation` uses the user messages, assistant prose, and cloned `TurnMemory` values for ordinary future LLM requests; it does not reconstruct those requests from transcript outputs.

`CloneMessage` and `CloneTurnMemory` deep-copy retained input maps and internal raw values so the active provider state and persisted message projection do not unintentionally share mutable maps.

### Retries

Provider retries can happen after a stream error. The REPL's `RewindForRetry` removes only trailing assistant and reasoning segments from the failed attempt. Completed tool, bash, permission, and diff segments from earlier tool-loop iterations remain. Historical activity is collected from the final surviving segment list, so abandoned retry prose is not included in the next `TurnMemory`.

### Automatic compaction

During a long active turn, provider clients maintain a separate compaction history. Activities generated by the active provider loop may include raw output in memory so the compaction request can preserve the information needed to summarize the current context. If compaction succeeds, the provider history is replaced by the generated summary and the native pending state is cleared. The REPL checkpoints the visible partial turn, persists its current activity summary, then starts a fresh segment and turn-memory accumulator for the remainder of the turn.

Compaction is a separate context-reduction mechanism; it should not be confused with the ordinary cross-turn `TurnMemory` projection.

## Tradeoffs

### Benefits

- Smaller and less stale future-turn prompts than full raw tool-history replay.
- Native provider protocol shape instead of an in-band memory string.
- Enough input and status information to preserve the causal shape of previous work.
- Fresh reads of mutable workspace and external state when details are needed again.
- In-memory native recovery for incomplete tool loops.

### Costs and limitations

- Retained inputs can still be significant: each eligible top-level field may occupy up to 4 KiB, and write/edit inputs can include bounded content or replacement text.
- In the default mode, historical tool outputs, full errors, search results, file contents returned by reads, bash stdout/stderr, MCP responses, and provider call IDs are not available through normal future-turn replay. `/tool-history full` retains result contents in memory for later turns, but increases context size and is not persisted with sessions.
- Later turns may repeat reads, searches, commands, and MCP calls.
- Bounded historical arguments can be copied by a model into a new call, but they are historical context, not a substitute for a fresh schema-valid invocation.
- Byte offsets and persisted input values require validation, which the formatter performs by skipping invalid activities.
- Pending recovery is lost if the process crashes before the native state is consumed.
- Keeping the session transcript and keeping model conversation history are different choices: the transcript may remain detailed even when the future prompt is intentionally lean.

This design works best when the workspace is the source of truth, read-only and external observations can be refreshed, and compact context is more valuable than exhaustive replay. A richer durable result store would be more appropriate for expensive or irreproducible observations.

## Summary

`TurnMemory` is a bounded historical execution summary, not a raw transcript. By default, for each retained completed tool activity, Keen stores its position in assistant prose, tool name, bounded invocation input, status, and—when applicable—the non-zero bash exit code. `/tool-history full` additionally retains complete tool results in memory for later turns in the current session; `/tool-history none` restores the compact default. Raw outputs are excluded from persisted session JSON.

Within an active provider loop, Keen keeps native tool calls and results, including raw outputs where needed for compaction. Across turns, providers receive native historical call/result blocks reconstructed from assistant prose and `TurnMemory`. If a turn fails after native work has accumulated, temporary provider-native pending state is used for recovery; it is separate from and authoritative over the compact historical summary.

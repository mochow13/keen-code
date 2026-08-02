# Context Compaction

Keen Code supports two forms of context compaction:

- **Manual compaction** runs when the user enters `/compact [prompt]`.
- **Automatic compaction** runs inside an active provider tool loop when the next request approaches the configured context budget.

Both forms ask an LLM to summarize the conversation into a smaller continuation context. They differ in when they run, what the user sees, how cancellation works, and how the replacement is applied.

## Concepts

### Context budget

Keen estimates input size locally. The usable input budget is the model context window minus a safety margin:

```text
input budget = context window - max(4096 tokens, 5% of context window)
```

Automatic compaction is considered at 90% of that input budget:

```text
auto-compaction threshold = 90% of input budget
```

These are approximate token counts. The safety margin covers estimation error and provider-side framing overhead.

### Context reduction

Before a provider request is sent, Keen may replace older raw tool results with:

```text
Tool result removed to fit context.
```

This reducer is a final local guardrail. If the reduced request still exceeds the input budget, Keen may run automatic compaction once and retry local request preparation with the replacement history.

Provider-reported context-window errors do **not** trigger automatic compaction. If the provider rejects a request for context length, the request fails through the normal provider error path.

### Compaction history

Each provider keeps two histories during an active stream:

| History | Purpose |
|---|---|
| Provider-native history | The exact OpenAI, Anthropic, Genkit, or Bedrock messages sent to the model. |
| `compactionHistory` | Generic `[]llm.Message` history used only to estimate or build automatic compaction. |

The generic history includes transient tool activity from the active loop, including raw tool outputs. This lets the compactor preserve important discoveries that have not yet reached persisted turn memory. Raw outputs remain transient and are excluded from normal JSON persistence.

## Shared summary format

Manual and automatic compaction share the same summary schema:

```text
## Goal
User objectives.

## Key Instructions
Important user constraints.

## Discoveries
Relevant codebase facts and requirements.

## Accomplished
Completed and remaining work, active progress, and next action.

## Relevant Files
Relevant files, commands, errors, and tool results.
```

The shared prompt is built by `llm.BuildCompactionPrompt`. Automatic compaction adds an internal-checkpoint instruction through `llm.BuildAutoCompactionPrompt`.

Automatic summaries additionally:

- preserve active-loop progress and meaningful tool results;
- omit the latest user message from the summary because Keen retains it separately;
- contain no conversational preamble;
- remain private and are never streamed as normal assistant output.

## Manual compaction

Manual compaction is initiated explicitly:

```text
/compact
/compact Focus on the recent API changes
```

The optional argument guides what the summary should retain.

### Flow

```text
User enters /compact [prompt]
        |
        v
AppState snapshots persisted conversation history
        |
        v
Build manual compaction request
  - compaction system prompt
  - conversation snapshot
  - final summarize instruction
  - no tools
        |
        v
Stream summary visibly in the REPL
        |
        +---- Esc ----> cancel manual compaction
        |
        v
Validate non-empty summary
        |
        v
Replace AppState history with one RoleUser summary
        |
        v
Persist compaction_applied session event
```

### Request construction

`AppState.StreamCompact` sends:

1. a `RoleSystem` message containing `BuildCompactionPrompt(extraPrompt)`;
2. a clone of current AppState messages;
3. a final user instruction requesting the continuation summary.

The request has no tools. Unlike automatic compaction, the summary is rendered as a normal visible stream.

### Applying the result

`AppState.ApplyCompaction` validates that the summary is non-empty and replaces stored conversation history with:

```go
[]llm.Message{{
    Role:    llm.RoleUser,
    Content: summary,
}}
```

AppState does not persist the normal agent system prompt. On the next normal turn, `AppState.StreamChat` builds a fresh system prompt containing current project instructions, skills, subagents, memory, and agent mode, then prepends it to the compacted conversation.

### Manual cancellation and failure

While manual compaction runs:

- the loader displays `Compacting...`;
- `Esc` cancels the manual compaction stream;
- failure leaves the previous AppState history unchanged;
- queued input resumes after the manual compaction flow finishes or fails.

## Automatic compaction

Automatic compaction runs within an existing agent turn. It is currently implemented by all provider clients:

- OpenAI-compatible Chat Completions
- OpenAI Responses
- OpenAI Codex
- Anthropic
- Genkit
- Bedrock

### Trigger points

Each provider checks for automatic compaction before a model request in its tool loop.

A proactive attempt requires:

- the stream is not `OneShot`;
- `DisableAutoCompaction` is false;
- at least one new tool turn has completed;
- automatic compaction is not suppressed for the current unchanged history;
- no unreconciled provider-native pending state is being replayed where the provider requires that restriction;
- estimated input has reached 90% of the local input budget.

The provider then runs the reducer. If reduction cannot fit the request within the hard input budget, Keen may perform one local forced compaction attempt. This is still a pre-request decision; provider context-length errors do not start compaction.

### Provider loop

```text
Active agent tool loop
        |
        v
New tool turn completed?
        |
        +-- no --> skip proactive compaction
        |
        +-- yes --> estimate unreduced input
                         |
                         +-- below 90% --> continue
                         |
                         +-- at/above 90% --> try private compaction

Then for every request:
        |
        v
Reduce old tool results if needed
        |
        +-- fits --> send provider request
        |
        +-- does not fit --> one local forced compaction attempt
                                  |
                                  +-- applied --> rebuild native history and retry preparation
                                  |
                                  +-- unavailable/failed --> terminal local context error
```

### Private compaction request

`llm.AutoCompact` creates a nested request using the same provider client:

```go
client.StreamChat(ctx, request, nil, llm.StreamOptions{
    SessionID:             sessionID,
    OneShot:               true,
    DisableAutoCompaction: true,
})
```

The nested request:

- has no tools;
- is one-shot;
- disables automatic compaction to prevent recursive compaction;
- privately collects only assistant text and usage;
- rejects an empty summary;
- does not forward summary chunks, reasoning, or tool events to the parent stream.

### Replacement history

A successful automatic compaction builds a provider-facing replacement containing:

```text
original RoleSystem message(s)
RoleUser: <compacted_context>summary</compacted_context>
          <last_user_message>latest user message verbatim</last_user_message>
```

The latest user request remains authoritative and is retained verbatim outside the generated summary.

System handling differs by provider representation:

| Provider | After compaction |
|---|---|
| OpenAI Chat | Rebuilds OpenAI messages from the replacement, including system messages. |
| OpenAI Responses | Rebuilds Responses input from the replacement. |
| OpenAI Codex | Rebuilds `instructions` and input from the replacement. |
| Anthropic | Keeps existing `systemBlocks` and rebuilds conversation messages. |
| Genkit | Rebuilds Genkit messages from the replacement. |
| Bedrock | Keeps existing system blocks and rebuilds conversation messages. |

Tool definitions are not conversation messages. They remain outside the loop and are attached again to the next parent request. The private compaction request receives no tools.

### Transactional behavior

The provider mutates active history only after a valid summary exists. If proactive compaction is cancelled or fails:

- provider-native history remains unchanged;
- `compactionHistory` remains unchanged;
- the original parent request continues;
- another proactive attempt is suppressed until a new tool turn advances history.

A successful compaction replaces provider-native history and `compactionHistory`, clears stale pending state represented by the old native history, and resumes the same parent turn.

## Automatic lifecycle events

Providers report private lifecycle state to the caller:

| Event | Meaning |
|---|---|
| `auto_compaction_started` | Private summary request started; includes a child cancellation callback. |
| `auto_compaction_applied` | A valid replacement was installed; includes replacement messages and optional usage. |
| `auto_compaction_cancelled` | The child compaction context was cancelled. |
| `auto_compaction_failed` | Summary generation or validation failed. |

These are control events, not assistant content.

## Interactive REPL behavior

### Started

The REPL:

- marks compaction as automatic;
- changes the existing parent stream loader text to `Compacting...`;
- stores the child cancellation callback.

The parent stream spinner is already running, so no second spinner is started.

### Cancellation

During automatic compaction:

- `Esc` invokes only the child compaction cancellation callback;
- the parent agent stream remains active;
- `Ctrl+C` retains its normal parent-stream cancellation behavior.

Cancelled and failed automatic compactions restore the normal loader and continue the parent turn without replacing AppState.

### Applied

When the provider emits `auto_compaction_applied`, the REPL:

1. flushes pending rendering;
2. clones current parent-stream segments;
3. derives persisted tool activity and text offsets from those segments;
4. creates an assistant checkpoint for output already visible before compaction;
5. removes `RoleSystem` messages from the replacement before AppState/session storage;
6. atomically persists the checkpoint and `compaction_applied` event;
7. checkpoints the active stream handler so pre-compaction output is not emitted twice;
8. replaces AppState history with the system-free replacement;
9. starts fresh turn-memory accumulation for post-compaction activity;
10. restores the normal loader and shows `Context compacted automatically.`

The private summary is never rendered.

### Why system messages are removed at the AppState boundary

The active provider loop needs system messages in its replacement so it can immediately resume correctly. AppState has a different contract: it stores conversation history without the normal system prompt and rebuilds that prompt for every new user turn.

Therefore:

```text
Provider in-flight replacement:
  system message(s) + compacted user message

Persisted/AppState replacement:
  compacted user message only
```

This prevents duplicate system prompts on the next turn while preserving the system prompt inside the active provider loop.

## Headless behavior

Headless mode handles only the applied lifecycle boundary. Started, cancelled, and failed events produce no loader, notification, or private output.

On apply, headless mode:

1. records current stream segments and turn memory;
2. captures current assistant text;
3. strips system messages from the persisted replacement;
4. atomically persists the assistant checkpoint and compaction event;
5. appends pre-compaction assistant text to `completedText`;
6. replaces AppState history;
7. clears only current stream content, leaving the parent stream active;
8. continues collecting post-compaction assistant text.

Final text output is:

```text
completed pre-compaction assistant text + current post-compaction assistant text
```

No separator is inserted because the two strings are consecutive chunks of one logical assistant response. Reasoning, loader text, lifecycle status, and the private summary are not included.

If the resumed parent stream fails, headless mode returns and writes a partial result containing both checkpointed text and any post-checkpoint text while still returning the original error.

## Session persistence and replay

A successful automatic compaction records two adjacent events:

```text
assistant_turn checkpoint
compaction_applied replacement
```

They are written with `Store.AppendBatch`. The store assigns consecutive sequence numbers, writes the existing transcript plus both new JSONL records to a temporary file in the session directory, then renames it over the transcript. AppState changes only after this write succeeds.

This avoids an orphan durable checkpoint if the compaction replacement cannot be persisted.

Session projection handles `compaction_applied` by replacing reconstructed conversation history:

```go
messages = cloneMessages(event.CompactionApplied.Messages)
```

The earlier assistant checkpoint remains available in the transcript for UI replay and audit, while the reconstructed model conversation starts from the compacted replacement. Automatic compaction stores an empty status string; the interactive-only notification is not persisted as assistant content.

## Manual and automatic comparison

| Behavior | Manual `/compact` | Automatic compaction |
|---|---|---|
| Trigger | Explicit slash command | 90% proactive threshold or local hard-budget failure |
| Runs inside parent turn | No | Yes |
| Summary visibility | Visible | Private |
| Tools in summary request | None | None |
| Latest user message | Summarized with history | Retained verbatim outside summary |
| System prompt during summary | Dedicated compaction prompt | Dedicated automatic compaction prompt |
| Parent tools after compaction | Recreated on next normal turn | Preserved and reused immediately |
| `Esc` behavior | Cancels manual compaction | Cancels only child compactor |
| Replacement in AppState | One `RoleUser` summary | System-free automatic replacement |
| Session persistence | One `compaction_applied` event | Atomic checkpoint + `compaction_applied` events |
| Failure behavior | Previous history remains | Parent request continues unchanged after proactive failure |
| Provider context rejection | Normal error | Normal error; does not trigger compaction |

## Key implementation files

| Area | Files |
|---|---|
| Shared prompts and compactor | `internal/llm/systemprompt.go`, `internal/llm/auto_compaction.go` |
| Budgeting and context reduction | `internal/llm/context_reducer.go` |
| Lifecycle event contract | `internal/llm/message.go`, `internal/llm/client.go` |
| Provider loops | `internal/llm/openai.go`, `openai_responses.go`, `openai_codex.go`, `anthropic.go`, `genkit.go`, `bedrock.go` |
| Manual AppState flow | `internal/cli/repl/appstate/state.go`, `internal/cli/repl/command_handlers.go` |
| Interactive automatic handling | `internal/cli/repl/handlers.go`, `internal/cli/repl/stream_handler.go` |
| Headless handling | `internal/cli/repl/headless_run.go` |
| Session persistence/replay | `internal/cli/repl/session_state.go`, `internal/session/store.go`, `internal/session/projection.go` |

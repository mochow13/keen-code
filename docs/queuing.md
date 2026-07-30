# Input Queuing

When the agent is actively streaming a response, Keen Code queues certain inputs instead of silently dropping them. Queued messages are displayed in a preview band below the spinner and are automatically submitted one at a time as each turn completes.

## When Queuing Applies

Queuing is active when the agent is busy — either a main stream is in progress (`streamHandler.IsActive()`) or compaction is running (`isCompacting`).

## What Gets Queued

| Input | Queued? | Reason |
|-------|---------|--------|
| Normal text prompts | Yes | Conversational turns, naturally sequential |
| `/<skill-name> [args]` (enabled skill) | Yes | Transforms to an activation message, runs as a normal LLM turn |
| `/adversary [prompt]` | Yes | Runs as a separate stream after the current turn finishes |
| `/btw <question>` | No — executes immediately | Side-channel stream, works alongside the main stream |
| `/emptyq` | No — executes immediately | Clears the queue (see below) |
| `/clear`, `/model`, `/compact`, `/help`, etc. | No | Immediate side-effects on REPL state; deferring is semantically wrong |
| `/nonexistent` (unknown skill/command) | No | Not a recognized skill or command |
| `!shell` commands | No | Shell execution, immediate side-effects |

The queue is capped at **5 items**. When the queue is full, additional queueable inputs are rejected with a "Queue is full" notification.

## Queue Preview

Queued messages are rendered between the spinner and the input area:

```text
   ⠋ working...
   ▎ queue  fix the off-by-one in parser
   ▎ queue  also add a test for it
┌─ ▎build ────────────────────────────┐
│  > |                                  │
└──────────────────────────────────────┘
```

- Each item shows a `queue` chip with a dimmed message body
- Multi-line messages display only the first line in the preview
- Long messages are truncated with an ellipsis to fit the terminal width
- The textarea remains free for new input while items are queued

## Draining the Queue

Queued items are submitted **one per turn** — not all at once. After the current stream completes successfully, the first queued item is popped and submitted through the normal input path. Remaining items stay in the queue for subsequent turns.

### What triggers a drain

| Event | Drains queue? |
|-------|---------------|
| Stream completed successfully (`handleLLMDone`) | Yes |
| Stream ended with incomplete response (`handleLLMIncomplete`) | Yes |
| Compaction completed (`handleCompactionDone`) | Yes |
| Compaction failed (`handleCompactionError`) | Yes |
| Stream interrupted via Ctrl+C/Esc | Yes |
| Stream failed with an error (`handleLLMError`) | **No** |

A user-initiated interruption immediately submits the next queued message. Stream errors do not drain the queue, so the user can inspect the error before deciding whether to continue.

## Clearing the Queue

There are two ways to clear the queue:

### `/emptyq` command

Works at any time, including while the agent is actively streaming. Clears all queued items and shows a "Queue cleared" notification. If the queue is already empty, shows "Queue is empty" instead.

### Ctrl+C / Esc while idle

When the agent is NOT actively streaming and the queue has items, pressing Ctrl+C or Esc clears the queue and shows a "Queue cleared" notification. If the agent IS streaming, Ctrl+C/Esc interrupts the stream and immediately submits the first queued item. Any remaining items stay queued for subsequent turns.

## Notifications

Notifications appear beside the loading text (or below the textarea when idle) in accent style for 2 seconds:

| Trigger | Message |
|---------|---------|
| Known slash command while busy (e.g. `/clear`) | `Operation not permitted` |
| Unknown `/`-prefixed input while busy | `No such skill found` |
| `!shell` while busy | `Operation not permitted` |
| `/adversary` while busy (queue full) | `Queue is full` |
| Queue full | `Queue is full` |
| Queue cleared via `/emptyq` or Ctrl+C/Esc | `Queue cleared` |
| `/emptyq` with empty queue | `Queue is empty` |

## Implementation

Queue logic lives in `internal/cli/repl/repl_queue.go`:

- `isQueueable(input)` — determines if an input can be queued
- `drainQueuedInput()` — pops the first item and submits it
- `queuedHeight()` — computes viewport height adjustment for the preview band
- `renderQueuedInputs()` — renders the preview band

The `queuedInputs []string` field on `replModel` holds the queue state. The `maxQueuedInputs` constant (currently 5) sets the cap.

Queue-related styles (`QueueItemStyle`, `QueueChipStyle`) are defined in `internal/cli/repl/theme/styles.go`.

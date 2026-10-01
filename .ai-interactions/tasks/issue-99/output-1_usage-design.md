# Design: `/usage` command — usage breakdown by model

Issue: https://github.com/mochow13/keen-code/issues/99

Implement a `/usage` command that shows a table of token usage per model, with
`←`/`→` to switch between All time / Last 7 days / Last 30 days and `Esc` to close.

## Requirements (from the issue)

- Each used model has a row.
- Each row shows input and output token counts plus KV cache read and write token counts.
- Rows sorted by the most-used model in terms of input count.
- `←`/`→` cycle between past 7 days, past 30 days, and all time, so usage must be
  stored to support all three windows.
- `Esc` to go back.

## Decisions (confirmed)

- **Scope:** record every provider call — main turn, manual `/compact`,
  auto-compaction, `/btw`, `/adversary`, and subagents.
- **Storage:** new global append-only ledger at `~/.keen/usage/usage.jsonl`.
  Session history is not used: it is namespaced per working directory and pruned
  after 14 days (`internal/cleanup/cleanup.go:19`), which breaks "all time" and
  cross-project totals.
- **Retention / rollup:** raw per-call records kept for 30 days; older records are
  rolled up to one record per **(UTC day, provider, model)**. Compaction runs
  **only when `/usage` is opened** (throttled to at most once per day) — not in
  `/cleanup`.
- **Cache read/write:** currently merged into a single `CachedTokens`; split into
  read and write so cache read and cache write can be shown separately. OpenAI-family
  providers only report cache reads, so their cache-write column renders `-`.

## Current state (findings)

- `core.TokenUsage` (`internal/llm/core/message.go:134`) has a single `CachedTokens`
  field: `Input, Output, Total, Reasoning, Cached`.
- Providers collapse read+write into it:
  - Anthropic (`internal/llm/anthropic.go:349`): `CacheCreationInputTokens + CacheReadInputTokens`.
  - Bedrock (`internal/llm/bedrock.go:642`): `CacheReadInputTokens + CacheWriteInputTokens`.
  - OpenAI (`internal/llm/openai.go:567`), Responses (`internal/llm/openai_responses.go:322`),
    Codex (`internal/llm/openai_codex.go:171`): only `cached_tokens` (read).
  - Genkit (`internal/llm/genkit.go:305`): no cache fields.
- One `usage` event is emitted per provider response (`core.StreamEventTypeUsage`), not
  per turn. A multi-iteration tool loop emits several, so recording per event is the
  most accurate for billing and survives interrupted turns.
- Usage event sinks today: main turn → `llmUsageMsg` → `handleLLMUsage`
  (`internal/cli/repl/handlers.go:37`); headless `keen run` (`internal/cli/repl/headless_run.go:145`);
  auto-compaction usage is on `core.AutoCompactionEvent.Usage` (`message.go:145`).
  `/btw` (`StreamBtw`) and `/adversary` (`StreamAdversary`) ignore `StreamEventTypeUsage`.
  Subagents stream through `collectResult` (`internal/subagents/activity.go:18`).
- Modal widget precedent to copy: session picker
  (`internal/cli/repl/widgets/session_picker.go`), wiring at
  `handlers.go:535` (`handleKeyMsg`), `repl.go:547` (render), `repl.go:1012` (clear).
- Existing helpers to reuse: `formatCompactTokens` (`internal/cli/repl/context_status.go:118`),
  `addCommandTable` / `maxColumnWidth` (`command_handlers.go`), `ModelSelection*` styles
  and `RuleStyle` (`internal/cli/repl/theme/styles.go`).

## Design

### 1. Split cache read/write in the token model

Add `CacheReadTokens` and `CacheWriteTokens` to `core.TokenUsage`; keep
`CachedTokens = CacheReadTokens + CacheWriteTokens` so existing context-status math
is unchanged.

| Provider | read | write |
|---|---|---|
| Anthropic | `CacheReadInputTokens` | `CacheCreationInputTokens` |
| Bedrock | `CacheReadInputTokens` | `CacheWriteInputTokens` |
| OpenAI / Responses / Codex | `prompt_tokens_details.cached_tokens` | 0 |
| Genkit | 0 | 0 |

Also update `cloneHeadlessUsage` (`headless_run.go:316`) to carry the new fields.

### 2. Ledger (`internal/usage`)

Append-only JSONL at `~/.keen/usage/usage.jsonl`, one record per provider response:

```json
{"ts":"2026-09-10T18:22:04Z","provider":"anthropic","model":"claude-sonnet-4-5","input":18432,"output":512,"cache_read":16000,"cache_write":1024,"reasoning":0}
```

Rolled-up record (one per UTC day + provider + model), `date` instead of `ts`:

```json
{"date":"2026-08-14","provider":"anthropic","model":"claude-sonnet-4-5","input":90210,"output":4410,"cache_read":77120,"cache_write":3072,"reasoning":0,"rollup":true}
```

API surface:

- `type Record struct { TS time.Time; Provider, Model string; Input, Output, CacheRead, CacheWrite, Reasoning int; Rollup bool }`
  with `Date string` used for rollups (mutually exclusive with `TS`).
- `Store{path string}`:
  - `Append(rec Record) error` — `O_APPEND|O_CREATE|O_WRONLY`, single line write,
    `mkdir -p` parent. Errors are logged, never fatal to a turn.
  - `Load() ([]Record, error)` — tolerant of corrupt/truncated lines (skip and continue).
  - `Compact(now time.Time) error` — see below.
- `Summarize(records []Record, since time.Time) Summary` — group by `provider/model`,
  sum fields, sort by input tokens desc, append a `Total` row. `since.IsZero()` means
  all time. Reads raw and rolled-up records identically.
- `type Range int` with `RangeAllTime`, `RangeLast7Days`, `RangeLast30Days` and
  `rangeSince(Range, now) time.Time`.
- `type ModelUsage struct { Provider, Model string; Input, Output, CacheRead, CacheWrite, Reasoning int }`
  and `type Summary struct { Rows []ModelUsage; Total ModelUsage }`.

**Rollup compaction.** `Compact`:

1. Cheap lock via `O_CREATE|O_EXCL` on `usage.lock`, with stale-lock recovery by mtime
   (e.g. > 5 min old → remove and retry). No flock dependency, so it stays portable.
2. Load all records. Split into `old` (`ts`/`date` strictly before `now - 30d`) and `keep`.
   Rollups are day-granular, so a rollup day is always strictly outside the 30-day window
   and never contaminates the 7d/30d filters.
3. Group `old` by (UTC day, provider, model) and merge with any existing rollup records in
   `keep` for the same key (idempotent re-runs).
4. Rewrite the file atomically (temp file + rename) with rollups first, then `keep`,
   preserving append-only semantics for future writes.
5. Remove the lock.

Throttle: a marker file `~/.keen/usage/.compacted` holding the last compaction date;
`MaybeCompact(now)` runs at most once per calendar day and is called from the
`/usage` open path only.

Bounded size ≈ `models × days` rollup lines + 30 days of raw turns.

### 3. Capture points

Add one repl helper:

```go
func (m *replModel) recordUsage(provider, model string, u *core.TokenUsage)
```

which is a no-op on nil usage and appends to the ledger. Each site becomes a one-liner:

- **Main turn** — `handleLLMUsage` (`handlers.go:37`); provider/model from `m.ctx.cfg`.
- **Headless `keen run`** — `headless_run.go:145`; same cfg.
- **Manual `/compact`** — usage events already flow through `llmUsageMsg`; covered by the
  main-turn path.
- **Auto-compaction** — handle `AutoCompactionEvent.Usage` in `llmAutoCompactionAppliedMsg`.
- **`/btw`** — add a `StreamEventTypeUsage` case in `handleBtwStreamMsg` (`m.ctx.cfg`).
- **`/adversary`** — add a `StreamEventTypeUsage` case in `handleAdversaryStreamMsg`
  (adversary cfg provider/model).
- **Subagents** — add `Usage chan<- usage.Record` to `subagents.Runner` next to the
  existing `Activity` channel (`internal/subagents/activity.go:33`); emit from
  `collectResult` on `StreamEventTypeUsage` using the profile's own provider/model from
  `Runner.resolvedConfig`. The repl drains it like `subagentActivity`.

### 4. UI

Modal overlay modeled on the session picker. Load the ledger once on open, precompute all
three windows, then `←`/`→` just swap an index.

```
  ────────────────────────────────────────────────────────────────────────
  Usage Data

   All time    Last 7 days    Last 30 days
  ────────────────────────────────────────────────────────────────────────
  Model                    Input     Output    Cache read   Cache write
  claude-sonnet-4-5        1.24M     342k      980k         120k
  gpt-5                    412k      88k       210k         -
  gemini-2.5-pro           34k       9k        -            -
  ────────────────────────────────────────────────────────────────────────
  Total                    1.69M     439k      1.19M        120k

  ←/→ change window   Esc close
  ────────────────────────────────────────────────────────────────────────
```

- Active tab bold + primary color, inactive tabs muted/faint; default **All time**;
  `←`/`→` wrap around.
- Rows sorted by input tokens desc (per the issue). `Total` row bold, same styling
  approach as `/context` (`context_status.go:188`).
- Numbers compacted with `formatCompactTokens`; `-` for zero/unsupported cache columns.
- Empty state: `No usage recorded yet.` Footer hint reflects the active window.
- Same card chrome (dim rule lines, `ModelSelection*` styles) as `/sessions` and `/model`
  so it reads as one family.

### 5. Wiring

- `internal/cli/repl/widgets/usage_view.go` + test — `UsageView{rangeIndex int, summaries [3]usage.Summary}`
  with `NextRange()`, `PrevRange()`, `CurrentSummary()`, `FormatUsageCard(view, width)`.
- `internal/cli/repl/commands/commands.go` — add `Usage = "/usage"` to `All` and `Suggestions`.
- `internal/cli/repl/command_handlers.go` — `case input == replcommands.Usage:` calls
  `usage.MaybeCompact(now)` then `m.startUsageView()`.
- `internal/cli/repl/handlers.go` — gate keys in `handleKeyMsg` (`:535`): `left`/`right`
  cycle the window, `esc` closes; add `keyLeft`/`keyRight` constants.
- `internal/cli/repl/repl.go` — render in `updateViewportContent` (`:547`); clear in
  `handleClearCommand` (`:1012`).
- `internal/cli/repl/repl_helpers.go` — `formatUsageCard` wrapper if needed.
- `docs/cli-usage.md` — command table row, `/usage` section with key table and a note on
  the ledger + 30-day rollup.

Note: `/cleanup` (`internal/cleanup/cleanup.go`) is intentionally **not** changed.

## File-by-file

| File | Change |
|---|---|
| `internal/llm/core/message.go` | `CacheReadTokens` / `CacheWriteTokens` fields |
| `internal/llm/{anthropic,bedrock,openai,openai_responses,openai_codex,genkit}.go` | populate read/write |
| `internal/llm/auto_compaction.go` | pass usage through unchanged (verify) |
| `internal/usage/store.go` | `Record`, `Store.Append/Load/MaybeCompact/Compact`, lock |
| `internal/usage/summary.go` | `Range`, `Summarize`, `ModelUsage`, `Summary` |
| `internal/usage/*_test.go` | new tests |
| `internal/subagents/{activity.go,runner.go}` | usage channel + emit |
| `internal/cli/repl/handlers.go` | `recordUsage`, main/btw/adversary/auto-compaction capture, key gating |
| `internal/cli/repl/headless_run.go` | record usage + carry new cache fields |
| `internal/cli/repl/widgets/usage_view.go` (+test) | card renderer |
| `internal/cli/repl/commands/commands.go` | register `/usage` |
| `internal/cli/repl/command_handlers.go` | dispatch + `startUsageView` |
| `internal/cli/repl/repl.go` | render + clear |
| `docs/cli-usage.md` | docs |

## Tests

- `internal/usage`: append/load roundtrip; corrupt-line tolerance; `Summarize` ordering
  and window filtering; `Compact` idempotency; the 30-day boundary (raw ↔ rollup);
  lock contention / stale-lock recovery; `MaybeCompact` once-per-day throttle.
- Providers: each maps cache read/write correctly into `TokenUsage`.
- Widget: table render per window, tab wrapping, empty state.
- Keys: `←`/`→` cycle the window, `Esc` closes, unrelated keys are swallowed.
- Capture: main turn, `/btw`, `/adversary`, auto-compaction, subagent each append a record.

## Open items

- Zero/unsupported cache columns render `-`.
- Cache write is only reported by Anthropic/Bedrock; OpenAI-family rows show `-` there.

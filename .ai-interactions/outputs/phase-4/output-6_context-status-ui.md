# Context Status UI — Implementation Plan

## Scope

Add a context status UI at the bottom-right area under the input textarea in the REPL, with:

- A horizontal progress bar
- A percentage indicator
- Real-time updates as conversation grows
- Real-time updates when model changes

Status formula:

- `usage_percent = estimated_context_tokens / model_context_window * 100`
- Estimation rule (per requirement): `estimated_tokens = words / 0.75` (equivalent to `~1.333 tokens/word`)

## Web Research Snapshot (for `context_window`)

Only models currently present in `providers/registry.yaml` are included below.

| Provider | Model ID in registry | Context Window | Source |
|---|---|---:|---|
| Anthropic | `claude-opus-4-6` | 1,000,000 | [Anthropic models overview](https://platform.claude.com/docs/en/about-claude/models/overview) |
| Anthropic | `claude-sonnet-4-6` | 1,000,000 | [Anthropic models overview](https://platform.claude.com/docs/en/about-claude/models/overview) |
| Anthropic | `claude-haiku-4-5` | 200,000 | [Anthropic models overview](https://platform.claude.com/docs/en/about-claude/models/overview) |
| OpenAI | `gpt-5.4` | 1,050,000 | [OpenAI GPT-5.4 model doc](https://developers.openai.com/api/docs/models/gpt-5.4) |
| OpenAI | `gpt-5.4-pro` | 1,050,000 | [OpenAI GPT-5.4 pro model doc](https://developers.openai.com/api/docs/models/gpt-5.4-pro) |
| OpenAI | `gpt-5.3-codex` | 400,000 | [OpenAI GPT-5.3-Codex model doc](https://developers.openai.com/api/docs/models/gpt-5.3-codex) |
| Google AI | `gemini-3.1-pro-preview` | 1,048,576 | [Gemini 3.1 Pro Preview](https://ai.google.dev/gemini-api/docs/models/gemini-3.1-pro-preview) |
| Google AI | `gemini-3-flash-preview` | 1,048,576 | [Gemini 3 Flash Preview](https://ai.google.dev/gemini-api/docs/models/gemini-3-flash-preview) |
| Moonshot AI | `kimi-k2-thinking` | 256,000 | [Kimi K2.5 Quickstart](https://platform.moonshot.ai/docs/guide/kimi-k2-5-quickstart) |
| Moonshot AI | `kimi-k2-thinking-turbo` | 256,000 | [Kimi K2.5 Quickstart](https://platform.moonshot.ai/docs/guide/kimi-k2-5-quickstart) |
| Moonshot AI | `kimi-k2.5` | 256,000 | [Kimi K2.5 Quickstart](https://platform.moonshot.ai/docs/guide/kimi-k2-5-quickstart) |
| DeepSeek | `deepseek-chat` | 128,000 | [DeepSeek models & pricing](https://api-docs.deepseek.com/quick_start/pricing) |
| DeepSeek | `deepseek-reasoner` | 128,000 | [DeepSeek models & pricing](https://api-docs.deepseek.com/quick_start/pricing) |

Notes:

- Removed previously listed non-registry models (for example `gpt-5.2-codex`, `gemini-2.5-*`) to align with Keen’s current supported set.
- `gemini-3-pro-preview` is deprecated in Google docs (shutdown date: March 9, 2026); keep only for current Keen compatibility unless/until removed from registry.

## Architecture Changes

### Data layer (provider registry)

Add `context_window` as a required numeric field per model in `providers/registry.yaml`.

Update provider model struct:

- File: `providers/loader.go`
- Add:
  - `ContextWindow int \`yaml:"context_window"\``

Add a helper lookup API:

- `(r *Registry) GetModelContextWindow(providerID, modelID string) (int, bool)`

### Context estimation

Create a new small utility in REPL package:

- File: `internal/cli/repl/context_status.go`
- Responsibilities:
  - Count words in current conversation text
  - Convert words to estimated tokens using `words / 0.75`
  - Clamp percent to `[0, 100]`
  - Return a small view model:
    - `CurrentTokens int`
    - `ContextWindow int`
    - `Percent int`

Conversation text sources:

- System prompt from `llm.Build(workingDir)`
- `AppState` message history
- In-flight assistant stream text from `streamHandler.GetResponse()` for real-time updates during streaming

### UI rendering

Add a dedicated context status renderer:

- File: `internal/cli/repl/context_status.go`
- Rendering output:
  - Progress bar (fixed width, e.g. 20 cells)
  - Percentage label (e.g. `63%`)
  - Theme-aligned colors from `styles.go`

Add styles:

- File: `internal/cli/repl/styles.go`
- New styles:
  - `contextStatusLabelStyle`
  - `contextBarEmptyStyle`
  - `contextBarFillStyle`
  - Optional warning styles for high usage thresholds (`>=80%`, `>=95%`)

### REPL layout integration

Target placement: bottom row under textarea, right side.

- File: `internal/cli/repl/repl.go`
- Change `inputMetaView()` to return one composed row:
  - Left: existing provider/model text
  - Right: context status UI
- Keep in single row using available width (truncate left side if width is constrained).

### Real-time update hooks

Ensure context status is recomputed on:

- Every user message submit
- Streaming chunks (assistant partial response growth)
- Stream completion
- Model selection completion (`/model`)
- Window resize

This can be done by computing status inside `View()` or `inputMetaView()` from current state, so no extra mutable state is required.

## Granular Todo List

### 1. Registry schema and mapping

- [ ] Add `context_window` to all models in `providers/registry.yaml`.
- [ ] Use the web-researched values above where exact.
- [ ] Add/update tests in `providers/loader_test.go` to verify non-zero `ContextWindow` parsing.

### 2. Provider lookup API

- [ ] Extend `providers.Model` struct with `ContextWindow`.
- [ ] Add `GetModelContextWindow(providerID, modelID)` helper in `providers/loader.go`.
- [ ] Add unit tests for lookup success/failure scenarios.

### 3. Context estimator utility

- [ ] Create `internal/cli/repl/context_status.go`.
- [ ] Implement word counting helper.
- [ ] Implement token estimation `ceil(words / 0.75)`.
- [ ] Implement percent calculation against model context window.
- [ ] Include safe fallback when context window is unknown (`0%`, hidden bar, or `N/A` per final decision).

### 4. Context status rendering

- [ ] Implement progress bar rendering helper (`[██████░░░░░░]` style, lipgloss styled).
- [ ] Implement right-aligned compact label (e.g. `63%`).
- [ ] Add warning color thresholds:
  - [ ] Normal `<80%`
  - [ ] Warning `80-94%`
  - [ ] Critical `>=95%`

### 5. Integrate in bottom metadata row

- [ ] Update `inputMetaView()` in `internal/cli/repl/repl.go` to compose left and right sections.
- [ ] Keep existing Provider/Model metadata unchanged on left.
- [ ] Add context status on right under input area as required.
- [ ] Handle narrow terminals gracefully (fallback to minimal `Ctx 63%`).

### 6. Real-time behavior

- [ ] Include in-flight stream text in current context estimation during active generation.
- [ ] Verify the percentage changes while chunks arrive.
- [ ] Verify percentage immediately changes after `/model` selection completes.

### 7. Tests

- [ ] Add `internal/cli/repl/context_status_test.go`:
  - [ ] Word count and token estimation.
  - [ ] Percent clamping and rounding.
  - [ ] Unknown model context window behavior.
  - [ ] Progress bar fill calculation.
- [ ] Add/extend `internal/cli/repl/repl_test.go`:
  - [ ] `inputMetaView()` contains context percent.
  - [ ] Model change updates percent denominator.
  - [ ] Streaming partial output affects displayed percentage.

### 8. Manual validation

- [ ] Start REPL and confirm status appears bottom-right under textarea.
- [ ] Send multiple prompts and confirm percentage grows.
- [ ] Switch models with very different context windows and confirm percentage recalculates immediately.
- [ ] Verify style consistency across light/dark terminals.

### 9. Final checks

- [ ] Run `go test ./...`.
- [ ] Verify no lingering model IDs without `context_window`.
- [ ] Keep commit message concise with bullet points per repository guideline.

## Open Decisions (to lock before implementation)

- Display when context window is unknown:
  - Option A: show `N/A` and empty bar
  - Option B: hide context status entirely
- Rounding for percentage:
  - Option A: nearest integer
  - Option B: floor (less jumpy)
- Whether to include assistant partial stream text in live estimate (recommended: yes).

# OpenAI-Compatible Client Plan For DeepSeek

## Goal

Replace the Genkit-backed path for DeepSeek with a first-party `OpenAICompatibleClient` that implements `LLMClient`. Use that client for:

- DeepSeek chat
- DeepSeek reasoner

The key requirement is support for `deepseek-reasoner` tool loops by preserving and replaying `reasoning_content` on assistant tool-call messages.

## Why This Change Is Needed

The current DeepSeek integration goes through Genkit's `compat_oai` adapter. That adapter reduces assistant history to plain text plus tool calls and does not preserve `reasoning_content`.

Relevant code:

- `internal/llm/genkit.go`
- `internal/llm/message.go`
- Genkit compat adapter:
  `go/pkg/mod/github.com/firebase/genkit/go@v1.4.0/plugins/compat_oai/generate.go`

DeepSeek reasoner requires prior assistant tool-call messages to include `reasoning_content`, so the second request in a tool loop fails if that field is missing.

## High-Level Approach

Introduce a new `OpenAICompatibleClient` in `internal/llm` and use it only for DeepSeek for now. Keep the existing Genkit path for all other providers.

Target split:

- `GenkitClient`
  - Anthropic
  - Google AI
  - OpenAI
  - Moonshot AI
- `OpenAICompatibleClient`
  - DeepSeek

This keeps the first pass narrow while still establishing the right client abstraction for DeepSeek.

## Design Overview

### 1. Add a dedicated provider-scoped OpenAI-compatible client

Create:

- `internal/llm/openai.go`

Add a new client type:

```go
type OpenAICompatibleClient struct {
	provider Provider
	model    string
	apiKey   string
	baseURL  string
	client   *openai.Client
}
```

Use `openai-go` for transport with the DeepSeek base URL:

- DeepSeek: `https://api.deepseek.com/`

The client will own:

- streaming request execution
- tool loop execution
- assistant/tool history replay
- provider-specific request shaping

### 2. Refactor client construction

Current construction is centered around `internal/llm/genkit.go` and the function name `NewGenkitClient`.

Refactor to:

- keep `NewGenkitClient` for Genkit-backed providers
- add `NewOpenAICompatibleClient` for `OpenAICompatibleClient`
- select concrete implementation by provider at the call site or in a small provider-selection helper

Recommended routing:

- `anthropic` -> `NewGenkitClient(...)`
- `googleai` -> `NewGenkitClient(...)`
- `openai` -> `NewGenkitClient(...)`
- `moonshotai` -> `NewGenkitClient(...)`
- `deepseek` -> `NewOpenAICompatibleClient(...)`

This keeps the public abstraction unchanged while limiting the new client to DeepSeek.

### 3. Keep the shared message model unchanged for now

The current message type in `internal/llm/message.go` only stores plain text:

```go
type Message struct {
	Role    Role
	Content string
}
```

For the current DeepSeek bug, that is acceptable because the failure happens inside the active tool loop within one `StreamChat` call.

Plan:

- keep `llm.Message` unchanged for now
- keep `AppState.messages` unchanged for now
- store provider-specific structured state only inside `OpenAICompatibleClient` during the active request

The in-memory SDK request history inside `OpenAICompatibleClient` should preserve:

- assistant text content
- assistant `reasoning_content`
- assistant tool calls
- tool response messages

This is enough to continue the active DeepSeek tool loop without expanding persisted app history.

### 4. Keep app state persistence unchanged for now

Current REPL state persistence only stores final assistant text:

- `internal/cli/repl/state.go`
- `internal/cli/repl/handlers.go`

That still loses intermediate assistant/tool state after the request finishes, but that is acceptable for this phase because we are only fixing the active DeepSeek tool loop.

Plan:

- keep `AddMessage(role, content)` unchanged
- keep REPL persistence behavior unchanged
- revisit structured persisted history only if future-turn replay becomes a requirement

### 5. Use SDK types first inside `OpenAICompatibleClient`

Do not scatter provider-specific behavior through the REPL or shared Genkit path. Keep the request shaping inside `OpenAICompatibleClient`.

Recommended helpers:

- `buildRequestMessages([]Message) ([]openai.ChatCompletionMessageParamUnion, error)`
- `buildTools(*tools.Registry) ([]any, error)`
- `providerBaseURLOptions() []option.RequestOption`
- `accumulateStream(...)`
- `extractReasoningContent(...)`
- `injectReasoningContent(...)`

Use typed `openai-go` request/response models wherever possible. Only use SDK escape hatches for the DeepSeek-specific field that the SDK does not model directly.

The shared flow should stay focused on DeepSeek chat and DeepSeek reasoner. The only non-standard part is `reasoning_content`.

### 6. Add provider-specific assistant serialization

For DeepSeek:

- `deepseek-chat` should behave like a standard chat model over the same client
- `deepseek-reasoner` must include `reasoning_content` when replaying assistant tool-call turns

Required replay shape for reasoner:

```json
{
  "role": "assistant",
  "content": "...",
  "reasoning_content": "...",
  "tool_calls": [...]
}
```

Because `openai-go@v1.8.2` does not model `reasoning_content` as a first-class field, use the SDK's undocumented-field support rather than replacing the message model.

Recommended request approach:

- build assistant/tool messages with normal `openai-go` types
- inject `reasoning_content` with `SetExtraFields(...)` on assistant message params
- use `option.WithJSONSet(...)` only as a fallback if nested extra fields become awkward

Recommended response approach:

- use normal streamed chunk types from `openai-go`
- read DeepSeek-specific fields from `chunk.Choices[0].Delta.JSON.ExtraFields`
- read final-message DeepSeek-specific fields from `choice.Message.JSON.ExtraFields` when available

Important caveat:

- `openai-go`'s `ChatCompletionAccumulator` does not accumulate `JSON` metadata
- so `reasoning_content` must be captured while processing stream chunks, not only from the final accumulated response

### 7. Implement the provider stream loop in `OpenAICompatibleClient`

The new client should mirror the current `GenkitClient.StreamChat` behavior:

1. build wire messages from internal history
2. send streaming request
3. emit text chunks as `StreamEventTypeChunk`
4. accumulate final assistant state during streaming:
   - content
   - reasoning content, if present
   - tool calls
5. if no tool calls, emit done
6. if tool calls exist:
   - emit `tool_start`
   - execute tools via the shared registry
   - emit `tool_end`
   - append assistant tool-call message to in-memory request history
   - append tool result messages to in-memory request history
   - repeat

This loop only needs to support DeepSeek right now. The important provider-specific behavior is how messages are encoded and how `reasoning_content` is parsed and replayed.

### 8. Keep `StreamEvent` and REPL behavior unchanged for now

Current `StreamEvent` is sufficient for this phase:

- text chunks
- errors
- UI-facing tool events

Do not expand `StreamEvent` yet. The client can keep the richer DeepSeek state internally and continue emitting the same external events.

### 9. Keep the REPL UI unchanged

No immediate UI work is required. The REPL should continue to show:

- streamed text output
- tool start/end lines

Persistence changes are not required in this phase.

Files impacted:

- none required for the first pass outside existing client construction call sites

## Concrete Task Breakdown

### Phase 1: Client split

1. Keep `NewGenkitClient` for Genkit-backed providers.
2. Add `NewOpenAICompatibleClient` returning `*OpenAICompatibleClient`.
3. Ensure both concrete clients implement `LLMClient`.
4. Route only `deepseek` to `NewOpenAICompatibleClient`.
5. Keep `anthropic`, `googleai`, `openai`, and `moonshotai` on `NewGenkitClient`.

### Phase 2: SDK-based request history

1. Build request history as `[]openai.ChatCompletionMessageParamUnion`.
2. Build assistant and tool messages with normal SDK types.
3. Keep all assistant tool-call and tool-response state in that in-memory request history during the active request.

### Phase 3: DeepSeek request building

1. Build DeepSeek request messages from plain `[]llm.Message`.
2. Add model-specific assistant serialization hooks for:
   - `deepseek-chat`
   - `deepseek-reasoner`
3. Ensure DeepSeek reasoner assistant replay includes `reasoning_content` via `SetExtraFields(...)`.

### Phase 4: DeepSeek stream loop

1. Implement streaming request handling in `OpenAICompatibleClient`.
2. Accumulate:
   - visible text
   - tool calls
   - provider-specific extra fields such as `reasoning_content`
3. Capture `reasoning_content` while processing chunks because the SDK accumulator drops JSON metadata.

### Phase 5: Tool loop replay

1. Reuse the existing tool execution behavior conceptually.
2. Append assistant tool-call messages with all fields needed for replay into the in-memory SDK request history.
3. Append tool result messages with correct `tool_call_id` into the in-memory SDK request history.
4. Continue looping until the model stops or `maxToolTurns` is reached.

### Phase 6: Keep REPL persistence unchanged

1. Leave `AppState.messages` unchanged.
2. Leave `handleLLMDone` unchanged.
3. Verify the UI output still works across:
   - plain response
   - tool call
   - multiple tool rounds
   - interrupted stream

### Phase 7: Cleanup

1. Remove DeepSeek initialization from `internal/llm/genkit.go`.
2. Keep Genkit handling Anthropic, Google AI, OpenAI, and Moonshot for now.
3. Keep provider wire logic isolated to `openai.go`.

## Testing Plan

Add unit tests for the new client first, then run the existing LLM and REPL suites.

### New tests

Create:

- `internal/llm/openai_test.go`

Test cases:

1. DeepSeek chat request builder
   - standard assistant/tool history serializes correctly

2. DeepSeek reasoner request builder
   - assistant tool-call replay includes `reasoning_content`

3. response accumulation
   - streamed text chunks are emitted
   - `reasoning_content` is captured from streamed extra fields before accumulator metadata is lost

4. single tool call loop
   - assistant tool-call message is appended to in-memory SDK request history with IDs and arguments
   - tool response includes `tool_call_id`

5. multi-turn tool loop
   - second request replays the first assistant tool-call message exactly

6. tool error handling
   - tool errors become valid tool-response messages

### Updated tests

Update:

- `internal/llm/genkit_test.go`

Focus on preserving current behavior for non-DeepSeek providers.

### Verification commands

Run after each stage:

```bash
go test ./internal/llm/...
go test ./internal/cli/repl/...
go test ./...
```

## Risks and Mitigations

### Risk: `openai-go` does not expose `reasoning_content`

Mitigation:

- use `SetExtraFields(...)` to send `reasoning_content` on assistant messages
- use `JSON.ExtraFields` to read DeepSeek response fields
- use `option.WithJSONSet(...)` only as a fallback
- capture streamed `reasoning_content` before the SDK accumulator drops JSON metadata

### Risk: in-memory SDK request history diverges from persisted app history

Mitigation:

- accept that limitation for this phase
- keep the scope focused on the active tool loop only
- revisit persisted structured history only if future-turn replay becomes necessary

### Risk: duplicated logic between `GenkitClient` and `OpenAICompatibleClient`

Mitigation:

- accept some duplication at first
- only extract shared helpers after the new client is stable

## Recommended Implementation Order

1. Keep `NewGenkitClient` for existing Genkit-backed providers.
2. Add `NewOpenAICompatibleClient` for DeepSeek.
3. Implement SDK-based request history inside `OpenAICompatibleClient`.
4. Implement `OpenAICompatibleClient` request building for DeepSeek chat and DeepSeek reasoner.
5. Add DeepSeek chat support on the new client.
6. Add DeepSeek reasoner parsing and `reasoning_content` replay.
7. Keep OpenAI and Moonshot on Genkit unchanged.
8. Run full tests and do manual smoke tests for:
   - DeepSeek chat
   - DeepSeek reasoner

## Definition of Done

The work is complete when:

- DeepSeek chat works through `OpenAICompatibleClient`
- DeepSeek reasoner can complete at least one tool-call round trip without the `reasoning_content` 400 error
- OpenAI and Moonshot continue to behave as they do today
- Anthropic and Google AI continue to pass through `GenkitClient`
- the active DeepSeek tool loop retains enough in-memory SDK request history to replay assistant tool-call messages correctly

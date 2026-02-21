# Phase-3 Tool Design Review

Review of `output-1_design-tools.md` against the current codebase and Genkit Go SDK v1.4.0.

---

## Critical Issues

### 1. Adapter Code Doesn't Match Genkit's Actual API

The adapter in Section 3 shows:

```go
func ToGenkitTool(t tools.Tool) ai.Tool {
    return ai.NewTool(
        t.Name(),
        t.Description(),
        func(ctx *ai.ToolContext, input any) (any, error) {
            return t.Execute(ctx, input)
        },
    )
}
```

`ai.NewTool` is a **generic function**: `NewTool[In, Out any](...)` and returns `*ai.ToolDef[In, Out]`, not `ai.Tool`. The correct call would be:

```go
func ToGenkitTool(t tools.Tool) *ai.ToolDef[any, any] {
    return ai.NewTool[any, any](
        t.Name(),
        t.Description(),
        func(ctx *ai.ToolContext, input any) (any, error) {
            return t.Execute(ctx, input)
        },
        ai.WithInputSchema(t.InputSchema()),  // ← MISSING
    )
}
```

**The `ai.WithInputSchema()` call is essential.** When using `any` as the type parameter, Genkit cannot infer the JSON schema from the Go type. Without it, the LLM won't know what arguments the tool accepts. The design's `InputSchema() map[string]any` method exists for exactly this purpose but the adapter never uses it.

### 2. `Message` Model Is Too Simple for Tool Interactions

The current `Message` struct (`internal/llm/message.go`):

```go
type Message struct {
    Role    Role
    Content string
}
```

This only supports plain text. Tool-calling conversations require:
- **Model messages** containing both text *and* tool request parts
- **Tool messages** containing tool response parts (output data, not text)

The design extends `StreamEvent` and adds `ToolCall` (Section 4) but **never addresses extending `Message` itself**. This is a gap because `AppState.messages` (`internal/cli/repl/state.go`) stores conversation history as `[]llm.Message`. After a tool call round-trip, the history needs to include the tool interaction for the LLM to have proper context on the next turn.

**Recommendation:** Either:
- (a) Extend `Message` to support multi-part content (text + tool requests + tool responses), or
- (b) Store Genkit's `[]*ai.Message` internally for the conversation thread and only use Keen's `Message` at the REPL boundary.

Option (b) is pragmatic for phase 3 since we're only targeting Genkit. Option (a) is needed if we truly want framework-agnostic conversation state.

### 3. Streaming + Tool Loop Is Underspecified

The current `StreamChat` (`internal/llm/genkit.go`) makes a single `genkit.GenerateStream` call. With `WithReturnToolRequests(true)`, the flow becomes:

1. Stream chunks → emit to channel
2. Receive `Done` with `response.ToolRequests()` populated
3. Execute tools manually (emit `tool_start` / `tool_end`)
4. **Call `GenerateStream` AGAIN** with updated messages (original + model's tool request message + tool response message)
5. Stream the new response

This is a **loop inside the goroutine**, and each iteration requires constructing proper `ai.Message` objects with tool request/response `Part`s. The design's flowchart (Section 7) shows the loop conceptually but doesn't address:
- How to build the `ai.Message` containing tool response parts (requires `ai.NewToolResponsePart`)
- How to append the model's response message (which contains `ToolRequest` parts) to the conversation
- That each new `GenerateStream` call returns a fresh `iter.Seq2` — the goroutine needs to iterate over multiple iterators

**Recommendation:** Add pseudocode for the actual goroutine loop, including message construction between iterations.

---

## Important Issues

### 4. `LLMClient` Interface Needs Updating

The current interface (`internal/llm/client.go`):

```go
type LLMClient interface {
    StreamChat(ctx context.Context, messages []Message) (<-chan StreamEvent, error)
}
```

The design says `StreamChat` will accept a `ToolRegistry` (Section 7), but doesn't show the new signature. This is a breaking change to the interface. Should it be:

```go
StreamChat(ctx context.Context, messages []Message, tools *tools.Registry) (<-chan StreamEvent, error)
```

or should tools be set at client construction time? The design should be explicit since this affects both `AppState.StreamChat` and all callers.

### 5. `ToolCall.StartTime`/`EndTime` Should Use Go Types

```go
type ToolCall struct {
    StartTime int64
    EndTime   int64
}
```

Using `time.Time` or `time.Duration` is more idiomatic Go. `int64` epoch times are error-prone (seconds vs milliseconds?). Consider:

```go
type ToolCall struct {
    Name     string
    Input    map[string]any
    Output   any
    Error    string
    Duration time.Duration
}
```

### 6. Registry Implementation Missing

Section 2 mentions a `tools.Registry` in the architecture diagram, but the document never defines it. Even a minimal sketch would help:

```go
type Registry struct {
    tools map[string]Tool
}

func (r *Registry) Register(t Tool)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) All() []Tool
```

---

## Minor / Suggestions

### 7. `Execute()` Input Type Deserves a Note

`Execute(ctx context.Context, input any) (any, error)` — the design should note that `input` will arrive as `map[string]any` (JSON-decoded by Genkit) when called through the framework. Tool implementations need to handle this (type assertion or `json.Unmarshal` into a typed struct).

### 8. Phase Ordering Consideration

The design proposes 4 sub-phases (3.1–3.4). Suggest merging 3.1 and 3.2 since `StreamEvent` extension and `ToolCall` struct are tiny and tightly coupled with the tool foundation. This gives three natural phases:
1. **Tool types + events** (tool.go, dummy.go, message.go extensions, genkit_tools.go adapter)
2. **Client integration** (GenkitClient loop with manual tool handling)
3. **UI integration** (REPL rendering)

### 9. Open Questions — Recommendations

- **Max tool turns:** Default 5 is sensible (matches Genkit's own default in `ai/generate.go`). Make it configurable later, not now.
- **Timeouts:** Per-tool timeout via `context.WithTimeout` wrapping `Execute()` is cleanest. Global timeout is orthogonal (the caller's context already handles that).
- **Parallel tools:** For phase 3 with a dummy tool, execute sequentially. Parallel execution is an optimization for later.
- **Tool versioning:** Not needed now. Over-engineering for a dummy tool phase.

---

## Summary

The architecture philosophy is sound — framework-agnostic tool interface with adapters is the right call. The main gaps are:

| # | Issue | Severity |
|---|-------|----------|
| 1 | Adapter code mismatches Genkit API (generics, `WithInputSchema`) | Critical |
| 2 | `Message` struct can't represent tool interactions / no history plan | Critical |
| 3 | Streaming tool loop implementation underspecified | Critical |
| 4 | `LLMClient` interface change not shown | Important |
| 5 | Non-idiomatic time types | Minor |
| 6 | Registry implementation missing | Important |

Recommend addressing items 1–4 in the design doc before starting implementation. Items 5–6 can be resolved during implementation.

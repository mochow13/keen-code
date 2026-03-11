# Permission UI Redesign Plan (Inline, Contextual Approval)

## Objective

Redesign permission UX from a standalone full-screen selector to an inline, contextual approval card rendered directly in the normal output stream below tool activity.

This should make it easier to understand what the LLM is trying to do in context, while preserving existing permission safety and blocking semantics.

This plan does **not** implement `edit_file` now. It prepares the permission UI architecture so a future `edit_file` tool can integrate diff-first review without additional UI redesign.

---

## Current Problem

Today, permission requests switch the REPL into a dedicated permission mode with a separate screen. This causes:

1. Context switch away from tool output/transcript
2. Lower visibility of what operation triggered the prompt
3. No room for richer inline context (for example edit diff)
4. Session-approved fast path bypasses the permission UI entirely

---

## Desired UX

### Baseline behavior

1. Tool emits start/output as usual
2. If permission is needed, show an inline approval card right below relevant tool output
3. User approves/denies directly in that inline card using keyboard controls
4. Decision status appears inline under the same card
5. Tool continues or aborts based on user decision

### Session-approved behavior

For existing tools, keep current behavior by default.

The redesign should support an optional future mode where a tool can request a visible inline review card even when session-approved (needed later for `edit_file` diff previews).

---

## Scope

### In scope

1. Inline permission-card rendering in transcript/viewport
2. Keyboard interaction for pending permission card
3. Card structure that can carry optional preview payloads (for future diff or other previews)
4. Tests for ordering, interaction, and resolution flow

### Out of scope (for this phase)

1. Mouse-driven selection UI for permission cards
2. Multi-card simultaneous approval UI (we can queue requests)
3. Fancy collapse/expand animations
4. Rich side-by-side diff layout
5. Implementing `edit_file` tool behavior

---

## High-Level Design

## 1. Event-driven permission rendering

Treat permission requests like stream events, similar to tool start/end.

Introduce UI event concepts:

1. `permission_request_start`
2. `permission_request_update` (optional, future)
3. `permission_request_resolved`

Each request has a stable `request_id` and references:

1. tool name
2. operation
3. path/resolved path
4. dangerous flag
5. optional `preview` payload (e.g. diff text in future)
6. status (`pending`, `allowed`, `allowed_session`, `denied`, `auto_allowed_session`)

## 2. Inline permission card segment

Render permission request as a stream segment in normal viewport flow:

1. Header: "Permission required" (or "Edit review")
2. Metadata: tool, operation, path, resolved path
3. Optional diff preview (truncated)
4. Choice rows: `Allow`, `Allow for this session`, `Deny` (dangerous => `Allow`, `Deny`)
5. Hint row with keyboard controls

Resolved cards remain in transcript with final status line.

## 3. Input routing model

Keep normal REPL view active; no separate permission mode.

When there is a pending permission request:

1. Route `up/down/enter/esc` to card selector
2. Keep `pgup/pgdown/home/end` for transcript scrolling
3. Ignore standard input submission (`enter`) until request is resolved

When no pending request exists, normal key behavior applies.

## 4. Blocking semantics remain unchanged

Tool execution still blocks on permission requester response channel. UI redesign should not change safety semantics.

Flow:

1. Tool requests permission
2. Requester publishes request event for rendering
3. Tool waits for response
4. User decision resolves request
5. Requester returns decision to tool

## 5. Future compatibility flow (for `edit_file` later)

Design the requester/rendering contract so a future tool can opt into one of two behaviors when session-approved:

1. Silent fast path (current behavior)
2. Visible auto-approved card (future `edit_file` requirement)

The UI layer should support both without structural changes.

---

## Data Model Changes

## Permission request model

Extend request payload (or introduce a UI-facing wrapper) with:

1. `RequestID string`
2. `Preview string` (optional, tool-defined)
3. `PreviewKind string` (optional, example `diff`)
4. `AutoApproved bool`
5. `Status PermissionStatus`

`PermissionStatus` enum:

1. `pending`
2. `allowed`
3. `allowed_session`
4. `denied`
5. `auto_allowed_session`

## Stream segment model

Add segment types:

1. `segmentPermission`

Segment stores:

1. request metadata
2. selector cursor (if pending and interactive)
3. resolved status

---

## Implementation Phases

## Phase 1: Inline rendering foundation

1. Add permission segment type in streaming/output pipeline
2. Add permission card formatter (`formatPermissionCard`)
3. Emit card when permission request is consumed
4. Keep existing modal selector temporarily as fallback behind a feature flag

Deliverable: permission requests can be displayed inline in transcript.

## Phase 2: Interaction handoff

1. Remove/disable `permissionSelector` modal path
2. Route approval keys to pending inline permission card state
3. On `enter`, send selected choice through requester
4. On `esc`, resolve as deny
5. Persist resolved card output in transcript

Deliverable: fully functional inline approval interaction.

## Phase 3: Edit diff integration

1. Add generic preview rendering capability in card (type-aware by `PreviewKind`)
2. Add preview truncation rules for long payloads (see limits below)
3. Keep styling conservative until a concrete tool integration is implemented

Deliverable: preview-capable inline approval UI ready for future `edit_file` integration.

## Phase 4: Session-approved visibility option

1. Add an explicit request flag to allow visible auto-approved cards
2. Mark status `auto_allowed_session`
3. Do not block for input in this path

Deliverable: optional visible auto-approved path available for future tools.

## Phase 5: Cleanup and hardening

1. Remove obsolete modal permission code paths if unused
2. Add regression tests (see test plan)
3. Validate keyboard handling with active stream + pending approval

Deliverable: stable, maintainable permission UI architecture.

---

## Preview Rendering Rules

To prevent output overload:

1. Show max first 120 preview lines by default
2. If truncated, append line: `... N more preview lines omitted`
3. Preserve raw preview text model-level and apply styling in view layer
4. If `PreviewKind` is known (for example `diff`), apply kind-specific styling rules

Optional future enhancement: keybinding to expand/collapse full preview.

---

## Error Handling & Edge Cases

1. Request canceled via context timeout => card resolves as denied/canceled
2. `esc` key on pending card => explicit deny
3. Multiple permission requests arriving quickly => queue and display one active pending card at a time
4. If permission request has no resolved path, still render safely with available metadata
5. If preview is empty, still render card with metadata and choices

---

## Testing Strategy

## Unit tests

1. Permission card rendering (pending vs resolved statuses)
2. Preview rendering styles and truncation behavior
3. Choice mapping for dangerous vs non-dangerous operations
4. Key handling while pending request exists
5. Auto-approved card behavior when explicitly requested by tool metadata

## Integration/repl tests

1. Tool output followed by inline permission card appears in correct order
2. Selecting allow/deny updates card and unblocks tool execution
3. Session-approved visible-card path shows card and continues without extra input
4. Existing non-permission interaction still works when no pending request

## Regression tests

1. Existing tools still request permission correctly
2. No deadlocks in requester/response channels
3. Transcript remains readable with long preview payloads

---

## Migration Notes

1. Implement behind a temporary feature flag if needed (`inlinePermissionUI`)
2. Run both code paths in tests during transition
3. Remove modal path once parity is confirmed and tests are green

---

## Risks and Mitigations

1. Risk: Keybinding conflicts with normal input
Mitigation: explicit pending-request gate in key router

2. Risk: Transcript clutter from long diffs
Mitigation: line cap + truncation message

3. Risk: Request ordering/race issues
Mitigation: single active pending request + deterministic queue

4. Risk: Behavior drift for dangerous operations
Mitigation: preserve current dangerous-choice set (`Allow`, `Deny`)

---

## Acceptance Criteria

1. Permission prompts appear inline beneath tool outputs (no standalone permission screen)
2. User can approve/deny using keyboard without leaving normal transcript view
3. Permission card supports optional tool-provided preview payload without new UI redesign
4. Architecture supports optional visible auto-approved cards for session-approved operations
5. Tool execution remains blocked until explicit decision except explicit auto-approved path
6. Existing tools continue to function with permission system unchanged in behavior

---

## Suggested Follow-up (Optional)

After this redesign lands, we can decide whether to standardize informational cards for all session-approved tools (not only `edit_file`) for complete UX consistency.

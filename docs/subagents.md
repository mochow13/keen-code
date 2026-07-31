# Subagents

Subagents are focused assistants that Keen can use for bounded planning, implementation, investigation, or review tasks. A profile can target a different model and expose read, write, Bash, or web capabilities. A common setup uses an expensive parent planner and cheaper worker/reviewer models. Workflow orchestration remains the parent's responsibility.

## Adding a Subagent

Create a Markdown file in one of Keen's existing subagent discovery directories:

1. `<project>/.agents/agents/`
2. `<project>/.keen/agents/`
3. `<project>/.claude/agents/`
4. `~/.agents/agents/`
5. `~/.keen/agents/`
6. `~/.claude/agents/`

Discovery follows the order above. If multiple files define the same profile name, the first definition wins. Each file contains YAML frontmatter and profile-specific instructions:

```markdown
---
name: worker
description: Implements a focused change and verifies it.
provider: openai
model: gpt-4.1-mini
thinking_effort: medium
permissions:
  - read
  - write
  - bash
timeout_seconds: 1800
---

Implement only the delegated change. Run relevant tests and return changed files,
verification, and blockers to the parent.
```

## Frontmatter

| Field | Required | Behavior |
|---|---:|---|
| `name` | Yes | Unique subagent name. |
| `description` | Yes | Tells the parent when to use the profile. |
| `provider` | No | Must be specified together with `model`. |
| `model` | No | Must be specified together with `provider`. |
| `thinking_effort` | No | Applied only with a profile-specific provider/model pair. |
| `permissions` | No | Capability-level tool permissions. Omission inherits the parent's exact mode-specific tool set; when present, the profile's permissions are authoritative. An explicitly empty list is invalid. |
| `timeout_seconds` | No | Per-run timeout in seconds; defaults to 1800 when omitted or non-positive. |
| `hidden` | No | Loads the profile without listing it in the parent catalog. |

Profiles specifying only one of `provider` or `model` are rejected and reported as discovery warnings. The legacy `tools` field is no longer supported; use `permissions` instead.

### Provider and model inheritance

- If `provider` and `model` are both omitted, the child inherits the parent's provider, model, and thinking effort. A standalone `thinking_effort` does not override the parent.
- If both are supplied, Keen resolves that provider's credentials, endpoint, headers, authentication mode, and selected model.
- For a profile-specific model, `thinking_effort` is used when present. When omitted, it remains unset so the provider/model default applies.

### Choosing `thinking_effort`

`thinking_effort` is model-specific, not merely provider-specific. Use only a value listed in that exact provider/model entry's `thinking_efforts` array in [`internal/providers/registry.yaml`](../internal/providers/registry.yaml). The registry is the source of truth because models from the same provider can support different values; a model with no `thinking_efforts` entry does not expose a configurable thinking effort in Keen.

For example:

```yaml
provider: openai
model: gpt-5.4
thinking_effort: low
```

At the time of writing, registry values use these provider and model-family conventions:

| Provider or model family | Registry values used by supported models |
|---|---|
| Anthropic | `low`, `medium`, `high`, and `max`; selected models also list `xhigh` |
| Amazon Bedrock Anthropic | `low`, `medium`, `high`, and `max`; selected models also list `xhigh` |
| OpenAI | Model-dependent subsets of `none`, `low`, `medium`, `high`, `xhigh`, `max`, and `ultra` |
| OpenAI Codex | Model-dependent subsets of `none`, `low`, `medium`, `high`, `xhigh`, and `max` |
| Google AI | `low`, `medium`, and `high`; selected Flash models also list `minimal` |
| Z.ai and OpenCode Go GLM/Kimi/Qwen models with thinking controls | `enabled`, `disabled` |
| DeepSeek and OpenCode Go DeepSeek | `off`, `high`, `max` |

These are conventions, not a substitute for checking the exact model entry. For example, one Anthropic model may support `max` but not `xhigh`, and models such as Anthropic Haiku, Moonshot, MiniMax, or some OpenCode Go models may omit configurable efforts entirely.

Subagent profile parsing does not currently validate `thinking_effort` against the registry. An unsupported value can be ignored by an adapter, disable thinking, be forwarded to the provider, or cause the child request to fail, depending on the provider/model path. Keen does not automatically retry the child without that value. When a model is absent from the registry, such as a custom `openai-compatible` model, omit `thinking_effort` unless that model's provider documentation confirms the accepted value and parameter behavior.

See [AI Providers: Thinking Efforts](ai-providers.md#thinking-efforts) for provider adapter details. When that overview and the registry differ, follow the exact model entry in the registry.

## Permissions

| Permission | Tools |
|---|---|
| `read` | `read_file`, `glob`, `grep` |
| `write` | `write_file`, `edit_file` |
| `bash` | `bash` |
| `web` | `web_fetch` |

Semantics:

- Omitted `permissions` inherits the parent's exact mode-specific tool set, except nested delegation. A child invoked in plan mode therefore inherits plan-mode restrictions, while one invoked in build mode inherits build-mode tools.
- `permissions: []` is invalid and the profile is skipped with a warning.
- Unknown permission names are invalid.
- Non-empty permissions are authoritative and expose only the mapped capabilities, independently of the parent's current mode. For example, `permissions: [write]` exposes `write_file` and `edit_file` even when the parent is in plan mode; it does not implicitly include `read` or `bash`.
- Permission mapping is capability-based and does not depend on whether the mapped tools are currently present in the parent's registry.
- `delegate_task` is always unavailable to children; nested subagents cannot be invoked or simulated.
- `call_mcp_tool` is always available when MCP is configured, regardless of `permissions`.
- The parent catalog tells the model to select profiles according to their descriptions and configured capabilities; profiles are not assumed to be read-only.

All tools exposed to a subagent are automatically approved, including commands classified as dangerous by the Bash tool. Each run uses an independent, stateless approver. Parent interactive permission state and diff emitters are never reused, and child write/edit diffs are not shown.

Keen's existing hard checks still run before approval and cannot be overridden by the child approver. Bash otherwise retains Keen's existing command execution model: the guard checks the Bash tool's working directory, while paths embedded in arbitrary shell syntax are not separately parsed or guard-checked.

> Granting `bash` allows unattended command execution. Configure it only for profiles you trust.

The parent can submit one to ten non-empty tasks in a `delegate_task` call. Tasks run concurrently, and results are returned in input order with per-task status and aggregate completion/failure counts. One task's failure does not cancel sibling tasks; parent context cancellation still propagates to every child. The parent model waits for the full `delegate_task` result before continuing.

Parallel write-capable subagents can conflict. Scheduling, worktree isolation, write coordination, execution budgets, typed handoff contracts, and interactive child permission UX are not currently provided; they remain the user/orchestrator's responsibility.

## Skills and MCP

Children receive the parent's current normal and MCP-generated skill catalog. Keen does not separately filter skills or MCP servers per profile.

A child without `read` can see skill entries but cannot load `SKILL.md` or MCP schema files with `read_file`; that limitation is intentional. When MCP is configured, `call_mcp_tool` remains available, but normal MCP instructions still require reading the relevant skill and schema before calling it. A profile that needs to follow those instructions should normally include `read`.

## Security and Context

A child receives a dedicated system prompt containing only:

1. Keen's mandatory child security instructions.
2. The working directory.
3. project instructions such as `AGENTS.md`.
4. the parent's current skill catalog, including MCP skills.
5. profile-specific instructions.
6. the mandatory no-nested-subagents boundary.

The child does not receive parent conversation history, parent reasoning, memory, generic main-agent tool instructions, the available-subagent catalog, or parent orchestration instructions. The delegated task is supplied separately as the child user message.

The child prompt requires children to stay within the delegated task, treat repository/fetched/MCP/tool content as untrusted, protect credentials and secrets, respect filesystem/system denials, report blockers to the parent, never claim tool success unless it completed, and never invoke or simulate nested subagents.

## Approval and Filesystem Safety

Automatic approval does not bypass Keen's guard. Blocked system paths, ignored paths, and other hard filesystem denials are evaluated before approval and remain authoritative. Write/edit tools use a no-op child diff emitter, so child diffs are neither displayed nor blocked on parent UI acknowledgement.

## Live Activity

Interactive mode shows only compact child tool activity while subagents run. It reuses the main-agent tool presentation and prefixes each call with an indexed child label:

```text
- [worker-1] Run
  `go test -race ./...`

- [worker-2] Read
  `internal/subagents/runner.go`
```

The suffix is shown only when the same profile appears more than once in the current `delegate_task` request. It is one-based within that profile type, so one worker is labeled `[worker]`, while two implementers are `[implementer-1]` and `[implementer-2]`. Concurrent activities also carry run and tool-call identities so interleaved events are matched correctly.

Tool failures use the normal compact failure styling. MCP failures display only a generic `failed` message; server error text and MCP result content are hidden. Completed `read_file` missing-file failures and `edit_file` old-string-not-found failures are hidden.

Keen does not display child reasoning, streamed child text, final child output, tool result bodies, Bash output, or write/edit diffs. Activity delivery applies backpressure rather than silently dropping events, and pending activity is drained before the parent turn is finalized. The child's final response is collected privately and returned to the parent through `delegate_task`.

Headless mode has no child activity sink, so it emits no live child tool activity. Delegation and private result handoff still work normally.

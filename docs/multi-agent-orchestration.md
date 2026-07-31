# Multi-Agent Orchestration

Multi-agent orchestration divides a larger task into bounded rounds coordinated by the main Keen agent. The main agent acts as the parent: it plans the work, calls `delegate_task`, evaluates child results, integrates shared changes, and owns final verification. Subagents are one-shot workers and cannot delegate recursively.

Keen does not include the example subagents or workflow described on this page. Users must create profiles appropriate for their project, provider, models, permissions, and budget. Orchestration can then be guided by a project document, project instructions, or a skill.

Keen does not currently enforce task dependencies, file ownership, cost budgets, or isolated worktrees. A `delegate_task` call is one parallel barrier: all tasks in that call start concurrently, and the parent receives the collected results after they finish. Dependent tasks must be placed in separate calls.

## Cost Caution

> Use subagents selectively. Every child is a separate model run with its own token usage, tool calls, and possible retries. Parallel exploration, implementation, and review can cost substantially more than completing a small task with the main agent. A high concurrency limit reduces elapsed time, not total cost. Delegate only when the expected benefit exceeds the additional model and tool cost.

Ways to control cost include:

- use the main agent directly for small or tightly coupled tasks;
- select the cheapest model that can reliably perform each bounded role;
- avoid asking several agents to repeat the same repository exploration;
- limit the number of tasks in each round;
- use focused task prompts and verification commands;
- configure model-specific thinking effort only when needed;
- set reasonable profile timeouts; and
- stop delegating when coordination costs more than the remaining work.

Failed, blocked, and retried runs still consume model and tool usage.

## Permission Warning

> Subagents can freely use every tool exposed by their configured `permissions`; child tool calls are automatically approved. In particular, `bash` permits unattended command execution, including commands Keen classifies as dangerous. Grant `write`, `bash`, `web`, or MCP access only to profiles you trust.

Filesystem hard denials still apply, but paths embedded in arbitrary shell syntax are not separately parsed and guard-checked. A role named `reviewer` is not inherently read-only: if its profile grants `write` or `bash`, those capabilities are available for the entire child run. MCP tools are also available when MCP is configured; see the **Skills and MCP** section in [Subagents](subagents.md).

Review every profile's instructions and permissions before using it. Prefer `read` alone for discovery, planning, and reviews unless command execution is genuinely required.

## 1. Prepare Subagent Profiles

Create only the roles your workflow needs. Project-scoped profiles can be placed in:

```text
<project>/.agents/agents/
```

The following four roles are possible examples. They are not bundled with Keen and do not exist unless the user creates them.

| Example profile | Purpose | Suggested permissions |
|---|---|---|
| `explorer` | Investigate one bounded repository question | `read` |
| `test-designer` | Design acceptance cases; optionally implement tests | `read`, or `read` and `write` when necessary |
| `implementer` | Make a focused change in an assigned scope | `read`, `write`, and optionally `bash` |
| `reviewer` | Independently inspect a completed change | `read`, and optionally `bash` |

### Example `explorer` Profile

Create `.agents/agents/explorer.md`:

```markdown
---
name: explorer
description: Investigates a bounded repository question without modifying files.
provider: <provider>
model: <inexpensive-model>
permissions:
  - read
timeout_seconds: 300
---

Investigate only the delegated question. Do not modify files.

Return:
- status: completed, partial, or blocked
- concise answer
- relevant paths, symbols, and line references
- current behavior and constraints
- likely change scope
- unresolved questions
```

### Example `test-designer` Profile

Create `.agents/agents/test-designer.md`:

```markdown
---
name: test-designer
description: Designs focused acceptance and regression tests for delegated behavior.
provider: <provider>
model: <inexpensive-model>
permissions:
  - read
timeout_seconds: 600
---

Analyze only the delegated behavior. Identify critical happy paths, boundaries,
failures, and regression cases using existing project conventions. Do not modify
files unless the task and this profile explicitly permit writing.

Return:
- status: completed, partial, or blocked
- behavior matrix with expected outcomes
- coverage rationale
- suggested test files
- gaps, risks, and unresolved requirements
```

Grant `write` only if this role should also create tests. Grant `bash` only if it should execute test commands.

### Example `implementer` Profile

Create `.agents/agents/implementer.md`:

```markdown
---
name: implementer
description: Implements a focused, well-specified change in an assigned scope.
provider: <provider>
model: <inexpensive-model>
permissions:
  - read
  - write
  - bash
timeout_seconds: 1200
---

Implement only the delegated change and modify only the assigned scope. Stop and
report a blocker when the task requires an architectural decision, unclear
requirements, or out-of-scope files. Follow project instructions and avoid
unrelated cleanup. Run only the requested or narrowly relevant checks.

Return:
- status: completed, partial, or blocked
- behavior implemented
- every changed file
- decisions and assumptions
- exact verification commands and outcomes
- remaining risks and blockers

Do not commit, discard, or overwrite unrelated work.
```

This example grants `bash`, so the child can execute commands without interactive approval. Remove that permission if the parent should retain command execution.

### Example `reviewer` Profile

Create `.agents/agents/reviewer.md`:

```markdown
---
name: reviewer
description: Reviews a bounded implementation for defects and critical test gaps.
provider: <provider>
model: <inexpensive-model>
permissions:
  - read
timeout_seconds: 600
---

Review only the delegated change. Do not modify files. Prioritize concrete
correctness defects, regressions, safety problems, race conditions, incompatible
behavior, and missing critical tests. Give exact file and line references.

Return:
- status: completed, partial, or blocked
- findings ordered by severity
- impact and recommended correction for each finding
- critical test gaps
- assumptions and verification limitations
```

Add `bash` only when the reviewer must run commands and the associated unattended execution risk is acceptable.

Replace `<provider>` and `<inexpensive-model>` with a configured provider and an exact supported model. If specifying `thinking_effort`, use only a value supported by that exact provider/model entry. See the **Choosing `thinking_effort`** section in [Subagents](subagents.md).

## 2. Define the Parent's Workflow

Profiles define worker capabilities; they do not tell the main agent how to coordinate a project. The user must provide orchestration instructions. Two simple options are:

1. Write a workflow document and ask the main agent to follow it.
2. Create a skill that teaches the main agent when and how to delegate.

### Workflow Document

A project can keep a workflow anywhere readable by the main agent, for example:

```text
docs/workflows/context-compaction.md
```

The user can then request:

```text
Follow docs/workflows/context-compaction.md to implement automatic context
compaction. Use the configured subagents where the workflow calls for them.
```

A workflow should identify:

- dependency-ordered rounds;
- which profile handles each task;
- files or directories owned by each writer;
- contracts that must stabilize before dependent work starts;
- acceptance criteria and focused checks for every task;
- integration and review gates; and
- final checks owned by the parent.

### Orchestration Skill

For a reusable policy, create `.agents/skills/orchestrate/SKILL.md`:

```markdown
---
name: orchestrate
description: Coordinates non-trivial coding tasks with configured subagents.
---

Use subagents only when delegation is worth its additional cost.

1. Keep ambiguity, architecture, shared contracts, and final acceptance with the parent.
2. Use read-only agents for independent discovery when repository knowledge is incomplete.
3. Synthesize discovery before creating implementation tasks.
4. Put dependent work in separate delegate_task calls.
5. Run write-capable tasks concurrently only when their owned files do not overlap.
6. Include context, scope, requirements, acceptance criteria, and checks in every task.
7. Inspect child results and the combined working tree after every write round.
8. Use independent review only when the risk justifies another model run.
9. Run final repository-wide verification in the parent.
10. Stop and handle work directly when a child is blocked, uncertain, or out of scope.
```

The user can explicitly activate the skill or ask the main agent to load it for a complex task. See [Skills System](skills-system.md) for discovery and activation details.

A skill provides instructions, not deterministic scheduling. The main model still decides how to apply them, and Keen still does not enforce dependencies or file ownership.

## 3. Example Orchestration Approach

Consider a feature that introduces automatic context compaction across:

- shared LLM contracts;
- several provider request loops;
- REPL and headless behavior;
- session persistence; and
- regression tests.

This shape benefits from selective parallelism because provider integrations can eventually be independent, while shared APIs must be established serially first.

```text
Parent establishes baseline
        |
        v
Round 1: parallel read-only discovery
        |
        v
Parent synthesizes findings and finalizes the plan
        |
        v
Round 2: serial shared foundation
        |
        v
Parent verifies and stabilizes shared contracts
        |
        v
Round 3: parallel provider integrations with disjoint ownership
        |
        v
Round 4: parallel UI and persistence integrations with disjoint ownership
        |
        v
Round 5: optional independent reviews
        |
        v
Parent fixes findings and runs final verification
```

### Round 1: Discovery

The parent can delegate independent read-only questions in one call:

| Example profile | Bounded question |
|---|---|
| `explorer` | Map shared LLM contracts, context reduction, compaction prompts, and relevant tests. |
| `explorer` | Map provider request loops, retries, pending state, tool-result handling, and native-history rebuilding. |
| `explorer` | Map REPL/headless streaming, cancellation, checkpoints, session events, and state replacement. |
| `test-designer` | Propose a minimal lifecycle and recovery test matrix without changing files. |

Example task packet:

```text
Investigate automatic context compaction in the shared LLM layer.

Scope:
- shared message and client contracts
- context reduction
- compaction prompt construction
- focused tests

Do not modify files. Identify current contracts, callers, invariants, existing
tests, and likely shared API changes. Return exact paths and symbols, unresolved
questions, and a suggested implementation scope.
```

The parent synthesizes all results before continuing. Children do not receive the parent's conversation history, so relevant findings must be copied into later task prompts.

### Round 2: Shared Foundation

The shared LLM layer should be implemented serially because all provider and UI tasks depend on its contracts. The parent may handle this directly or assign one `implementer` a non-overlapping owned scope.

Possible deliverables include:

- effective context-budget calculation and a compaction trigger;
- typed context-overflow errors;
- compaction lifecycle events;
- recursion prevention;
- transactional history replacement;
- transient handling of raw tool results; and
- focused shared tests.

The parent inspects the result and runs the shared package gate before continuing. If the contract is unstable, fix it first; launching more workers will not compensate for a broken foundation.

### Round 3: Provider Integrations

After shared interfaces stabilize, provider integrations can run concurrently when each task owns distinct files:

| Task | Example ownership |
|---|---|
| Provider A | Provider A implementation and dedicated tests |
| Provider B | Provider B implementation and dedicated tests |
| Provider C | Provider C implementation and dedicated tests |

Every task should receive the same finalized acceptance contract. Include an explicit boundary such as:

> Modify only the assigned provider implementation and dedicated tests. Do not change shared contracts. If a shared helper is missing, return `blocked` and describe the required change.

Keen does not enforce this ownership and all children share the working tree. The parent must inspect changed-file lists and repository status after the round. Never intentionally run concurrent writers against overlapping files.

### Round 4: UI and Persistence

Once lifecycle events are stable, UI and persistence tasks may run concurrently if their files and responsibilities are disjoint. For example:

| Task | Example responsibility |
|---|---|
| REPL integration | Stream events, cancellation boundaries, loader state, checkpoints, and focused UI tests |
| Headless/session integration | Output ordering, persisted events, checkpoints, and focused session tests |

The parent supplies the finalized shared event contract. These workers should integrate it rather than redesigning it.

### Round 5: Review

After all writers finish, optional read-only review tasks can inspect independent risk areas:

| Example profile | Review focus |
|---|---|
| `reviewer` | Shared policy, transactionality, recursion prevention, budgets, and retry ordering |
| `reviewer` | Provider state consistency, reset behavior, and retry-once guarantees |
| `reviewer` | UI/session cancellation, private output, checkpoint persistence, and duplicate prevention |
| `test-designer` | Implemented coverage compared with the acceptance matrix |

Do not run reviewers concurrently with active writers. Review is another model round with a real cost, so use it for changes whose risk justifies independent analysis. The parent evaluates findings rather than accepting them automatically.

### Final Verification

The parent owns final integration and repository-wide verification. A child's report that tests passed is evidence, not final acceptance. The parent should independently verify the combined working tree using the project's required formatting, dependency, test, lint, and build commands.

## Task Handoff Template

A worker performs best when its task is self-contained:

```markdown
## Objective

One concrete outcome.

## Context

Relevant discoveries and finalized contracts from earlier rounds.

## Scope

Allowed files and directories, plus explicit exclusions.

## Requirements

Observable behavior, compatibility constraints, and error handling.

## Acceptance Criteria

Specific tests or outcomes that demonstrate completion.

## Verification

Exact focused commands to run, if the profile has the required permission.

## Return

Status, summary, files inspected or changed, decisions, command results,
remaining risks, and blockers.
```

Do not rely on a child to infer omitted context from the parent's conversation.

## Coordination Rules

- Prefer direct parent execution for small, tightly coupled, ambiguous, or security-sensitive tasks.
- Use parallel discovery only when questions are genuinely independent.
- Stabilize shared contracts before dependent integrations.
- Never schedule dependent work in the same `delegate_task` call.
- Never intentionally give concurrent writers overlapping files.
- Keep architecture, shared-file integration, and final acceptance with the parent.
- Verify the combined working tree after every write round.
- Treat profile timeouts as limits, not spending budgets.
- Treat profiles with `write`, `bash`, `web`, or MCP capabilities as privileged automation.
- Remember that failed and retried runs still incur usage.

The goal is not to maximize the number of agents. It is to use additional model calls only where clear boundaries, independent ownership, and objective verification make delegation worth its cost and risk.

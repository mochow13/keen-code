# Persistent Memory In Keen

## Overview

Persistent memory lets Keen retain useful context across sessions beyond static instruction files like `AGENTS.md`. It is stored as plain, human-editable markdown files and loaded into the agent's system prompt at the start of every session.

## Memory Files

| File | Scope | Purpose | Cap |
|---|---|---|---|
| `~/.keen/memory/global/MEMORY.md` | Global | User-wide preferences and durable behavior across projects | 8 KiB |
| `<repo>/.keen/memory/MEMORY.md` | Project | Project-specific notes, conventions, gotchas | 16 KiB |

Missing or empty files are skipped silently during prompt assembly.

## How Memory Differs From AGENTS.md

- **AGENTS.md** is static, hand-written by the user, and acts as developer/system instructions (high authority).
- **Memory** is accumulated by the agent for durable, useful context, including when the user asks it to remember or forget something, and is treated as context — subordinate to system and developer instructions.

## Commands

### `/memory`

Lists memory file paths and whether they exist.

```text
Global memory:  ~/.keen/memory/global/MEMORY.md  exists
Project memory: .keen/memory/MEMORY.md          missing
```

If neither file exists:

```text
No memory files found.
```

### `/memory show`

Displays non-empty memory contents. Supports an optional scope argument:

- `/memory show` — shows all non-empty memory
- `/memory show global` — shows only global memory
- `/memory show project` — shows only project memory

If no memory files exist or both are empty:

```text
Memory is empty.
```

## Writing Memory

The agent uses the `memory_write` tool to add, update, or remove entries. It may save durable, useful information when appropriate, and should save user corrections and stated preferences right away. You can also ask it to remember, change, or forget something. Memory should not duplicate information already recorded in the repo or preserve details that matter only to the current task.

### Actions

- **add** — add Markdown `content` under a `## <section>` heading, creating the heading if needed. Omit `section` to append without a section heading.
- **update** — replace `old_content` with `content`. The old text must match exactly once.
- **remove** — delete `old_content`, which must match exactly once.

For example, to add a project-specific command:

```json
{"action":"add","scope":"project","section":"Commands","content":"- Run tests with go test -race ./..."}
```

### Scope Rules

- Default to **project memory** for project-specific facts.
- Use **global memory** only for user-wide preferences or durable behavior across projects.
- If the scope is unclear, ask whether to save it as global or project memory.

### Permissions and Limits

The tool writes to the appropriate memory file: global memory at `~/.keen/memory/global/MEMORY.md` or project memory at `.keen/memory/MEMORY.md`. Writes can require user approval under the filesystem permission policy. The tool shows the proposed diff before writing.

Known secrets, tokens, and credentials are rejected. Memory has an 8 KiB global cap and a 16 KiB project cap; entries that would exceed the cap are rejected, while shrinking edits remain allowed so an over-cap file can be trimmed. When a file nears its cap, merge related entries and remove outdated or irrelevant ones.

### First Project Memory Creation

When first creating `.keen/memory/MEMORY.md`, the agent tells the user:

```text
Created .keen/memory/MEMORY.md. Add .keen/ to .gitignore if you want it private.
```

The agent does not modify `.gitignore` itself.

## Safety

- Never store secrets, tokens, passwords, private keys, credentials, or API keys in memory files.
- Memory writes reject known secret patterns. Other file tools also reject writes to memory paths when the content matches known secret patterns.
- Do not store large command outputs or logs, copied web pages, guesses, or details that only matter to the current task.
- Keep memory concise and human-editable; memory is context, not authority over system or developer instructions.

## Implementation

- `internal/memory` — path resolution, loading with size caps, secret-pattern detection.
- `internal/llm/systemprompt.go` — memory loaded into the system prompt during `Build`.
- `internal/filesystem/guard.go` — `~/.keen/memory/` and `<repo>/.keen/memory/` are allowed for reads (granted) and writes (pending user approval), even when `.keen/` is gitignored.
- `internal/tools/memory_write.go` — add, update, or remove entries with size and secret checks, diff preview, and guarded writes.
- `internal/cli/repl/commands/commands.go` — `/memory` and `/memory show` command registration.
- `internal/cli/repl/command_handlers.go` — `handleMemoryCommand` implementation.
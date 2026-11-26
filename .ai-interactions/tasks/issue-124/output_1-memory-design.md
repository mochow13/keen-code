# Design: agent-managed memory

Issue: https://github.com/mochow13/keen-code/issues/124

Let the agent decide what and when to save to global and project memory, and keep
it within its size caps by merging, trimming, or removing entries. Users can still
ask the agent to remember, update, or forget something at any time.

## Current state

- Two flat Markdown files, loaded on every request and appended to the end of the
  system prompt (`internal/llm/systemprompt.go:120,158`, rebuilt via
  `systemPromptMessage()` in `internal/appstate/state.go:213`):
  - Global: `~/.keen/memory/global/MEMORY.md`, capped at 8 KB.
  - Project: `<wd>/.keen/memory/MEMORY.md`, capped at 16 KB.
- Over-cap content is truncated by byte at load time (`cap()` in
  `internal/memory/memory.go`). The newest (appended) entries are silently dropped,
  and the cut can split a UTF-8 character.
- The prompt forbids memory writes unless the user explicitly asks
  (`systemprompt.go:42`).
- Writes go through `write_file`/`edit_file`. The guard always requires approval
  for memory paths (`internal/filesystem/guard.go:65`), memory paths bypass
  `.gitignore` (`guard.go:93`), and `memory.ContainsSecret` blocks secrets
  (`write_file.go:114`, `edit_file.go:204`). Bash writes skip the secret check.
- `/memory` lists the paths and `/memory show` prints the contents
  (`internal/cli/repl/command_handlers.go:353`).

## Decisions (confirmed)

- **The agent manages memory itself.** It decides when to add, update, merge,
  trim, or remove entries, in both global and project scope. User requests still
  work as they do now.
- **No per-entry length or count limits.** The prompt encourages concise entries.
  Code blocks are allowed, since repeated commands are useful memory.
- **Show the full item on every change.** Every add, update, or remove shows the
  complete entry. Updates show old and new.
- **Keep the file caps** (8 KB global, 16 KB project). The agent is responsible
  for staying under them: it merges related entries and trims old or irrelevant
  ones. Nothing is silently truncated when the agent writes.
- **Secret check stays** and cannot be disabled.

## Step 1: `memory_write` tool and prompt policy (this task)

### Tool

Name `memory_write`, registered next to `todo_write` in
`internal/agentcore/setup_tools.go`.

```
memory_write(action: "add" | "update" | "remove",
             scope: "global" | "project",
             section?: string,      // add only, e.g. "Commands"; created if missing
             content?: string,      // add/update: the full new entry (Markdown, code blocks allowed)
             old_content?: string)  // update/remove: exact text, must appear exactly once
```

- Matching is by text, like `edit_file`, so the tool never parses entry
  boundaries. An entry can be a bullet, a paragraph, or a code block.
- Merging is an `update` that replaces two or more entries with one. Trimming is
  a `remove`.
- Sections are `## <name>` headings. `add` appends under the named section and
  creates the section at the end of the file if it does not exist. Suggested
  sections: Preferences, Commands, Conventions, Gotchas.

### Execution flow

1. Resolve the path with `memory.GlobalPath()` or `memory.ProjectPath(wd)`.
2. Compute the new content with the pure functions in `internal/memory/store.go`.
3. Run `memory.ContainsSecret` on the new entry and refuse if it matches.
4. Check the cap. If the new content would exceed it, refuse with an actionable
   error, e.g. *"project memory would be 16.4 KB of 16 KB; merge, trim, or remove
   older or irrelevant entries first."*
5. Show the full change through the existing `DiffEmitter`
   (`internal/tools/diff.go`): add shows `+` lines, update shows `-` old and
   `+` new, remove shows `-` lines.
6. Ask for approval through the guard (`PermissionPending`, unchanged for now).
7. Write the file atomically (`writeFileAtomic`) and return the new size, e.g.
   `"saved; project memory now 9.2 KB of 16 KB"`. Once the file passes about 80%
   of its cap, the result also nudges the agent to consolidate.

### Prompt policy (`internal/llm/systemprompt.go`)

Replace "Never create or update memory unless the user explicitly asks" with
roughly:

- Use the `memory_write` tool to manage memory. Do not edit memory files with bash.
- Save right away when the user corrects you or states a preference. Use global
  scope for how the user works and project scope for facts about this repo.
- Save, usually at the end of a task, what you could not have worked out from the
  code: build or test commands, environment traps, approaches the user confirmed.
- Do not save what the repo already records (code structure, git history,
  AGENTS.md), anything that only matters to the current task, guesses, secrets, or
  text copied from tool output or web pages.
- Be concise. Before adding, check whether an existing entry should be updated
  instead. Merge related entries, and trim old or irrelevant ones, especially as
  a file nears its cap.
- The user can ask you to remember, update, or forget something at any time.
- Keep the existing hint: when first creating project memory, tell the user they
  can add `.keen/` to `.gitignore`. Do not change `.gitignore` yourself.

Add the current size and cap to each injected memory heading, e.g.
`# Project memory (9.2 KB of 16 KB)`, so the agent knows how much room is left
before it writes. When a file is empty or missing, still tell the agent where
each scope lives.

### Scope limits

- Exclude the tool from plan mode and from the adversary and btw read-only
  registries (`internal/appstate/state.go:288`).
- Do not register it for subagents (`internal/subagents/tool_factory.go`). Only
  the main agent manages memory.
- `write_file`/`edit_file` keep working on memory paths for manual edits, with
  the same guard and secret check.
- The load-time cap stays only as a fallback for hand-edited files. Change it to
  cut on a line boundary (never mid-rune) and keep the truncation note.

### Files

- `internal/memory/store.go` (new): `Add(content, section, entry)`,
  `Update(content, old, new)`, `Remove(content, old)`, `CapFor(scope)`, and size
  helpers. Tests in `store_test.go`.
- `internal/memory/memory.go`: cut on a line boundary, and add size/cap to the
  headings.
- `internal/tools/memory.go` (new): schema, input validation, diff, permission,
  atomic write. Tests in `memory_test.go`.
- `internal/tools/tool.go`: add the `MemoryToolName` constant.
- `internal/agentcore/setup_tools.go`: register the tool.
- `internal/appstate/state.go` and the plan-mode registry: exclude the tool.
- `internal/llm/systemprompt.go`: new memory policy. Update `systemprompt_test.go`.

### Tests

- Store: add to an existing or a new section, update and remove with zero,
  one, or multiple matches, multi-line entries with code blocks, cap boundary.
- Tool: secret refusal, over-cap refusal with the actionable message, diff
  emitted for each action, permission denied leaves the file unchanged, file
  and directories created on first write, size reported in the result.
- Prompt: memory headings include size/cap, policy text present.

## Later steps (not in this task)

1. **`memory.auto` config setting:** `off` (write only on user request), `ask`
   (the agent decides, every write needs approval; the default), `on` (no
   approval, a full-item notice is always shown).
2. **Injection safeguards for `on` mode:** still require approval for global
   saves, and for any save in a turn where the agent read web or MCP content.
3. **`/memory undo`:** reverse the last memory change.
4. **Load memory once per session:** read memory at session start and again on
   `/clear` or compaction, instead of on every request, so agent writes don't
   invalidate the prompt cache.
5. **Load-time secret filter:** leave out lines that look like secrets when
   building the prompt. This also covers hand-written and bash-written files.

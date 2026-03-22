# Tool Prompt Improvement Plan

## Goal

Improve Keen Code's built-in tool prompts so the model chooses tools more reliably, supplies better arguments, avoids misuse, and gets more predictable results from the existing tool surface.

This plan is based on:

- Keen Code tool definitions in `internal/tools/*.go`
- Comparable built-in tools in `../opencode/packages/opencode/src/tool/*`
- Comparable built-in tools in `../kimi-cli/src/kimi_cli/tools/*`

---

## Current Keen Code Tools

Keen Code currently exposes 6 tools:

| Tool | Current description | Current input schema |
|---|---|---|
| `read_file` | Read a UTF-8 text file after filesystem policy + user permission checks. | `path` |
| `write_file` | Write content to a file. Creates parent directories if needed. Overwrites existing files. | `path`, `content` |
| `edit_file` | Edit a file by replacing occurrences of a string. The file must already exist. | `path`, `oldString`, `newString`, `shouldReplaceAll?` |
| `glob` | Search for files matching a glob pattern after filesystem policy + user permission checks. | `pattern`, `path?` |
| `grep` | Search for text patterns in files recursively after filesystem policy + user permission checks. | `pattern`, `path?`, `include?`, `output_mode?` |
| `bash` | Execute bash commands in the terminal. Use `isDangerous=true` for commands that modify files or system state. Do not use this tool for writing or editing files. | `command`, `isDangerous?`, `summary?` |

### Key observation

Keen Code's current prompts are short and mechanically accurate, but they mostly describe what the tool does, not how the model should use it well. `opencode` and `kimi-cli` both put much more prompt budget into:

- when to use a tool
- when not to use it
- parameter-specific guidance
- common failure modes
- batching and parallelism guidance
- output-size and search-scope guidance

---

## Comparison Summary

## `read_file`

### Keen Code today

- Minimal description.
- Schema only has `path`.
- No pagination, no partial-read controls, no usage tips.

### `opencode`

- Much richer description text.
- Supports `filePath`, `offset`, `limit`.
- Explains line-numbered output, truncation, directory reads, image/PDF behavior, and when to use `grep` or `glob` first.

### `kimi-cli`

- Supports `path`, `line_offset`, `n_lines`.
- Description explains line numbering, max lines, truncation, parallel reads, and when to prefer `Grep`.

### Gap

Keen Code's read tool is underspecified for large files and does not teach the model a good exploration workflow.

---

## `write_file`

### Keen Code today

- Very short description.
- Always overwrite behavior, but the description does not emphasize that strongly enough.
- Schema only has `path`, `content`.

### `opencode`

- No direct dedicated write tool in the same style.
- File creation is folded into `edit` via empty old content and reinforced by strong edit guidance.

### `kimi-cli`

- Supports `path`, `content`, `mode`.
- Description tells the model to be cautious and to chunk long writes across multiple calls.

### Gap

Keen Code's write tool is usable, but the prompt surface does not help the model avoid accidental full-file overwrites or decide when `edit_file` is safer.

---

## `edit_file`

### Keen Code today

- Accurate but minimal description.
- Schema supports only one replacement per call.
- No guidance about uniqueness, preserving exact text, or when to use replace-all.

### `opencode`

- Strongest reference point.
- Description explains exact-match behavior, requirement to read first, uniqueness issues, line-number pitfalls, and when to use `replaceAll`.
- Schema is still compact: `filePath`, `oldString`, `newString`, `replaceAll?`.

### `kimi-cli`

- Supports a single edit or a list of edits.
- Description explicitly says to prefer this tool over full rewrites and shell-based edits.

### Gap

This is the biggest prompt gap in Keen Code. The current tool description does not teach the model how exact-string replacement fails in practice, which will increase retries and bad edits.

---

## `glob`

### Keen Code today

- Minimal description.
- Schema supports `pattern`, `path?`.
- No guidance about safe breadth, examples, or when to omit `path`.

### `opencode`

- Strong usage guidance.
- Explicitly says the tool is fast, gives pattern examples, recommends batching searches, and warns that open-ended exploration may need a different strategy.

### `kimi-cli`

- Adds `directory` and `include_dirs`.
- Description contains concrete good/bad examples and strongly warns against overly broad recursive patterns like leading `**`.

### Gap

Keen Code's prompt does not help the model write selective glob patterns, so it will be more likely to run broad searches and hit result caps.

---

## `grep`

### Keen Code today

- Better than most Keen tools already.
- Schema supports `pattern`, `path?`, `include?`, `output_mode?`.
- Still lacks guidance on regex syntax, search scoping, examples, and when to choose `file` vs `content`.

### `opencode`

- Strong usage guidance.
- Explicitly distinguishes content search from counting and suggests using shell `rg` only for counting/advanced cases.

### `kimi-cli`

- Richest schema.
- Supports file path or directory path, glob filtering, file type filtering, context lines, case-insensitive mode, multiline mode, head limits, and multiple output modes.
- Description strongly pushes the model toward Grep instead of shell `grep`/`rg`.

### Gap

Keen Code has decent core parameters, but its prompt does not teach the model how to form efficient searches or how to escalate when the simple schema is not enough.

---

## `bash`

### Keen Code today

- Minimal description with one important rule: do not use it for file writes/edits.
- Schema has `command`, `isDangerous?`, `summary?`.
- No `workdir`, no timeout parameter, and almost no guidance on quoting, chaining, or preferred use cases.

### `opencode`

- Much more developed.
- Supports `command`, `timeout?`, `workdir?`, `description`.
- Description teaches when bash is appropriate, when to use dedicated tools instead, how to avoid `cd &&`, how to batch commands, and how to handle long output.

### `kimi-cli`

- Similar rich guidance through `Shell`.
- Supports `command`, `timeout`, `run_in_background`, `description`.
- Description explains shell isolation, safety, timeout expectations, chaining, and long-running commands.

### Gap

Keen Code's bash prompt is too thin for a high-risk tool. It does not sufficiently steer command composition or tool avoidance, and it lacks explicit execution control for long-running commands.

---

## Main Problems To Fix

1. Descriptions are mostly capability statements, not usage instructions.
2. Parameter descriptions are accurate but too generic to shape model behavior.
3. Tools do not form a coherent workflow as a set.
4. Large-file and large-search ergonomics are weak, especially for `read_file`.
5. `edit_file` and `bash` lack the strongest failure-prevention guidance.
6. Naming is a little inconsistent across tools: `path` sometimes means file, sometimes base directory; `shouldReplaceAll` is less standard than `replaceAll`.

---

## Recommended Improvement Strategy

## Phase 1: Upgrade Descriptions Without Changing Behavior

First improve prompt quality using only `Description()` text. This gives the fastest win with the lowest implementation risk.

### Shared description pattern

Adopt the same structure for every tool:

1. One-sentence capability summary
2. "Use this when" guidance
3. "Do not use this when" guidance
4. Parameter-specific notes
5. Common failure modes or performance advice

### Per-tool description upgrades

#### `read_file`

- Explain that it reads UTF-8 text only.
- Explicitly tell the model to use `glob` when the filename is unknown.
- Explicitly tell the model to use `grep` when searching for content.
- Mention the 10 MB limit and binary/invalid-UTF-8 rejection.
- If pagination is not yet added, say so clearly to avoid the model expecting partial reads.

#### `write_file`

- Emphasize full overwrite behavior.
- Tell the model to prefer `edit_file` for targeted modifications to existing files.
- Tell the model to verify the target path before overwriting.
- Note that parent directories are created automatically.

#### `edit_file`

- Explain exact-string match behavior.
- Tell the model to read the file first.
- Explain that edits fail if `oldString` is not found.
- Explain that `shouldReplaceAll` should only be used when all matches are intended.
- Encourage including enough surrounding context in `oldString` to make the match unique.

#### `glob`

- Give concrete pattern examples.
- Warn against overly broad patterns.
- Clarify that `path` is the base directory and can be omitted.
- Tell the model to batch multiple likely-useful patterns in parallel when exploring.

#### `grep`

- Explain that `pattern` is a regular expression.
- Clarify `output_mode=file` vs `output_mode=content`.
- Give examples for `include`.
- Tell the model to narrow `path` and `include` when possible.
- Tell the model to use `bash` with `rg` only for advanced needs not covered by the tool.

#### `bash`

- Make it explicit that this is the fallback tool, not the default.
- Tell the model not to use bash for reading, writing, editing, globbing, or grep-like search when a dedicated tool exists.
- Add guidance for quoting paths with spaces.
- Add guidance for chaining dependent commands vs batching independent commands.
- Tell the model to mark state-changing commands with `isDangerous=true`.

---

## Phase 2: Improve Input Schemas Where the UX Gain Is Clear

These changes are worth doing because both comparison repos show real prompt and usability benefits.

### `read_file`

Add:

- `offset` or `line_offset`
- `limit` or `n_lines`

Recommendation:

- Prefer `offset` + `limit` for consistency with `opencode`, or `line_offset` + `n_lines` for explicitness.
- I would favor `offset` + `limit` if you want a compact schema, and `line_offset` + `n_lines` if you want self-documenting names.

Why:

- Large-file reads become practical.
- The model can page through files instead of switching to bash.
- This aligns Keen Code with both comparison repos.

### `write_file`

Consider adding:

- `mode`: `overwrite` or `append`

Why:

- `kimi-cli` shows this is useful for controlled long writes.
- It reduces the need for bash when appending generated content.

Tradeoff:

- This is optional. If Keen Code wants to keep write semantics simple, stronger description text may be enough for now.

### `edit_file`

Recommended changes:

- Rename `shouldReplaceAll` to `replaceAll`
- Optionally support `edits: []` for multiple exact replacements in one call

Why:

- `replaceAll` is the more standard parameter name.
- Multiple edits per call reduce tool round trips.

Tradeoff:

- Multi-edit increases implementation complexity slightly.
- Renaming requires compatibility handling if older prompts or tests refer to `shouldReplaceAll`.

### `glob`

Consider adding:

- `include_dirs`

Why:

- `kimi-cli` shows value here.
- Sometimes the model wants files only; sometimes directory discovery matters.

Tradeoff:

- Lower priority than `read_file` pagination or `edit_file` cleanup.

### `grep`

Possible additions:

- `ignore_case`
- `type`
- `before_context`
- `after_context`
- `context`

Why:

- These are high-leverage search arguments.
- `kimi-cli` demonstrates they are useful.

Tradeoff:

- Do not add too many at once if the current implementation is intentionally simple.
- At minimum, improve prompt guidance before expanding the schema.

### `bash`

Recommended additions:

- `workdir`
- `timeout`

Why:

- Both comparison repos support better execution control.
- `workdir` prevents bad `cd && ...` habits.
- `timeout` makes long-running command behavior more explicit.
- Keen Code does not need background execution for this step; a foreground timeout is enough.

Recommended timeout behavior:

- Add `timeout` to the bash tool schema.
- Default it to `180` seconds.
- Kill the command if it exceeds the timeout.
- Return a clear timeout error/result so the model knows the command did not finish.
- Do not add background execution support as part of this change.

---

## Phase 3: Make the Tool Set Work as a System

The descriptions should cross-reference other tools consistently.

### Cross-tool routing rules to encode

- Unknown file name: use `glob`
- Known file, need contents: use `read_file`
- Need to find text in files: use `grep`
- Need targeted modification: use `edit_file`
- Need full-file creation or replacement: use `write_file`
- Need shell-native operations: use `bash`

### Why this matters

`opencode` and `kimi-cli` both use tool descriptions to teach routing, not just isolated tool capabilities. Keen Code should do the same so the model follows a stable workflow instead of improvising with bash.

---

## Phase 4: Tighten Names and Schema Language

Standardize parameter names and descriptions where possible.

### Recommended cleanup

- Use `replaceAll` instead of `shouldReplaceAll`
- Use a consistent meaning for `path`
  - file tools: target file path
  - search tools: base directory path
- In search tools, explicitly say "omit to use the working directory"
- Make descriptions mention whether absolute and relative paths are both accepted

This will reduce ambiguity in model-generated tool calls.

---

## Phase 5: Add Tests That Lock Prompt Quality In

Current tests only check that descriptions and schemas exist. Add stronger assertions.

### New tests to add

- Description contains critical routing guidance for each tool
- Schema includes new parameters where adopted
- Schema enums and required fields remain stable
- Backward compatibility behavior if `shouldReplaceAll` is renamed

### Why

Without prompt-focused tests, these strings will regress over time during refactors.

---

## Proposed Implementation Order

1. Upgrade `edit_file` description
2. Upgrade `bash` description
3. Upgrade `read_file` schema and description with pagination
4. Upgrade `glob` and `grep` descriptions
5. Decide whether `write_file` gets `mode`
6. Decide whether `edit_file` gets multi-edit support
7. Add prompt-focused tests for all tools

This order prioritizes the tools where prompt quality most directly affects correctness and safety.

---

## Concrete Recommendations

## Must-do

- Expand every tool description substantially
- Add explicit cross-tool guidance
- Add `read_file` pagination parameters
- Rename or alias `shouldReplaceAll` to `replaceAll`
- Add `bash.workdir`
- Add `bash.timeout` with a default of `180s` and kill-on-timeout behavior
- Add tests that assert important description content

## Good next step

- Add `write_file.mode`
- Add richer `grep` options such as `ignore_case` and context controls

## Optional

- Add multi-edit support to `edit_file`
- Add `glob.include_dirs`

---

## Suggested Success Criteria

The tool prompt redesign is successful if:

- the model uses `bash` less often for file-oriented work
- the model uses `read_file` and `grep` more selectively on large codebases
- `edit_file` failures from weak `oldString` matches drop noticeably
- fewer corrective retries are needed for `glob` and `grep`
- tests protect the prompt contract

---

## Source Files Reviewed

### Keen Code

- `internal/tools/read_file.go`
- `internal/tools/write_file.go`
- `internal/tools/edit_file.go`
- `internal/tools/glob.go`
- `internal/tools/grep.go`
- `internal/tools/bash.go`

### opencode

- `../opencode/packages/opencode/src/tool/read.ts`
- `../opencode/packages/opencode/src/tool/read.txt`
- `../opencode/packages/opencode/src/tool/edit.ts`
- `../opencode/packages/opencode/src/tool/edit.txt`
- `../opencode/packages/opencode/src/tool/glob.ts`
- `../opencode/packages/opencode/src/tool/glob.txt`
- `../opencode/packages/opencode/src/tool/grep.ts`
- `../opencode/packages/opencode/src/tool/grep.txt`
- `../opencode/packages/opencode/src/tool/bash.ts`
- `../opencode/packages/opencode/src/tool/bash.txt`
- `../opencode/packages/opencode/src/tool/ls.ts`
- `../opencode/packages/opencode/src/tool/ls.txt`

### kimi-cli

- `../kimi-cli/src/kimi_cli/tools/file/read.py`
- `../kimi-cli/src/kimi_cli/tools/file/read.md`
- `../kimi-cli/src/kimi_cli/tools/file/write.py`
- `../kimi-cli/src/kimi_cli/tools/file/write.md`
- `../kimi-cli/src/kimi_cli/tools/file/replace.py`
- `../kimi-cli/src/kimi_cli/tools/file/replace.md`
- `../kimi-cli/src/kimi_cli/tools/file/glob.py`
- `../kimi-cli/src/kimi_cli/tools/file/glob.md`
- `../kimi-cli/src/kimi_cli/tools/file/grep_local.py`
- `../kimi-cli/src/kimi_cli/tools/file/grep.md`
- `../kimi-cli/src/kimi_cli/tools/shell/__init__.py`
- `../kimi-cli/src/kimi_cli/tools/shell/bash.md`
- `../kimi-cli/src/kimi_cli/tools/todo/__init__.py`
- `../kimi-cli/src/kimi_cli/tools/todo/set_todo_list.md`

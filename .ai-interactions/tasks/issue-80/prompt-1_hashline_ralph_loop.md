You are implementing the Hashline Edit plan in this repository.

Authoritative plan: `.ai-interactions/tasks/issue-80/output-1_hashline.md`.
Progress checklist: `ralph/hashline/PROGRESS.md`.

Work autonomously, but complete **exactly one** unchecked task per invocation, in order. Do not begin a later task until all prior tasks are checked off. Treat a task as incomplete unless its stated implementation and test requirements are satisfied.

## Required workflow for this invocation

1. Read the authoritative plan and the progress checklist.
2. Inspect `git status --short`, the current implementation, and tests relevant to the next task.
3. Implement only that task. Make minimal, idiomatic changes that preserve current behavior outside the task.
4. Run `gofmt` on every modified Go file and run `go mod tidy` after the change.
5. Run focused tests for the changed package(s). If this completes the final task, also run `go test -race ./...`.
6. Update `ralph/hashline/PROGRESS.md` in the same worktree: check off the completed task; add a concise note with changed files and tests run; record any blocker instead of claiming completion.
7. Inspect `git diff --check` and `git status --short` before responding.

## Locked design constraints

- Modify `read_file` and `edit_file` in place; `write_file` retains its public schema.
- Use `N:HASH` anchors: 1-based line number plus exactly three lowercase hex characters from FNV-1a 32-bit over the line's raw content bytes.
- Do **not** compute or require a file hash.
- Do **not** use context hashes, anchor relocation, snapshot merge, a `replace_text` fallback, returned anchors for chaining, or parallel tool execution.
- All same-file changes known at edit time belong in one `edit_file` call's `ops` array. Validate every op against one immutable pre-edit snapshot, then apply all validated ops bottom-up and atomically.
- Keep the existing full-file unified diff preview through `computeEditDiff` and `DiffEmitter`; it is CLI-only and must remain plain (without hashes).
- A later same-file edit requires another `read_file`.
- `grep` fast-follow adds `line_hash` only to structured content matches; preserve existing fields and behavior.
- Do not add dependencies unless the plan explicitly requires one (it does not).

## Completion protocol

Do not emit the completion marker until all Tasks 0–9 in the plan are done, all checklist entries are checked, no blocker remains, `go mod tidy` has passed, `gofmt` has been run on modified Go files, and `go test -race ./...` passes.

When and only when everything is complete, end your final response with this exact standalone line:

<HASHLINE_IMPLEMENTATION_COMPLETE>

Otherwise, summarize the one task completed, tests run, and the next unchecked task. Do not emit the marker.

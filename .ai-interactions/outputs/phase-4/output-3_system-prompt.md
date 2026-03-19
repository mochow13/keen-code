# System Prompt Plan

## Goal

Give Keen Code a system prompt that is assembled at call time from three layers:

1. **Static identity and behaviour** — who Keen is, how it writes, what it never does
2. **Dynamic environment block** — working directory, OS, date, git status, top-level directory listing
3. **Project instructions** — contents of `AGENTS.md` (or `CLAUDE.md`) found by walking up from `workingDir`

The result is a single `llm.RoleSystem` message prepended to every `StreamChat` call.

---

## Why These Three Layers

| Layer | What the model learns immediately | Reference |
|---|---|---|
| Static | Identity, tone, tool rules, safety limits, git rules | opencode `anthropic.txt` / `qwen.txt` |
| Dynamic env | Where it is, what OS, today's date, is it a git repo, project shape | opencode `system.ts` `environment()` |
| Project instructions | Build commands, test commands, coding style, team conventions | kimi-cli `${KIMI_AGENTS_MD}` / opencode `InstructionPrompt.system()` |

Without the env block the model has to call a tool just to find out where it is. Without the project instructions block, users who maintain an `AGENTS.md` get no benefit from it. Without a static identity block the model has no baseline behaviour rule to fall back on.

---

## New Files

```
configs/prompts/
  systemprompt.go        ← builder: Build(workingDir) string
  systemprompt_test.go   ← unit tests for all three layers
```

No other packages are introduced. The prompt is self-contained inside the `prompts` package.

---

## `systemprompt.go` — Full Design

### Package-level constants

```go
package llm

// staticPrompt is the fixed identity and behaviour section.
// It is embedded at compile time so there is no runtime file I/O.
const staticPrompt = `You are Keen Code, an expert coding agent running in your terminal.
...`
```

Keeping it as a Go `const` (not a file embed) is the simplest approach for now. It avoids a `//go:embed` dependency and keeps the single responsibility clear: this file builds the prompt, nothing else.

### `Build` function — the only public surface

```go
// Build assembles the full system prompt for a session.
// workingDir is the directory keen was launched from.
func Build(workingDir string) string
```

Called once per `StreamChat` invocation from `AppState.StreamChat`.

### Internal helpers

```go
// envBlock returns the <env>…</env> XML block.
func envBlock(workingDir string) string

// dirListing returns a compact top-level listing of workingDir (≤ 40 entries).
// Returns "" if the directory cannot be read.
func dirListing(workingDir string) string

// projectInstructions walks up from workingDir looking for AGENTS.md or CLAUDE.md.
// Returns "" if neither file is found in the entire upward path.
func projectInstructions(workingDir string) string

// isGitRepo checks whether workingDir is inside a git repository.
func isGitRepo(workingDir string) bool

// findUpward walks from dir toward the filesystem root, stopping at the first
// file whose base name matches any of the provided candidates.
// Returns ("", "") if not found.
func findUpward(dir string, candidates []string) (path string, content string)
```

---

## Static Prompt Content

The static section is the largest part. Here is the exact text, broken into named sections so the model weights them correctly:

```
You are Keen Code, an expert coding agent running in terminal environment.

You help with software engineering tasks: fixing bugs, writing new features,
refactoring code, explaining code, exploring codebases, writing tests, and more.

# Tone and style
- Be concise and direct. Output is displayed on a CLI in a monospace font.
  Use GitHub-flavored markdown.
- No emojis unless the user explicitly asks for them.
- No unnecessary preamble or postamble. Do not summarise what you just did.
  Do not explain a code block you are about to write.
- One-word or one-line answers are fine when that is all the question needs.
- Never use bash or code comments as a communication channel — write to the
  user in your response text only.

# Doing tasks
- Explore before acting. Use grep/glob/read_file to understand the codebase
  before making changes.
- Follow existing conventions: mimic the style, naming, and patterns already
  in the project.
- Never assume a library is available. Check go.mod, package.json, pom.xml, or the
  relevant manifest before writing code that uses a dependency.
- Make minimal changes. Prefer editing an existing file to creating a new one.
- Verify your work. After making changes, run the project's test command if
  you know it. If you do not know it, check AGENTS.md, the README.md, or ask.

# Tool usage
- Prefer specialised tools over bash for file operations:
    read_file  → reading file contents
    write_file → creating new files
    edit_file  → modifying existing files
    glob       → listing files by pattern
    grep       → searching file contents
    bash       → shell commands that have no dedicated tool
- Run independent tool calls in parallel where possible.
- Reference code as `file_path:line_number` so the user can jump straight
  to the source.

# Git rules
- Never run git commit, git push, git reset, or git rebase unless the user
  explicitly asks you to.

# Safety
- Never introduce code that logs, exposes, or commits secrets or API keys.
- Refuse requests to write malicious code, even framed as educational.
- Before working on a file, consider what the code is supposed to do. If it
  looks malicious, refuse.
```

**Rationale for each section:**

- `Tone and style` — prevents the most common LLM anti-patterns in CLI contexts: verbosity, emoji spam, self-narration. Maps directly to opencode `qwen.txt` "minimize output tokens" and "no preamble/postamble".
- `Doing tasks` — the explore-first, minimal-changes, follow-conventions workflow that makes the agent actually useful on real codebases. From opencode `qwen.txt` "Following conventions" and kimi-cli "Make MINIMAL changes".
- `Tool usage` — makes the agent use the right tool for the right job and run parallel calls. From opencode "use dedicated tools" policy and kimi-cli "HIGHLY RECOMMENDED to make them in parallel".
- `Git rules` — the most-violated safety rule in every coding agent. From kimi-cli "DO NOT run git commit…" and opencode "NEVER commit unless user explicitly asks".
- `Safety` — refusal rules for malicious code. From opencode `qwen.txt` "IMPORTANT: Refuse to write code … that may be used maliciously".

---

## Dynamic Environment Block

Assembled at call time using `runtime.GOOS`, `os.Getwd`-style resolution, `time.Now()`, and a small `exec.Command("git", "rev-parse", ...)` check.

```
<env>
  Working directory: /Users/alice/projects/my-api
  Platform: darwin
  Today's date: 2025-07-15
  Is git repo: yes
</env>

Top-level project structure:
```
cmd/
internal/
  config/
  llm/
  tools/
go.mod
go.sum
README.md
AGENTS.md
```
```

**Design decisions:**

- XML-tag wrapping (`<env>`) follows opencode's convention. It gives the model a clear anchor to reference.
- Directory listing is capped at 40 entries (top level only, no recursion) to avoid token bloat. kimi-cli does a flat `ls` output; opencode comments out a deeper tree (`ripgrep --tree`) because it is too large. 40 top-level entries is a safe middle ground.
- The listing is produced with `os.ReadDir(workingDir)`, which is fast and needs no shell. Directories get a trailing `/`.
- If `workingDir` cannot be read, the directory listing section is omitted entirely — the rest of the prompt is still valid.
- The `isGitRepo` check uses `exec.Command("git", "rev-parse", "--is-inside-work-tree")` — one tiny subprocess, no library dependency.

---

## Project Instructions Block (`AGENTS.md` loading)

```
# Project Instructions (from /Users/alice/projects/my-api/AGENTS.md)

## Keen Code
CLI-based coding agent powered by AI using Firebase Genkit for LLM interactions.

## Architecture
...

## Important Guidelines
- Minimal comments — Only when strictly necessary
- Test critical paths — Not aiming for 100% coverage
- Always run the tests — After each change
```

**`findUpward` algorithm:**

```
start:  workingDir  (e.g. /Users/alice/projects/my-api)
check:  /Users/alice/projects/my-api/AGENTS.md  → found → done
                         or
check:  /Users/alice/projects/my-api/AGENTS.md  → not found
check:  /Users/alice/projects/AGENTS.md         → not found
check:  /Users/alice/AGENTS.md                  → found → done
stop at filesystem root or home directory
```

- Candidate file names searched in order: `AGENTS.md`, `CLAUDE.md`
- The walk stops at the filesystem root — it will not read `/AGENTS.md`
- If the file is found but cannot be read (permissions), the section is silently omitted
- If the file content is empty, the section is omitted

**Why walk upward?**
A user may `cd` into a subdirectory of their project. The `AGENTS.md` is at the repo root. Walking upward finds it without requiring the user to always launch from the root. Both opencode (`InstructionPrompt.systemPaths` + `Filesystem.findUp`) and kimi-cli (`${KIMI_AGENTS_MD}`) use this pattern.

**Size guard:**
If `AGENTS.md` exceeds 8 KB, include only the first 8 KB and append a note: `[truncated — full file at <path>]`. This prevents a single large AGENTS.md from dominating the context window.

---

## Integration Point — `AppState.StreamChat`

```go
// internal/cli/repl/state.go

func (s *AppState) StreamChat(ctx context.Context, cfg *config.ResolvedConfig) (<-chan llm.StreamEvent, error) {
    if s.llmClient == nil {
        return nil, nil
    }

    systemMsg := llm.Message{
        Role:    llm.RoleSystem,
        Content: llm.Build(s.workingDir),
    }

    messages := append([]llm.Message{systemMsg}, s.GetMessages()...)
    return s.llmClient.StreamChat(ctx, messages, s.toolRegistry)
}
```

**Required change to `AppState`:**

`AppState` currently holds no `workingDir`. Add it:

```go
type AppState struct {
    messages     []llm.Message
    llmClient    llm.LLMClient
    toolRegistry *tools.Registry
    workingDir   string          // ← new field
}

func NewAppState(client llm.LLMClient, workingDir string) *AppState {
    return &AppState{
        messages:     []llm.Message{},
        llmClient:    client,
        toolRegistry: tools.NewRegistry(),
        workingDir:   workingDir,
    }
}
```

`workingDir` is already available in `initialModel` via `ctx.workingDir` — it is passed to `setupToolRegistry` today. Adding it to `AppState` requires a one-line change at the call site in `repl.go`:

```go
// internal/cli/repl/repl.go  initialModel()
appState := NewAppState(llmClient, ctx.workingDir)   // was: NewAppState(llmClient)
```

The system message is **not stored** in `AppState.messages`. It is prepended fresh on every `StreamChat` call. This means:
- `ClearMessages()` does not accidentally wipe the system prompt
- The env block (`today's date`, directory listing) stays current if a session runs across midnight or the user changes directory
- The `AGENTS.md` content reflects any edits the agent itself may have made during the session

---

## `systemprompt_test.go` — Test Coverage

| Test | What it checks |
|---|---|
| `TestBuild_ContainsIdentity` | Output contains "Keen Code" |
| `TestBuild_ContainsWorkingDir` | Output contains the workingDir path |
| `TestBuild_ContainsPlatform` | Output contains `runtime.GOOS` |
| `TestBuild_ContainsDate` | Output contains today's date in `2006-01-02` format |
| `TestBuild_GitRepo` | When a `.git` dir is present, env block says `yes` |
| `TestBuild_NoGitRepo` | When no `.git` dir, env block says `no` |
| `TestBuild_DirListing` | Output contains known files from a temp dir |
| `TestBuild_DirListing_Empty` | Empty dir produces no listing section (no panic) |
| `TestBuild_DirListing_Unreadable` | Unreadable dir produces no listing section (no panic) |
| `TestBuild_AgentsMd_Found` | `AGENTS.md` in workingDir appears in output |
| `TestBuild_AgentsMd_WalkUp` | `AGENTS.md` one level up is found |
| `TestBuild_ClaudeMd_Fallback` | `CLAUDE.md` used when no `AGENTS.md` exists |
| `TestBuild_NoInstructionFile` | No file found → no project instructions section |
| `TestBuild_AgentsMd_Truncation` | File > 8 KB is truncated with note |
| `TestBuild_AgentsMd_Empty` | Empty file → section omitted |
| `TestBuild_SystemMessage_NotStored` | `AppState.GetMessages()` never contains a system role entry |
| `TestBuild_FreshOnEachCall` | Two calls to `Build` with same args produce identical output structure |

---

## File Changeset

| File | Change |
|---|---|
| `internal/llm/systemprompt.go` | **New** — `Build()`, `envBlock()`, `dirListing()`, `projectInstructions()`, `findUpward()`, `isGitRepo()`, `staticPrompt` const |
| `internal/llm/systemprompt_test.go` | **New** — full test suite (see table above) |
| `internal/cli/repl/state.go` | **Edit** — add `workingDir string` field to `AppState`; update `NewAppState` signature; update `StreamChat` to prepend system message |
| `internal/cli/repl/state_test.go` | **Edit** — update `NewAppState` call sites |
| `internal/cli/repl/repl.go` | **Edit** — pass `ctx.workingDir` to `NewAppState` |

No changes to any LLM client files (`openai.go`, `genkit.go`, `openai_responses.go`). The system message is just another `llm.Message` with `RoleSystem` — all three clients already handle it via their existing `toOpenAIMessages` / `toGenkitMessages` / `toOpenAIResponseInput` converters.

---

## Prompt Assembly Order

```
[system message content]
─────────────────────────────────
1. staticPrompt          (identity, tone, tasks, tools, git, safety)
2. "\n\n" + envBlock()   (<env> working dir, platform, date, git flag </env>
                          + top-level directory listing)
3. "\n\n" + projectInstructions()   (# Project Instructions … AGENTS.md content)
─────────────────────────────────
[user / assistant message history]
```

---

## Token Budget Estimate

| Section | Typical tokens |
|---|---|
| Static prompt | ~450 |
| Env block | ~60 |
| Dir listing (40 entries) | ~120 |
| AGENTS.md (8 KB max) | ~2 000 |
| **Total** | **~2 600** |

At 200 k context (DeepSeek / Kimi), this is ~1.3 % of the window. Negligible.

---

## What This Does Not Include (Deferred)

- **Per-model routing** — a single prompt works well enough for all providers currently in keen-code. Add model-specific variants only once a concrete behavioural difference is observed.
- **Global `AGENTS.md`** — reading from `~/.keen/AGENTS.md` (opencode-style global instructions). Useful but not required for the first iteration.
- **`<system-reminder>` injection** — injecting mid-conversation reminders (opencode plan mode). Deferred to a future plan-mode feature.
- **Subdirectory `AGENTS.md` files** — only the first file found walking upward is used. Loading multiple nested files adds complexity with marginal benefit at this stage.

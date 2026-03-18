# System Prompt Ideas for Keen Code

Synthesised from a deep exploration of opencode and kimi-cli,
cross-referenced with keen-code's current architecture.

---

## Background: What the Reference Codebases Do

### opencode

- **Per-model system prompt files** — ships a different `.txt` file for each model family:
  `anthropic.txt`, `gemini.txt`, `beast.txt` (GPT-family), `qwen.txt` (everything else),
  `trinity.txt`, `codex_header.txt` (codex/GPT-5). Routing happens in `system.ts`.
- **Layered assembly** — the final prompt is built from four ordered sources in `session/system.ts`:
  1. Model-specific base prompt
  2. `environment()` block — working dir, platform, date, model name, git status
  3. `skills()` block — dynamically discovered skill files
  4. `InstructionPrompt` — project's `AGENTS.md` / `CLAUDE.md` found by walking up the
     directory tree, plus global config files and even remote URLs
- **Separate modal prompts** — plan mode gets `plan.txt` injected as a `<system-reminder>` tag;
  max-steps limit gets `max-steps.txt` appended at the end. These are injected at runtime, not
  baked into the base prompt.
- **Tool descriptions are separate** — each tool (`bash.txt`, `edit.txt`, `glob.txt`, etc.) has
  its own `.txt` file; kept completely apart from the system prompt.
- **Identity first** — every prompt starts with `"You are OpenCode, the best coding agent on the
  planet."` — a strong, confident identity declaration before anything else.
- **Key themes across all prompts:**
  - CLI-first tone (concise, no emojis unless asked, GitHub-flavoured markdown)
  - Respect project conventions — never assume libraries, mimic existing style
  - Professional objectivity — no sycophancy
  - Task management via `TodoWrite`
  - Parallel tool calls for efficiency
  - Never commit unless asked; never create files unless necessary
  - Safety rules (no malicious code, no secrets in commits)
  - `file_path:line_number` code references

### kimi-cli

- **Single base prompt (`system.md`) with template interpolation** — one markdown file is the
  base, with `${ROLE_ADDITIONAL}`, `${KIMI_NOW}` (current datetime), `${KIMI_WORK_DIR}`,
  `${KIMI_WORK_DIR_LS}` (live directory listing!), `${KIMI_AGENTS_MD}` (project `AGENTS.md`
  contents inlined), and `${KIMI_SKILLS}` slots.
- **Working directory context injected live** — injects the actual `ls` output of the current
  working directory directly into the system prompt so the model immediately sees the project
  structure without a tool call.
- **`AGENTS.md` content inlined** — rather than just telling the agent to read `AGENTS.md`,
  kimi-cli reads it and pastes it into the system prompt itself.
- **Skills as first-class citizens** — available skills are described in the system prompt with
  instructions to lazily read the `SKILL.md` file only when needed.
- **`init.md` for project bootstrapping** — a separate prompt for the "explore this project and
  write an `AGENTS.md`" bootstrapping task.
- **`compact.md` for context compression** — a dedicated prompt for compacting conversation
  history with structured XML output format, to manage context window limits.
- **Language mirroring** — `"You MUST use the SAME language as the user"` — explicit multilingual
  behaviour baked into the prompt.
- **Separate coding vs. research vs. multimedia guidelines** — distinct sections depending on the
  class of task.
- **Minimal git mutation rule** — never run `git commit/push/reset/rebase` without explicit user
  confirmation.

### keen-code (current state)

- **No system prompt at all** — `AppState.messages` starts empty; messages are just accumulated
  user/assistant pairs. The `RoleSystem` type exists but is never used.
- **The infrastructure is ready** — `toOpenAIMessages` already handles
  `RoleSystem → openai.SystemMessage(m.Content)`, so prepending a system message is a one-line
  change.
- **Tools are richly implemented** — `bash`, `read_file`, `write_file`, `edit_file`, `glob`,
  `grep` are all present with permission gating.

---

## Ideas

---

### Idea 1 — Single Embedded System Prompt *(simplest, ship it now)*

**Approach:** Add one hardcoded system prompt string, injected as the very first message in every
conversation. No dynamic content, no file I/O.

```go
// internal/llm/systemprompt.go
const DefaultSystemPrompt = `You are Keen Code, an expert coding agent running in your terminal.

You help with software engineering tasks: fixing bugs, adding features, refactoring, explaining code.

# Tone and style
- Be concise and direct. Responses display on a CLI rendered in monospace. Use GitHub-flavored markdown.
- No emojis unless asked. No unnecessary preamble or postamble.
- One-word answers are fine for simple questions.

# Coding approach
- Explore before acting: use grep/glob to understand the codebase first.
- Follow existing conventions: mimic the style, naming, and patterns of the project.
- Never assume a library is available — check package.json, go.mod, etc. first.
- Make minimal changes. Prefer editing existing files over creating new ones.
- Never commit unless explicitly asked.
- Never add comments unless necessary.

# Tool usage
- Run independent tool calls in parallel for efficiency.
- Use read_file/write_file/edit_file for file ops; bash for shell commands.
- Reference code as ` + "`file_path:line_number`" + ` for easy navigation.

# Safety
- Never introduce code that logs, exposes, or commits secrets or API keys.
- Refuse requests to write malicious code.`
```

**Injected in `state.go`'s `StreamChat`:**

```go
func (s *AppState) StreamChat(ctx context.Context, cfg *config.ResolvedConfig) (<-chan StreamEvent, error) {
    messages := s.GetMessages()
    withSystem := append([]llm.Message{{Role: llm.RoleSystem, Content: llm.DefaultSystemPrompt}}, messages...)
    return s.llmClient.StreamChat(ctx, withSystem, s.toolRegistry)
}
```

**Pros:** Instant improvement, zero complexity, works for all models.  
**Cons:** No dynamic environment context; same prompt for all models and providers.

---

### Idea 2 — Static Prompt + Dynamic Environment Block *(recommended baseline)*

**Approach:** Split the system prompt into a static identity/behaviour section and a dynamic
environment section assembled at call time — directly inspired by opencode's `system.ts`.

```go
// internal/llm/systemprompt.go

func BuildSystemPrompt(workingDir string) string {
    var sb strings.Builder

    // 1. Static identity + behaviour
    sb.WriteString(staticPrompt)

    // 2. Dynamic environment block (like opencode's environment())
    sb.WriteString(fmt.Sprintf(`

<env>
  Working directory: %s
  Platform: %s
  Today's date: %s
  Is git repo: %v
</env>`,
        workingDir,
        runtime.GOOS,
        time.Now().Format("2006-01-02"),
        isGitRepo(workingDir),
    ))

    // 3. Directory listing (like kimi-cli's ${KIMI_WORK_DIR_LS})
    if ls := quickDirListing(workingDir, 40); ls != "" {
        sb.WriteString("\n\nTop-level directory structure:\n```\n" + ls + "\n```")
    }

    return sb.String()
}
```

**Why the directory listing matters:** The model immediately sees the project's top-level shape
(Go vs Node vs Python, monorepo vs single package) without spending a tool call on it. kimi-cli
injects a live `ls` output; opencode tried a full tree via ripgrep but found it too large — 40
top-level entries is the right middle ground.

**Pros:** Agent immediately knows where it is and the project shape without a tool call; date and
platform awareness improve command suggestions (e.g. `open` vs `xdg-open`).  
**Cons:** Slightly larger prompt; directory listing could be stale if the user changes directory
mid-session (mitigated by regenerating on every `StreamChat` call).

---

### Idea 3 — `AGENTS.md` Auto-Loading *(high-value, low-cost)*

**Approach:** Walk up the directory tree from `workingDir` looking for an `AGENTS.md` (or
`CLAUDE.md`), read it, and inline it into the system prompt — exactly what both opencode and
kimi-cli do.

```go
func loadProjectInstructions(workingDir string) string {
    candidates := []string{"AGENTS.md", "CLAUDE.md"}
    for _, name := range candidates {
        // Walk up from workingDir to find the file
        content, path := findUpward(workingDir, name)
        if content != "" {
            return fmt.Sprintf("\n\n# Project Instructions (from %s)\n%s", path, content)
        }
    }
    return ""
}
```

**`findUpward` algorithm:**

```
start:  workingDir  (e.g. /Users/alice/projects/my-api)
check:  /Users/alice/projects/my-api/AGENTS.md  → found → done
                         or
check:  /Users/alice/projects/my-api/AGENTS.md  → not found
check:  /Users/alice/projects/AGENTS.md         → not found
check:  /Users/alice/AGENTS.md                  → found → done
stop at filesystem root
```

**Why this matters:** If a user drops an `AGENTS.md` in their repo describing the build commands,
coding style, and test strategy, the agent will immediately respect it without the user having to
repeat themselves every session. Walking upward means launching from a subdirectory still finds
the repo-root `AGENTS.md`.

**Pros:** Zero user friction — drop a file and it is automatically respected; works for any
project type.  
**Cons:** Reading file from disk on every call (mitigated by the file being small and the OS page
cache making it effectively free on repeat calls).

> **Note:** Ideas 2 and 3 are designed to be implemented together as a single cohesive unit.
> See `output-3_system-prompt.md` for the full implementation plan covering both ideas.

---

### Idea 4 — Per-Provider System Prompts *(opencode-style routing)*

**Approach:** Different models behave differently. The most important distinction for keen-code
is between reasoning models (which have internal chain-of-thought) and standard models (which
benefit from more verbose workflow guidance in the prompt).

```go
// internal/llm/systemprompt.go
func GetSystemPromptForModel(model string) string {
    switch {
    case strings.Contains(model, "deepseek-reasoner") || strings.Contains(model, "-r1"):
        return reasonerSystemPrompt  // Shorter, action-focused
    default:
        return defaultSystemPrompt   // Full workflow guidance
    }
}
```

**Model-specific prompt differences:**

| Model Family | Key Difference | Prompt Adjustment |
|---|---|---|
| DeepSeek R1 (`deepseek-reasoner`) | Has extended internal thinking; reasons about plans automatically | Shorter prompt, action-focused, no "think before you act" instruction |
| DeepSeek V3 / Moonshot / kimi-k2 | No built-in thinking step | Full workflow guidance, explicit explore-first instruction |
| Future Claude support | Prefers XML tags, has strong instruction following | XML-structured prompt with `<instructions>` tags |
| Future GPT-4o support | Responds well to role-playing framing | Strong identity declaration up front |

For reasoning models like DeepSeek R1, the system prompt should be leaner — the model's
chain-of-thought handles the planning step internally, so repeating "think before you act" in the
prompt is redundant and wastes tokens.

**Pros:** Extracts the best behaviour from each model family; avoids wasting tokens on reasoning
models; easy to extend as new providers are added.  
**Cons:** More maintenance surface; prompts can drift out of sync; requires observing concrete
model-specific failure modes before knowing what to tune.

---

### Idea 5 — Structured Multi-Section Prompt with Clear Headers

**Approach:** Inspired by kimi-cli's clean markdown section structure. Rather than flowing prose
or a flat bullet list, organize the prompt into distinct named sections with `#` headers that the
model can reliably locate and reference during generation.

```markdown
You are Keen Code, an expert coding agent for your terminal.

# Core Behaviour
[Identity, tone, conciseness rules — weights heavily because it is first]

# Coding Workflow
[Explore-first, minimal changes, follow conventions, verify with tests]

# Tool Usage Policy
[Which tools to use when, parallel calls, file_path:line_number references]

# Git Rules
[Never commit/push/reset/rebase without explicit confirmation]

# Security
[No secrets, no malicious code, refuse suspicious requests]
```

**Why section ordering matters:** LLMs weight early content more heavily than late content. The
correct order is: identity → tone → workflow → tools → constraints. Putting safety rules first
(as some prompts do) makes the model over-cautious. Putting them last makes them feel like
afterthoughts. Constraints belong at the end — they apply to an agent that is already behaving
correctly in all other respects.

**Pros:** Highly readable and maintainable; easy to add, remove, or reorder sections; the model
can reference specific sections by name.  
**Cons:** Pure organisational choice — no functional difference from equivalent prose at inference
time.

---

### Idea 6 — Plan Mode via `<system-reminder>` Injection

**Approach:** Following opencode's `plan.txt` pattern, add a `/plan` command to keen-code that
injects a `<system-reminder>` block into the next outgoing message, flipping the agent into
read-only analysis mode without modifying `AppState.messages`.

```go
// When the user types /plan, prepend this to their next user message:
const planModeReminder = `<system-reminder>
PLAN MODE ACTIVE — READ ONLY. Do NOT edit, write, or run mutating bash commands.
Explore the codebase, analyse the problem, and describe a detailed implementation plan.
Ask clarifying questions if needed before proposing any solution. Do not execute anything.
</system-reminder>

`

// Usage in StreamChat:
if s.planMode {
    userContent = planModeReminder + userContent
    s.planMode = false  // one-shot
}
```

**Why `<system-reminder>` and not a new system message?** Most LLM APIs only support one system
message. Injecting the reminder into the user turn as a clearly labelled XML block achieves the
same effect without breaking the message structure. opencode uses exactly this pattern.

**Natural pairings:**
- `/plan` → describe what you would do (read-only)
- `/do` → now actually do it (normal mode)
- ESC to interrupt either mode

**Pros:** High-value UX feature; prevents the agent from making changes when the user just wants
a proposal; pairs naturally with the existing ESC-to-interrupt flow.  
**Cons:** Requires a new command in the REPL input handler; slightly more complex state to track
(even if just a single bool).

---

### Idea 7 — `AGENTS.md` Self-Update Reminder

**Approach:** A single line added to the static prompt (or to the project instructions section)
instructing the agent to keep `AGENTS.md` current when it modifies structures mentioned in it.

```
If you modify any structures, configurations, commands, or workflows that are
described in AGENTS.md, you MUST update AGENTS.md to reflect those changes.
```

kimi-cli includes this exact pattern. The effect is a positive feedback loop: the agent that
modifies the codebase also keeps the AI instructions for future sessions accurate.

**Where to put it:** Append to the project instructions section only when an `AGENTS.md` file
was actually found (no point in the instruction if the file doesn't exist):

```go
if agentsMdPath != "" {
    instructions += fmt.Sprintf(
        "\n\nIf you modify anything described in this file, update %s to reflect the changes.",
        agentsMdPath,
    )
}
```

**Pros:** Free improvement — one sentence, high leverage; encourages a self-documenting project
over time.  
**Cons:** The agent may over-eagerly edit `AGENTS.md` for minor changes; can be mitigated by
saying "significant structural changes" instead of "any changes".

---

### Idea 8 — Global `~/.keen/AGENTS.md` for User-Level Instructions

**Approach:** In addition to the project-level `AGENTS.md` walk, read a global instructions file
from `~/.keen/AGENTS.md` (or `~/.config/keen/AGENTS.md`) and prepend it before the project
instructions. opencode reads from `~/.config/opencode/instructions/` and even supports remote
URL instructions.

```go
func globalInstructions() string {
    home, err := os.UserHomeDir()
    if err != nil {
        return ""
    }
    candidates := []string{
        filepath.Join(home, ".keen", "AGENTS.md"),
        filepath.Join(home, ".config", "keen", "AGENTS.md"),
    }
    for _, path := range candidates {
        content, err := os.ReadFile(path)
        if err == nil && len(content) > 0 {
            return fmt.Sprintf("# Global User Instructions (from %s)\n%s", path, string(content))
        }
    }
    return ""
}
```

**Assembly order with global instructions:**

```
[system message]
  1. staticPrompt
  2. envBlock()
  3. globalInstructions()          ← user-level preferences
  4. projectInstructions()         ← project-level conventions
[user / assistant history]
```

**Pros:** Lets users set personal preferences once (e.g. "I prefer functional style", "always use
pnpm not npm") that apply across all projects.  
**Cons:** Introduces a home-directory dependency; needs documentation so users know the file
exists; global instructions could conflict with project instructions.

---

### Idea 9 — `compact.md`: Context Window Compression Prompt

**Approach:** Following kimi-cli's `compact.md`, add a dedicated prompt and a `/compact` REPL
command that summarises the conversation history into a structured block, then replaces the full
message history with that summary to reclaim context window space.

```go
const compactPrompt = `Summarise the conversation so far into a structured block:

<summary>
  <task>One sentence: what the user is trying to achieve.</task>
  <done>Bullet list: what has been completed successfully.</done>
  <state>Current state of the codebase: what was changed, where.</state>
  <next>What remains to be done.</next>
  <decisions>Any important decisions or constraints the user expressed.</decisions>
</summary>

Output ONLY the <summary> block. No other text.`
```

When the user types `/compact`:
1. Append the compact prompt as a user message
2. Get the model's `<summary>` response
3. Replace `AppState.messages` with a single synthetic user message containing the summary
4. Continue the session — the model now has a compressed history

**Pros:** Directly solves context window exhaustion on long coding sessions without losing the
essential thread of what has been done; XML structure makes the summary machine-parseable for
future features.  
**Cons:** Some context fidelity is lost; requires a new REPL command; the summary quality depends
on the model.

---

## Summary Table

| # | Idea | Complexity | Value | Status |
|---|---|---|---|---|
| 1 | Single embedded static prompt | Low | High | Superseded by Ideas 2+3 |
| 2 | Static prompt + dynamic env block | Low | Very High | **Planned — see output-3** |
| 3 | `AGENTS.md` auto-loading | Low | Very High | **Planned — see output-3** |
| 4 | Per-provider system prompts | Medium | Medium | Deferred |
| 5 | Structured multi-section prompt | Low | Medium | Incorporated into Ideas 2+3 |
| 6 | Plan mode via `<system-reminder>` | Medium | High | Future feature |
| 7 | `AGENTS.md` self-update reminder | Very Low | Medium | Include in Ideas 2+3 impl |
| 8 | Global `~/.keen/AGENTS.md` | Low | Medium | Deferred |
| 9 | `compact.md` context compression | Medium | High | Future feature |

---

## Recommended Implementation Order

1. **Ideas 2 + 3** (static + env + `AGENTS.md`) — the highest-leverage change; full plan in
   `output-3_system-prompt.md`.
2. **Idea 7** (`AGENTS.md` self-update reminder) — a one-line addition to Ideas 2+3
   implementation; near-zero cost.
3. **Idea 8** (global `~/.keen/AGENTS.md`) — low complexity, rounds out the instruction loading
   story.
4. **Idea 6** (plan mode) — compelling UX feature once the base is solid.
5. **Idea 9** (`/compact`) — important for long sessions; implement when context exhaustion
   becomes a real user complaint.
6. **Idea 4** (per-model routing) — implement only once a concrete behavioural difference is
   observed and measured; premature optimisation otherwise.

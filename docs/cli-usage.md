# CLI Usage

Keen Code provides slash commands (prefixed with `/`) for controlling the agent. Type `/` and press `Tab` to see command suggestions.

## Command Reference

| Command | Description |
|---------|-------------|
| `/adversary [prompt]` | Run a second model as an adversarial critic of the main agent's work |
| `/adversary model` | Configure which provider and model acts as the adversary |
| `/btw <question>` | Ask a quick side question without adding it to the main conversation |
| `/help` | Show available commands |
| `/model` | Change provider or model |
| `/allow-permission <tool_names...>` | Always allow these tools without prompting |
| `/reset-permission <tool_names...>` | Reset tool permissions to Keen's default mechanism |
| `/thinking <effort>` | Set thinking effort for the current model |
| `/show-thinking [on\|off]` | Show, hide, or inspect thinking token display |
| `/sessions` or `/resume` | Open the saved-session picker for the current directory |
| `/skills [list\|reload]` | List or reload skills |
| `/skills <name> [enable\|disable]` | Enable or disable a skill |
| `/<skill-name> [args...]` | Activate an enabled skill |
| `/compact [prompt]` | Compact conversation context; provide a prompt to guide what to retain |
| `/context` | Show estimated context-window usage by prompt, messages, tool definitions, and tool results |
| `/cleanup` | Remove expired Keen data and trim input history |
| `/memory` or `/memory show` | Show memory file locations; `show` includes their contents |
| `/mcp [status\|connect <server>]` | Show MCP server status or connect a configured server |
| `/mode [plan\|build]` | Show or switch the agent mode |
| `/subagents [list]` | List available subagent profiles |
| `/tool-history [full\|none]` | Show or control whether future tool outputs are retained between turns |
| `/clear` or `/new` | Clear the current session and start a new one |
| `/emptyq` | Clear the input queue (works while agent is streaming) |
| `/logout` | Sign out of the current OAuth provider |
| `/exit` | Quit Keen Code |

## `/adversary [prompt]`

Runs a separately-configured model as an adversarial critic of the main agent's work. The adversary reviews the full conversation and looks for problems — bugs, logic errors, security issues, faulty assumptions, risks in plans, or ideas the main agent didn't consider.

```text
/adversary
/adversary Focus on security implications
```

Behavior:

- Passes the full conversation history to the adversary model, with the main agent's turns clearly labelled so the adversary does not confuse them with its own output
- Starts fresh each invocation — previous adversary responses are not included in the context
- Has read-only tool access (`read_file`, `glob`, `grep`) — no write, edit, bash, or MCP tools
- Cannot be invoked while the main agent is streaming; the command is queued instead (see [Input Queuing](queuing.md))
- Press `Esc` to cancel an in-progress adversary response
- Rendered as a teal left-border block in the output, separate from the main conversation
- Adversary output is not added to main session history

Run `/adversary model` first to configure which model acts as the adversary.

## `/adversary model`

Opens the interactive model picker to select the provider and model used for adversary reviews. The adversary model is stored separately from the main agent model in Keen's global config.

```text
/adversary model
```

Navigation is the same as `/model`. Once configured, the adversary model persists across sessions.

## `/btw <question>`

Asks a quick side question inline without adding the question or answer to the main conversation. Use it for short clarifications that should not steer the active task.

```text
/btw What does this error mean?
```

Behavior:

- Uses up to the last 10 messages (5 user + 5 agent) as read-only context
- Runs without tool access
- Works even while the main assistant response is streaming
- Press `Esc` to cancel the stream while the response is loading
- Running `/btw` with no question shows usage help

## `/help`

Displays a formatted list of built-in slash commands with descriptions.

```text
/help
```

## `/model`

Opens an interactive model selector to change the AI provider and model. Use this command when you want to:

- Switch between providers (Anthropic, OpenAI, Google AI, Amazon Bedrock, etc.)
- Change to a different model within the same provider
- Configure API keys for a provider
- Configure an optional custom base URL for API-key providers that support it
- Sign in to OAuth providers such as Codex (ChatGPT OAuth)

```text
/model
```

Navigation:

| Key | Action |
|-----|--------|
| `↑` / `↓` or `k` / `j` | Move through provider, model, or thinking-effort lists |
| `Enter` | Select the highlighted item or confirm input |
| `Esc` | Cancel model selection |

API keys are masked while typed. If an API key already exists for the provider, press `Enter` on an empty API-key prompt to keep it.

Custom HTTP headers for a provider can be configured by editing `~/.keen/configs.json` directly. See [`docs/ai-providers.md`](ai-providers.md#custom-headers).

## `/allow-permission <tool_names...>`

Always allow the listed tools without prompting. The setting is saved to `.keen/permissions.json` in the working directory and takes effect immediately.

```text
/allow-permission bash                  # Always run bash without prompting, including dangerous commands
/allow-permission write_file edit_file  # Allow multiple tools at once
```

Running `/allow-permission` with no arguments prints usage and lists the available tool names.

Notes:

- `allow-permission` bypasses the interactive prompt but the filesystem guard still applies — the agent cannot reach system directories, `.gitignore`d files, or dotfiles under `$HOME` regardless of this setting.
- For `bash`, non-dangerous commands are already auto-granted; the only effect of allowing it is suppressing the prompt for dangerous commands.
- Settings are project-scoped and do not affect other working directories.

## `/reset-permission <tool_names...>`

Removes the project-level allow setting for the listed tools, falling back to Keen's default permission mechanism.

```text
/reset-permission bash
/reset-permission write_file edit_file
```

Running `/reset-permission` with no arguments prints usage and lists the available tool names.

## `/thinking <effort>`

Sets the thinking effort level for the current model. The accepted values come from the selected model, not just the provider. If the current model does not support configurable thinking, Keen shows an error.

Common supported efforts include:

| Provider/model family | Efforts |
|-----------------------|---------|
| Anthropic Claude Opus/Fable/Sonnet | `low`, `medium`, `high`, `xhigh`, `max` |
| OpenAI GPT models | `none`, `low`, `medium`, `high`, `xhigh` (some models also support `max`) |
| Codex GPT models | `low`, `medium`, `high`, `xhigh` (some models also support `max` or `ultra`) |
| Google AI Gemini Pro | `low`, `medium`, `high` |
| Google AI Gemini Flash / Flash-Lite | `minimal`, `low`, `medium`, `high` |
| DeepSeek V4 | `disabled`, `high`, `max` |
| Amazon Bedrock Claude | varies by model; includes `low`, `medium`, `high`, `xhigh`, and `max` |
| OpenCode Go models | varies by model; examples include `enabled`/`disabled`, `none` through `max`, and `low`/`high`/`max` |

```text
/thinking high
```

If no effort is specified, or the effort is invalid for the current model, Keen prints the valid options.

## `/show-thinking [on|off]`

Controls whether thinking/reasoning tokens are displayed in the output.

```text
/show-thinking on   # Show thinking tokens
/show-thinking off  # Hide thinking tokens
/show-thinking      # Show the current setting
```

The setting is saved in Keen's global config.

## `/sessions` / `/resume`

Opens the saved-session picker for the current working directory. Sessions are stored in `~/.keen/sessions/` and namespaced by directory.

```text
/sessions
/resume
```

Navigation:

| Key | Action |
|-----|--------|
| `↑` / `↓` or `k` / `j` | Move through sessions |
| `Enter` | Resume the highlighted session |
| `Esc` | Close the picker |

If there are no saved sessions for the directory, Keen prints a message instead of opening the picker.

## `/skills [list|reload|<name> enable|disable]`

Manages skill plugins. Without arguments, lists all available skills.

```text
/skills
/skills list              # List all skills
/skills reload            # Reload skills from disk
/skills <name> enable     # Enable a skill
/skills <name> disable    # Disable a skill
```

Skills are discovered from:

- `<working-dir>/.agents/skills/`
- `<working-dir>/.keen/skills/`
- `<working-dir>/.claude/skills/`
- `~/.agents/skills/`
- `~/.keen/skills/`
- `~/.claude/skills/`
- Bundled skills embedded in the binary

Skills are enabled by default unless disabled in the skills config.

## `/<skill-name> [args...]`

Enabled skills can be activated like slash commands:

```text
/review
/review docs/cli-usage.md
```

When a skill is activated with a slash command, Keen reads that skill's `SKILL.md`, substitutes arguments into the skill body using `$ARGUMENTS`, `$1`, `$2`, etc., and injects the processed body into the conversation as an activation message. See [`docs/skills-system.md`](skills-system.md) for the skill file format.

This is different from model-driven skill use. Enabled skills are also listed in the system prompt with their descriptions and `SKILL.md` paths, so the LLM may decide a skill is relevant and read that file itself with the `read_file` tool. In that case, the slash command was not used, no slash-command argument substitution occurs, and the skill instructions arrive through the tool result instead of a pre-injected activation message.

The persistence is also different across turns. A slash-activated skill is injected as a conversation message, so its processed instructions remain in retained conversation history. A model-read `SKILL.md` is just a tool result for the active assistant turn; after a normal turn finish, Keen replaces raw tool traffic with compact turn memory, so the next turn may need to read the skill file again if those instructions are still needed.

Skill names participate in slash-command autocomplete alongside built-in commands.

## `/compact [prompt]`

Triggers context compaction to reduce conversation history while preserving important information. Optionally includes a prompt to guide what to retain.

```text
/compact
/compact Focus on the recent API changes
```

Compaction requires an initialized LLM client. While compaction is running, press `Esc` to cancel it.

Compaction is useful when context is filling up and you want to summarize the conversation before continuing. Keen may also suggest `Try /compact` in the status line when context usage is high.

## `/clear` / `/new`

Clears the current session and starts fresh. This clears:

- Conversation history
- Turn memory
- Session state
- Session-scoped permission approvals
- Current input history state

```text
/clear
/new
```

## `/emptyq`

Clears all queued inputs. Works at any time, including while the agent is actively streaming. Shows a "Queue cleared" notification, or "Queue is empty" if there is nothing to clear.

```text
/emptyq
```

See [Input Queuing](queuing.md) for full details on how queued messages are displayed and drained.

## `/logout`

Signs out of the current OAuth provider. This is useful for providers that use browser-based OAuth, currently Codex (ChatGPT OAuth). It removes stored OAuth credentials and clears the active LLM client.

```text
/logout
```

If the current provider does not use OAuth, Keen shows an error.

## `/exit`

Quits Keen Code.

```text
/exit
```

## Input Queuing

When the agent is actively streaming a response, Keen Code queues certain inputs instead of dropping them. Queued messages appear in a preview band below the spinner and are automatically submitted one at a time as each turn completes.

**Queueable inputs:**

- Normal text prompts
- Enabled skill activations (`/<skill-name> [args]`)
- `/adversary [prompt]`

**Not queued** (rejected with a notification while busy):

- Slash commands like `/clear`, `/model`, `/compact` — these are immediate state operations
- Unknown `/`-prefixed inputs — shows "No such skill found"
- `!shell` commands — immediate shell execution

The queue holds up to **5 items**. Use `/emptyq` to clear the queue at any time, or press `Ctrl+C`/`Esc` while idle to clear it.

See [`docs/queuing.md`](queuing.md) for the full behavior specification.

## File Mentions

Use `@` mentions to reference files in prompts. Type `@` followed by part of a filename or path, then use autocomplete to insert a match.

```text
Review @docs/cli-usage.md
```

File mention autocomplete:

- Starts when `@<token>` appears at the beginning of input or after a space
- Searches for relative paths whose names contain the token
- Respects Keen's filesystem guard and ignored/blocked paths
- Inserts the selected path as `@path ` with a trailing space

## Autocomplete

Type a partial slash command and press `Tab` to see suggestions:

```text
/mo<Tab> → /model
/th<Tab> → /thinking
```

Autocomplete supports:

- Built-in slash commands, including `/btw`
- Enabled skills as slash commands
- File mentions with `@<token>`

When suggestions are visible:

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move through suggestions |
| `Tab` | Accept the highlighted suggestion |
| `Enter` | Accept the highlighted suggestion |
| `Esc` | Hide suggestions when no response is streaming |

Slash-command autocomplete matches command prefixes case-insensitively. Command execution itself uses the command names shown above.

## Keyboard Shortcuts

These shortcuts are available in the REPL:

| Shortcut | Action |
|----------|--------|
| `Enter` | Submit the current input |
| `Shift+Enter` | Insert a newline in the input box |
| `Ctrl+C` | Clear non-empty input; clear queued messages when idle; quit when input is empty and queue is empty; copy selected text when a selection is active |
| `Ctrl+D` | Clear non-empty input; quit when input is empty |
| `Esc` | Interrupt an active response; clear queued messages when idle; cancel an active `/btw` stream; cancel compaction/model/session pickers; hide suggestions; clear selections |
| `Tab` | Show or accept autocomplete suggestions |
| `↑` / `↓` | Navigate input history when the cursor is at the top/bottom of input; otherwise move the cursor; scroll output when history cannot move |
| `PageUp` / `PageDown` | Scroll output by a half page |
| `Home` / `End` | Jump to top/bottom of output |

## Mouse Selection

Keen supports mouse selection in the output and input areas:

| Action | Result |
|--------|--------|
| Click and drag | Select text |
| Double-click | Select word |
| Triple-click | Select line |
| `Alt`/`Ctrl` + click on a link | Open the URL in your browser |
| `Ctrl+C` / `Cmd+C` with a selection | Copy selected text |
| `Esc` | Clear selection |

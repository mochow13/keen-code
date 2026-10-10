# MCP Servers Driven by Skills

Keen integrates MCP servers through the existing Skills system. Connected MCP servers generate ordinary skill directories under `~/.keen/skills`, and the LLM uses those skill files to discover MCP tools before calling the shared `call_mcp_tool` tool.

## Overview

The flow is:

1. User configures MCP servers in `~/.keen/mcp/configs.json`, or a file selected with `--mcp-config <file_path>` (see [configuration location](mcp-servers.md#configuration-location)).
2. Keen starts the MCP manager and connects to configured servers.
3. For each connected server, Keen lists MCP tools.
4. Keen generates a skill named `mcp:<server>`.
5. The generated skill appears in the skills catalog sent in the system prompt.
6. When the LLM needs that server, it reads the generated `SKILL.md` and relevant schema file.
7. The LLM calls the built-in `call_mcp_tool` tool with the configured server name, tool name, and schema-shaped arguments.

This makes MCP discovery prompt-efficient: the system prompt only lists a concise skill entry, while detailed tool lists and schemas are loaded on demand.

## Generated skill layout

For a configured server named `github`, Keen generates:

```text
~/.keen/skills/mcp:github/
├── SKILL.md
├── .keen-generated-mcp.json
└── schemas/
    ├── create_issue.json
    └── list_issues.json
```

The skill name is always:

```text
mcp:<server>
```

For example:

| MCP server | Generated skill |
| --- | --- |
| `github` | `mcp:github` |
| `context7` | `mcp:context7` |
| `posthog` | `mcp:posthog` |

## How MCP skills are generated

Keen generates MCP skills from connected server tool metadata:

- `SKILL.md` contains YAML frontmatter and a markdown table of available tools.
- One schema file is written for each tool's MCP input schema.
- Tool schemas are written to `schemas/<tool-name>.json`.
- If a tool has no input schema, Keen writes an empty JSON object.
- Generation is atomic: Keen writes to a temporary directory, then replaces the target skill directory.
- Tool names that attempt path traversal are rejected.
- The generated tool table is capped at 1000 rows.
- Long descriptions are truncated.

Example generated `SKILL.md`:

```markdown
---
name: mcp:github
description: Use this skill to interact with the `github` MCP server.
---
## Available tools
| Tool | Description |
|------|-------------|
| create_issue | Create a GitHub issue |
| list_issues | List issues |
```

The generated metadata file looks like:

```json
{
  "managed_by": "keen-mcp",
  "server": "github",
  "status": "connected",
  "tool_count": 2,
  "last_successful_refresh": "2026-05-21T12:00:00Z",
  "last_error": ""
}
```

## Schema generation

Each MCP tool's `InputSchema` becomes a JSON file in the generated skill directory.

For a tool named `create_issue`, the schema path is:

```text
~/.keen/skills/mcp:github/schemas/create_issue.json
```

For nested tool names, directories may be created under `schemas/` as long as the name does not escape the schemas directory.

The LLM is instructed by `call_mcp_tool` to read:

1. `~/.keen/skills/mcp:<server>/SKILL.md`
2. `~/.keen/skills/mcp:<server>/schemas/<tool>.json`

Then it must pass arguments matching the schema exactly.

## How the LLM uses MCP skills

Keen includes enabled skills in the system prompt as a catalog entry like:

```markdown
- mcp:github: Use this skill to interact with the `github` MCP server. → read ~/.keen/skills/mcp:github/SKILL.md
```

When the user asks for something that needs the MCP server, the LLM follows the same skill activation process as regular skills:

1. Read the generated `SKILL.md` to discover available tools.
2. Pick the relevant MCP tool.
3. Read that tool's schema file.
4. Call `call_mcp_tool`.

> ⚠️ There is a persistence difference between explicit slash activation and LLM-driven skill reading:
>
> - **Slash activation** (`/mcp:<server> ...`) is handled before the LLM turn. Keen reads `SKILL.md`, applies slash-command argument substitution, and injects the processed skill body as an `[Activate skill: ...]` conversation message. Because it is a message, it remains in retained conversation history after the turn finishes.
> - **LLM-driven activation** happens when the model reads `~/.keen/skills/mcp:<server>/SKILL.md` itself after seeing the skill catalog entry. No slash-command argument substitution is applied. The skill body arrives as a tool result for the active turn, and raw tool traffic is compacted when the turn finishes, so a later turn may need to read the skill file again.
> - **For context management**, prefer letting the LLM read `SKILL.md` on demand instead of explicitly injecting MCP skills with a slash command. This keeps detailed skill instructions out of retained conversation history unless they are needed again.

Example `call_mcp_tool` input:

```json
{
  "server": "github",
  "tool": "create_issue",
  "arguments": {
    "title": "Fix flaky test",
    "body": "The race test fails intermittently."
  }
}
```

The `call_mcp_tool` tool is generic. It does not expose one separate LLM tool per MCP server tool. The generated skill and schema files provide the discovery and argument contract.

## Benefits of the skill-driven approach

| Benefit | Why it matters |
| --- | --- |
| Smaller system prompt | The prompt includes only a skill catalog entry, not every MCP tool schema. |
| On-demand context | The LLM reads only the MCP server and schema it needs. |
| Works with existing skill UX | MCP servers appear in `/skills list`, can be enabled/disabled, and are activated like other skills. |
| Stable tool surface | Keen only needs one built-in LLM tool: `call_mcp_tool`. |
| Dynamic server discovery | Generated skill files reflect the server's live tool list after successful startup or refresh. |
| Clear audit point | User approval happens for each `call_mcp_tool` execution. |
| Failure isolation in prompt | Failed MCP servers are disabled as skills, keeping unavailable tools out of the skill catalog. |

## Startup synchronization

After MCP startup completes, the REPL syncs MCP statuses with skills:

| MCP state | Skill behavior |
| --- | --- |
| `connected` | Generate/refresh `mcp:<server>` skill, mark it enabled, then reload skills. |
| `disconnected` | Mark `mcp:<server>` disabled, then reload skills. |
| `auth_required` | Mark `mcp:<server>` disabled, show a connect hint. |
| `auth_failed` | Mark `mcp:<server>` disabled, show a connect hint. |
| Removed from MCP config | If a previously enabled `mcp:<server>` skill is not in the configured server set, mark it disabled. |

Generated files for failed or removed servers are not necessarily deleted. Instead, Keen disables the skill so it is hidden from the enabled skills catalog.

## Manual refresh and authentication

Use:

```text
/mcp connect <server>
```

This calls MCP `Refresh` for the server.

For OAuth servers, `/mcp connect` also:

- uses the browser OAuth code fetcher,
- uses `http://localhost:1456/auth/mcp/callback` as the redirect URL,
- forces re-authentication by clearing the stored token first,
- waits up to 5 minutes for connection/authentication.

On success:

1. Keen refreshes the server's MCP tools.
2. Keen regenerates the `mcp:<server>` skill.
3. Keen enables the skill.
4. Keen reloads skills so the LLM sees the skill catalog entry.

On failure:

1. Keen reports the connection error.
2. Keen disables `mcp:<server>`.
3. Keen reloads skills so the unavailable server is removed from the enabled skills catalog.

## Enabling and disabling MCP server skills

MCP skills use the normal skills config file:

```text
~/.keen/skills/config.json
```

Example:

```json
{
  "is_enabled": {
    "mcp:github": true,
    "mcp:posthog": false
  }
}
```

You can manage them with normal skill commands:

```text
/skills list
/skills disable mcp:github
/skills enable mcp:github
/skills reload
```

Important behavior:

- Skills default to enabled if they are not present in `config.json`.
- A connected MCP server refreshes its generated skill files during startup MCP sync, but it does not override an explicitly disabled skill.
- Running `/mcp connect <server>` successfully refreshes the generated skill files and enables the skill.
- A failed MCP server is automatically disabled during MCP sync.
- A stale MCP skill whose server was removed from MCP config is removed from skills config and its generated skill directory is deleted during MCP sync.
- Disabling a generated MCP skill hides it from the skills catalog, but it does not remove the MCP server from the selected MCP configuration file and does not stop an already-running MCP session.

To fully remove a server from Keen's MCP runtime, remove it from the selected MCP configuration file (`~/.keen/mcp/configs.json` by default, or the file passed to `--mcp-config`) and restart Keen.

## What happens when MCP fails

| Scenario | Skill outcome | LLM outcome |
| --- | --- | --- |
| Server connects and tools list succeeds on startup | Skill files are generated or refreshed; existing enabled/disabled preference is preserved. | LLM can discover the skill only when it is enabled, but connected MCP tools remain callable through `call_mcp_tool`. |
| Server fails to connect | Skill is disabled. | LLM does not see the skill in the enabled catalog. |
| OAuth is required | Skill is disabled and Keen suggests `/mcp connect <server>`. | LLM does not see the skill until auth succeeds. |
| OAuth/API key auth fails | Skill is disabled. | LLM does not see the skill until the server reconnects. |
| Tool listing fails | Skill generation is skipped; server state becomes failed. | Existing generated files may remain, but sync disables the skill for failure states. |
| `/mcp connect <server>` succeeds | Skill files are generated or refreshed, and the skill is enabled. | LLM can discover the skill in future turns. |
| Server is removed from config | Any explicit skill status is removed from skills config and generated skill files are deleted on next startup sync. | LLM no longer sees the generated skill. |
| MCP manager cannot start due invalid config | No MCP runtime is registered and `call_mcp_tool` is unavailable. | LLM cannot call MCP tools. Previously generated skills may still exist on disk, but they are only useful if enabled and the tool exists; fix config and restart. |

## Security and permissions

MCP tools can perform arbitrary actions depending on the remote server. Keen adds these guardrails:

- Every `call_mcp_tool` request asks the user for approval.
- The approval prompt includes the server, tool, and JSON arguments.
- API keys are redacted from MCP errors/logs when possible.
- Stdio credentials should be provided via environment variables in MCP config or the parent environment.
- Generated schemas are local files under `~/.keen/skills/mcp:<server>/schemas`.

## Implementation references

| Concern | Code |
| --- | --- |
| MCP config format and validation | `internal/mcp/config.go` |
| MCP runtime, connection, tool listing, tool calls | `internal/mcp/manager.go` |
| OAuth handling and token persistence | `internal/mcp/oauth.go` |
| Generated MCP skills and schemas | `internal/mcpskills/mcpskills.go` |
| Startup MCP/skill synchronization | `internal/cli/repl/repl_helpers.go` |
| `/mcp` commands | `internal/cli/repl/command_handlers.go` |
| Generic LLM MCP tool | `internal/tools/call_mcp_tool.go` |
| Skills config and catalog | `internal/skills/config.go`, `internal/skills/discover.go` |

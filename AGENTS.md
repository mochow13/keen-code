# Keen Code

Terminal-based AI coding agent written in Go. It supports multiple LLM providers, built-in and MCP tools, skills, subagents, persistent sessions, and a Bubble Tea REPL.

## Working on this repository

- Keep changes focused; avoid speculative refactors and unnecessary comments.
- Preserve existing public tool contracts and permission checks. File operations must go through `internal/filesystem` guards.
- Do not expose, log, or commit API keys, OAuth credentials, session transcripts, or other secrets.
- Project instructions are loaded from `AGENTS.md`; skills, agents, and memory are separate user/project configuration.

## Architecture

- `cmd/` — CLI entry point.
- `internal/cli/` — Cobra commands and Bubble Tea REPL.
- `internal/llm/` — provider clients, tool execution, context management, and prompt formatting.
- `internal/tools/` — built-in LLM tools and input validation.
- `internal/filesystem/` — working-directory, sensitive-path, and `.gitignore` access guard.
- `internal/config/` and `internal/providers/` — provider configuration and model metadata.
- `internal/session/` — JSONL-backed session persistence.
- `internal/mcp/`, `internal/mcpskills/`, `internal/skills/`, `internal/subagents/`, `internal/memory/` — extensibility and agent features.
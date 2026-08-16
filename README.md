<div align="center">

<img src="./assets/keen-code-2.png" alt="Keen Code" width="500"/>

[![Latest Release](https://img.shields.io/github/v/release/mochow13/keen-code?style=flat-square&logo=github)](https://github.com/mochow13/keen-code/releases/latest)
[![Build Status](https://img.shields.io/github/actions/workflow/status/mochow13/keen-code/go.yml?branch=main&style=flat-square&logo=githubactions&logoColor=white)](https://github.com/mochow13/keen-code/actions)
[![Coverage](https://codecov.io/gh/mochow13/keen-code/graph/badge.svg)](https://codecov.io/gh/mochow13/keen-code)
[![Go Reference](https://pkg.go.dev/badge/github.com/mochow13/keen-code.svg)](https://pkg.go.dev/github.com/mochow13/keen-code)
[![Go Version](https://img.shields.io/github/go-mod/go-version/mochow13/keen-code?style=flat-square&logo=go)](https://go.dev/)
[![License](https://img.shields.io/github/license/mochow13/keen-code?style=flat-square&logo=opensourceinitiative&logoColor=white)](https://github.com/mochow13/keen-code/blob/main/LICENSE)

<a href="https://www.producthunt.com/products/keen-code-a-cli-coding-agent?embed=true&utm_source=badge-featured&utm_medium=badge&utm_campaign=badge-keen-code" target="_blank" rel="noopener noreferrer"><img alt="Keen Code - A context-efficient CLI coding agent built by agents | Product Hunt" width="250" height="54" src="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1162542&theme=dark&t=1780593417757"></a>

</div>

**Keen Code** is a terminal-based AI coding agent like Claude Code or Codex CLI. Written in Go, it is simpler, lighter, minimalistic but useful coding agent for typical software engineering tasks. It supports multiple providers, skills, MCPs, subagents with multi-agent orchestration, and more.

Keen Code is highly opinionated. It avoids features that are not necessarily needed or useful for a regular software engineer. It tries to avoid unnecessary complexity and attempts to keep the agent harness as simple as possible.

From requirements to implementation, Keen Code was engineered using a wide range of coding agents and agentic IDEs. By far, AI coding agents are the most ubiquitous use case in the era of AI agents. One of the goals of the project is to showcase how coding agents can be used to develop coding agents themselves. This is why most prompts and output docs are saved as markdown files in the [`.ai-interactions`](.ai-interactions/) directory. 

Keen Code is also an experiment to play with the *new way of working* where engineers work with AI agents to develop software. In this setting, engineers are sometimes referred to as "orchestrators".

<h3 align="center">
  Born as an experiment, Keen is now a fully functional coding agent designed for real-world software development.
</h3>

<div align="center">
  <img src="./assets/demo.gif" alt="Keen Code demo" width="1200"/>
</div>

## Table of Contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Development Philosophy](#development-philosophy)
- [Development Cycle Example](#development-cycle-example)
- [Install Keen Code](#install-keen-code)
  - [Install with script](#install-with-script)
  - [Install with npm](#install-with-npm)
- [Run Keen](#run-keen)
- [Supported Providers](#supported-providers)
- [Built-in Tools](#built-in-tools)
- [Telemetry](#telemetry)
- [How Keen Handles Context](#how-keen-handles-context)
- [Further Reading](#further-reading)


## Features

- **Multi-provider** — Anthropic, OpenAI, Codex (via OAuth), Gemini, DeepSeek, Kimi, GLM, MiniMax, OpenCode Go, and Amazon Bedrock. Switch with `/model`. More providers will be added in the future.
- **6 minimal tools** — `read_file`, `write_file`, `edit_file`, `glob`, `grep`, `bash`. Deliberately lean.
- **Skills system** — Specialized workflows for planning, debugging, refactoring, code review, and more.
- **Thinking mode** — Extended reasoning for complex tasks. Use `/thinking` to change the thinking effort level for the current model. All models that support thinking can be configured.
- **Session management** — Persistent sessions with resume capability.
- **Configurable tool history** — Lean cross-turn `TurnMemory` summaries by default; use `/tool-history full` to retain full tool outputs for future turns. More information can be found in [docs/turn-memory.md](docs/turn-memory.md).
- **User-triggered compaction** - When the context window is nearing the limit, use `/compact` to compact the context.

## Screenshots

<table>
  <tr>
    <td width="50%" align="center">
      <a href="./assets/full-interface.png">
        <img src="./assets/full-interface.png" alt="Keen Code full interface" width="100%">
      </a>
      <br><sub>Full interface</sub>
    </td>
    <td width="50%" align="center">
      <a href="./assets/output.png">
        <img src="./assets/output.png" alt="Keen Code command output" width="100%">
      </a>
      <br><sub>Command output</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" align="center">
      <a href="./assets/diff.png">
        <img src="./assets/diff.png" alt="Reviewing a code diff in Keen Code" width="100%">
      </a>
      <br><sub>Diff review</sub>
    </td>
    <td width="50%" align="center">
      <a href="./assets/commands.png">
        <img src="./assets/commands.png" alt="Keen Code interactive commands" width="100%">
      </a>
      <br><sub>Interactive commands</sub>
    </td>
  </tr>
  <tr>
    <td colspan="2" align="center">
      <a href="./assets/permission.png">
        <img src="./assets/permission.png" alt="Keen Code permission prompt" width="50%">
      </a>
      <br><sub>Permission prompt</sub>
    </td>
  </tr>
</table>

## Telemetry

Keen Code collects two minimal anonymous usage events for actual interactive and headless coding sessions: `keen_session_start` and `keen_session_end`. Help, version, and invalid command invocations are not counted. The events contain a random resettable installation ID, a session ID, Keen version, OS, architecture, interactive/headless mode, and session duration in milliseconds. Google Analytics derives coarse country from the request connection, so no separate country event or country value is sent.

Keen Code never sends prompts, responses, code, paths, commands, repository details, model/provider details, or raw errors. Set `KEEN_TELEMETRY=off` or `DO_NOT_TRACK=1` to disable telemetry. It is also disabled automatically in CI.

Telemetry delivery is fail-silent. The session-start event is emitted asynchronously, and the session-end event is emitted on normal exit. Each event has a one-second timeout. Forced termination, crashes, and network failures can still prevent delivery.


## How Keen Handles Context

Keen takes a deliberately lean approach to cross-turn context. Within a single assistant turn the model has full access to its tool calls and results. By default, later model requests receive a bounded `TurnMemory` summary attached to assistant messages: where retained tools ran, their bounded invocation inputs, status, and non-zero bash exit codes—not their raw outputs.

For ideation or work that benefits from revisiting exact earlier results, run `/tool-history full`. Tool outputs from future turns are then retained in the current session's cross-turn model context. Run `/tool-history none` to return to the compact default, or `/tool-history` to inspect the setting. Full history increases prompt size and token cost; it is not persisted when a session is saved.

Subsequent turns therefore receive:

- prior user and assistant messages
- provider-native historical tool-call/result blocks reconstructed from assistant prose and `TurnMemory`
- any pending provider-native state from a turn that failed mid-loop, so the model can resume instead of starting over

The default tradeoff is intentional: smaller context and a better signal-to-noise ratio, at the cost of occasionally re-reading files or re-running searches when older observations are needed again. Read-only facts and external observations are refreshed when needed rather than treated as durable evidence. `/tool-history full` lets you choose continuity over that default for the remainder of the session.

For the full rationale, lifecycle, and comparison with other coding agents, see [`docs/turn-memory.md`](docs/turn-memory.md).

## Development Philosophy

Developing Keen Code is guided by the following philosophy:

- All the code is written by AI agents, not humans
- The project is developed iteratively using spec-task-code-review cycle by a human engineer
- The human engineer has a very strict set of roles:
  - Specifiy and clarify the requirements
  - Review design docs and influence design decisions
  - Review changes made by the agents
    - Changes can also be reviewed by the agents themselves
  - Ensure the quality and correctness of the code
  - Focus on best practices and standards relevant to the programing language (Go in this case)
  - Thoroughly review and test the product after each iteration
  - Continously provide feedback to the agents to improve the product
- Prompts are saved as markdown files in the `.ai-interactions/prompts` directory
  - Almost all of the prompts are stored to showcase how the project evolved from the initial requirements to the current state
  - Prompts are pretty much chronologically ordered which demonstrates the thought process and iterative nature of the development
- All the outputs are saved as markdown files in the `.ai-interactions/outputs` directory
  - These outputs are basically plans, design docs, and breakdowns of the tasks
  - These outputs are the "specs" that the agents later use to implement the tasks

## Development Cycle Example

All features follow a **spec → plan → task → review** cycle. Here's a concrete example — the `read_file` tool from Phase 3:

**Spec** — [`prompts/phase-3/prompt-3_read-file-tool.md`](.ai-interactions/prompts/phase-3/prompt-3_read-file-tool.md)
Requirements defined upfront: ask permission before reading, respect FileGuard path rules, text files only, 1 MB limit, support relative and absolute paths.

**Plan** — [`outputs/phase-3/output-3_read-file-tool.md`](.ai-interactions/outputs/phase-3/output-3_read-file-tool.md)
Design doc produced by the agent: how `Guard.CheckPath` maps to the REPL permission prompt, exact struct contracts, permission flow diagram.

**Task** — [`prompts/phase-3/prompt-2_phase-3-tasks.md`](.ai-interactions/prompts/phase-3/prompt-2_phase-3-tasks.md)
Implementation broken into steps — tool contract, permission bridge, REPL selector, unit tests — each approved before the next began.

**Review** — (inline feedback during implementation)
The LLM was rejecting `.go` files because MIME detection flagged them as binary. Review caught this; switched to character-based text validation. The fix landed in the same iteration.

## Install Keen Code
### Install with script

```bash
curl -fsSL https://raw.githubusercontent.com/mochow13/keen-code/main/scripts/install.sh | bash
```

The same command above updates the CLI to the latest version.

To pin a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/mochow13/keen-code/main/scripts/install.sh | bash -s -- -v v0.16.1
```

Installs to `/usr/local/bin` if writable, otherwise `$HOME/.local/bin`.

### Install with `npm`

Install the CLI globally:

```bash
npm install -g keen-code
```

Update the global install:

```bash
npm install -g keen-code@latest
# or
npm update -g keen-code
```

`npm update` without `-g` only updates local project dependencies.

Check that the install worked:

```bash
keen --version
which keen
```

You can also run it without a global install:

```bash
npx keen-code --version
```

## Run Keen

Start Keen in your current directory:

```bash
keen
```

## Supported Providers

- Anthropic
- OpenAI
- Codex (ChatGPT OAuth)
- Google AI (Gemini)
- Moonshot AI (Kimi)
- DeepSeek
- Z.ai (GLM)
- MiniMax
- OpenCode Go
- Amazon Bedrock

> Use `/model` to switch providers. The ChatGPT/Codex option opens a browser-based OpenAI sign-in flow and stores OAuth credentials in `~/.keen/auth.json`.

MiniMax uses its Anthropic-compatible API and includes MiniMax M2.7 and M2.5.
OpenCode Go uses an API key and includes GLM, Kimi, DeepSeek, MiMo, MiniMax, and Qwen models.

## Built-in Tools

Keen Code aims to support minimal set of useful tools for coding. Currently, these tools are built in:

- `read_file` — read a UTF-8 text file with `N:HASH|` line anchors
- `glob` — find files by glob patterns
- `grep` — search for text patterns in files
- `write_file` — create or overwrite files
- `edit_file` — hash-anchored multi-op edits (`LINE:HASH` anchors in one `ops` array, applied atomically)
- `bash` — run shell commands

## Further Reading

- [TOUR.md](TOUR.md) — the full story of how this project was built
- [CHANGELOG.md](CHANGELOG.md) — release history
- [ROADMAP.md](ROADMAP.md) — what's planned next
- [CONTRIBUTING.md](CONTRIBUTING.md) — how to contribute
- [`docs/`](docs/) — architecture, tools, sessions, skills, and more

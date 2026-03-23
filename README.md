![Keen Code](./assets/keen-code.png)

# Keen Code

**Keen Code** is a terminal-based AI coding agent like Claude Code or OpenCode. Written in Go, it is simple and lightweight, avoids feature bloat, and aims at being a minimalistic coding agent.

From requirements to implementation, Keen Code was engineered using a wide range of coding agents and agentic IDEs—Cursor, Windsurf, Claude Code, OpenCode, Codex CLI, and Kimi CLI were used for developing this project.

By far, AI coding agents are the most ubiquitous use case for AI in the era of AI agents. The goal of the project is to showcase how coding agents can be used to develop the coding agents themselves. It's a fairly complicated project which helps to test the capablities and push the limits of AI coding agents for a real-world and popular use case.

## Principles

Developing Keen Code is guided by the following principles:

- All the code is written by AI agents, not humans
- The project is developed iteratively using spec-task-code-review cycle by a human engineer
- The human engineer has a very strict set of roles:
  - Specifiy and clarify the requirements
  - Review design docs and influence design decisions
  - Review **each and every change** made by the agents
  - Keep a sharp eye on the quality and correctness of the code
  - Focus on best practices and standards relevant to the programing language (Go in this case)
  - Thoroughly review and test the product after each iteration
  - Continously provide feedback to the agents to improve the product
- Prompts are saved as markdown files in the `.ai-interactions/prompts` directory
  - Almost all of the prompts are stored to showcase how the project evolved from the initial requirements to the current state
  - Prompts are pretty much chronologically ordered which demonstrates the thought process and iterative nature of the development
- All the outputs are saved as markdown files in the `.ai-interactions/outputs` directory
  - These outputs are basically plans, design docs, and breakdowns of the tasks
  - Outputs are also chronologically ordered


## Install with script

```bash
curl -fsSL https://raw.githubusercontent.com/mochow13/keen-code/main/scripts/install.sh | bash
```

To pin a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/mochow13/keen-code/main/scripts/install.sh | bash -s -- -v v0.1.4
```

Installs to `/usr/local/bin` if writable, otherwise `$HOME/.local/bin`.

## Install with `npm`

Install the CLI globally:

```bash
npm install -g keen-code
```

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
- Google AI (Gemini)
- Moonshot AI (Kimi)
- DeepSeek

More providers will be added in the future.

## Built-in Tools

Keen Code aims to support minimal set of useful tools for coding. Currently, these tools are built in:

- `read_file` — read a UTF-8 text file
- `glob` — find files by glob patterns
- `grep` — search for text patterns in files
- `write_file` — create or overwrite files
- `edit_file` — replace specific text in existing files
- `bash` — run shell commands

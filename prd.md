I want you to help me develop a coding agent CLI like Claude Code, OpenCode, or Codex CLI. The requirements I have are:

- It should be usable directly from the terminal, just like all othere CLIs
- The development language should be in Go
- It must not reinvent any wheel and use any supported or already implemented libraries in Go
- It should have the fundamental support for the tools required to perform its job of coding agent
- It should follow all the best practices of developing such a codebase
- The code should be in idiomatic Go
- In the first version, the CLI should have the core feature implemented: open in terminal > take a prompt > read files > suggest edits or update the code directly upon user's permission > explain what it did
- The CLI should also have both "plan" and "work" mode
  - In "plan" mode, the CLI should only create plans and suggestions but no edits
  - In "work" mode, CLI will edit the code but confirm user's permission for each edit


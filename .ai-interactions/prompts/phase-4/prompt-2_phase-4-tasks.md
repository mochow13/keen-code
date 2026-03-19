## Phase 4 Tasks

### System Prompt

1. Currently, in keen-code, we don't have a system prompt. Since it's a coding agent, we need one or more system prompts that are optimized for coding tasks. Explore the codebase in ../opencode and ../kimi-cli. Then based on the learnings, give me some ideas on how to approach system prompt for keen-code.
2. We will implement idea 2 and 3 where we have a static system prompt with dynamic environment variables along with rich context through AGENTS.md. Create a plan on this and save as output-3_system-prompt.md in the .ai-interactions/outputs/phase-4 directory.
3. There is a plan for system prompts in @.ai-interactions/outputs/phase-4/output-3_system-prompt.md. We need to implement this plan. First create todo list for yourself and then implement one by one.

### Distributing Keen Code

1. How can we distribute Keen Code so that people can download and use it simply by typing `keen` on terminal? Share some ideas.
2. I want you to create a plan on how we can distribute this project through
    Github pipeline in homebrew, install script, and npm. For example, I want to
    enable users to be able to download keen-code like this:

    Install Claude Code:
        **MacOS/Linux (Recommended):**
        ```bash
        curl -fsSL https://claude.ai/install.sh | bash
        ```
        **Homebrew (MacOS/Linux):**
        ```bash
        brew install --cask claude-code
        ```
        **NPM (Deprecated):**
        ```bash
        npm install -g @anthropic-ai/claude-code
3. 
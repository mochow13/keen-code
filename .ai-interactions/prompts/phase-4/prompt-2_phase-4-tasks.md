## Phase 4 Tasks

### System Prompt

1. Currently, in keen-code, we don't have a system prompt. Since it's a coding agent, we need one or more system prompts that are optimized for coding tasks. Explore the codebase in ../opencode and ../kimi-cli. Then based on the learnings, give me some ideas on how to approach system prompt for keen-code.
2. We will implement idea 2 and 3 where we have a static system prompt with dynamic environment variables along with rich context through AGENTS.md. Create a plan on this and save as output-3_system-prompt.md in the .ai-interactions/outputs/phase-4 directory.
3. There is a plan for system prompts in @.ai-interactions/outputs/phase-4/output-3_system-prompt.md. We need to implement this plan. First create todo list for yourself and then implement one by one.

### Tool Prompts

1. We want to improve the tool prompts for Keen Code. Let's improve this step by step.
    - First, figure out what tools Keen Code has right now.
    - Check each tool's Description and InputSchema.
    - Check for the similar tools in ../opencode and ../kimi-cli and their descriptions and input schemas.
    - Based on the above, create a plan on how we can improve the tools for Keen Code.
    - Save the plan as output-5_tool-prompts.md in the .ai-interactions/outputs/phase-4 directory.

### UI Improvements

1. Currently, we use quite bright colors for keen code UI. Check the styles in @internal/cli/repl/styles.go. Based on that, update the syles with proper material UI colors.

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

### Interruption

1. Let's say we interrupt a message while LLM is working on it. Then we send another message after interrupting. Right now, LLM still tries to complete the last message before interruption. Ideal behaviour will be ignore the interrupted message/task and focus on the new one. Why does it happen?
2. What if we discard the message that was interrupted from the message history?
3. Between "interruption marker" and "discarding message", which approach is followed by ../opencode and ../kimi-cli? Make a recommendation based on your research.
4. Show me the code changes required to implement the "interruption marker" approach.
5. But if we check for length in approach 1, LLM won't have the interrupted signal either.

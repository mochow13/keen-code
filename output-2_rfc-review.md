# Architecture RFC Review by Gemini 3 Pro

## Executive Summary
The proposed architecture in `ARCHITECTURE.md` provides a robust and idiomatic foundation for a Go-based Coding Agent CLI. The layered design (UI, Core, Tools, LLM) and the choice of the "Charm" stack (Bubbletea, Lipgloss) for the TUI are excellent choices that align well with modern Go CLI standards. Standardizing on `spf13/cobra` and `spf13/viper` ensures maintainability and familiar UX.

However, a few critical areas—specifically git awareness, context window management, and update mechanisms—should be addressed earlier in the development lifecycle to avoid technical debt.

## Strengths
- **Clear Separation of Concerns**: The distinction between Orchestrator, Tool Manager, and LLM Provider layers is well-defined.
- **Security First**: Explicitly designing a `FileGuard` component is a strong move for an agent that modifies files.
- **Idiomatic Stack**: The selected libraries (`cobra`, `viper`, `bubbletea`, `slog`) are the industry standard for high-quality Go tools.
- **Mode-Based Security**: The `Plan` vs `Work` mode distinction is a great way to handle user trust and safety.

## Suggestions for Improvement

### 1. Git Awareness (Elevate from Future to Phase 1)
**Issue:** The RFC lists Git integration as a future enhancement.
**Recommendation:** For a coding agent, respecting `.gitignore` is not optional; it's a requirement to avoid reading `node_modules` or `target` directories, which would waste tokens and confuse the context.
- **Action**: Add a `GitAwareness` component to the `FileSystem` layer immediately.
- **Goal**: Ensure `list_dir` and `grep` tools respect `.gitignore` by default.

### 2. Context Window Management strategy
**Issue:** The "Conversation Context" is mentioned, but mechanisms for managing limited context windows (token limits) are not detailed.
**Recommendation:** Explicitly design a `ContextManager` that handles:
- **Token Counting**: Estimate token usage before sending requests.
- **Pruning**: Strategy for dropping old messages or summarizing past tool outputs (e.g., "Output truncated..." for large reads).
- **Priority**: Keep system prompts and the latest user query as immutable, compress the middle history.

### 3. TUI Rendering & Markdown
**Issue:** The RFC mentions `lipgloss` but doesn't explicitly detail how the LLM's markdown response will be rendered.
**Recommendation:** Integrate `github.com/charmbracelet/glamour` for rendering Markdown responses in the terminal.
- **Benefit**: Provides a much better reading experience for code blocks, lists, and headers in the terminal.

### 4. Tooling Details
- **`grep` Tool**: Relying on standard `grep` can be flaky across OS (BSD vs GNU grep).
    - **Recommendation**: Use `github.com/rullzer/go-ripgrep` or bundle `ripgrep`, or implement a pure Go regex search that is performant enough for moderate codebases. given the constraint "no wheel reinvention", wrapping a robust library or binary is preferred.
- **`list_dir` Tool**: Needs a `max_depth` and `exclude` pattern support to avoid flooding the context with deep directory trees.

### 5. Update Mechanism
**Recommendation**: Add a mechanism for the CLI to check for updates. Since this tool interacts with rapidly changing LLM APIs, keeping it up-to-date is crucial.
- **Action**: Add a `version` command that checks a GitHub release endpoint.

### 6. Telemetry / Debugging
**Recommendation**: Add a `--debug` or `--trace` flag that dumps the raw prompt and response cycles to a file. This is invaluable for debugging why the agent "thought" something wrong.

## Revised Phase 1 & 2 Priorities

I recommend moving **Git Awareness** into **Phase 1 (Foundation)** or **Phase 3 (Tool System)** at the latest, rather than waiting for "Future".
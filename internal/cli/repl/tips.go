package repl

import "math/rand"

var tips = []string{
	"Use `/btw <question>` to ask a quick side question without adding it to the conversation history.",
	"Press Shift+Tab to toggle between plan and build modes instantly.",
	"Use `/adversary` to run a second AI as a critic — it reviews your conversation for bugs, security issues, and faulty assumptions.",
	"Type `@` followed by a filename fragment to get autocomplete — the resolved path is inserted directly into your prompt.",
	"Press Shift+Enter to add a new line in the input without submitting.",
	"PageUp/PageDown scrolls the output by half a page; Home/End jumps to the very top or bottom.",
	"Press Esc to interrupt an active response, cancel a `/btw` stream, or dismiss the model picker.",
	"`/compact` accepts an optional prompt to guide what context the summary should retain, e.g. `/compact focus on the API changes`.",
	"`/sessions` shows your full session history for this project — sessions are scoped per working directory.",
	"`/allow-permission bash` suppresses the dangerous-command prompt for bash; the filesystem guard still blocks system directories.",
	"`/reset-permission <tool>` restores default permission behavior after you've used `/allow-permission`.",
	"Skills are loaded from `.agents/skills/`, `.keen/skills/`, or `.claude/skills/` — in your project or home directory.",
	"Skill prompts support `$1`, `$2`, etc. for positional args and `$ARGUMENTS` for all args at once.",
	"`/show-thinking on` reveals the model's internal reasoning tokens in the output.",
	"`/thinking` accepts effort values: `low`, `medium`, `high`, or `max` — the available values depend on the provider.",
	"`/mcp connect <server>` re-authenticates by clearing the stored OAuth token, useful when your token has expired.",
	"The `grep` tool supports `output_mode: file` to return only matching filenames instead of matched lines.",
	"The `read_file` tool supports `offset` and `limit` so the model can read specific line ranges of large files.",
	"Bash commands are auto-approved — only dangerous ones like `rm` or `git push` trigger a prompt.",
	"`/adversary model` configures a separate model just for adversarial review, independent of your main model.",
	"In plan mode the model cannot make file changes — it must describe the plan first. Switch with `/mode build` when ready.",
	"`/skills list` shows all available skills including ones auto-generated from connected MCP servers.",
	"Press Tab to move focus between the input box and the output viewport for keyboard scrolling.",
	"Drag to select text in the output or input — it copies automatically when you release the mouse.",
	"Alt/Option+click or Ctrl+click a link in the output to open it in your browser.",
	"While the agent is working, your next prompts (up to 5) are queued and run automatically when it finishes.",
	"Use `/emptyq` to clear queued prompts while the agent is still working.",
	"Press Ctrl+C or Esc while idle to clear queued prompts without starting a new turn.",
}

func randomTip() string {
	return tips[rand.Intn(len(tips))]
}

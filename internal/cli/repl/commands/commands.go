package commands

import "strings"

const (
	Adversary       = "/adversary"
	AdversaryModel  = "/adversary model"
	Btw             = "/btw"
	Clear           = "/clear"
	Compact         = "/compact"
	Context         = "/context"
	Cleanup         = "/cleanup"
	EmptyQueue      = "/emptyq"
	Exit            = "/exit"
	Help            = "/help"
	Logout          = "/logout"
	Memory          = "/memory"
	MemoryShow      = "/memory show"
	Model           = "/model"
	MCP             = "/mcp"
	MCPConnect      = "/mcp connect"
	MCPStatus       = "/mcp status"
	Mode            = "/mode"
	New             = "/new"
	AllowPermission = "/allow-permission"
	ResetPermission = "/reset-permission"
	Resume          = "/resume"
	Sessions        = "/sessions"
	ShowThinking    = "/show-thinking"
	Skills          = "/skills"
	SkillsDisable   = "/skills disable"
	SkillsEnable    = "/skills enable"
	SkillsList      = "/skills list"
	SkillsReload    = "/skills reload"
	SkillsStatus    = "/skills status"
	Subagents       = "/subagents"
	SubagentsList   = "/subagents list"
	Thinking        = "/thinking"
	ToolHistory     = "/tool-history"
)

type SlashCommand struct {
	Name        string
	Description string
}

var All = []SlashCommand{
	{Adversary, "Adversarially review the conversation for issues, bugs, risks, and security problems"},
	{AdversaryModel, "Configure the adversary model"},
	{AllowPermission, "Always allow a tool (bypasses prompts, including dangerous bash)"},
	{Btw, "Ask a quick side question (not added to conversation)"},
	{Clear, "Clear the session and create a new one (also /new)"},
	{Compact, "Compact conversation context"},
	{Context, "Show context window usage breakdown"},
	{Cleanup, "Remove expired Keen data and trim input history"},
	{EmptyQueue, "Clear all queued messages"},
	{Exit, "Quit Keen"},
	{Help, "Show available commands"},
	{Logout, "Sign out of the current OAuth provider"},
	{Memory, "Show memory file paths and contents"},
	{MCP, "Show MCP status or refresh a server"},
	{Model, "Change provider or model stored in ~/.keen/configs.json"},
	{Mode, "Switch agent mode (plan|build)"},
	{New, "Start a new session (also /clear)"},
	{ResetPermission, "Reset tool permissions to Keen's default mechanism"},
	{Resume, "Resume the last session directly"},
	{Sessions, "Open the session picker"},
	{ShowThinking, "Toggle thinking token display (on|off)"},
	{Skills, "List, show status, reload, enable, or disable skills"},
	{Subagents, "List available subagents"},
	{Thinking, "Change thinking effort for the current model"},
	{ToolHistory, "Control tool output retention between turns (full|none)"},
}

var Suggestions = []SlashCommand{
	{Adversary, "Adversarially review the conversation for issues, bugs, risks, and security problems"},
	{AdversaryModel, "Configure the adversary model"},
	{AllowPermission, "Always allow a tool (bypasses prompts, including dangerous bash)"},
	{Btw, "Ask a quick side question (not added to conversation)"},
	{Clear, "Clear the session and create a new one (also /new)"},
	{Compact, "Compact conversation context"},
	{Context, "Show context window usage breakdown"},
	{Cleanup, "Remove expired Keen data and trim input history"},
	{EmptyQueue, "Clear all queued messages"},
	{Exit, "Quit Keen"},
	{Help, "Show available commands"},
	{Logout, "Sign out of the current OAuth provider"},
	{Memory, "Show memory file paths and contents"},
	{MemoryShow, "Show memory contents"},
	{MCP, "Show MCP commands"},
	{MCPConnect, "Connect an MCP server"},
	{MCPStatus, "Show MCP server status"},
	{Model, "Change provider or model stored in ~/.keen/configs.json"},
	{Mode, "Switch agent mode (plan|build)"},
	{New, "Start a new session (also /clear)"},
	{ResetPermission, "Reset tool permissions to Keen's default mechanism"},
	{Resume, "Resume the last session directly"},
	{Sessions, "Open the session picker"},
	{ShowThinking, "Toggle thinking token display (on|off)"},
	{Skills, "Show skills commands"},
	{SkillsDisable, "Disable a skill"},
	{SkillsEnable, "Enable a skill"},
	{SkillsList, "List available skills"},
	{SkillsReload, "Reload available skills"},
	{SkillsStatus, "Show skills status"},
	{Subagents, "Show subagent commands"},
	{SubagentsList, "List available subagents"},
	{Thinking, "Change thinking effort for the current model"},
	{ToolHistory, "Control tool output retention between turns (full|none)"},
}

func IsKnownCommand(input string) bool {
	if !strings.HasPrefix(input, "/") {
		return false
	}
	for _, cmd := range All {
		if input == cmd.Name || strings.HasPrefix(input, cmd.Name+" ") {
			return true
		}
	}
	return false
}

func Filter(input string) []SlashCommand {
	if input == "" || !strings.HasPrefix(input, "/") {
		return nil
	}
	prefix := strings.ToLower(strings.TrimPrefix(input, "/"))
	var results []SlashCommand
	for _, cmd := range Suggestions {
		name := strings.ToLower(strings.TrimPrefix(cmd.Name, "/"))
		if strings.HasPrefix(name, prefix) || suggestionMatchesWords(name, prefix) {
			results = append(results, cmd)
		}
	}
	return results
}

func suggestionMatchesWords(name, prefix string) bool {
	prefixWords := strings.Fields(prefix)
	if len(prefixWords) == 0 {
		return true
	}
	nameWords := strings.Fields(name)
	if len(prefixWords) > len(nameWords) {
		return false
	}
	for i, word := range prefixWords {
		if !strings.HasPrefix(nameWords[i], word) {
			return false
		}
	}
	return true
}

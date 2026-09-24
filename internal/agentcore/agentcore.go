package agentcore

import (
	"context"

	"github.com/mochow13/keen-code/internal/config"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
)

// AgentCore is the UI-facing boundary to the backend agent.
type AgentCore interface {
	IsReady() bool
	UpdateClient() error
	ClearClient()
	IsAdversaryReady() bool
	SetAdversaryClient() error
	ResetClientState()

	Mode() Mode
	SetMode(Mode)

	FormatUserMessage(content string) string
	AppendMessage(message Message)
	GetMessages() []Message
	ClearMessages()
	ReplaceMessages(messages []Message)
	ApplyCompaction(summary string) error
	ClearContextMetrics()
	WithoutSystemMessages(messages []Message) []Message

	Submit(ctx context.Context, sessionID string, input string) (<-chan StreamEvent, error)
	Compact(ctx context.Context, sessionID string, hint string) (<-chan StreamEvent, error)
	Btw(ctx context.Context, question string) (<-chan StreamEvent, error)
	Adversary(ctx context.Context, focus string) (<-chan StreamEvent, error)

	GetContextBreakdown() ContextBreakdown
	GetLastUsage() *TokenUsage
	SetLastUsage(usage *TokenUsage)

	ReloadSkills() SkillsDiscovery
	GetSkills() SkillsDiscovery
	GetSkillsConfig() SkillsConfig
	SetSkillStatus(name string, status SkillStatus) error
	RemoveSkillStatus(name string) error
	FindEnabledSkill(name string) (Skill, bool)
	SkillSuggestions() []Skill
	SkillsCatalog() string

	ReloadSubagents() SubagentsDiscovery
	GetSubagents() SubagentsDiscovery
	SubagentsCatalog() string

	WorkingDir() string
	RegisteredToolNames() []string

	Config() *config.ResolvedConfig
	SetConfig(cfg *config.ResolvedConfig)
	GlobalConfig() *config.GlobalConfig
	SetGlobalConfig(cfg *config.GlobalConfig)

	SetupTools(
		ctx context.Context,
		permissionRequester PermissionRequester,
		diffEmitter DiffEmitter,
		askUserRequester AskUserRequester,
		mcpRuntime keenmcp.Runtime,
		forwardActivity bool,
	) <-chan ToolActivity
}

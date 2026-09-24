package repl

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/llm/core"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/tools"

	tea "charm.land/bubbletea/v2"
)

// Backend initialization installs bundled skills, and session helpers use HOME.
// Keep even tests without their own home fixture away from user configuration.
func TestMain(m *testing.M) {
	os.Exit(runWithTestHome(m))
}

func runWithTestHome(m *testing.M) int {
	home, err := os.MkdirTemp("", "keen-repl-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(home)
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(key, home); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return m.Run()
}

type mockLLMClient struct {
	streamChatFunc func(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error)
	resetCount     int
}

func (m *mockLLMClient) StreamChat(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error) {
	if m.streamChatFunc != nil {
		return m.streamChatFunc(ctx, messages, toolRegistry, opts...)
	}
	ch := make(chan core.StreamEvent)
	close(ch)
	return ch, nil
}

func (m *mockLLMClient) Reset() {
	m.resetCount++
}

func newAgentCore(client llm.LLMClient, workingDir string, cfg ...*config.ResolvedConfig) agentcore.AgentCore {
	resolved := &config.ResolvedConfig{}
	if len(cfg) > 0 && cfg[0] != nil {
		resolved = cfg[0]
	}
	return agentcore.New(client, workingDir, resolved, nil)
}

func processCmd(m replModel, cmd tea.Cmd) (replModel, tea.Cmd) {
	if cmd == nil {
		return m, nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				m, _ = processCmd(m, c)
			}
		}
		return m, nil
	}
	return m.updateNormalMode(msg)
}

type mockAgentCore struct {
	submitFunc          func(ctx context.Context, sessionID, input string) (<-chan agentcore.StreamEvent, error)
	compactFunc         func(ctx context.Context, sessionID, hint string) (<-chan agentcore.StreamEvent, error)
	btwFunc             func(ctx context.Context, question string) (<-chan agentcore.StreamEvent, error)
	adversaryFunc       func(ctx context.Context, focus string) (<-chan agentcore.StreamEvent, error)
	mode                agentcore.Mode
	messages            []agentcore.Message
	ready               bool
	lastUsage           *agentcore.TokenUsage
	resetCount          int
	registeredToolNames []string
	cfg                 *config.ResolvedConfig
	globalCfg           *config.GlobalConfig
}

func (m *mockAgentCore) IsReady() bool             { return m.ready }
func (m *mockAgentCore) UpdateClient() error       { return nil }
func (m *mockAgentCore) ClearClient()              { m.ready = false }
func (m *mockAgentCore) ResetClientState()         { m.resetCount++ }
func (m *mockAgentCore) IsAdversaryReady() bool    { return false }
func (m *mockAgentCore) SetAdversaryClient() error { return nil }

func (m *mockAgentCore) Mode() agentcore.Mode {
	switch m.mode {
	case agentcore.ModePlan, agentcore.ModeBuild, agentcore.ModeYolo:
		return m.mode
	default:
		return agentcore.ModeBuild
	}
}
func (m *mockAgentCore) SetMode(mode agentcore.Mode) {
	switch mode {
	case agentcore.ModePlan, agentcore.ModeYolo:
		m.mode = mode
	default:
		m.mode = agentcore.ModeBuild
	}
}

func (m *mockAgentCore) FormatUserMessage(content string) string { return content }
func (m *mockAgentCore) AppendMessage(message agentcore.Message) {
	m.messages = append(m.messages, message)
}
func (m *mockAgentCore) GetMessages() []agentcore.Message             { return m.messages }
func (m *mockAgentCore) ReplaceMessages(messages []agentcore.Message) { m.messages = messages }
func (m *mockAgentCore) ApplyCompaction(summary string) error {
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("compaction returned empty summary")
	}
	m.messages = []agentcore.Message{{Role: agentcore.RoleUser, Content: summary}}
	return nil
}
func (m *mockAgentCore) ClearContextMetrics() {}
func (m *mockAgentCore) ClearMessages()       { m.messages = nil }
func (m *mockAgentCore) WithoutSystemMessages(messages []agentcore.Message) []agentcore.Message {
	result := make([]agentcore.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != agentcore.RoleSystem {
			result = append(result, msg)
		}
	}
	return result
}

func (m *mockAgentCore) Submit(ctx context.Context, sessionID, input string) (<-chan agentcore.StreamEvent, error) {
	m.messages = append(m.messages, agentcore.Message{Role: agentcore.RoleUser, Content: input})
	if m.submitFunc != nil {
		return m.submitFunc(ctx, sessionID, input)
	}
	ch := make(chan agentcore.StreamEvent)
	close(ch)
	return ch, nil
}
func (m *mockAgentCore) Compact(ctx context.Context, sessionID, hint string) (<-chan agentcore.StreamEvent, error) {
	if m.compactFunc != nil {
		return m.compactFunc(ctx, sessionID, hint)
	}
	ch := make(chan agentcore.StreamEvent)
	close(ch)
	return ch, nil
}
func (m *mockAgentCore) Btw(ctx context.Context, question string) (<-chan agentcore.StreamEvent, error) {
	if m.btwFunc != nil {
		return m.btwFunc(ctx, question)
	}
	ch := make(chan agentcore.StreamEvent)
	close(ch)
	return ch, nil
}
func (m *mockAgentCore) Adversary(ctx context.Context, focus string) (<-chan agentcore.StreamEvent, error) {
	if m.adversaryFunc != nil {
		return m.adversaryFunc(ctx, focus)
	}
	ch := make(chan agentcore.StreamEvent)
	close(ch)
	return ch, nil
}

func (m *mockAgentCore) GetContextBreakdown() agentcore.ContextBreakdown {
	return agentcore.ContextBreakdown{}
}
func (m *mockAgentCore) GetLastUsage() *agentcore.TokenUsage      { return m.lastUsage }
func (m *mockAgentCore) SetLastUsage(usage *agentcore.TokenUsage) { m.lastUsage = usage }

func (m *mockAgentCore) ReloadSkills() agentcore.SkillsDiscovery { return agentcore.SkillsDiscovery{} }
func (m *mockAgentCore) GetSkills() agentcore.SkillsDiscovery    { return agentcore.SkillsDiscovery{} }
func (m *mockAgentCore) GetSkillsConfig() agentcore.SkillsConfig {
	return agentcore.SkillsConfig{IsEnabled: map[string]bool{}}
}
func (m *mockAgentCore) SetSkillStatus(name string, status agentcore.SkillStatus) error { return nil }
func (m *mockAgentCore) RemoveSkillStatus(name string) error                            { return nil }
func (m *mockAgentCore) FindEnabledSkill(name string) (agentcore.Skill, bool) {
	return agentcore.Skill{}, false
}
func (m *mockAgentCore) SkillSuggestions() []agentcore.Skill { return nil }
func (m *mockAgentCore) SkillsCatalog() string               { return "" }

func (m *mockAgentCore) ReloadSubagents() agentcore.SubagentsDiscovery {
	return agentcore.SubagentsDiscovery{}
}
func (m *mockAgentCore) GetSubagents() agentcore.SubagentsDiscovery {
	return agentcore.SubagentsDiscovery{}
}
func (m *mockAgentCore) SubagentsCatalog() string { return "" }

func (m *mockAgentCore) RegisteredToolNames() []string { return m.registeredToolNames }

func (m *mockAgentCore) Config() *config.ResolvedConfig { return m.cfg }

func (m *mockAgentCore) SetConfig(cfg *config.ResolvedConfig) { m.cfg = cfg }

func (m *mockAgentCore) GlobalConfig() *config.GlobalConfig { return m.globalCfg }

func (m *mockAgentCore) SetGlobalConfig(cfg *config.GlobalConfig) { m.globalCfg = cfg }

func (m *mockAgentCore) WorkingDir() string { return "" }

func (m *mockAgentCore) SetupTools(context.Context, agentcore.PermissionRequester, agentcore.DiffEmitter, agentcore.AskUserRequester, keenmcp.Runtime, bool) <-chan agentcore.ToolActivity {
	return nil
}

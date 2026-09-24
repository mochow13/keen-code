package agentcore

import (
	"context"
	"sync"

	"github.com/mochow13/keen-code/internal/appstate"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/skills"
	"github.com/mochow13/keen-code/internal/subagents"
)

type adapter struct {
	mu        sync.RWMutex
	appState  *appstate.AppState
	cfg       *config.ResolvedConfig
	globalCfg *config.GlobalConfig
}

func New(client llm.LLMClient, workingDir string, cfg *config.ResolvedConfig, globalCfg *config.GlobalConfig) AgentCore {
	appState := appstate.New(client, workingDir)
	return &adapter{appState: appState, cfg: cfg, globalCfg: globalCfg}
}

func (a *adapter) Config() *config.ResolvedConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *adapter) SetConfig(cfg *config.ResolvedConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg
}

func (a *adapter) GlobalConfig() *config.GlobalConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.globalCfg
}

func (a *adapter) SetGlobalConfig(cfg *config.GlobalConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.globalCfg = cfg
}

func (a *adapter) IsReady() bool {
	return a.appState.IsClientReady(a.Config())
}

func (a *adapter) UpdateClient() error {
	client, err := llm.NewClient(a.Config())
	if err != nil {
		return err
	}
	a.appState.UpdateClient(client)
	return nil
}

func (a *adapter) ClearClient() {
	a.appState.UpdateClient(nil)
}

func (a *adapter) IsAdversaryReady() bool {
	return a.appState.IsAdversaryClientReady()
}

func (a *adapter) SetAdversaryClient() error {
	resolved, err := config.ResolveAdversary(a.GlobalConfig())
	if err != nil {
		return err
	}
	client, err := llm.NewClient(resolved)
	if err != nil {
		return err
	}
	a.appState.SetAdversaryClient(client)
	return nil
}

func (a *adapter) ResetClientState() {
	a.appState.ResetClientState()
}

func (a *adapter) Mode() Mode {
	return toMode(a.appState.Mode())
}

func (a *adapter) SetMode(mode Mode) {
	a.appState.SetMode(fromMode(mode))
}

func (a *adapter) FormatUserMessage(content string) string {
	return a.appState.FormatUserMessage(content)
}

func (a *adapter) AppendMessage(message Message) {
	a.appState.AppendMessage(fromMessage(message))
}

func (a *adapter) GetMessages() []Message {
	return toMessages(a.appState.GetMessages())
}

func (a *adapter) ClearMessages() {
	a.appState.ClearMessages()
}

func (a *adapter) ReplaceMessages(messages []Message) {
	a.appState.ReplaceMessages(fromMessages(messages))
}

func (a *adapter) ApplyCompaction(summary string) error {
	return a.appState.ApplyCompaction(summary)
}

func (a *adapter) ClearContextMetrics() {
	a.appState.ClearContextMetrics()
}

func (a *adapter) WithoutSystemMessages(messages []Message) []Message {
	return toMessages(appstate.WithoutSystemMessages(fromMessages(messages)))
}

func (a *adapter) Submit(ctx context.Context, sessionID string, input string) (<-chan StreamEvent, error) {
	ctx = subagents.WithParentSessionID(ctx, sessionID)
	a.appState.AddUserMessage(input)
	eventCh, err := a.appState.StreamChat(ctx, a.Config(), core.StreamOptions{SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	return a.adaptStream(ctx, eventCh), nil
}

func (a *adapter) Compact(ctx context.Context, sessionID string, hint string) (<-chan StreamEvent, error) {
	eventCh, err := a.appState.StreamCompact(ctx, a.Config(), hint, core.StreamOptions{SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	return a.adaptStream(ctx, eventCh), nil
}

func (a *adapter) Btw(ctx context.Context, question string) (<-chan StreamEvent, error) {
	eventCh, err := a.appState.StreamBtw(ctx, question)
	if err != nil {
		return nil, err
	}
	return a.adaptStream(ctx, eventCh), nil
}

func (a *adapter) Adversary(ctx context.Context, focus string) (<-chan StreamEvent, error) {
	eventCh, err := a.appState.StreamAdversary(ctx, focus)
	if err != nil {
		return nil, err
	}
	return a.adaptStream(ctx, eventCh), nil
}

func (a *adapter) GetContextBreakdown() ContextBreakdown {
	return toContextBreakdown(a.appState.GetContextBreakdown())
}

func (a *adapter) GetLastUsage() *TokenUsage {
	return toTokenUsage(a.appState.GetLastUsage())
}

func (a *adapter) SetLastUsage(usage *TokenUsage) {
	a.appState.SetLastUsage(fromTokenUsage(usage))
}

func (a *adapter) ReloadSkills() SkillsDiscovery {
	return toSkillsDiscovery(a.appState.ReloadSkills())
}

func (a *adapter) GetSkills() SkillsDiscovery {
	return toSkillsDiscovery(a.appState.GetSkills())
}

func (a *adapter) GetSkillsConfig() SkillsConfig {
	return toSkillsConfig(a.appState.GetSkillsConfig())
}

func (a *adapter) SetSkillStatus(name string, status SkillStatus) error {
	return a.appState.SetSkillStatus(name, skills.Status(status))
}

func (a *adapter) RemoveSkillStatus(name string) error {
	return a.appState.RemoveSkillStatus(name)
}

func (a *adapter) FindEnabledSkill(name string) (Skill, bool) {
	skill, ok := a.appState.FindEnabledSkill(name)
	return toSkill(skill), ok
}

func (a *adapter) SkillSuggestions() []Skill {
	suggestions := a.appState.SkillSuggestions()
	result := make([]Skill, len(suggestions))
	for i, skill := range suggestions {
		result[i] = toSkill(skill)
	}
	return result
}

func (a *adapter) SkillsCatalog() string {
	return a.appState.SkillsCatalog()
}

func (a *adapter) ReloadSubagents() SubagentsDiscovery {
	return toSubagentsDiscovery(a.appState.ReloadSubagents())
}

func (a *adapter) GetSubagents() SubagentsDiscovery {
	return toSubagentsDiscovery(a.appState.GetSubagents())
}

func (a *adapter) SubagentsCatalog() string {
	return a.appState.SubagentsCatalog()
}

func (a *adapter) WorkingDir() string {
	return a.appState.WorkingDir()
}

func (a *adapter) RegisteredToolNames() []string {
	registry := a.appState.GetToolRegistry()
	if registry == nil {
		return nil
	}
	all := registry.All()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.Name())
	}
	return names
}

func (a *adapter) adaptStream(ctx context.Context, eventCh <-chan core.StreamEvent) <-chan StreamEvent {
	if eventCh == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	out := make(chan StreamEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-eventCh:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- toStreamEvent(event):
				}
			}
		}
	}()
	return out
}

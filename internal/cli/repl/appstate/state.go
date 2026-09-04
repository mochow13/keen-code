package appstate

import (
	"context"
	"fmt"
	"strings"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/skills"
	"github.com/mochow13/keen-code/internal/subagents"
	"github.com/mochow13/keen-code/internal/tools"
)

const compactionUserInstruction = "Please compact this conversation according to the system instructions."

type AppState struct {
	messages        []llm.Message
	llmClient       llm.LLMClient
	adversaryClient llm.LLMClient
	toolRegistry    *tools.Registry
	mode            llm.AgentMode
	workingDir      string
	lastUsage       *llm.TokenUsage
	skills          skills.Discovery
	skillsConfig    skills.Config
	subagents       subagents.Discovery
}

func New(client llm.LLMClient, workingDir string) *AppState {
	state := &AppState{
		messages:     []llm.Message{},
		llmClient:    client,
		toolRegistry: tools.NewRegistry(),
		mode:         llm.ModeBuild,
		workingDir:   workingDir,
	}
	state.ReloadSkills()
	state.ReloadSubagents()
	return state
}

func (s *AppState) AddMessage(role llm.Role, content string) {
	s.AppendMessage(llm.Message{
		Role:    role,
		Content: content,
	})
}

func (s *AppState) AppendMessage(message llm.Message) {
	s.messages = append(s.messages, llm.CloneMessage(message))
}

func (s *AppState) GetMessages() []llm.Message {
	return llm.CloneMessages(s.messages)
}

func (s *AppState) ClearMessages() {
	s.messages = []llm.Message{}
}

func (s *AppState) ReloadSkills() skills.Discovery {
	s.skillsConfig = skills.LoadConfig()
	if strings.TrimSpace(s.workingDir) == "" {
		s.skills = skills.Discovery{}
		return s.GetSkills()
	}
	bundledDir, bundledErr := skills.EnsureBundled()
	discovery := skills.LoadMetadata(skills.Discover(s.workingDir, bundledDir))
	if bundledErr != nil {
		discovery.Warnings = append(discovery.Warnings, "Bundled skills failed to extract: "+bundledErr.Error())
	}
	s.skills = discovery
	return s.GetSkills()
}

func (s *AppState) GetSkills() skills.Discovery {
	return skills.Discovery{
		Skills:   append([]skills.Skill(nil), s.skills.Skills...),
		Warnings: append([]string(nil), s.skills.Warnings...),
	}
}

func (s *AppState) GetSkillsConfig() skills.Config {
	return cloneSkillsConfig(s.skillsConfig)
}

func (s *AppState) SetSkillStatus(name string, status skills.Status) error {
	cfg := cloneSkillsConfig(s.skillsConfig)
	cfg.SetStatus(name, status)
	if err := skills.SaveConfig(cfg); err != nil {
		return err
	}
	s.skillsConfig = cfg
	return nil
}

func (s *AppState) RemoveSkillStatus(name string) error {
	cfg := cloneSkillsConfig(s.skillsConfig)
	cfg.RemoveStatus(name)
	if err := skills.SaveConfig(cfg); err != nil {
		return err
	}
	s.skillsConfig = cfg
	return nil
}

func (s *AppState) FindEnabledSkill(name string) (skills.Skill, bool) {
	skill, ok := skills.Find(s.skills.Skills, name)
	if !ok || !s.skillsConfig.Enabled(skill.Name) {
		return skills.Skill{}, false
	}
	return skill, true
}

func (s *AppState) SkillSuggestions() []skills.Skill {
	items := make([]skills.Skill, 0, len(s.skills.Skills))
	for _, skill := range s.skills.Skills {
		if s.skillsConfig.Enabled(skill.Name) {
			items = append(items, skill)
		}
	}
	return items
}

func (s *AppState) SkillsCatalog() string {
	return skills.Catalog(s.skills.Skills, s.skillsConfig)
}

func (s *AppState) ReloadSubagents() subagents.Discovery {
	if strings.TrimSpace(s.workingDir) == "" {
		s.subagents = subagents.Discovery{}
		return s.GetSubagents()
	}
	s.subagents = subagents.LoadMetadata(subagents.Discover(s.workingDir))
	return s.GetSubagents()
}

func (s *AppState) GetSubagents() subagents.Discovery {
	return subagents.Discovery{
		Profiles: append([]subagents.Profile(nil), s.subagents.Profiles...),
		Warnings: append([]string(nil), s.subagents.Warnings...),
	}
}

func (s *AppState) SubagentsCatalog() string {
	return subagents.Catalog(s.subagents.Profiles)
}

func cloneSkillsConfig(cfg skills.Config) skills.Config {
	cloned := skills.Config{IsEnabled: map[string]bool{}}
	for name, enabled := range cfg.IsEnabled {
		cloned.IsEnabled[name] = enabled
	}
	return cloned
}

func (s *AppState) ResetClientState() {
	if s.llmClient != nil {
		s.llmClient.Reset()
	}
}

func WithoutSystemMessages(messages []llm.Message) []llm.Message {
	withoutSystem := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		if message.Role != llm.RoleSystem {
			withoutSystem = append(withoutSystem, message)
		}
	}
	return llm.CloneMessages(withoutSystem)
}

func (s *AppState) ReplaceMessages(messages []llm.Message) {
	s.messages = llm.CloneMessages(messages)
}

func (s *AppState) StreamChat(ctx context.Context, cfg *config.ResolvedConfig, opts ...llm.StreamOptions) (<-chan llm.StreamEvent, error) {
	if s.llmClient == nil {
		return nil, nil
	}
	systemMsg := llm.Message{
		Role:    llm.RoleSystem,
		Content: llm.Build(s.workingDir, s.SkillsCatalog(), s.SubagentsCatalog(), s.mode),
	}
	messages := append([]llm.Message{systemMsg}, s.GetMessages()...)
	return s.llmClient.StreamChat(ctx, messages, s.EffectiveToolRegistry(), opts...)
}

func (s *AppState) buildCompactionRequest(cfg *config.ResolvedConfig, extraPrompt string) ([]llm.Message, error) {
	if len(s.messages) == 0 {
		return nil, nil
	}
	if s.llmClient == nil {
		return nil, fmt.Errorf("LLM client not initialized")
	}
	if cfg == nil || cfg.Model == "" || (config.RequiresAPIKey(cfg.Provider) && cfg.APIKey == "") {
		return nil, fmt.Errorf("LLM client not initialized")
	}

	snapshot := s.GetMessages()
	requestMessages := make([]llm.Message, 0, len(snapshot)+2)
	requestMessages = append(requestMessages, llm.Message{
		Role:    llm.RoleSystem,
		Content: llm.BuildCompactionPrompt(extraPrompt),
	})
	requestMessages = append(requestMessages, snapshot...)
	requestMessages = append(requestMessages, llm.Message{
		Role:    llm.RoleUser,
		Content: compactionUserInstruction,
	})
	return requestMessages, nil
}

func (s *AppState) StreamCompact(ctx context.Context, cfg *config.ResolvedConfig, extraPrompt string, opts ...llm.StreamOptions) (<-chan llm.StreamEvent, error) {
	requestMessages, err := s.buildCompactionRequest(cfg, extraPrompt)
	if err != nil || requestMessages == nil {
		return nil, err
	}
	return s.llmClient.StreamChat(ctx, requestMessages, nil, opts...)
}

func (s *AppState) StreamBtw(ctx context.Context, question string, opts ...llm.StreamOptions) (<-chan llm.StreamEvent, error) {
	if s.llmClient == nil {
		return nil, nil
	}
	history := btwContext(s.messages, 10)
	messages := make([]llm.Message, 0, 2+len(history))
	messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: llm.BuildBtwPrompt(s.workingDir)})
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: question})
	streamOpts := llm.StreamOptions{OneShot: true}
	if len(opts) > 0 {
		streamOpts.SessionID = opts[0].SessionID
	}
	return s.llmClient.StreamChat(ctx, messages, nil, streamOpts)
}

func (s *AppState) StreamAdversary(ctx context.Context, focus string) (<-chan llm.StreamEvent, error) {
	if s.adversaryClient == nil {
		return nil, nil
	}
	history := s.GetMessages()
	messages := make([]llm.Message, 0, 2+len(history))
	messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: llm.BuildAdversaryPrompt(s.workingDir)})
	for _, msg := range history {
		if msg.Role == llm.RoleAssistant {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: "[main agent]: " + msg.Content})
		} else {
			messages = append(messages, msg)
		}
	}
	instruction := "Review this conversation."
	if focus != "" {
		instruction = focus
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: instruction})
	readOnlyRegistry := s.toolRegistry.Without(
		tools.WriteFileToolName,
		tools.EditFileToolName,
		tools.BashToolName,
		tools.CallMCPToolName,
		tools.DelegateToolName,
	)
	return s.adversaryClient.StreamChat(ctx, messages, readOnlyRegistry, llm.StreamOptions{OneShot: true})
}

func btwContext(messages []llm.Message, max int) []llm.Message {
	end := len(messages)
	if end > 0 && messages[end-1].Role == llm.RoleUser {
		end--
	}
	if end == 0 {
		return nil
	}
	start := end - max
	if start < 0 {
		start = 0
	}
	result := make([]llm.Message, end-start)
	copy(result, messages[start:end])
	return result
}

func (s *AppState) ApplyCompaction(summary string) error {
	compacted := strings.TrimSpace(summary)
	if compacted == "" {
		return fmt.Errorf("compaction returned empty summary")
	}
	s.messages = []llm.Message{{
		Role:    llm.RoleUser,
		Content: compacted,
	}}
	return nil
}

func (s *AppState) IsClientReady(cfg *config.ResolvedConfig) bool {
	return s.llmClient != nil && cfg != nil && cfg.Model != "" && (!config.RequiresAPIKey(cfg.Provider) || cfg.APIKey != "")
}

func (s *AppState) UpdateClient(client llm.LLMClient) {
	s.llmClient = client
}

func (s *AppState) SetAdversaryClient(client llm.LLMClient) {
	s.adversaryClient = client
}

func (s *AppState) IsAdversaryClientReady() bool {
	return s.adversaryClient != nil
}

func (s *AppState) GetClient() llm.LLMClient {
	return s.llmClient
}

func (s *AppState) GetToolRegistry() *tools.Registry {
	return s.toolRegistry
}

func (s *AppState) EffectiveToolRegistry() *tools.Registry {
	if s.mode == llm.ModePlan {
		return s.toolRegistry.Without(tools.WriteFileToolName, tools.EditFileToolName)
	}
	return s.toolRegistry
}

func (s *AppState) RegisterTool(tool tools.Tool) error {
	return s.toolRegistry.Register(tool)
}

func (s *AppState) SetMode(mode llm.AgentMode) {
	if mode != llm.ModePlan {
		mode = llm.ModeBuild
	}
	s.mode = mode
}

func (s *AppState) Mode() llm.AgentMode {
	if s.mode == "" {
		return llm.ModeBuild
	}
	return s.mode
}

func (s *AppState) WorkingDir() string {
	return s.workingDir
}

func (s *AppState) SetLastUsage(usage *llm.TokenUsage) {
	if usage == nil {
		s.lastUsage = nil
		return
	}
	cloned := *usage
	s.lastUsage = &cloned
}

func (s *AppState) GetLastUsage() *llm.TokenUsage {
	if s.lastUsage == nil {
		return nil
	}
	cloned := *s.lastUsage
	return &cloned
}

func (s *AppState) ClearContextMetrics() {
	s.lastUsage = nil
}

// GetContextBreakdown estimates token usage per context category. When the
// provider has reported input token usage, category counts are scaled so they
// sum to the reported total; otherwise raw heuristic estimates are returned.
func (s *AppState) GetContextBreakdown() llm.ContextBreakdown {
	systemPrompt := llm.Build(s.workingDir, s.SkillsCatalog(), s.SubagentsCatalog(), s.Mode())

	var toolDefs []llm.ContextToolDef
	for _, tool := range s.EffectiveToolRegistry().All() {
		toolDefs = append(toolDefs, llm.ContextToolDef{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}

	messages := make([]llm.ContextMessage, 0, len(s.messages))
	for _, msg := range s.messages {
		cm := llm.ContextMessage{Role: msg.Role, Content: msg.Content}
		if msg.TurnMemory != nil {
			cm.ToolActivity = msg.TurnMemory.ToolActivity
		}
		messages = append(messages, cm)
	}

	breakdown := llm.EstimateContextBreakdown(systemPrompt, toolDefs, messages)

	if usage := s.GetLastUsage(); usage != nil && usage.InputTokens > 0 && breakdown.TotalEstimated > 0 {
		scale := float64(usage.InputTokens) / float64(breakdown.TotalEstimated)
		breakdown.SystemPromptTokens = scaleTokens(breakdown.SystemPromptTokens, scale)
		breakdown.ToolDefTokens = scaleTokens(breakdown.ToolDefTokens, scale)
		breakdown.UserMessageTokens = scaleTokens(breakdown.UserMessageTokens, scale)
		breakdown.AssistantTokens = scaleTokens(breakdown.AssistantTokens, scale)
		breakdown.ToolResultTokens = scaleTokens(breakdown.ToolResultTokens, scale)
		breakdown.TotalEstimated = usage.InputTokens
	}

	return breakdown
}

func scaleTokens(tokens int, scale float64) int {
	return int(float64(tokens)*scale + 0.5)
}

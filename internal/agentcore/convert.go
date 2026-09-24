package agentcore

import (
	"maps"

	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/skills"
	"github.com/mochow13/keen-code/internal/subagents"
	"github.com/mochow13/keen-code/internal/tools"
)

func toMode(mode llm.AgentMode) Mode {
	switch mode {
	case llm.ModePlan:
		return ModePlan
	case llm.ModeYolo:
		return ModeYolo
	case llm.ModeBuild:
		return ModeBuild
	default:
		return Mode(mode)
	}
}

func fromMode(mode Mode) llm.AgentMode {
	switch mode {
	case ModePlan:
		return llm.ModePlan
	case ModeYolo:
		return llm.ModeYolo
	case ModeBuild:
		return llm.ModeBuild
	default:
		return llm.AgentMode(mode)
	}
}

func toRole(role core.Role) Role {
	switch role {
	case core.RoleSystem:
		return RoleSystem
	case core.RoleUser:
		return RoleUser
	case core.RoleAssistant:
		return RoleAssistant
	default:
		return Role(role)
	}
}

func fromRole(role Role) core.Role {
	switch role {
	case RoleSystem:
		return core.RoleSystem
	case RoleUser:
		return core.RoleUser
	case RoleAssistant:
		return core.RoleAssistant
	default:
		return core.Role(role)
	}
}

func toMessage(msg core.Message) Message {
	return Message{
		Role:       toRole(msg.Role),
		Content:    msg.Content,
		TurnMemory: toTurnMemory(msg.TurnMemory),
	}
}

func fromMessage(msg Message) core.Message {
	return core.Message{
		Role:       fromRole(msg.Role),
		Content:    msg.Content,
		TurnMemory: fromTurnMemory(msg.TurnMemory),
	}
}

func toMessages(messages []core.Message) []Message {
	result := make([]Message, len(messages))
	for i, msg := range messages {
		result[i] = toMessage(msg)
	}
	return result
}

func fromMessages(messages []Message) []core.Message {
	result := make([]core.Message, len(messages))
	for i, msg := range messages {
		result[i] = fromMessage(msg)
	}
	return result
}

func toTurnMemory(memory *core.TurnMemory) *TurnMemory {
	if memory == nil {
		return nil
	}
	activities := make([]HistoricalToolActivity, len(memory.ToolActivity))
	for i, a := range memory.ToolActivity {
		activities[i] = HistoricalToolActivity{
			TextOffset:     a.TextOffset,
			Tool:           a.Tool,
			Input:          cloneInputMap(a.Input),
			Status:         a.Status,
			ExitCode:       a.ExitCode,
			HasRawOutput:   a.HasRawOutput,
			RawOutput:      cloneInputValue(a.RawOutput),
			RetainedOutput: cloneInputValue(a.RetainedOutput),
		}
	}
	return &TurnMemory{ToolActivity: activities}
}

func fromTurnMemory(memory *TurnMemory) *core.TurnMemory {
	if memory == nil {
		return nil
	}
	activities := make([]core.HistoricalToolActivity, len(memory.ToolActivity))
	for i, a := range memory.ToolActivity {
		activities[i] = core.HistoricalToolActivity{
			TextOffset:     a.TextOffset,
			Tool:           a.Tool,
			Input:          cloneInputMap(a.Input),
			Status:         a.Status,
			ExitCode:       a.ExitCode,
			HasRawOutput:   a.HasRawOutput,
			RawOutput:      cloneInputValue(a.RawOutput),
			RetainedOutput: cloneInputValue(a.RetainedOutput),
		}
	}
	return &core.TurnMemory{ToolActivity: activities}
}

func cloneInputMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneInputValue(value)
	}
	return cloned
}

func cloneInputValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneInputMap(value)
	case []any:
		cloned := make([]any, len(value))
		for i, item := range value {
			cloned[i] = cloneInputValue(item)
		}
		return cloned
	case []string:
		cloned := make([]string, len(value))
		copy(cloned, value)
		return cloned
	case []map[string]any:
		cloned := make([]map[string]any, len(value))
		for i, item := range value {
			cloned[i] = cloneInputMap(item)
		}
		return cloned
	case map[string][]map[string]any:
		cloned := make(map[string][]map[string]any, len(value))
		for key, matches := range value {
			clonedMatches := make([]map[string]any, len(matches))
			for i, match := range matches {
				clonedMatches[i] = cloneInputMap(match)
			}
			cloned[key] = clonedMatches
		}
		return cloned
	default:
		return value
	}
}

func toToolCall(toolCall *core.ToolCall) *ToolCall {
	if toolCall == nil {
		return nil
	}
	return &ToolCall{
		Name:     toolCall.Name,
		Input:    cloneInputMap(toolCall.Input),
		Output:   cloneInputValue(toolCall.Output),
		Error:    toolCall.Error,
		Duration: toolCall.Duration,
	}
}

func fromToolCall(toolCall *ToolCall) *core.ToolCall {
	if toolCall == nil {
		return nil
	}
	return &core.ToolCall{
		Name:     toolCall.Name,
		Input:    cloneInputMap(toolCall.Input),
		Output:   cloneInputValue(toolCall.Output),
		Error:    toolCall.Error,
		Duration: toolCall.Duration,
	}
}

func toTokenUsage(usage *core.TokenUsage) *TokenUsage {
	if usage == nil {
		return nil
	}
	return &TokenUsage{
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		TotalTokens:     usage.TotalTokens,
		ReasoningTokens: usage.ReasoningTokens,
		CachedTokens:    usage.CachedTokens,
	}
}

func fromTokenUsage(usage *TokenUsage) *core.TokenUsage {
	if usage == nil {
		return nil
	}
	return &core.TokenUsage{
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		TotalTokens:     usage.TotalTokens,
		ReasoningTokens: usage.ReasoningTokens,
		CachedTokens:    usage.CachedTokens,
	}
}

func toAutoCompactionEvent(event *core.AutoCompactionEvent) *AutoCompactionEvent {
	if event == nil {
		return nil
	}
	return &AutoCompactionEvent{
		Cancel:      event.Cancel,
		Replacement: toMessages(event.Replacement),
		Usage:       toTokenUsage(event.Usage),
		Error:       event.Error,
	}
}

func fromAutoCompactionEvent(event *AutoCompactionEvent) *core.AutoCompactionEvent {
	if event == nil {
		return nil
	}
	return &core.AutoCompactionEvent{
		Cancel:      event.Cancel,
		Replacement: fromMessages(event.Replacement),
		Usage:       fromTokenUsage(event.Usage),
		Error:       event.Error,
	}
}

func toStreamEvent(event core.StreamEvent) StreamEvent {
	return StreamEvent{
		Type:           toStreamEventType(event.Type),
		Content:        event.Content,
		Error:          event.Error,
		ToolCall:       toToolCall(event.ToolCall),
		Usage:          toTokenUsage(event.Usage),
		Attempt:        event.Attempt,
		AutoCompaction: toAutoCompactionEvent(event.AutoCompaction),
	}
}

func toStreamEventType(eventType core.StreamEventType) StreamEventType {
	switch eventType {
	case core.StreamEventTypeChunk:
		return StreamEventTypeChunk
	case core.StreamEventTypeReasoningChunk:
		return StreamEventTypeReasoningChunk
	case core.StreamEventTypeDone:
		return StreamEventTypeDone
	case core.StreamEventTypeError:
		return StreamEventTypeError
	case core.StreamEventTypeToolStart:
		return StreamEventTypeToolStart
	case core.StreamEventTypeToolEnd:
		return StreamEventTypeToolEnd
	case core.StreamEventTypeUsage:
		return StreamEventTypeUsage
	case core.StreamEventTypeRetry:
		return StreamEventTypeRetry
	case core.StreamEventTypeIncomplete:
		return StreamEventTypeIncomplete
	case core.StreamEventTypeAutoCompactionStarted:
		return StreamEventTypeAutoCompactionStarted
	case core.StreamEventTypeAutoCompactionApplied:
		return StreamEventTypeAutoCompactionApplied
	case core.StreamEventTypeAutoCompactionCancelled:
		return StreamEventTypeAutoCompactionCancelled
	case core.StreamEventTypeAutoCompactionFailed:
		return StreamEventTypeAutoCompactionFailed
	default:
		return StreamEventType(eventType)
	}
}

func toContextBreakdown(breakdown core.ContextBreakdown) ContextBreakdown {
	return ContextBreakdown{
		SystemPromptTokens:  breakdown.SystemPromptTokens,
		ToolDefinitionCount: breakdown.ToolDefinitionCount,
		ToolDefTokens:       breakdown.ToolDefTokens,
		UserMessageTokens:   breakdown.UserMessageTokens,
		AssistantTokens:     breakdown.AssistantTokens,
		ToolResultTokens:    breakdown.ToolResultTokens,
		TotalEstimated:      breakdown.TotalEstimated,
	}
}

func ConvertToolActivity(activity subagents.ToolActivity) ToolActivity {
	return ToolActivity{
		RunID:  activity.RunID,
		CallID: activity.CallID,
		Agent:  activity.Agent,
		Event:  toStreamEvent(activity.Event),
	}
}

func FromCoreMessages(messages []core.Message) []Message {
	return toMessages(messages)
}

func ToCoreMessages(messages []Message) []core.Message {
	return fromMessages(messages)
}

func ToCoreTurnMemory(memory *TurnMemory) *core.TurnMemory {
	return fromTurnMemory(memory)
}

func CloneTurnMemory(memory *TurnMemory) *TurnMemory {
	if memory == nil {
		return nil
	}
	activities := make([]HistoricalToolActivity, len(memory.ToolActivity))
	for i, a := range memory.ToolActivity {
		activities[i] = HistoricalToolActivity{
			TextOffset:     a.TextOffset,
			Tool:           a.Tool,
			Input:          cloneInputMap(a.Input),
			Status:         a.Status,
			ExitCode:       a.ExitCode,
			HasRawOutput:   a.HasRawOutput,
			RawOutput:      cloneInputValue(a.RawOutput),
			RetainedOutput: cloneInputValue(a.RetainedOutput),
		}
	}
	return &TurnMemory{ToolActivity: activities}
}

func toSkill(skill skills.Skill) Skill {
	return Skill{
		Name:        skill.Name,
		Description: skill.Description,
		Location:    skill.Location,
	}
}

func toSkillsDiscovery(discovery skills.Discovery) SkillsDiscovery {
	result := SkillsDiscovery{
		Skills:   make([]Skill, len(discovery.Skills)),
		Warnings: append([]string(nil), discovery.Warnings...),
	}
	for i, skill := range discovery.Skills {
		result.Skills[i] = toSkill(skill)
	}
	return result
}

func toSkillsConfig(cfg skills.Config) SkillsConfig {
	isEnabled := make(map[string]bool, len(cfg.IsEnabled))
	maps.Copy(isEnabled, cfg.IsEnabled)
	return SkillsConfig{IsEnabled: isEnabled}
}

func fromSkillsConfig(cfg SkillsConfig) skills.Config {
	isEnabled := make(map[string]bool, len(cfg.IsEnabled))
	maps.Copy(isEnabled, cfg.IsEnabled)
	return skills.Config{IsEnabled: isEnabled}
}

func toSubagentProfile(profile subagents.Profile) SubagentProfile {
	return SubagentProfile{
		Name:        profile.Name,
		Description: profile.Description,
		Hidden:      profile.Hidden,
	}
}

func toSubagentsDiscovery(discovery subagents.Discovery) SubagentsDiscovery {
	result := SubagentsDiscovery{
		Profiles: make([]SubagentProfile, len(discovery.Profiles)),
		Warnings: append([]string(nil), discovery.Warnings...),
	}
	for i, profile := range discovery.Profiles {
		result.Profiles[i] = toSubagentProfile(profile)
	}
	return result
}

func FindSkill(skills []Skill, name string) (Skill, bool) {
	for _, skill := range skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return Skill{}, false
}

func ActivationMessage(skill Skill, args []string) (string, error) {
	return skills.ActivationMessage(skills.Skill{
		Name:        skill.Name,
		Description: skill.Description,
		Location:    skill.Location,
	}, args)
}

func SaveSkillsConfig(cfg SkillsConfig) error {
	return skills.SaveConfig(fromSkillsConfig(cfg))
}

func LoadSkillsConfig() SkillsConfig {
	return toSkillsConfig(skills.LoadConfig())
}

func ToToolDiffLines(lines []EditDiffLine) []tools.EditDiffLine {
	result := make([]tools.EditDiffLine, len(lines))
	for i, line := range lines {
		result[i] = tools.EditDiffLine{
			Kind:       tools.EditDiffLineKind(line.Kind),
			OldLineNum: line.OldLineNum,
			NewLineNum: line.NewLineNum,
			Content:    line.Content,
		}
	}
	return result
}

func FromToolDiffLines(lines []tools.EditDiffLine) []EditDiffLine {
	result := make([]EditDiffLine, len(lines))
	for i, line := range lines {
		result[i] = EditDiffLine{
			Kind:       EditDiffLineKind(line.Kind),
			OldLineNum: line.OldLineNum,
			NewLineNum: line.NewLineNum,
			Content:    line.Content,
		}
	}
	return result
}

func toAskUserRequest(req tools.AskUserRequest) AskUserRequest {
	questions := make([]AskUserQuestion, len(req.Questions))
	for i, q := range req.Questions {
		questions[i] = AskUserQuestion{Question: q.Question, Options: append([]string(nil), q.Options...)}
	}
	return AskUserRequest{Questions: questions}
}

func fromAskUserRequest(req AskUserRequest) tools.AskUserRequest {
	questions := make([]tools.AskUserQuestion, len(req.Questions))
	for i, q := range req.Questions {
		questions[i] = tools.AskUserQuestion{Question: q.Question, Options: append([]string(nil), q.Options...)}
	}
	return tools.AskUserRequest{Questions: questions}
}

func toAskUserResult(res tools.AskUserResult) AskUserResult {
	return AskUserResult{
		Answers:   append([]string(nil), res.Answers...),
		Cancelled: res.Cancelled,
	}
}

func fromAskUserResult(res AskUserResult) tools.AskUserResult {
	return tools.AskUserResult{
		Answers:   append([]string(nil), res.Answers...),
		Cancelled: res.Cancelled,
	}
}

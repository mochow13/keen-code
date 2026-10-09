package repl

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/charmbracelet/x/ansi"
	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/decision/tasks/taskcomplexity"
	"github.com/mochow13/keen-code/internal/providers"
	"reflect"
	"strings"
	"testing"
)

func routerTestModel(t *testing.T) replModel {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ctx.loader = config.NewLoader()
	m.ctx.globalCfg = config.DefaultGlobalConfig()
	m.ctx.globalCfg.Providers[config.ProviderOpenAI] = config.ProviderConfig{APIKey: "test-key"}
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{{ID: config.ProviderOpenAI, Models: []providers.Model{{ID: "gpt-5.4"}}}}}
	choice := config.RouterModel{Provider: config.ProviderOpenAI, Model: "gpt-5.4"}
	m.ctx.globalCfg.Router = &config.RouterConfig{Enabled: true, Simple: choice, Standard: choice, Complex: choice}
	m.ctx.globalCfg.Decision = &config.DecisionConfig{ActiveProvider: "typesafe", ActiveModel: "classifier"}
	m.classifiers = &classificationManager{complexity: &taskcomplexity.Classifier{}}
	m.agentCore.(*mockAgentCore).ready = true
	return m
}

func completeRouterCategory(t *testing.T, m replModel) replModel {
	t.Helper()
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	m.modelSelection, _ = m.modelSelection.Update(enter)
	var cmd tea.Cmd
	m.modelSelection, cmd = m.modelSelection.Update(enter)
	if cmd == nil {
		t.Fatal("expected completion")
	}
	updated, _, handled := m.consumeRouterSelection(cmd())
	if !handled {
		t.Fatal("router selection completion not handled")
	}
	return updated
}

func TestRouterSelectionSavesOnlyAfterAllCategories(t *testing.T) {
	m := routerTestModel(t)
	previous := *m.ctx.globalCfg.Router
	m.router.draft = &config.RouterConfig{}
	m.router.activate = true
	m.startRouterSelection()
	for i := 0; i < 3; i++ {
		m = completeRouterCategory(t, m)
		if i < 2 {
			if !reflect.DeepEqual(*m.ctx.globalCfg.Router, previous) || m.ctx.loader.Exists() {
				t.Fatal("saved before all categories completed")
			}
		}
	}
	saved, err := m.ctx.loader.Load()
	if err != nil || !saved.Router.Configured() || !saved.Router.Enabled {
		t.Fatalf("saved router: %#v, err %v", saved, err)
	}
	if m.modelSelection != nil || m.router.draft != nil {
		t.Fatal("setup not cleared")
	}
}

func TestRouterSelectionCancelPreservesConfiguration(t *testing.T) {
	m := routerTestModel(t)
	previous := *m.ctx.globalCfg.Router
	draft := previous
	m.router.draft = &draft
	m.startRouterSelection()
	m = completeRouterCategory(t, m)
	_, cmd := m.modelSelection.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _, _ = m.consumeRouterSelection(cmd())
	if !reflect.DeepEqual(*m.ctx.globalCfg.Router, previous) || m.ctx.loader.Exists() {
		t.Fatal("cancel changed configuration")
	}
}

func TestRouterClassifiesBeforeSubmittingAndRoutes(t *testing.T) {
	for _, category := range []string{config.RouterCategorySimple, config.RouterCategoryStandard, config.RouterCategoryComplex, "failure"} {
		t.Run(category, func(t *testing.T) {
			m := routerTestModel(t)
			model := "gpt-5.4"
			m.ctx.globalCfg.Router.Simple.Model = "gpt-5.4-mini"
			m.ctx.globalCfg.Router.Complex.Model = "gpt-5.4-pro"
			m, cmd := m.beginRouterPrompt("hello", false)
			if cmd == nil || !m.router.pending || len(m.agentCore.GetMessages()) != 0 {
				t.Fatalf("submitted before classification: cmd=%v pending=%v messages=%d output=%q", cmd != nil, m.router.pending, len(m.agentCore.GetMessages()), m.output.Join())
			}
			classification := routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.Category(category)}}
			if category == "failure" {
				classification.err = errors.New("timeout")
			}
			if category == config.RouterCategorySimple {
				model = "gpt-5.4-mini"
			}
			if category == config.RouterCategoryComplex {
				model = "gpt-5.4-pro"
			}
			m, _ = m.handleRouterResult(routerResultMsg{generation: m.router.generation, input: "hello", classification: classification})
			if m.ctx.cfg.Model != model {
				t.Fatalf("model %s want %s; output %s", m.ctx.cfg.Model, model, m.output.Join())
			}
			if !strings.Contains(m.output.Join(), config.ProviderOpenAI+"/"+model) {
				t.Fatalf("classification output missing selected provider/model: %s", m.output.Join())
			}
			if category == "failure" && (strings.Contains(m.output.Join(), "timeout") || !strings.Contains(m.output.Join(), "Task classification unavailable")) {
				t.Fatalf("expected safe fallback message without classifier error: %s", m.output.Join())
			}
			if len(m.agentCore.GetMessages()) != 1 || m.router.pending {
				t.Fatal("routed prompt not submitted exactly once")
			}
			if m.inputMetaModel() != "typesafe/classifier (router)" {
				t.Fatal(m.inputMetaModel())
			}
		})
	}

}
func TestRouterRecordsAssistantResponseForNextClassification(t *testing.T) {
	m := routerTestModel(t)
	m.classifiers.RecordUserMessage("first request")
	m.stream.handler.Start(make(chan agentcore.StreamEvent), "Working...")
	m.stream.handler.HandleChunk("first response")
	m, _ = m.handleLLMDone()
	if got := m.classifiers.complexityExchange; len(got) != 2 || got[1].Role != "assistant" || got[1].Content != "first response" {
		t.Fatalf("classification exchange = %#v", got)
	}
}

func TestRouterConfigValidationRejectsUnresolvableModel(t *testing.T) {
	m := routerTestModel(t)
	m.ctx.globalCfg.Providers[config.ProviderOpenAI] = config.ProviderConfig{}
	err := m.validateRouterConfig(m.ctx.globalCfg.Router)
	if err == nil || !strings.Contains(err.Error(), config.RouterCategorySimple) {
		t.Fatalf("validation error = %v, want simple category error", err)
	}
}

func TestRouterIgnoresCancelledResult(t *testing.T) {
	m := routerTestModel(t)
	m, _ = m.beginRouterPrompt("hello", false)
	generation := m.router.generation
	m, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	m, _ = m.handleRouterResult(routerResultMsg{generation: generation, input: "hello"})
	if len(m.agentCore.GetMessages()) != 0 || m.router.pending {
		t.Fatal("cancelled routing submitted a prompt")
	}
}

func TestRouterSuggestionsAndOldCommandRemoved(t *testing.T) {
	m := routerTestModel(t)
	m.refreshSuggestions("/model router")
	if item := m.suggestion.Current(); item == nil || item.Value != "/model router" {
		t.Fatal("missing router suggestion")
	}
	m, _, _ = m.dispatchCommand("/model auto")
	if !strings.Contains(m.output.Join(), "Usage: /model") {
		t.Fatal("old auto command should be rejected")
	}
}

func TestSelectingRegularModelDisablesRouter(t *testing.T) {
	m := routerTestModel(t)
	m = m.startModelSelection()
	_, cmd, err := m.modelSelection.Select(config.ProviderOpenAI, "gpt-5.4")
	if err != nil {
		t.Fatal(err)
	}
	if cmd != nil {
		t.Fatal("expected provider configuration confirmation")
	}
	_, cmd = m.modelSelection.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected model selection completion")
	}
	m, _, _ = m.consumeModelSelectionResult(cmd())
	if m.routerEnabled() {
		t.Fatal("regular model left router enabled")
	}
	saved, err := m.ctx.loader.Load()
	if err != nil || saved.Router.Enabled {
		t.Fatal("router disable not persisted")
	}
}

func TestClassifierHistoryRetainedAcrossRouterActivation(t *testing.T) {
	m := routerTestModel(t)
	m.ctx.globalCfg.Router.Enabled = false
	m, _ = m.submitPrompt("regular request", false, false)
	m.stream.handler.HandleChunk("regular response")
	m, _ = m.handleLLMDone()
	m.ctx.globalCfg.Router.Enabled = true
	m, _ = m.beginRouterPrompt("routed request", false)
	if got := m.classifiers.complexityExchange; len(got) != 3 || got[0].Content != "regular request" || got[1].Role != "assistant" || got[1].Content != "regular response" || got[2].Content != "routed request" {
		t.Fatalf("classification exchange = %#v", got)
	}
	m, _ = m.handleRouterResult(routerResultMsg{generation: m.router.generation, input: "routed request", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.Category(config.RouterCategoryStandard)}}})
	if len(m.classifiers.complexityExchange) != 3 {
		t.Fatal("routed request was recorded twice")
	}
	m.stream.handler.HandleChunk("routed response")
	m, _ = m.handleLLMDone()
	if got := m.classifiers.complexityExchange; len(got) != 4 || got[3].Content != "routed response" {
		t.Fatalf("classification exchange = %#v", got)
	}
}

func TestShowRouterDescriptiveConfiguration(t *testing.T) {
	m := routerTestModel(t)
	m = m.showRouter()
	output := ansi.Strip(m.output.Join())
	for _, want := range []string{"Automatic model routing powered by decision models", "Status:", "enabled", "Classifier: typesafe/classifier", "Simple tasks", "Standard tasks", "Complex tasks", "classification fallback", "openai • gpt-5.4 • default (no override)", "Use /model router to activate routing."} {
		if !strings.Contains(output, want) {
			t.Errorf("router output missing %q: %s", want, output)
		}
	}
	for _, unwanted := range []string{"Provider:", "Thinking effort:", "/router config", "disable routing"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("router output contains unwanted text %q", unwanted)
		}
	}
}

func TestRouterReusesClientWhenResolvedConfigUnchanged(t *testing.T) {
	m := routerTestModel(t)
	resolved, err := config.ResolveProvider(m.ctx.globalCfg, config.ProviderOpenAI, "gpt-5.4", "")
	if err != nil {
		t.Fatal(err)
	}
	m.ctx.cfg = resolved
	m.agentCore.SetConfig(resolved)
	backend := m.agentCore.(*mockAgentCore)
	for _, category := range []string{config.RouterCategorySimple, config.RouterCategoryStandard} {
		m.router.pending = true
		m, _ = m.handleRouterResult(routerResultMsg{input: "continue", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.Category(category)}}})
	}
	if backend.updateCount != 0 {
		t.Fatalf("unchanged routing recreated client %d times", backend.updateCount)
	}
}

func TestRouterReinitializesClientWhenConfigurationChanges(t *testing.T) {
	for _, change := range []string{"provider", "model", "thinking", "credentials", "headers", "not ready"} {
		t.Run(change, func(t *testing.T) {
			m := routerTestModel(t)
			resolved, err := config.ResolveProvider(m.ctx.globalCfg, config.ProviderOpenAI, "gpt-5.4", "")
			if err != nil {
				t.Fatal(err)
			}
			m.ctx.cfg = resolved
			m.agentCore.SetConfig(resolved)
			backend := m.agentCore.(*mockAgentCore)
			switch change {
			case "provider":
				m.ctx.cfg.Provider = "previous-provider"
			case "model":
				m.ctx.globalCfg.Router.Standard.Model = "gpt-5.4-mini"
			case "thinking":
				m.ctx.globalCfg.Router.Standard.ThinkingEffort = "high"
			case "credentials":
				m.ctx.globalCfg.Providers[config.ProviderOpenAI] = config.ProviderConfig{APIKey: "new-test-key"}
			case "headers":
				m.ctx.globalCfg.Providers[config.ProviderOpenAI] = config.ProviderConfig{APIKey: "test-key", Headers: map[string]string{"X-Test": "value"}}
			case "not ready":
				backend.ready = false
			}
			m.router.pending = true
			m, _ = m.handleRouterResult(routerResultMsg{input: "hello", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.CategoryStandard}}})
			if backend.updateCount != 1 {
				t.Fatalf("changed %s: client updates = %d, want 1; output: %s", change, backend.updateCount, m.output.Join())
			}
		})
	}
}

func TestRouterPreservesIncompleteTurnOnModelSwitch(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(map[bool]string{false: "incomplete", true: "interrupted"}[interrupted], func(t *testing.T) {
			m := routerTestModel(t)
			resolved, err := config.ResolveProvider(m.ctx.globalCfg, config.ProviderOpenAI, "gpt-5.4", "")
			if err != nil {
				t.Fatal(err)
			}
			m.ctx.cfg = resolved
			m.agentCore.SetConfig(resolved)
			m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "first request"})
			m.stream.handler.Start(make(chan agentcore.StreamEvent), "Working...")
			m.stream.handler.HandleChunk("partial response")
			m.startAssistantTurnMemory()
			m.stream.handler.HandleToolEnd(&agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "file.go"}, Output: "file contents"})
			if interrupted {
				m.interruptStream(interruptedPromptText)
			} else {
				m, _ = m.handleLLMIncomplete(nil)
			}
			if len(m.agentCore.GetMessages()) != 1 {
				t.Fatal("incomplete turn should remain in client pending state until switching")
			}
			m.ctx.globalCfg.Router.Standard.Model = "gpt-5.4-mini"
			m.router.pending = true
			m, _ = m.handleRouterResult(routerResultMsg{input: "continue", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.CategoryStandard}}})
			messages := m.agentCore.GetMessages()
			if len(messages) != 3 || messages[1].Content != "partial response" || messages[1].TurnMemory == nil || len(messages[1].TurnMemory.ToolActivity) != 1 || messages[2].Content != "continue" {
				t.Fatalf("incomplete turn lost on switch: %#v", messages)
			}
			if len(m.router.pendingTurns) != 0 {
				t.Fatal("restored pending turns not cleared")
			}
		})
	}
}

func TestThinkingCommandExplainsRouterSettingsWithoutChangingConfig(t *testing.T) {
	m := routerTestModel(t)
	m.ctx.cfg.ThinkingEffort = "low"
	m.ctx.globalCfg.ThinkingEffort = "medium"
	before := *m.ctx.globalCfg.Router
	m, cmd := m.handleThinkingCommand("/thinking high")
	if cmd != nil || m.ctx.cfg.ThinkingEffort != "low" || m.ctx.globalCfg.ThinkingEffort != "medium" || !reflect.DeepEqual(before, *m.ctx.globalCfg.Router) || m.ctx.loader.Exists() || m.agentCore.(*mockAgentCore).updateCount != 0 {
		t.Fatal("thinking command changed router configuration or client")
	}
	output := ansi.Strip(m.output.Join())
	for _, want := range []string{"Thinking effort cannot be set while routing is enabled", "each router category has its own thinking setting", "/router config"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing router thinking explanation %q: %s", want, output)
		}
	}
}

func TestRouterDoesNotPersistClassifierFailures(t *testing.T) {
	m := routerTestModel(t)
	m.sessions = newReplSessionState(t.TempDir())
	m.router.pending = true
	m, _ = m.handleRouterResult(routerResultMsg{input: "hello", classification: routerClassification{err: errors.New("sensitive classifier response")}})
	summaries, err := m.sessions.listSessions()
	if err != nil || len(summaries) != 1 {
		t.Fatalf("summaries: %v, error: %v", summaries, err)
	}
	loaded, err := m.sessions.load(summaries[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 2 || loaded.Events[1].UserMessage.Content != "hello" {
		t.Fatalf("failure persisted classification metadata: %#v", loaded.Events)
	}
	if strings.Contains(m.output.Join(), "sensitive classifier response") {
		t.Fatal("classifier error rendered")
	}
}

func TestRouterRestoresMultiplePendingTurnsInConversationOrder(t *testing.T) {
	m := routerTestModel(t)
	m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "first request"})
	m.rememberPendingRouterTurn(agentcore.Message{Role: agentcore.RoleAssistant, Content: "first partial"})
	m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "second request"})
	m.rememberPendingRouterTurn(agentcore.Message{Role: agentcore.RoleAssistant, Content: "second partial"})
	m.rememberPendingRouterTurn(agentcore.Message{Role: agentcore.RoleAssistant, Content: "third partial"})
	if err := m.updateLLMClient(); err != nil {
		t.Fatal(err)
	}
	var contents []string
	for _, message := range m.agentCore.GetMessages() {
		contents = append(contents, message.Content)
	}
	want := []string{"first request", "first partial", "second request", "second partial", "third partial"}
	if !reflect.DeepEqual(contents, want) {
		t.Fatalf("restored conversation: %v, want %v", contents, want)
	}
	if err := m.updateLLMClient(); err != nil {
		t.Fatal(err)
	}
	if len(m.agentCore.GetMessages()) != len(want) {
		t.Fatal("pending turns restored twice")
	}
}

func TestRouterPendingTurnsClearedWithNewSession(t *testing.T) {
	m := routerTestModel(t)
	m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "old request"})
	m.rememberPendingRouterTurn(agentcore.Message{Role: agentcore.RoleAssistant, Content: "old partial"})
	m = m.handleClearCommand()
	if err := m.updateLLMClient(); err != nil {
		t.Fatal(err)
	}
	if len(m.router.pendingTurns) != 0 || len(m.agentCore.GetMessages()) != 0 {
		t.Fatal("old pending turn leaked into new session")
	}
}

func TestRouterKeepsNativePendingStateUntilTurnCompletes(t *testing.T) {
	m := routerTestModel(t)
	resolved, err := config.ResolveProvider(m.ctx.globalCfg, config.ProviderOpenAI, "gpt-5.4", "")
	if err != nil {
		t.Fatal(err)
	}
	m.ctx.cfg = resolved
	m.agentCore.SetConfig(resolved)
	m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "first request"})
	m.rememberPendingRouterTurn(agentcore.Message{Role: agentcore.RoleAssistant, Content: "partial response"})
	m.router.pending = true
	m, _ = m.handleRouterResult(routerResultMsg{input: "continue", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.CategoryStandard}}})
	if m.agentCore.(*mockAgentCore).updateCount != 0 || len(m.agentCore.GetMessages()) != 2 || len(m.router.pendingTurns) != 1 {
		t.Fatal("unchanged routing promoted or discarded native pending state")
	}
	m.stream.handler.HandleChunk("completed response")
	m, _ = m.handleLLMDone()
	if len(m.router.pendingTurns) != 0 {
		t.Fatal("completed turn retained stale pending state")
	}
	if err := m.updateLLMClient(); err != nil {
		t.Fatal(err)
	}
	if len(m.agentCore.GetMessages()) != 3 {
		t.Fatal("completed turn replayed stale pending state on client replacement")
	}
}

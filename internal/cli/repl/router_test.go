package repl

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/charmbracelet/x/ansi"
	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/decision/tasks/taskcomplexity"
	"github.com/mochow13/keen-code/internal/providers"
	"github.com/mochow13/keen-code/internal/session"
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

func TestRouterInputMetaHidesUnselectedModel(t *testing.T) {
	m := routerTestModel(t)
	m.ctx.cfg = &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "gpt-4.1"}

	if got, want := m.inputMetaModel(), "typesafe/classifier (router)"; got != want {
		t.Fatalf("inputMetaModel() = %q, want %q", got, want)
	}
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
			classification := routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.Category(category), Probability: 0.8, Confidence: 0.9, Consequential: 0.2, NewTask: 0.85}}
			if category == "failure" {
				classification.err = errors.New("timeout")
				current, err := config.ResolveProvider(m.ctx.globalCfg, config.ProviderOpenAI, "gpt-5.4-mini", "low")
				if err != nil {
					t.Fatal(err)
				}
				m.ctx.cfg = current
				m.agentCore.SetConfig(current)
				model = "gpt-5.4-mini"
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
			if category != "failure" && !strings.Contains(m.output.Join(), "new task (p=0.85)") {
				t.Fatalf("classification output missing new-task probability: %s", m.output.Join())
			}
			if category == "failure" && (strings.Contains(m.output.Join(), "timeout") || !strings.Contains(m.output.Join(), "Task classification unavailable")) {
				t.Fatalf("expected safe fallback message without classifier error: %s", m.output.Join())
			}
			if strings.Contains(m.output.Join(), "model:") || strings.Contains(m.output.Join(), config.ProviderOpenAI+"/"+model) {
				t.Fatalf("classification output contains selected model: %s", m.output.Join())
			}
			if len(m.agentCore.GetMessages()) != 1 || m.router.pending {
				t.Fatal("routed prompt not submitted exactly once")
			}
			if got, want := m.inputMetaModel(), "typesafe/classifier (router) → "+config.ProviderOpenAI+"/"+model; got != want {
				t.Fatalf("inputMetaModel() = %q, want %q", got, want)
			}
		})
	}

}
func TestTaskComplexityMessagesRebuildsFromAgentConversation(t *testing.T) {
	got := taskComplexityMessages([]agentcore.Message{
		{Role: agentcore.RoleSystem, Content: "system prompt"},
		{Role: agentcore.RoleUser, Content: "old task"},
		{Role: agentcore.RoleAssistant, Content: "old response"},
		{Role: agentcore.RoleAssistant, Content: ""},
	})
	if len(got) != 3 || got[0].Role != "user" || got[0].Content != "old task" || got[1].Role != "assistant" || got[1].Content != "old response" || got[2].Role != "assistant" {
		t.Fatalf("task complexity messages = %#v", got)
	}
}

func TestTaskComplexityInputInterleavesPendingTurnsInOrder(t *testing.T) {
	m := routerTestModel(t)
	m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "first request"})
	m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleSystem, Content: "system prompt"})
	m.rememberPendingRouterTurn(agentcore.Message{Role: agentcore.RoleAssistant, Content: "first partial"})
	m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "second request"})
	m.rememberPendingRouterTurn(agentcore.Message{Role: agentcore.RoleAssistant, Content: "second partial"})
	m.rememberPendingRouterTurn(agentcore.Message{Role: agentcore.RoleAssistant, Content: "third partial"})

	var contents []string
	for _, message := range m.taskComplexityInput("next request") {
		contents = append(contents, message.Content)
	}
	want := []string{"first request", "first partial", "second request", "second partial", "third partial", "next request"}
	if !reflect.DeepEqual(contents, want) {
		t.Fatalf("classifier input = %v, want %v", contents, want)
	}
	if len(m.agentCore.GetMessages()) != 3 {
		t.Fatalf("classifier input mutated the conversation: %#v", m.agentCore.GetMessages())
	}
}

func TestRouterRebuildsClassifierStateFromResumedSession(t *testing.T) {
	m := routerTestModel(t)
	loaded := &session.LoadedSession{Events: []session.Event{
		{Kind: session.KindUserMessage, UserMessage: &session.MessagePayload{Content: "resume this task"}},
		{Kind: session.KindAssistantTurn, AssistantTurn: &session.AssistantTurnPayload{Message: "I will continue."}},
	}}
	m.replayLoadedSession(loaded)
	m, _ = m.beginRouterPrompt("continue the same task", false)
	messages := m.taskComplexityInput("continue the same task")
	if len(messages) != 3 || messages[0].Content != "resume this task" || messages[1].Content != "I will continue." || messages[2].Content != "continue the same task" {
		t.Fatalf("rebuilt classifier history = %#v", messages)
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

func TestClassifierStateUsesAgentConversationAcrossRouterActivation(t *testing.T) {
	m := routerTestModel(t)
	m.ctx.globalCfg.Router.Enabled = false
	m, _ = m.submitPrompt("regular request", false, false)
	m.stream.handler.HandleChunk("regular response")
	m, _ = m.handleLLMDone()
	m.ctx.globalCfg.Router.Enabled = true
	m, _ = m.beginRouterPrompt("routed request", false)
	if got := taskComplexityMessages(m.agentCore.GetMessages()); len(got) != 2 || got[0].Content != "regular request" || got[1].Content != "regular response" {
		t.Fatalf("task complexity messages = %#v", got)
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
		m, _ = m.handleRouterResult(routerResultMsg{input: "continue", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.Category(category), Probability: 0.9, NewTask: 1}}})
	}
	if backend.updateCount != 0 {
		t.Fatalf("unchanged routing recreated client %d times", backend.updateCount)
	}
}

func TestRouterSwitchHeuristicBands(t *testing.T) {
	tierModels := map[string]string{
		config.RouterCategorySimple:   "gpt-5.4-mini",
		config.RouterCategoryStandard: "gpt-5.4",
		config.RouterCategoryComplex:  "gpt-5.4-pro",
	}
	for _, tc := range []struct {
		name        string
		category    string
		probability float64
		newTask     float64
		active      string
		switchModel bool
	}{
		{name: "low category probability blocks new task", category: config.RouterCategoryComplex, probability: 0.5, newTask: 0.9, active: config.RouterCategorySimple},
		{name: "low category probability blocks escalation", category: config.RouterCategoryComplex, probability: 0.49, newTask: 0.1, active: config.RouterCategorySimple},
		{name: "new task switches up", category: config.RouterCategoryComplex, probability: 0.6, newTask: 0.6, active: config.RouterCategorySimple, switchModel: true},
		{name: "new task switches down", category: config.RouterCategorySimple, probability: 0.6, newTask: 0.6, active: config.RouterCategoryComplex, switchModel: true},
		{name: "continuation keeps model", category: config.RouterCategoryComplex, probability: 0.6, newTask: 0.1, active: config.RouterCategorySimple},
		{name: "continuation below new task boundary keeps model", category: config.RouterCategoryComplex, probability: 0.6, newTask: 0.5999, active: config.RouterCategorySimple},
		{name: "confident escalation switches", category: config.RouterCategoryComplex, probability: 0.9, newTask: 0.1, active: config.RouterCategoryStandard, switchModel: true},
		{name: "confident escalation at boundary keeps model", category: config.RouterCategoryComplex, probability: 0.8, newTask: 0.1, active: config.RouterCategoryStandard},
		{name: "confident downgrade keeps model", category: config.RouterCategorySimple, probability: 0.9, newTask: 0.1, active: config.RouterCategoryComplex},
		{name: "confident same tier keeps model", category: config.RouterCategoryStandard, probability: 0.9, newTask: 0.1, active: config.RouterCategoryStandard},
		{name: "confident escalation from manual model switches", category: config.RouterCategoryComplex, probability: 0.9, newTask: 0.1, switchModel: true},
		{name: "low-confidence classification from manual model switches", category: config.RouterCategoryComplex, probability: 0.49, newTask: 0.1, switchModel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := routerTestModel(t)
			m.ctx.globalCfg.Router.Simple = config.RouterModel{Provider: config.ProviderOpenAI, Model: tierModels[config.RouterCategorySimple]}
			m.ctx.globalCfg.Router.Standard = config.RouterModel{Provider: config.ProviderOpenAI, Model: tierModels[config.RouterCategoryStandard]}
			m.ctx.globalCfg.Router.Complex = config.RouterModel{Provider: config.ProviderOpenAI, Model: tierModels[config.RouterCategoryComplex]}
			activeModel := "manual-model"
			if tc.active != "" {
				activeModel = tierModels[tc.active]
			}
			current, err := config.ResolveProvider(m.ctx.globalCfg, config.ProviderOpenAI, activeModel, "")
			if err != nil {
				t.Fatal(err)
			}
			m.ctx.cfg = current
			m.agentCore.SetConfig(current)
			backend := m.agentCore.(*mockAgentCore)
			m.router.pending = true
			m, _ = m.handleRouterResult(routerResultMsg{input: "request", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.Category(tc.category), Probability: tc.probability, NewTask: tc.newTask}}})
			if tc.switchModel {
				if m.ctx.cfg.Model != tierModels[tc.category] || backend.updateCount != 1 {
					t.Fatalf("routing did not switch to %s model: model=%s updates=%d", tc.category, m.ctx.cfg.Model, backend.updateCount)
				}
			} else if m.ctx.cfg != current || backend.cfg != current || backend.updateCount != 0 {
				t.Fatalf("routing changed active configuration or recreated client: model=%s updates=%d", m.ctx.cfg.Model, backend.updateCount)
			}
			wantStatus := "not switching"
			if tc.switchModel {
				wantStatus = "switching"
			}
			if !strings.Contains(strings.Join(strings.Fields(ansi.Strip(m.output.Join())), " "), "· "+wantStatus) {
				t.Fatalf("classification output missing %q status: %s", wantStatus, m.output.Join())
			}
			if len(m.agentCore.GetMessages()) != 1 || m.router.pending {
				t.Fatal("prompt not submitted exactly once")
			}
		})
	}
}

func TestRouterClassificationFailureKeepsActiveModel(t *testing.T) {
	m := routerTestModel(t)
	current, err := config.ResolveProvider(m.ctx.globalCfg, config.ProviderOpenAI, "gpt-5.4-mini", "low")
	if err != nil {
		t.Fatal(err)
	}
	m.ctx.cfg = current
	m.agentCore.SetConfig(current)
	m.router.pending = true
	m, _ = m.handleRouterResult(routerResultMsg{input: "request", classification: routerClassification{err: errors.New("timeout")}})
	if m.ctx.cfg != current || m.agentCore.(*mockAgentCore).updateCount != 0 {
		t.Fatalf("classification failure changed the active model: model=%s updates=%d", m.ctx.cfg.Model, m.agentCore.(*mockAgentCore).updateCount)
	}
	if len(m.agentCore.GetMessages()) != 1 || m.router.pending {
		t.Fatal("classification failure did not submit the prompt")
	}
	if !strings.Contains(m.output.Join(), "Using the current model") {
		t.Fatalf("classification failure did not print the current-model notice: %s", m.output.Join())
	}
	if !strings.Contains(m.output.Join(), "not switching") {
		t.Fatalf("classification failure did not indicate no model switch: %s", m.output.Join())
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
			m, _ = m.handleRouterResult(routerResultMsg{input: "hello", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.CategoryStandard, Probability: 0.9, NewTask: 1}}})
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
			m.taskHint.skipSession = true
			m.router.pending = true
			m, _ = m.handleRouterResult(routerResultMsg{input: "continue", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.CategoryStandard, Probability: 0.9, NewTask: 1}}})
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
	output := strings.Join(strings.Fields(ansi.Strip(m.output.Join())), " ")
	for _, want := range []string{"Thinking effort cannot be set while routing is enabled", "each router category has its own thinking setting", "/router config"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing router thinking explanation %q: %s", want, output)
		}
	}
}

func TestRouterDoesNotPersistClassifierFailures(t *testing.T) {
	m := routerTestModel(t)
	m.sessions = newTestSession(t.TempDir())
	m.router.pending = true
	m, _ = m.handleRouterResult(routerResultMsg{input: "hello", classification: routerClassification{err: errors.New("sensitive classifier response")}})
	summaries, err := m.sessions.List()
	if err != nil || len(summaries) != 1 {
		t.Fatalf("summaries: %v, error: %v", summaries, err)
	}
	loaded, err := m.sessions.Load(summaries[0])
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

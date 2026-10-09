package repl

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mochow13/keen-code/internal/agentcore"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/decision/tasks/taskcomplexity"
)

type routerState struct {
	draft        *config.RouterConfig
	step         int
	activate     bool
	pending      bool
	generation   uint64
	cancel       context.CancelFunc
	pendingTurns []pendingRouterTurn
}

type pendingRouterTurn struct {
	message      agentcore.Message
	messageCount int
}

type routerClassification struct {
	result   taskcomplexity.Result
	err      error
	provider string
}

type routerResultMsg struct {
	generation     uint64
	input          string
	classification routerClassification
}

var routerCategories = []string{config.RouterCategorySimple, config.RouterCategoryStandard, config.RouterCategoryComplex}

func (m replModel) routerEnabled() bool {
	return m.ctx != nil && m.ctx.globalCfg != nil && m.ctx.globalCfg.Router != nil && m.ctx.globalCfg.Router.Enabled
}

func (m *replModel) enableRouter(reconfigure bool) replModel {
	if m.ctx == nil || m.ctx.globalCfg == nil || m.ctx.loader == nil {
		m.output.AddError("Router configuration is unavailable.", repltheme.ErrorStyle)
	} else if err := m.classifiers.Configure(m.ctx.globalCfg); err != nil {
		m.output.AddStyledLine("  Configure decision model with `/decision model` command to use router", repltheme.UsageHintStyle)
	} else if !reconfigure && m.ctx.globalCfg.Router.Configured() {
		if err := m.validateRouterConfig(m.ctx.globalCfg.Router); err != nil {
			m.output.AddError(err.Error(), repltheme.ErrorStyle)
		} else {
			previous := *m.ctx.globalCfg.Router
			m.ctx.globalCfg.Router.Enabled = true
			if err := m.ctx.loader.Save(m.ctx.globalCfg); err != nil {
				*m.ctx.globalCfg.Router = previous
				m.output.AddError("Failed to save router configuration.", repltheme.ErrorStyle)
			} else {
				m.output.AddStyledLine("  Router enabled", repltheme.HighlightStyle)
			}
		}
	} else {
		draft := &config.RouterConfig{}
		if m.ctx.globalCfg.Router != nil {
			*draft = *m.ctx.globalCfg.Router
		}
		m.router.draft, m.router.step = draft, 0
		m.router.activate = !reconfigure || draft.Enabled
		m.startRouterSelection()
	}
	if m.modelSelection == nil {
		m.output.AddEmptyLine()
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) startRouterSelection() {
	category := routerCategories[m.router.step]
	existing := m.router.draft.ModelForCategory(category)
	m.modelSelection = replwidgets.NewRouterSelection(m.ctx.registry, m.ctx.globalCfg, category, existing, func(selected config.RouterModel) error { return nil })
}

func (m *replModel) consumeRouterSelection(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if m.router.draft == nil || m.modelSelection == nil {
		return *m, nil, false
	}
	if replwidgets.IsCancel(msg) {
		m.router.draft = nil
		m.modelSelection = nil
		m.output.AddStyledLine("  Router configuration cancelled.", repltheme.UsageHintStyle)
		m.output.AddStyledLine("  Previous configuration unchanged.", repltheme.UsageHintStyle)
		m.output.AddEmptyLine()
	} else if replwidgets.IsComplete(msg) {
		selection := m.modelSelection
		selected := config.RouterModel{Provider: selection.SelectedProvider, Model: selection.SelectedModel, ThinkingEffort: selection.SelectedThinking}
		if meta, ok := m.ctx.registry.GetModel(selected.Provider, selected.Model); !ok || !meta.SupportsThinkingEffort() {
			selected.ThinkingEffort = ""
		}
		switch m.router.step {
		case 0:
			m.router.draft.Simple = selected
		case 1:
			m.router.draft.Standard = selected
		case 2:
			m.router.draft.Complex = selected
		}
		m.router.step++
		if m.router.step < len(routerCategories) {
			m.startRouterSelection()
		} else {
			if err := m.validateRouterConfig(m.router.draft); err != nil {
				m.router.step = 2
				m.modelSelection.ErrorMessage = err.Error()
				m.updateViewportContent()
				m.viewport.GotoBottom()
				return *m, nil, true
			}
			previous := m.ctx.globalCfg.Router
			m.router.draft.Enabled = m.router.activate
			m.ctx.globalCfg.Router = m.router.draft
			if err := m.ctx.loader.Save(m.ctx.globalCfg); err != nil {
				m.ctx.globalCfg.Router = previous
				m.router.step = 2
				m.modelSelection.ErrorMessage = "Failed to save router configuration.\nPress Enter to retry or Esc to cancel."
			} else {
				m.router.draft = nil
				m.modelSelection = nil
				m.output.AddStyledLine("  Router configuration saved", repltheme.HighlightStyle)
				m.output.AddEmptyLine()
			}
		}
	} else {
		return *m, nil, false
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m, nil, true
}

func (m *replModel) showRouter() replModel {
	r := m.ctx.globalCfg.Router
	if !r.Configured() {
		m.output.AddStyledLine("  No router configuration.", repltheme.UsageHintStyle)
		m.output.AddStyledLine("  Use /model router to configure and enable it.", repltheme.UsageHintStyle)
	} else {
		m.output.AddStyledLine("  Automatic model routing powered by decision models", repltheme.PrimaryBoldStyle)
		status := "disabled — using the selected model directly"
		if r.Enabled {
			status = repltheme.HighlightStyle.Render("enabled") + " — selecting a model for each prompt"
		}
		m.output.AddStyledLine("  Status: "+status, repltheme.AssistantStyle)
		if decision := m.ctx.globalCfg.Decision; decision != nil && decision.ActiveProvider != "" && decision.ActiveModel != "" {
			m.output.AddStyledLine("  Classifier: "+repltheme.HighlightStyle.Render(decision.ActiveProvider+"/"+decision.ActiveModel), repltheme.AssistantStyle)
		} else {
			m.output.AddStyledLine("  Classifier not configured. Use /decision model.", repltheme.UsageHintStyle)
		}
		for _, category := range routerCategories {
			model := r.ModelForCategory(category)
			style := repltheme.PrimaryBoldStyle
			description := "Standard tasks — everyday coding and debugging; classification fallback"
			switch category {
			case config.RouterCategorySimple:
				style = style.Foreground(repltheme.SecondaryColor)
				description = "Simple tasks — quick questions and small edits"
			case config.RouterCategoryComplex:
				style = style.Foreground(repltheme.AccentColor)
				description = "Complex tasks — deep reasoning and architectural changes"
			}
			m.output.AddEmptyLine()
			m.output.AddStyledLine("  "+description, repltheme.AssistantStyle)
			effort := model.ThinkingEffort
			if effort == "" {
				effort = "default (no override)"
			}
			m.output.AddStyledLine(fmt.Sprintf("    %s • %s • %s", model.Provider, model.Model, effort), style)
		}
		m.output.AddEmptyLine()
		m.output.AddStyledLine("  Use "+repltheme.PrimaryBoldStyle.Render("/model router")+" to activate routing.", repltheme.AssistantStyle)
	}
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) validateRouterConfig(router *config.RouterConfig) error {
	for _, category := range routerCategories {
		model := router.ModelForCategory(category)
		if _, err := config.ResolveProvider(m.ctx.globalCfg, model.Provider, model.Model, model.ThinkingEffort); err != nil {
			return fmt.Errorf("Cannot use %s model: %w", category, err)
		}
	}
	return nil
}

func (m *replModel) beginRouterPrompt(input string, fromQueue bool) (replModel, tea.Cmd) {
	if m.classifiers == nil {
		m.classifiers = &classificationManager{}
	}
	if m.classifiers.complexity == nil {
		if err := m.classifiers.Configure(m.ctx.globalCfg); err != nil {
			m.output.AddStyledLine("  Configure decision model with `/decision model` command to use router", repltheme.UsageHintStyle)
			m.output.AddEmptyLine()
			m.textarea.Reset()
			m.updateViewportContent()
			return *m, nil
		}
	}
	if !m.ctx.globalCfg.Router.Configured() {
		m.output.AddError("Router models are not configured. Use /router config.", repltheme.ErrorStyle)
		m.output.AddEmptyLine()
		m.textarea.Reset()
		m.updateViewportContent()
		return *m, nil
	}
	m.classifiers.RecordUserMessage(input)
	m.router.generation++
	generation := m.router.generation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	m.router.cancel, m.router.pending = cancel, true
	// Snapshot the exchange before the command runs on Bubble Tea's command goroutine.
	manager := *m.classifiers
	manager.complexityExchange = append(manager.complexityExchange[:0:0], m.classifiers.complexityExchange...)
	m.startLoading("Classifying task...")
	if !fromQueue {
		m.textarea.Reset()
	}
	m.adjustTextareaHeight()
	m.updateViewportContent()
	cwd, branch := m.ctx.workingDir, m.gitBranch
	return *m, tea.Batch(m.loading.spinner.Tick, func() tea.Msg {
		defer cancel()
		result, provider, err := manager.ClassifyTask(ctx, cwd, branch)
		return routerResultMsg{generation: generation, input: input, classification: routerClassification{result: result, provider: provider, err: err}}
	})
}

func (m *replModel) handleRouterResult(msg routerResultMsg) (replModel, tea.Cmd) {
	if !m.router.pending || m.router.generation != msg.generation {
		return *m, nil
	}
	m.router.pending, m.router.cancel = false, nil
	classification := msg.classification
	category := config.RouterCategoryStandard
	if classification.err == nil {
		category = string(classification.result.Category)
		m.recordUsage(classification.provider, classification.result.Model, &agentcore.TokenUsage{InputTokens: classification.result.Usage.InputTokens, OutputTokens: classification.result.Usage.OutputTokens})
	}
	selected := m.ctx.globalCfg.Router.ModelForCategory(category)
	if classification.err != nil {
		m.output.AddStyledLine(fmt.Sprintf("  Task classification unavailable. Using standard model: %s/%s", selected.Provider, selected.Model), repltheme.UsageHintStyle)
		m.output.AddEmptyLine()
	} else {
		m.output.AddStyledLine(fmt.Sprintf("  task: %s (p=%.2f) · consequential (p=%.2f) · model: %s/%s", category, classification.result.Probability, classification.result.Consequential, selected.Provider, selected.Model), repltheme.FaintedStyle)
		m.output.AddEmptyLine()
	}
	resolved, err := config.ResolveProvider(m.ctx.globalCfg, selected.Provider, selected.Model, selected.ThinkingEffort)
	if err == nil && (!m.agentCore.IsReady() || !reflect.DeepEqual(m.ctx.cfg, resolved)) {
		previous := m.ctx.cfg
		m.ctx.cfg = resolved
		err = m.updateLLMClient()
		if err != nil {
			m.ctx.cfg = previous
			m.agentCore.SetConfig(previous)
		}
	}
	if err != nil {
		m.stopLoading()
		m.output.AddError("Could not initialize routed model. Check /router config and provider authentication.", repltheme.ErrorStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		return m.drainQueuedInput()
	}
	m.refreshContextStatus()
	return m.submitPrompt(msg.input, true, true)
}

func (m *replModel) rememberPendingRouterTurn(message agentcore.Message) {
	m.router.pendingTurns = append(m.router.pendingTurns, pendingRouterTurn{message: message, messageCount: len(m.agentCore.GetMessages())})
}

func (m *replModel) restorePendingRouterTurns() {
	if len(m.router.pendingTurns) == 0 {
		return
	}
	messages := m.agentCore.GetMessages()
	for _, turn := range slices.Backward(m.router.pendingTurns) {
		index := min(turn.messageCount, len(messages))
		messages = append(messages, agentcore.Message{})
		copy(messages[index+1:], messages[index:])
		messages[index] = turn.message
	}
	m.agentCore.ReplaceMessages(messages)
	m.router.pendingTurns = nil
}

package repl

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/llm"
	"github.com/user/keen-code/providers"
)

const (
	exitCommand  = "/exit"
	helpCommand  = "/help"
	modelCommand = "/model"

	defaultWidth = 120
	maxHeight    = 3
)

var loadingTexts = []string{
	"Cogitating...",
	"Schemulating...",
	"Logicrafting...",
	"Factweaving...",
	"Signalparsing...",
	"Mindmapping...",
	"Codeforging...",
	"Tasksplicing...",
	"Datasifting...",
	"Pathfinding...",
	"Threadspinning...",
	"Plansmithing...",
	"Synthesizing...",
	"Constraintcrunching...",
	"Flowtuning...",
	"Resultshaping...",
	"Contextfolding...",
	"Syntaxstitching...",
	"Hypothesishammering...",
	"Signaldistilling...",
	"Threadaligning...",
	"Contextlathing...",
	"Promptcalibrating...",
	"Intentdecoding...",
	"Plancompiling...",
	"Tokenjuggling...",
	"Edgecasemapping...",
	"Inferencepolishing...",
	"Tracemining...",
	"Branchsculpting...",
	"Outcomeforging...",
	"Latencytrimming...",
	"Modelwhispering...",
	"Heuristicbraiding...",
	"Difforbiting...",
	"Semanticssanding...",
	"Constraintweaving...",
	"Decisionlinting...",
	"Querytempering...",
	"Contextbuffering...",
	"Pathuntangling...",
	"Resulthoning...",
}

var loadingSpinners = []spinner.Spinner{
	spinner.Line,
	spinner.Dot,
	spinner.MiniDot,
	spinner.Jump,
	spinner.Pulse,
	spinner.Points,
	spinner.Meter,
	spinner.Hamburger,
	spinner.Globe,
	spinner.Moon,
	spinner.Monkey,
}

func nextLoadingText() string {
	return loadingTexts[rand.Intn(len(loadingTexts))]
}

func nextLoadingSpinner() spinner.Spinner {
	return loadingSpinners[rand.Intn(len(loadingSpinners))]
}

type replContext struct {
	version    string
	workingDir string
	cfg        *config.ResolvedConfig
	globalCfg  *config.GlobalConfig
	loader     *config.Loader
	registry   *providers.Registry
}

type replModel struct {
	textarea            textarea.Model
	viewport            viewport.Model
	ctx                 *replContext
	appState            *AppState
	output              *OutputBuilder
	modelSelection      *Model
	permissionRequester *REPLPermissionRequester
	diffEmitter         *REPLDiffEmitter
	quitting            bool
	streamHandler       *StreamHandler
	mdRenderer          *MarkdownRenderer
	width               int
	height              int
	spinner             spinner.Model
	showSpinner         bool
	loadingText         string
	userScrolled        bool
	streamCancel        context.CancelFunc
}

func abbreviateHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func initialModel(ctx *replContext, llmClient llm.LLMClient, needsSetup bool) replModel {
	ta := textarea.New()
	ta.Placeholder = "What are we building?"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(defaultWidth - 3)
	ta.SetHeight(maxHeight)
	ta.MaxHeight = 0
	ta.ShowLineNumbers = false
	ta.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return "> "
		}
		return "  "
	})

	styles := ta.Styles()
	styles.Focused.Prompt = promptStyle
	styles.Focused.Text = lipgloss.NewStyle()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.Prompt = promptStyle
	styles.Blurred.Text = lipgloss.NewStyle()
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(styles)

	ta.KeyMap.InsertNewline.SetKeys("ctrl+enter")
	ta.KeyMap.InsertNewline.SetEnabled(true)

	s := spinner.New()
	s.Spinner = spinner.Pulse
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)

	initialOutput := buildInitialScreen(ctx)
	appState := NewAppState(llmClient, ctx.workingDir)

	permissionRequester := NewREPLPermissionRequester()
	diffEmitter := NewREPLDiffEmitter()
	setupToolRegistry(ctx.workingDir, appState, permissionRequester, diffEmitter)

	mdRenderer, err := NewMarkdownRenderer(defaultWidth)

	if err != nil {
		mdRenderer = nil
	}

	vp := viewport.New(viewport.WithWidth(defaultWidth), viewport.WithHeight(24))
	vp.SetContent(strings.Join(initialOutput, "\n"))

	model := replModel{
		textarea:            ta,
		viewport:            vp,
		ctx:                 ctx,
		appState:            appState,
		output:              NewOutputBuilder(defaultWidth),
		spinner:             s,
		streamHandler:       NewStreamHandler(mdRenderer),
		mdRenderer:          mdRenderer,
		permissionRequester: permissionRequester,
		diffEmitter:         diffEmitter,
	}

	if needsSetup {
		welcomeStyle := lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
		model.output.AddEmptyLine()
		model.output.AddStyledLine(welcomeStyle.Render("👋 Welcome to Keen!"), lipgloss.NewStyle())
		model.output.AddEmptyLine()
		model.output.AddEmptyLine()
		model = model.startModelSelection()
	}

	return model
}

func buildInitialScreen(ctx *replContext) []string {
	var lines []string

	asciiArt := []string{
		"░█░█░█▀▀░█▀▀░█▀█░░░█▀▀░█▀█░█▀▄░█▀▀",
		"░█▀▄░█▀▀░█▀▀░█░█░░░█░░░█░█░█░█░█▀▀",
		"░▀░▀░▀▀▀░▀▀▀░▀░▀░░░▀▀▀░▀▀▀░▀▀░░▀▀▀",
	}

	colors := []string{
		"#00F2FE", "#05E5FE", "#10D3FE", "#1ABFFE", "#25ACFE", "#4FACFE", "#6696FE", "#7C3AED",
	}

	lines = append(lines, "")
	for i, line := range asciiArt {
		color := colors[i%len(colors)]
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(line))
	}

	lines = append(lines, "")
	lines = append(lines, "  "+titleStyle.Render("Keen v"+ctx.version))
	lines = append(lines, "")

	displayDir := abbreviateHome(ctx.workingDir)
	lines = append(lines, "  "+infoLabelStyle.Render("Directory:")+" "+infoValueStyle.Render(displayDir))
	lines = append(lines, "  "+infoLabelStyle.Render("Provider:")+" "+highlightStyle.Render(ctx.cfg.Provider))
	lines = append(lines, "  "+infoLabelStyle.Render("Model:")+" "+infoValueStyle.Render(ctx.cfg.Model))
	lines = append(lines, "")

	tips := []string{
		"Type /help  for available commands",
		"Type /exit  to quit",
		"Type /model to change provider or model",
		"Press Enter to send, Ctrl+J for new line",
		"Shift+click to select and copy text",
	}
	tipsBox := boxStyle.Render(tipStyle.Render(strings.Join(tips, "\n")))
	lines = append(lines, tipsBox)
	lines = append(lines, "")

	return lines
}

func (m replModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m *replModel) spinnerHeight() int {
	if m.showSpinner && m.streamHandler != nil && m.streamHandler.IsActive() {
		return 1
	}
	return 0
}

func (m *replModel) adjustTextareaHeight() {
	if m.height <= 0 {
		return
	}
	m.textarea.SetHeight(maxHeight)
	m.viewport.SetHeight(m.height - m.textarea.Height() - 4 - m.spinnerHeight())
}

func (m replModel) isAtTopOfInput() bool {
	return m.textarea.Line() == 0
}

func (m replModel) isAtBottomOfInput() bool {
	return m.textarea.Line() >= m.textarea.LineCount()-1
}

func (m *replModel) startModelSelection() replModel {
	onComplete := func(provider, model, apiKey string) error {
		return m.updateLLMClient()
	}
	m.modelSelection = New(
		m.ctx.registry,
		m.ctx.globalCfg,
		m.ctx.loader,
		m.ctx.cfg,
		onComplete,
	)
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) handleEnterKey() (replModel, tea.Cmd) {
	input := m.textarea.Value()
	if input == "" {
		return *m, nil
	}

	if m.streamHandler.IsActive() {
		return *m, nil
	}

	m.output.AddUserInput(input, promptStyle)

	if input == exitCommand {
		m.quitting = true
		return *m, tea.Quit
	}

	if input == helpCommand {
		m.output.AddLine(getHelpText())
		m.output.AddEmptyLine()
		m.textarea.Reset()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if input == modelCommand {
		m.textarea.Reset()
		return m.startModelSelection(), nil
	}

	if !m.appState.IsClientReady(m.ctx.cfg) {
		m.output.AddError("LLM client not initialized. Use /model to configure.", errorStyle)
		m.textarea.Reset()
		return *m, nil
	}

	m.appState.AddMessage(llm.RoleUser, input)

	ctx := m.startStreamContext()
	eventCh, err := m.appState.StreamChat(ctx, m.ctx.cfg)
	if err != nil {
		m.clearStreamCancel()
		m.output.AddError(err.Error(), errorStyle)
		m.textarea.Reset()
		return *m, nil
	}

	m.showSpinner = true
	m.spinner.Spinner = nextLoadingSpinner()
	m.loadingText = nextLoadingText()
	m.streamHandler.Start(eventCh, m.loadingText)
	m.textarea.Reset()
	m.userScrolled = false
	m.adjustTextareaHeight()
	m.updateViewportContent()
	m.viewport.GotoBottom()

	return *m, tea.Batch(m.spinner.Tick, m.waitForAsyncEvent())
}

func (m *replModel) startStreamContext() context.Context {
	if m.streamCancel != nil {
		m.streamCancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.streamCancel = cancel
	return ctx
}

func (m *replModel) clearStreamCancel() {
	m.streamCancel = nil
}

func (m *replModel) updateViewportContent() {
	if m.viewport.Width() == 0 {
		return
	}

	contentWidth := m.width
	if contentWidth <= 0 {
		contentWidth = m.viewport.Width()
	}

	var content strings.Builder

	if m.output != nil && !m.output.IsEmpty() {
		content.WriteString(m.output.Join())
	}

	if m.streamHandler != nil && m.streamHandler.IsActive() {
		content.WriteString(m.streamHandler.View(contentWidth))
	}

	if m.modelSelection != nil {
		content.WriteString(formatModelSelectionCard(m.modelSelection))
	}

	m.viewport.SetContent(content.String())
}

func (m replModel) waitForAsyncEvent() tea.Cmd {
	if m.streamHandler == nil || !m.streamHandler.IsActive() || m.streamHandler.eventCh == nil {
		return nil
	}
	var permissionCh <-chan *PermissionRequest
	if m.permissionRequester != nil {
		permissionCh = m.permissionRequester.GetRequestChan()
	}
	var diffCh <-chan diffEmitRequest
	if m.diffEmitter != nil {
		diffCh = m.diffEmitter.GetDiffChan()
	}
	return waitForAsyncEvent(
		m.streamHandler.eventCh,
		permissionCh,
		diffCh,
	)
}

func waitForAsyncEvent(llmCh <-chan llm.StreamEvent, permissionCh <-chan *PermissionRequest, diffCh <-chan diffEmitRequest) tea.Cmd {
	if llmCh == nil {
		return nil
	}

	return func() tea.Msg {
		select {
		case req := <-permissionCh:
			return permissionReadyMsg{req: req}
		case req := <-diffCh:
			return diffReadyMsg{req: req}
		case event, ok := <-llmCh:
			if !ok {
				return llmDoneMsg{}
			}

			switch event.Type {
			case llm.StreamEventTypeChunk:
				return llmChunkMsg(event.Content)
			case llm.StreamEventTypeReasoningChunk:
				return llmReasoningChunkMsg(event.Content)
			case llm.StreamEventTypeDone:
				return llmDoneMsg{}
			case llm.StreamEventTypeError:
				return llmErrorMsg{err: event.Error}
			case llm.StreamEventTypeToolStart:
				return llmToolStartMsg{toolCall: event.ToolCall}
			case llm.StreamEventTypeToolEnd:
				return llmToolEndMsg{toolCall: event.ToolCall}
			default:
				return llmDoneMsg{}
			}
		}
	}
}

func formatModelSelectionCard(ms *Model) string {
	boxed := userPromptCardStyle.Render(ms.ViewString())
	lines := strings.Split(strings.TrimRight(boxed, "\n"), "\n")
	var sb strings.Builder
	sb.WriteString("\n")
	for _, l := range lines {
		sb.WriteString("  " + l + "\n")
	}
	return sb.String()
}

func (m *replModel) applyWindowSize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	m.textarea.SetWidth(msg.Width - 3)
	if m.mdRenderer != nil {
		m.mdRenderer.UpdateWidth(msg.Width)
	}
	m.viewport.SetWidth(msg.Width)
	m.viewport.SetHeight(msg.Height - m.textarea.Height() - 4 - m.spinnerHeight())
}

func (m replModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.updateNormalMode(msg)
	return &updatedModel, cmd
}

func (m replModel) updateNormalMode(msg tea.Msg) (replModel, tea.Cmd) {
	if updated, cmd, handled := m.handleLLMStreamMsg(msg); handled {
		return updated, cmd
	}

	if updated, cmd, handled := m.consumeModelSelectionResult(msg); handled {
		return updated, cmd
	}

	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.applyWindowSize(sizeMsg)
		if m.modelSelection != nil {
			m.updateViewportContent()
		}
		return m, nil
	}

	if m.modelSelection != nil {
		return m.handleKeyMsg(msg)
	}

	switch msg := msg.(type) {
	case diffReadyMsg:
		m.streamHandler.HandleDiff(msg.req.lines)
		close(msg.req.done)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, m.waitForAsyncEvent()

	case permissionReadyMsg:
		m.streamHandler.HandlePermissionRequest(msg.req)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, m.waitForAsyncEvent()

	case spinner.TickMsg:
		if updated, cmd, handled := m.handleSpinnerTick(msg); handled {
			return updated, cmd
		}

	case tea.WindowSizeMsg:
		m.applyWindowSize(msg)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.viewport.ScrollUp(3)
			m.userScrolled = !m.viewport.AtBottom()
		case tea.MouseWheelDown:
			m.viewport.ScrollDown(3)
			m.userScrolled = !m.viewport.AtBottom()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.adjustTextareaHeight()
	return m, cmd
}

func (m replModel) consumeModelSelectionResult(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if m.modelSelection == nil {
		return m, nil, false
	}

	if IsComplete(msg) {
		successMsg := "✓ Updated to " + m.modelSelection.SelectedProvider + " / " + m.modelSelection.SelectedModel
		m.output.AddStyledLine("  "+successMsg, highlightStyle)
		m.output.AddEmptyLine()
		m.modelSelection = nil
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil, true
	}

	if IsCancel(msg) {
		cancelStyle := lipgloss.NewStyle().Foreground(mutedColor)
		m.output.AddStyledLine("  Model selection cancelled", cancelStyle)
		m.output.AddEmptyLine()
		m.modelSelection = nil
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil, true
	}

	return m, nil, false
}

func (m replModel) handleSpinnerTick(msg spinner.TickMsg) (replModel, tea.Cmd, bool) {
	if !m.showSpinner {
		return m, nil, false
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	m.updateViewportContent()
	return m, cmd, true
}

func (m replModel) View() tea.View {
	var content string

	if m.quitting {
		content = lipgloss.NewStyle().Foreground(mutedColor).Render("\n  Goodbye!\n")
	} else {
		var view strings.Builder

		view.WriteString(m.viewport.View())
		view.WriteString("\n")

		if m.showSpinner && m.streamHandler != nil && m.streamHandler.IsActive() {
			spinnerText := m.spinner.View() + " " + loadingTextStyled.Render(m.loadingText)
			padding := m.width - lipgloss.Width(spinnerText) - 1
			if padding < 0 {
				padding = 0
			}
			view.WriteString(strings.Repeat(" ", padding) + spinnerText)
			view.WriteString("\n")
		}

		view.WriteString(inputBorderStyle.Render(m.textarea.View()))
		view.WriteString("\n")
		view.WriteString(m.inputMetaView())

		content = view.String()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m replModel) inputMetaView() string {
	provider := "-"
	model := "-"

	if m.ctx != nil && m.ctx.cfg != nil {
		if m.ctx.cfg.Provider != "" {
			provider = m.ctx.cfg.Provider
		}
		if m.ctx.cfg.Model != "" {
			model = m.ctx.cfg.Model
		}
	}

	metaLabelStyle := lipgloss.NewStyle().Foreground(mutedColor)
	providerText := metaLabelStyle.Render("Provider:") + " " + highlightStyle.Render(provider)
	modelText := metaLabelStyle.Render("Model:") + " " + infoValueStyle.Render(model)

	return "  " + providerText + "   " + modelText
}

func getHelpText() string {
	cmds := []struct{ cmd, desc string }{
		{"/help", "Show available commands"},
		{"/model", "Change provider or model"},
		{"/exit", "Quit Keen"},
	}

	var lines []string
	lines = append(lines, titleStyle.Render("Available Commands"))
	lines = append(lines, "")
	for _, c := range cmds {
		lines = append(lines, "  "+helpCmdStyle.Render(c.cmd)+" "+helpDescStyle.Render(c.desc))
	}

	return strings.Join(lines, "\n")
}

func (m *replModel) updateLLMClient() error {
	client, err := llm.NewClient(m.ctx.cfg)
	if err != nil {
		return err
	}
	m.appState.UpdateClient(client)
	return nil
}

func RunREPL(
	version string,
	workingDir string,
	cfg *config.ResolvedConfig,
	loader *config.Loader,
	globalCfg *config.GlobalConfig,
	registry *providers.Registry,
	needsSetup bool,
) error {
	ctx := &replContext{
		version:    version,
		workingDir: workingDir,
		cfg:        cfg,
		globalCfg:  globalCfg,
		loader:     loader,
		registry:   registry,
	}

	var llmClient llm.LLMClient
	if cfg.APIKey != "" && cfg.Model != "" {
		client, err := llm.NewClient(cfg)
		if err != nil {
			return fmt.Errorf("failed to initialize LLM client: %w", err)
		}
		llmClient = client
	}

	m := initialModel(ctx, llmClient, needsSetup)
	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}

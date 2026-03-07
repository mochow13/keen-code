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
	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/cli/modelselection"
	"github.com/user/keen-cli/internal/config"
	"github.com/user/keen-cli/internal/llm"
)

const (
	/* Commands */
	exitCommand  = "/exit"
	helpCommand  = "/help"
	modelCommand = "/model"

	/* UI */
	defaultWidth = 120
	maxHeight    = 3
)

var loadingTexts = []string{
	"Cooking...",
	"Building...",
	"Brewing...",
	"Figuring out...",
	"Getting answers...",
	"Composing...",
	"Finding out...",
	"Answering...",
	"Hmmm...",
	"Let me check...",
	"Let me see...",
	"Let me find out...",
	"Sizzling...",
	"Whisking...",
	"Stirring...",
	"Whizzing...",
	"Beep boop...",
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
	modelSelection      *modelselection.Model
	permissionSelector  *PermissionSelector
	permissionRequester *REPLPermissionRequester
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
	appState := NewAppState(llmClient)

	permissionRequester := NewREPLPermissionRequester()
	setupToolRegistry(ctx.workingDir, appState, permissionRequester)

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
		"██╗  ██╗███████╗███████╗███╗   ██╗     ██████╗ ██████╗ ██████╗ ███████╗",
		"██║ ██╔╝██╔════╝██╔════╝████╗  ██║    ██╔════╝██╔═══██╗██╔══██╗██╔════╝",
		"█████╔╝ █████╗  █████╗  ██╔██╗ ██║    ██║     ██║   ██║██║  ██║█████╗  ",
		"██╔═██╗ ██╔══╝  ██╔══╝  ██║╚██╗██║    ██║     ██║   ██║██║  ██║██╔══╝  ",
		"██║  ██╗███████╗███████╗██║ ╚████║    ╚██████╗╚██████╔╝██████╔╝███████╗",
		"╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═══╝     ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝",
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

func (m *replModel) adjustTextareaHeight() {
	m.textarea.SetHeight(maxHeight)
	if m.height > 0 {
		m.viewport.SetHeight(m.height - m.textarea.Height() - 4)
	}
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
	m.modelSelection = modelselection.New(
		m.ctx.registry,
		m.ctx.globalCfg,
		m.ctx.loader,
		m.ctx.cfg,
		onComplete,
	)
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
	m.loadingText = loadingTexts[rand.Intn(len(loadingTexts))]
	m.streamHandler.Start(eventCh, m.loadingText)
	m.textarea.Reset()
	m.userScrolled = false
	m.updateViewportContent()
	m.viewport.GotoBottom()

	return *m, tea.Batch(m.spinner.Tick, m.streamHandler.WaitForEvent())
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

	var content strings.Builder

	if m.output != nil && !m.output.IsEmpty() {
		content.WriteString(m.output.Join())
	}

	if m.streamHandler != nil && m.streamHandler.IsActive() {
		content.WriteString(m.streamHandler.View(m.width, m.showSpinner, m.spinner.View()))
	}

	m.viewport.SetContent(content.String())
}

func (m replModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.permissionSelector != nil {
		updatedModel, cmd := m.updatePermissionMode(msg)
		return &updatedModel, cmd
	}

	if m.modelSelection != nil {
		updated, cmd := m.handleKeyMsg(msg)
		return &updated, cmd
	}

	updatedModel, cmd := m.updateNormalMode(msg)
	return &updatedModel, cmd
}

func (m replModel) updatePermissionMode(msg tea.Msg) (replModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg, permissionKeyEnter, permissionKeyCancel:
		return m.handlePermissionKeyMsg(msg)
	case spinner.TickMsg:
		updated, cmd, handled := m.handleSpinnerTick(msg)
		if handled {
			return updated, cmd
		}
		return m, nil
	default:
		if updated, cmd, handled := m.handleLLMStreamMsg(msg); handled {
			if m.showSpinner {
				return updated, tea.Batch(cmd, m.spinner.Tick)
			}
			return updated, cmd
		}
		return m, nil
	}
}

func (m replModel) updateNormalMode(msg tea.Msg) (replModel, tea.Cmd) {
	if updated, cmd, handled := m.handleLLMStreamMsg(msg); handled {
		return updated, cmd
	}

	if updated, cmd, handled := m.consumePermissionRequest(msg); handled {
		return updated, cmd
	}

	switch msg := msg.(type) {
	case spinner.TickMsg:
		if updated, cmd, handled := m.handleSpinnerTick(msg); handled {
			return updated, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width - 3)
		if m.mdRenderer != nil {
			m.mdRenderer.UpdateWidth(msg.Width)
		}
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - m.textarea.Height() - 4)
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

func (m replModel) consumePermissionRequest(msg tea.Msg) (replModel, tea.Cmd, bool) {
	switch msg.(type) {
	case llmChunkMsg, llmDoneMsg, llmErrorMsg:
		return m, nil, false
	}

	select {
	case req := <-m.permissionRequester.GetRequestChan():
		var cmd tea.Cmd
		if tickMsg, ok := msg.(spinner.TickMsg); ok && m.showSpinner {
			m.spinner, cmd = m.spinner.Update(tickMsg)
		}
		m.permissionSelector = NewPermissionSelector(req.ToolName, req.Path, req.ResolvedPath, req.Operation)
		return m, cmd, true
	default:
		return m, nil, false
	}
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

func (m replModel) handleLLMStreamMsg(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if m.streamHandler == nil || !m.streamHandler.IsActive() {
		switch msg.(type) {
		case llmChunkMsg, llmDoneMsg, llmErrorMsg, llmToolStartMsg, llmToolEndMsg:
			return m, nil, true
		}
	}

	switch msg := msg.(type) {
	case llmChunkMsg:
		updated, cmd := m.handleLLMChunk(string(msg))
		return updated, cmd, true
	case llmDoneMsg:
		updated, cmd := m.handleLLMDone()
		return updated, cmd, true
	case llmErrorMsg:
		updated, cmd := m.handleLLMError(msg.err)
		return updated, cmd, true
	case llmToolStartMsg:
		updated, cmd := m.handleToolStart(msg.toolCall)
		return updated, cmd, true
	case llmToolEndMsg:
		updated, cmd := m.handleToolEnd(msg.toolCall)
		return updated, cmd, true
	default:
		return m, nil, false
	}
}

func (m replModel) View() tea.View {
	var content string

	if m.quitting {
		content = lipgloss.NewStyle().Foreground(mutedColor).Render("\n  Goodbye!\n")
	} else if m.permissionSelector != nil {
		content = m.permissionSelector.ViewString()
	} else if m.modelSelection != nil {
		content = m.modelSelection.ViewString()
	} else {
		var view strings.Builder

		view.WriteString(m.viewport.View())
		view.WriteString("\n")

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

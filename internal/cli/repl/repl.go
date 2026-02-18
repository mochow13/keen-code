package repl

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	defaultWidth  = 120
	maxHeight     = 20
	initialHeight = 1
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
	textarea       textarea.Model
	viewport       viewport.Model
	ctx            *replContext
	appState       *AppState
	output         *OutputBuilder
	modelSelection *modelselection.Model
	quitting       bool
	streamHandler  *StreamHandler
	mdRenderer     *MarkdownRenderer
	width          int
	height         int
	spinner        spinner.Model
	showSpinner    bool
	loadingText    string
	userScrolled   bool
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
	ta.Placeholder = "Type your message..."
	ta.Focus()
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetWidth(defaultWidth)
	ta.SetHeight(initialHeight)
	ta.MaxHeight = maxHeight
	ta.ShowLineNumbers = false

	ta.KeyMap.InsertNewline.SetKeys("ctrl+j")
	ta.KeyMap.InsertNewline.SetEnabled(true)

	s := spinner.New()
	s.Spinner = spinner.Pulse
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)

	initialOutput := buildInitialScreen(ctx)
	appState := NewAppState(llmClient)
	mdRenderer, err := NewMarkdownRenderer(defaultWidth)

	if err != nil {
		mdRenderer = nil
	}

	vp := viewport.New(defaultWidth, 24)
	vp.SetContent(strings.Join(initialOutput, "\n"))

	model := replModel{
		textarea:      ta,
		viewport:      vp,
		ctx:           ctx,
		appState:      appState,
		output:        NewOutputBuilder(defaultWidth),
		spinner:       s,
		streamHandler: NewStreamHandler(mdRenderer),
		mdRenderer:    mdRenderer,
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
	lines = append(lines, "  "+titleStyle.Render("Keen v"+ctx.version)+"  "+modeStyle.Render("plan mode"))
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
	currentValue := m.textarea.Value()
	lineCount := strings.Count(currentValue, "\n") + 1
	if lineCount > maxHeight {
		lineCount = maxHeight
	}
	m.textarea.SetHeight(lineCount)
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
		m.textarea.SetHeight(1)
		return m.startModelSelection(), nil
	}

	if !m.appState.IsClientReady(m.ctx.cfg) {
		m.output.AddError("LLM client not initialized. Use /model to configure.", errorStyle)
		m.textarea.Reset()
		m.textarea.SetHeight(1)
		return *m, nil
	}

	m.appState.AddMessage(llm.RoleUser, input)

	ctx := context.Background()
	eventCh, err := m.appState.StreamChat(ctx, m.ctx.cfg)
	if err != nil {
		m.output.AddError(err.Error(), errorStyle)
		m.textarea.Reset()
		m.textarea.SetHeight(1)
		return *m, nil
	}

	m.showSpinner = true
	m.loadingText = loadingTexts[rand.Intn(len(loadingTexts))]
	m.streamHandler.Start(eventCh, m.loadingText)
	m.textarea.Reset()
	m.textarea.SetHeight(1)
	m.userScrolled = false
	m.updateViewportContent()
	m.viewport.GotoBottom()

	return *m, tea.Batch(m.spinner.Tick, m.streamHandler.WaitForEvent())
}

func (m *replModel) updateViewportContent() {
	if m.viewport.Width == 0 {
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

func (m *replModel) handleCtrlJ() (replModel, tea.Cmd) {
	currentValue := m.textarea.Value()
	newValue := currentValue + "\n"
	m.textarea.SetValue(newValue)

	lineCount := strings.Count(newValue, "\n") + 1
	if lineCount > 10 {
		lineCount = 10
	}
	m.textarea.SetHeight(lineCount)
	m.textarea.CursorEnd()
	return *m, nil
}

func (m replModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.modelSelection != nil {
		return m.handleKeyMsg(msg)
	}

	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.showSpinner {
			var spinnerCmd tea.Cmd
			m.spinner, spinnerCmd = m.spinner.Update(msg)
			return m, spinnerCmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width - 3)
		if m.mdRenderer != nil {
			m.mdRenderer.UpdateWidth(msg.Width)
		}
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - m.textarea.Height() - 2
		return m, nil

	case llmChunkMsg:
		return m.handleLLMChunk(string(msg))

	case llmDoneMsg:
		return m.handleLLMDone()

	case llmErrorMsg:
		return m.handleLLMError(msg.err)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.viewport.ScrollDown(3)
		}
		return m, nil
	}

	m.textarea, cmd = m.textarea.Update(msg)
	m.adjustTextareaHeight()
	return m, cmd
}

func (m replModel) View() string {
	if m.quitting {
		return lipgloss.NewStyle().Foreground(mutedColor).Render("\n  Goodbye!\n")
	}

	if m.modelSelection != nil {
		return m.modelSelection.View()
	}

	var view strings.Builder

	view.WriteString(m.viewport.View())
	view.WriteString("\n")

	textareaView := m.textarea.View()
	lines := strings.Split(textareaView, "\n")

	view.WriteString(promptStyle.Render("> "))
	view.WriteString(inputLineStyle.Render(lines[0]))

	for i := 1; i < len(lines); i++ {
		view.WriteString("\n")
		view.WriteString(inputLineStyle.Render("  " + lines[i]))
	}

	return view.String()
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

	p := tea.NewProgram(initialModel(ctx, llmClient, needsSetup), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}

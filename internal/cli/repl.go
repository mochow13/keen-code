package cli

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/cli/modelselection"
	"github.com/user/keen-cli/internal/config"
	"github.com/user/keen-cli/internal/llm"
)

const (
	exitCommand  = "/exit"
	helpCommand  = "/help"
	modelCommand = "/model"
)

var loadingTexts = []string{
	"Cooking...",
	"Building...",
	"Brewing...",
	"Figuring out...",
	"Getting answers...",
	"Composing...",
}

type replState struct {
	version    string
	workingDir string
	cfg        *config.ResolvedConfig
	globalCfg  *config.GlobalConfig
	loader     *config.Loader
	registry   *providers.Registry
	llmClient  llm.LLMClient
	messages   []llm.Message
}

type replModel struct {
	textarea       textarea.Model
	state          *replState
	outputLines    []string
	modelSelection *modelselection.Model
	quitting       bool
	streamHandler  *StreamHandler
	width          int
	spinner        spinner.Model
	showSpinner    bool
	loadingText    string
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

func initialModel(state *replState, needsSetup bool) replModel {
	ta := textarea.New()
	ta.Placeholder = "Type your message..."
	ta.Focus()
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetWidth(120)
	ta.SetHeight(1)
	ta.MaxHeight = 10
	ta.ShowLineNumbers = false

	ta.KeyMap.InsertNewline.SetKeys("ctrl+j")
	ta.KeyMap.InsertNewline.SetEnabled(true)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(primaryColor)

	initialOutput := buildInitialScreen(state)

	model := replModel{
		textarea:      ta,
		state:         state,
		outputLines:   initialOutput,
		spinner:       s,
		streamHandler: NewStreamHandler(),
	}

	if needsSetup {
		welcomeStyle := lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
		model.outputLines = append(model.outputLines, "")
		model.outputLines = append(model.outputLines, welcomeStyle.Render("👋 Welcome to Keen!"))
		model.outputLines = append(model.outputLines, "")
		model.outputLines = append(model.outputLines, "")
		model = model.startModelSelection()
	}

	return model
}

func buildInitialScreen(state *replState) []string {
	var lines []string

	asciiArt := []string{
		"██╗  ██╗███████╗███████╗███╗   ██╗",
		"██║ ██╔╝██╔════╝██╔════╝████╗  ██║",
		"█████╔╝ █████╗  █████╗  ██╔██╗ ██║",
		"██╔═██╗ ██╔══╝  ██╔══╝  ██║╚██╗██║",
		"██║  ██╗███████╗███████╗██║ ╚████║",
		"╚═╝  ╚═╝╚══════╝╚══════╝╚═╝  ╚═══╝",
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
	lines = append(lines, "  "+titleStyle.Render("Keen v"+state.version)+"  "+modeStyle.Render("plan mode"))
	lines = append(lines, "")

	displayDir := abbreviateHome(state.workingDir)
	lines = append(lines, "  "+infoLabelStyle.Render("Directory:")+" "+infoValueStyle.Render(displayDir))
	lines = append(lines, "  "+infoLabelStyle.Render("Provider:")+" "+highlightStyle.Render(state.cfg.Provider))
	lines = append(lines, "  "+infoLabelStyle.Render("Model:")+" "+infoValueStyle.Render(state.cfg.Model))
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

func (m replModel) formatInputForDisplay(input string) string {
	inputLines := strings.Split(input, "\n")
	wrapStyle := m.contentStyle()
	var formattedInput strings.Builder
	formattedInput.WriteString(promptStyle.Render("> "))
	formattedInput.WriteString(wrapStyle.Render(inputLines[0]))
	for i := 1; i < len(inputLines); i++ {
		formattedInput.WriteString("\n  ")
		formattedInput.WriteString(wrapStyle.Render(inputLines[i]))
	}
	return formattedInput.String()
}

func (m *replModel) adjustTextareaHeight() {
	currentValue := m.textarea.Value()
	lineCount := strings.Count(currentValue, "\n") + 1
	if lineCount > 10 {
		lineCount = 10
	}
	m.textarea.SetHeight(lineCount)
}

func (m replModel) startModelSelection() replModel {
	onComplete := func(provider, model, apiKey string) error {
		return m.updateLLMClient()
	}
	m.modelSelection = modelselection.New(
		m.state.registry,
		m.state.globalCfg,
		m.state.loader,
		m.state.cfg,
		onComplete,
	)
	return m
}

func (m replModel) contentStyle() lipgloss.Style {
	return lipgloss.NewStyle().Width(m.width - 4)
}

func (m replModel) handleEnterKey() (replModel, tea.Cmd) {
	input := m.textarea.Value()
	if input == "" {
		return m, nil
	}

	if m.streamHandler.IsActive() {
		return m, nil
	}

	m.outputLines = append(m.outputLines, m.formatInputForDisplay(input))
	m.outputLines = append(m.outputLines, "")

	if input == exitCommand {
		m.quitting = true
		return m, tea.Quit
	}

	if input == helpCommand {
		m.outputLines = append(m.outputLines, getHelpText())
		m.outputLines = append(m.outputLines, "")
		m.textarea.Reset()
		return m, nil
	}

	if input == modelCommand {
		m.textarea.Reset()
		m.textarea.SetHeight(1)
		return m.startModelSelection(), nil
	}

	if m.state.llmClient == nil {
		m.outputLines = append(m.outputLines, m.contentStyle().Render(errorStyle.Render("  Error: LLM client not initialized. Use /model to configure.")))
		m.outputLines = append(m.outputLines, "")
		m.textarea.Reset()
		m.textarea.SetHeight(1)
		return m, nil
	}

	m.state.messages = append(m.state.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: input,
	})

	ctx := context.Background()
	eventCh, err := m.state.llmClient.StreamChat(ctx, m.state.messages)
	if err != nil {
		m.outputLines = append(m.outputLines, errorStyle.Render("  Error: "+err.Error()))
		m.outputLines = append(m.outputLines, "")
		m.textarea.Reset()
		m.textarea.SetHeight(1)
		return m, nil
	}

	m.showSpinner = true
	m.loadingText = loadingTexts[rand.Intn(len(loadingTexts))]
	m.streamHandler.Start(eventCh, m.loadingText)
	m.textarea.Reset()
	m.textarea.SetHeight(1)

	return m, tea.Batch(m.spinner.Tick, m.streamHandler.WaitForEvent())
}

func (m replModel) handleCtrlJ() (replModel, tea.Cmd) {
	currentValue := m.textarea.Value()
	newValue := currentValue + "\n"
	m.textarea.SetValue(newValue)

	lineCount := strings.Count(newValue, "\n") + 1
	if lineCount > 10 {
		lineCount = 10
	}
	m.textarea.SetHeight(lineCount)
	m.textarea.CursorEnd()
	return m, nil
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
		m.textarea.SetWidth(msg.Width - 3)
		return m, nil

	case llmChunkMsg:
		return m.handleLLMChunk(string(msg))

	case llmDoneMsg:
		return m.handleLLMDone()

	case llmErrorMsg:
		return m.handleLLMError(msg.err)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
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

	if len(m.outputLines) > 0 {
		view.WriteString(strings.Join(m.outputLines, "\n"))
	}

	if m.streamHandler.IsActive() {
		view.WriteString(m.streamHandler.View(m.width, m.showSpinner, m.spinner.View()))
	}

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
	client, err := llm.NewClient(m.state.cfg)
	if err != nil {
		return err
	}
	m.state.llmClient = client
	return nil
}

func RunREPL(version, workingDir string, cfg *config.ResolvedConfig, loader *config.Loader, globalCfg *config.GlobalConfig, registry *providers.Registry, needsSetup bool) error {
	state := &replState{
		version:    version,
		workingDir: workingDir,
		cfg:        cfg,
		globalCfg:  globalCfg,
		loader:     loader,
		registry:   registry,
		messages:   []llm.Message{},
	}

	if cfg.APIKey != "" && cfg.Model != "" {
		client, err := llm.NewClient(cfg)
		if err != nil {
			return fmt.Errorf("failed to initialize LLM client: %w", err)
		}
		state.llmClient = client
	}

	p := tea.NewProgram(initialModel(state, needsSetup), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}

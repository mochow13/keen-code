package repl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	replappstate "github.com/mochow13/keen-code/internal/cli/repl/appstate"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replcommands "github.com/mochow13/keen-code/internal/cli/repl/commands"
	replfilesearch "github.com/mochow13/keen-code/internal/cli/repl/filesearch"
	replhistory "github.com/mochow13/keen-code/internal/cli/repl/history"
	replmarkdown "github.com/mochow13/keen-code/internal/cli/repl/markdown"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	repltooling "github.com/mochow13/keen-code/internal/cli/repl/tooling"
	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/filesystem"
	"github.com/mochow13/keen-code/internal/llm"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/providers"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/skills"
	"github.com/mochow13/keen-code/internal/subagents"
)

const (
	defaultWidth             = 120
	defaultViewportHeight    = 24
	defaultHelpWidth         = 80
	inputMinHeight           = 1
	inputMaxHeight           = 15
	inputMaxContentHeight    = 10000
	contentHorizontalPadding = 4
	viewportChromeHeight     = 5
	statusBlockHeight        = 2
	copyNotificationTimeout  = 2 * time.Second
	copyNotificationMessage  = "Copied to clipboard"
	streamRenderInterval     = 200 * time.Millisecond
)

type replContext struct {
	version       string
	workingDir    string
	cfg           *config.ResolvedConfig
	globalCfg     *config.GlobalConfig
	loader        *config.Loader
	registry      *providers.Registry
	mcp           keenmcp.Runtime
	resumeSession *session.LoadedSession
}

type replModel struct {
	textarea            textarea.Model
	viewport            viewport.Model
	ctx                 *replContext
	mode                llm.AgentMode
	appState            *replappstate.AppState
	output              *reploutput.OutputBuilder
	modelSelection      *replwidgets.Model
	permissionRequester *replpermissions.Requester
	askUser             askUserState
	projectPerms        *config.ProjectPermissions
	diffEmitter         *repltooling.DiffEmitter
	sessions            *replSessionState
	sessionPicker       *replwidgets.SessionPicker
	suggestion          replwidgets.SuggestionModel
	fileSearcher        *replfilesearch.FileSearcher
	quitting            bool
	stream              streamState
	mdRenderer          *replmarkdown.Renderer
	width               int
	height              int
	userScrolled        bool
	turnMemory          *turnMemoryAccumulator
	compaction          compactionState
	contextStatus       contextStatus
	showThinking        bool
	toolHistory         toolHistoryMode
	history             replhistory.InputHistory
	selection           viewportSelection
	inputSelection      inputSelection
	loading             loadingState
	btw                 btwState
	bang                bangState
	adversary           adversaryState
	lastSession         *session.Summary
	projectPermsErr     error
	initialScreenDone   bool
	notification        notificationState
	queuedInputs        []string
	gitBranch           string
	subagentActivity    <-chan subagents.ToolActivity
}

type toolHistoryMode uint8

const (
	toolHistoryNone toolHistoryMode = iota
	toolHistoryFull
)

type bangState struct {
	active bool
	events <-chan tea.Msg
	cancel context.CancelFunc
}

type loadingState struct {
	spinner            spinner.Model
	showSpinner        bool
	text               string
	startedAt          time.Time
	lastTurnElapsedMsg string
}

type btwState struct {
	lines         []string
	question      string
	streamHandler *StreamHandler
	streamCancel  context.CancelFunc
	showSpinner   bool
	spinner       spinner.Model
}

type adversaryState struct {
	streamHandler  *StreamHandler
	streamCancel   context.CancelFunc
	lines          []string
	focus          string
	showSpinner    bool
	spinner        spinner.Model
	modelSelection *replwidgets.Model
}

type streamState struct {
	handler *StreamHandler
	cancel  context.CancelFunc

	// Streamed tokens arrive faster than the terminal can redraw. Batch
	// viewport rebuilds to a short window so updates are smooth and complete.
	renderPending  bool
	renderInterval time.Duration
}

type compactionMode uint8

const (
	compactionNone compactionMode = iota
	compactionManual
	compactionAutomatic
)

type compactionState struct {
	active bool
	mode   compactionMode
	cancel context.CancelFunc
}

type notificationState struct {
	text      string
	expiresAt time.Time
}

func initialModel(ctx *replContext, llmClient llm.LLMClient, needsSetup bool) replModel {
	ta := textarea.New()
	ta.Placeholder = "What are we building?"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(defaultWidth - inputPromptWidth)
	ta.DynamicHeight = true
	ta.MinHeight = inputMinHeight
	ta.MaxHeight = inputMaxHeight
	ta.MaxContentHeight = inputMaxContentHeight
	ta.SetHeight(inputMinHeight)
	ta.ShowLineNumbers = false
	ta.SetPromptFunc(3, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return " ▶ "
		}
		return "   "
	})

	styles := ta.Styles()
	styles.Focused.Prompt = repltheme.PromptStyle
	styles.Focused.Text = lipgloss.NewStyle()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.Prompt = repltheme.InputRuleBlurredStyle
	styles.Blurred.Text = lipgloss.NewStyle()
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(styles)

	ta.KeyMap.InsertNewline.SetKeys("shift+enter")
	ta.KeyMap.InsertNewline.SetEnabled(true)

	s := spinner.New()
	s.Spinner = spinner.Pulse
	s.Style = lipgloss.NewStyle().Foreground(repltheme.PrimaryColor)

	bs := spinner.New()
	bs.Spinner = spinner.MiniDot
	bs.Style = lipgloss.NewStyle().Foreground(repltheme.AccentColor)

	as := spinner.New()
	as.Spinner = spinner.MiniDot
	as.Style = lipgloss.NewStyle().Foreground(repltheme.SecondaryColor)

	appState := replappstate.New(llmClient, ctx.workingDir)

	projectPerms, projectPermsErr := config.LoadProjectPermissions(ctx.workingDir)
	if projectPermsErr != nil {
		projectPerms = config.NewProjectPermissions()
	}

	permissionRequester := replpermissions.NewRequester(projectPerms)
	diffEmitter := repltooling.NewDiffEmitter()
	askUserRequester := replaskuser.NewRequester()
	sessions := newReplSessionState(ctx.workingDir)

	fileGitAwareness := filesystem.NewGitAwareness()
	_ = fileGitAwareness.LoadGitignore(filepath.Join(ctx.workingDir, ".gitignore"))
	fileGuard := filesystem.NewGuard(ctx.workingDir, fileGitAwareness)
	fileSearcher := replfilesearch.NewFileSearcher(ctx.workingDir, fileGuard)

	subagentActivity := make(chan subagents.ToolActivity, 64)
	repltooling.SetupToolRegistry(
		ctx.workingDir,
		appState,
		permissionRequester,
		diffEmitter,
		askUserRequester,
		ctx.mcp,
		ctx.cfg,
		ctx.globalCfg,
		subagentActivity,
	)

	mdRenderer, err := replmarkdown.New(defaultWidth)

	if err != nil {
		mdRenderer = nil
	}

	output := reploutput.NewOutputBuilder(defaultWidth, ctx.workingDir)
	var lastSession *session.Summary
	if summaries, err := sessions.listSessions(); err == nil && len(summaries) > 0 {
		lastSession = &summaries[0]
	}

	vp := viewport.New(viewport.WithWidth(defaultWidth), viewport.WithHeight(defaultViewportHeight))

	model := replModel{
		textarea:            ta,
		viewport:            vp,
		ctx:                 ctx,
		mode:                llm.ModeBuild,
		appState:            appState,
		output:              output,
		loading:             loadingState{spinner: s},
		stream:              streamState{handler: NewStreamHandler(mdRenderer), renderInterval: streamRenderInterval},
		mdRenderer:          mdRenderer,
		permissionRequester: permissionRequester,
		askUser:             askUserState{requester: askUserRequester},
		projectPerms:        projectPerms,
		diffEmitter:         diffEmitter,
		sessions:            sessions,
		suggestion:          replwidgets.NewSuggestionModel(),
		fileSearcher:        fileSearcher,
		showThinking:        true,
		btw: btwState{
			spinner:       bs,
			streamHandler: NewStreamHandler(mdRenderer),
		},
		adversary: adversaryState{
			spinner:       as,
			streamHandler: NewStreamHandler(mdRenderer),
		},
		lastSession:      lastSession,
		projectPermsErr:  projectPermsErr,
		subagentActivity: subagentActivity,
	}
	if ctx.globalCfg != nil && ctx.globalCfg.ShowThinking != nil {
		model.showThinking = *ctx.globalCfg.ShowThinking
	}
	model.stream.handler.workingDir = ctx.workingDir
	model.stream.handler.showThinking = model.showThinking
	model.refreshGitBranch()

	if ctx.resumeSession != nil {
		model.sessions.setSession(ctx.resumeSession.Session)
		model.replayLoadedSession(ctx.resumeSession)
		model.initialScreenDone = true
	}

	historyDir, err := os.UserHomeDir()
	if err == nil {
		historyDir = filepath.Join(historyDir, ".keen")
		if mkdirErr := os.MkdirAll(historyDir, 0755); mkdirErr == nil {
			_ = model.history.LoadFromFile(filepath.Join(historyDir, "input-history"))
		}
	}

	model.refreshContextStatus()

	if needsSetup {
		welcomeStyle := lipgloss.NewStyle().Foreground(repltheme.PrimaryColor).Bold(true)
		model.output.AddEmptyLine()
		model.output.AddStyledLine(welcomeStyle.Render("👋 Welcome to Keen!"), lipgloss.NewStyle())
		model.output.AddEmptyLine()
		model.output.AddEmptyLine()
		model = model.startModelSelection()
	} else {
		model.updateViewportContent()
	}

	return model
}

func (m replModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, checkForUpdate(m.ctx.version), waitForMCPStartup(m.ctx.mcp))
}

func (m *replModel) handleEnterKey() (replModel, tea.Cmd) {
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return *m, nil
	}

	if input == replcommands.Adversary || strings.HasPrefix(input, replcommands.Adversary+" ") {
		if m.loading.showSpinner {
			if len(m.queuedInputs) < maxQueuedInputs {
				m.queuedInputs = append(m.queuedInputs, input)
				m.textarea.Reset()
				m.adjustTextareaHeight()
				return *m, nil
			}
			return *m, m.showNotification("Queue is full")
		}
		m.history.Push(input)
		m.textarea.Reset()
		result, cmd := m.handleAdversaryCommand(input)
		return result, cmd
	}

	if input == replcommands.Btw || strings.HasPrefix(input, replcommands.Btw+" ") {
		m.history.Push(input)
		m.textarea.Reset()
		result, cmd := m.handleBtwCommand(input)
		return result, cmd
	}

	if strings.HasPrefix(input, "!") {
		if m.loading.showSpinner || m.bang.active {
			return *m, m.showNotification("Operation not permitted")
		}
		m.history.Push(input)
		m.textarea.Reset()
		result, cmd := m.handleBangCommand(input)
		return result, cmd
	}

	if input == replcommands.EmptyQueue {
		m.textarea.Reset()
		if len(m.queuedInputs) == 0 {
			return *m, m.showNotification("Queue is empty")
		}
		m.queuedInputs = nil
		m.updateViewportContent()
		m.adjustTextareaHeight()
		return *m, m.showNotification("Queue cleared")
	}

	if m.stream.handler.IsActive() || m.compaction.active {
		if m.isQueueable(input) {
			if len(m.queuedInputs) < maxQueuedInputs {
				m.queuedInputs = append(m.queuedInputs, input)
				m.textarea.Reset()
				m.adjustTextareaHeight()
				return *m, nil
			}
			return *m, m.showNotification("Queue is full")
		}
		msg := "Operation not permitted"
		if strings.HasPrefix(input, "/") && !replcommands.IsKnownCommand(input) {
			msg = "No such skill found"
		}
		return *m, m.showNotification(msg)
	}

	return m.submitInput(input, false)
}

func (m *replModel) submitInput(input string, fromQueue bool) (replModel, tea.Cmd) {
	m.flushBtwToOutput()
	m.flushAdversaryToOutput()
	m.output.AddUserInput(input, repltheme.PromptStyle)
	m.history.Push(input)

	if updated, cmd, handled := m.dispatchCommand(input); handled {
		return updated, cmd
	}

	if activated, ok := m.activateSkillInput(input); ok {
		input = activated
	}

	if !m.appState.IsClientReady(m.ctx.cfg) {
		m.output.AddError("LLM client not initialized. Use /model to configure.", repltheme.ErrorStyle)
		if !fromQueue {
			m.textarea.Reset()
		}
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if err := m.sessions.appendUserMessage(input); err != nil {
		m.output.AddError("Session persistence failed: "+err.Error(), repltheme.ErrorStyle)
		if !fromQueue {
			m.textarea.Reset()
		}
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	m.appState.AddMessage(llm.RoleUser, input)

	ctx := m.startStreamContext()
	eventCh, err := m.appState.StreamChat(ctx, m.ctx.cfg, llm.StreamOptions{SessionID: m.sessions.currentID()})
	if err != nil {
		m.clearStreamCancel()
		m.output.AddError(err.Error(), repltheme.ErrorStyle)
		if !fromQueue {
			m.textarea.Reset()
		}
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	m.startLoading(nextLoadingText())
	m.startAssistantTurnMemory()
	m.stream.handler.Start(eventCh, m.loading.text)
	if !fromQueue {
		m.textarea.Reset()
	}
	m.userScrolled = false
	m.adjustTextareaHeight()
	m.updateViewportContent()
	m.viewport.GotoBottom()

	return *m, tea.Batch(m.loading.spinner.Tick, m.waitForAsyncEvent())
}

func (m *replModel) showNotification(msg string) tea.Cmd {
	expiresAt := time.Now().Add(copyNotificationTimeout)
	m.notification.text = msg
	m.notification.expiresAt = expiresAt
	m.adjustTextareaHeight()
	return clearCopyNotificationCmd(expiresAt)
}

func (m *replModel) activateSkillInput(input string) (string, bool) {
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	firstLine := input
	rest := ""
	if idx := strings.Index(input, "\n"); idx >= 0 {
		firstLine = input[:idx]
		rest = strings.TrimSpace(input[idx+1:])
	}
	fields := strings.Fields(strings.TrimPrefix(firstLine, "/"))
	if len(fields) == 0 {
		return "", false
	}

	skill, ok := m.appState.FindEnabledSkill(fields[0])
	if !ok {
		return "", false
	}
	args := fields[1:]
	if rest != "" {
		args = append(args, rest)
	}
	msg, err := skills.ActivationMessage(skill, args)
	if err != nil {
		return "", false
	}
	return msg, true
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

	if m.stream.handler != nil && m.stream.handler.IsActive() {
		content.WriteString(m.stream.handler.View(contentWidth))
	}

	if m.btw.streamHandler != nil && m.btw.streamHandler.IsActive() {
		content.WriteString(m.renderBtwInline(contentWidth))
	} else if m.btw.lines != nil {
		content.WriteString(m.renderBtwInlineFinished(contentWidth))
	}

	if m.adversary.streamHandler != nil && m.adversary.streamHandler.IsActive() {
		content.WriteString(m.renderAdversaryInline(contentWidth))
	} else if m.adversary.lines != nil {
		content.WriteString(m.renderAdversaryInlineFinished(contentWidth))
	}

	if m.modelSelection != nil {
		content.WriteString(formatModelSelectionCard(m.modelSelection, m.viewport.Width()))
	}

	if m.adversary.modelSelection != nil {
		content.WriteString(formatModelSelectionCard(m.adversary.modelSelection, m.viewport.Width()))
	}

	if m.sessionPicker != nil {
		content.WriteString(replwidgets.FormatSessionPickerCard(m.sessionPicker, m.viewport.Width(), m.viewport.Height()))
	}

	viewportContent := content.String()
	m.viewport.SetContent(viewportContent)
	m.selection.setContent(viewportContent)
}

func (m replModel) waitForAsyncEvent() tea.Cmd {
	if m.stream.handler == nil || !m.stream.handler.IsActive() || m.stream.handler.eventCh == nil {
		return nil
	}
	var askUserCh <-chan *replaskuser.Request
	if m.askUser.requester != nil {
		askUserCh = m.askUser.requester.GetRequestChan()
	}
	var permissionCh <-chan *replpermissions.Request
	if m.permissionRequester != nil {
		permissionCh = m.permissionRequester.GetRequestChan()
	}
	var diffCh <-chan repltooling.DiffRequest
	if m.diffEmitter != nil {
		diffCh = m.diffEmitter.GetDiffChan()
	}
	return waitForAsyncEvent(
		m.stream.handler.eventCh,
		permissionCh,
		diffCh,
		m.subagentActivity,
		askUserCh,
	)
}

func (m replModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updatedModel, cmd := m.updateNormalMode(msg)
	return &updatedModel, cmd
}

func (m replModel) updateNormalMode(msg tea.Msg) (replModel, tea.Cmd) {
	if updated, cmd, handled := m.handleBangMsg(msg); handled {
		return updated, cmd
	}

	if updated, cmd, handled := m.handleLLMStreamMsg(msg); handled {
		return updated, cmd
	}

	if updated, cmd, handled := m.consumeModelSelectionResult(msg); handled {
		return updated, cmd
	}

	if updated, cmd, handled := m.consumeAdversaryModelSelectionResult(msg); handled {
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

	if m.adversary.modelSelection != nil {
		return m.handleKeyMsg(msg)
	}

	switch msg := msg.(type) {
	case compactionDoneMsg:
		return m.handleCompactionDone()
	case compactionErrMsg:
		return m.handleCompactionError(msg.err)
	case cleanupDoneMsg:
		m.stopLoading()
		if msg.err != nil {
			m.output.AddError("Failed, check ~/.keen/ to manually cleanup.", repltheme.ErrorStyle)
		} else {
			m.output.AddStyledLine("  ✓ Cleanup completed", repltheme.HighlightStyle)
			m.output.AddEmptyLine()
		}
		m.adjustTextareaHeight()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil
	case updateCheckMsg:
		m.handleUpdateCheckMsg(msg)
		return m, nil
	case mcpStartupStatusMsg:
		m.handleMCPStartupStatus(msg)
		return m, nil
	case mcpConnectDoneMsg:
		m.handleMCPConnectDone(msg)
		return m, nil
	case copyNotificationExpiredMsg:
		if m.notification.expiresAt.UnixNano() == msg.expiresAt {
			m.notification.text = ""
			m.notification.expiresAt = time.Time{}
			m.adjustTextareaHeight()
		}
		return m, nil
	case subagentActivityMsg:
		m.stream.handler.HandleSubagentActivity(msg.activity)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, m.waitForAsyncEvent()

	case diffReadyMsg:
		m.stream.handler.HandleDiff(msg.req.Lines)
		close(msg.req.Done)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, m.waitForAsyncEvent()

	case askUserReadyMsg:
		if m.askUser.requester == nil || !m.askUser.requester.IsPending(msg.req) {
			return m, m.waitForAsyncEvent()
		}
		m.askUser.begin(msg.req)
		m.stream.handler.SetAskUser(&m.askUser)
		m.textarea.Reset()
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, m.waitForAsyncEvent()

	case permissionReadyMsg:
		m.stream.handler.HandlePermissionRequest(msg.req)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, m.waitForAsyncEvent()

	case spinner.TickMsg:
		if updated, cmd, handled := m.handleSpinnerTick(msg); handled {
			return updated, cmd
		}

	case tea.WindowSizeMsg:
		m.applyWindowSize(msg)
		return m, nil

	case tea.PasteMsg:
		if m.askUser.active() {
			return m.handleAskUserPasteMsg(msg)
		}

	case tea.KeyPressMsg:
		if m.inputSelection.hasSelection() {
			if msg.String() == keyEsc {
				m.inputSelection.clear()
				return m, nil
			}
			m.inputSelection.clear()
		}
		if m.selection.hasSelection() {
			if msg.String() == keyEsc {
				m.selection.clear()
				return m, nil
			}
		}
		return m.handleKeyMsg(msg)

	case tea.MouseClickMsg:
		if handled, cmd := m.handleMouseDown(msg); handled {
			return m, cmd
		}

	case tea.MouseMotionMsg:
		if handled := m.handleMouseDrag(msg); handled {
			return m, nil
		}

	case tea.MouseReleaseMsg:
		if handled, cmd := m.handleMouseUp(); handled {
			return m, cmd
		}

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

	if replwidgets.IsComplete(msg) {
		modelName := displayModelName(m.modelSelection.SelectedProvider, m.modelSelection.SelectedModel)
		successMsg := "✓ Updated to " + m.modelSelection.SelectedProvider + " / " + modelName
		m.output.AddStyledLine("  "+successMsg, repltheme.HighlightStyle)
		m.output.AddEmptyLine()
		m.modelSelection = nil
		m.refreshContextStatus()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil, true
	}

	if replwidgets.IsCancel(msg) {
		cancelStyle := lipgloss.NewStyle().Foreground(repltheme.MutedColor)
		m.output.AddStyledLine("  Model selection cancelled", cancelStyle)
		m.output.AddEmptyLine()
		m.modelSelection = nil
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil, true
	}

	return m, nil, false
}

func (m replModel) consumeAdversaryModelSelectionResult(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if m.adversary.modelSelection == nil {
		return m, nil, false
	}

	if replwidgets.IsComplete(msg) {
		modelName := displayModelName(m.adversary.modelSelection.SelectedProvider, m.adversary.modelSelection.SelectedModel)
		successMsg := "✓ Adversary set to " + m.adversary.modelSelection.SelectedProvider + " / " + modelName
		m.output.AddStyledLine("  "+successMsg, repltheme.HighlightStyle)
		m.output.AddEmptyLine()
		m.adversary.modelSelection = nil
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil, true
	}

	if replwidgets.IsCancel(msg) {
		cancelStyle := lipgloss.NewStyle().Foreground(repltheme.MutedColor)
		m.output.AddStyledLine("  Adversary model selection cancelled", cancelStyle)
		m.output.AddEmptyLine()
		m.adversary.modelSelection = nil
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil, true
	}

	return m, nil, false
}

func (m replModel) handleSpinnerTick(msg spinner.TickMsg) (replModel, tea.Cmd, bool) {
	if !m.loading.showSpinner && !m.btw.showSpinner && !m.adversary.showSpinner {
		return m, nil, false
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	if m.loading.showSpinner {
		m.loading.spinner, cmd = m.loading.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.btw.showSpinner {
		m.btw.spinner, cmd = m.btw.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.adversary.showSpinner {
		m.adversary.spinner, cmd = m.adversary.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}
	m.updateViewportContent()
	return m, tea.Batch(cmds...), true
}

func (m replModel) View() tea.View {
	var content string

	if m.quitting {
		content = lipgloss.NewStyle().Foreground(repltheme.MutedColor).Render("\n  Goodbye!\n")
	} else {
		var view strings.Builder

		viewportView := m.selection.render(m.viewport.View(), m.viewport.Width(), m.viewport.Height(), m.viewport.YOffset())
		view.WriteString(viewportView)
		view.WriteString("\n")

		if m.loading.showSpinner {
			elapsed := time.Duration(0)
			if !m.loading.startedAt.IsZero() {
				elapsed = time.Since(m.loading.startedAt)
			}
			spinnerText := " " + m.loading.spinner.View() + " " + renderLoadingText(m.loading.text, elapsed)
			if m.notification.text != "" {
				spinnerText += "  " + repltheme.AccentStyle.Render(m.notification.text)
			}
			view.WriteString("\n")
			view.WriteString(spinnerText)
			view.WriteString("\n")
		} else if m.notification.text != "" {
			view.WriteString("\n")
			view.WriteString("  ")
			view.WriteString(repltheme.AccentStyle.Render(m.notification.text))
			view.WriteString("\n")
		} else if m.loading.lastTurnElapsedMsg != "" {
			view.WriteString("\n")
			view.WriteString(repltheme.TurnElapsedStyle.Render(m.loading.lastTurnElapsedMsg))
			view.WriteString("\n")
		}

		if queuedView := m.renderQueuedInputs(); queuedView != "" {
			view.WriteString(queuedView)
		}

		inputValue := m.textarea.Value()
		shellMode := strings.HasPrefix(inputValue, "!")
		btwMode := inputValue == replcommands.Btw || strings.HasPrefix(inputValue, replcommands.Btw+" ")
		adversaryMode := inputValue == replcommands.Adversary || strings.HasPrefix(inputValue, replcommands.Adversary+" ")
		styles := m.textarea.Styles()
		switch {
		case shellMode:
			styles.Focused.Prompt = repltheme.ShellPromptStyle
		case btwMode:
			styles.Focused.Prompt = repltheme.BtwPromptStyle
		case adversaryMode:
			styles.Focused.Prompt = repltheme.AdversaryPromptStyle
		case m.currentMode() == llm.ModePlan:
			styles.Focused.Prompt = repltheme.PromptPlanStyle
		default:
			styles.Focused.Prompt = repltheme.PromptStyle
		}
		m.textarea.SetStyles(styles)

		textareaView := m.inputSelection.render(m.textarea.View(), m.textarea.Width()+inputPromptWidth, m.textarea.Height(), m.textarea.ScrollYOffset(), inputPromptWidth)
		view.WriteString(renderInputArea(textareaView, m.width, m.textarea.Focused(), shellMode, btwMode, adversaryMode, m.currentMode()))
		view.WriteString("\n")
		if m.suggestion.Visible() {
			view.WriteString(m.suggestion.View(m.width))
			view.WriteString("\n")
		}
		view.WriteString(m.inputMetaView())

		content = view.String()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m replModel) inputMetaView() string {
	return m.inputMetaLocationLine() + "\n" + m.inputMetaStatusLine()
}

func (m replModel) inputMetaLocationLine() string {
	const leftPad = "  "
	const sep = " • "

	model := m.inputMetaModel()
	location := abbreviateHome(m.ctx.workingDir)
	prefix := leftPad + repltheme.HighlightStyle.Render(model) + repltheme.MetaLabelStyle.Render(sep)
	available := m.width - lipgloss.Width(prefix)
	if m.width > 0 && lipgloss.Width(location) > available && available > 1 {
		location = ansi.Truncate(location, available-1, "…")
	}

	line := prefix + repltheme.HighlightStyle.Render(location)
	if m.gitBranch != "" {
		branch := m.gitBranch
		available -= lipgloss.Width(location) + lipgloss.Width(sep)
		if available > 1 && lipgloss.Width(branch) > available {
			branch = ansi.Truncate(branch, available-1, "…")
		}
		if available > 0 {
			line += repltheme.MetaLabelStyle.Render(sep) + repltheme.HighlightStyle.Render(branch)
		}
	}
	if m.contextStatus.ShouldSuggestCompaction() {
		hint := repltheme.CompactionSuggestionStyle.Render("Try /compact")
		if m.width <= 0 || lipgloss.Width(line)+1+lipgloss.Width(hint) <= m.width {
			line += repltheme.MetaLabelStyle.Render(sep) + hint
		}
	}
	return line
}

func (m replModel) inputMetaModel() string {
	if m.ctx == nil || m.ctx.cfg == nil || m.ctx.cfg.Model == "" {
		return "-"
	}

	model := displayModelName(m.ctx.cfg.Provider, m.ctx.cfg.Model)
	if m.ctx.cfg.Provider != "" {
		return m.ctx.cfg.Provider + "/" + model
	}
	return model
}

func (m replModel) inputMetaStatusLine() string {
	thinkingText := ""
	if m.ctx != nil && m.ctx.cfg != nil && m.ctx.cfg.ThinkingEffort != "" && m.ctx.registry != nil {
		if modelMeta, ok := m.ctx.registry.GetModel(m.ctx.cfg.Provider, m.ctx.cfg.Model); ok && modelMeta.SupportsThinkingEffort() {
			effortValue := m.ctx.cfg.ThinkingEffort
			if m.ctx.cfg.Provider == config.ProviderAnthropic {
				effortValue += " (adaptive)"
			}
			thinkingText = repltheme.MetaLabelStyle.Render("thinking:") + " " + repltheme.HighlightStyle.Render(effortValue)
		}
	}

	contextText := renderContextStatus(m.contextStatus)

	timerText := ""
	if m.loading.showSpinner {
		timerText = repltheme.LoadingTimerStyle.Render("⏱ " + m.loadingElapsedText())
	}

	parts := make([]string, 0, 3)
	if thinkingText != "" {
		parts = append(parts, thinkingText)
	}
	parts = append(parts, contextText)
	if timerText != "" {
		parts = append(parts, timerText)
	}
	return "  " + strings.Join(parts, repltheme.MetaLabelStyle.Render(" • "))
}

func (m *replModel) replayLoadedSession(loaded *session.LoadedSession) {
	if loaded == nil {
		return
	}

	replay := newSessionReplay(m.width, m.mdRenderer, m.ctx.workingDir)
	replay.handler.showThinking = m.showThinking

	for _, event := range loaded.Events {
		replay.applyEvent(event)
	}

	replay.flushDone()

	m.output = replay.output
	m.appState.ReplaceMessages(session.BuildConversation(loaded.Events))
	m.history.Reset()
	m.sessionPicker = nil
	m.contextStatus.ResetTotals()
	m.refreshContextStatus()
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func RunREPL(
	version string,
	workingDir string,
	cfg *config.ResolvedConfig,
	loader *config.Loader,
	globalCfg *config.GlobalConfig,
	registry *providers.Registry,
	needsSetup bool,
	mcpRuntime keenmcp.Runtime,
	resumeSession *session.LoadedSession,
) (string, error) {
	ctx := &replContext{
		version:       version,
		workingDir:    workingDir,
		cfg:           cfg,
		globalCfg:     globalCfg,
		loader:        loader,
		registry:      registry,
		mcp:           mcpRuntime,
		resumeSession: resumeSession,
	}

	var llmClient llm.LLMClient
	if cfg.Model != "" && (!config.RequiresAPIKey(cfg.Provider) || cfg.APIKey != "") {
		client, err := llm.NewClient(cfg)
		if err != nil {
			return "", fmt.Errorf("failed to initialize LLM client: %w", err)
		}
		llmClient = client
	}

	m := initialModel(ctx, llmClient, needsSetup)
	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		return "", err
	}

	return m.sessions.currentID(), nil
}

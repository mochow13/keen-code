package repl

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mochow13/keen-code/internal/cleanup"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	repltooling "github.com/mochow13/keen-code/internal/cli/repl/tooling"
	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
	"github.com/mochow13/keen-code/internal/mcpskills"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/skills"
	"github.com/mochow13/keen-code/internal/subagents"
	"github.com/mochow13/keen-code/internal/updater"
)

var loadingTexts = []string{
	"`/btw` asks aside, off the record",
	"`Shift+Tab` swaps plan ↔ build",
	"`/adversary` calls in a critic",
	"`@file` autocompletes paths",
	"`Shift+Enter` adds a newline",
	"`PgUp`/`PgDn` scrolls half a page",
	"`Home`/`End` jumps to top/bottom",
	"`Esc` cancels the active stream",
	"`/compact <hint>` shapes the summary",
	"`/sessions` lists past chats",
	"`/allow-permission` silences prompts",
	"`/reset-permission` restores defaults",
	"Skills live in `.agents/skills/`",
	"Skills take `$1`, `$2`, `$ARGUMENTS`",
	"`/show-thinking on` reveals reasoning",
	"`/thinking` sets effort: low → max",
	"`/tool-history full` retains tool outputs between turns",
	"`/mcp connect` re-auths a server",
	"`grep` tool's `output_mode:file` lists names",
	"`read_file` tool accepts `offset` & `limit`",
	"Workdir `bash` auto-approves",
	"`/adversary model` picks the critic",
	"`/mode build` exits plan-only mode",
	"`/skills list` shows everything",
	"`Tab` swaps input ↔ viewport focus",
	"Drag selection copies on release",
	"`/mcp connect` takes tool names too",
	"`Alt`/`Option`+click opens a link",
	"Queued prompts auto-run when agent finishes",
	"`/emptyq` clears the queue",
}

func displayModelName(providerID, modelID string) string {
	if providerID == config.ProviderBedrock {
		return strings.TrimPrefix(modelID, "global.")
	}
	return modelID
}

var keenSparkleSpinner = spinner.Spinner{
	Frames: []string{"·", "✦", "✧", "✫", "✧", "✦"},
	FPS:    time.Second / 8,
}

var keenWandTrailSpinner = spinner.Spinner{
	Frames: []string{"~", "⌒", "∼", "≈", "∼", "⌒"},
	FPS:    time.Second / 8,
}

var keenBrailleDriftSpinner = spinner.Spinner{
	Frames: []string{"⠁", "⠂", "⠄", "⡀", "⢀", "⠠", "⠐", "⠈"},
	FPS:    time.Second / 10,
}

var keenStarTwinkleSpinner = spinner.Spinner{
	Frames: []string{"⋆", "✧", "✦", "✷", "✦", "✧"},
	FPS:    time.Second / 8,
}

var keenStarBlinkSpinner = spinner.Spinner{
	Frames: []string{"*", "+", "×", "+"},
	FPS:    time.Second / 8,
}

var keenPotionBubblesSpinner = spinner.Spinner{
	Frames: []string{"·", "°", "∘", "○", "∘", "°"},
	FPS:    time.Second / 8,
}

var keenCrystalBallSpinner = spinner.Spinner{
	Frames: []string{"◌", "○", "◎", "●", "◎", "○"},
	FPS:    time.Second / 8,
}

var keenBeepingPointerSpinner = spinner.Spinner{
	Frames: []string{"●", " "},
	FPS:    time.Second / 4,
}

var loadingSpinners = []spinner.Spinner{
	spinner.Dot,
	spinner.MiniDot,
	spinner.Jump,
	spinner.Pulse,
	keenSparkleSpinner,
	keenWandTrailSpinner,
	keenBrailleDriftSpinner,
	keenStarTwinkleSpinner,
	keenPotionBubblesSpinner,
	keenCrystalBallSpinner,
	keenBeepingPointerSpinner,
	keenStarBlinkSpinner,
}

func nextLoadingText() string {
	return loadingTexts[rand.IntN(len(loadingTexts))]
}

func renderLoadingText(text string, elapsed time.Duration) string {
	parts := strings.Split(text, "`")

	visibleLen := 0
	for _, p := range parts {
		visibleLen += len([]rune(p))
	}
	if visibleLen == 0 {
		return ""
	}

	cycleLen := visibleLen + 12
	pos := int(elapsed.Milliseconds()/30) % cycleLen

	var sb strings.Builder
	visIdx := 0
	for i, part := range parts {
		isCode := i%2 == 1
		for _, r := range part {
			d := pos - visIdx
			if d < 0 {
				d = -d
			}
			var style lipgloss.Style
			if isCode {
				style = repltheme.LoadingTextCodeStyle
				switch {
				case d == 0:
					style = repltheme.LoadingTextCodeShimmerStyle
				case d <= 2:
					style = repltheme.LoadingTextCodeShimmerMid
				}
			} else {
				style = repltheme.LoadingTextStyled
				switch {
				case d == 0:
					style = repltheme.LoadingTextShimmerStyle
				case d <= 2:
					style = repltheme.LoadingTextShimmerMid
				}
			}
			sb.WriteString(style.Render(string(r)))
			visIdx++
		}
	}
	return sb.String()
}

func nextLoadingSpinner() spinner.Spinner {
	return loadingSpinners[rand.IntN(len(loadingSpinners))]
}

func (m *replModel) startLoading(text string) {
	m.loading.lastTurnElapsedMsg = ""
	m.loading.showSpinner = true
	m.loading.spinner.Spinner = nextLoadingSpinner()
	m.loading.text = text
	m.loading.startedAt = time.Now()
}

func cleanupCmd() tea.Cmd {
	return func() tea.Msg {
		return cleanupDoneMsg{err: cleanup.Run()}
	}
}

func (m *replModel) stopLoading() {
	if !m.loading.startedAt.IsZero() {
		elapsed := time.Since(m.loading.startedAt)
		m.loading.lastTurnElapsedMsg = " ✔ " + pickTurnElapsedVerb() + " " + formatTurnElapsed(elapsed)
	} else {
		m.loading.lastTurnElapsedMsg = ""
	}
	m.loading.showSpinner = false
	m.loading.startedAt = time.Time{}
	m.refreshGitBranch()
}

func waitForMCPStartup(runtime keenmcp.Runtime) tea.Cmd {
	if runtime == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := runtime.WaitInitialScan(ctx)
		return mcpStartupStatusMsg{Statuses: runtime.Servers(), Err: err}
	}
}

func (m *replModel) handleMCPStartupStatus(msg mcpStartupStatusMsg) {
	m.syncMCPSkills(msg.Statuses)

	if msg.Err != nil {
		m.output.AddLine(wrapTextWithStyle("  MCP startup timed out: "+msg.Err.Error(), repltheme.ErrorStyle, m.messageWidth()))
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
	}

	var failed []keenmcp.ServerStatus
	for _, status := range msg.Statuses {
		if isMCPFailureState(status.State) {
			failed = append(failed, status)
		}
	}
	if len(failed) == 0 {
		return
	}
	for _, status := range failed {
		line := "  MCP connection failed for " + status.Name + ". Try `/mcp connect " + status.Name + "` to connect."
		if status.LastError != "" {
			line += " (" + status.LastError + ")"
		}
		m.output.AddLine(wrapTextWithStyle(line, repltheme.ErrorStyle, m.messageWidth()))
	}
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func (m *replModel) messageWidth() int {
	width := m.width
	if width <= 0 && m.viewport.Width() > 0 {
		width = m.viewport.Width()
	}
	if width <= 0 {
		width = defaultWidth
	}
	return max(width-4, 1)
}

func renderTipText(text string, width int) string {
	parts := strings.Split(text, "`")
	var sb strings.Builder
	for i, part := range parts {
		if i%2 == 1 {
			sb.WriteString(repltheme.TipCodeStyle.Render(part))
		} else {
			sb.WriteString(repltheme.TipStyle.Render(part))
		}
	}
	width = max(width, 1)
	return lipgloss.NewStyle().Width(width).Render(sb.String())
}

func wrapTextWithStyle(text string, style lipgloss.Style, width int) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Width(width).Render(style.Render(text))
}

func isMCPFailureState(state keenmcp.ServerState) bool {
	return state == keenmcp.StateDisconnected || state == keenmcp.StateAuthRequired || state == keenmcp.StateAuthFailed
}

func (m *replModel) syncMCPSkills(statuses []keenmcp.ServerStatus) {
	configured := map[string]bool{}
	reloadSkills := false
	for _, status := range statuses {
		configured[status.Name] = true
		if status.State == keenmcp.StateConnected {
			if m.refreshMCPSkill(status.Name, status.Description) {
				reloadSkills = true
			}
			continue
		}
		if isMCPFailureState(status.State) {
			_ = m.appState.SetSkillStatus(mcpskills.SkillName(status.Name), skills.StatusDisabled)
			reloadSkills = true
		}
	}
	if m.removeUnconfiguredMCPSkillStatuses(configured) {
		reloadSkills = true
	}
	if reloadSkills {
		m.appState.ReloadSkills()
	}
}

func (m *replModel) removeUnconfiguredMCPSkillStatuses(configured map[string]bool) bool {
	changed := false
	for name := range m.appState.GetSkillsConfig().IsEnabled {
		if !mcpskills.IsSkillName(name) {
			continue
		}
		server := mcpskills.ServerName(name)
		if configured[server] {
			continue
		}
		_ = m.appState.RemoveSkillStatus(name)
		_ = mcpskills.Remove(server)
		changed = true
	}
	return changed
}

func (m *replModel) refreshMCPSkill(server, description string) bool {
	if m.ctx == nil || m.ctx.mcp == nil {
		return false
	}
	tools, err := m.ctx.mcp.ListTools(context.Background(), server)
	if err != nil {
		slog.Default().Debug("mcpskills list tools failed", "server", server, "error", err)
		return false
	}
	if err := mcpskills.Generate(server, description, tools); err != nil {
		slog.Default().Debug("mcpskills generate failed", "server", server, "error", err)
		return false
	}
	return true
}

func (m *replModel) handleMCPConnectDone(msg mcpConnectDoneMsg) {
	m.stopLoading()
	m.adjustTextareaHeight()
	changed := false
	if msg.Err != nil {
		m.output.AddError("MCP connect failed for "+msg.Server+": "+msg.Err.Error(), repltheme.ErrorStyle)
		_ = m.appState.SetSkillStatus(mcpskills.SkillName(msg.Server), skills.StatusDisabled)
		changed = true
	} else {
		m.output.AddStyledLine("  ✔ MCP server connected: "+msg.Server, repltheme.HighlightStyle)
		description := ""
		if m.ctx != nil && m.ctx.mcp != nil {
			description = m.ctx.mcp.Status(msg.Server).Description
		}
		if m.refreshMCPSkill(msg.Server, description) {
			changed = true
		}
		_ = m.appState.SetSkillStatus(mcpskills.SkillName(msg.Server), skills.StatusEnabled)
		changed = true
	}
	if changed {
		m.appState.ReloadSkills()
	}
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func (m replModel) loadingElapsedText() string {
	if m.loading.startedAt.IsZero() {
		return "0:00"
	}
	return formatLoadingElapsed(time.Since(m.loading.startedAt))
}

func formatLoadingElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int(d.Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

var turnElapsedVerbs = []string{
	"Crunched for",
	"Processed in",
	"Completed in",
	"Finished in",
	"Resolved in",
	"Handled in",
	"Took",
	"Ran for",
}

func pickTurnElapsedVerb() string {
	return turnElapsedVerbs[rand.IntN(len(turnElapsedVerbs))]
}

func formatTurnElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSeconds := int(d.Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	switch {
	case minutes > 0 && seconds > 0:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

func (m *replModel) currentMode() llm.AgentMode {
	if m.mode == "" {
		return llm.ModeBuild
	}
	return m.mode
}

func (m *replModel) setMode(mode llm.AgentMode) {
	if mode != llm.ModePlan {
		mode = llm.ModeBuild
	}
	m.mode = mode
	if m.appState != nil {
		m.appState.SetMode(mode)
	}
}

func (m *replModel) toggleMode() {
	if m.currentMode() == llm.ModePlan {
		m.setMode(llm.ModeBuild)
	} else {
		m.setMode(llm.ModePlan)
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func abbreviateHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if after, ok := strings.CutPrefix(path, home); ok {
		return "~" + after
	}
	return path
}

func buildInitialScreen(ctx *replContext, lastSession *session.Summary, width int) []string {
	if width <= 0 {
		width = defaultWidth
	}
	var lines []string

	lines = append(lines, "")
	metadataStyle := repltheme.InitialScreenMetadataStyle
	artStyles := repltheme.InitialScreenArtStyles
	header := repltheme.InitialScreenWordmarkStyle.Render("✦ keen v" + ctx.version + " .✦ ݁˖")
	art := []string{
		"░█░█░█▀▀░█▀▀░█▀█░░░█▀▀░█▀█░█▀▄░█▀▀",
		"░█▀▄░█▀▀░█▀▀░█░█░░░█░░░█░█░█░█░█▀▀",
		"░▀░▀░▀▀▀░▀▀▀░▀░▀░░░▀▀▀░▀▀▀░▀▀░░▀▀▀",
	}
	for lineIndex, line := range art {
		runes := []rune(line)
		var artLine strings.Builder
		for i, style := range artStyles {
			start := i * len(runes) / len(artStyles)
			end := (i + 1) * len(runes) / len(artStyles)
			artLine.WriteString(style.Render(string(runes[start:end])))
		}
		renderedLine := "  " + artLine.String()
		if lineIndex == len(art)-1 {
			renderedLine += " " + header
		}
		lines = append(lines, renderedLine)
	}
	lines = append(lines, "")

	if lastSession != nil && lastSession.LastUserMessage != "" {
		preview := strings.Join(strings.Fields(lastSession.LastUserMessage), " ")
		previewWidth := max(width-24, 12)
		if len([]rune(preview)) > previewWidth {
			preview = string([]rune(preview)[:previewWidth-1]) + "…"
		}

		rail := metadataStyle.Render("┃")
		lines = append(lines, "  "+rail+"  "+metadataStyle.Render("Last session")+" "+metadataStyle.Render("·")+" "+repltheme.InitialScreenWordmarkStyle.Render("“"+preview+"”"))
		lines = append(lines, "  "+rail+"  "+metadataStyle.Render(formatTimeAgo(lastSession.UpdatedAt))+" "+metadataStyle.Render("·")+" "+metadataStyle.Render("/resume to begin where you left off"))
		lines = append(lines, "")
	}

	rule := repltheme.InitialScreenRuleStyle.Render(strings.Repeat("─", width))
	label := "  " + repltheme.InitialScreenTipLabelStyle.Render("✦ Tip of the session")
	indent := "  "
	tipText := indent + strings.ReplaceAll(renderTipText(randomTip(), width-len(indent)), "\n", "\n"+indent)
	lines = append(lines, rule)
	lines = append(lines, label)
	lines = append(lines, tipText)
	lines = append(lines, rule)
	lines = append(lines, "")

	return lines
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d day(s) ago", days)
	default:
		return t.Local().Format("Jan 2")
	}
}

func checkForUpdate(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		latest, newer, err := updater.CheckLatest(context.Background(), currentVersion, "mochow13", "keen-code")
		if err != nil || !newer {
			return updateCheckMsg{}
		}
		return updateCheckMsg{latest: latest}
	}
}

func formatModelSelectionCard(ms *replwidgets.Model, width int) string {
	ruleWidth := defaultWidth
	if width > 0 {
		ruleWidth = width
	}
	if ruleWidth < 1 {
		ruleWidth = 1
	}

	contentWidth := max(ruleWidth-2, 1)

	rule := repltheme.ModelSelectionRuleStyle.Render(strings.Repeat("─", ruleWidth))
	lines := strings.Split(strings.TrimRight(ms.ViewString(), "\n"), "\n")
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(rule)
	sb.WriteString("\n\n")
	for _, l := range lines {
		wrapped := wrapTextWithStyle(l, lipgloss.NewStyle(), contentWidth)
		for _, wrappedLine := range strings.Split(wrapped, "\n") {
			sb.WriteString("  ")
			sb.WriteString(wrappedLine)
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")
	sb.WriteString(rule)
	sb.WriteString("\n")
	return sb.String()
}

func renderRulesWithChip(width int, ruleStyle lipgloss.Style, chipText string, chipStyle lipgloss.Style) (topRule string, bottomRule string) {
	chip := chipStyle.Render(chipText)
	chipWidth := lipgloss.Width(chip)
	trailingDash := 3
	leftDashLen := max(width-chipWidth-trailingDash, 0)
	rightDashLen := max(width-leftDashLen-chipWidth, 0)
	topRule = ruleStyle.Render(strings.Repeat("─", leftDashLen)) + chip + ruleStyle.Render(strings.Repeat("─", rightDashLen))
	bottomRule = ruleStyle.Render(strings.Repeat("─", width))
	return
}

func renderInputArea(content string, width int, focused bool, shellMode bool, btwMode bool, adversaryMode bool, mode llm.AgentMode) string {
	ruleWidth := defaultWidth
	if width > 0 {
		ruleWidth = width
	}
	if ruleWidth < 1 {
		ruleWidth = 1
	}

	ruleStyle := repltheme.InputRuleStyle
	if !focused {
		ruleStyle = repltheme.InputRuleBlurredStyle
	} else if shellMode {
		ruleStyle = repltheme.ShellInputRuleStyle
	} else if btwMode {
		ruleStyle = repltheme.BtwInputRuleStyle
	} else if adversaryMode {
		ruleStyle = repltheme.AdversaryInputRuleStyle
	} else if mode == llm.ModePlan {
		ruleStyle = repltheme.PlanInputRuleStyle
	}

	switch {
	case shellMode && focused:
		topRule, bottomRule := renderRulesWithChip(ruleWidth, ruleStyle, "shell", repltheme.ShellChipStyle)
		return topRule + "\n" + content + "\n" + bottomRule
	case btwMode && focused:
		topRule, bottomRule := renderRulesWithChip(ruleWidth, ruleStyle, "btw", repltheme.BtwChipStyle)
		return topRule + "\n" + content + "\n" + bottomRule
	case adversaryMode && focused:
		topRule, bottomRule := renderRulesWithChip(ruleWidth, ruleStyle, "adversary", repltheme.AdversaryChipStyle)
		return topRule + "\n" + content + "\n" + bottomRule
	}

	chipStyle := repltheme.ModeBuildChipStyle
	if mode == llm.ModePlan {
		chipStyle = repltheme.ModePlanChipStyle
	}
	topRule, bottomRule := renderRulesWithChip(ruleWidth, ruleStyle, string(mode), chipStyle)
	return topRule + "\n" + content + "\n" + bottomRule
}

func waitForAsyncEvent(llmCh <-chan llm.StreamEvent, permissionCh <-chan *replpermissions.Request, diffCh <-chan repltooling.DiffRequest, subagentCh <-chan subagents.ToolActivity, askUserCh <-chan *replaskuser.Request) tea.Cmd {
	if llmCh == nil {
		return nil
	}

	return func() tea.Msg {
		select {
		case activity := <-subagentCh:
			return subagentActivityMsg{activity: activity}
		case req := <-permissionCh:
			return permissionReadyMsg{req: req}
		case req := <-askUserCh:
			return askUserReadyMsg{req: req}
		case req := <-diffCh:
			return diffReadyMsg{req: req}
		case event, ok := <-llmCh:
			return mainStreamMsg{eventCh: llmCh, event: event, closed: !ok}
		}
	}
}

func (m *replModel) spinnerHeight() int {
	if m.loading.showSpinner {
		return 2
	}
	return 0
}

func (m *replModel) notificationHeight() int {
	if m.notification.text == "" || m.loading.showSpinner {
		return 0
	}
	return 2
}

func (m *replModel) elapsedTimeHeight() int {
	if !m.loading.showSpinner && m.notification.text == "" && m.loading.lastTurnElapsedMsg != "" {
		return 2
	}
	return 0
}

func (m *replModel) adjustTextareaHeight() {
	if m.height <= 0 {
		return
	}
	m.viewport.SetHeight(m.height - m.textarea.Height() - 5 - m.spinnerHeight() - m.notificationHeight() - m.elapsedTimeHeight() - m.suggestion.Height() - m.queuedHeight())
}

func (m replModel) isAtTopOfInput() bool {
	return m.textarea.Line() == 0
}

func (m replModel) isAtBottomOfInput() bool {
	return m.textarea.Line() >= m.textarea.LineCount()-1
}

func (m *replModel) focusInput() tea.Cmd {
	m.suggestion.Hide()
	return m.textarea.Focus()
}

func (m *replModel) blurInput() {
	m.suggestion.Hide()
	m.textarea.Blur()
}

func (m *replModel) toggleInputFocus() tea.Cmd {
	if m.textarea.Focused() {
		m.blurInput()
		return nil
	}
	return m.focusInput()
}

func (m *replModel) handleViewportFocusKeyMsg(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case keyUp, keyShiftUp:
		m.viewport.ScrollUp(1)
		m.userScrolled = !m.viewport.AtBottom()
		return true
	case keyDown, keyShiftDown:
		m.viewport.ScrollDown(1)
		m.userScrolled = !m.viewport.AtBottom()
		return true
	case keyPageUp:
		m.viewport.HalfPageUp()
		m.userScrolled = !m.viewport.AtBottom()
		return true
	case keyPageDown:
		m.viewport.HalfPageDown()
		m.userScrolled = !m.viewport.AtBottom()
		return true
	case keyHome:
		m.viewport.GotoTop()
		m.userScrolled = true
		return true
	case keyEnd:
		m.viewport.GotoBottom()
		m.userScrolled = false
		return true
	}
	return false
}

func (m *replModel) startStreamContext() context.Context {
	if m.stream.cancel != nil {
		m.stream.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.stream.cancel = cancel
	return ctx
}

func (m *replModel) clearStreamCancel() {
	m.stream.cancel = nil
}

func (m *replModel) cancelBtwStream() {
	if m.btw.streamHandler != nil && m.btw.streamHandler.IsActive() {
		m.btw.streamHandler.HandleInterrupt()
	}
	if m.btw.streamCancel != nil {
		m.btw.streamCancel()
		m.btw.streamCancel = nil
	}
	m.btw.lines = nil
	m.btw.showSpinner = false
}

func (m *replModel) flushBtwToOutput() {
	if m.btw.lines == nil {
		return
	}
	m.output.AddLine(renderBtwLeftBorder(renderBtwQuestionHeader(m.btw.question)))
	for _, line := range m.btw.lines {
		m.output.AddLine(renderBtwLeftBorder(line))
	}
	m.output.AddEmptyLine()
	m.btw.lines = nil
	m.btw.question = ""
}

func (m *replModel) cancelAdversaryStream() {
	if m.adversary.streamHandler != nil && m.adversary.streamHandler.IsActive() {
		m.adversary.streamHandler.HandleInterrupt()
	}
	if m.adversary.streamCancel != nil {
		m.adversary.streamCancel()
		m.adversary.streamCancel = nil
	}
	m.adversary.lines = nil
	m.adversary.showSpinner = false
}

func (m *replModel) flushAdversaryToOutput() {
	if m.adversary.lines == nil {
		return
	}
	m.output.AddLine(renderAdversaryLeftBorder(renderAdversaryHeader(m.adversary.focus)))
	for _, line := range m.adversary.lines {
		m.output.AddLine(renderAdversaryLeftBorder(line))
	}
	m.output.AddEmptyLine()
	m.adversary.lines = nil
	m.adversary.focus = ""
}

func (m *replModel) buildAdversaryClient() error {
	resolved, err := config.ResolveAdversary(m.ctx.globalCfg)
	if err != nil {
		return err
	}
	client, err := llm.NewClient(resolved)
	if err != nil {
		return err
	}
	m.appState.SetAdversaryClient(client)
	return nil
}

func waitForAdversaryEvent(llmCh <-chan llm.StreamEvent) tea.Cmd {
	if llmCh == nil {
		return nil
	}

	return func() tea.Msg {
		for {
			event, ok := <-llmCh
			if !ok {
				return adversaryDoneMsg{}
			}

			switch event.Type {
			case llm.StreamEventTypeChunk:
				return adversaryChunkMsg(event.Content)
			case llm.StreamEventTypeToolStart:
				return adversaryToolStartMsg{toolCall: event.ToolCall}
			case llm.StreamEventTypeToolEnd:
				return adversaryToolEndMsg{toolCall: event.ToolCall}
			case llm.StreamEventTypeDone:
				return adversaryDoneMsg{}
			case llm.StreamEventTypeError:
				return adversaryErrorMsg{err: event.Error}
			case llm.StreamEventTypeIncomplete:
				return adversaryErrorMsg{err: event.Error}
			default:
				continue
			}
		}
	}
}

func (m *replModel) scrollToBottomIfFollowing() {
	if !m.userScrolled {
		m.viewport.GotoBottom()
	}
}

// afterStreamUpdate decides whether to redraw the viewport immediately or to
// batch the update for a later streamRenderMsg. Callers must still schedule the
// next async event wait themselves.
func (m *replModel) afterStreamUpdate() tea.Cmd {
	if m.stream.renderInterval <= 0 {
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return nil
	}

	if m.stream.renderPending {
		return nil
	}

	m.stream.renderPending = true
	return tea.Tick(m.stream.renderInterval, func(time.Time) tea.Msg {
		return streamRenderMsg{}
	})
}

// flushStreamRender forces any pending batched render to happen now. It should
// be called at stream boundaries (done/error/tool events) and on user input so
// the UI never lags behind the final state.
func (m *replModel) flushStreamRender() {
	if !m.stream.renderPending {
		return
	}
	m.stream.renderPending = false
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
}

func waitForBtwEvent(llmCh <-chan llm.StreamEvent) tea.Cmd {
	if llmCh == nil {
		return nil
	}

	return func() tea.Msg {
		for {
			event, ok := <-llmCh
			if !ok {
				return btwDoneMsg{}
			}

			switch event.Type {
			case llm.StreamEventTypeChunk:
				return btwChunkMsg(event.Content)
			case llm.StreamEventTypeDone:
				return btwDoneMsg{}
			case llm.StreamEventTypeError:
				return btwErrorMsg{err: event.Error}
			case llm.StreamEventTypeIncomplete:
				return btwErrorMsg{err: event.Error}
			default:
				continue
			}
		}
	}
}

func (m *replModel) applyWindowSize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	m.textarea.SetWidth(msg.Width - 3)
	if m.mdRenderer != nil {
		m.mdRenderer.UpdateWidth(msg.Width)
	}
	if m.output != nil {
		m.output.SetWidth(msg.Width)
	}
	m.viewport.SetWidth(msg.Width)
	m.viewport.SetHeight(msg.Height - m.textarea.Height() - 5 - m.spinnerHeight() - m.notificationHeight() - m.elapsedTimeHeight() - m.suggestion.Height() - m.queuedHeight())

	if !m.initialScreenDone && msg.Width > 0 {
		for _, line := range buildInitialScreen(m.ctx, m.lastSession, m.width) {
			m.output.AddLine(line)
		}
		if m.projectPermsErr != nil {
			m.output.AddError("Failed to load .keen/permissions.json: "+m.projectPermsErr.Error()+" (using defaults)", repltheme.ErrorStyle)
			m.output.AddEmptyLine()
		}
		m.initialScreenDone = true
		m.updateViewportContent()
		m.viewport.GotoBottom()
	}
}

func (m *replModel) updateLLMClient() error {
	client, err := llm.NewClient(m.ctx.cfg)
	if err != nil {
		return err
	}
	m.appState.UpdateClient(client)
	return nil
}

func (m *replModel) handleSessionPersistenceError(err error) {
	if err == nil {
		return
	}
	m.output.AddError("Session persistence failed: "+err.Error(), repltheme.ErrorStyle)
}

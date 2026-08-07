package repl

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	keenauth "github.com/user/keen-code/internal/auth"
	replcommands "github.com/user/keen-code/internal/cli/repl/commands"
	reploutput "github.com/user/keen-code/internal/cli/repl/output"
	repltheme "github.com/user/keen-code/internal/cli/repl/theme"
	replwidgets "github.com/user/keen-code/internal/cli/repl/widgets"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/llm"
	keenmcp "github.com/user/keen-code/internal/mcp"
	"github.com/user/keen-code/internal/memory"
	"github.com/user/keen-code/internal/skills"
	"github.com/user/keen-code/internal/subagents"
)

const (
	mcpConnectTimeout = 5 * time.Minute
	mcpUsage          = "Usage: /mcp status | /mcp connect <server>"
	skillsUsage       = "Usage: /skills list|status | /skills reload | /skills enable|disable <name>"
	subagentsUsage    = "Usage: /subagents [list]"
	bangTimeout       = 180 * time.Second
)

func (m *replModel) dispatchCommand(input string) (replModel, tea.Cmd, bool) {
	switch {
	case input == replcommands.Exit:
		m.quitting = true
		_ = m.history.Flush()
		return *m, tea.Quit, true

	case input == replcommands.Help:
		m.output.AddLine(getHelpText(m.helpWidth()))
		m.output.AddEmptyLine()
		m.textarea.Reset()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil, true

	case input == replcommands.Cleanup:
		m.textarea.Reset()
		m.startLoading("Cleaning up...")
		m.adjustTextareaHeight()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, tea.Batch(m.loading.spinner.Tick, cleanupCmd()), true

	case input == replcommands.Model:
		m.textarea.Reset()
		return m.startModelSelection(), nil, true

	case strings.HasPrefix(input, replcommands.Model+" "):
		m.textarea.Reset()
		pair := strings.TrimSpace(strings.TrimPrefix(input, replcommands.Model))
		return m.startModelSelectionPair(pair)

	case input == replcommands.MCP || strings.HasPrefix(input, replcommands.MCP+" "):
		m.textarea.Reset()
		result, cmd := m.handleMCPCommand(input)
		return result, cmd, true

	case input == replcommands.Mode || strings.HasPrefix(input, replcommands.Mode+" "):
		m.textarea.Reset()
		result := m.handleModeCommand(input)
		return result, nil, true

	case input == replcommands.Logout:
		m.textarea.Reset()
		result := m.handleLogoutCommand()
		return result, nil, true

	case input == replcommands.Memory || input == replcommands.MemoryShow || strings.HasPrefix(input, replcommands.Memory+" "):
		m.textarea.Reset()
		result := m.handleMemoryCommand(input, replcommands.MemoryShow)
		return result, nil, true

	case input == replcommands.Resume:
		m.textarea.Reset()
		summaries, err := m.sessions.listSessions()
		if err != nil {
			m.output.AddError("Failed to load sessions: "+err.Error(), repltheme.ErrorStyle)
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil, true
		}
		if len(summaries) == 0 {
			m.output.AddStyledLine("  No saved sessions for this directory.", repltheme.MutedStyle)
			m.output.AddEmptyLine()
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil, true
		}
		loaded, err := m.sessions.load(summaries[0])
		if err != nil {
			m.output.AddError("Failed to load session: "+err.Error(), repltheme.ErrorStyle)
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil, true
		}
		m.replayLoadedSession(loaded)
		return *m, nil, true

	case input == replcommands.Sessions:
		m.textarea.Reset()
		summaries, err := m.sessions.listSessions()
		if err != nil {
			m.output.AddError("Failed to load sessions: "+err.Error(), repltheme.ErrorStyle)
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil, true
		}
		if len(summaries) == 0 {
			m.output.AddStyledLine("  No saved sessions for this directory.", repltheme.MutedStyle)
			m.output.AddEmptyLine()
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil, true
		}
		m.sessionPicker = replwidgets.NewSessionPicker(summaries)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil, true

	case input == replcommands.Clear || input == replcommands.New:
		m.textarea.Reset()
		result := m.handleClearCommand()
		return result, nil, true

	case input == replcommands.Thinking || strings.HasPrefix(input, replcommands.Thinking+" "):
		m.textarea.Reset()
		result, cmd := m.handleThinkingCommand(input)
		return result, cmd, true

	case input == replcommands.ShowThinking || strings.HasPrefix(input, replcommands.ShowThinking+" "):
		m.textarea.Reset()
		result := m.handleShowThinkingCommand(input)
		return result, nil, true

	case input == replcommands.Skills || strings.HasPrefix(input, replcommands.Skills+" "):
		m.textarea.Reset()
		result := m.handleSkillsCommand(input)
		return result, nil, true

	case input == replcommands.Subagents || strings.HasPrefix(input, replcommands.Subagents+" "):
		m.textarea.Reset()
		result := m.handleSubagentsCommand(input)
		return result, nil, true

	case input == replcommands.AllowPermission || strings.HasPrefix(input, replcommands.AllowPermission+" "):
		m.textarea.Reset()
		result := m.handleToolPermissionCommand(input, replcommands.AllowPermission, true)
		return result, nil, true

	case input == replcommands.ResetPermission || strings.HasPrefix(input, replcommands.ResetPermission+" "):
		m.textarea.Reset()
		result := m.handleToolPermissionCommand(input, replcommands.ResetPermission, false)
		return result, nil, true

	case input == replcommands.Compact || strings.HasPrefix(input, replcommands.Compact+" "):
		extraPrompt := strings.TrimSpace(strings.TrimPrefix(input, replcommands.Compact))
		if !m.appState.IsClientReady(m.ctx.cfg) {
			m.output.AddError("LLM client not initialized. Use /model to configure.", repltheme.ErrorStyle)
			m.textarea.Reset()
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil, true
		}
		m.textarea.Reset()
		result, cmd := m.startCompaction(extraPrompt)
		return result, cmd, true

	case input == replcommands.Context:
		m.textarea.Reset()
		m.handleContextCommand()
		return *m, nil, true

	default:
		return *m, nil, false
	}
}

func (m *replModel) startModelSelection() replModel {
	onComplete := func(provider, model, apiKey string) error {
		return m.updateLLMClient()
	}
	m.modelSelection = replwidgets.New(
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

func (m *replModel) startModelSelectionPair(pair string) (replModel, tea.Cmd, bool) {
	providerID, modelID, ok := strings.Cut(pair, "/")
	if !ok || providerID == "" || modelID == "" || strings.Contains(modelID, "/") {
		m.output.AddError("Usage: /model <provider/model-id>", repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil, true
	}

	updated := m.startModelSelection()
	selection, cmd, err := updated.modelSelection.Select(providerID, modelID)
	updated.modelSelection = selection
	if err != nil {
		updated.output.AddError(err.Error(), repltheme.ErrorStyle)
		updated.modelSelection = nil
		updated.updateViewportContent()
		updated.viewport.GotoBottom()
		return updated, nil, true
	}
	updated.updateViewportContent()
	updated.viewport.GotoBottom()
	return updated, cmd, true
}

func (m *replModel) startCompaction(extraPrompt string) (replModel, tea.Cmd) {
	if m.compaction.cancel != nil {
		m.compaction.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	eventCh, err := m.appState.StreamCompact(ctx, m.ctx.cfg, extraPrompt, llm.StreamOptions{SessionID: m.sessions.currentID()})
	if err != nil {
		cancel()
		m.output.AddError(err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}
	if eventCh == nil {
		cancel()
		m.output.AddError("compaction stream unavailable", repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	m.compaction.cancel = cancel
	m.compaction.active = true
	m.compaction.mode = compactionManual
	m.startLoading("Compacting...")
	m.clearTurnMemory()
	m.stream.handler.Start(eventCh, m.loading.text)
	m.userScrolled = false
	m.adjustTextareaHeight()
	m.updateViewportContent()
	m.viewport.GotoBottom()

	return *m, tea.Batch(m.loading.spinner.Tick, m.waitForAsyncEvent())
}

func (m *replModel) handleThinkingCommand(input string) (replModel, tea.Cmd) {
	effort := strings.TrimSpace(strings.TrimPrefix(input, replcommands.Thinking))

	modelMeta, ok := m.ctx.registry.GetModel(m.ctx.cfg.Provider, m.ctx.cfg.Model)
	if !ok || !modelMeta.SupportsThinkingEffort() {
		m.output.AddError("Current model does not support configurable thinking", repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if !slices.Contains(modelMeta.ThinkingEfforts, effort) {
		m.output.AddError("Usage: /thinking "+strings.Join(modelMeta.ThinkingEfforts, "|"), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	m.ctx.cfg.ThinkingEffort = effort
	m.ctx.globalCfg.ThinkingEffort = effort
	if err := m.ctx.loader.Save(m.ctx.globalCfg); err != nil {
		m.output.AddError("Failed to save config: "+err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if err := m.updateLLMClient(); err != nil {
		m.output.AddError("Failed to reinitialize LLM client: "+err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	m.output.AddStyledLine("  ✓ Thinking effort set to: "+effort, repltheme.HighlightStyle)
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m, nil
}

func (m *replModel) handleMemoryCommand(input, showCommand string) replModel {
	isShow := input == showCommand || strings.HasPrefix(input, showCommand+" ")
	scope := strings.TrimSpace(strings.TrimPrefix(input, replcommands.Memory))
	if isShow {
		scope = strings.TrimSpace(strings.TrimPrefix(input, showCommand))
	}

	global, project := memory.Files(m.ctx.workingDir)

	if !isShow {
		if scope != "" {
			m.output.AddStyledLine("  Usage: /memory | /memory show", repltheme.UsageHintStyle)
			m.output.AddEmptyLine()
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m
		}
		if !global.Exists && !project.Exists && global.Path != "" && project.Path != "" {
			m.output.AddStyledLine("  No memory files found.", repltheme.MutedStyle)
			m.output.AddEmptyLine()
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m
		}
		m.output.AddStyledLine("  Global memory:  "+global.Path+existsLabel(global.Exists), labelStyle(global.Exists))
		m.output.AddStyledLine("  Project memory: "+project.Path+existsLabel(project.Exists), labelStyle(project.Exists))
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	switch scope {
	case "":
		if !global.Exists && !project.Exists {
			m.output.AddStyledLine("  Memory is empty.", repltheme.MutedStyle)
			m.output.AddEmptyLine()
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m
		}
		if global.Exists {
			m.output.AddStyledLine("  ## Global memory", repltheme.MutedStyle.Bold(true))
			m.output.AddStyledLine("  path: "+global.Path, repltheme.HelpDescStyle)
			m.output.AddLine("")
			for _, line := range strings.Split(strings.TrimSpace(global.Content), "\n") {
				m.output.AddLine("  " + line)
			}
			m.output.AddLine("")
		}
		if project.Exists {
			m.output.AddStyledLine("  ## Project memory", repltheme.MutedStyle.Bold(true))
			m.output.AddStyledLine("  path: "+project.Path, repltheme.HelpDescStyle)
			m.output.AddLine("")
			for _, line := range strings.Split(strings.TrimSpace(project.Content), "\n") {
				m.output.AddLine("  " + line)
			}
			m.output.AddLine("")
		}
	case "global":
		if global.Exists {
			m.output.AddStyledLine("  ## Global memory", repltheme.MutedStyle.Bold(true))
			m.output.AddStyledLine("  path: "+global.Path, repltheme.HelpDescStyle)
			m.output.AddLine("")
			for _, line := range strings.Split(strings.TrimSpace(global.Content), "\n") {
				m.output.AddLine("  " + line)
			}
		} else {
			m.output.AddStyledLine("  Global memory is empty.", repltheme.MutedStyle)
		}
		m.output.AddEmptyLine()
	case "project":
		if project.Exists {
			m.output.AddStyledLine("  ## Project memory", repltheme.MutedStyle.Bold(true))
			m.output.AddStyledLine("  path: "+project.Path, repltheme.HelpDescStyle)
			m.output.AddLine("")
			for _, line := range strings.Split(strings.TrimSpace(project.Content), "\n") {
				m.output.AddLine("  " + line)
			}
		} else {
			m.output.AddStyledLine("  Project memory is empty.", repltheme.MutedStyle)
		}
		m.output.AddEmptyLine()
	}

	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func existsLabel(exists bool) string {
	if exists {
		return "  exists"
	}
	return "  missing"
}

func labelStyle(exists bool) lipgloss.Style {
	if exists {
		return repltheme.HighlightStyle
	}
	return repltheme.MutedStyle
}

func (m *replModel) handleModeCommand(input string) replModel {
	arg := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(input, replcommands.Mode)))

	switch arg {
	case "plan":
		m.setMode(llm.ModePlan)
	case "build":
		m.setMode(llm.ModeBuild)
	case "":
		m.output.AddStyledLine("  Mode: "+string(m.currentMode())+" (use /mode plan|build)", repltheme.HighlightStyle)
	default:
		m.output.AddError("Usage: /mode plan|build", repltheme.ErrorStyle)
	}

	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) handleShowThinkingCommand(input string) replModel {
	arg := strings.TrimSpace(strings.TrimPrefix(input, replcommands.ShowThinking))

	switch arg {
	case "on":
		m.showThinking = true
		m.stream.handler.showThinking = true
		m.saveShowThinking(true)
		m.output.AddStyledLine("  ✓ Thinking tokens shown", repltheme.HighlightStyle)
	case "off":
		m.showThinking = false
		m.stream.handler.showThinking = false
		m.saveShowThinking(false)
		m.output.AddStyledLine("  ✓ Thinking tokens hidden", repltheme.HighlightStyle)
	default:
		if m.showThinking {
			m.output.AddStyledLine("  Thinking tokens: shown (use /show-thinking off to hide)", repltheme.HighlightStyle)
		} else {
			m.output.AddStyledLine("  Thinking tokens: hidden (use /show-thinking on to show)", repltheme.HighlightStyle)
		}
	}

	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) handleMCPCommand(input string) (replModel, tea.Cmd) {
	args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(input, replcommands.MCP)))
	if m.ctx == nil || m.ctx.mcp == nil {
		m.output.AddStyledLine("  MCP is not configured.", repltheme.MutedStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if len(args) == 0 || args[0] == "status" {
		m.addMCPStatus(m.ctx.mcp.Servers())
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if args[0] != "connect" || len(args) != 2 {
		m.output.AddStyledLine("  "+mcpUsage, repltheme.UsageHintStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	server, err := m.resolveMCPServer(args[1])
	if err != nil {
		m.output.AddError(err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	m.startLoading("Connecting to MCP server " + server + "...")
	m.adjustTextareaHeight()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m, tea.Batch(m.loading.spinner.Tick, m.connectMCPCmd(server))
}

func (m *replModel) addMCPStatus(statuses []keenmcp.ServerStatus) {
	if len(statuses) == 0 {
		m.output.AddStyledLine("  No MCP servers configured.", repltheme.MutedStyle)
		m.output.AddEmptyLine()
		return
	}
	rows := make([][]string, 0, len(statuses))
	states := make([]keenmcp.ServerState, 0, len(statuses))
	for _, status := range statuses {
		detail := status.LastError
		if status.State == keenmcp.StateConnected {
			detail = strings.TrimSpace(status.NegotiatedServerName + " " + status.NegotiatedServerVersion)
			if detail == "" {
				detail = "tools: " + strconv.Itoa(status.ToolCount)
			}
		}
		rows = append(rows, []string{status.Name, mcpStatusLabel(status.State), status.AuthType, detail})
		states = append(states, status.State)
	}

	nameWidth := maxColumnWidth("Server", rows, 0)
	statusWidth := maxColumnWidth("Status", rows, 1)
	authWidth := maxColumnWidth("Auth", rows, 2)
	m.addCommandTable([]string{"Server", "Status", "Auth", "Detail"}, rows, func(row, col int, style lipgloss.Style) lipgloss.Style {
		if col == 0 {
			style = style.Width(nameWidth + commandTableCellPadding)
			if row != table.HeaderRow {
				style = style.Inherit(repltheme.PrimaryBoldStyle)
			}
		}
		if col == 1 {
			style = style.Width(statusWidth + commandTableCellPadding)
			if row != table.HeaderRow {
				switch states[row] {
				case keenmcp.StateConnected:
					style = style.Inherit(repltheme.HighlightStyle)
				case keenmcp.StateDisconnected:
					style = style.Inherit(repltheme.ErrorStyle)
				default:
					style = style.Inherit(repltheme.AccentStyle)
				}
			}
		}
		if col == 2 {
			style = style.Width(authWidth + commandTableCellPadding)
		}
		if col == 3 && row != table.HeaderRow {
			style = style.Inherit(repltheme.HelpDescStyle)
		}
		return style
	})
	m.output.AddEmptyLine()
}

func mcpStatusLabel(state keenmcp.ServerState) string {
	switch state {
	case keenmcp.StateConnected:
		return "✓ connected"
	case keenmcp.StateConnecting:
		return "• connecting"
	default:
		return "✗ " + string(state)
	}
}

func (m *replModel) resolveMCPServer(name string) (string, error) {
	servers := m.ctx.mcp.Servers()
	for _, server := range servers {
		if server.Name == name {
			return name, nil
		}
	}
	var matches []string
	for _, server := range servers {
		tools, err := m.ctx.mcp.ListTools(context.Background(), server.Name)
		if err != nil {
			continue
		}
		for _, tool := range tools {
			if tool.Name == name {
				matches = append(matches, server.Name)
				break
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("MCP tool %q is provided by multiple servers: %s", name, strings.Join(matches, ", "))
	}
	return "", fmt.Errorf("unknown MCP server or tool: %s", name)
}

func (m replModel) connectMCPCmd(server string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mcpConnectTimeout)
		defer cancel()
		redirectURL := keenmcp.DefaultOAuthRedirectURL
		fetcher := keenmcp.NewBrowserOAuthCodeFetcher(redirectURL)
		err := m.ctx.mcp.Refresh(
			ctx,
			server,
			keenmcp.WithRefreshConnectTimeout(mcpConnectTimeout),
			keenmcp.WithRefreshOAuthRedirectURL(redirectURL),
			keenmcp.WithRefreshOAuthAuthorizationCodeFetcher(fetcher),
			keenmcp.WithRefreshOAuthForceReauth(true),
		)
		return mcpConnectDoneMsg{Server: server, Status: m.ctx.mcp.Status(server), Err: err}
	}
}

func (m *replModel) handleToolPermissionCommand(input, command string, allow bool) replModel {
	args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(input, command)))
	toolNames := m.registeredToolNames()

	if len(args) == 0 {
		m.output.AddStyledLine("  Usage: "+command+" <tool_names...>", repltheme.UsageHintStyle)
		m.output.AddStyledLine("  Available tools: "+strings.Join(toolNames, ", "), repltheme.MutedStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	for _, name := range args {
		if !slices.Contains(toolNames, name) {
			m.output.AddError("Unknown tool: "+name, repltheme.ErrorStyle)
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m
		}
	}

	for _, name := range args {
		if allow {
			m.projectPerms.Allow[name] = struct{}{}
		} else {
			delete(m.projectPerms.Allow, name)
		}
	}

	if err := config.SaveProjectPermissions(m.ctx.workingDir, m.projectPerms); err != nil {
		m.output.AddError("Failed to save permissions: "+err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	verb := "Allowed"
	if !allow {
		verb = "Reset"
	}
	m.output.AddStyledLine("  ✓ "+verb+": "+strings.Join(args, ", "), repltheme.HighlightStyle)
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) registeredToolNames() []string {
	all := m.appState.GetToolRegistry().All()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.Name())
	}
	return names
}

func (m *replModel) handleSkillsCommand(input string) replModel {
	args := parseSkillArgs(input)
	discovery := m.appState.GetSkills()
	cfg := m.appState.GetSkillsConfig()

	for _, warning := range discovery.Warnings {
		m.output.AddError(warning, repltheme.ErrorStyle)
	}

	if len(args) == 0 || (len(args) == 1 && (args[0] == "list" || args[0] == "status")) {
		m.output.AddStyledLine("  Available Skills\n", repltheme.MutedStyle.Bold(true))
		if len(discovery.Skills) == 0 {
			m.output.AddStyledLine("    No skills found.", repltheme.MutedStyle)
		} else {
			m.addSkillTable(discovery.Skills, cfg)
		}
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	if len(args) == 1 && args[0] == "reload" {
		discovery = m.appState.ReloadSkills()
		for _, warning := range discovery.Warnings {
			m.output.AddError(warning, repltheme.ErrorStyle)
		}
		m.output.AddStyledLine("  ✓ Skills reloaded", repltheme.HighlightStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	if len(args) != 2 || (args[0] != "enable" && args[0] != "disable") {
		m.output.AddError(skillsUsage, repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	name := args[1]
	if _, ok := skills.Find(discovery.Skills, name); !ok {
		m.output.AddError("Skill not found: "+name, repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	status := skills.StatusDisabled
	if args[0] == "enable" {
		status = skills.StatusEnabled
	}
	if err := m.appState.SetSkillStatus(name, status); err != nil {
		m.output.AddError("Failed to save skills config: "+err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	if status == skills.StatusEnabled {
		m.output.AddStyledLine("  ✓ Skill \""+name+"\" enabled", repltheme.HighlightStyle)
	} else {
		m.output.AddStyledLine("  ✓ Skill \""+name+"\" disabled", repltheme.HighlightStyle)
	}
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func parseSkillArgs(input string) []string {
	return strings.Fields(strings.TrimSpace(strings.TrimPrefix(input, replcommands.Skills)))
}

func (m *replModel) handleSubagentsCommand(input string) replModel {
	args := parseSubagentArgs(input)
	if len(args) > 1 || len(args) == 1 && args[0] != "list" {
		m.output.AddError(subagentsUsage, repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}

	discovery := m.appState.GetSubagents()
	for _, warning := range discovery.Warnings {
		m.output.AddError(warning, repltheme.ErrorStyle)
	}

	m.output.AddStyledLine("  Available Subagents\n", repltheme.MutedStyle.Bold(true))
	visible := visibleSubagents(discovery.Profiles)
	if len(visible) == 0 {
		m.output.AddStyledLine("    No subagents found.", repltheme.MutedStyle)
	} else {
		m.addSubagentTable(visible)
	}
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func parseSubagentArgs(input string) []string {
	return strings.Fields(strings.TrimSpace(strings.TrimPrefix(input, replcommands.Subagents)))
}

func visibleSubagents(profiles []subagents.Profile) []subagents.Profile {
	items := make([]subagents.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if !profile.Hidden {
			items = append(items, profile)
		}
	}
	return items
}

func (m *replModel) addSubagentTable(profiles []subagents.Profile) {
	nameWidth := max(maxSubagentNameWidth(profiles), len("Subagent"))
	rows := make([][]string, 0, len(profiles))
	for _, profile := range profiles {
		rows = append(rows, []string{profile.Name, truncateSkillDescription(profile.Description)})
	}

	m.addCommandTable([]string{"Subagent", "Description"}, rows, func(row, col int, style lipgloss.Style) lipgloss.Style {
		if col == 0 {
			style = style.Width(nameWidth + commandTableCellPadding)
			if row != table.HeaderRow {
				style = style.Inherit(repltheme.PrimaryBoldStyle)
			}
		}
		if col == 1 && row != table.HeaderRow {
			style = style.Inherit(repltheme.HelpDescStyle)
		}
		return style
	})
}

func maxSubagentNameWidth(profiles []subagents.Profile) int {
	width := 0
	for _, profile := range profiles {
		width = max(width, lipgloss.Width(profile.Name))
	}
	return width
}

func (m *replModel) addSkillTable(skillList []skills.Skill, cfg skills.Config) {
	nameWidth := max(maxSkillNameWidth(skillList), len("Skill"))
	statusWidth := max(lipgloss.Width("Status"), lipgloss.Width("✗ disabled"))

	rows := make([][]string, 0, len(skillList))
	for _, skill := range skillList {
		status := "✓ enabled"
		if !cfg.Enabled(skill.Name) {
			status = "✗ disabled"
		}
		rows = append(rows, []string{skill.Name, status, truncateSkillDescription(skill.Description)})
	}

	disabledStatusStyle := repltheme.AccentStyle

	m.addCommandTable([]string{"Skill", "Status", "Description"}, rows, func(row, col int, style lipgloss.Style) lipgloss.Style {
		if col == 0 {
			style = style.Width(nameWidth + commandTableCellPadding)
			if row != table.HeaderRow {
				style = style.Inherit(repltheme.PrimaryBoldStyle)
			}
		}
		if col == 1 {
			style = style.Width(statusWidth + commandTableCellPadding)
			if row != table.HeaderRow {
				if strings.HasPrefix(rows[row][col], "✓") {
					style = style.Inherit(repltheme.HighlightStyle)
				} else {
					style = style.Inherit(disabledStatusStyle)
				}
			}
		}
		if col == 2 && row != table.HeaderRow {
			style = style.Inherit(repltheme.HelpDescStyle)
		}
		return style
	})
}

const (
	commandTableLeftPadding   = "    "
	commandTableRightPadding  = 2
	commandTableCellPadding   = 2
	skillDescriptionWordLimit = 50
)

func (m *replModel) addCommandTable(headers []string, rows [][]string, styleFunc func(row, col int, style lipgloss.Style) lipgloss.Style) {
	tableWidth := m.width - lipgloss.Width(commandTableLeftPadding) - commandTableRightPadding
	if tableWidth < 1 {
		tableWidth = m.viewport.Width() - lipgloss.Width(commandTableLeftPadding) - commandTableRightPadding
	}
	tableWidth = max(1, tableWidth)

	rendered := table.New().
		Headers(headers...).
		Rows(rows...).
		Width(tableWidth).
		Wrap(true).
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderRow(false).
		BorderHeader(true).
		BorderStyle(repltheme.RuleStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			style := lipgloss.NewStyle().PaddingRight(commandTableCellPadding)
			if row == table.HeaderRow {
				style = style.Inherit(repltheme.MutedStyle.Bold(true))
			}
			if styleFunc != nil {
				style = styleFunc(row, col, style)
			}
			return style
		}).
		Render()
	for line := range strings.SplitSeq(rendered, "\n") {
		m.output.AddLine(commandTableLeftPadding + line)
	}
}

func maxSkillNameWidth(skillList []skills.Skill) int {
	width := 0
	for _, skill := range skillList {
		width = max(width, lipgloss.Width(skill.Name))
	}
	return width
}

func truncateSkillDescription(description string) string {
	words := strings.Fields(description)
	if len(words) <= skillDescriptionWordLimit {
		return description
	}
	return strings.Join(words[:skillDescriptionWordLimit], " ") + "..."
}

func maxColumnWidth(header string, rows [][]string, col int) int {
	width := lipgloss.Width(header)
	for _, row := range rows {
		if col < len(row) {
			width = max(width, lipgloss.Width(row[col]))
		}
	}
	return width
}

func (m *replModel) saveShowThinking(val bool) {
	if m.ctx == nil || m.ctx.globalCfg == nil || m.ctx.loader == nil {
		return
	}
	m.ctx.globalCfg.ShowThinking = &val
	_ = m.ctx.loader.Save(m.ctx.globalCfg)
}

func (m *replModel) handleBtwCommand(input string) (replModel, tea.Cmd) {
	question := strings.TrimSpace(strings.TrimPrefix(input, replcommands.Btw))
	if question == "" {
		m.output.AddStyledLine("  Usage: /btw <question>", repltheme.UsageHintStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if !m.appState.IsClientReady(m.ctx.cfg) {
		m.output.AddError("LLM client not initialized. Use /model to configure.", repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if m.btw.streamCancel != nil {
		m.btw.streamCancel()
	}
	m.flushBtwToOutput()

	ctx, cancel := context.WithCancel(context.Background())
	m.btw.streamCancel = cancel

	eventCh, err := m.appState.StreamBtw(ctx, question)
	if err != nil {
		cancel()
		m.btw.streamCancel = nil
		m.output.AddError(err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	m.btw.lines = nil
	m.btw.question = question
	m.btw.streamHandler.Start(eventCh, nextLoadingText())
	m.btw.showSpinner = true
	m.userScrolled = false
	m.updateViewportContent()
	m.viewport.GotoBottom()

	return *m, tea.Batch(m.btw.spinner.Tick, waitForBtwEvent(eventCh))
}

func (m *replModel) handleBangCommand(input string) (replModel, tea.Cmd) {
	command := strings.TrimSpace(strings.TrimPrefix(input, "!"))
	if command == "" {
		m.output.AddStyledLine("  Usage: !<command>", repltheme.UsageHintStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	topRule, _ := renderRulesWithChip(m.width, repltheme.RuleStyle, "shell", repltheme.ShellChipOutputStyle)

	m.output.AddEmptyLine()
	m.output.AddLine(topRule)
	m.output.AddStyledLine("  $ "+command, repltheme.BashCommandStyle)
	m.updateViewportContent()
	m.viewport.GotoBottom()

	ctx, cancel := context.WithTimeout(context.Background(), bangTimeout)
	m.bang = bangState{
		active: true,
		events: startBangCommand(ctx, command),
		cancel: cancel,
	}

	m.startLoading(nextLoadingText())
	m.adjustTextareaHeight()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m, tea.Batch(m.loading.spinner.Tick, waitForBangEvent(m.bang.events))
}

func (m *replModel) handleAdversaryCommand(input string) (replModel, tea.Cmd) {
	arg := strings.TrimSpace(strings.TrimPrefix(input, replcommands.Adversary))

	if arg == "model" {
		return m.startAdversaryModelSelection(), nil
	}

	if m.ctx.globalCfg.AdversaryProvider == "" || m.ctx.globalCfg.AdversaryModel == "" {
		m.output.AddStyledLine("  Run `/adversary model` to configure an adversary model", repltheme.HighlightStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if !m.appState.IsAdversaryClientReady() {
		if err := m.buildAdversaryClient(); err != nil {
			m.output.AddError("Failed to initialize adversary client: "+err.Error(), repltheme.ErrorStyle)
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil
		}
	}

	m.cancelAdversaryStream()
	m.flushAdversaryToOutput()

	ctx, cancel := context.WithCancel(context.Background())
	m.adversary.streamCancel = cancel

	eventCh, err := m.appState.StreamAdversary(ctx, arg)
	if err != nil {
		cancel()
		m.adversary.streamCancel = nil
		m.output.AddError(err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}
	if eventCh == nil {
		cancel()
		m.adversary.streamCancel = nil
		m.output.AddError("adversary stream unavailable", repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	m.adversary.lines = nil
	m.adversary.focus = arg
	m.adversary.streamHandler.Start(eventCh, nextLoadingText())
	m.adversary.showSpinner = true
	m.userScrolled = false
	m.updateViewportContent()
	m.viewport.GotoBottom()

	return *m, tea.Batch(m.adversary.spinner.Tick, waitForAdversaryEvent(eventCh))
}

func (m *replModel) startAdversaryModelSelection() replModel {
	savedProvider := m.ctx.globalCfg.ActiveProvider
	savedModel := m.ctx.globalCfg.ActiveModel
	savedThinkingEffort := m.ctx.globalCfg.ThinkingEffort

	onComplete := func(provider, model, apiKey string) error {
		m.ctx.globalCfg.AdversaryProvider = provider
		m.ctx.globalCfg.AdversaryModel = model
		m.ctx.globalCfg.ActiveProvider = savedProvider
		m.ctx.globalCfg.ActiveModel = savedModel
		m.ctx.globalCfg.ThinkingEffort = savedThinkingEffort
		if err := m.ctx.loader.Save(m.ctx.globalCfg); err != nil {
			return err
		}
		return m.buildAdversaryClient()
	}

	adversaryResolved, _ := config.ResolveAdversary(m.ctx.globalCfg)
	if adversaryResolved == nil {
		adversaryResolved = &config.ResolvedConfig{}
	}
	m.adversary.modelSelection = replwidgets.New(
		m.ctx.registry,
		m.ctx.globalCfg,
		m.ctx.loader,
		adversaryResolved,
		onComplete,
	)
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) handleLogoutCommand() replModel {
	if m.ctx == nil || m.ctx.cfg == nil || m.ctx.cfg.Provider == "" {
		m.output.AddError("No provider is configured.", repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}
	if config.AuthModeForProvider(m.ctx.cfg.Provider) != config.AuthModeOAuth {
		m.output.AddError("Current provider does not use OAuth.", repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}
	if err := keenauth.NewStore().Remove(m.ctx.cfg.Provider); err != nil {
		m.output.AddError("Failed to remove OAuth credentials: "+err.Error(), repltheme.ErrorStyle)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}
	m.appState.UpdateClient(nil)
	m.output.AddStyledLine("  ✓ Signed out of "+m.ctx.cfg.Provider, repltheme.HighlightStyle)
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) handleClearCommand() replModel {
	currentMode := m.currentMode()
	m.appState.ClearMessages()
	m.appState.ResetClientState()
	m.appState.ClearContextMetrics()
	m.contextStatus.ResetTotals()
	m.appState.SetMode(currentMode)
	m.sessions.resetSession()
	if m.permissionRequester != nil {
		m.permissionRequester.ResetSessionPermissions()
	}
	m.history.Reset()
	m.loading.lastTurnElapsedMsg = ""
	m.adjustTextareaHeight()

	newOutput := reploutput.NewOutputBuilder(m.width, m.ctx.workingDir)
	initialLines := buildInitialScreen(m.ctx, m.lastSession, m.width)
	for _, line := range initialLines {
		newOutput.AddLine(line)
	}
	if currentMode != llm.ModeBuild {
		newOutput.AddStyledLine("  ✓ Mode restored: "+string(currentMode), repltheme.HighlightStyle)
	}
	newOutput.AddStyledLine("  ✓ New session started", repltheme.CompactionSuccessStyle)
	newOutput.AddEmptyLine()
	m.output = newOutput

	m.refreshContextStatus()
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) helpWidth() int {
	width := m.width
	if width <= 0 {
		width = m.viewport.Width()
	}
	if width <= 0 {
		width = 80
	}
	return width
}

func getHelpText(width int) string {
	const (
		leftPadding  = "  "
		rightPadding = 2
		colGap       = "  "
	)

	cmdWidth := max(maxCommandNameWidth(replcommands.All), len("Command"))
	descWidth := width - lipgloss.Width(leftPadding) - cmdWidth - lipgloss.Width(colGap) - rightPadding
	if descWidth < 1 {
		descWidth = 1
	}

	var lines []string
	lines = append(lines, leftPadding+repltheme.TitleStyle.Render("Available Commands"))
	lines = append(lines, "")
	for _, c := range replcommands.All {
		descriptionLines := strings.Split(lipgloss.NewStyle().Width(descWidth).Render(c.Description), "\n")
		if len(descriptionLines) == 0 {
			descriptionLines = []string{""}
		}

		lines = append(lines, leftPadding+repltheme.HelpCmdStyle.Width(cmdWidth).Render(c.Name)+colGap+repltheme.HelpDescStyle.Render(descriptionLines[0]))
		continuation := leftPadding + strings.Repeat(" ", cmdWidth) + colGap
		for _, line := range descriptionLines[1:] {
			lines = append(lines, continuation+repltheme.HelpDescStyle.Render(line))
		}
	}

	return strings.Join(lines, "\n")
}

func maxCommandNameWidth(commands []replcommands.SlashCommand) int {
	width := 0
	for _, command := range commands {
		width = max(width, lipgloss.Width(command.Name))
	}
	return width
}

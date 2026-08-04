package repl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	replappstate "github.com/user/keen-code/internal/cli/repl/appstate"
	reploutput "github.com/user/keen-code/internal/cli/repl/output"
	replpermissions "github.com/user/keen-code/internal/cli/repl/permissions"
	repltheme "github.com/user/keen-code/internal/cli/repl/theme"
	repltooling "github.com/user/keen-code/internal/cli/repl/tooling"
	replwidgets "github.com/user/keen-code/internal/cli/repl/widgets"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/llm"
	"github.com/user/keen-code/internal/providers"
	"github.com/user/keen-code/internal/session"
	"github.com/user/keen-code/internal/tools"
)

func styleColorPrefix(style lipgloss.Style) string {
	rendered := style.Render("x")
	if idx := strings.Index(rendered, "x"); idx > 0 {
		return rendered[:idx]
	}
	return rendered
}

func newTestModel() replModel {
	ta := textarea.New()
	ta.Focus()
	ta.SetWidth(80)
	ta.DynamicHeight = true
	ta.MinHeight = inputMinHeight
	ta.MaxHeight = inputMaxHeight
	ta.MaxContentHeight = inputMaxContentHeight
	ta.SetHeight(inputMinHeight)
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	return replModel{
		textarea:            ta,
		viewport:            vp,
		ctx:                 &replContext{cfg: &config.ResolvedConfig{}},
		mode:                llm.ModeBuild,
		appState:            replappstate.New(nil, ""),
		output:              reploutput.NewOutputBuilder(80, ""),
		stream:              streamState{handler: NewStreamHandler(nil)},
		permissionRequester: replpermissions.NewRequester(nil),
		projectPerms:        config.NewProjectPermissions(),
		diffEmitter:         repltooling.NewDiffEmitter(),
		sessions:            newReplSessionState(""),
		loading:             loadingState{spinner: spinner.New()},
		btw:                 btwState{spinner: spinner.New()},
		width:               80,
		height:              30,
		showThinking:        true,
	}
}

func TestFormatTurnElapsed(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "zero", in: 0, want: "0s"},
		{name: "seconds only", in: 6 * time.Second, want: "6s"},
		{name: "under minute with ms", in: 9*time.Second + 900*time.Millisecond, want: "9s"},
		{name: "minutes and seconds", in: time.Minute + 23*time.Second, want: "1m 23s"},
		{name: "exact minute", in: 2 * time.Minute, want: "2m"},
		{name: "negative", in: -time.Second, want: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTurnElapsed(tt.in); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
func scrollViewportAwayFromBottom(t *testing.T, m *replModel) int {
	t.Helper()

	m.viewport.SetHeight(6)
	for range 40 {
		m.output.AddLine("existing output")
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()
	if m.viewport.AtTop() {
		t.Fatal("expected test viewport to be scrollable")
	}
	m.viewport.ScrollUp(4)
	if m.viewport.AtBottom() {
		t.Fatal("expected test viewport to be above bottom")
	}
	m.userScrolled = true
	return m.viewport.YOffset()
}

func TestUpdate_InlinePermission_AllowsToolStartEvent(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	req := &replpermissions.Request{
		RequestID:    "1",
		ToolName:     "read_file",
		Path:         "../foo.txt",
		ResolvedPath: "/tmp/foo.txt",
		Status:       replpermissions.StatusPending,
		ResponseChan: make(chan bool, 1),
	}
	sh.HandlePermissionRequest(req)

	m := replModel{
		stream:  streamState{handler: sh},
		loading: loadingState{showSpinner: true},
		width:   80,
		output:  reploutput.NewOutputBuilder(80, ""),
	}

	toolCall := &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "../foo.txt"}}
	updatedModel, cmd := m.Update(llmToolStartMsg{toolCall: toolCall})

	updated, ok := updatedModel.(*replModel)
	if !ok {
		t.Fatalf("expected *replModel, got %T", updatedModel)
	}

	if !updated.loading.showSpinner {
		t.Error("expected showSpinner to remain true after tool start while permission is pending")
	}

	if len(updated.output.GetLines()) != 0 {
		t.Errorf("expected no persisted output line for tool start, got %d", len(updated.output.GetLines()))
	}

	if cmd == nil {
		t.Error("expected non-nil cmd when handling tool start event")
	}
}

func TestAdjustTextareaHeight(t *testing.T) {
	m := newTestModel()
	m.adjustTextareaHeight()

	if m.textarea.Height() != inputMinHeight {
		t.Errorf("expected textarea height %d for empty input, got %d", inputMinHeight, m.textarea.Height())
	}
	expectedVPHeight := m.height - m.textarea.Height() - 5
	if m.viewport.Height() != expectedVPHeight {
		t.Errorf("expected viewport height %d, got %d", expectedVPHeight, m.viewport.Height())
	}

	m.textarea.SetValue("line1\nline2\nline3\nline4")
	m.adjustTextareaHeight()
	if m.textarea.Height() != 4 {
		t.Errorf("expected textarea height 4 for 4-line input, got %d", m.textarea.Height())
	}
	expectedVPHeight = m.height - m.textarea.Height() - 5
	if m.viewport.Height() != expectedVPHeight {
		t.Errorf("expected viewport height %d, got %d", expectedVPHeight, m.viewport.Height())
	}

	m.textarea.Reset()
	m.adjustTextareaHeight()
	if m.textarea.Height() != inputMinHeight {
		t.Errorf("expected textarea to shrink to %d after reset, got %d", inputMinHeight, m.textarea.Height())
	}
}

func TestActivateSkillInput(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\n# Demo\nargs=$ARGUMENTS"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	activated, ok := m.activateSkillInput("/demo thing")
	if !ok {
		t.Fatal("expected skill activation")
	}
	if !strings.Contains(activated, "[Activate skill: demo]") || !strings.Contains(activated, "# Demo") || !strings.Contains(activated, "args=thing") {
		t.Fatalf("unexpected activation message: %q", activated)
	}
}

func TestActivateSkillInput_UsesFrontmatterNameNotDir(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "any-dir")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: real-name\ndescription: Demo skill\n---\nbody=$ARGUMENTS"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)

	if _, ok := m.activateSkillInput("/any-dir foo"); ok {
		t.Fatal("expected /<dirname> to NOT activate")
	}

	activated, ok := m.activateSkillInput("/real-name foo")
	if !ok {
		t.Fatal("expected /<frontmatter-name> to activate")
	}
	if !strings.Contains(activated, "[Activate skill: real-name]") || !strings.Contains(activated, "body=foo") {
		t.Fatalf("unexpected activation message: %q", activated)
	}
}

func TestUpdateNormalMode_WindowResize(t *testing.T) {
	m := newTestModel()

	resizeMsg := tea.WindowSizeMsg{Width: 100, Height: 40}
	newM, cmd := m.updateNormalMode(resizeMsg)

	if newM.width != 100 {
		t.Errorf("expected width 100, got %d", newM.width)
	}
	if newM.height != 40 {
		t.Errorf("expected height 40, got %d", newM.height)
	}
	if cmd != nil {
		t.Error("expected nil cmd for window resize")
	}
}

func TestUpdateNormalMode_WindowResizeWhileModelSelectionActive(t *testing.T) {
	m := newTestModel()
	m.modelSelection = &replwidgets.Model{}

	resizeMsg := tea.WindowSizeMsg{Width: 100, Height: 40}
	newM, cmd := m.updateNormalMode(resizeMsg)

	if newM.width != 100 {
		t.Errorf("expected width 100, got %d", newM.width)
	}
	if newM.height != 40 {
		t.Errorf("expected height 40, got %d", newM.height)
	}
	if newM.viewport.Height() != 34 {
		t.Errorf("expected viewport height 34, got %d", newM.viewport.Height())
	}
	if cmd != nil {
		t.Error("expected nil cmd for window resize")
	}
}

func TestUpdateViewportContent_UsesViewportWidthWhenModelStartsWithoutResize(t *testing.T) {
	m := newTestModel()
	m.width = 0
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.stream.handler.HandleReasoningChunk("thinking")

	m.updateViewportContent()

	content := m.viewport.View()
	if strings.Contains(content, "  t\n  h\n  i") {
		t.Fatalf("expected reasoning to use viewport width fallback, got %q", content)
	}
	if !strings.Contains(content, "thinking") {
		t.Fatalf("expected reasoning content to be rendered, got %q", content)
	}
}

func TestInitialModel_DimsBlurredPromptGlyph(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := initialModel(&replContext{version: "test", workingDir: t.TempDir(), cfg: &config.ResolvedConfig{}}, nil, false)
	styles := m.textarea.Styles()

	got := styles.Blurred.Prompt.Render(" ▶ ")
	want := repltheme.InputRuleBlurredStyle.Render(" ▶ ")
	if got != want {
		t.Fatalf("expected blurred prompt glyph to use blurred input style, got %q want %q", got, want)
	}
}

func TestInitialModel_PlanModeSetsPromptStyle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := initialModel(&replContext{version: "test", workingDir: t.TempDir(), cfg: &config.ResolvedConfig{}}, nil, false)
	m.setMode(llm.ModePlan)
	view := m.View().Content
	if !strings.Contains(view, repltheme.ModePlanChipStyle.Render("plan")) {
		t.Fatalf("expected plan mode chip in view, got %q", view)
	}
	if !strings.Contains(view, styleColorPrefix(repltheme.InputRulePlanStyle)) {
		t.Fatalf("expected plan mode prompt to use secondary color, got %q", view)
	}
}

func TestRenderInputArea_UsesViewportWidthRules(t *testing.T) {
	focusedWide := renderInputArea("▶ hello", 80, true, false, false, false, llm.ModeBuild)
	blurredWide := renderInputArea("▶ hello", 80, false, false, false, false, llm.ModeBuild)
	if focusedWide == blurredWide {
		t.Fatal("expected focused and blurred input areas to render differently")
	}

	wide := focusedWide
	wideLines := strings.Split(strings.TrimRight(wide, "\n"), "\n")
	if len(wideLines) != 3 {
		t.Fatalf("expected 3 input-area lines, got %v", wideLines)
	}
	if !strings.Contains(wideLines[0], "─") || !strings.Contains(wideLines[2], "─") {
		t.Fatalf("expected top and bottom input rules, got %q", wide)
	}
	if wideRuleWidth := lipgloss.Width(wideLines[0]); wideRuleWidth != 80 {
		t.Fatalf("expected wide input rules to match viewport width, got width %d", wideRuleWidth)
	}

	narrow := renderInputArea("▶ hi", 24, true, false, false, false, llm.ModeBuild)
	narrowLines := strings.Split(strings.TrimRight(narrow, "\n"), "\n")
	if len(narrowLines) != 3 {
		t.Fatalf("expected 3 narrow input-area lines, got %v", narrowLines)
	}
	if narrowRuleWidth := lipgloss.Width(narrowLines[0]); narrowRuleWidth != 24 {
		t.Fatalf("expected narrow input rules to match viewport width, got width %d", narrowRuleWidth)
	}
}

func TestFormatModelSelectionCard_UsesViewportWidthRules(t *testing.T) {
	card := formatModelSelectionCard(&replwidgets.Model{}, 24)
	lines := strings.Split(strings.TrimRight(card, "\n"), "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		if w := lipgloss.Width(line); w > 24 {
			t.Fatalf("line exceeds viewport width (%d > %d): %q", w, 24, line)
		}
		if strings.TrimSpace(line) != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) < 2 {
		t.Fatalf("expected ruled model selection output, got %v", nonEmpty)
	}
	if !strings.Contains(nonEmpty[0], "─") || !strings.Contains(nonEmpty[len(nonEmpty)-1], "─") {
		t.Fatalf("expected top and bottom rules, got %q", card)
	}
	if !strings.Contains(nonEmpty[0], styleColorPrefix(repltheme.ModelSelectionRuleStyle)) {
		t.Fatalf("expected model selection rule style, got %q", nonEmpty[0])
	}
	if ruleWidth := lipgloss.Width(nonEmpty[0]); ruleWidth != 24 {
		t.Fatalf("expected rules to match viewport width, got width %d", ruleWidth)
	}
	if strings.TrimSpace(lines[2]) != "" {
		t.Fatalf("expected blank line after top rule, got %q", lines[2])
	}
	if strings.TrimSpace(lines[len(lines)-2]) != "" {
		t.Fatalf("expected blank line before bottom rule, got %q", lines[len(lines)-2])
	}
}

func TestFormatModelSelectionCard_WrapsOAuthURLWithinViewportWidth(t *testing.T) {
	url := "https://auth.openai.com/oauth/authorize?" + strings.Repeat("abcdef", 16)
	card := formatModelSelectionCard(&replwidgets.Model{
		Step:        replwidgets.StepOAuth,
		OAuthStatus: "Complete authentication in your browser.",
		OAuthURL:    url,
	}, 40)

	if !strings.Contains(card, "https://auth.openai.com") {
		t.Fatalf("expected OAuth URL in card, got %q", card)
	}
	for _, line := range strings.Split(strings.TrimRight(card, "\n"), "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Fatalf("line exceeds viewport width (%d > %d): %q", w, 40, line)
		}
	}
}

func TestUpdate_RoutesToNormalMode(t *testing.T) {
	m := newTestModel()

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	updated := result.(*replModel)

	if updated.width != 100 {
		t.Errorf("expected width 100, got %d", updated.width)
	}
}

func TestUpdate_CleanupFailureShowsGenericMessage(t *testing.T) {
	m := newTestModel()
	m.startLoading("Cleaning up...")

	updated, _ := m.updateNormalMode(cleanupDoneMsg{err: errors.New("disk full")})
	if updated.loading.showSpinner {
		t.Fatal("expected cleanup spinner to stop")
	}
	output := updated.output.Join()
	if !strings.Contains(output, "Failed, check ~/.keen/ to manually cleanup.") {
		t.Fatalf("unexpected cleanup failure output: %q", output)
	}
	if strings.Contains(output, "disk full") {
		t.Fatalf("cleanup error leaked to UI: %q", output)
	}
}

func TestUpdate_RoutesToPermissionHandling(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := &replpermissions.Request{
		RequestID:    "1",
		ToolName:     "read_file",
		Path:         "foo.txt",
		ResolvedPath: "/resolved/foo.txt",
		Status:       replpermissions.StatusPending,
		ResponseChan: make(chan bool, 1),
	}
	m.stream.handler.HandlePermissionRequest(req)

	if !m.stream.handler.HasPendingPermission() {
		t.Fatal("expected pending permission")
	}

	// Pressing down should move the cursor down
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	updated := result.(*replModel)

	if !updated.stream.handler.HasPendingPermission() {
		t.Error("expected pending permission to remain after down key")
	}
}

func TestHandleLLMStreamMsg_UnknownMsg(t *testing.T) {
	m := newTestModel()
	_, _, handled := m.handleLLMStreamMsg(tea.WindowSizeMsg{})

	if handled {
		t.Error("expected unknown msg to not be handled")
	}
}

func TestHandleLLMStreamMsg_RoutesChunk(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.loading.showSpinner = true

	newM, _, handled := m.handleLLMStreamMsg(llmChunkMsg("hello"))

	if !handled {
		t.Error("expected chunk msg to be handled")
	}
	if !newM.loading.showSpinner {
		t.Error("expected showSpinner to remain true after chunk")
	}
}

func TestHandleLLMStreamMsg_StreamRenderFlushesWithoutMainStream(t *testing.T) {
	m := newTestModel()
	m.stream.renderPending = true
	m.btw.showSpinner = true
	m.btw.question = "why?"
	m.btw.lines = []string{"thinking"}

	newM, cmd, handled := m.handleLLMStreamMsg(streamRenderMsg{})

	if !handled {
		t.Fatal("expected stream render msg to be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for stream render msg")
	}
	if newM.stream.renderPending {
		t.Fatal("expected pending stream render to be flushed")
	}
}

func TestUpdateNormalMode_PermissionReadyRendersImmediately(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.loading.showSpinner = true

	req := &replpermissions.Request{
		RequestID:    "1",
		ToolName:     "read_file",
		Path:         "../foo.txt",
		ResolvedPath: "/tmp/foo.txt",
		Status:       replpermissions.StatusPending,
		ResponseChan: make(chan bool, 1),
	}

	newM, cmd := m.updateNormalMode(permissionReadyMsg{req: req})

	if !newM.stream.handler.HasPendingPermission() {
		t.Fatal("expected pending permission to be rendered immediately")
	}
	if !newM.loading.showSpinner {
		t.Fatal("expected spinner to remain active when permission prompt appears")
	}
	if cmd == nil {
		t.Fatal("expected async waiter to be re-armed")
	}
}

func TestUpdateNormalMode_PermissionReadyPreservesUserScroll(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.loading.showSpinner = true
	offset := scrollViewportAwayFromBottom(t, &m)

	req := &replpermissions.Request{
		RequestID:    "1",
		ToolName:     "read_file",
		Path:         "../foo.txt",
		ResolvedPath: "/tmp/foo.txt",
		Status:       replpermissions.StatusPending,
		ResponseChan: make(chan bool, 1),
	}

	newM, _ := m.updateNormalMode(permissionReadyMsg{req: req})

	if got := newM.viewport.YOffset(); got != offset {
		t.Fatalf("expected permission prompt to preserve scroll offset %d, got %d", offset, got)
	}
}

func TestUpdateNormalMode_DiffReadyRendersImmediately(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	done := make(chan struct{})
	req := repltooling.DiffRequest{
		Lines: []tools.EditDiffLine{
			{Kind: tools.DiffLineAdded, Content: "hello", NewLineNum: 1},
		},
		Done: done,
	}

	newM, cmd := m.updateNormalMode(diffReadyMsg{req: req})

	if len(newM.stream.handler.segments) != 1 || newM.stream.handler.segments[0].kind != segmentDiff {
		t.Fatal("expected diff segment to be rendered immediately")
	}
	select {
	case <-done:
	default:
		t.Fatal("expected diff emitter to be unblocked immediately")
	}
	if cmd == nil {
		t.Fatal("expected async waiter to be re-armed")
	}
}

func TestUpdateNormalMode_DiffReadyPreservesUserScroll(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	offset := scrollViewportAwayFromBottom(t, &m)

	done := make(chan struct{})
	req := repltooling.DiffRequest{
		Lines: []tools.EditDiffLine{
			{Kind: tools.DiffLineAdded, Content: "hello", NewLineNum: 1},
		},
		Done: done,
	}

	newM, _ := m.updateNormalMode(diffReadyMsg{req: req})

	if got := newM.viewport.YOffset(); got != offset {
		t.Fatalf("expected diff prompt to preserve scroll offset %d, got %d", offset, got)
	}
}

func TestHandleUpdateCheckMsg_PreservesUserScroll(t *testing.T) {
	m := newTestModel()
	offset := scrollViewportAwayFromBottom(t, &m)

	m.handleUpdateCheckMsg(updateCheckMsg{latest: "9.9.9"})

	if got := m.viewport.YOffset(); got != offset {
		t.Fatalf("expected update notice to preserve scroll offset %d, got %d", offset, got)
	}
}

func TestReplayLoadedSession_RebuildsOutputAndConversation(t *testing.T) {
	m := newTestModel()
	loaded := &session.LoadedSession{
		Events: []session.Event{
			{
				Kind:        session.KindUserMessage,
				UserMessage: &session.MessagePayload{Content: "hello"},
			},
			{
				Kind: session.KindAssistantTurn,
				AssistantTurn: &session.AssistantTurnPayload{
					Transcript: []session.TranscriptItem{
						{
							Kind:    session.TranscriptItemReasoning,
							Content: "thinking",
						},
						{
							Kind:    session.TranscriptItemText,
							Content: "world",
						},
					},
					Message: "world",
				},
			},
			{
				Kind: session.KindCompactionApplied,
				CompactionApplied: &session.CompactionAppliedPayload{
					Status: "Context compacted.",
					Transcript: []session.TranscriptItem{
						{
							Kind:    session.TranscriptItemText,
							Content: "summary",
						},
					},
					Messages: []llm.Message{
						{Role: llm.RoleUser, Content: "summary"},
					},
				},
			},
		},
	}

	m.replayLoadedSession(loaded)

	if !strings.Contains(m.output.Join(), "hello") {
		t.Fatalf("expected replayed user message, got %q", m.output.Join())
	}
	if !strings.Contains(m.output.Join(), "thinking") {
		t.Fatalf("expected replayed reasoning, got %q", m.output.Join())
	}
	if !strings.Contains(m.output.Join(), "world") {
		t.Fatalf("expected replayed assistant, got %q", m.output.Join())
	}
	if !strings.Contains(m.output.Join(), "summary") {
		t.Fatalf("expected replayed compaction transcript, got %q", m.output.Join())
	}

	messages := m.appState.GetMessages()
	if len(messages) != 1 || messages[0].Content != "summary" {
		t.Fatalf("expected compacted conversation state, got %#v", messages)
	}
}

func TestRenderInputArea_UsesSecondaryStyleForPlanMode(t *testing.T) {
	area := renderInputArea("▶ hello", 80, true, false, false, false, llm.ModePlan)
	lines := strings.Split(strings.TrimRight(area, "\n"), "\n")
	if len(lines) < 1 {
		t.Fatalf("expected at least 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], repltheme.ModePlanChipStyle.Render("plan")) {
		t.Fatalf("expected plan mode chip in top rule, got %q", lines[0])
	}
	if !strings.Contains(lines[0], styleColorPrefix(repltheme.PlanInputRuleStyle)) {
		t.Fatalf("expected plan mode rule to use secondary color, got %q", lines[0])
	}
}

func TestRenderInputArea_UsesPrimaryStyleForBuildMode(t *testing.T) {
	area := renderInputArea("▶ hello", 80, true, false, false, false, llm.ModeBuild)
	lines := strings.Split(strings.TrimRight(area, "\n"), "\n")
	if len(lines) < 1 {
		t.Fatalf("expected at least 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], repltheme.ModeBuildChipStyle.Render("build")) {
		t.Fatalf("expected build mode chip in top rule, got %q", lines[0])
	}
}

func TestRenderInputArea_UsesAccentStyleForBtw(t *testing.T) {
	area := renderInputArea("▶ hello", 80, true, false, true, false, llm.ModeBuild)
	lines := strings.Split(strings.TrimRight(area, "\n"), "\n")
	if len(lines) < 1 {
		t.Fatalf("expected at least 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], repltheme.BtwChipStyle.Render("btw")) {
		t.Fatalf("expected btw chip in top rule, got %q", lines[0])
	}
	if !strings.Contains(lines[0], styleColorPrefix(repltheme.BtwInputRuleStyle)) {
		t.Fatalf("expected btw rule to use btw color, got %q", lines[0])
	}
}

func TestRenderInputArea_UsesSecondaryStyleForAdversary(t *testing.T) {
	area := renderInputArea("▶ hello", 80, true, false, false, true, llm.ModeBuild)
	lines := strings.Split(strings.TrimRight(area, "\n"), "\n")
	if len(lines) < 1 {
		t.Fatalf("expected at least 1 line, got %v", lines)
	}
	if !strings.Contains(lines[0], repltheme.AdversaryChipStyle.Render("adversary")) {
		t.Fatalf("expected adversary chip in top rule, got %q", lines[0])
	}
	if !strings.Contains(lines[0], styleColorPrefix(repltheme.AdversaryInputRuleStyle)) {
		t.Fatalf("expected adversary rule to use adversary color, got %q", lines[0])
	}
}

func TestInputMetaView_SuggestsCompactionAtSeventyPercent(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.contextStatus = contextStatus{
		KnownWindow:   true,
		KnownTokens:   true,
		ContextWindow: 100,
		CurrentTokens: 70,
		Percent:       70,
	}

	lines := strings.Split(m.inputMetaView(), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "Try /compact") {
		t.Fatalf("expected compaction hint on location line, got %q", lines[0])
	}
}

func TestSpinnerHeight_IncludesCompactionSpinner(t *testing.T) {
	m := newTestModel()
	m.loading.showSpinner = true
	m.compaction.active = true

	if got := m.spinnerHeight(); got != 2 {
		t.Fatalf("expected spinner height 2 during compaction, got %d", got)
	}
}

func TestCopyNotificationHeight(t *testing.T) {
	m := newTestModel()
	if got := m.notificationHeight(); got != 0 {
		t.Fatalf("expected notification height 0 when empty, got %d", got)
	}

	m.notification.text = copyNotificationMessage
	if got := m.notificationHeight(); got != 2 {
		t.Fatalf("expected notification height 2, got %d", got)
	}

	m.loading.showSpinner = true
	if got := m.notificationHeight(); got != 0 {
		t.Fatalf("expected notification height 0 when spinner is shown, got %d", got)
	}
}

func TestNotificationAdjustsViewportHeight(t *testing.T) {
	m := newTestModel()
	m.applyWindowSize(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	baseHeight := m.viewport.Height()

	m.showNotification(copyNotificationMessage)
	if got := m.viewport.Height(); got != baseHeight-2 {
		t.Fatalf("expected viewport height %d with notification, got %d", baseHeight-2, got)
	}

	updated, _ := m.updateNormalMode(copyNotificationExpiredMsg{expiresAt: m.notification.expiresAt.UnixNano()})
	if got := updated.viewport.Height(); got != baseHeight {
		t.Fatalf("expected viewport height %d after notification expires, got %d", baseHeight, got)
	}
}

func TestFormatLoadingElapsed(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "zero", in: 0, want: "0:00"},
		{name: "under minute", in: 9*time.Second + 900*time.Millisecond, want: "0:09"},
		{name: "minute", in: time.Minute + 5*time.Second, want: "1:05"},
		{name: "negative", in: -time.Second, want: "0:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLoadingElapsed(tt.in); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestView_RendersSpinnerOnLeftWithTopPadding(t *testing.T) {
	m := newTestModel()
	m.loading.showSpinner = true
	m.loading.text = "Accio..."
	m.viewport.SetHeight(1)
	m.viewport.SetContent("assistant output")

	view := m.View().Content
	lines := strings.Split(view, "\n")

	outputLine := -1
	spinnerLine := -1
	for i, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, "assistant output") {
			outputLine = i
		}
		if strings.Contains(stripped, "Accio...") {
			spinnerLine = i
		}
	}

	if outputLine == -1 || spinnerLine == -1 {
		t.Fatalf("expected view to contain output and spinner text, got %q", view)
	}
	if spinnerLine != outputLine+2 {
		t.Fatalf("expected blank spacer line before spinner, got %q", view)
	}
	if strings.TrimSpace(lines[outputLine+1]) != "" {
		t.Fatalf("expected blank spacer line before spinner, got %q", view)
	}
	if !strings.HasPrefix(lines[spinnerLine], " ") {
		t.Fatalf("expected spinner text to preserve left padding, got %q", lines[spinnerLine])
	}
	if !strings.Contains(lines[spinnerLine], "| ") {
		t.Fatalf("expected spacing after spinner glyph, got %q", lines[spinnerLine])
	}
	if strings.Contains(lines[spinnerLine], "0:00") {
		t.Fatalf("expected spinner line not to include elapsed timer, got %q", lines[spinnerLine])
	}
}

func TestHandleCompactionDone_StopsCompactionAndRefreshesOutput(t *testing.T) {
	m := newTestModel()
	m.compaction.active = true
	m.loading.showSpinner = true
	m.compaction.cancel = func() {}
	m.contextStatus = contextStatus{KnownWindow: true, Percent: 10}
	m.stream.handler.Start(make(chan llm.StreamEvent), "Compacting...")
	m.stream.handler.HandleChunk("compacted summary")

	newM, cmd := m.handleCompactionDone()

	if newM.compaction.active || newM.loading.showSpinner {
		t.Fatal("expected compaction mode to stop")
	}
	if newM.compaction.cancel != nil {
		t.Fatal("expected compaction cancel func to be cleared")
	}
	if !strings.Contains(newM.output.Join(), "compacted summary") {
		t.Fatalf("expected streamed compaction summary, got %q", newM.output.Join())
	}
	compacted := newM.appState.GetMessages()
	if len(compacted) != 1 || compacted[0].Role != llm.RoleUser || compacted[0].Content != "compacted summary" {
		t.Fatalf("expected compacted state to keep summary as single user message, got %#v", compacted)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
}

func TestHandleCompactionError_CancelledShowsSoftMessage(t *testing.T) {
	m := newTestModel()
	m.compaction.active = true
	m.loading.showSpinner = true
	m.compaction.cancel = func() {}

	newM, cmd := m.handleCompactionError(context.Canceled)

	if newM.compaction.active || newM.loading.showSpinner {
		t.Fatal("expected compaction mode to stop")
	}
	if !strings.Contains(newM.output.Join(), "Compaction cancelled.") {
		t.Fatalf("expected cancellation message, got %q", newM.output.Join())
	}
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
}

func TestInputMetaView_UnknownContextWindowShowsNA(t *testing.T) {
	m := newTestModel()
	m.width = 120
	m.ctx = &replContext{
		workingDir: "",
		cfg: &config.ResolvedConfig{
			Provider: "openai",
			Model:    "unknown-model",
		},
		registry: &providers.Registry{
			Providers: []providers.Provider{
				{
					ID: "openai",
					Models: []providers.Model{
						{ID: "gpt-5.4", ContextWindow: 2000},
					},
				},
			},
		},
	}

	m.refreshContextStatus()
	meta := m.inputMetaView()
	if !strings.Contains(meta, "N/A") {
		t.Fatalf("expected N/A for unknown context window, got %q", meta)
	}
}

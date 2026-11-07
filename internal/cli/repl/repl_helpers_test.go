package repl

import (
	"context"
	"errors"
	"github.com/mochow13/keen-code/internal/agentcore"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	replstream "github.com/mochow13/keen-code/internal/cli/repl/stream"
	repltooling "github.com/mochow13/keen-code/internal/cli/repl/tooling"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/usage"
)

func TestDisplayModelName(t *testing.T) {
	if got := displayModelName(config.ProviderBedrock, "global.model"); got != "model" {
		t.Fatalf("displayModelName() = %q, want model", got)
	}
	if got := displayModelName(config.ProviderAnthropic, "global.model"); got != "global.model" {
		t.Fatalf("displayModelName() = %q, want global.model", got)
	}
}

func TestRenderLoadingText(t *testing.T) {
	if got := renderLoadingText("", 0); got != "" {
		t.Fatalf("renderLoadingText(empty) = %q", got)
	}
	got := ansi.Strip(renderLoadingText("Use `grep` now", 150*time.Millisecond))
	if got != "Use grep now" {
		t.Fatalf("renderLoadingText() = %q", got)
	}
}

func TestMessageWidthFallbacks(t *testing.T) {
	m := newTestModel()
	m.width = 0
	m.viewport.SetWidth(50)
	if got := m.messageWidth(); got != 46 {
		t.Fatalf("messageWidth() = %d, want 46", got)
	}
	m.viewport.SetWidth(0)
	if got := m.messageWidth(); got != defaultWidth-contentHorizontalPadding {
		t.Fatalf("default messageWidth() = %d, want %d", got, defaultWidth-contentHorizontalPadding)
	}
	m.width = 2
	if got := m.messageWidth(); got != 1 {
		t.Fatalf("narrow messageWidth() = %d, want 1", got)
	}
}

func TestLoadingElapsedText(t *testing.T) {
	m := newTestModel()
	if got := m.loadingElapsedText(); got != "0:00" {
		t.Fatalf("loadingElapsedText() = %q, want 0:00", got)
	}
	m.loading.startedAt = time.Now().Add(-2 * time.Second)
	if got := m.loadingElapsedText(); got != "0:02" {
		t.Fatalf("loadingElapsedText() = %q, want 0:02", got)
	}
}

func TestCurrentModeDefaultsToBuild(t *testing.T) {
	m := newTestModel()
	if got := m.currentMode(); got != agentcore.ModeBuild {
		t.Fatalf("currentMode() = %q, want build", got)
	}
	// An unknown mode behind the AgentCore boundary falls back to build.
	m.agentCore.(*mockAgentCore).mode = agentcore.Mode("unknown")
	if got := m.currentMode(); got != agentcore.ModeBuild {
		t.Fatalf("currentMode() = %q, want build for unknown mode", got)
	}
	m.setMode(agentcore.ModePlan)
	if got := m.currentMode(); got != agentcore.ModePlan {
		t.Fatalf("currentMode() = %q, want plan", got)
	}
}

func TestAbbreviateHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := abbreviateHome(home + "/project"); got != "~/project" {
		t.Fatalf("abbreviateHome() = %q", got)
	}
	if got := abbreviateHome("/outside/project"); got != "/outside/project" {
		t.Fatalf("abbreviateHome(outside) = %q", got)
	}
}

func TestFormatTimeAgo(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "recent", at: now.Add(-10 * time.Second), want: "just now"},
		{name: "minutes", at: now.Add(-5 * time.Minute), want: "5m ago"},
		{name: "hours", at: now.Add(-3 * time.Hour), want: "3h ago"},
		{name: "days", at: now.Add(-48 * time.Hour), want: "2 day(s) ago"},
		{name: "date", at: now.Add(-8 * 24 * time.Hour), want: now.Add(-8 * 24 * time.Hour).Local().Format("Jan 2")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTimeAgo(tt.at); got != tt.want {
				t.Fatalf("formatTimeAgo() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildInitialScreenIncludesLastSession(t *testing.T) {
	ctx := &replContext{version: "1.2.3"}
	last := &session.Summary{
		LastUserMessage: "a long previous question with extra whitespace and enough text to require truncation",
		UpdatedAt:       time.Now().Add(-2 * time.Hour),
	}
	got := ansi.Strip(strings.Join(buildInitialScreen(ctx, last, 40), "\n"))
	for _, want := range []string{"keen v1.2.3", "Last session", "2h ago", "/resume", "…"} {
		if !strings.Contains(got, want) {
			t.Fatalf("initial screen missing %q: %q", want, got)
		}
	}

	withoutSession := ansi.Strip(strings.Join(buildInitialScreen(ctx, nil, 0), "\n"))
	if strings.Contains(withoutSession, "Last session") {
		t.Fatal("initial screen rendered a missing session")
	}
}

func TestWaitForAsyncEventRoutesReadyInputs(t *testing.T) {
	if waitForAsyncEvent(nil, nil, nil, nil, nil, nil) != nil {
		t.Fatal("waitForAsyncEvent returned a command without an LLM channel")
	}

	llmCh := make(chan agentcore.StreamEvent, 1)
	llmCh <- agentcore.StreamEvent{Type: agentcore.StreamEventTypeChunk, Content: "chunk"}
	if msg, ok := waitForAsyncEvent(llmCh, nil, nil, nil, nil, nil)().(mainStreamMsg); !ok || msg.event.Content != "chunk" {
		t.Fatalf("unexpected LLM message %#v", msg)
	}

	permissionCh := make(chan *replpermissions.Request, 1)
	request := &replpermissions.Request{ToolName: "read_file"}
	permissionCh <- request
	if msg, ok := waitForAsyncEvent(make(chan agentcore.StreamEvent), permissionCh, nil, nil, nil, nil)().(permissionReadyMsg); !ok || msg.req != request {
		t.Fatalf("unexpected permission message %#v", msg)
	}

	diffCh := make(chan repltooling.DiffRequest, 1)
	diffCh <- repltooling.DiffRequest{}
	if _, ok := waitForAsyncEvent(make(chan agentcore.StreamEvent), nil, diffCh, nil, nil, nil)().(diffReadyMsg); !ok {
		t.Fatal("expected diffReadyMsg")
	}

	subagentCh := make(chan agentcore.ToolActivity, 1)
	subagentCh <- agentcore.ToolActivity{}
	if _, ok := waitForAsyncEvent(make(chan agentcore.StreamEvent), nil, nil, subagentCh, nil, nil)().(subagentActivityMsg); !ok {
		t.Fatal("expected subagentActivityMsg")
	}

	askUserCh := make(chan *replaskuser.Request, 1)
	askRequest := &replaskuser.Request{}
	askUserCh <- askRequest
	if msg, ok := waitForAsyncEvent(make(chan agentcore.StreamEvent), nil, nil, nil, askUserCh, nil)().(askUserReadyMsg); !ok || msg.req != askRequest {
		t.Fatalf("unexpected ask_user message %#v", msg)
	}
}

func TestFlushBtwToOutput(t *testing.T) {
	m := newTestModel()
	m.btw.question = "question"
	m.btw.lines = []string{"answer one", "answer two"}
	m.flushBtwToOutput()
	got := ansi.Strip(m.output.Join())
	for _, want := range []string{"question", "answer one", "answer two"} {
		if !strings.Contains(got, want) {
			t.Fatalf("BTW output missing %q: %q", want, got)
		}
	}
	if m.btw.lines != nil || m.btw.question != "" {
		t.Fatal("flushBtwToOutput did not clear transient state")
	}
}

func TestCancelAndFlushAdversaryStream(t *testing.T) {
	m := newTestModel()
	cancelled := false
	m.adversary.streamCancel = func() { cancelled = true }
	m.adversary.showSpinner = true
	m.adversary.lines = []string{"discarded"}
	m.cancelAdversaryStream()
	if !cancelled || m.adversary.streamCancel != nil || m.adversary.showSpinner || m.adversary.lines != nil {
		t.Fatal("cancelAdversaryStream did not clear active state")
	}

	m.adversary.focus = "security"
	m.adversary.lines = []string{"finding"}
	m.flushAdversaryToOutput()
	got := ansi.Strip(m.output.Join())
	if !strings.Contains(got, "security") || !strings.Contains(got, "finding") {
		t.Fatalf("adversary output = %q", got)
	}
	if m.adversary.lines != nil || m.adversary.focus != "" {
		t.Fatal("flushAdversaryToOutput did not clear transient state")
	}
}

func TestWaitForAdversaryEvent(t *testing.T) {
	if waitForAdversaryEvent(nil) != nil {
		t.Fatal("waitForAdversaryEvent returned command for nil channel")
	}
	tests := []struct {
		name  string
		event agentcore.StreamEvent
		check func(tea.Msg) bool
	}{
		{name: "chunk", event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeChunk, Content: "text"}, check: func(msg tea.Msg) bool { v, ok := msg.(adversaryChunkMsg); return ok && string(v) == "text" }},
		{name: "usage", event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeUsage, Usage: &agentcore.TokenUsage{InputTokens: 7}}, check: func(msg tea.Msg) bool {
			v, ok := msg.(adversaryUsageMsg)
			return ok && v.usage != nil && v.usage.InputTokens == 7
		}},
		{name: "tool start", event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeToolStart, ToolCall: &agentcore.ToolCall{Name: "read_file"}}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryToolStartMsg); return ok }},
		{name: "tool end", event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeToolEnd, ToolCall: &agentcore.ToolCall{Name: "read_file"}}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryToolEndMsg); return ok }},
		{name: "done", event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeDone}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryDoneMsg); return ok }},
		{name: "error", event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeError, Error: errors.New("failed")}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryErrorMsg); return ok }},
		{name: "incomplete", event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeIncomplete, Error: errors.New("incomplete")}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryErrorMsg); return ok }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan agentcore.StreamEvent, 1)
			ch <- tt.event
			if msg := waitForAdversaryEvent(ch)(); !tt.check(msg) {
				t.Fatalf("unexpected message %#v", msg)
			}
		})
	}
	closed := make(chan agentcore.StreamEvent)
	close(closed)
	if _, ok := waitForAdversaryEvent(closed)().(adversaryDoneMsg); !ok {
		t.Fatal("closed channel did not produce adversaryDoneMsg")
	}
}

func TestWaitForBtwEvent(t *testing.T) {
	if waitForBtwEvent(nil) != nil {
		t.Fatal("waitForBtwEvent returned command for nil channel")
	}
	tests := []struct {
		event agentcore.StreamEvent
		check func(tea.Msg) bool
	}{
		{event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeChunk, Content: "text"}, check: func(msg tea.Msg) bool { v, ok := msg.(btwChunkMsg); return ok && string(v) == "text" }},
		{event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeUsage, Usage: &agentcore.TokenUsage{InputTokens: 7}}, check: func(msg tea.Msg) bool {
			v, ok := msg.(btwUsageMsg)
			return ok && v.usage != nil && v.usage.InputTokens == 7
		}},
		{event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeDone}, check: func(msg tea.Msg) bool { _, ok := msg.(btwDoneMsg); return ok }},
		{event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeError, Error: errors.New("failed")}, check: func(msg tea.Msg) bool { _, ok := msg.(btwErrorMsg); return ok }},
		{event: agentcore.StreamEvent{Type: agentcore.StreamEventTypeIncomplete, Error: errors.New("incomplete")}, check: func(msg tea.Msg) bool { _, ok := msg.(btwErrorMsg); return ok }},
	}
	for _, tt := range tests {
		ch := make(chan agentcore.StreamEvent, 1)
		ch <- tt.event
		if msg := waitForBtwEvent(ch)(); !tt.check(msg) {
			t.Fatalf("unexpected message %#v", msg)
		}
	}
	closed := make(chan agentcore.StreamEvent)
	close(closed)
	if _, ok := waitForBtwEvent(closed)().(btwDoneMsg); !ok {
		t.Fatal("closed channel did not produce btwDoneMsg")
	}
}

func TestAfterStreamUpdateScheduling(t *testing.T) {
	m := newTestModel()
	m.stream.renderInterval = 0
	if cmd := m.afterStreamUpdate(); cmd != nil {
		t.Fatal("immediate stream update returned command")
	}

	m.stream.renderInterval = time.Second
	if cmd := m.afterStreamUpdate(); cmd == nil || !m.stream.renderPending {
		t.Fatal("batched stream update was not scheduled")
	}
	if cmd := m.afterStreamUpdate(); cmd != nil {
		t.Fatal("pending stream update scheduled a duplicate command")
	}
}

func TestHandleSessionPersistenceError(t *testing.T) {
	m := newTestModel()
	m.handleSessionPersistenceError(nil)
	if !m.output.IsEmpty() {
		t.Fatal("nil persistence error changed output")
	}
	m.handleSessionPersistenceError(errors.New("disk full"))
	if got := ansi.Strip(m.output.Join()); !strings.Contains(got, "Session persistence failed: disk full") {
		t.Fatalf("unexpected persistence error output %q", got)
	}
}

func TestSetModeYoloSyncsAppStateAndRequester(t *testing.T) {
	m := newTestModel()
	m.setMode(agentcore.ModeYolo)
	if got := m.currentMode(); got != agentcore.ModeYolo {
		t.Fatalf("currentMode() = %q, want yolo", got)
	}
	if got := m.agentCore.Mode(); got != agentcore.ModeYolo {
		t.Fatalf("agentCore.Mode() = %q, want yolo", got)
	}
	allowed, err := m.permissionRequester.RequestPermission(context.Background(), "bash", "rm -rf /tmp/x", "", true)
	if err != nil || !allowed {
		t.Fatalf("yolo RequestPermission() = (%v, %v), want (true, nil)", allowed, err)
	}
	if m.permissionRequester.HasPendingRequest() {
		t.Fatal("yolo mode created a pending permission request")
	}
	select {
	case req := <-m.permissionRequester.GetRequestChan():
		t.Fatalf("yolo mode sent a permission request: %#v", req)
	default:
	}

	m.setMode(agentcore.ModeBuild)
	if got := m.agentCore.Mode(); got != agentcore.ModeBuild {
		t.Fatalf("agentCore.Mode() = %q, want build", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan bool, 1)
	go func() {
		allowed, _ := m.permissionRequester.RequestPermission(ctx, "bash", "rm -rf /tmp/x", "", true)
		resultCh <- allowed
	}()
	select {
	case req := <-m.permissionRequester.GetRequestChan():
		if req.ToolName != "bash" {
			t.Errorf("expected tool=bash, got %q", req.ToolName)
		}
	case got := <-resultCh:
		t.Fatalf("expected a permission prompt, got result %v", got)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for permission prompt after leaving yolo")
	}
	cancel()
	<-resultCh

	m.setMode(agentcore.Mode("unknown"))
	if got := m.currentMode(); got != agentcore.ModeBuild {
		t.Fatalf("currentMode() = %q, want build for unknown mode", got)
	}
	if got := m.agentCore.Mode(); got != agentcore.ModeBuild {
		t.Fatalf("agentCore.Mode() = %q, want build for unknown mode", got)
	}
}

func TestToggleModeCyclesBuildPlanYolo(t *testing.T) {
	m := newTestModel()
	want := []agentcore.Mode{agentcore.ModePlan, agentcore.ModeYolo, agentcore.ModeBuild}
	for _, mode := range want {
		m.toggleMode()
		if got := m.currentMode(); got != mode {
			t.Fatalf("toggleMode() = %q, want %q", got, mode)
		}
		if got := m.agentCore.Mode(); got != mode {
			t.Fatalf("agentCore.Mode() = %q, want %q", got, mode)
		}
		if mode == agentcore.ModeYolo {
			allowed, err := m.permissionRequester.RequestPermission(context.Background(), "bash", "rm -rf /tmp/x", "", true)
			if err != nil || !allowed {
				t.Fatalf("yolo RequestPermission() = (%v, %v), want (true, nil)", allowed, err)
			}
			if m.permissionRequester.HasPendingRequest() {
				t.Fatal("yolo mode created a pending permission request")
			}
		}
	}
	if m.permissionRequester.HasPendingRequest() {
		t.Fatal("unexpected pending request after toggle cycle")
	}
}

func TestHandleLLMDoneAppendsTurnUsageLine(t *testing.T) {
	sh := replstream.NewStreamHandler(nil)
	sh.Start(make(chan agentcore.StreamEvent), "Loading...")
	sh.HandleChunk("Hello world")

	m := newTestModel()
	m.stream.handler = sh
	m.startLoading("Working...")
	m.loading.startedAt = time.Now().Add(-4 * time.Second)

	m, _ = m.handleLLMUsage(&agentcore.TokenUsage{
		InputTokens:     10000,
		OutputTokens:    250,
		CacheReadTokens: 8000,
	})
	m, _ = m.handleLLMUsage(&agentcore.TokenUsage{
		InputTokens:     4200,
		OutputTokens:    130,
		CacheReadTokens: 3500,
	})

	m, _ = m.handleLLMDone()
	output := strings.Join(m.output.GetLines(), "\n")
	want := "4s · in 14.2k · out 380 · cache 11.5k"
	if !strings.Contains(output, "✔ ") || !strings.Contains(output, want) {
		t.Fatalf("output = %q, want to contain %q", output, want)
	}
	if m.turnUsage.hasUsage {
		t.Fatal("expected turnUsage to be reset after turn completes")
	}
}

func TestHandleLLMDoneOmitsZeroTokenUsageLine(t *testing.T) {
	sh := replstream.NewStreamHandler(nil)
	sh.Start(make(chan agentcore.StreamEvent), "Loading...")
	sh.HandleChunk("Hello world")

	m := newTestModel()
	m.stream.handler = sh
	m.startLoading("Working...")
	m.loading.startedAt = time.Now().Add(-4 * time.Second)

	m, _ = m.handleLLMUsage(&agentcore.TokenUsage{})
	m, _ = m.handleLLMUsage(&agentcore.TokenUsage{CacheWriteTokens: 500, ReasoningTokens: 120})

	m, _ = m.handleLLMDone()
	output := strings.Join(m.output.GetLines(), "\n")
	if !strings.Contains(output, "4s") {
		t.Fatalf("output = %q, want an elapsed-only turn line", output)
	}
	if strings.Contains(output, "in 0") || strings.Contains(output, "·") {
		t.Fatalf("output = %q, want no zero token segments", output)
	}
}

func TestHandleLLMDoneKeepsUsageAfterZeroTokenEvent(t *testing.T) {
	sh := replstream.NewStreamHandler(nil)
	sh.Start(make(chan agentcore.StreamEvent), "Loading...")
	sh.HandleChunk("Hello world")

	m := newTestModel()
	m.stream.handler = sh
	m.startLoading("Working...")
	m.loading.startedAt = time.Now().Add(-4 * time.Second)

	m, _ = m.handleLLMUsage(&agentcore.TokenUsage{})
	m, _ = m.handleLLMUsage(&agentcore.TokenUsage{
		InputTokens:     6000,
		OutputTokens:    240,
		CacheReadTokens: 1200,
	})

	m, _ = m.handleLLMDone()
	output := strings.Join(m.output.GetLines(), "\n")
	want := "4s · in 6k · out 240 · cache 1.2k"
	if !strings.Contains(output, want) {
		t.Fatalf("output = %q, want to contain %q", output, want)
	}
}

func TestHandleLLMDoneIncludesAutoCompactionUsage(t *testing.T) {
	sh := replstream.NewStreamHandler(nil)
	sh.Start(make(chan agentcore.StreamEvent), "Loading...")
	sh.HandleChunk("Hello world")

	m := newTestModel()
	m.stream.handler = sh
	m.startLoading("Working...")
	m.loading.startedAt = time.Now().Add(-4 * time.Second)

	m, _ = m.handleLLMUsage(&agentcore.TokenUsage{
		InputTokens:     10000,
		OutputTokens:    250,
		CacheReadTokens: 8000,
	})
	m, _ = m.handleAutoCompactionApplied(&agentcore.AutoCompactionEvent{
		Usage: &agentcore.TokenUsage{
			InputTokens:     2000,
			OutputTokens:    100,
			CacheReadTokens: 1500,
		},
	})

	m, _ = m.handleLLMDone()
	output := strings.Join(m.output.GetLines(), "\n")
	want := "in 12k · out 350 · cache 9.5k"
	if !strings.Contains(output, want) {
		t.Fatalf("output = %q, want to contain %q", output, want)
	}
}

func TestHandleLLMDoneIncludesSubagentUsage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sh := replstream.NewStreamHandler(nil)
	sh.Start(make(chan agentcore.StreamEvent), "Loading...")
	sh.HandleChunk("Hello world")

	subagentUsage := make(chan usage.Record, 4)
	subagentUsage <- usage.Record{Provider: "p", Model: "m", Input: 3000, Output: 120, CacheRead: 500}

	m := newTestModel()
	m.stream.handler = sh
	m.subagentUsage = subagentUsage
	m.startLoading("Working...")
	m.loading.startedAt = time.Now().Add(-4 * time.Second)

	m, _ = m.handleLLMUsage(&agentcore.TokenUsage{
		InputTokens:     10000,
		OutputTokens:    250,
		CacheReadTokens: 8000,
	})

	m, _ = m.handleLLMDone()
	output := strings.Join(m.output.GetLines(), "\n")
	want := "in 13k · out 370 · cache 8.5k"
	if !strings.Contains(output, want) {
		t.Fatalf("output = %q, want to contain %q", output, want)
	}
}

func TestSubagentUsageMsgAddsToTurnUsage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.startLoading("Working...")

	updated, _ := m.updateNormalMode(subagentUsageMsg{record: usage.Record{
		Provider: "p", Model: "m", Input: 2000, Output: 90, CacheRead: 400,
	}})
	if updated.turnUsage.usage.InputTokens != 2000 || updated.turnUsage.usage.OutputTokens != 90 {
		t.Fatalf("turn usage = %+v, want subagent tokens folded in", updated.turnUsage.usage)
	}
	if !updated.turnUsage.hasUsage {
		t.Fatal("expected turnUsage to be marked as populated")
	}
}

func TestAutoCompactionCancelledAddsUsageToTurn(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan agentcore.StreamEvent), "Working...")
	m.startLoading("Working...")
	updated, _, handled := m.handleLLMStreamMsg(llmAutoCompactionCancelledMsg{
		event: &agentcore.AutoCompactionEvent{
			Usage: &agentcore.TokenUsage{InputTokens: 700, OutputTokens: 30, CacheReadTokens: 100},
		},
	})
	if !handled {
		t.Fatal("expected cancellation event to be handled")
	}
	if updated.turnUsage.usage.InputTokens != 700 || updated.turnUsage.usage.OutputTokens != 30 {
		t.Fatalf("turn usage = %+v, want cancelled compaction tokens folded in", updated.turnUsage.usage)
	}
}

func TestNarrowStartupAndHelpFit(t *testing.T) {
	const width = 20
	ctx := &replContext{version: "1.2.3"}
	last := &session.Summary{LastUserMessage: "a long previous message to display", UpdatedAt: time.Now()}
	views := []string{strings.Join(buildInitialScreenWithTip(ctx, last, width, "A tip with several words to wrap"), "\n"), getHelpText(width)}
	for _, view := range views {
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("line exceeds width %d: %q", width, line)
			}
		}
	}
}

func TestCardAndFooterFitNarrowWidth(t *testing.T) {
	m := newTestModel()
	m.width = 12
	m.ctx.workingDir = "/a/very/long/directory/location"
	for _, view := range []string{fitCardWidth("a long title with several words", m.width), m.inputMetaView()} {
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > m.width {
				t.Fatalf("line exceeds width: %q", line)
			}
		}
	}
}

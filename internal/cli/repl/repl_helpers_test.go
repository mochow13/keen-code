package repl

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	repltooling "github.com/mochow13/keen-code/internal/cli/repl/tooling"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/subagents"
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
	m.mode = ""
	if got := m.currentMode(); got != llm.ModeBuild {
		t.Fatalf("currentMode() = %q, want build", got)
	}
	m.mode = llm.ModePlan
	if got := m.currentMode(); got != llm.ModePlan {
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
	if waitForAsyncEvent(nil, nil, nil, nil, nil) != nil {
		t.Fatal("waitForAsyncEvent returned a command without an LLM channel")
	}

	llmCh := make(chan llm.StreamEvent, 1)
	llmCh <- llm.StreamEvent{Type: llm.StreamEventTypeChunk, Content: "chunk"}
	if msg, ok := waitForAsyncEvent(llmCh, nil, nil, nil, nil)().(mainStreamMsg); !ok || msg.event.Content != "chunk" {
		t.Fatalf("unexpected LLM message %#v", msg)
	}

	permissionCh := make(chan *replpermissions.Request, 1)
	request := &replpermissions.Request{ToolName: "read_file"}
	permissionCh <- request
	if msg, ok := waitForAsyncEvent(make(chan llm.StreamEvent), permissionCh, nil, nil, nil)().(permissionReadyMsg); !ok || msg.req != request {
		t.Fatalf("unexpected permission message %#v", msg)
	}

	diffCh := make(chan repltooling.DiffRequest, 1)
	diffCh <- repltooling.DiffRequest{}
	if _, ok := waitForAsyncEvent(make(chan llm.StreamEvent), nil, diffCh, nil, nil)().(diffReadyMsg); !ok {
		t.Fatal("expected diffReadyMsg")
	}

	subagentCh := make(chan subagents.ToolActivity, 1)
	subagentCh <- subagents.ToolActivity{}
	if _, ok := waitForAsyncEvent(make(chan llm.StreamEvent), nil, nil, subagentCh, nil)().(subagentActivityMsg); !ok {
		t.Fatal("expected subagentActivityMsg")
	}

	askUserCh := make(chan *replaskuser.Request, 1)
	askRequest := &replaskuser.Request{}
	askUserCh <- askRequest
	if msg, ok := waitForAsyncEvent(make(chan llm.StreamEvent), nil, nil, nil, askUserCh)().(askUserReadyMsg); !ok || msg.req != askRequest {
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
		event llm.StreamEvent
		check func(tea.Msg) bool
	}{
		{name: "chunk", event: llm.StreamEvent{Type: llm.StreamEventTypeChunk, Content: "text"}, check: func(msg tea.Msg) bool { v, ok := msg.(adversaryChunkMsg); return ok && string(v) == "text" }},
		{name: "tool start", event: llm.StreamEvent{Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "read_file"}}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryToolStartMsg); return ok }},
		{name: "tool end", event: llm.StreamEvent{Type: llm.StreamEventTypeToolEnd, ToolCall: &llm.ToolCall{Name: "read_file"}}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryToolEndMsg); return ok }},
		{name: "done", event: llm.StreamEvent{Type: llm.StreamEventTypeDone}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryDoneMsg); return ok }},
		{name: "error", event: llm.StreamEvent{Type: llm.StreamEventTypeError, Error: errors.New("failed")}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryErrorMsg); return ok }},
		{name: "incomplete", event: llm.StreamEvent{Type: llm.StreamEventTypeIncomplete, Error: errors.New("incomplete")}, check: func(msg tea.Msg) bool { _, ok := msg.(adversaryErrorMsg); return ok }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan llm.StreamEvent, 1)
			ch <- tt.event
			if msg := waitForAdversaryEvent(ch)(); !tt.check(msg) {
				t.Fatalf("unexpected message %#v", msg)
			}
		})
	}
	closed := make(chan llm.StreamEvent)
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
		event llm.StreamEvent
		check func(tea.Msg) bool
	}{
		{event: llm.StreamEvent{Type: llm.StreamEventTypeChunk, Content: "text"}, check: func(msg tea.Msg) bool { v, ok := msg.(btwChunkMsg); return ok && string(v) == "text" }},
		{event: llm.StreamEvent{Type: llm.StreamEventTypeDone}, check: func(msg tea.Msg) bool { _, ok := msg.(btwDoneMsg); return ok }},
		{event: llm.StreamEvent{Type: llm.StreamEventTypeError, Error: errors.New("failed")}, check: func(msg tea.Msg) bool { _, ok := msg.(btwErrorMsg); return ok }},
		{event: llm.StreamEvent{Type: llm.StreamEventTypeIncomplete, Error: errors.New("incomplete")}, check: func(msg tea.Msg) bool { _, ok := msg.(btwErrorMsg); return ok }},
	}
	for _, tt := range tests {
		ch := make(chan llm.StreamEvent, 1)
		ch <- tt.event
		if msg := waitForBtwEvent(ch)(); !tt.check(msg) {
			t.Fatalf("unexpected message %#v", msg)
		}
	}
	closed := make(chan llm.StreamEvent)
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

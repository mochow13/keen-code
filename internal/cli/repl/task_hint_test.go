package repl

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/decision/tasks/taskcomplexity"
)

func taskHintTestModel(t *testing.T) replModel {
	t.Helper()
	m := routerTestModel(t)
	m.ctx.globalCfg.Router.Enabled = false
	m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "old task"})
	return m
}

func newTaskResult(newTask float64) routerResultMsg {
	return routerResultMsg{input: "new task", classification: routerClassification{result: taskcomplexity.Result{Category: taskcomplexity.CategoryStandard, Probability: 0.9, NewTask: newTask}}}
}

func showTaskHint(t *testing.T, m replModel) replModel {
	t.Helper()
	m.taskHint.display = "new task"
	m.router.pending = true
	m, _ = m.handleRouterResult(newTaskResult(0.9))
	if !m.taskHint.active || len(m.agentCore.GetMessages()) != 1 {
		t.Fatalf("expected task hint without submission: active=%v messages=%d", m.taskHint.active, len(m.agentCore.GetMessages()))
	}
	if !strings.Contains(ansi.Strip(m.viewport.View()), "Moving to a new task? You can save some cost.") {
		t.Fatalf("task hint not rendered: %s", ansi.Strip(m.viewport.View()))
	}
	return m
}

func chooseTaskHint(m replModel, index int) (replModel, tea.Cmd) {
	for range index {
		m, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	return m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestTaskHintClassifiesOnlyWithPriorConversationWhenRouterDisabled(t *testing.T) {
	for _, prior := range []bool{false, true} {
		m := routerTestModel(t)
		m.ctx.globalCfg.Router.Enabled = false
		if prior {
			m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "old task"})
		}
		m, _ = m.submitInput("new task", false)
		// Without prior conversation the prompt is submitted directly; with it, submission waits for classification.
		if m.router.pending != prior || len(m.agentCore.GetMessages()) != 1 {
			t.Fatalf("prior=%v pending=%v messages=%d", prior, m.router.pending, len(m.agentCore.GetMessages()))
		}
	}
}

func TestTaskHintSubmitsWhenBelowThresholdOrFailed(t *testing.T) {
	for _, msg := range []routerResultMsg{newTaskResult(taskHintNewTaskThreshold - 0.01), {input: "new task", classification: routerClassification{err: errors.New("timeout")}}} {
		m := taskHintTestModel(t)
		m.router.pending = true
		m, _ = m.handleRouterResult(msg)
		if m.taskHint.active || len(m.agentCore.GetMessages()) != 2 {
			t.Fatalf("expected direct submission: active=%v messages=%d", m.taskHint.active, len(m.agentCore.GetMessages()))
		}
		if strings.Contains(m.output.Join(), "Task classification unavailable") {
			t.Fatal("router-off classification printed router output")
		}
	}
}

func TestTaskHintShowsProbabilityWithoutRouter(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  routerResultMsg
		want string
	}{
		{name: "below threshold", msg: newTaskResult(0.25), want: "Task check: probably continuing the same task (p=0.25)"},
		{name: "at threshold", msg: newTaskResult(0.7), want: "Task check: this looks like a new task (p=0.70)"},
		{name: "above threshold", msg: newTaskResult(0.9), want: "Task check: this looks like a new task (p=0.90)"},
		{name: "classification failed", msg: routerResultMsg{input: "new task", classification: routerClassification{err: errors.New("timeout")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := taskHintTestModel(t)
			m.router.pending = true
			m, _ = m.handleRouterResult(tc.msg)
			output := ansi.Strip(m.output.Join())
			if tc.want != "" && !strings.Contains(output, tc.want) {
				t.Fatalf("missing probability %q in %q", tc.want, output)
			}
			if tc.want == "" && strings.Contains(output, "Task check:") {
				t.Fatalf("unexpected probability in %q", output)
			}
		})
	}
}
func TestTaskHintShownWhenRouterEnabled(t *testing.T) {
	m := taskHintTestModel(t)
	m.ctx.globalCfg.Router.Enabled = true
	showTaskHint(t, m)
}

func TestTaskHintCreateNewSession(t *testing.T) {
	m := showTaskHint(t, taskHintTestModel(t))
	m, _ = chooseTaskHint(m, 0)
	messages := m.agentCore.GetMessages()
	if m.taskHint.active || len(messages) != 1 || messages[0].Content != "new task" {
		t.Fatalf("new session messages = %#v", messages)
	}
	if output := ansi.Strip(m.output.Join()); !strings.Contains(output, "New session started") || !strings.Contains(output, "new task") {
		t.Fatalf("new session output = %s", output)
	}
}

func TestTaskHintCompactThenSubmit(t *testing.T) {
	m := showTaskHint(t, taskHintTestModel(t))
	m, _ = chooseTaskHint(m, 1)
	if !m.compaction.active || len(m.agentCore.GetMessages()) != 1 {
		t.Fatal("expected compaction before submission")
	}
	m.stream.handler.HandleChunk("summary")
	m, _ = m.handleCompactionDone()
	messages := m.agentCore.GetMessages()
	if len(messages) != 2 || messages[0].Content != "summary" || messages[1].Content != "new task" {
		t.Fatalf("messages after compaction = %#v", messages)
	}
}

func TestTaskHintCompactFailureRestoresInput(t *testing.T) {
	m := showTaskHint(t, taskHintTestModel(t))
	m, _ = chooseTaskHint(m, 1)
	m, _ = m.handleCompactionError(errors.New("boom"))
	if m.textarea.Value() != "new task" || len(m.agentCore.GetMessages()) != 1 || m.taskHint.afterCompaction != "" {
		t.Fatalf("textarea=%q messages=%d", m.textarea.Value(), len(m.agentCore.GetMessages()))
	}
}

func TestTaskHintSkipChoices(t *testing.T) {
	for _, tc := range []struct {
		name        string
		key         tea.KeyPressMsg
		down        int
		skipSession bool
	}{
		{name: "skip this time", key: tea.KeyPressMsg{Code: tea.KeyEnter}, down: 2},
		{name: "esc", key: tea.KeyPressMsg{Code: tea.KeyEscape}},
		{name: "skip this session", key: tea.KeyPressMsg{Code: tea.KeyEnter}, down: 3, skipSession: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := showTaskHint(t, taskHintTestModel(t))
			for range tc.down {
				m, _ = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			m, _ = m.handleKeyMsg(tc.key)
			if m.taskHint.active || len(m.agentCore.GetMessages()) != 2 {
				t.Fatalf("expected submission: active=%v messages=%d", m.taskHint.active, len(m.agentCore.GetMessages()))
			}
			if m.taskHint.skipSession != tc.skipSession || m.taskHintEligible() == tc.skipSession {
				t.Fatalf("skipSession=%v eligible=%v", m.taskHint.skipSession, m.taskHintEligible())
			}
			if tc.skipSession {
				m = m.handleClearCommand()
				m.agentCore.AppendMessage(agentcore.Message{Role: agentcore.RoleUser, Content: "old task"})
				if !m.taskHintEligible() {
					t.Fatal("skip-session flag survived a new session")
				}
			}
		})
	}
}

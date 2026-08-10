package repl

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

const (
	bangStreamStdout = "stdout"
	bangStreamStderr = "stderr"
)

func startBangCommand(ctx context.Context, command string) <-chan tea.Msg {
	events := make(chan tea.Msg, 32)

	go func() {
		defer close(events)

		start := time.Now()
		cmd := exec.CommandContext(ctx, "bash", "-c", command)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			events <- bangDoneMsg{err: err, duration: time.Since(start).Truncate(time.Millisecond)}
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			events <- bangDoneMsg{err: err, duration: time.Since(start).Truncate(time.Millisecond)}
			return
		}
		if err := cmd.Start(); err != nil {
			events <- bangDoneMsg{err: err, duration: time.Since(start).Truncate(time.Millisecond)}
			return
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go readBangLines(ctx, events, &wg, bangStreamStdout, stdout)
		go readBangLines(ctx, events, &wg, bangStreamStderr, stderr)
		wg.Wait()

		err = cmd.Wait()
		msg := bangDoneMsg{err: err, duration: time.Since(start).Truncate(time.Millisecond)}
		if ctx.Err() == context.DeadlineExceeded {
			msg.timedOut = true
		} else if ctx.Err() == context.Canceled {
			msg.canceled = true
			msg.err = context.Canceled
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg.exitCode = exitErr.ExitCode()
		}
		events <- msg
	}()

	return events
}

func readBangLines(ctx context.Context, events chan<- tea.Msg, wg *sync.WaitGroup, stream string, r io.Reader) {
	defer wg.Done()

	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line != "" {
			select {
			case events <- bangOutputMsg{stream: stream, line: line}:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func waitForBangEvent(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return bangDoneMsg{}
		}
		return msg
	}
}

func (m replModel) handleBangMsg(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if !m.bang.active {
		switch msg.(type) {
		case bangOutputMsg, bangDoneMsg:
			return m, nil, true
		}
		return m, nil, false
	}

	switch msg := msg.(type) {
	case bangOutputMsg:
		style := repltheme.BashOutputStyle
		if msg.stream == bangStreamStderr {
			style = repltheme.ErrorStyle
		}
		m.output.AddStyledLine("  "+msg.line, style)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, waitForBangEvent(m.bang.events), true
	case bangDoneMsg:
		m.bang = bangState{}
		m.stopLoading()
		m.adjustTextareaHeight()

		if msg.err != nil {
			if msg.timedOut {
				m.output.AddStyledLine("  command timed out after "+bangTimeout.String(), repltheme.ErrorStyle)
			} else if msg.canceled {
				m.output.AddStyledLine("  command cancelled.", repltheme.InterruptedStyle)
			} else if msg.exitCode != 0 {
				m.output.AddStyledLine("  exit code: "+strconv.Itoa(msg.exitCode), repltheme.ErrorStyle)
			} else if !errors.Is(msg.err, context.Canceled) {
				m.output.AddStyledLine("  "+msg.err.Error(), repltheme.ErrorStyle)
			}
		} else {
			m.output.AddStyledLine("  ‹ done in "+msg.duration.String(), repltheme.BashSummaryStyle)
		}

		_, bottomRule := renderRulesWithChip(m.width, repltheme.RuleStyle, "shell", repltheme.ShellChipOutputStyle)
		m.output.AddLine("\n" + bottomRule)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m *replModel) cancelBangCommand() {
	if !m.bang.active {
		return
	}
	if m.bang.cancel != nil {
		m.bang.cancel()
		m.bang.cancel = nil
	}
}

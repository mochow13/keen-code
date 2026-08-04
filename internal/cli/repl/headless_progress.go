package repl

import (
	"fmt"
	"io"

	reploutput "github.com/user/keen-code/internal/cli/repl/output"
	"github.com/user/keen-code/internal/llm"
)

// headlessProgress streams live agent text chunks and tool start lines to the
// console while a headless run is in flight. It is not safe for concurrent use.
type headlessProgress struct {
	out        io.Writer
	workingDir string
	midLine    bool
}

func newHeadlessProgress(out io.Writer, workingDir string) *headlessProgress {
	return &headlessProgress{out: out, workingDir: workingDir}
}

func (p *headlessProgress) writeText(content string) {
	if p.out == nil || content == "" {
		return
	}
	_, _ = io.WriteString(p.out, content)
	p.midLine = true
}

func (p *headlessProgress) writeToolEnd(toolCall *llm.ToolCall) {
	if p.out == nil || toolCall == nil {
		return
	}
	p.newLine()
	_, _ = fmt.Fprintln(p.out, reploutput.FormatToolEnd(toolCall))
}

func (p *headlessProgress) newLine() {
	if !p.midLine {
		return
	}
	_, _ = io.WriteString(p.out, "\n")
	p.midLine = false
}

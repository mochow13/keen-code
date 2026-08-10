package output

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/llm"
)

func TestNewOutputBuilder(t *testing.T) {
	ob := NewOutputBuilder(80, "")

	if ob.width != 80 {
		t.Errorf("width = %d, want 80", ob.width)
	}

	if !ob.IsEmpty() {
		t.Error("new OutputBuilder should be empty")
	}
}

func TestOutputBuilder_AddLine(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	ob.AddLine("hello")
	ob.AddLine("world")

	lines := ob.GetLines()
	if len(lines) != 2 {
		t.Errorf("len(lines) = %d, want 2", len(lines))
	}

	if lines[0] != "hello" {
		t.Errorf("lines[0] = %q, want 'hello'", lines[0])
	}

	if lines[1] != "world" {
		t.Errorf("lines[1] = %q, want 'world'", lines[1])
	}
}

func TestOutputBuilder_AddEmptyLine(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	ob.AddLine("hello")
	ob.AddEmptyLine()
	ob.AddLine("world")

	lines := ob.GetLines()
	if len(lines) != 3 {
		t.Errorf("len(lines) = %d, want 3", len(lines))
	}

	if lines[1] != "" {
		t.Errorf("lines[1] = %q, want empty string", lines[1])
	}
}

func TestOutputBuilder_SetLines(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	ob.SetLines([]string{"a", "b", "c"})

	lines := ob.GetLines()
	if len(lines) != 3 {
		t.Errorf("len(lines) = %d, want 3", len(lines))
	}

	if lines[0] != "a" {
		t.Errorf("lines[0] = %q, want 'a'", lines[0])
	}
}

func TestOutputBuilder_Join(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	ob.AddLine("line1")
	ob.AddLine("line2")
	ob.AddLine("line3")

	result := ob.Join()
	expected := "line1\nline2\nline3"
	if result != expected {
		t.Errorf("Join() = %q, want %q", result, expected)
	}
}

func TestOutputBuilder_IsEmpty(t *testing.T) {
	ob := NewOutputBuilder(80, "")

	if !ob.IsEmpty() {
		t.Error("IsEmpty() should be true for new builder")
	}

	ob.AddLine("content")

	if ob.IsEmpty() {
		t.Error("IsEmpty() should be false after adding line")
	}
}

func TestFormatToolInput_ShowsOnlyPathBaseName(t *testing.T) {
	workingDir := filepath.Join(string(filepath.Separator), "tmp", "project")
	got := FormatToolInput("read_file", map[string]any{
		"path": filepath.Join(workingDir, "internal", "cli", "repl", "output.go"),
	}, workingDir)

	if got != "output.go" {
		t.Fatalf("expected path base name display, got %q", got)
	}
}

func TestFormatToolInput_ShowsOnlyRelativePathBaseName(t *testing.T) {
	got := FormatToolInput("read_file", map[string]any{"path": "internal/cli/repl/output.go"}, "/tmp/project")

	if got != "output.go" {
		t.Fatalf("expected relative input path base name, got %q", got)
	}
}

func TestFormatToolInput_WriteFileShowsOnlyPathBaseName(t *testing.T) {
	workingDir := filepath.Join(string(filepath.Separator), "tmp", "project")
	got := FormatToolInput("write_file", map[string]any{
		"path":    filepath.Join(workingDir, "docs", "README.md"),
		"content": "ignored",
	}, workingDir)

	if got != "README.md" {
		t.Fatalf("expected write_file UI to show only path base name, got %q", got)
	}
}

func TestFormatToolInput_GrepShowsQuotedPatternInPathBaseName(t *testing.T) {
	got := FormatToolInput("grep", map[string]any{
		"include":     "*.go",
		"output_mode": "content",
		"path":        "internal/cli/repl",
		"pattern":     "FormatToolInput",
	}, "/tmp/project")

	expected := `FormatToolInput in repl`
	if got != expected {
		t.Fatalf("expected quoted pattern with path base name, got %q", got)
	}
}

func TestFormatToolInput_GrepUnescapesLiteralRegexPunctuationForDisplay(t *testing.T) {
	pattern := `\[Unreleased\]|\[0\.38\.1\] \\d+ \\w+ \\s+ \\b`
	got := FormatToolInput("grep", map[string]any{"pattern": pattern, "path": "CHANGELOG.md"}, "/tmp/project")

	want := `[Unreleased]|[0.38.1] \d+ \w+ \s+ \b in CHANGELOG.md`
	if got != want {
		t.Fatalf("expected display-only literal unescaping, got %q, want %q", got, want)
	}
	if pattern != `\[Unreleased\]|\[0\.38\.1\] \\d+ \\w+ \\s+ \\b` {
		t.Fatalf("expected original pattern to remain unchanged, got %q", pattern)
	}
}

func TestFormatToolInput_CallMCPToolShowsOnlyServerAndTool(t *testing.T) {
	got := FormatToolInput("call_mcp_tool", map[string]any{
		"server": "context7",
		"tool":   "query-docs",
		"arguments": map[string]any{
			"query":     "React useEffect API reference",
			"libraryId": "/reactjs/react.dev",
		},
		"checkCache": false,
	}, "/tmp/project")

	if got != "context7/query-docs" {
		t.Fatalf("expected call_mcp_tool input to show only server/tool, got %q", got)
	}
}

func TestFormatToolInput_DelegateTaskShowsTaskAndAgentCounts(t *testing.T) {
	got := FormatToolInput("delegate_task", map[string]any{
		"tasks": []any{
			map[string]any{"agent": "explorer", "task": "Inspect internal/subagents."},
			map[string]any{"agent": "reviewer", "task": "Review tests."},
			map[string]any{"agent": "explorer", "task": "Inspect internal/tools."},
		},
	}, "/tmp/project")

	if got != "3 tasks (explorer ×2, reviewer ×1)" {
		t.Fatalf("unexpected delegate_task input display %q", got)
	}
}

func TestFormatToolDone_DelegateTaskShowsResultCounts(t *testing.T) {
	start := &llm.ToolCall{Name: "delegate_task", Input: map[string]any{
		"tasks": []any{
			map[string]any{"agent": "explorer", "task": "one"},
			map[string]any{"agent": "reviewer", "task": "two"},
			map[string]any{"agent": "explorer", "task": "three"},
		},
	}}
	tests := []struct {
		name       string
		output     map[string]any
		wantStatus string
		wantMarker string
	}{
		{
			name: "success",
			output: map[string]any{
				"completed": 3, "failed": 0,
				"completed_by_agent": map[string]int{"explorer": 2, "reviewer": 1},
				"failed_by_agent":    map[string]int{},
			},
			wantStatus: "3 completed (explorer ×2, reviewer ×1)", wantMarker: "✓",
		},
		{
			name: "partial failure",
			output: map[string]any{
				"completed": 2, "failed": 1,
				"completed_by_agent": map[string]int{"explorer": 1, "reviewer": 1},
				"failed_by_agent":    map[string]int{"explorer": 1},
			},
			wantStatus: "2 completed (explorer ×1, reviewer ×1), 1 failed (explorer ×1)", wantMarker: "✗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatToolDone(start, &llm.ToolCall{Name: "delegate_task", Output: tt.output}, "/tmp/project")
			if !strings.Contains(got, tt.wantStatus) || !strings.Contains(got, tt.wantMarker) {
				t.Fatalf("FormatToolDone() = %q, want marker %q and status %q", got, tt.wantMarker, tt.wantStatus)
			}
		})
	}
}

func TestFormatToolEnd_DoesNotAddTrailingNewline(t *testing.T) {
	got := FormatToolEnd(&llm.ToolCall{Name: "call_mcp_tool", Duration: 5 * 1e6})

	if strings.HasSuffix(got, "\n") {
		t.Fatalf("expected no trailing newline in tool end, got %q", got)
	}
}

func TestFormatToolInput_ShowsLongPathBaseName(t *testing.T) {
	longPath := "internal/cli/repl/" + strings.Repeat("deeply-nested-dir/", 8) + "output.go"
	got := FormatToolInput("read_file", map[string]any{"path": longPath}, "/tmp/project")

	if got != "output.go" {
		t.Fatalf("expected path base name, got %q", got)
	}
}

func TestFormatToolInput_CompactsLongPattern(t *testing.T) {
	longPattern := "(?i)^" + strings.Repeat("(alternation|", 20) + "value" + strings.Repeat(")*", 20) + "$"
	got := FormatToolInput("grep", map[string]any{"pattern": longPattern, "path": "internal"}, "/tmp/project")

	if !strings.HasPrefix(got, `(?i)^(altern`) {
		t.Fatalf("expected pattern prefix preserved, got %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("expected compacted pattern to contain ellipsis, got %q", got)
	}
	if !strings.HasSuffix(got, "in internal") {
		t.Fatalf("expected path suffix, got %q", got)
	}
}

func TestFormatToolInput_EscapesControlCharacters(t *testing.T) {
	got := FormatToolInput("grep", map[string]any{"pattern": "line1\nline2\ttab"}, "/tmp/project")

	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("expected control characters to be escaped, got %q", got)
	}
	if !strings.Contains(got, `line1\nline2\ttab`) {
		t.Fatalf("expected escaped control characters in output, got %q", got)
	}
}

func TestFormatToolInput_GenericFallbackIsBounded(t *testing.T) {
	got := FormatToolInput("unknown_tool", map[string]any{
		"alpha": "one",
		"beta":  2,
		"gamma": true,
		"delta": "four",
		"zeta":  5,
		"omega": 6,
	}, "/tmp/project")

	if !strings.Contains(got, `alpha=one`) || !strings.Contains(got, "beta=2") {
		t.Fatalf("expected first sorted scalar fields, got %q", got)
	}
	if !strings.Contains(got, "+4") {
		t.Fatalf("expected overflow count, got %q", got)
	}
	if strings.Contains(got, "gamma") || strings.Contains(got, "delta") || strings.Contains(got, "zeta") || strings.Contains(got, "omega") {
		t.Fatalf("expected overflow fields to be hidden, got %q", got)
	}
}

func TestFormatToolInput_GenericFallbackCompactsCollections(t *testing.T) {
	got := FormatToolInput("unknown_tool", map[string]any{
		"options":  map[string]any{"nested": true},
		"patterns": []any{"a", "b", "c"},
	}, "/tmp/project")

	if !strings.Contains(got, "options={…}") {
		t.Fatalf("expected nested map summary, got %q", got)
	}
	if !strings.Contains(got, "patterns=[3]") {
		t.Fatalf("expected array count summary, got %q", got)
	}
}

func TestToolDisplayName_FallbackCapitalizes(t *testing.T) {
	if got := toolDisplayName("some_custom_tool"); got != "Some Custom Tool" {
		t.Fatalf("expected title-cased fallback label, got %q", got)
	}
	if got := toolDisplayName("read_file"); got != "Read" {
		t.Fatalf("expected mapped label, got %q", got)
	}
}

func TestFormatToolInput_GenericFallbackLongStringCompacted(t *testing.T) {
	got := FormatToolInput("unknown_tool", map[string]any{"query": strings.Repeat("x", 200)}, "/tmp/project")

	if !strings.Contains(got, "…") {
		t.Fatalf("expected long generic value to be compacted, got %q", got)
	}
	if len([]rune(got)) > maxDisplayValueRunes+len(`query=`)+1 {
		t.Fatalf("expected compacted generic value, got %d runes: %q", len([]rune(got)), got)
	}
}

func TestFormatToolDone_ShowsResultMetadata(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		input    map[string]any
		output   map[string]any
		contains []string
	}{
		{
			name:   "read_file",
			tool:   "read_file",
			input:  map[string]any{"path": "go.mod"},
			output: map[string]any{"total_lines": 42, "bytes_read": 2048, "truncated": false},
			contains: []string{
				"✓ Read", "go.mod", "42 lines", "2.0 KB", "8ms",
			},
		},
		{
			name:   "grep",
			tool:   "grep",
			input:  map[string]any{"pattern": "foo", "path": "internal"},
			output: map[string]any{"count": 6, "output_mode": "content"},
			contains: []string{
				"✓ Search", `foo in internal`, "6 matches",
			},
		},
		{
			name:   "glob",
			tool:   "glob",
			input:  map[string]any{"pattern": "**/*.go"},
			output: map[string]any{"count": 84},
			contains: []string{
				"✓ Find", "84 files",
			},
		},
		{
			name:   "write_file created",
			tool:   "write_file",
			input:  map[string]any{"path": "README.md"},
			output: map[string]any{"created": true, "bytes_written": 512},
			contains: []string{
				"✓ Write", "created", "512 bytes",
			},
		},
		{
			name:   "edit_file",
			tool:   "edit_file",
			input:  map[string]any{"path": "output.go"},
			output: map[string]any{"replacementCount": 2},
			contains: []string{
				"✓ Edit", "2 replacements",
			},
		},
		{
			name:   "web_fetch",
			tool:   "web_fetch",
			input:  map[string]any{"url": "https://example.com"},
			output: map[string]any{"status_code": 200, "content": strings.Repeat("a", 4096)},
			contains: []string{
				"✓ Fetch", "4.0 KB",
			},
		},
		{
			name:   "bash non-zero exit",
			tool:   "bash",
			input:  map[string]any{"command": "go test ./..."},
			output: map[string]any{"exit_code": 1},
			contains: []string{
				"✓ Run", "exit 1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := &llm.ToolCall{Name: tt.tool, Input: tt.input}
			end := &llm.ToolCall{Name: tt.tool, Output: tt.output, Duration: 8 * 1e6}
			got := FormatToolDone(start, end, "/tmp/project")
			plain := ansi.Strip(got)
			for _, want := range tt.contains {
				if !strings.Contains(plain, want) {
					t.Fatalf("FormatToolDone() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestFormatToolDone_ErrorShowsReasonAndDuration(t *testing.T) {
	start := &llm.ToolCall{Name: "edit_file", Input: map[string]any{"path": "output.go"}}
	end := &llm.ToolCall{Name: "edit_file", Error: "op 1: line hash mismatch at 2:fff (actual a1b)", Duration: 2 * 1e6}
	got := FormatToolDone(start, end, "/tmp/project")

	for _, want := range []string{"✗ Edit", "output.go", "line hash mismatch", "2ms"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatToolDone() = %q, want it to contain %q", got, want)
		}
	}
}

func TestMCPFailuresHideErrorDetailsForMainAndSubagent(t *testing.T) {
	start := &llm.ToolCall{
		Name:  "call_mcp_tool",
		Input: map[string]any{"server": "context7", "tool": "query-docs"},
	}
	end := &llm.ToolCall{
		Name:     "call_mcp_tool",
		Error:    "transport failed\nsensitive MCP response content",
		Duration: time.Second,
	}

	for name, got := range map[string]string{
		"main":     FormatToolDone(start, end, "/tmp/project"),
		"subagent": FormatSubagentTool("worker-0", start, end, "/tmp/project"),
		"orphan":   FormatToolEnd(end),
	} {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{"MCP", "failed", "1.0s"} {
				if !strings.Contains(got, want) {
					t.Fatalf("formatted MCP failure = %q, want %q", got, want)
				}
			}
			for _, hidden := range []string{"transport", "sensitive", "response content"} {
				if strings.Contains(got, hidden) {
					t.Fatalf("formatted MCP failure leaked %q: %q", hidden, got)
				}
			}
		})
	}
}

func TestFormatSubagentToolPrefixesAgentAndHidesOutput(t *testing.T) {
	start := &llm.ToolCall{Name: "bash", Input: map[string]any{"command": "go test -race ./..."}}
	end := &llm.ToolCall{Name: "bash", Output: map[string]any{"stdout": "hidden-output", "exit_code": 9}, Error: "failed", Duration: time.Second}
	got := FormatSubagentTool("worker", start, end, "/tmp/project")
	for _, want := range []string{"[worker]", "Run", "failed", "1.0s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatSubagentTool() = %q, want %q", got, want)
		}
	}
	for _, hidden := range []string{"hidden-output", "exit 9"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("FormatSubagentTool leaked %q: %q", hidden, got)
		}
	}
}

func TestFormatToolEnd_OrphanEndShowsLabelAndDuration(t *testing.T) {
	got := FormatToolEnd(&llm.ToolCall{Name: "read_file", Duration: 1500 * 1e6})

	if !strings.Contains(got, "✓") || !strings.Contains(got, "Read") || !strings.Contains(got, "1.5s") {
		t.Fatalf("expected label and duration, got %q", got)
	}
	if strings.Contains(got, "read_file") {
		t.Fatalf("expected friendly label instead of canonical name, got %q", got)
	}
}

func TestFormatToolStart_UsesFriendlyLabel(t *testing.T) {
	got := FormatToolStart(&llm.ToolCall{Name: "grep", Input: map[string]any{"pattern": "foo", "path": "internal"}}, "/tmp/project")

	if !strings.Contains(got, "●") || !strings.Contains(got, "Search") {
		t.Fatalf("expected friendly running label, got %q", got)
	}
	if !strings.Contains(got, "→") || !strings.Contains(ansi.Strip(got), `foo in internal`) {
		t.Fatalf("expected detail after arrow, got %q", got)
	}
	if !strings.Contains(got, repltheme.ToolMetaStyle.Render(" in ")) {
		t.Fatalf("expected search path separator to use metadata style, got %q", got)
	}
}

func TestFormatToolDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{0, "0ms"},
		{500 * time.Microsecond, "0ms"},
		{8 * time.Millisecond, "8ms"},
		{1500 * time.Millisecond, "1.5s"},
		{12 * time.Second, "12s"},
		{95 * time.Second, "1m35s"},
	}
	for _, tt := range tests {
		if got := formatToolDuration(tt.duration); got != tt.want {
			t.Errorf("formatToolDuration(%v) = %q, want %q", tt.duration, got, tt.want)
		}
	}
}

func TestOutputBuilder_AddUserInput(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	style := lipgloss.NewStyle()

	ob.AddUserInput("hello", style)

	lines := ob.GetLines()
	// 1 top padding + 1 content line + 1 bottom padding + 1 trailing empty line
	if len(lines) != 4 {
		t.Errorf("len(lines) = %d, want 4 (top pad + content + bottom pad + empty)", len(lines))
	}

	if !strings.Contains(ob.Join(), "hello") {
		t.Errorf("output should contain 'hello', got %q", ob.Join())
	}
}

func TestOutputBuilder_AddUserInput_MultiLine(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	style := lipgloss.NewStyle()

	ob.AddUserInput("line1\nline2", style)

	lines := ob.GetLines()
	// 1 top padding + 2 content lines + 1 bottom padding + 1 trailing empty line
	if len(lines) != 5 {
		t.Errorf("len(lines) = %d, want 5", len(lines))
	}

	joined := ob.Join()
	if !strings.Contains(joined, "line1") {
		t.Errorf("output should contain 'line1', got %q", joined)
	}

	if !strings.Contains(joined, "line2") {
		t.Errorf("output should contain 'line2', got %q", joined)
	}
}

func TestOutputBuilder_AddUserInput_WrappedLinesAreIndented(t *testing.T) {
	ob := NewOutputBuilder(12, "")
	style := lipgloss.NewStyle()

	ob.AddUserInput("hello world", style)

	lines := ob.GetLines()
	if len(lines) != 5 {
		t.Fatalf("len(lines) = %d, want 5", len(lines))
	}

	line := ansi.Strip(lines[2])
	if strings.Contains(line, "hello") || !strings.HasPrefix(line, "    world") {
		t.Errorf("wrapped line should be indented, got %q", line)
	}
}

func TestOutputBuilder_AddUserInput_FitsWithinWidthAfterPadding(t *testing.T) {
	ob := NewOutputBuilder(12, "")
	style := lipgloss.NewStyle()

	ob.AddUserInput("hello world", style)

	for _, line := range ob.GetLines() {
		if width := lipgloss.Width(line); width > 12 {
			t.Fatalf("line width = %d, want <= 12: %q", width, line)
		}
	}
}

func TestOutputBuilder_AddAssistantResponse(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	style := lipgloss.NewStyle()

	ob.AddAssistantResponse("response text", style)

	lines := ob.GetLines()
	if len(lines) != 2 {
		t.Errorf("len(lines) = %d, want 2 (response + empty)", len(lines))
	}

	if !strings.Contains(lines[0], "response text") {
		t.Errorf("lines[0] should contain 'response text', got %q", lines[0])
	}
}

func TestOutputBuilder_AddAssistantResponse_MultiLine(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	style := lipgloss.NewStyle()

	ob.AddAssistantResponse("line1\nline2\nline3", style)

	lines := ob.GetLines()
	if len(lines) != 4 {
		t.Errorf("len(lines) = %d, want 4", len(lines))
	}
}

func TestOutputBuilder_AddError(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	style := lipgloss.NewStyle()

	ob.AddError("something went wrong", style)

	lines := ob.GetLines()
	if len(lines) != 2 {
		t.Errorf("len(lines) = %d, want 2 (error + empty)", len(lines))
	}

	if !strings.Contains(lines[0], "Error: something went wrong") {
		t.Errorf("lines[0] should contain error message, got %q", lines[0])
	}
}

func TestOutputBuilder_AddStyledLine(t *testing.T) {
	ob := NewOutputBuilder(80, "")
	style := lipgloss.NewStyle()

	ob.AddStyledLine("styled content", style)

	lines := ob.GetLines()
	if len(lines) != 1 {
		t.Errorf("len(lines) = %d, want 1", len(lines))
	}

	if !strings.Contains(lines[0], "styled content") {
		t.Errorf("lines[0] should contain 'styled content', got %q", lines[0])
	}
}

func TestOutputBuilder_SetWidth(t *testing.T) {
	builder := NewOutputBuilder(10, "")
	builder.SetWidth(20)
	if builder.width != 20 {
		t.Fatalf("width = %d, want 20", builder.width)
	}
}

func TestOutputBuilder_AddToolLifecycle(t *testing.T) {
	builder := NewOutputBuilder(80, "/tmp/project")
	builder.AddToolStart(&llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "README.md"}})
	builder.AddToolEnd(&llm.ToolCall{Name: "read_file"})
	if len(builder.GetLines()) != 2 {
		t.Fatalf("tool lifecycle lines = %d, want 2", len(builder.GetLines()))
	}
}

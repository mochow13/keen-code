package repl

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestNewOutputBuilder(t *testing.T) {
	ob := NewOutputBuilder(80)

	if ob.width != 80 {
		t.Errorf("width = %d, want 80", ob.width)
	}

	if !ob.IsEmpty() {
		t.Error("new OutputBuilder should be empty")
	}
}

func TestOutputBuilder_AddLine(t *testing.T) {
	ob := NewOutputBuilder(80)
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
	ob := NewOutputBuilder(80)
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
	ob := NewOutputBuilder(80)
	ob.SetLines([]string{"a", "b", "c"})

	lines := ob.GetLines()
	if len(lines) != 3 {
		t.Errorf("len(lines) = %d, want 3", len(lines))
	}

	if lines[0] != "a" {
		t.Errorf("lines[0] = %q, want 'a'", lines[0])
	}
}

func TestOutputBuilder_Join_Empty(t *testing.T) {
	ob := NewOutputBuilder(80)

	result := ob.Join()
	if result != "" {
		t.Errorf("Join() = %q, want empty string", result)
	}
}

func TestOutputBuilder_Join(t *testing.T) {
	ob := NewOutputBuilder(80)
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
	ob := NewOutputBuilder(80)

	if !ob.IsEmpty() {
		t.Error("IsEmpty() should be true for new builder")
	}

	ob.AddLine("content")

	if ob.IsEmpty() {
		t.Error("IsEmpty() should be false after adding line")
	}
}

func TestOutputBuilder_AddUserInput(t *testing.T) {
	ob := NewOutputBuilder(80)
	style := lipgloss.NewStyle()

	ob.AddUserInput("hello", style)

	lines := ob.GetLines()
	if len(lines) != 2 {
		t.Errorf("len(lines) = %d, want 2 (input + empty)", len(lines))
	}

	if !strings.Contains(lines[0], "hello") {
		t.Errorf("lines[0] should contain 'hello', got %q", lines[0])
	}
}

func TestOutputBuilder_AddUserInput_MultiLine(t *testing.T) {
	ob := NewOutputBuilder(80)
	style := lipgloss.NewStyle()

	ob.AddUserInput("line1\nline2", style)

	lines := ob.GetLines()
	if len(lines) != 3 {
		t.Errorf("len(lines) = %d, want 3", len(lines))
	}

	if !strings.Contains(lines[0], "line1") {
		t.Errorf("lines[0] should contain 'line1', got %q", lines[0])
	}

	if !strings.Contains(lines[1], "line2") {
		t.Errorf("lines[1] should contain 'line2', got %q", lines[1])
	}
}

func TestOutputBuilder_AddAssistantResponse(t *testing.T) {
	ob := NewOutputBuilder(80)
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
	ob := NewOutputBuilder(80)
	style := lipgloss.NewStyle()

	ob.AddAssistantResponse("line1\nline2\nline3", style)

	lines := ob.GetLines()
	if len(lines) != 4 {
		t.Errorf("len(lines) = %d, want 4", len(lines))
	}
}

func TestOutputBuilder_AddError(t *testing.T) {
	ob := NewOutputBuilder(80)
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
	ob := NewOutputBuilder(80)
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

func TestOutputBuilder_MultipleOperations(t *testing.T) {
	ob := NewOutputBuilder(80)
	style := lipgloss.NewStyle()

	ob.AddLine("header")
	ob.AddEmptyLine()
	ob.AddUserInput("user query", style)
	ob.AddAssistantResponse("assistant answer", style)
	ob.AddError("warning", style)
	ob.AddStyledLine("footer", style)

	lines := ob.GetLines()
	if ob.IsEmpty() {
		t.Error("builder should not be empty after multiple operations")
	}

	result := ob.Join()
	if result == "" {
		t.Error("Join() should not return empty after operations")
	}

	if len(lines) < 6 {
		t.Errorf("expected at least 6 lines, got %d", len(lines))
	}
}

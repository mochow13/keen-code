package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/keen-code/internal/filesystem"
)

type mockBashPermissionRequester struct {
	allow       bool
	called      bool
	isDangerous bool
}

func (m *mockBashPermissionRequester) RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error) {
	m.called = true
	m.isDangerous = isDangerous
	return m.allow, nil
}

func TestBashTool_Name(t *testing.T) {
	tool := NewBashTool(nil, nil)
	if tool.Name() != "bash" {
		t.Errorf("expected name 'bash', got %q", tool.Name())
	}
}

func TestBashTool_Description(t *testing.T) {
	tool := NewBashTool(nil, nil)
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
}

func TestBashTool_InputSchema(t *testing.T) {
	tool := NewBashTool(nil, nil)
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Error("schema type should be 'object'")
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}

	if _, ok := properties["command"]; !ok {
		t.Error("command property should exist")
	}

	if _, ok := properties["isDangerous"]; !ok {
		t.Error("isDangerous property should exist")
	}

	if _, ok := properties["summary"]; !ok {
		t.Error("summary property should exist")
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required should be a string slice")
	}

	found := false
	for _, r := range required {
		if r == "command" {
			found = true
			break
		}
	}
	if !found {
		t.Error("command should be required")
	}
}

func TestBashTool_Execute_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{
			name:  "nil_input",
			input: nil,
		},
		{
			name:  "string_input",
			input: "not a map",
		},
		{
			name:  "int_input",
			input: 123,
		},
		{
			name:  "missing_command",
			input: map[string]any{},
		},
		{
			name:  "non-string_command",
			input: map[string]any{"command": 123},
		},
		{
			name:  "empty_command",
			input: map[string]any{"command": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewBashTool(nil, nil)
			_, err := tool.Execute(context.Background(), tt.input)
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

func TestBashTool_Execute_SimpleCommand(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["exit_code"] != 0 {
		t.Errorf("expected exit code 0, got %v", resultMap["exit_code"])
	}

	stdout, ok := resultMap["stdout"].(string)
	if !ok {
		t.Fatal("stdout should be a string")
	}

	if !strings.Contains(stdout, "hello") {
		t.Errorf("expected stdout to contain 'hello', got %q", stdout)
	}
}

func TestBashTool_Execute_CommandWithArguments(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello world foo bar",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	stdout := resultMap["stdout"].(string)
	if !strings.Contains(stdout, "hello world foo bar") {
		t.Errorf("expected stdout to contain 'hello world foo bar', got %q", stdout)
	}
}

func TestBashTool_Execute_CommandWithPipes(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo 'line1\nline2\nline3' | head -2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	stdout := resultMap["stdout"].(string)
	if !strings.Contains(stdout, "line1") || !strings.Contains(stdout, "line2") {
		t.Errorf("expected stdout to contain 'line1' and 'line2', got %q", stdout)
	}
}

func TestBashTool_Execute_InvalidCommand(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "nonexistentcommand12345",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	if resultMap["exit_code"] == 0 {
		t.Error("expected non-zero exit code for invalid command")
	}

	stderr := resultMap["stderr"].(string)
	if stderr == "" {
		t.Error("expected stderr to contain error message")
	}
}

func TestBashTool_Execute_WithSummary(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo test",
		"summary": "Print test message",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	summary, ok := resultMap["summary"].(string)
	if !ok || summary != "Print test message" {
		t.Errorf("expected summary 'Print test message', got %v", resultMap["summary"])
	}
}

func TestBashTool_Execute_IsDangerous_FlagPassed(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	_, err := tool.Execute(context.Background(), map[string]any{
		"command":     "echo test",
		"isDangerous": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mockPR.called {
		t.Error("permission requester should have been called for dangerous command")
	}

	if !mockPR.isDangerous {
		t.Error("isDangerous flag should have been passed as true")
	}
}

func TestBashTool_Execute_IsDangerous_Denied(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: false}
	tool := NewBashTool(guard, mockPR)

	_, err := tool.Execute(context.Background(), map[string]any{
		"command":     "echo test",
		"isDangerous": true,
	})
	if err == nil {
		t.Error("expected error when dangerous command permission is denied")
	}

	if !strings.Contains(err.Error(), "dangerous command execution rejected") {
		t.Errorf("expected error message about dangerous command rejection, got %q", err.Error())
	}
}

func TestBashTool_Execute_PermissionDenied(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: false}
	tool := NewBashTool(guard, mockPR)

	// For dangerous commands, permission is always requested regardless of guard
	_, err := tool.Execute(context.Background(), map[string]any{
		"command":     "echo test",
		"isDangerous": true,
	})
	if err == nil {
		t.Error("expected error when permission is denied")
	}

	if !strings.Contains(err.Error(), "dangerous command execution rejected") {
		t.Errorf("expected error message about dangerous command rejection, got %q", err.Error())
	}
}

func TestBashTool_Execute_NoPermissionRequester(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	tool := NewBashTool(guard, nil)

	// For dangerous commands, permission requester is required
	_, err := tool.Execute(context.Background(), map[string]any{
		"command":     "echo test",
		"isDangerous": true,
	})
	if err == nil {
		t.Error("expected error when permission requester is nil for dangerous command")
	}

	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("expected error message about permission requester not available, got %q", err.Error())
	}
}

func TestBashTool_Execute_CapturesStderr(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "ls /nonexistent_directory_12345",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	// Command should fail with non-zero exit code
	if resultMap["exit_code"] == 0 {
		t.Error("expected non-zero exit code for failing command")
	}

	stderr := resultMap["stderr"].(string)
	if stderr == "" {
		t.Error("expected stderr to contain error message for failing command")
	}
}

func TestBashTool_Execute_CommandInWorkingDir(t *testing.T) {
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": fmt.Sprintf("cat %s", testFile),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	stdout := resultMap["stdout"].(string)
	if !strings.Contains(stdout, "test content") {
		t.Errorf("expected stdout to contain 'test content', got %q", stdout)
	}
}

func TestBashTool_Execute_LargeOutput(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "seq 1 1000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	stdout := resultMap["stdout"].(string)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 1000 {
		t.Errorf("expected 1000 lines, got %d", len(lines))
	}
}

func TestBashTool_Execute_WithBooleanIsDangerous(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)

	tests := []struct {
		name        string
		isDangerous any
		expectCall  bool
	}{
		{
			name:        "true_boolean",
			isDangerous: true,
			expectCall:  true,
		},
		{
			name:        "false_boolean",
			isDangerous: false,
			expectCall:  false,
		},
		{
			name:        "string_true",
			isDangerous: "true",
			expectCall:  false,
		},
		{
			name:        "nil",
			isDangerous: nil,
			expectCall:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPR := &mockBashPermissionRequester{allow: true}
			tool := NewBashTool(guard, mockPR)

			input := map[string]any{
				"command": "echo test",
			}
			if tt.isDangerous != nil {
				input["isDangerous"] = tt.isDangerous
			}

			_, err := tool.Execute(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectCall && !mockPR.called {
				t.Error("expected permission requester to be called for dangerous command")
			}
		})
	}
}

func TestBashTool_Execute_ResultStructure(t *testing.T) {
	tempDir := t.TempDir()
	guard := filesystem.NewGuard(tempDir, nil)
	mockPR := &mockBashPermissionRequester{allow: true}
	tool := NewBashTool(guard, mockPR)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
		"summary": "Test command",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("result should be a map")
	}

	requiredFields := []string{"command", "exit_code", "stdout", "stderr"}
	for _, field := range requiredFields {
		if _, ok := resultMap[field]; !ok {
			t.Errorf("result should contain %s field", field)
		}
	}

	if resultMap["command"] != "echo hello" {
		t.Errorf("expected command 'echo hello', got %v", resultMap["command"])
	}

	if resultMap["summary"] != "Test command" {
		t.Errorf("expected summary 'Test command', got %v", resultMap["summary"])
	}
}

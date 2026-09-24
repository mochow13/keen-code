package agentcore_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mochow13/keen-code/internal/agentcore"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	"github.com/mochow13/keen-code/internal/cli/repl/tooling"
	"github.com/mochow13/keen-code/internal/config"
)

func setupTestCore(t *testing.T, workingDir string, requester *replaskuser.Requester) agentcore.AgentCore {
	t.Helper()
	core := agentcore.New(nil, workingDir,
		&config.ResolvedConfig{Model: "model"},
		config.DefaultGlobalConfig())
	_ = core.SetupTools(
		context.Background(),
		replpermissions.NewAutoApproveRequester(),
		tooling.NewDiffEmitter(),
		requester,
		nil,
		false,
	)
	return core
}

func TestSetupToolsOmitsDelegateToolWithoutProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()
	core := setupTestCore(t, workingDir, nil)

	if slices.Contains(core.RegisteredToolNames(), "delegate_task") {
		t.Fatal("delegate_task should not be registered without subagent profiles")
	}
}

func TestSetupToolsRegistersAskUserOnlyWithRequester(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()

	if slices.Contains(setupTestCore(t, workingDir, nil).RegisteredToolNames(), agentcore.ToolNameAskUser) {
		t.Fatal("ask_user should not be registered without an interactive requester")
	}
	if !slices.Contains(setupTestCore(t, workingDir, replaskuser.NewRequester()).RegisteredToolNames(), agentcore.ToolNameAskUser) {
		t.Fatal("ask_user should be registered with an interactive requester")
	}
}

func TestSetupToolsRegistersDelegateToolForVisibleProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()
	agentsDir := filepath.Join(workingDir, ".agents", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("create agents directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "worker.md"), []byte(`---
name: worker
description: Handles focused work.
---
`), 0o644); err != nil {
		t.Fatalf("write visible profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "hidden.md"), []byte(`---
name: hidden
description: Hidden work.
hidden: true
---
`), 0o644); err != nil {
		t.Fatalf("write hidden profile: %v", err)
	}

	core := setupTestCore(t, workingDir, nil)

	if !slices.Contains(core.RegisteredToolNames(), "delegate_task") {
		t.Fatal("delegate_task should be registered with a visible profile")
	}
}

func TestRegisteredToolNamesIncludesWriteEditInPlanMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	core := setupTestCore(t, t.TempDir(), nil)
	core.SetMode(agentcore.ModePlan)

	for _, name := range []string{agentcore.ToolNameWriteFile, agentcore.ToolNameEditFile} {
		if !slices.Contains(core.RegisteredToolNames(), name) {
			t.Fatalf("%s should remain available for /allow in plan mode", name)
		}
	}
}

func TestSetupToolsWithoutActivityReturnsNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()
	core := agentcore.New(nil, workingDir,
		&config.ResolvedConfig{Model: "model"},
		config.DefaultGlobalConfig())
	activity := core.SetupTools(
		context.Background(),
		replpermissions.NewAutoApproveRequester(),
		tooling.NewDiffEmitter(),
		nil,
		nil,
		false,
	)
	if activity != nil {
		t.Fatal("expected nil activity channel when forwarding is disabled")
	}
}

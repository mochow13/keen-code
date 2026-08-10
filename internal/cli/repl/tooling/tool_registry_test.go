package tooling

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	replappstate "github.com/mochow13/keen-code/internal/cli/repl/appstate"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	"github.com/mochow13/keen-code/internal/config"
)

func TestSetupToolRegistryOmitsDelegateToolWithoutProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()
	state := replappstate.New(nil, workingDir)

	SetupToolRegistry(
		workingDir,
		state,
		replpermissions.NewAutoApproveRequester(),
		NewDiffEmitter(),
		nil,
		&config.ResolvedConfig{Model: "model"},
		config.DefaultGlobalConfig(),
		nil,
	)

	if _, ok := state.GetToolRegistry().Get("delegate_task"); ok {
		t.Fatal("delegate_task should not be registered without subagent profiles")
	}
}

func TestSetupToolRegistryRegistersDelegateToolForVisibleProfiles(t *testing.T) {
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

	state := replappstate.New(nil, workingDir)
	SetupToolRegistry(
		workingDir,
		state,
		replpermissions.NewAutoApproveRequester(),
		NewDiffEmitter(),
		nil,
		&config.ResolvedConfig{Model: "model"},
		config.DefaultGlobalConfig(),
		nil,
	)

	tool, ok := state.GetToolRegistry().Get("delegate_task")
	if !ok {
		t.Fatal("delegate_task should be registered with a visible profile")
	}

	properties := tool.InputSchema()["properties"].(map[string]any)
	tasks := properties["tasks"].(map[string]any)
	items := tasks["items"].(map[string]any)
	agent := items["properties"].(map[string]any)["agent"].(map[string]any)
	if !reflect.DeepEqual(agent["enum"], []string{"worker"}) {
		t.Fatalf("agent enum = %#v, want [worker]", agent["enum"])
	}
}

package agentcore_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mochow13/keen-code/internal/agentcore"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	"github.com/mochow13/keen-code/internal/cli/repl/tooling"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/usage"
)

func TestSetupToolsForwardsActivity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workingDir := t.TempDir()
	core := agentcore.New(nil, workingDir,
		&config.ResolvedConfig{Model: "model"},
		config.DefaultGlobalConfig())
	sink := make(chan usage.Record, 4)
	out := core.SetupTools(
		context.Background(),
		replpermissions.NewAutoApproveRequester(),
		tooling.NewDiffEmitter(),
		replaskuser.NewRequester(),
		nil,
		true,
		sink,
	)
	if out == nil {
		t.Fatal("expected non-nil activity channel when forwarding is enabled")
	}
	if !slices.Contains(core.RegisteredToolNames(), agentcore.ToolNameAskUser) {
		t.Fatal("ask_user should be registered with an interactive requester")
	}
}

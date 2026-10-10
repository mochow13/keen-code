package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keenmcp "github.com/mochow13/keen-code/internal/mcp"
)

func TestRootCommandsCustomMCPConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeRootConfig(t, home, `{"active_provider":"anthropic","active_model":"claude-sonnet-4-5","providers":{"anthropic":{"api_key":"test-key"}}}`)
	defaultPath := keenmcp.DefaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultPath, []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "custom mcp.json")
	if err := os.WriteFile(path, []byte(`{"servers":{"custom":{"command":"example-mcp"}}}`), 0600); err != nil {
		t.Fatal(err)
	}

	previous := newMCPManager
	t.Cleanup(func() { newMCPManager = previous })
	stop := errors.New("stop before starting MCP servers")
	calls := 0
	newMCPManager = func(opts ...keenmcp.Option) (keenmcp.Runtime, error) {
		calls++
		manager, err := keenmcp.NewManager(opts...)
		if err != nil {
			t.Fatal(err)
		}
		defer manager.Close()
		servers := manager.Servers()
		if len(servers) != 1 || servers[0].Name != "custom" {
			t.Fatalf("Servers() = %#v, want only custom", servers)
		}
		return nil, stop
	}
	for _, args := range [][]string{
		{"--mcp-config", path},
		{"--mcp-config", path, "run", "hello"},
		{"run", "--mcp-config", path, "hello"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := NewRootCommand("test")
			cmd.SetArgs(args)
			cmd.SetIn(strings.NewReader(""))
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			before := calls
			if err := cmd.Execute(); !errors.Is(err, stop) {
				t.Fatalf("Execute() error = %v, want MCP startup error", err)
			}
			if calls != before+1 {
				t.Fatal("command did not create MCP manager")
			}
		})
	}
}

func TestRootCommandsCustomMCPConfigErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeRootConfig(t, home, `{"active_provider":"anthropic","active_model":"claude-sonnet-4-5","providers":{"anthropic":{"api_key":"test-key"}}}`)
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range [][]string{nil, {"run"}} {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{args: []string{"--mcp-config", path}, want: "parse MCP config"},
			{args: []string{"--mcp-config", path + ".missing"}, want: "load MCP config"},
			{args: []string{"--mcp-config", ""}, want: "non-empty file path"},
			{args: []string{"--mcp-config"}, want: "flag needs an argument"},
		} {
			args := append(append([]string{}, prefix...), tc.args...)
			if len(prefix) != 0 && len(tc.args) == 2 {
				args = append(args, "hello")
			}
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				cmd := NewRootCommand("test")
				cmd.SetArgs(args)
				cmd.SetIn(strings.NewReader(""))
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("Execute() error = %v, want containing %q", err, tc.want)
				}
			})
		}
	}
}

func TestStartMCPRuntimeCustomConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, []byte(`{"servers":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	runtime, closeMCP, err := startMCPRuntime(context.Background(), keenmcp.WithConfigPath(path))
	defer closeMCP()
	if err != nil {
		t.Fatal(err)
	}
	if runtime == nil || len(runtime.Servers()) != 0 {
		t.Fatal("expected empty MCP runtime")
	}
}

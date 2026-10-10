package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCustomConfig(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigWithPathAbsolute(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dir with spaces")
	path := filepath.Join(dir, "custom.json")
	writeCustomConfig(t, path, `{"servers":{"custom":{"url":"https://example.com/mcp","auth":{"type":"none"}}}}`)
	cfg, err := LoadConfigWithPath(path)
	if err != nil {
		t.Fatalf("LoadConfigWithPath() error = %v", err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers["custom"].URL != "https://example.com/mcp" {
		t.Fatalf("Servers = %#v", cfg.Servers)
	}
}

func TestLoadConfigWithPathRelative(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root with spaces")
	writeCustomConfig(t, filepath.Join(root, "rel with spaces", "custom.json"), `{"servers":{"rel":{"command":"example-mcp"}}}`)
	t.Chdir(root)
	cfg, err := LoadConfigWithPath(filepath.Join("rel with spaces", "custom.json"))
	if err != nil {
		t.Fatalf("LoadConfigWithPath() error = %v", err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers["rel"].Command != "example-mcp" {
		t.Fatalf("Servers = %#v", cfg.Servers)
	}
}

func TestLoadConfigWithPathReplacesDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeCustomConfig(t, DefaultConfigPath(), `{"servers":{"fromdefault":{"url":"https://default.example/mcp"}}}`)
	custom := filepath.Join(t.TempDir(), "custom.json")
	writeCustomConfig(t, custom, `{"servers":{"fromcustom":{"url":"https://custom.example/mcp"}}}`)
	cfg, err := LoadConfigWithPath(custom)
	if err != nil {
		t.Fatalf("LoadConfigWithPath() error = %v", err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers["fromcustom"].URL != "https://custom.example/mcp" {
		t.Fatalf("Servers = %#v, want only custom", cfg.Servers)
	}
}

func TestLoadConfigWithPathEmptyUsesDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeCustomConfig(t, DefaultConfigPath(), `{"servers":{"dflt":{"url":"https://example.com/mcp"}}}`)
	cfg, err := LoadConfigWithPath("")
	if err != nil {
		t.Fatalf("LoadConfigWithPath() error = %v", err)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("Servers length = %d, want 1", len(cfg.Servers))
	}
}

func TestLoadConfigWithPathMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no such dir", "missing.json")
	_, err := LoadConfigWithPath(missing)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("LoadConfigWithPath() error = %v, want path %q", err, missing)
	}
}

func TestLoadConfigWithPathInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	writeCustomConfig(t, path, `{`)
	_, err := LoadConfigWithPath(path)
	if err == nil || !strings.Contains(err.Error(), "parse MCP config") {
		t.Fatalf("LoadConfigWithPath() error = %v, want parse error", err)
	}
	invalid := filepath.Join(t.TempDir(), "invalid.json")
	writeCustomConfig(t, invalid, `{"servers":{"bad":{"url":"ftp://example.com/mcp"}}}`)
	_, err = LoadConfigWithPath(invalid)
	if err == nil || !strings.Contains(err.Error(), "url must use http or https") {
		t.Fatalf("LoadConfigWithPath() error = %v, want validation error", err)
	}
}

func TestManagerWithConfigPathLoadsOnlyCustom(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeCustomConfig(t, DefaultConfigPath(), `{"servers":{"fromdefault":{"url":"https://default.example/mcp"}}}`)
	custom := filepath.Join(t.TempDir(), "custom.json")
	writeCustomConfig(t, custom, `{"servers":{"fromcustom":{"url":"https://custom.example/mcp"}}}`)
	manager, err := NewManager(WithConfigPath(custom))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	statuses := manager.Servers()
	if len(statuses) != 1 || statuses[0].Name != "fromcustom" {
		t.Fatalf("Servers() = %#v, want only fromcustom", statuses)
	}
}

func TestManagerWithConfigPathMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	missing := filepath.Join(t.TempDir(), "missing.json")
	_, err := NewManager(WithConfigPath(missing))
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("NewManager() error = %v, want path %q", err, missing)
	}
}
